package metadata

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dj/tag-track-cli/internal/cache"
)

func createDummyAudioFile(t *testing.T, dir, filename string) string {
	t.Helper()
	filePath := filepath.Join(dir, filename)
	cmd := exec.Command("ffmpeg", "-v", "quiet", "-hide_banner", "-f", "lavfi", "-i", "anullsrc=r=48000:cl=stereo", "-t", "1", "-y", filePath)
	if err := cmd.Run(); err != nil {
		t.Skipf("ffmpeg not available or failed to create dummy audio file: %v", err)
	}
	return filePath
}

func TestApplyMetadataToLocalTrack_ZeroTranscodePreservesNativeFormat(t *testing.T) {
	outDir := t.TempDir()
	srcDir := t.TempDir()

	tests := []struct {
		name          string
		inputFilename string
		meta          TrackMetadataResult
		wantFileName  string
		wantExt       string
	}{
		{
			name:          "mp3 input stream copied as mp3",
			inputFilename: "Gopnik.mp3",
			meta: TrackMetadataResult{
				Title:       "Gopnik",
				Artist:      "DJ Blyatman",
				Album:       "Gopnik - Single",
				ReleaseYear: "2020",
				Genre:       "Hardbass",
				Source:      "iTunes API",
			},
			wantFileName: "DJ Blyatman - Gopnik.mp3",
			wantExt:      ".mp3",
		},
		{
			name:          "m4a input stream copied as m4a",
			inputFilename: "Space X.m4a",
			meta: TrackMetadataResult{
				Title:       "Space X",
				Artist:      "Boris Brejcha",
				Album:       "Space X - Single",
				ReleaseYear: "2024",
				Genre:       "Minimal Techno",
				Source:      "iTunes API",
			},
			wantFileName: "Boris Brejcha - Space X.m4a",
			wantExt:      ".m4a",
		},
		{
			name:          "unknown artist falls back to clean filename with native extension",
			inputFilename: "Raw Track.mp3",
			meta: TrackMetadataResult{
				Title:  "Raw Track",
				Artist: "Unknown Artist",
				Source: "Filename Fallback",
			},
			wantFileName: "Raw Track.mp3",
			wantExt:      ".mp3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inputPath := createDummyAudioFile(t, srcDir, tt.inputFilename)

			taggedPath, err := ApplyMetadataToLocalTrack(context.Background(), inputPath, tt.meta, outDir)
			if err != nil {
				t.Fatalf("ApplyMetadataToLocalTrack failed: %v", err)
			}

			if filepath.Base(taggedPath) != tt.wantFileName {
				t.Errorf("ApplyMetadataToLocalTrack output filename = %q, want %q", filepath.Base(taggedPath), tt.wantFileName)
			}

			if !strings.HasSuffix(taggedPath, tt.wantExt) {
				t.Errorf("ApplyMetadataToLocalTrack file extension is not %s: %q", tt.wantExt, taggedPath)
			}

			if _, err := os.Stat(taggedPath); os.IsNotExist(err) {
				t.Errorf("tagged output file does not exist on disk: %s", taggedPath)
			}
		})
	}
}

func TestApplyMetadataToLocalTrack_TmpCleanup(t *testing.T) {
	outDir := t.TempDir()
	srcDir := t.TempDir()

	inputPath := createDummyAudioFile(t, srcDir, "CleanupTest.mp3")

	meta := TrackMetadataResult{
		Title:  "Cleanup Test",
		Artist: "Test Artist",
		Source: "Test",
	}

	dotTmpPath := ".tmp"
	_, errStat := os.Stat(dotTmpPath)
	existedBefore := !os.IsNotExist(errStat)

	_, err := ApplyMetadataToLocalTrack(context.Background(), inputPath, meta, outDir)
	if err != nil {
		t.Fatalf("ApplyMetadataToLocalTrack failed: %v", err)
	}

	if !existedBefore {
		if _, err := os.Stat(dotTmpPath); !os.IsNotExist(err) {
			t.Errorf(".tmp directory was created during execution and not cleaned up")
		}
	}
}

func TestApplyMetadataToLocalTrack_PreservesOriginDateAndProvenance(t *testing.T) {
	outDir := t.TempDir()
	srcDir := t.TempDir()

	testFiles := []string{"test_provenance.m4a", "test_provenance.mp3"}

	for _, fname := range testFiles {
		t.Run(fname, func(t *testing.T) {
			inputPath := createDummyAudioFile(t, srcDir, fname)

			meta := TrackMetadataResult{
				Title:          "Provenance Track",
				Artist:         "Provenance Artist",
				Album:          "Provenance Album",
				ReleaseDate:    "2023-04-15",
				ReleaseYear:    "2023",
				Genre:          "Techno",
				Source:         "Shazam API",
				AudioSourceURL: "https://example.com/audio/sample",
				FetchedAt:      time.Date(2026, 8, 23, 10, 0, 0, 0, time.UTC),
			}

			taggedPath, err := ApplyMetadataToLocalTrack(context.Background(), inputPath, meta, outDir)
			if err != nil {
				t.Fatalf("ApplyMetadataToLocalTrack failed: %v", err)
			}

			// Verify tags via ffprobe
			cmd := exec.Command("ffprobe",
				"-v", "quiet",
				"-print_format", "json",
				"-show_format",
				taggedPath,
			)
			var stdout bytes.Buffer
			cmd.Stdout = &stdout
			if err := cmd.Run(); err != nil {
				t.Fatalf("ffprobe failed: %v", err)
			}

			var probe struct {
				Format struct {
					Tags map[string]string `json:"tags"`
				} `json:"format"`
			}
			if err := json.Unmarshal(stdout.Bytes(), &probe); err != nil {
				t.Fatalf("unmarshal ffprobe json failed: %v", err)
			}

			tags := probe.Format.Tags
			findTag := func(names ...string) string {
				for _, name := range names {
					for k, v := range tags {
						if strings.EqualFold(k, name) {
							return v
						}
					}
				}
				return ""
			}

			gotTitle := findTag("title", "TIT2")
			if gotTitle != "Provenance Track" {
				t.Errorf("tag title = %q, want 'Provenance Track'", gotTitle)
			}

			gotArtist := findTag("artist", "TPE1")
			if gotArtist != "Provenance Artist" {
				t.Errorf("tag artist = %q, want 'Provenance Artist'", gotArtist)
			}

			gotDate := findTag("date", "year", "TDRC", "TYER")
			if !strings.HasPrefix(gotDate, "2023-04-15") && !strings.HasPrefix(gotDate, "2023") {
				t.Errorf("tag date = %q, expected full origin release date starting with 2023", gotDate)
			}

			gotComment := findTag("comment", "COMM", "description")
			if !strings.Contains(gotComment, "Source: https://example.com/audio/sample") {
				t.Errorf("comment tag missing source url provenance: %q", gotComment)
			}
			if !strings.Contains(gotComment, "Metadata: Shazam API") {
				t.Errorf("comment tag missing metadata provider provenance: %q", gotComment)
			}
			if !strings.Contains(gotComment, "Tagged: 2026-08-23") {
				t.Errorf("comment tag missing acquisition date provenance: %q", gotComment)
			}
		})
	}
}

func TestApplyMetadataToLocalTrack_WithCoverArt(t *testing.T) {
	outDir := t.TempDir()
	srcDir := t.TempDir()

	imgServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cmd := exec.Command("ffmpeg", "-v", "quiet", "-hide_banner", "-f", "lavfi", "-i", "color=c=red:s=400x400", "-vframes", "1", "-f", "image2pipe", "-vcodec", "mjpeg", "-")
		var out bytes.Buffer
		cmd.Stdout = &out
		if err := cmd.Run(); err == nil {
			w.Header().Set("Content-Type", "image/jpeg")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(out.Bytes())
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer imgServer.Close()

	inputPath := createDummyAudioFile(t, srcDir, "CoverTrack.m4a")

	meta := TrackMetadataResult{
		Title:       "Cover Track",
		Artist:      "Cover Artist",
		CoverArtURL: imgServer.URL + "/cover.jpg",
		Source:      "iTunes API",
	}

	taggedPath, err := ApplyMetadataToLocalTrack(context.Background(), inputPath, meta, outDir, true)
	if err != nil {
		t.Fatalf("ApplyMetadataToLocalTrack with cover art failed: %v", err)
	}

	// Verify attached picture stream with ffprobe
	cmd := exec.Command("ffprobe",
		"-v", "quiet",
		"-print_format", "json",
		"-show_streams",
		taggedPath,
	)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		t.Fatalf("ffprobe on tagged track failed: %v", err)
	}

	var probe struct {
		Streams []struct {
			CodecType   string            `json:"codec_type"`
			CodecName   string            `json:"codec_name"`
			Width       int               `json:"width"`
			Height      int               `json:"height"`
			Disposition map[string]int    `json:"disposition"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &probe); err != nil {
		t.Fatalf("unmarshaling ffprobe streams json failed: %v", err)
	}

	hasCover := false
	for _, stream := range probe.Streams {
		if stream.CodecType == "video" || stream.Disposition["attached_pic"] == 1 {
			hasCover = true
			if stream.Width != 1400 || stream.Height != 1400 {
				t.Errorf("embedded artwork dimensions = %dx%d, want 1400x1400 normalized square", stream.Width, stream.Height)
			}
			break
		}
	}

	if !hasCover {
		t.Errorf("no embedded cover art / attached_pic stream found in %s", taggedPath)
	}

	// Test cached cover art branch
	cacheDir := t.TempDir()
	cacheInst := cache.NewInDir(cacheDir, true)
	inputPath2 := createDummyAudioFile(t, srcDir, "CoverTrack2.m4a")
	_, err2 := ApplyMetadataToLocalTrack(context.Background(), inputPath2, meta, outDir, true, cacheInst)
	if err2 != nil {
		t.Fatalf("ApplyMetadataToLocalTrack with cache failed: %v", err2)
	}

	// Test cache hit branch
	inputPath3 := createDummyAudioFile(t, srcDir, "CoverTrack3.m4a")
	_, err3 := ApplyMetadataToLocalTrack(context.Background(), inputPath3, meta, outDir, cacheInst)
	if err3 != nil {
		t.Fatalf("ApplyMetadataToLocalTrack on cache hit failed: %v", err3)
	}

	// Test embedded video fallback when URL is unreachable
	inputPath4 := createDummyAudioFile(t, srcDir, "CoverTrack4.m4a")
	metaBadURL := TrackMetadataResult{
		Title:       "Bad Art",
		Artist:      "Artist",
		CoverArtURL: "http://127.0.0.1:54321/unreachable.jpg",
	}
	_, err4 := ApplyMetadataToLocalTrack(context.Background(), inputPath4, metaBadURL, outDir)
	if err4 != nil {
		t.Fatalf("ApplyMetadataToLocalTrack with bad art URL failed: %v", err4)
	}

	// Test empty URL
	inputPath5 := createDummyAudioFile(t, srcDir, "CoverTrack5.m4a")
	metaNoURL := TrackMetadataResult{
		Title:  "No Art",
		Artist: "Artist",
	}
	_, err5 := ApplyMetadataToLocalTrack(context.Background(), inputPath5, metaNoURL, outDir)
	if err5 != nil {
		t.Fatalf("ApplyMetadataToLocalTrack with no art failed: %v", err5)
	}
}

func TestNormalizeCoverArtToSquare(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	t.Run("empty path", func(t *testing.T) {
		_, err := NormalizeCoverArtToSquare(ctx, "", tempDir)
		if err == nil {
			t.Fatal("expected error for empty image path")
		}
	})

	t.Run("nonexistent path", func(t *testing.T) {
		_, err := NormalizeCoverArtToSquare(ctx, filepath.Join(tempDir, "missing.jpg"), tempDir)
		if err == nil {
			t.Fatal("expected error for nonexistent image path")
		}
	})

	t.Run("crops 16:9 to 1:1 square", func(t *testing.T) {
		rectPath := filepath.Join(tempDir, "rect.jpg")
		cmd := exec.Command("ffmpeg", "-v", "quiet", "-hide_banner", "-f", "lavfi", "-i", "color=c=blue:s=1920x1080", "-vframes", "1", "-y", rectPath)
		if err := cmd.Run(); err != nil {
			t.Skipf("ffmpeg not available or failed to generate test image: %v", err)
		}

		squarePath, err := NormalizeCoverArtToSquare(ctx, rectPath, tempDir)
		if err != nil {
			t.Fatalf("NormalizeCoverArtToSquare failed: %v", err)
		}

		// Probe the normalized image dimensions with ffprobe
		probeCmd := exec.Command("ffprobe",
			"-v", "quiet",
			"-print_format", "json",
			"-show_streams",
			squarePath,
		)
		var stdout bytes.Buffer
		probeCmd.Stdout = &stdout
		if err := probeCmd.Run(); err != nil {
			t.Fatalf("ffprobe on square cover failed: %v", err)
		}

		var probe struct {
			Streams []struct {
				Width  int `json:"width"`
				Height int `json:"height"`
			} `json:"streams"`
		}
		if err := json.Unmarshal(stdout.Bytes(), &probe); err != nil {
			t.Fatalf("unmarshaling ffprobe json failed: %v", err)
		}

		if len(probe.Streams) == 0 {
			t.Fatal("no video/image stream found in normalized square output")
		}

		w := probe.Streams[0].Width
		h := probe.Streams[0].Height
		if w != 1400 || h != 1400 {
			t.Errorf("normalized square dimensions = %dx%d, want 1400x1400", w, h)
		}
	})
}
