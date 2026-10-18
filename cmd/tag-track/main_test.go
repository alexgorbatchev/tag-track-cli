package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alexgorbatchev/tag-track-cli/internal/cache"
	"github.com/alexgorbatchev/tag-track-cli/internal/deps"
	"github.com/alexgorbatchev/tag-track-cli/internal/metadata"
	"github.com/spf13/cobra"
)

func createTestAudioFile(t *testing.T, dir, filename string) string {
	t.Helper()
	filePath := filepath.Join(dir, filename)
	cmd := exec.Command("ffmpeg", "-v", "quiet", "-hide_banner", "-f", "lavfi", "-i", "anullsrc=r=44100:cl=stereo", "-t", "1", "-y", filePath)
	if err := cmd.Run(); err != nil {
		t.Skipf("ffmpeg not available or failed to create test audio file: %v", err)
	}
	return filePath
}

func captureOutput(fn func()) string {
	r, w, _ := os.Pipe()
	oldStdout := os.Stdout
	os.Stdout = w

	outChan := make(chan string)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		outChan <- buf.String()
	}()

	fn()
	_ = w.Close()
	os.Stdout = oldStdout
	return <-outChan
}

func setupMainTestEnv(t *testing.T) (string, func()) {
	t.Helper()
	tempCacheDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tempCacheDir)
	t.Setenv("AGENT", "0")

	c := cache.NewInDir(filepath.Join(tempCacheDir, "tag-track"), true)
	_ = c.Put("deps", "ffmpeg", "ffmpeg version 8.1.2", time.Hour)
	_ = c.Put("deps", "ffprobe", "ffprobe version 8.1.2", time.Hour)

	cleanupDeps := deps.SetDefaultRunner(func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return []byte(fmt.Sprintf("%s version 8.1.2", name)), nil
	})

	cleanupHTTP := metadata.SetDefaultHTTPClient(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if strings.Contains(req.URL.Host, "itunes.apple.com") {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(strings.NewReader(`{
						"resultCount": 1,
						"results": [
							{
								"trackName": "Space X",
								"artistName": "Boris Brejcha",
								"collectionName": "Level One",
								"primaryGenreName": "Minimal Techno",
								"releaseDate": "2024-05-10T07:00:00Z",
								"artworkUrl100": "https://img.example.com/100x100bb.jpg"
							}
						]
					}`)),
				}, nil
			}
			if strings.Contains(req.URL.Host, "img.example.com") {
				minimalJPEG := []byte{
					0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46, 0x00, 0x01, 0x01, 0x01, 0x00, 0x48, 0x00, 0x48, 0x00, 0x00,
					0xFF, 0xDB, 0x00, 0x43, 0x00, 0x08, 0x06, 0x06, 0x07, 0x06, 0x05, 0x08, 0x07, 0x07, 0x07, 0x09, 0x09, 0x08, 0x0A, 0x0C,
					0x14, 0x0D, 0x0C, 0x0B, 0x0B, 0x0C, 0x19, 0x12, 0x13, 0x0F, 0x14, 0x1D, 0x1A, 0x1F, 0x1E, 0x1D, 0x1A, 0x1C, 0x1C, 0x20,
					0x24, 0x2E, 0x27, 0x20, 0x22, 0x2C, 0x23, 0x1C, 0x1C, 0x28, 0x37, 0x29, 0x2C, 0x30, 0x31, 0x34, 0x34, 0x34, 0x1F, 0x27,
					0x39, 0x3D, 0x38, 0x32, 0x3C, 0x2E, 0x33, 0x34, 0x32, 0xFF, 0xC0, 0x00, 0x0B, 0x08, 0x00, 0x01, 0x00, 0x01, 0x01, 0x01,
					0x11, 0x00, 0xFF, 0xC4, 0x00, 0x1F, 0x00, 0x00, 0x01, 0x05, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x00, 0x00, 0x00, 0x00,
					0x00, 0x00, 0x00, 0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0A, 0x0B, 0xFF, 0xDA, 0x00, 0x08, 0x01,
					0x01, 0x00, 0x00, 0x3F, 0x00, 0x7F, 0x00, 0xFF, 0xD9,
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewReader(minimalJPEG)),
				}, nil
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"results":[]}`)),
			}, nil
		}),
	})

	return tempCacheDir, func() {
		cleanupDeps()
		cleanupHTTP()
	}
}

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func executeCommand(root *cobra.Command, args ...string) (string, error) {
	var err error
	out := captureOutput(func() {
		root.SetArgs(args)
		err = root.Execute()
	})
	return out, err
}

func TestMainFunction(t *testing.T) {
	cmd := newRootCommand()
	if cmd == nil {
		t.Fatal("expected root command to be non-nil")
	}
	if cmd.Use != "tag-track [file...]" {
		t.Fatalf("expected Use 'tag-track [file...]', got %q", cmd.Use)
	}
}

func TestRootCommand_Help(t *testing.T) {
	cmd := newRootCommand()
	var outBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetErr(&outBuf)
	cmd.SetArgs([]string{"--help"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := outBuf.String()
	if !strings.Contains(out, "tag-track") && !strings.Contains(out, "track") {
		t.Fatalf("expected help output, got: %s", out)
	}
}

func TestInspectCommand(t *testing.T) {
	_, cleanupEnv := setupMainTestEnv(t)
	defer cleanupEnv()

	tmpDir := t.TempDir()
	audioPath := createTestAudioFile(t, tmpDir, "InspectTest - Track.m4a")

	t.Run("human mode", func(t *testing.T) {
		t.Setenv("AGENT", "0")
		cmd := newRootCommand()
		out, err := executeCommand(cmd, "track", "inspect", audioPath)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, "File:") {
			t.Fatalf("expected File header, got: %s", out)
		}
	})

	t.Run("agent mode", func(t *testing.T) {
		t.Setenv("AGENT", "1")
		cmd := newRootCommand()
		out, err := executeCommand(cmd, "inspect", audioPath)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, "file:") {
			t.Fatalf("expected file: output in agent mode, got: %s", out)
		}
	})

	t.Run("nonexistent file", func(t *testing.T) {
		t.Setenv("AGENT", "1")
		cmd := newRootCommand()
		out, err := executeCommand(cmd, "track", "inspect", filepath.Join(tmpDir, "nonexistent.m4a"))
		if err != nil {
			t.Fatalf("expected command not to return fatal error for missing file, got: %v", err)
		}
		if !strings.Contains(out, "status: error") {
			t.Fatalf("expected status: error in agent mode, got: %s", out)
		}
	})
}

func TestUpdateCommand(t *testing.T) {
	_, cleanupEnv := setupMainTestEnv(t)
	defer cleanupEnv()

	tmpDir := t.TempDir()
	outDirectory := t.TempDir()
	audioPath := createTestAudioFile(t, tmpDir, "Boris Brejcha - Space X.m4a")

	t.Run("human mode update", func(t *testing.T) {
		t.Setenv("AGENT", "0")
		cmd := newRootCommand()
		out, err := executeCommand(cmd, "track", "update", "-o", outDirectory, audioPath)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, "DONE:") {
			t.Fatalf("expected DONE: output in human mode, got: %s", out)
		}
	})

	t.Run("agent mode update with overrides", func(t *testing.T) {
		t.Setenv("AGENT", "1")
		cmd := newRootCommand()
		out, err := executeCommand(cmd, "update", "-o", outDirectory, "--artist", "Override Artist", "--title", "Override Title", "--album", "Override Album", "--date", "2024", "--genre", "Minimal", audioPath)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, "artist: Override Artist") {
			t.Fatalf("expected artist: Override Artist, got: %s", out)
		}
	})

	t.Run("dry-run mode", func(t *testing.T) {
		t.Setenv("AGENT", "0")
		cmd := newRootCommand()
		out, err := executeCommand(cmd, audioPath, "--dry-run")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, "[DRY RUN - file was not modified]") {
			t.Fatalf("expected dry run notice, got: %s", out)
		}
	})

	t.Run("dry-run mode in agent", func(t *testing.T) {
		t.Setenv("AGENT", "1")
		cmd := newRootCommand()
		out, err := executeCommand(cmd, audioPath, "--dry-run")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, "dry_run: true") {
			t.Fatalf("expected dry_run: true, got: %s", out)
		}
	})

	t.Run("with progress target socket", func(t *testing.T) {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("listening on tcp socket: %v", err)
		}
		defer l.Close()

		go func() {
			for {
				conn, err := l.Accept()
				if err != nil {
					return
				}
				buf := make([]byte, 4096)
				for {
					_, err := conn.Read(buf)
					if err != nil {
						break
					}
				}
				_ = conn.Close()
			}
		}()

		cmd := newRootCommand()
		_, err = executeCommand(cmd, audioPath, "-o", outDirectory, "--progress-target", "tcp://"+l.Addr().String())
		if err != nil {
			t.Fatalf("execution with progress socket failed: %v", err)
		}
	})

	t.Run("file not found in processFiles", func(t *testing.T) {
		t.Setenv("AGENT", "0")
		cmd := newRootCommand()
		out, err := executeCommand(cmd, filepath.Join(tmpDir, "missing.m4a"))
		if err != nil {
			t.Fatalf("unexpected fatal error: %v", err)
		}
		if !strings.Contains(out, "[ERROR] File not found") {
			t.Fatalf("expected error message, got: %s", out)
		}

		t.Setenv("AGENT", "1")
		cmd2 := newRootCommand()
		out2, _ := executeCommand(cmd2, filepath.Join(tmpDir, "missing.m4a"))
		if !strings.Contains(out2, "status: error") {
			t.Fatalf("expected status: error, got: %s", out2)
		}
	})
}

func TestArtworkEmbedCommand(t *testing.T) {
	tmpDir := t.TempDir()
	audioPath := createTestAudioFile(t, tmpDir, "EmbedTest - Song.m4a")

	t.Run("missing artwork flag returns error", func(t *testing.T) {
		cmd := newRootCommand()
		_, err := executeCommand(cmd, "artwork", "embed", audioPath)
		if err == nil {
			t.Fatal("expected error when artwork flag is missing")
		}
	})

	t.Run("artwork embed with local file in agent mode", func(t *testing.T) {
		t.Setenv("AGENT", "1")
		artFile := filepath.Join(tmpDir, "art.jpg")
		_ = exec.Command("ffmpeg", "-v", "quiet", "-hide_banner", "-f", "lavfi", "-i", "color=c=black:s=500x500", "-vframes", "1", "-y", artFile).Run()

		cmd := newRootCommand()
		out, err := executeCommand(cmd, "artwork", "embed", audioPath, "--artwork", artFile, "--in-place")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, "status: ok") {
			t.Fatalf("expected status: ok in agent mode, got: %s", out)
		}
	})

	t.Run("artwork embed with HTTP URL in human mode", func(t *testing.T) {
		t.Setenv("AGENT", "0")
		imgServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cmd := exec.Command("ffmpeg", "-v", "quiet", "-hide_banner", "-f", "lavfi", "-i", "color=c=green:s=400x400", "-vframes", "1", "-f", "image2pipe", "-vcodec", "mjpeg", "-")
			var out bytes.Buffer
			cmd.Stdout = &out
			if err := cmd.Run(); err == nil {
				w.Header().Set("Content-Type", "image/jpeg")
				_, _ = w.Write(out.Bytes())
			}
		}))
		defer imgServer.Close()

		cmd := newRootCommand()
		out, err := executeCommand(cmd, "artwork", "embed", audioPath, "--url", imgServer.URL+"/cover.jpg", "--in-place")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, "[OK] Artwork embedded:") {
			t.Fatalf("expected [OK] Artwork embedded, got: %s", out)
		}
	})
}

func TestDepsCommand(t *testing.T) {
	mockRunner := func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return []byte(name + " version 6.1.1"), nil
	}
	cleanup := deps.SetDefaultRunner(mockRunner)
	defer cleanup()

	t.Run("deps verify agent mode", func(t *testing.T) {
		t.Setenv("AGENT", "1")
		cmd := newRootCommand()
		out, err := executeCommand(cmd, "deps")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, "ffmpeg: ok") {
			t.Fatalf("expected ffmpeg: ok in deps output, got: %s", out)
		}
	})

	t.Run("deps verify human mode", func(t *testing.T) {
		t.Setenv("AGENT", "0")
		cmd := newRootCommand()
		out, err := executeCommand(cmd, "deps")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, "All dependencies met.") {
			t.Fatalf("expected All dependencies met, got: %s", out)
		}
	})

	t.Run("deps install", func(t *testing.T) {
		cmd := newRootCommand()
		_, err := executeCommand(cmd, "deps", "install")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		cmd2 := newRootCommand()
		_, err2 := executeCommand(cmd2, "deps", "install", "ffmpeg")
		if err2 != nil {
			t.Fatalf("unexpected error on specific install: %v", err2)
		}
	})

	t.Run("deps update", func(t *testing.T) {
		cmd := newRootCommand()
		_, err := executeCommand(cmd, "deps", "update")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		cmd2 := newRootCommand()
		_, err2 := executeCommand(cmd2, "deps", "update", "ffmpeg")
		if err2 != nil {
			t.Fatalf("unexpected error on specific update: %v", err2)
		}
	})
}

func TestEnsureDependencies_Branches(t *testing.T) {
	mockRunner := func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return nil, errors.New("missing")
	}
	cleanup := deps.SetDefaultRunner(mockRunner)
	defer cleanup()

	t.Run("auto-install flag enables auto installation", func(t *testing.T) {
		t.Setenv("AGENT", "0")
		autoInstall = true
		defer func() { autoInstall = false }()
		_ = ensureDependencies(context.Background())
	})

	t.Run("interactive prompt answered yes", func(t *testing.T) {
		t.Setenv("AGENT", "0")
		autoInstall = false
		origStdin := stdinReader
		defer func() { stdinReader = origStdin }()
		stdinReader = strings.NewReader("y\n")
		_ = ensureDependencies(context.Background())
	})

	t.Run("interactive prompt answered no", func(t *testing.T) {
		t.Setenv("AGENT", "0")
		autoInstall = false
		origStdin := stdinReader
		defer func() { stdinReader = origStdin }()
		stdinReader = strings.NewReader("n\n")
		err := ensureDependencies(context.Background())
		if err == nil {
			t.Fatal("expected error when prompt is answered no")
		}
	})
}

func TestUpgradeCommand(t *testing.T) {
	t.Run("agent mode upgrade already current", func(t *testing.T) {
		t.Setenv("AGENT", "1")
		cleanup := deps.SetUpgradeSelfFunc(func(ctx context.Context, owner, repo, currentVersion string) (string, error) {
			return currentVersion, errors.New("already at the latest version")
		})
		defer cleanup()

		cmd := newRootCommand()
		out, err := executeCommand(cmd, "upgrade")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, "upgrade: current") {
			t.Fatalf("expected upgrade: current, got: %s", out)
		}
	})

	t.Run("human mode upgrade already current", func(t *testing.T) {
		t.Setenv("AGENT", "0")
		cleanup := deps.SetUpgradeSelfFunc(func(ctx context.Context, owner, repo, currentVersion string) (string, error) {
			return currentVersion, errors.New("already at the latest version")
		})
		defer cleanup()

		cmd := newRootCommand()
		out, err := executeCommand(cmd, "upgrade")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, "already up to date") {
			t.Fatalf("expected already up to date, got: %s", out)
		}
	})

	t.Run("human mode upgrade success", func(t *testing.T) {
		t.Setenv("AGENT", "0")
		cleanup := deps.SetUpgradeSelfFunc(func(ctx context.Context, owner, repo, currentVersion string) (string, error) {
			return "1.2.0", nil
		})
		defer cleanup()

		cmd := newRootCommand()
		out, err := executeCommand(cmd, "upgrade")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, "Successfully upgraded tag-track to version 1.2.0") {
			t.Fatalf("expected upgrade success message, got: %s", out)
		}
	})

	t.Run("agent mode upgrade success", func(t *testing.T) {
		t.Setenv("AGENT", "1")
		cleanup := deps.SetUpgradeSelfFunc(func(ctx context.Context, owner, repo, currentVersion string) (string, error) {
			return "1.2.0", nil
		})
		defer cleanup()

		cmd := newRootCommand()
		out, err := executeCommand(cmd, "upgrade")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, "upgrade: ok") {
			t.Fatalf("expected upgrade: ok, got: %s", out)
		}
	})

	t.Run("upgrade failure", func(t *testing.T) {
		t.Setenv("AGENT", "0")
		cleanup := deps.SetUpgradeSelfFunc(func(ctx context.Context, owner, repo, currentVersion string) (string, error) {
			return "", errors.New("network error")
		})
		defer cleanup()

		cmd := newRootCommand()
		_, err := executeCommand(cmd, "upgrade")
		if err == nil {
			t.Fatal("expected error on upgrade network failure")
		}
	})
}

func TestInspectCommand_TopLevelAlias(t *testing.T) {
	tmpDir := t.TempDir()
	audioPath := createTestAudioFile(t, tmpDir, "TopInspect.m4a")

	t.Setenv("AGENT", "0")
	cmd := newRootCommand()
	out, err := executeCommand(cmd, "inspect", audioPath, "-v")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "File:") {
		t.Fatalf("expected File output: %s", out)
	}
}

func TestEmbedArtworkOnFiles_Errors(t *testing.T) {
	tmpDir := t.TempDir()
	badAudio := filepath.Join(tmpDir, "bad.m4a")
	_ = os.WriteFile(badAudio, []byte("not audio"), 0644)

	t.Setenv("AGENT", "0")
	cmd := newRootCommand()
	out, _ := executeCommand(cmd, "artwork", "embed", badAudio, "--artwork", "http://example.com/art.jpg")
	if !strings.Contains(out, "[ERROR] Embedding artwork") {
		t.Fatalf("expected error message: %s", out)
	}

	t.Setenv("AGENT", "1")
	cmd2 := newRootCommand()
	out2, _ := executeCommand(cmd2, "artwork", "embed", badAudio, "--artwork", "http://example.com/art.jpg")
	if !strings.Contains(out2, "status: error") {
		t.Fatalf("expected status: error in agent mode: %s", out2)
	}
}

func TestInspectCommand_FullTags(t *testing.T) {
	tmpDir := t.TempDir()
	audioPath := createTestAudioFile(t, tmpDir, "FullTag - Track.m4a")

	// Embed tags first so inspect displays all fields
	meta := metadata.TrackMetadataResult{
		Title:       "Track Title",
		Artist:      "Artist Name",
		Album:       "Album Name",
		ReleaseDate: "2024-01-01",
		Genre:       "Techno",
		Source:      "Shazam API",
	}
	_, _ = metadata.ApplyMetadataToLocalTrack(context.Background(), audioPath, meta, tmpDir)

	t.Run("human mode with tags", func(t *testing.T) {
		t.Setenv("AGENT", "0")
		cmd := newRootCommand()
		out, err := executeCommand(cmd, "track", "inspect", filepath.Join(tmpDir, "Artist Name - Track Title.m4a"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, "Title: Track Title") {
			t.Fatalf("expected Title in output: %s", out)
		}
	})

	t.Run("agent mode with tags", func(t *testing.T) {
		t.Setenv("AGENT", "1")
		cmd := newRootCommand()
		out, err := executeCommand(cmd, "track", "inspect", filepath.Join(tmpDir, "Artist Name - Track Title.m4a"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, "title: Track Title") {
			t.Fatalf("expected title in agent output: %s", out)
		}
	})
}

func TestResolveInputFiles(t *testing.T) {
	t.Run("args provided", func(t *testing.T) {
		files, err := resolveInputFiles([]string{"file1.m4a", "file2.mp3"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(files) != 2 || files[0] != "file1.m4a" || files[1] != "file2.mp3" {
			t.Fatalf("unexpected files: %v", files)
		}
	})

	t.Run("stdin piping", func(t *testing.T) {
		origStdin := stdinReader
		defer func() { stdinReader = origStdin }()

		stdinReader = strings.NewReader("DONE: /path/to/one.m4a\noutput: /path/to/two.mp3\n")
		files, err := resolveInputFiles([]string{"-"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(files) != 2 || files[0] != "/path/to/one.m4a" || files[1] != "/path/to/two.mp3" {
			t.Fatalf("unexpected files parsed from stdin: %v", files)
		}
	})
}
