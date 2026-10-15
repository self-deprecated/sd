//go:build unix

package shell

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
)

// A command that ignores SIGINT must still be gone when Run returns, so an
// interrupted task cannot leave builds or servers running.
func TestInterruptKillsCommandsThatIgnoreIt(t *testing.T) {
	for _, form := range []string{"%s", "%s & while true; do :; done"} {
		t.Run(form, func(t *testing.T) {
			pidFile := filepath.Join(t.TempDir(), "pid")
			command := fmt.Sprintf(`SD_SHELL_HELPER=1 %q ignore-interrupt %q`, os.Args[0], pidFile)
			assertInterruptKills(t, fmt.Sprintf(form, command), pidFile)
		})
	}
}

func assertInterruptKills(t *testing.T, script, pidFile string) {
	t.Helper()
	out := &syncBuffer{}
	interrupts := make(chan os.Signal, 1)
	result := runAsync(t, script, interrupts, out)
	var pid int
	for pid == 0 {
		data, _ := os.ReadFile(pidFile)
		pid, _ = strconv.Atoi(string(data))
	}
	interrupts <- os.Interrupt
	waitInterrupted(t, result)
	if err := syscall.Kill(pid, 0); err == nil {
		_ = syscall.Kill(pid, syscall.SIGKILL)
		t.Fatalf("command %d still running after Run returned", pid)
	}
}

// Scripts without a shebang run in a nested interpreter; their background
// commands must still be stopped.
func TestInterruptKillsCommandsStartedByScriptsWithoutShebang(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "pid")
	script := fmt.Sprintf("SD_SHELL_HELPER=1 %q ignore-interrupt %q &\nwhile true; do :; done\n", os.Args[0], pidFile)
	if err := os.WriteFile(filepath.Join(dir, "script"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	assertInterruptKills(t, fmt.Sprintf("cd %q && ./script", dir), pidFile)
}

func TestScriptsWithoutShebangGetArgsAndExportedEnv(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "script"), []byte(`printf '%s|%s|%s|%s' "$1" "$#" "$EXPORTED" "$LOCAL"`), 0o755); err != nil {
		t.Fatal(err)
	}
	out, err := run(t, fmt.Sprintf(`cd %q && export EXPORTED=yes && LOCAL=no && ./script "two words" b`, dir))
	if err != nil || out != "two words|2|yes|" {
		t.Fatalf("out = %q err = %v", out, err)
	}
}

func TestScriptsWithoutShebangDoNotSeeUnsetVariables(t *testing.T) {
	t.Setenv("SD_TEST_INHERITED", "old")
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "script"), []byte(`printf '%s' "${SD_TEST_INHERITED-absent}"`), 0o755); err != nil {
		t.Fatal(err)
	}
	out, err := run(t, fmt.Sprintf(`cd %q && unset SD_TEST_INHERITED && ./script`, dir))
	if err != nil || out != "absent" {
		t.Fatalf("out = %q err = %v", out, err)
	}
}
