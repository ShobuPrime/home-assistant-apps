// fan-uart-hold keeps a serial break on a UART for as long as it runs.
//
// A break drives the port's TX line low. On the Home Assistant Yellow the fan's
// signal wire is on GPIO14, which is UART0 TX, so holding a break switches the
// fan off and releasing it lets the line idle high again, which is fan on.
//
// The break ends when this process exits for any reason. Clearing it on
// SIGTERM is a courtesy: the kernel clears it anyway when the last holder of
// the port closes it, which covers SIGKILL and crashes.
//
// Usage: fan-uart-hold /dev/ttyAMA0
package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// From asm-generic/ioctls.h. Identical on arm64 and amd64.
const (
	tiocsbrk = 0x5427
	tioccbrk = 0x5428
)

func ioctl(fd int, req uintptr) error {
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), req, 0); errno != 0 {
		return errno
	}
	return nil
}

// hold opens the port, asserts a break, and keeps it until stop is closed or
// orphaned reports true. It returns nil once the break has been released.
func hold(path string, stop <-chan struct{}, orphaned func() bool) error {
	// O_NONBLOCK: a serial port without carrier would otherwise block in open.
	fd, err := syscall.Open(path, syscall.O_RDWR|syscall.O_NOCTTY|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer syscall.Close(fd)

	if err := ioctl(fd, tiocsbrk); err != nil {
		return fmt.Errorf("set break on %s: %w", path, err)
	}
	defer ioctl(fd, tioccbrk)

	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-stop:
			return nil
		case <-tick.C:
			if orphaned() {
				return nil
			}
		}
	}
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: fan-uart-hold <serial device>")
		os.Exit(2)
	}

	stop := make(chan struct{})
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	go func() {
		<-sig
		close(stop)
	}()

	// If the daemon that started us dies without killing us, we are handed to
	// another parent and nothing would ever turn the fan back on.
	parent := os.Getppid()
	orphaned := func() bool { return os.Getppid() != parent }

	if err := hold(os.Args[1], stop, orphaned); err != nil {
		fmt.Fprintln(os.Stderr, "fan-uart-hold:", err)
		os.Exit(1)
	}
}
