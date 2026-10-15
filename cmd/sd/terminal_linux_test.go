package main

import (
	"bytes"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// sd must not query the terminal (and wait for a reply that may never come)
// unless it is about to draw the picker.
func TestCommandsDoNotQueryTheTerminal(t *testing.T) {
	controller, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		t.Skipf("no pseudo-terminal support: %v", err)
	}
	defer controller.Close()
	if err := unix.IoctlSetPointerInt(int(controller.Fd()), unix.TIOCSPTLCK, 0); err != nil {
		t.Fatal(err)
	}
	n, err := unix.IoctlGetUint32(int(controller.Fd()), unix.TIOCGPTN)
	if err != nil {
		t.Fatal(err)
	}
	terminal, err := os.OpenFile("/dev/pts/"+strconv.Itoa(int(n)), os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer terminal.Close()

	cmd := exec.Command(sdBinary, "--version")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = terminal, terminal, terminal
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")
	// Make the pseudo-terminal sd's controlling terminal, as in a real shell.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	start := time.Now()
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)
	_ = unix.SetNonblock(int(controller.Fd()), true)
	output := make([]byte, 4096)
	read, _ := controller.Read(output)
	output = output[:max(read, 0)]
	if bytes.Contains(output, []byte("\x1b]11;?")) || elapsed > time.Second {
		t.Fatalf("sd --version took %v and wrote %q", elapsed, output)
	}
}
