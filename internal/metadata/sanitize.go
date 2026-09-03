package metadata

import (
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

var (
	dashRe    = regexp.MustCompile(`[–—−‐‑‒―]`)
	illegalRe = regexp.MustCompile("[/\\\\?%*:|\"<>`]")
	hyphenRe  = regexp.MustCompile(`\s+-\s+`)
	spaceRe   = regexp.MustCompile(`\s+`)
	idRe      = regexp.MustCompile(`\s*\[[a-zA-Z0-9_-]{11}\]\s*`)
)

// SanitizeFilename normalizes hyphens, removes bracket video IDs (e.g. [dQw4w9WgXcQ]),
// replaces illegal OS characters with '_', cleans spaces around hyphens, and trims leading/trailing spaces.
func SanitizeFilename(input string) string {
	res := idRe.ReplaceAllString(input, " ")
	res = dashRe.ReplaceAllString(res, "-")
	res = illegalRe.ReplaceAllString(res, "_")
	res = hyphenRe.ReplaceAllString(res, " - ")
	res = spaceRe.ReplaceAllString(res, " ")
	return strings.TrimSpace(res)
}

// NormalizeUnicode performs full Unicode NFD decomposition, strips all non-spacing combining
// diacritics/accents via golang.org/x/text, maps Cyrillic Yo, and returns a clean lowercase string.
func NormalizeUnicode(input string) string {
	s := strings.ToLower(input)
	s = strings.ReplaceAll(s, "ё", "е")

	t := transform.Chain(
		norm.NFD,
		runes.Remove(runes.In(unicode.Mn)),
		norm.NFC,
	)

	res, _, err := transform.String(t, s)
	if err != nil {
		return s
	}
	return res
}
