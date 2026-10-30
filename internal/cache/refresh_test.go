package cache

import (
	"os"
	"testing"
	"time"
)

func TestRefreshBypassesReadsAndStoresReplacements(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		c := NewInDir(t.TempDir(), enabled)
		if err := c.Put("metadata", "track", "old", time.Hour); err != nil {
			t.Fatal(err)
		}
		if _, err := c.PutFile("artworks", "url", []byte("old")); err != nil {
			t.Fatal(err)
		}
		c.Refresh()
		var value string
		if c.Get("metadata", "track", &value) {
			t.Fatal("refresh read stale metadata")
		}
		if _, ok := c.GetFile("artworks", "url", time.Hour); ok {
			t.Fatal("refresh read stale artwork")
		}
		if err := c.Put("metadata", "track", "fresh", time.Hour); err != nil {
			t.Fatal(err)
		}
		if _, err := c.PutFile("artworks", "url", []byte("fresh")); err != nil {
			t.Fatal(err)
		}
		reader := NewInDir(c.baseDir, enabled)
		if reader.Get("metadata", "track", &value) != enabled {
			t.Fatal("wrong cache write policy")
		}
		if enabled {
			if value != "fresh" {
				t.Fatal(value)
			}
			path, ok := reader.GetFile("artworks", "url", time.Hour)
			if !ok {
				t.Fatal("no fresh artwork")
			}
			data, err := os.ReadFile(path)
			if err != nil || string(data) != "fresh" {
				t.Fatalf("artwork=%q err=%v", data, err)
			}
		}
	}
}
