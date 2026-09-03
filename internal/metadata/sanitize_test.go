package metadata

import (
	"testing"
)

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Clean simple name",
			input:    "Artist - Title",
			expected: "Artist - Title",
		},
		{
			name:     "Illegal characters in filename",
			input:    "AC/DC - Highway To Hell? [Extended]: 100%",
			expected: "AC_DC - Highway To Hell_ [Extended]_ 100_",
		},
		{
			name:     "Em dash and en dash normalization",
			input:    "Artist — Title – Remix",
			expected: "Artist - Title - Remix",
		},
		{
			name:     "Extra whitespace around hyphens",
			input:    "  Artist    -   Track Title   ",
			expected: "Artist - Track Title",
		},
		{
			name:     "YouTube video bracket ID removal",
			input:    "Boris Brejcha - Space X [dQw4w9WgXcQ]",
			expected: "Boris Brejcha - Space X",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := SanitizeFilename(tc.input)
			if got != tc.expected {
				t.Errorf("SanitizeFilename(%q) = %q; want %q", tc.input, got, tc.expected)
			}
		})
	}
}
