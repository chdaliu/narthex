package main

import (
	"path/filepath"
	"testing"
)

func TestResolveFromDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cases := []struct{ dir, path, want string }{
		{"/cfg", "", ""},
		{"/cfg", "/abs/x", "/abs/x"},
		{"/cfg", "rel.json", "/cfg/rel.json"},
		{"/cfg", "~/x", filepath.Join(home, "x")},
		{"/cfg", "~", home},
	}
	for _, c := range cases {
		if got := resolveFromDir(c.dir, c.path); got != c.want {
			t.Errorf("resolveFromDir(%q, %q) = %q, want %q", c.dir, c.path, got, c.want)
		}
	}
}

func TestDataDirFor(t *testing.T) {
	if got := dataDirFor("/cfg", "", "vscodium-data"); got != "/cfg/vscodium-data" {
		t.Errorf("fallback = %q, want /cfg/vscodium-data", got)
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	if got := dataDirFor("/cfg", "~/.vscodium-server", "vscodium-data"); got != filepath.Join(home, ".vscodium-server") {
		t.Errorf("configured = %q", got)
	}
}
