package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alexgorbatchev/tag-track-cli/internal/cache"
	"github.com/alexgorbatchev/tag-track-cli/internal/metadata"
)

func TestRefreshCommandCachePolicy(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		flags                []string
		wantFresh, wantWrite bool
	}{
		{"cached", nil, false, false},
		{"refresh", []string{"--refresh"}, true, true},
		{"no cache overrides refresh", []string{"--refresh", "--no-cache"}, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, cleanup := setupMainTestEnv(t)
			defer cleanup()
			t.Setenv("AGENT", "1")
			file := createTestAudioFile(t, t.TempDir(), "Boris Brejcha - Space X.m4a")
			c, err := cache.New(true)
			if err != nil {
				t.Fatal(err)
			}
			key := fmt.Sprintf("%s:Boris Brejcha - Space X:Boris Brejcha:Space X", file)
			if err := c.Put("metadata", key, metadata.TrackMetadataResult{Artist: "Stale", Title: "Stale"}, time.Hour); err != nil {
				t.Fatal(err)
			}
			args := append([]string{file, "--dry-run"}, tc.flags...)
			out, err := executeCommand(newRootCommand(), args...)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(out, "Stale") == tc.wantFresh {
				t.Fatalf("wrong cache read policy: %s", out)
			}
			var stored metadata.TrackMetadataResult
			if !c.Get("metadata", key, &stored) {
				t.Fatal("cache entry missing")
			}
			if (stored.Title != "Stale") != tc.wantWrite {
				t.Fatalf("wrong cache write policy: %+v", stored)
			}
		})
	}
}

func TestCommentCommandExplicitEmptyAndOmission(t *testing.T) {
	_, cleanup := setupMainTestEnv(t)
	defer cleanup()
	t.Setenv("AGENT", "1")
	dir := t.TempDir()
	file := createTestAudioFile(t, dir, "input.m4a")
	for _, tc := range []struct {
		flags []string
		want  string
	}{
		{[]string{"--comment", "Warm-up track"}, "Warm-up track"},
		{nil, "Warm-up track | Source: local file | Metadata: Filename Fallback | Tagged: " + time.Now().Format("2006-01-02")},
		{[]string{"--comment="}, ""},
	} {
		args := append([]string{"track", "update", file, "--artist", "Test", "--title", "Track"}, tc.flags...)
		out, err := executeCommand(newRootCommand(), args...)
		if err != nil {
			t.Fatal(err)
		}
		file = filepath.Join(dir, "Test - Track.m4a")
		info, err := metadata.InspectAudioTrack(context.Background(), file, nil)
		if err != nil {
			t.Fatalf("%v; output=%s", err, out)
		}
		if info.Comment != tc.want {
			t.Fatalf("comment=%q", info.Comment)
		}
	}
	for _, args := range [][]string{{"--force"}, {"track", "update", "--force"}} {
		if _, err := executeCommand(newRootCommand(), args...); err == nil || !strings.Contains(err.Error(), "unknown flag") {
			t.Fatalf("--force accepted: %v", err)
		}
	}
}
