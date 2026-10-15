package glob_test

import (
	"testing"

	"github.com/self-deprecated/sd/internal/glob"
)

func TestMatch(t *testing.T) {
	cases := []struct {
		pattern string
		name    string
		want    bool
	}{
		{"**", "a/b/c", true},
		{"*.env", ".env", true},
		{"*.env", "local.env", true},
		{"*.env", "a/b/local.env", true},
		{".env.*", "apps/web/.env.local", true},
		{"docs/*.md", "docs/guide.md", true},
		{"docs/*.md", "docs/internal/guide.md", false},
		{"docs/*.md", "site/docs/guide.md", true},
		{"src/*", "src/nested/file.go", false},
		{"**/private/*", "a/private/secret.txt", true},
		{"**/private/*", "private/top.txt", true},
		{"**/private/*", "a/private/deeper/keep.txt", false},
		{"node_modules/**", "node_modules", true},
		{"node_modules/**", "node_modules/pkg/index.js", true},
		{"node_modules/**", "apps/mobile/node_modules/expo/index.js", true},
		{"node_modules/**", "apps/node_modules_old/index.js", false},
		{"docs/app-store/**", "docs/app-store/en/listing.md", true},
		{"a/**/b", "a/b", true},
		{"a/**/b", "a/x/y/b", true},
		{"a/**/b", "a/x/y/c", false},
		{"apps/mobile/build-*.apk", "apps/mobile/build-1.apk", true},
		{"apps/mobile/build-*.apk", "apps/mobile/sub/build-1.apk", false},
		{"/build/**", "build/out.js", true},
		{"/build/**", "apps/build/out.js", false},
		{"/README.md", "README.md", true},
		{"/README.md", "docs/README.md", false},
		{"file[0-9].txt", "file7.txt", true},
		{"fil?.txt", "file.txt", true},
		{"fil?.txt", "fil/.txt", false},
		{"build/", "build", true},
	}
	for _, tc := range cases {
		got, err := glob.Match(tc.pattern, tc.name)
		if err != nil {
			t.Errorf("Match(%q, %q) error: %v", tc.pattern, tc.name, err)
			continue
		}
		if got != tc.want {
			t.Errorf("Match(%q, %q) = %v, want %v", tc.pattern, tc.name, got, tc.want)
		}
	}
}

func TestMatchRejectsMalformedPatterns(t *testing.T) {
	for _, pattern := range []string{"", "/", ".", "..", "../x", "[abc", "docs/[z-a", "build**", "a/**b/c", `trailing\`} {
		if _, err := glob.Match(pattern, "a/b"); err == nil {
			t.Errorf("Match(%q) succeeded, want error", pattern)
		}
		if err := glob.Validate(pattern); err == nil {
			t.Errorf("Validate(%q) succeeded, want error", pattern)
		}
	}
}
