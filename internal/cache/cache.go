package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Cache manages local XDG caching for search queries, metadata, cover art, and dependency probes.
type Cache struct {
	baseDir     string
	enabled     bool
	bypassReads bool
}

// Refresh bypasses reads while allowing successful lookups to replace cached entries.
// A disabled cache remains disabled for both reads and writes.
func (c *Cache) Refresh() {
	if c != nil {
		c.bypassReads = true
	}
}

// Entry wraps cached items with timestamp and TTL metadata.
type Entry struct {
	CreatedAt time.Time       `json:"created_at"`
	TTL       time.Duration   `json:"ttl"`
	Data      json.RawMessage `json:"data"`
}

// New creates a new Cache instance. If enabled is false, all read/write operations bypass local storage.
func New(enabled bool) (*Cache, error) {
	if !enabled {
		return &Cache{enabled: false}, nil
	}

	var baseDir string
	if xdg := strings.TrimSpace(os.Getenv("XDG_CACHE_HOME")); xdg != "" {
		baseDir = filepath.Join(xdg, "tag-track")
	} else {
		userCache, err := os.UserCacheDir()
		if err != nil {
			userCache = os.TempDir()
		}
		baseDir = filepath.Join(userCache, "tag-track")
	}

	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return &Cache{enabled: false}, nil
	}

	return &Cache{
		baseDir: baseDir,
		enabled: true,
	}, nil
}

// NewInDir creates a Cache instance rooted in a specific custom directory (useful for unit testing).
func NewInDir(dir string, enabled bool) *Cache {
	if !enabled {
		return &Cache{enabled: false}
	}
	_ = os.MkdirAll(dir, 0755)
	return &Cache{
		baseDir: dir,
		enabled: true,
	}
}

// KeyHash produces a deterministic hex filename key for a given namespace and query/URL string.
func KeyHash(namespace, key string) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s:%s", namespace, key)))
	return hex.EncodeToString(h[:16])
}

// Enabled returns whether caching is active.
func (c *Cache) Enabled() bool {
	return c != nil && c.enabled
}

// Get retrieves a cached item if present and within its TTL.
func (c *Cache) Get(namespace, key string, target interface{}) bool {
	if !c.Enabled() || c.bypassReads {
		return false
	}

	filePath := filepath.Join(c.baseDir, namespace, KeyHash(namespace, key)+".json")
	data, err := os.ReadFile(filePath)
	if err != nil {
		return false
	}

	var entry Entry
	if err := json.Unmarshal(data, &entry); err != nil {
		return false
	}

	if entry.TTL > 0 && time.Since(entry.CreatedAt) > entry.TTL {
		_ = os.Remove(filePath)
		return false
	}

	if err := json.Unmarshal(entry.Data, target); err != nil {
		return false
	}

	return true
}

// Put serializes and stores an item with a specified TTL.
func (c *Cache) Put(namespace, key string, item interface{}, ttl time.Duration) error {
	if !c.Enabled() {
		return nil
	}

	dir := filepath.Join(c.baseDir, namespace)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	rawItem, err := json.Marshal(item)
	if err != nil {
		return err
	}

	entry := Entry{
		CreatedAt: time.Now(),
		TTL:       ttl,
		Data:      rawItem,
	}

	entryData, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	filePath := filepath.Join(dir, KeyHash(namespace, key)+".json")
	return os.WriteFile(filePath, entryData, 0644)
}

// GetFile retrieves a cached file path (e.g. cover art) if present and within TTL.
func (c *Cache) GetFile(namespace, key string, maxAge time.Duration) (string, bool) {
	if !c.Enabled() || c.bypassReads {
		return "", false
	}

	filePath := filepath.Join(c.baseDir, namespace, KeyHash(namespace, key)+".bin")
	stat, err := os.Stat(filePath)
	if err != nil || stat.Size() == 0 {
		return "", false
	}

	if maxAge > 0 && time.Since(stat.ModTime()) > maxAge {
		_ = os.Remove(filePath)
		return "", false
	}

	return filePath, true
}

// PutFile writes raw bytes to a cached file.
func (c *Cache) PutFile(namespace, key string, data []byte) (string, error) {
	if !c.Enabled() {
		return "", nil
	}

	dir := filepath.Join(c.baseDir, namespace)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}

	filePath := filepath.Join(dir, KeyHash(namespace, key)+".bin")
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return "", err
	}
	return filePath, nil
}

// Delete removes an item from cache.
func (c *Cache) Delete(namespace, key string) {
	if !c.Enabled() {
		return
	}
	_ = os.Remove(filepath.Join(c.baseDir, namespace, KeyHash(namespace, key)+".json"))
	_ = os.Remove(filepath.Join(c.baseDir, namespace, KeyHash(namespace, key)+".bin"))
}
