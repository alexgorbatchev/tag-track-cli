package metadata

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// Only remove the reserved, generated suffix; retain arbitrary user comment text.
var provenanceSuffix = regexp.MustCompile(`^Source: .* \| Metadata: .* \| Tagged: [0-9]{4}-[0-9]{2}-[0-9]{2}$`)

func resolveUserComment(ctx context.Context, path string, override *string) (string, error) {
	if override != nil {
		return *override, nil
	}
	info, err := InspectAudioTrack(ctx, path, nil)
	if err != nil {
		return "", fmt.Errorf("reading existing comment: %w", err)
	}
	if i := strings.LastIndex(info.Comment, " | Source: "); i >= 0 && provenanceSuffix.MatchString(info.Comment[i+3:]) {
		return info.Comment[:i], nil
	}
	return provenanceSuffix.ReplaceAllString(info.Comment, ""), nil
}
