package metadata

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestInspectAudioTrack(t *testing.T) {
	t.Run("empty path", func(t *testing.T) {
		_, err := InspectAudioTrack(context.Background(), "", nil)
		if err == nil {
			t.Fatal("expected error on empty path")
		}
	})

	t.Run("nonexistent file", func(t *testing.T) {
		_, err := InspectAudioTrack(context.Background(), "/nonexistent/path.m4a", nil)
		if err == nil {
			t.Fatal("expected error on nonexistent file")
		}
	})

	t.Run("valid probed audio with tags and square artwork", func(t *testing.T) {
		tmpDir := t.TempDir()
		fakeFile := filepath.Join(tmpDir, "test.m4a")
		_ = os.WriteFile(fakeFile, []byte("audio"), 0644)

		mockJSON := `{
			"format": {
				"filename": "` + fakeFile + `",
				"format_name": "mov,mp4,m4a,3gp,3g2,mj2",
				"duration": "180.5",
				"size": "5242880",
				"bit_rate": "256000",
				"tags": {
					"title": "Space X",
					"artist": "Boris Brejcha",
					"album": "Level One",
					"date": "2024-01-01",
					"genre": "High-Tech Minimal",
					"comment": "Source: direct | Metadata: iTunes"
				}
			},
			"streams": [
				{
					"codec_type": "audio",
					"codec_name": "aac",
					"sample_rate": "44100",
					"channels": 2,
					"bit_rate": "256000"
				},
				{
					"codec_type": "video",
					"codec_name": "mjpeg",
					"width": 1400,
					"height": 1400,
					"disposition": {
						"attached_pic": 1
					}
				}
			]
		}`

		runner := func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return []byte(mockJSON), nil
		}

		info, err := InspectAudioTrack(context.Background(), fakeFile, runner)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if info.Title != "Space X" {
			t.Errorf("expected title Space X, got %s", info.Title)
		}
		if info.Artist != "Boris Brejcha" {
			t.Errorf("expected artist Boris Brejcha, got %s", info.Artist)
		}
		if info.Album != "Level One" {
			t.Errorf("expected album Level One, got %s", info.Album)
		}
		if info.Date != "2024-01-01" {
			t.Errorf("expected date 2024-01-01, got %s", info.Date)
		}
		if info.Genre != "High-Tech Minimal" {
			t.Errorf("expected genre High-Tech Minimal, got %s", info.Genre)
		}
		if !info.HasArtwork || !info.IsSquareArtwork {
			t.Errorf("expected square artwork, got has=%v, square=%v", info.HasArtwork, info.IsSquareArtwork)
		}
		if info.ArtworkWidth != 1400 || info.ArtworkHeight != 1400 {
			t.Errorf("expected 1400x1400 artwork dimensions, got %dx%d", info.ArtworkWidth, info.ArtworkHeight)
		}
		if info.Codec != "aac" {
			t.Errorf("expected codec aac, got %s", info.Codec)
		}
		if info.SampleRateHz != 44100 {
			t.Errorf("expected 44100 Hz, got %d", info.SampleRateHz)
		}
	})

	t.Run("invalid json from ffprobe", func(t *testing.T) {
		tmpDir := t.TempDir()
		fakeFile := filepath.Join(tmpDir, "corrupt.m4a")
		_ = os.WriteFile(fakeFile, []byte("audio"), 0644)

		runner := func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return []byte("invalid json output"), nil
		}

		_, err := InspectAudioTrack(context.Background(), fakeFile, runner)
		if err == nil {
			t.Fatal("expected json unmarshal error")
		}
	})
}
