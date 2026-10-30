package metadata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/alexgorbatchev/tag-track-cli/internal/cache"
)

func TestArtworkRefreshAndNoCache(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		requests := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests++; _, _ = w.Write([]byte("fresh artwork")) }))
		c := cache.NewInDir(t.TempDir(), enabled)
		if _, err := c.PutFile("artworks", server.URL, []byte("old artwork")); err != nil {
			t.Fatal(err)
		}
		c.Refresh()
		path, err := DownloadArtwork(context.Background(), server.URL, t.TempDir(), c)
		server.Close()
		if err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(path)
		if err != nil || string(data) != "fresh artwork" || requests != 1 {
			t.Fatalf("data=%q requests=%d err=%v", data, requests, err)
		}
		if _, ok := c.GetFile("artworks", "missing", time.Hour); ok {
			t.Fatal("unexpected cache hit")
		}
	}
}
