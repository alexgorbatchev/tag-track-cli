package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBatchFailureReturnsErrorAndContinues(t *testing.T) {
	for _, command := range []string{"inspect", "update", "embed"} {
		t.Run(command, func(t *testing.T) {
			_, cleanup := setupMainTestEnv(t)
			defer cleanup()
			t.Setenv("AGENT", "1")
			dir := t.TempDir()
			bad := filepath.Join(dir, "broken.m4a")
			if err := os.WriteFile(bad, []byte("invalid audio"), 0600); err != nil {
				t.Fatal(err)
			}
			good := createTestAudioFile(t, dir, "Boris Brejcha - Space X.m4a")
			args := []string{"track", command, bad, good}
			if command == "embed" {
				args = []string{"artwork", "embed", bad, good, "--artwork", "https://img.example.com/cover.jpg"}
			}
			out, err := executeCommand(newRootCommand(), args...)
			if err == nil {
				t.Fatalf("batch containing corrupt audio succeeded: %s", out)
			}
			if !strings.Contains(out, good) || !strings.Contains(out, "status: error") {
				t.Fatalf("missing per-file results: %s", out)
			}
			if command == "inspect" && !strings.Contains(out, "codec:") {
				t.Fatalf("later file was not inspected: %s", out)
			}
			if command != "inspect" && !strings.Contains(out, "output:") {
				t.Fatalf("later file was not written: %s", out)
			}
		})
	}
}
