package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

// openPTY returns the path of a fresh pseudo-terminal's slave side. A pty
// accepts TIOCSBRK and ignores it, which is enough to exercise hold without a
// real UART.
func openPTY(t *testing.T) string {
	t.Helper()
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		t.Skipf("no /dev/ptmx: %v", err)
	}
	t.Cleanup(func() { master.Close() })

	var unlock int32
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, master.Fd(), syscall.TIOCSPTLCK, uintptr(unsafe.Pointer(&unlock))); errno != 0 {
		t.Skipf("cannot unlock pty: %v", errno)
	}
	var n uint32
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, master.Fd(), syscall.TIOCGPTN, uintptr(unsafe.Pointer(&n))); errno != 0 {
		t.Skipf("cannot name pty: %v", errno)
	}
	return fmt.Sprintf("/dev/pts/%d", n)
}

func TestHoldRejectsMissingDevice(t *testing.T) {
	err := hold(filepath.Join(t.TempDir(), "nope"), make(chan struct{}), func() bool { return false })
	if !errors.Is(err, syscall.ENOENT) {
		t.Fatalf("want ENOENT, got %v", err)
	}
}

// A regular file is not a terminal, so the break ioctl must fail rather than
// leave the caller believing the line is held.
func TestHoldRejectsNonTerminal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	err := hold(path, make(chan struct{}), func() bool { return false })
	if !errors.Is(err, syscall.ENOTTY) {
		t.Fatalf("want ENOTTY, got %v", err)
	}
}

func TestHoldReleasesOnStop(t *testing.T) {
	pty := openPTY(t)
	stop := make(chan struct{})
	done := make(chan error, 1)
	go func() { done <- hold(pty, stop, func() bool { return false }) }()

	select {
	case err := <-done:
		t.Fatalf("hold returned before stop: %v", err)
	case <-time.After(200 * time.Millisecond):
	}
	close(stop)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("hold: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("hold did not return after stop")
	}
}

func TestHoldReleasesWhenOrphaned(t *testing.T) {
	pty := openPTY(t)
	done := make(chan error, 1)
	go func() { done <- hold(pty, make(chan struct{}), func() bool { return true }) }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("hold: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("hold kept the break after its parent went away")
	}
}
