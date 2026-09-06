package procman

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// startMdbook launches the mdBook server for the book in dir:
// `mdbook serve --port <port> --hostname <host>`. mdBook does not open a
// browser unless --open is passed.
func (m *Manager) startMdbook(dir, id string, port int) (int, error) {
	bin := DetectMdbook()
	if bin == "" {
		return 0, fmt.Errorf("mdbook CLI not found in PATH")
	}
	cmd := exec.Command(bin, "serve", "--port", strconv.Itoa(port), "--hostname", m.MdbookHostname)
	cmd.Dir = dir
	return m.spawn(cmd, id)
}

// MdbookProject is a recognized mdBook book: a directory containing a
// book.toml, found under one of the configured project directories.
type MdbookProject struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// DetectMdbookProjects scans the configured dirs one level deep for mdBook
// projects. Each configured dir itself counts as a project when it holds a
// book.toml; every immediate subdirectory with a book.toml is a project
// too. Results are sorted by path.
func DetectMdbookProjects(dirs []string) []MdbookProject {
	seen := map[string]bool{}
	var out []MdbookProject
	for _, d := range dirs {
		d = filepath.Clean(d)
		if isBook(d) && !seen[d] {
			seen[d] = true
			out = append(out, MdbookProject{Name: filepath.Base(d), Path: d})
		}
		entries, err := os.ReadDir(d)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			sub := filepath.Join(d, e.Name())
			if !isBook(sub) {
				continue
			}
			if !seen[sub] {
				seen[sub] = true
				out = append(out, MdbookProject{Name: e.Name(), Path: sub})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

func isBook(dir string) bool {
	st, err := os.Stat(filepath.Join(dir, "book.toml"))
	return err == nil && !st.IsDir()
}

// DetectMdbook returns the path to the mdbook CLI, or "" when it is not
// installed. Resolution order: NARTHEX_MDBOOK_BIN (test/container
// override), then the mdbook executable on PATH.
func DetectMdbook() string {
	if v := os.Getenv("NARTHEX_MDBOOK_BIN"); v != "" {
		return v
	}
	if p, err := exec.LookPath("mdbook"); err == nil {
		return p
	}
	return ""
}

// ValidBookName reports whether name is usable as a new book directory:
// 1-64 characters, no path separators, not "." or "..", and not hidden.
func ValidBookName(name string) bool {
	if len(name) < 1 || len(name) > 64 {
		return false
	}
	if name == "." || name == ".." || strings.HasPrefix(name, ".") {
		return false
	}
	return !strings.ContainsAny(name, `/\`)
}

// CreateMdbookProject runs `mdbook init <name> --force --ignore none
// --title <name>` inside parent, creating parent/<name> as a new book.
// The flags keep the command fully non-interactive: --force skips the
// prompts and --ignore none skips the .gitignore question.
func CreateMdbookProject(bin, parent, name string) error {
	if bin == "" {
		return errors.New("mdbook CLI not found")
	}
	if !ValidBookName(name) {
		return fmt.Errorf("invalid book name %q", name)
	}
	target := filepath.Join(parent, name)
	if _, err := os.Stat(target); err == nil {
		return fmt.Errorf("%s already exists", target)
	}
	cmd := exec.Command(bin, "init", name, "--force", "--ignore", "none", "--title", name)
	cmd.Dir = parent
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("mdbook init: %w", err)
	}
	return nil
}
