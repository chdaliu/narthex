// Package restart re-executes the running narthex process in place so a
// freshly built binary takes effect without changing the PID or dropping
// a launchd job.
package restart

import (
	"os"
	"path/filepath"
	"syscall"
)

// execve is a seam for tests.
var execve = syscall.Exec

// Self replaces the current process image with a fresh copy of the
// executable the process was started from, preserving argv and the
// environment. On success it does not return. It is the fallback for when
// narthex is not managed by launchd.
func Self() error {
	bin, err := os.Executable()
	if err != nil {
		return err
	}
	if resolved, err := filepath.EvalSymlinks(bin); err == nil {
		bin = resolved
	}
	bin, err = filepath.Abs(bin)
	if err != nil {
		return err
	}
	return execve(bin, os.Args, os.Environ())
}
