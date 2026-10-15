package shell

import (
	"os"
	"strconv"
	"testing"

	"golang.org/x/sys/unix"
)

// openPTY returns the controller and terminal ends of a new pseudo-terminal.
func openPTY(t *testing.T) (*os.File, *os.File) {
	t.Helper()
	controller, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		t.Skipf("no pseudo-terminal support: %v", err)
	}
	t.Cleanup(func() { controller.Close() })
	unlock := 0
	if err := unix.IoctlSetPointerInt(int(controller.Fd()), unix.TIOCSPTLCK, unlock); err != nil {
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
	// The terminal end is not closed: an abandoned interpreter goroutine may
	// still be reading it. Closing the controller ends that read.
	return controller, terminal
}

// A silent read disables terminal echo; an interrupted task must restore it.
func TestInterruptRestoresTerminalAfterSilentRead(t *testing.T) {
	_, terminal := openPTY(t)
	before, err := unix.IoctlGetTermios(int(terminal.Fd()), unix.TCGETS)
	if err != nil {
		t.Fatal(err)
	}
	if before.Lflag&unix.ECHO == 0 {
		t.Fatal("expected a fresh terminal to echo")
	}
	out := &syncBuffer{}
	interrupts := make(chan os.Signal, 1)
	result := make(chan error, 1)
	go func() {
		result <- Run(interrupts, t.TempDir(), `printf ready; read -s value; printf after`, nil, os.Environ(), terminal, out, out)
	}()
	interruptWhen(t, out, "ready", interrupts)
	waitInterrupted(t, result)
	after, err := unix.IoctlGetTermios(int(terminal.Fd()), unix.TCGETS)
	if err != nil {
		t.Fatal(err)
	}
	if after.Lflag&unix.ECHO == 0 {
		t.Fatal("terminal echo left disabled after an interrupted silent read")
	}
}
