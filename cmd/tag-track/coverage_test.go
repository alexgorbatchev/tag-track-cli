package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/alexgorbatchev/tag-track-cli/internal/deps"
)

func TestInputFailuresReachCommands(t *testing.T) {
	old := stdinReader
	t.Cleanup(func() { stdinReader = old })
	for _, prefix := range [][]string{nil, {"track", "update"}, {"track", "inspect"}, {"artwork", "embed"}} {
		for _, input := range []string{"", strings.Repeat("x", 70000)} {
			t.Run(strings.Join(prefix, "_")+map[bool]string{true: "empty", false: "oversized"}[input == ""], func(t *testing.T) {
				stdinReader = strings.NewReader(input)
				cmd := newRootCommand()
				args := append(append([]string{}, prefix...), "-")
				cmd.SetArgs(args)
				var output bytes.Buffer
				cmd.SetOut(&output)
				err := cmd.ExecuteContext(context.Background())
				if input == "" {
					if err != nil || !strings.Contains(output.String(), "tag-track") {
						t.Fatalf("expected help, got %q, %v", output.String(), err)
					}
					return
				}
				if err == nil {
					t.Fatal("expected empty/oversized input error")
				}
				if input != "" && !strings.Contains(err.Error(), "token too long") {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestDependencyFailurePresentation(t *testing.T) {
	for _, mode := range []string{"0", "1"} {
		for _, missing := range []bool{false, true} {
			t.Run(mode+map[bool]string{true: "missing", false: "outdated"}[missing], func(t *testing.T) {
				t.Setenv("AGENT", mode)
				t.Setenv("XDG_CACHE_HOME", t.TempDir())
				t.Setenv("XDG_DATA_HOME", t.TempDir())
				t.Setenv("PATH", os.Getenv("PATH"))
				cleanup := deps.SetDefaultRunner(func(context.Context, string, ...string) ([]byte, error) {
					if missing {
						return nil, errors.New("executable missing")
					}
					return []byte("ffmpeg version 1.0"), nil
				})
				defer cleanup()
				out, err := executeCommand(newRootCommand(), "deps")
				if err == nil {
					t.Fatal("expected dependency error")
				}
				want := "1.0"
				if missing {
					want = "missing"
				}
				if !strings.Contains(out, want) || !strings.Contains(out, "ffmpeg") {
					t.Fatalf("missing diagnosis: %s", out)
				}
			})
		}
	}
}

func TestDependencyInstallFailures(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("PATH", os.Getenv("PATH"))
	cleanup := deps.SetDefaultRunner(func(context.Context, string, ...string) ([]byte, error) {
		return nil, errors.New("fixture installer failure")
	})
	defer cleanup()
	for _, args := range [][]string{{"deps", "install"}, {"deps", "install", "ffmpeg"}, {"deps", "update"}, {"deps", "update", "ffmpeg"}} {
		_, err := executeCommand(newRootCommand(), args...)
		if err == nil || !strings.Contains(err.Error(), "fixture installer failure") {
			t.Fatalf("%v: %v", args, err)
		}
	}
}

func TestDependencyInstallationRecovers(t *testing.T) {
	for _, auto := range []bool{false, true} {
		t.Run(map[bool]string{true: "automatic", false: "prompt"}[auto], func(t *testing.T) {
			t.Setenv("AGENT", "0")
			t.Setenv("XDG_CACHE_HOME", t.TempDir())
			t.Setenv("XDG_DATA_HOME", t.TempDir())
			t.Setenv("PATH", os.Getenv("PATH"))
			newRootCommand()
			autoInstall = auto
			noCache = true
			old := stdinReader
			stdinReader = strings.NewReader("yes\n")
			t.Cleanup(func() { stdinReader = old; autoInstall = false; noCache = false })
			installs := 0
			cleanup := deps.SetDefaultRunner(func(ctx context.Context, name string, args ...string) ([]byte, error) {
				if name == "ffmpeg" || name == "ffprobe" {
					return []byte(name + " version 1.0"), nil
				}
				installs++
				return nil, nil
			})
			defer cleanup()
			out := captureOutput(func() {
				if err := ensureDependencies(context.Background()); err != nil {
					t.Fatal(err)
				}
			})
			if installs == 0 || !strings.Contains(out, "Successfully installed:") {
				t.Fatalf("installs=%d output=%s", installs, out)
			}
		})
	}
}
