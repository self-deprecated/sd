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
	pidFile := filepath.Join(t.TempDir(), "pid")
	script := fmt.Sprintf(`SD_SHELL_HELPER=1 %q ignore-interrupt %q`, os.Args[0], pidFile)
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
