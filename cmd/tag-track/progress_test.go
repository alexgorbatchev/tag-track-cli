package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestProgressJobEnvelope(t *testing.T) {
	for _, command := range [][]string{nil, {"track", "update"}} {
		t.Run(strings.Join(command, " "), func(t *testing.T) {
			_, cleanup := setupMainTestEnv(t)
			defer cleanup()
			t.Setenv("AGENT", "1")
			file := createTestAudioFile(t, t.TempDir(), "Artist - Track.m4a")
			args := append(command, file, file, "--dry-run", "--progress-target", "stdout", "--job-id", "abc123")
			out, err := executeCommand(newRootCommand(), args...)
			if err != nil {
				t.Fatal(err)
			}
			sequence, complete := 0, 0
			for _, line := range strings.Split(out, "\n") {
				var ev struct {
					JobID    string   `json:"job_id"`
					Sequence int      `json:"sequence"`
					Type     string   `json:"type"`
					Percent  *float64 `json:"percent"`
				}
				if json.Unmarshal([]byte(line), &ev) != nil || ev.Type == "" {
					continue
				}
				sequence++
				if ev.JobID != "abc123" || ev.Sequence != sequence || ev.Percent == nil {
					t.Fatalf("bad envelope: %s", line)
				}
				if ev.Type == "complete" {
					complete++
					if *ev.Percent != float64(complete*50) {
						t.Fatalf("bad batch progress: %s", line)
					}
				}
			}
			if complete != 2 {
				t.Fatalf("got %d completed files; output: %s", complete, out)
			}
		})
	}
}
