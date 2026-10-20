package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/alexgorbatchev/tag-track-cli/internal/deps"
)

func TestJSONPreflightFailures(t *testing.T) {
	_, cleanup := setupMainTestEnv(t)
	defer cleanup()
	old := stdinReader
	t.Cleanup(func() { stdinReader = old })
	commands := [][]string{nil, {"update"}, {"track", "update"}, {"inspect"}, {"track", "inspect"}, {"artwork", "embed"}}
	for _, command := range commands {
		for _, input := range []string{"empty", "broken", "dependencies"} {
			t.Run(strings.Join(command, "_")+"/"+input, func(t *testing.T) {
				stdinReader = strings.NewReader("")
				args := append(append([]string{}, command...), "--json", "-")
				if input == "broken" {
					stdinReader = brokenInput{}
				}
				if input == "dependencies" {
					t.Setenv("XDG_CACHE_HOME", t.TempDir())
					restore := deps.SetDefaultRunner(func(context.Context, string, ...string) ([]byte, error) { return nil, errors.New("missing dependency") })
					defer restore()
					args[len(args)-1] = "input.m4a"
					if len(command) > 0 && command[0] == "artwork" {
						args = append(args, "--artwork=cover.png")
					}
				}
				assertJSONPreflightFailure(t, args...)
			})
		}
	}
	assertJSONPreflightFailure(t, "artwork", "embed", "--json", "input.m4a")
	out, err := executeCommand(newRootCommand(), "update", "--json", "--progress-target=unsupported://fixture", "input.m4a")
	var batch tagBatch
	if jsonErr := json.Unmarshal([]byte(out), &batch); err == nil || jsonErr != nil || len(batch.Results) != 1 || !strings.Contains(batch.Results[0].Error, "initializing progress reporter") {
		t.Fatalf("reporter failure ignored: %s %v", out, err)
	}
}

func assertJSONPreflightFailure(t *testing.T, args ...string) {
	t.Helper()
	out, err := executeCommand(newRootCommand(), args...)
	if err == nil {
		t.Fatalf("%v unexpectedly succeeded: %s", args, out)
	}
	var batch tagBatch
	if decodeErr := json.Unmarshal([]byte(out), &batch); decodeErr != nil {
		t.Fatalf("%v invalid JSON %q: %v", args, out, decodeErr)
	}
	if batch.Status != "error" || len(batch.Results) == 0 || batch.Results[0].Error == "" {
		t.Fatalf("missing failure: %+v", batch)
	}
}
