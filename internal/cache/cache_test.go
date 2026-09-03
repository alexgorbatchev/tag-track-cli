package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

type TestItem struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
}

func TestCache_GetPut(t *testing.T) {
	dir := t.TempDir()
	c := NewInDir(dir, true)

	if !c.Enabled() {
		t.Fatal("expected cache to be enabled")
	}

	item := TestItem{Name: "track", Value: 42}
	if err := c.Put("tracks", "key1", item, 1*time.Hour); err != nil {
		t.Fatalf("unexpected put error: %v", err)
	}

	var retrieved TestItem
	if !c.Get("tracks", "key1", &retrieved) {
		t.Fatal("expected cache hit")
	}
	if retrieved.Name != "track" || retrieved.Value != 42 {
		t.Fatalf("unexpected value: %+v", retrieved)
	}

	var missing TestItem
	if c.Get("tracks", "nonexistent", &missing) {
		t.Fatal("expected cache miss")
	}
}

func TestCache_ExpiredTTL(t *testing.T) {
	dir := t.TempDir()
	c := NewInDir(dir, true)

	item := TestItem{Name: "expired", Value: 1}
	if err := c.Put("tracks", "expKey", item, 10*time.Millisecond); err != nil {
		t.Fatalf("unexpected put error: %v", err)
	}

	time.Sleep(20 * time.Millisecond)

	var retrieved TestItem
	if c.Get("tracks", "expKey", &retrieved) {
		t.Fatal("expected cache miss due to expiration")
	}
}

func TestCache_GetFile_Expired(t *testing.T) {
	dir := t.TempDir()
	c := NewInDir(dir, true)

	data := []byte("image data")
	filePath, err := c.PutFile("artworks", "art1", data)
	if err != nil || filePath == "" {
		t.Fatalf("unexpected put file error: %v", err)
	}

	// Change file modification time to 2 hours ago
	past := time.Now().Add(-2 * time.Hour)
	_ = os.Chtimes(filePath, past, past)

	if _, found := c.GetFile("artworks", "art1", 1*time.Hour); found {
		t.Fatal("expected expired artwork file to be missed")
	}
}

func TestCache_InvalidOuterJSON(t *testing.T) {
	dir := t.TempDir()
	c := NewInDir(dir, true)

	namespace := "tracks"
	key := "corruptKey"
	subDir := filepath.Join(dir, namespace)
	_ = os.MkdirAll(subDir, 0755)
	corruptFile := filepath.Join(subDir, KeyHash(namespace, key)+".json")
	_ = os.WriteFile(corruptFile, []byte("{invalid json"), 0644)

	var item TestItem
	if c.Get(namespace, key, &item) {
		t.Fatal("expected cache miss on corrupt outer JSON")
	}
}

func TestCache_InnerTargetUnmarshalError(t *testing.T) {
	dir := t.TempDir()
	c := NewInDir(dir, true)

	// Put an item that is a map
	if err := c.Put("tracks", "mapKey", map[string]string{"foo": "bar"}, 1*time.Hour); err != nil {
		t.Fatalf("put failed: %v", err)
	}

	// Try to unmarshal into a type that will fail, like an int
	var intTarget int
	if c.Get("tracks", "mapKey", &intTarget) {
		t.Fatal("expected failure unmarshaling map into int")
	}
}

func TestCache_GetPutFile(t *testing.T) {
	dir := t.TempDir()
	c := NewInDir(dir, true)

	data := []byte("raw image data")
	path, err := c.PutFile("artworks", "url1", data)
	if err != nil {
		t.Fatalf("put file failed: %v", err)
	}
	if path == "" {
		t.Fatal("expected non-empty path")
	}

	retrievedPath, found := c.GetFile("artworks", "url1", 1*time.Hour)
	if !found {
		t.Fatal("expected file to be found")
	}
	if retrievedPath != path {
		t.Fatalf("path mismatch: got %s, want %s", retrievedPath, path)
	}
}

func TestCache_PutFileDirError(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "blocked")
	_ = os.WriteFile(filePath, []byte("file"), 0644)

	// Root cache inside a file path so MkdirAll fails
	c := NewInDir(filepath.Join(filePath, "sub"), true)
	if _, err := c.PutFile("ns", "k", []byte("d")); err == nil {
		t.Fatal("expected error creating directory when path is blocked by file")
	}
}

func TestCache_PutUnmarshalableType(t *testing.T) {
	dir := t.TempDir()
	c := NewInDir(dir, true)

	// Functions cannot be JSON marshaled
	if err := c.Put("ns", "k", func() {}, 1*time.Hour); err == nil {
		t.Fatal("expected json marshal error")
	}
}

func TestCache_Disabled(t *testing.T) {
	c, _ := New(false)
	if c.Enabled() {
		t.Fatal("expected disabled cache")
	}

	if err := c.Put("test", "key", "val", 1*time.Hour); err != nil {
		t.Fatalf("put should no-op without error: %v", err)
	}

	var val string
	if c.Get("test", "key", &val) {
		t.Fatal("get should return false when disabled")
	}

	p, err := c.PutFile("test", "key", []byte("data"))
	if err != nil || p != "" {
		t.Fatalf("putfile should return empty when disabled: %v, %s", err, p)
	}

	if _, found := c.GetFile("test", "key", 1*time.Hour); found {
		t.Fatal("getfile should return false when disabled")
	}
}

func TestCache_Delete(t *testing.T) {
	dir := t.TempDir()
	c := NewInDir(dir, true)

	_ = c.Put("ns", "k", "v", 1*time.Hour)
	_, _ = c.PutFile("ns", "k", []byte("d"))

	c.Delete("ns", "k")

	var v string
	if c.Get("ns", "k", &v) {
		t.Fatal("expected item to be deleted")
	}
	if _, found := c.GetFile("ns", "k", 1*time.Hour); found {
		t.Fatal("expected file to be deleted")
	}

	// Calling Delete on disabled cache should not panic
	cDisabled := &Cache{enabled: false}
	cDisabled.Delete("ns", "k")
}

func TestNew(t *testing.T) {
	t.Run("enabled with xdg", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("XDG_CACHE_HOME", dir)
		c, err := New(true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !c.Enabled() {
			t.Fatal("expected cache to be enabled")
		}
	})

	t.Run("enabled without xdg", func(t *testing.T) {
		t.Setenv("XDG_CACHE_HOME", "")
		c, err := New(true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !c.Enabled() {
			t.Fatal("expected cache to be enabled")
		}
	})

	t.Run("disabled", func(t *testing.T) {
		c, err := New(false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if c.Enabled() {
			t.Fatal("expected cache to be disabled")
		}
	})
}
