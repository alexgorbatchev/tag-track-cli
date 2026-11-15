package main

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/alexgorbatchev/tag-track-cli/internal/metadata"
)

var existingTags = map[string]string{
	"title":     "Undertow Signal",
	"artist":    "Fenwick Rao",
	"album":     "Fold Sessions",
	"date":      "2026",
	"genre":     "Tech House",
	"comment":   "Peak-time opener",
	"publisher": "Fold Recordings",
}

func createTaggedAudioFile(t *testing.T, dir, filename string) string {
	t.Helper()
	path := filepath.Join(dir, filename)
	args := []string{"-v", "quiet", "-hide_banner", "-f", "lavfi", "-i", "anullsrc=r=44100:cl=stereo", "-t", "1"}
	for _, key := range []string{"title", "artist", "album", "date", "genre", "comment", "publisher"} {
		args = append(args, "-metadata", key+"="+existingTags[key])
	}
	if strings.HasSuffix(filename, ".mp3") {
		args = append(args, "-id3v2_version", "3")
	}
	if err := exec.Command("ffmpeg", append(args, "-y", path)...).Run(); err != nil {
		t.Skipf("ffmpeg cannot create %s: %v", filename, err)
	}
	return path
}

// forbidNetwork fails the test on any lookup request and reports how many were attempted.
func forbidNetwork(t *testing.T) *atomic.Int32 {
	t.Helper()
	var requests atomic.Int32
	restore := metadata.SetDefaultHTTPClient(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requests.Add(1)
		t.Errorf("unexpected lookup request to %s", req.URL.Host)
		return nil, http.ErrHandlerTimeout
	})})
	t.Cleanup(restore)
	return &requests
}

func inspectTags(t *testing.T, path string) map[string]string {
	t.Helper()
	info, err := metadata.InspectAudioTrack(context.Background(), path, nil)
	if err != nil {
		t.Fatal(err)
	}
	tags := map[string]string{}
	for key, value := range info.RawTags {
		tags[strings.ToLower(key)] = value
	}
	return tags
}

func TestNoLookupWritesOnlyExplicitTags(t *testing.T) {
	for _, filename := range []string{"Fenwick Rao - Undertow Signal.flac", "Fenwick Rao - Undertow Signal.mp3"} {
		t.Run(filepath.Ext(filename), func(t *testing.T) {
			_, cleanup := setupMainTestEnv(t)
			defer cleanup()
			requests := forbidNetwork(t)
			dir := t.TempDir()
			file := createTaggedAudioFile(t, dir, filename)
			before := inspectTags(t, file)

			batch, err := runTagJSON(t, "track", "update", "--json", "--no-lookup", "--title", "Undertow Signal (Extended Mix)", file)
			if err != nil || len(batch.Results) != 1 || batch.Results[0].Status != "success" {
				t.Fatalf("no-lookup write: %+v %v", batch, err)
			}
			if batch.Results[0].OutputFile != file {
				t.Fatalf("no-lookup write renamed the file: %q", batch.Results[0].OutputFile)
			}
			meta := batch.Results[0].Metadata
			if meta == nil || meta.Title != "Undertow Signal (Extended Mix)" || meta.Artist != "Fenwick Rao" || meta.Album != "Fold Sessions" ||
				meta.Genre != "Tech House" || meta.ReleaseDate != "2026" || meta.Comment == nil || *meta.Comment != "Peak-time opener" ||
				meta.Source != "Existing tags" {
				t.Fatalf("reported metadata: %+v", meta)
			}
			after := inspectTags(t, file)
			if after["title"] != "Undertow Signal (Extended Mix)" {
				t.Fatalf("title not written: %v", after)
			}
			for _, key := range []string{"artist", "album", "date", "genre", "comment", "publisher"} {
				if after[key] != before[key] {
					t.Fatalf("%s changed from %q to %q", key, before[key], after[key])
				}
			}
			if _, ok := after["year"]; ok != (before["year"] != "") {
				t.Fatalf("year tag appeared: %v", after)
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 1 {
				t.Fatalf("no-lookup write left extra files: %v %v", entries, err)
			}
			if requests.Load() != 0 {
				t.Fatalf("%d lookup requests", requests.Load())
			}
		})
	}
}

func TestNoLookupPreviewReportsExistingTagsWithEdits(t *testing.T) {
	_, cleanup := setupMainTestEnv(t)
	defer cleanup()
	requests := forbidNetwork(t)
	file := createTaggedAudioFile(t, t.TempDir(), "input.flac")
	before, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}

	batch, err := runTagJSON(t, "track", "update", "--json", "--dry-run", "--no-lookup", "--genre", "Minimal", "--comment=", "--date", "2024-05-10", file)
	if err != nil || len(batch.Results) != 1 || !batch.Results[0].DryRun {
		t.Fatalf("no-lookup preview: %+v %v", batch, err)
	}
	meta := batch.Results[0].Metadata
	if meta == nil || meta.Title != "Undertow Signal" || meta.Artist != "Fenwick Rao" || meta.Album != "Fold Sessions" ||
		meta.Genre != "Minimal" || meta.ReleaseDate != "2024-05-10" || meta.ReleaseYear != "2024" || meta.Comment == nil || *meta.Comment != "" ||
		meta.CoverArtURL != "" || meta.Source != "Existing tags" {
		t.Fatalf("preview metadata: %+v", meta)
	}
	after, err := os.ReadFile(file)
	if err != nil || string(after) != string(before) {
		t.Fatal("no-lookup preview modified audio")
	}
	if requests.Load() != 0 {
		t.Fatalf("%d lookup requests", requests.Load())
	}
}

func TestNoLookupClearsAndArtworkAreExplicit(t *testing.T) {
	_, cleanup := setupMainTestEnv(t)
	defer cleanup()
	dir := t.TempDir()
	file := createTaggedAudioFile(t, dir, "input.flac")
	cover := filepath.Join(dir, "cover.png")
	if err := exec.Command("ffmpeg", "-v", "quiet", "-f", "lavfi", "-i", "color=c=red:s=32x24", "-frames:v", "1", "-y", cover).Run(); err != nil {
		t.Skipf("ffmpeg cannot create artwork: %v", err)
	}
	out := t.TempDir()

	batch, err := runTagJSON(t, "track", "update", "--json", "--no-lookup", "--album=", "--artwork", cover, "-o", out, file)
	if err != nil || len(batch.Results) != 1 || batch.Results[0].Status != "success" {
		t.Fatalf("no-lookup clear: %+v %v", batch, err)
	}
	written := batch.Results[0].OutputFile
	if written != filepath.Join(out, "input.flac") {
		t.Fatalf("output path %q", written)
	}
	tags := inspectTags(t, written)
	if tags["album"] != "" || tags["artist"] != "Fenwick Rao" || tags["comment"] != "Peak-time opener" {
		t.Fatalf("tags after clear: %v", tags)
	}
	info, err := metadata.InspectAudioTrack(context.Background(), written, nil)
	if err != nil || !info.HasArtwork || info.ArtworkWidth != 1400 || info.ArtworkHeight != 1400 {
		t.Fatalf("explicit artwork: %+v %v", info, err)
	}

	kept, err := runTagJSON(t, "track", "update", "--json", "--no-lookup", "--genre", "House", written)
	if err != nil || len(kept.Results) != 1 || kept.Results[0].Status != "success" {
		t.Fatalf("no-lookup rewrite: %+v %v", kept, err)
	}
	info, err = metadata.InspectAudioTrack(context.Background(), written, nil)
	if err != nil || !info.HasArtwork || info.ArtworkWidth != 1400 || info.Genre != "House" {
		t.Fatalf("existing artwork lost: %+v %v", info, err)
	}
}

func TestNoLookupRejectsProvenanceAndUnreadableFiles(t *testing.T) {
	_, cleanup := setupMainTestEnv(t)
	defer cleanup()
	forbidNetwork(t)
	dir := t.TempDir()
	file := createTaggedAudioFile(t, dir, "input.flac")

	batch, err := runTagJSON(t, "track", "update", "--json", "--no-lookup", "--source-url", "https://example.com/a", "--title", "New", file)
	if err == nil || batch.Status != "error" || len(batch.Results) != 1 || batch.Results[0].Error != "--source-url records lookup provenance and cannot be combined with --no-lookup" {
		t.Fatalf("source URL accepted: %+v %v", batch, err)
	}
	if inspectTags(t, file)["title"] != "Undertow Signal" {
		t.Fatal("rejected request wrote the file")
	}

	broken := filepath.Join(dir, "unreadable.flac")
	if err := os.WriteFile(broken, []byte("audio"), 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(broken, 0o600) })
	batch, err = runTagJSON(t, "track", "update", "--json", "--no-lookup", "--title", "New", broken, file)
	if err == nil || len(batch.Results) != 2 || batch.Results[0].Status != "error" || !strings.HasPrefix(batch.Results[0].Error, "reading existing tags: ") ||
		batch.Results[1].Status != "success" {
		t.Fatalf("unreadable file: %+v %v", batch, err)
	}
	if inspectTags(t, file)["title"] != "New" {
		t.Fatal("batch stopped after an unreadable file")
	}

	t.Setenv("AGENT", "1")
	out, err := executeCommand(newRootCommand(), "track", "update", "--no-lookup", "--artist", "Agent", file)
	if err != nil || !strings.Contains(out, "artist: Agent") || !strings.Contains(out, "source: Existing tags") {
		t.Fatalf("agent output: %s %v", out, err)
	}

	t.Setenv("AGENT", "0")
	out, err = executeCommand(newRootCommand(), "track", "update", "--no-lookup", "--verbose", "--genre", "House", "--comment", "Closer", file)
	if err != nil || !strings.Contains(out, "  - edited: genre, comment\n") || strings.Contains(out, "matched:") {
		t.Fatalf("human output: %s %v", out, err)
	}
	out, err = executeCommand(newRootCommand(), "track", "update", "--no-lookup", "--verbose", file)
	if err != nil || !strings.Contains(out, "  - edited: no tags\n") {
		t.Fatalf("human output without edits: %s %v", out, err)
	}
	if tags := inspectTags(t, file); tags["genre"] != "House" || tags["comment"] != "Closer" || tags["artist"] != "Agent" {
		t.Fatalf("tags after human-mode writes: %v", tags)
	}
}
