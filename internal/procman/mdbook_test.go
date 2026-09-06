package procman

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeFakeMdbookInit(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "mdbook")
	shim := `#!/bin/sh
# args: init <name> --force --ignore none --title <name>
name="$2"
mkdir -p "$name/src"
printf '[book]\ntitle = "x"\n' > "$name/book.toml"
printf '# Summary\n\n- [Chapter 1](./chapter_1.md)\n' > "$name/src/SUMMARY.md"
printf '# Chapter 1\n' > "$name/src/chapter_1.md"
`
	if err := os.WriteFile(bin, []byte(shim), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin
}

// writeFakeMdbookServe creates a fake `mdbook` CLI that mimics `mdbook
// serve` by serving HTTP on the port given via `--port`.
func writeFakeMdbookServe(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "mdbook")
	shim := `#!/bin/sh
if [ "$1" = "serve" ]; then
  port=4500
  prev=
  for a in "$@"; do
    if [ "$prev" = "--port" ]; then port=$a; fi
    prev=$a
  done
  exec python3 -m http.server "$port" --bind 127.0.0.1
fi
exit 0
`
	if err := os.WriteFile(bin, []byte(shim), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin
}

func TestDetectMdbook(t *testing.T) {
	t.Setenv("NARTHEX_MDBOOK_BIN", "/opt/fake/bin/mdbook")
	if got := DetectMdbook(); got != "/opt/fake/bin/mdbook" {
		t.Fatalf("DetectMdbook with override = %q", got)
	}
	t.Setenv("NARTHEX_MDBOOK_BIN", "")
	dir := t.TempDir()
	bin := filepath.Join(dir, "mdbook")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	if got := DetectMdbook(); got != bin {
		t.Fatalf("DetectMdbook from PATH = %q, want %q", got, bin)
	}
}

func TestDetectMdbookProjects(t *testing.T) {
	root := t.TempDir()
	// A configured dir that is itself a book.
	rootBook := filepath.Join(root, "root-book")
	mustWrite(t, filepath.Join(rootBook, "book.toml"), "[book]\n")
	// A configured dir holding subprojects.
	books := filepath.Join(root, "books")
	for _, name := range []string{"alpha", "beta"} {
		mustWrite(t, filepath.Join(books, name, "book.toml"), "[book]\n")
	}
	// A subdirectory without book.toml is not a project.
	if err := os.MkdirAll(filepath.Join(books, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	got := DetectMdbookProjects([]string{books, rootBook, filepath.Join(root, "missing")})
	if len(got) != 3 {
		t.Fatalf("projects = %+v, want 3", got)
	}
	for _, want := range []string{
		filepath.Join(books, "alpha"),
		filepath.Join(books, "beta"),
		rootBook,
	} {
		if !hasProject(got, want) {
			t.Fatalf("missing project %s in %+v", want, got)
		}
	}
}

func hasProject(projects []MdbookProject, path string) bool {
	for _, p := range projects {
		if p.Path == path {
			return true
		}
	}
	return false
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestCreateMdbookProject(t *testing.T) {
	bin := writeFakeMdbookInit(t)
	parent := t.TempDir()
	if err := CreateMdbookProject(bin, parent, "mybook"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(parent, "mybook", "book.toml")); err != nil {
		t.Fatalf("book.toml missing after init: %v", err)
	}
	if err := CreateMdbookProject(bin, parent, "mybook"); err == nil {
		t.Fatal("re-creating an existing book should error")
	}
	for _, bad := range []string{"", "../evil", "a/b", ".hidden", "..", strings.Repeat("x", 65)} {
		if err := CreateMdbookProject(bin, parent, bad); err == nil {
			t.Fatalf("name %q should be rejected", bad)
		}
	}
	if err := CreateMdbookProject("", parent, "x"); err == nil {
		t.Fatal("empty bin should error")
	}
}

func TestValidBookName(t *testing.T) {
	for _, ok := range []string{"book", "my-book_1", "Café", "a"} {
		if !ValidBookName(ok) {
			t.Errorf("ValidBookName(%q) should be true", ok)
		}
	}
	for _, bad := range []string{"", ".hidden", "..", "a/b", "a\\b", strings.Repeat("x", 65)} {
		if ValidBookName(bad) {
			t.Errorf("ValidBookName(%q) should be false", bad)
		}
	}
}

func TestMdbookLifecycle(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available; needed for the mdbook shim")
	}
	bin := writeFakeMdbookServe(t)
	t.Setenv("NARTHEX_MDBOOK_BIN", bin)
	bookDir := t.TempDir()
	mustWrite(t, filepath.Join(bookDir, "book.toml"), "[book]\n")
	m := &Manager{
		MdbookHostname:  "127.0.0.1",
		MdbookPortRange: [2]int{4580, 4599},
		LogDir:          t.TempDir(),
	}
	pid, port, err := m.Start("mdbook", bookDir, "mb1")
	if err != nil {
		t.Fatal(err)
	}
	if pid <= 0 || port < 4580 || port > 4599 {
		t.Fatalf("unexpected pid=%d port=%d", pid, port)
	}
	deadline := time.Now().Add(5 * time.Second)
	st := m.Status("mdbook", pid, port)
	for (!st.Alive || !st.Healthy) && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
		st = m.Status("mdbook", pid, port)
	}
	if !st.Alive || !st.Healthy {
		t.Fatalf("mdbook shim should become alive+healthy: %+v", st)
	}
	if err := m.Stop(pid); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(3 * time.Second)
	for m.Status("mdbook", pid, port).Alive && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if m.Status("mdbook", pid, port).Alive {
		t.Fatal("mdbook shim should be dead after stop")
	}
}

func TestMdbookStartError(t *testing.T) {
	t.Setenv("NARTHEX_MDBOOK_BIN", "")
	t.Setenv("PATH", t.TempDir())
	m := &Manager{
		MdbookHostname:  "127.0.0.1",
		MdbookPortRange: [2]int{4580, 4599},
		LogDir:          t.TempDir(),
	}
	if _, _, err := m.Start("mdbook", t.TempDir(), "x"); err == nil {
		t.Fatal("start without an mdbook binary should error")
	}
}
