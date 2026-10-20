package main

import (
	"context"
	"encoding/json"
	"github.com/dj/tag-track-cli/internal/metadata"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestJSONInspectReportsFilesAndFailures(t *testing.T) {
	dir, cleanup := setupMainTestEnv(t)
	defer cleanup()
	file := createTestAudioFile(t, dir, "sample.wav")
	cmd := newRootCommand()
	cmd.SetArgs([]string{"track", "inspect", "--json", file, filepath.Join(dir, "missing.wav")})
	var runErr error
	output := captureOutput(func() { runErr = cmd.Execute() })
	if runErr == nil {
		t.Fatal("missing file must fail the batch")
	}
	var result struct {
		Status  string `json:"status"`
		Results []struct {
			Input      string          `json:"input_file"`
			Status     string          `json:"status"`
			Inspection json.RawMessage `json:"inspection"`
			Error      string          `json:"error"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("JSON: %v; output %s", err, output)
	}
	if len(result.Results) != 2 || result.Results[0].Input != file || result.Results[0].Status != "success" || len(result.Results[0].Inspection) == 0 || result.Results[1].Error == "" {
		t.Fatalf("result: %+v", result)
	}
}

func TestJSONArtworkLocalFile(t *testing.T) {
	dir, cleanup := setupMainTestEnv(t)
	defer cleanup()
	file := createTestAudioFile(t, dir, "sample.m4a")
	before, err := metadata.InspectAudioTrack(context.Background(), file, nil)
	if err != nil {
		t.Fatal(err)
	}
	art := filepath.Join(dir, "cover.png")
	f, err := os.Create(art)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, image.NewRGBA(image.Rect(0, 0, 80, 40))); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, path string
		fails      bool
	}{
		{"local", art, false},
		{"missing", filepath.Join(dir, "missing.png"), true},
		{"invalid", file, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := newRootCommand()
			cmd.SetArgs([]string{"artwork", "embed", "--json", "--artwork", tc.path, "--out-dir", t.TempDir(), file})
			var runErr error
			output := captureOutput(func() { runErr = cmd.Execute() })
			if (runErr != nil) != tc.fails {
				t.Fatalf("error = %v; JSON %s", runErr, output)
			}
			var result tagBatch
			if err := json.Unmarshal([]byte(output), &result); err != nil {
				t.Fatal(err)
			}
			if tc.fails {
				return
			}
			after, err := metadata.InspectAudioTrack(context.Background(), result.Results[0].OutputFile, nil)
			if err != nil {
				t.Fatal(err)
			}
			if !after.HasArtwork || after.ArtworkWidth != 1400 || after.ArtworkHeight != 1400 || after.Comment != before.Comment || after.Title != before.Title {
				t.Fatalf("artwork/tags: %+v; before %+v", after, before)
			}
			if _, err := os.Stat(art); err != nil {
				t.Fatalf("source artwork lost: %v", err)
			}
			for _, agent := range []string{"0", "1"} {
				t.Setenv("AGENT", agent)
				output, err := executeCommand(newRootCommand(), "inspect", result.Results[0].OutputFile)
				if err != nil || !strings.Contains(output, "1400x1400") {
					t.Fatalf("normalized artwork missing in inspection: %s %v", output, err)
				}
			}
		})
	}
}

func TestJSONUpdateExplicitEmptyTags(t *testing.T) {
	dir, cleanup := setupMainTestEnv(t)
	defer cleanup()
	file := createTestAudioFile(t, dir, "sample.m4a")
	cmd := newRootCommand()
	cmd.SetArgs([]string{"track", "update", "--json", "--album=", "--title=Chosen", "--comment=Personal note", "--out-dir", t.TempDir(), file})
	var runErr error
	output := captureOutput(func() { runErr = cmd.Execute() })
	if runErr != nil {
		t.Fatal(runErr)
	}
	var result tagBatch
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Results) != 1 {
		t.Fatalf("result: %+v", result)
	}
	inspection, err := metadata.InspectAudioTrack(context.Background(), result.Results[0].OutputFile, nil)
	if err != nil {
		t.Fatal(err)
	}
	if inspection.Title != "Chosen" || inspection.Album != "" || inspection.Comment != "Personal note" {
		t.Fatalf("tags: %+v", inspection)
	}
}
