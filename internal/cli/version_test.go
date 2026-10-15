package cli_test

import (
	"strings"
	"testing"
)

func TestVersionFlagPrintsVersion(t *testing.T) {
	for _, flag := range []string{"--version", "-v"} {
		stdout, stderr, code := execute(t, t.TempDir(), flag)
		if code != 0 {
			t.Fatalf("%s exited %d, stderr=%q", flag, code, stderr)
		}
		if !strings.Contains(stdout, "sd version dev") {
			t.Fatalf("%s stdout=%q, want version line", flag, stdout)
		}
	}
}
