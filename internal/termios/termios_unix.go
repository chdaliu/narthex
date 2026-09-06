//go:build darwin || linux || freebsd || netbsd || openbsd

// Package termios reads a password from the terminal without echo.
package termios

import (
	"os"
	"syscall"
	"unsafe"
)

// ReadPassword prints prompt to stderr and reads a line from stdin with
// echo disabled. The trailing newline is not included in the result.
func ReadPassword(prompt string) (string, error) {
	os.Stderr.WriteString(prompt)
	fd := int(os.Stdin.Fd())

	var old syscall.Termios
	if _, _, errno := syscall.Syscall6(syscall.SYS_IOCTL, uintptr(fd), uintptr(ioctlGet), uintptr(unsafe.Pointer(&old)), 0, 0, 0); errno != 0 {
		return "", errno
	}
	newState := old
	newState.Lflag &^= syscall.ECHO | syscall.ECHONL
	if _, _, errno := syscall.Syscall6(syscall.SYS_IOCTL, uintptr(fd), uintptr(ioctlSet), uintptr(unsafe.Pointer(&newState)), 0, 0, 0); errno != 0 {
		return "", errno
	}
	defer func() {
		syscall.Syscall6(syscall.SYS_IOCTL, uintptr(fd), uintptr(ioctlSet), uintptr(unsafe.Pointer(&old)), 0, 0, 0)
		os.Stderr.WriteString("\n")
	}()

	var buf []byte
	b := make([]byte, 1)
	for {
		n, err := os.Stdin.Read(b)
		if err != nil || n == 0 {
			break
		}
		if b[0] == '\n' || b[0] == '\r' {
			break
		}
		buf = append(buf, b[0])
	}
	return string(buf), nil
}
