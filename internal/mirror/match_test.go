package mirror

import "testing"

func TestMatchesAnyKeepsSingleStarWithinOnePathSegment(t *testing.T) {
	cases := []struct {
		pattern string
		rel     string
		want    bool
	}{
		{"docs/*.md", "docs/guide.md", true},
		{"docs/*.md", "docs/internal/guide.md", false},
		{"src/*", "src/nested/file.go", false},
	}
	for _, tc := range cases {
		if got := matchesAny(tc.rel, []string{tc.pattern}); got != tc.want {
			t.Errorf("matchesAny(%q, %q) = %v, want %v", tc.rel, tc.pattern, got, tc.want)
		}
	}
}
