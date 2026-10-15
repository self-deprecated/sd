package runner

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestDoubleStarSlashMatchesZeroDirectories(t *testing.T) {
	cases := []struct {
		pattern, file string
		want          bool
	}{
		{"**/*.go", "main.go", true},
		{"**/*.go", "cmd/sd/main.go", true},
		{"src/**/x.ts", "src/x.ts", true},
		{"src/**/x.ts", "src/a/b/x.ts", true},
		{"src/**/x.ts", "srcx.ts", false},
	}
	for _, tc := range cases {
		if got := matchChangedPattern(tc.pattern, tc.file); got != tc.want {
			t.Errorf("matchChangedPattern(%q, %q) = %v, want %v", tc.pattern, tc.file, got, tc.want)
		}
	}
}

func TestUnresolvableChangeBaseIsAnError(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "base"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	t.Setenv("SD_CHANGED_FILES", "")
	os.Unsetenv("SD_CHANGED_FILES")
	t.Setenv("SD_CHANGE_BASE", "no-such-revision")
	_, err := changedFiles(root)
	if err == nil || !strings.Contains(err.Error(), "no-such-revision") {
		t.Fatalf("err = %v, want an error naming the base", err)
	}
}
