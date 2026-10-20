package metadata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexgorbatchev/tag-track-cli/internal/cache"
)

func TestDownloadArtwork(t *testing.T) {
	for _, tc := range []struct {
		name    string
		status  int
		body    string
		cached  bool
		wantErr string
	}{
		{"download", http.StatusOK, "image bytes", false, ""},
		{"cache", http.StatusOK, "cached image", true, ""},
		{"missing", http.StatusNotFound, "missing", false, "status 404"},
		{"empty", http.StatusOK, "", false, "empty artwork payload"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.WriteHeader(tc.status)
				if _, err := w.Write([]byte(tc.body)); err != nil {
					t.Error(err)
				}
			}))
			defer srv.Close()
			restore := SetDefaultHTTPClient(srv.Client())
			defer restore()
			var c *cache.Cache
			if tc.cached {
				c = cache.NewInDir(t.TempDir(), true)
			}
			dir := t.TempDir()
			path, err := DownloadArtwork(context.Background(), srv.URL, dir, c)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(path)
			if err != nil || string(data) != tc.body {
				t.Fatalf("download = %q, %v", data, err)
			}
			if tc.cached {
				again, err := DownloadArtwork(context.Background(), srv.URL, dir, c)
				if err != nil || again != path || calls != 1 {
					t.Fatalf("cache = %q, calls %d, error %v", again, calls, err)
				}
			}
		})
	}
	t.Run("local missing", func(t *testing.T) {
		if _, err := DownloadArtwork(context.Background(), filepath.Join(t.TempDir(), "missing.png"), t.TempDir(), nil); err == nil {
			t.Fatal("missing image accepted")
		}
	})
}

func TestArtworkFailuresPreserveSource(t *testing.T) {
	dir := t.TempDir()
	image := filepath.Join(dir, "cover.png")
	if err := os.WriteFile(image, []byte("invalid image bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, location := range []string{"", "http://[invalid", image} {
		_, err := DownloadArtwork(context.Background(), location, filepath.Join(dir, "absent"), nil)
		if err == nil {
			t.Fatalf("accepted failed artwork %q", location)
		}
	}
	for _, input := range []string{"", filepath.Join(dir, "missing"), image} {
		if _, err := NormalizeCoverArtToSquare(context.Background(), input, dir); err == nil {
			t.Fatalf("accepted invalid image %q", input)
		}
	}
	data, err := os.ReadFile(image)
	if err != nil || string(data) != "invalid image bytes" {
		t.Fatalf("source changed: %q %v", data, err)
	}
}

func TestDownloadArtworkHTTPFailures(t *testing.T) {
	for _, failure := range []string{"truncated", "destination", "cancelled"} {
		t.Run(failure, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if failure == "truncated" {
					w.Header().Set("Content-Length", "100")
				}
				_, _ = w.Write([]byte("image"))
			}))
			defer srv.Close()
			restore := SetDefaultHTTPClient(srv.Client())
			defer restore()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if failure == "cancelled" {
				cancel()
			}
			dir := t.TempDir()
			if failure == "destination" {
				dir = filepath.Join(dir, "absent")
			}
			if _, err := DownloadArtwork(ctx, srv.URL, dir, nil); err == nil {
				t.Fatal("artwork failure hidden")
			}
		})
	}
}

func TestRequiredArtworkFailureDoesNotMutateAudio(t *testing.T) {
	dir := t.TempDir()
	file := createDummyAudioFile(t, dir, "source.mp3")
	before, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	badArt := filepath.Join(dir, "invalid.jpg")
	if err := os.WriteFile(badArt, []byte("not an image"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, art := range []string{filepath.Join(dir, "missing.jpg"), badArt} {
		_, err := ApplyMetadataToLocalTrack(context.Background(), file, TrackMetadataResult{Title: "Retagged", Artist: "Artist", CoverArtURL: art, RequireArtwork: true}, dir)
		if err == nil {
			t.Fatal("explicit artwork error lost")
		}
		after, err := os.ReadFile(file)
		if err != nil || string(after) != string(before) {
			t.Fatal("failed artwork modified source")
		}
		if _, err := os.Stat(filepath.Join(dir, "Artist - Retagged.mp3")); !os.IsNotExist(err) {
			t.Fatalf("failed artwork installed output: %v", err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ApplyMetadataToLocalTrack(ctx, file, TrackMetadataResult{Title: "Cancelled"}, dir); err == nil {
		t.Fatal("cancelled tagging succeeded")
	}
	after, err := os.ReadFile(file)
	if err != nil || string(after) != string(before) {
		t.Fatal("cancelled tagging modified source")
	}
	if _, err := ApplyMetadataToLocalTrack(context.Background(), file, TrackMetadataResult{}, file); err == nil {
		t.Fatal("file accepted as output directory")
	}
}
