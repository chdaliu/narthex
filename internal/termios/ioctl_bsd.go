//go:build darwin || freebsd || netbsd || openbsd

package termios

import "syscall"

const (
	ioctlGet = syscall.TIOCGETA
	ioctlSet = syscall.TIOCSETA
)
