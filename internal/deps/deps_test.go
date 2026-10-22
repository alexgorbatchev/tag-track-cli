package deps

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/alexgorbatchev/tag-track-cli/internal/cache"
)

func TestParseVersionOutput(t *testing.T) {
	tests := []struct {
		name        string
		output      string
		wantVersion string
	}{
		{
			name:        "ffmpeg standard version line",
			output:      "ffmpeg version 6.1.1 Copyright (c) 2000-2023 the FFmpeg developers",
			wantVersion: "6.1.1",
		},
		{
			name:        "ffmpeg prefixed with n",
			output:      "ffmpeg version n7.0-dev Copyright (c) 2000-2024 the FFmpeg developers",
			wantVersion: "7.0-dev",
		},
		{
			name:        "ffprobe standard version line",
			output:      "ffprobe version 5.1.2-0ubuntu1 Copyright (c) 2007-2022 the FFmpeg developers",
			wantVersion: "5.1.2-0ubuntu1",
		},
		{
			name:        "ffmpeg nightly build",
			output:      "ffmpeg version N-112000-g12345678",
			wantVersion: "112000-g12345678",
		},
		{
			name:        "empty string",
			output:      "",
			wantVersion: "",
		},
		{
			name:        "dotted version in another banner",
			output:      "custom-tool 2.4.1 (build abc)\nsecond line",
			wantVersion: "2.4.1",
		},
		{
			name:        "date version",
			output:      "2024.08.01",
			wantVersion: "2024.08.01",
		},
		{
			name:        "no version",
			output:      "usage: tool [options]",
			wantVersion: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseVersionOutput("tool", tt.output)
			if got != tt.wantVersion {
				t.Errorf("ParseVersionOutput() = %v, want %v", got, tt.wantVersion)
			}
		})
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		name       string
		current    string
		minVersion string
		wantErr    bool
	}{
		{
			name:       "ffmpeg newer version",
			current:    "6.1.1",
			minVersion: "4.4",
			wantErr:    false,
		},
		{
			name:       "ffmpeg exact version",
			current:    "4.4",
			minVersion: "4.4",
			wantErr:    false,
		},
		{
			name:       "ffmpeg older major",
			current:    "3.8.0",
			minVersion: "4.4",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CompareVersions(tt.current, tt.minVersion)
			if (err != nil) != tt.wantErr {
				t.Fatalf("CompareVersions() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestVerifyDependenciesWithRunner(t *testing.T) {
	mockRunner := func(ctx context.Context, name string, args ...string) ([]byte, error) {
		switch name {
		case "ffmpeg":
			return []byte("ffmpeg version 6.1.1 Copyright"), nil
		case "ffprobe":
			return []byte("ffprobe version 6.1.1 Copyright"), nil
		default:
			return nil, errors.New("command not found")
		}
	}

	reports, err := VerifyDependenciesWithRunner(context.Background(), mockRunner, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(reports) != len(RequiredDependencies) {
		t.Fatalf("expected %d reports, got %d", len(RequiredDependencies), len(reports))
	}

	for _, r := range reports {
		if !r.Satisfied {
			t.Errorf("expected dependency %s to be satisfied", r.Name)
		}
	}
}

func TestCheckDependenciesWithRunner(t *testing.T) {
	t.Run("all dependencies met", func(t *testing.T) {
		t.Setenv("AGENT", "0")
		mockRunner := func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return []byte(name + " version 6.1.1"), nil
		}
		err := CheckDependenciesWithRunner(context.Background(), mockRunner, nil)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
	})

	t.Run("missing dependency", func(t *testing.T) {
		t.Setenv("AGENT", "0")
		mockRunner := func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return nil, errors.New("executable not found in $PATH")
		}
		err := CheckDependenciesWithRunner(context.Background(), mockRunner, nil)
		if err == nil {
			t.Fatal("expected error for missing dependencies, got nil")
		}
	})
}

func TestCleanErrorMessage(t *testing.T) {
	msg := CleanErrorMessage("ffmpeg error: file not found\n")
	if !strings.Contains(msg, "file not found") {
		t.Fatalf("expected error message to contain file not found, got: %q", msg)
	}
}

func TestIsAgentMode(t *testing.T) {
	t.Setenv("AGENT", "1")
	if !IsAgentMode() {
		t.Fatal("expected agent mode to be true")
	}
	t.Setenv("AGENT", "0")
	if IsAgentMode() {
		t.Fatal("expected agent mode to be false")
	}
}

func TestSetDefaultRunner(t *testing.T) {
	called := false
	cleanup := SetDefaultRunner(func(ctx context.Context, name string, args ...string) ([]byte, error) {
		called = true
		return []byte(name + " version 6.0"), nil
	})
	defer cleanup()

	r := getRunner()
	_, _ = r(context.Background(), "ffmpeg", "-version")
	if !called {
		t.Fatal("expected custom runner to be called")
	}
}

func TestUpgradeSelfFunctions(t *testing.T) {
	restore := SetUpgradeSelfFunc(func(ctx context.Context, owner, repo, currentVersion string) (string, error) {
		return "2.0.0", nil
	})
	defer restore()

	ver, err := UpgradeSelf(context.Background(), "1.0.0")
	if err != nil || ver != "2.0.0" {
		t.Fatalf("UpgradeSelf failed: ver=%s, err=%v", ver, err)
	}
}

func TestManagerOperations(t *testing.T) {
	t.Setenv("AGENT", "0")
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	mockRunner := func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return []byte(name + " version 6.1.1"), nil
	}
	restore := SetDefaultRunner(mockRunner)
	defer restore()

	_ = InitManagedPath()
	_, _ = GetManagedBinDir()
	_, _ = VerifyDependencies(context.Background())
	_ = CheckDependencies(context.Background())
	_ = InstallDependency(context.Background(), "ffmpeg")
	_ = UpdateDependency(context.Background(), "ffmpeg")
	_, _ = InstallMissingDependencies(context.Background())
	_, _ = UpdateAllDependencies(context.Background())
	_ = CacheAdapterTest(t)
}

func CacheAdapterTest(t *testing.T) error {
	dir := t.TempDir()
	c := cache.NewInDir(dir, true)
	adapter := &cacheAdapter{cache: c}

	if err := adapter.Put("test_key", "val"); err != nil {
		t.Fatalf("put failed: %v", err)
	}
	var val string
	if !adapter.Get("test_key", &val) || val != "val" {
		t.Fatalf("get failed: got %q", val)
	}
	if err := adapter.Delete("test_key"); err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	return nil
}
