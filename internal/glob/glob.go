// Package glob matches slash-separated relative paths against patterns.
//
// Within one path segment, `*` matches any run of characters, `?` matches one
// character, `[...]` matches a character class and `\` escapes the next
// character, as in path.Match. A `**` segment matches zero or more whole
// segments.
//
// Patterns are unanchored: a pattern matches a path when it matches the whole
// path or any trailing run of its segments, so `*.env` matches `a/b.env` and
// `node_modules/**` matches `apps/web/node_modules/pkg/index.js`. A leading `/`
// anchors the pattern to the root, so `/build/**` matches only the top-level
// build directory.
package glob

import (
	"fmt"
	"path"
	"strings"
)

// Match reports whether the slash-separated relative path name matches pattern.
// It returns an error for malformed patterns.
func Match(pattern, name string) (bool, error) {
	segments, err := compile(pattern)
	if err != nil {
		return false, err
	}
	name = strings.Trim(name, "/")
	if name == "" {
		return false, nil
	}
	return matchSegments(segments, strings.Split(name, "/")), nil
}

// Validate returns an error when pattern is malformed.
func Validate(pattern string) error {
	_, err := compile(pattern)
	return err
}

func compile(pattern string) ([]string, error) {
	anchored := strings.HasPrefix(pattern, "/")
	cleaned := path.Clean(strings.Trim(pattern, "/"))
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return nil, fmt.Errorf("invalid pattern %q: must name a path inside the tree", pattern)
	}
	var segments []string
	if !anchored {
		segments = append(segments, "**")
	}
	for _, segment := range strings.Split(cleaned, "/") {
		if segment == "**" {
			if len(segments) == 0 || segments[len(segments)-1] != "**" {
				segments = append(segments, segment)
			}
			continue
		}
		if strings.Contains(segment, "**") {
			return nil, fmt.Errorf("invalid pattern %q: ** must be a whole path segment", pattern)
		}
		if _, err := path.Match(segment, ""); err != nil {
			return nil, fmt.Errorf("invalid pattern %q: %w", pattern, err)
		}
		segments = append(segments, segment)
	}
	return segments, nil
}

func matchSegments(pattern, name []string) bool {
	for len(pattern) > 0 {
		if pattern[0] == "**" {
			if len(pattern) == 1 {
				return true
			}
			for i := 0; i <= len(name); i++ {
				if matchSegments(pattern[1:], name[i:]) {
					return true
				}
			}
			return false
		}
		if len(name) == 0 {
			return false
		}
		if ok, _ := path.Match(pattern[0], name[0]); !ok {
			return false
		}
		pattern, name = pattern[1:], name[1:]
	}
	return len(name) == 0
}
