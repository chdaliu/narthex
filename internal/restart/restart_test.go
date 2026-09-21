package restart

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSelfExecsResolvedPath(t *testing.T) {
	var gotBin string
	var gotArgv, gotEnv []string
	prev := execve
	execve = func(bin string, argv, env []string) error {
		gotBin, gotArgv, gotEnv = bin, argv, env
		return nil
	}
	t.Cleanup(func() { execve = prev })

	if err := Self(); err != nil {
		t.Fatal(err)
	}
	want, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if resolved, err := filepath.EvalSymlinks(want); err == nil {
		want = resolved
	}
	want, _ = filepath.Abs(want)
	if gotBin != want {
		t.Errorf("exec path = %q, want %q", gotBin, want)
	}
	if len(gotArgv) != len(os.Args) {
		t.Errorf("argv len = %d, want %d", len(gotArgv), len(os.Args))
	}
	if len(gotEnv) == 0 {
		t.Error("environment should be preserved")
	}
}

func TestSelfPropagatesExecError(t *testing.T) {
	prev := execve
	execve = func(string, []string, []string) error { return os.ErrPermission }
	t.Cleanup(func() { execve = prev })
	if err := Self(); err == nil {
		t.Fatal("expected exec error to propagate")
	}
}
