package metadata

import (
	"context"
	"strings"
	"testing"
)

func TestRetagPreservesOrReplacesUserComment(t *testing.T) {
	for _, tc := range []struct {
		name     string
		override *string
		want     string
	}{
		{"preserve", nil, "Warm-up track"},
		{"replace", new("Peak time"), "Peak time"},
		{"source text", new("Source: my playlist"), "Source: my playlist"},
		{"clear", new(""), ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			path := createDummyAudioFile(t, t.TempDir(), "song.m4a")
			meta := TrackMetadataResult{Artist: "A", Title: "T", Comment: new("Warm-up track")}
			path, err := ApplyMetadataToLocalTrack(context.Background(), path, meta, "")
			if err != nil {
				t.Fatal(err)
			}
			meta.Comment = tc.override
			for range 2 {
				path, err = ApplyMetadataToLocalTrack(context.Background(), path, meta, "")
				if err != nil {
					t.Fatal(err)
				}
				meta.Comment = nil // A subsequent ordinary retag must retain the requested text.
			}
			info, err := InspectAudioTrack(context.Background(), path, nil)
			if err != nil {
				t.Fatal(err)
			}
			prefix := tc.want
			if prefix != "" {
				prefix += " | "
			}
			if !strings.HasPrefix(info.Comment, prefix+"Source: local file | Metadata: tag-track | Tagged: ") || strings.Count(info.Comment, "Tagged:") != 1 {
				t.Fatalf("comment=%q", info.Comment)
			}
		})
	}
}
