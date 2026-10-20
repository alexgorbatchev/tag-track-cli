package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alexgorbatchev/tag-track-cli/internal/cache"
	"github.com/alexgorbatchev/tag-track-cli/internal/deps"
	"github.com/alexgorbatchev/tag-track-cli/internal/metadata"
)

func runTagJSON(t *testing.T, args ...string) (tagBatch, error) {
	t.Helper()
	output, runErr := executeCommand(newRootCommand(), args...)
	var batch tagBatch
	if err := json.Unmarshal([]byte(output), &batch); err != nil {
		t.Fatalf("JSON %q: %v", output, err)
	}
	return batch, runErr
}

type brokenInput struct{}

func (brokenInput) Read([]byte) (int, error) { return 0, errors.New("input stream failed") }

func TestCommandsRejectFailedInputAndMissingDependencies(t *testing.T) {
	_, cleanup := setupMainTestEnv(t)
	defer cleanup()
	old := stdinReader
	defer func() { stdinReader = old }()
	for _, command := range [][]string{nil, {"track", "update"}, {"track", "inspect"}, {"artwork", "embed"}} {
		stdinReader = brokenInput{}
		args := append(append([]string{}, command...), "-")
		_, err := executeCommand(newRootCommand(), args...)
		if err == nil || !strings.Contains(err.Error(), "input stream failed") {
			t.Fatalf("%v input failure: %v", command, err)
		}
		stdinReader = strings.NewReader("")
		if _, err := executeCommand(newRootCommand(), args...); err != nil {
			t.Fatalf("%v empty stdin: %v", command, err)
		}
	}
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("AGENT", "1")
	restore := deps.SetDefaultRunner(func(context.Context, string, ...string) ([]byte, error) { return nil, errors.New("dependency missing") })
	defer restore()
	for _, command := range [][]string{nil, {"track", "update"}, {"track", "inspect"}, {"artwork", "embed", "--artwork=cover.png"}} {
		args := append(append([]string{}, command...), "--json", "input.m4a")
		out, err := executeCommand(newRootCommand(), args...)
		var batch tagBatch
		if jsonErr := json.Unmarshal([]byte(out), &batch); err == nil || jsonErr != nil || batch.Status != "error" || len(batch.Results) != 1 || batch.Results[0].InputFile != "input.m4a" || batch.Results[0].Error == "" {
			t.Fatalf("%v lost structured preflight failure: %q %v", command, out, err)
		}
	}
}

func TestJSONRefreshBypassesCachedMetadataAndDryRunPreservesAudio(t *testing.T) {
	dir, cleanup := setupMainTestEnv(t)
	defer cleanup()
	file := createTestAudioFile(t, dir, "Artist - Title.m4a")
	before, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	c, err := cache.New(true)
	if err != nil {
		t.Fatal(err)
	}
	key := fmt.Sprintf("%s:Artist - Title:Artist:Title", file)
	if err := c.Put("metadata", key, metadata.TrackMetadataResult{Artist: "Cached artist", Title: "Cached title", Source: "seed"}, time.Hour); err != nil {
		t.Fatal(err)
	}
	for _, refresh := range []bool{false, true} {
		args := []string{"track", "update", "--json", "--dry-run", file}
		if refresh {
			args = append(args, "--refresh")
		}
		batch, err := runTagJSON(t, args...)
		if err != nil || len(batch.Results) != 1 || !batch.Results[0].DryRun || batch.Results[0].OutputFile != "" {
			t.Fatalf("dry run: %+v %v", batch, err)
		}
		got := batch.Results[0].Metadata
		if got == nil {
			t.Fatal("missing preview metadata")
		}
		if refresh && strings.Contains(got.Source, "cached") {
			t.Fatalf("refresh reused metadata: %+v", got)
		}
		if !refresh && got.Title != "Cached title" {
			t.Fatalf("cache not used: %+v", got)
		}
	}
	after, err := os.ReadFile(file)
	if err != nil || string(after) != string(before) {
		t.Fatal("dry run modified audio")
	}
}

func TestJSONUpdatePartialBatchKeepsSuccessfulOutput(t *testing.T) {
	dir, cleanup := setupMainTestEnv(t)
	defer cleanup()
	file := createTestAudioFile(t, dir, "input.m4a")
	broken := filepath.Join(dir, "broken.m4a")
	if err := os.WriteFile(broken, []byte("invalid media"), 0600); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(dir, "missing.m4a")
	batch, err := runTagJSON(t, "track", "update", "--json", "--artist=Chosen Artist", "--title=Chosen Title", "--album=Chosen Album", "--date=2001-02-03", "--genre=House", "--comment=Personal", "--source-url=https://example.com/source", "--out-dir", filepath.Join(dir, "output"), missing, broken, file)
	if err == nil || batch.Status != "error" || len(batch.Results) != 3 {
		t.Fatalf("partial batch: %+v %v", batch, err)
	}
	for i, input := range []string{missing, broken, file} {
		if batch.Results[i].InputFile != input {
			t.Fatalf("input order lost: %+v", batch.Results)
		}
		if i < 2 && (batch.Results[i].Status != "error" || batch.Results[i].Error == "") {
			t.Fatalf("failure lost: %+v", batch.Results[i])
		}
	}
	result := batch.Results[2]
	got, err := metadata.InspectAudioTrack(context.Background(), result.OutputFile, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "success" || got.Artist != "Chosen Artist" || got.Title != "Chosen Title" || got.Album != "Chosen Album" || got.Date != "2001-02-03" || got.Genre != "House" || got.Comment != "Personal" {
		t.Fatalf("tags lost: %+v", got)
	}
	for _, agent := range []string{"0", "1"} {
		t.Setenv("AGENT", agent)
		out, err := executeCommand(newRootCommand(), "track", "inspect", result.OutputFile)
		if err != nil {
			t.Fatal(err)
		}
		for _, value := range []string{"Chosen Title", "Chosen Artist", "Chosen Album", "2001-02-03", "House", "Personal"} {
			if !strings.Contains(out, value) {
				t.Fatalf("inspection lost %q: %s", value, out)
			}
		}
	}
}

func TestJSONUpdateClearedDateAndFailedDestination(t *testing.T) {
	dir, cleanup := setupMainTestEnv(t)
	defer cleanup()
	file := createTestAudioFile(t, dir, "input.m4a")
	batch, err := runTagJSON(t, "update", "--json", "--dry-run", "--artist=", "--title=", "--date=", "--genre=", file)
	if err != nil || len(batch.Results) != 1 {
		t.Fatalf("clear preview: %+v %v", batch, err)
	}
	meta := batch.Results[0].Metadata
	if meta == nil || meta.Artist != "" || meta.Title != "" || meta.ReleaseDate != "" || meta.ReleaseYear != "" || meta.Genre != "" {
		t.Fatalf("explicit clears lost: %+v", meta)
	}
	batch, err = runTagJSON(t, "update", "--json", "--out-dir", file, file)
	if err == nil || len(batch.Results) != 1 || !strings.Contains(batch.Results[0].Error, "creating output directory") {
		t.Fatalf("destination failure lost: %+v %v", batch, err)
	}
}
