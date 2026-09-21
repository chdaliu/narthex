package autostart

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// withDarwin swaps the OS seam to true for the duration of a test so the
// Install/Uninstall/Status flow can be exercised on Linux/CI.
func withDarwin(t *testing.T) {
	t.Helper()
	prev := fnIsDarwin
	fnIsDarwin = func() bool { return true }
	t.Cleanup(func() { fnIsDarwin = prev })
}

// withFakes replaces every side-effectful seam with capturable fakes and
// restores them on cleanup.
type fakes struct {
	bootstrapCalls []struct{ domain, path string }
	bootoutCalls   []struct{ domain, label string }
	loadedResult   bool
	loadedCalls    []struct{ domain, label string }
	kickstartCalls []string
	kickstartErr   error
	files          map[string][]byte
	mkdirs         []string
}

func withFakes(t *testing.T, f *fakes) {
	t.Helper()
	if f.files == nil {
		f.files = map[string][]byte{}
	}
	prevBootstrap := fnBootstrap
	prevBootout := fnBootout
	prevLoaded := fnLoaded
	prevKickstart := fnKickstart
	prevWrite := fnWriteFile
	prevRemove := fnRemove
	prevStat := fnStat
	prevMkdir := fnMkdirAll
	fnBootstrap = func(domain, path string) error {
		f.bootstrapCalls = append(f.bootstrapCalls, struct{ domain, path string }{domain, path})
		return nil
	}
	fnBootout = func(domain, label string) error {
		f.bootoutCalls = append(f.bootoutCalls, struct{ domain, label string }{domain, label})
		return nil
	}
	fnLoaded = func(domain, label string) bool {
		f.loadedCalls = append(f.loadedCalls, struct{ domain, label string }{domain, label})
		return f.loadedResult
	}
	fnKickstart = func(target string) error {
		f.kickstartCalls = append(f.kickstartCalls, target)
		return f.kickstartErr
	}
	fnWriteFile = func(path string, data []byte, _ os.FileMode) error {
		f.files[path] = data
		return nil
	}
	fnRemove = func(path string) error {
		if _, ok := f.files[path]; !ok {
			return os.ErrNotExist
		}
		delete(f.files, path)
		return nil
	}
	fnStat = func(path string) (os.FileInfo, error) {
		if _, ok := f.files[path]; !ok {
			return nil, os.ErrNotExist
		}
		return nil, nil
	}
	fnMkdirAll = func(path string, _ os.FileMode) error {
		f.mkdirs = append(f.mkdirs, path)
		return nil
	}
	t.Cleanup(func() {
		fnBootstrap = prevBootstrap
		fnBootout = prevBootout
		fnLoaded = prevLoaded
		fnKickstart = prevKickstart
		fnWriteFile = prevWrite
		fnRemove = prevRemove
		fnStat = prevStat
		fnMkdirAll = prevMkdir
	})
}

func TestValidate(t *testing.T) {
	if err := Validate(Spec{Kind: KindServe}); err != nil {
		t.Fatalf("serve should be valid: %v", err)
	}
	if err := Validate(Spec{Kind: "agent"}); err == nil {
		t.Fatal("agent kind should be invalid now")
	}
	if err := Validate(Spec{Kind: "other"}); err == nil {
		t.Fatal("unknown kind should be invalid")
	}
}

func TestLabel(t *testing.T) {
	if got := Label(Spec{Kind: KindServe}); got != "com.narthex.serve" {
		t.Fatalf("Label = %q, want com.narthex.serve", got)
	}
}

func TestPlistPath(t *testing.T) {
	t.Run("user-level under home LaunchAgents", func(t *testing.T) {
		p, err := PlistPath(Spec{Kind: KindServe})
		if err != nil {
			t.Fatal(err)
		}
		home, _ := os.UserHomeDir()
		want := filepath.Join(home, "Library", "LaunchAgents", "com.narthex.serve.plist")
		if p != want {
			t.Errorf("got %q, want %q", p, want)
		}
	})
}

func TestLogPathDefault(t *testing.T) {
	spec := Spec{Kind: KindServe, ConfigPath: "/home/u/.config/narthex/config.json"}
	got := logPath(spec)
	want := "/home/u/.config/narthex/serve.log"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if spec.LogPath != "" {
		t.Error("spec should be unchanged")
	}
}

func TestLogPathOverride(t *testing.T) {
	spec := Spec{Kind: KindServe, LogPath: "/var/log/narthex.log"}
	if got := logPath(spec); got != spec.LogPath {
		t.Errorf("got %q, want %q", got, spec.LogPath)
	}
}

func TestProgramArgs(t *testing.T) {
	t.Run("serve includes --config", func(t *testing.T) {
		spec := Spec{Kind: KindServe, BinaryPath: "/x/narthex", ConfigPath: "/c/config.json"}
		args, err := programArgs(spec)
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"/x/narthex", "serve", "--config", "/c/config.json"}
		if strings.Join(args, " ") != strings.Join(want, " ") {
			t.Errorf("got %v, want %v", args, want)
		}
	})
	t.Run("serve omits --config when empty", func(t *testing.T) {
		spec := Spec{Kind: KindServe, BinaryPath: "/x/narthex"}
		args, err := programArgs(spec)
		if err != nil {
			t.Fatal(err)
		}
		if len(args) != 2 || args[0] != "/x/narthex" || args[1] != KindServe {
			t.Errorf("got %v", args)
		}
	})
}

func TestRender(t *testing.T) {
	spec := Spec{Kind: KindServe, BinaryPath: "/x/narthex", ConfigPath: "/c/config.json"}
	out, err := Render(spec)
	if err != nil {
		t.Fatal(err)
	}
	checks := []string{
		`<string>com.narthex.serve</string>`,
		`<string>/x/narthex</string>`,
		`<string>serve</string>`,
		`<string>--config</string>`,
		`<string>/c/config.json</string>`,
		`<key>RunAtLoad</key>`,
		`<true/>`,
		`<key>KeepAlive</key>`,
		`<string>/c/serve.log</string>`,
	}
	for _, c := range checks {
		if !strings.Contains(out, c) {
			t.Errorf("plist missing %q\n---\n%s", c, out)
		}
	}
}

func TestRenderEscapesXML(t *testing.T) {
	spec := Spec{Kind: KindServe, BinaryPath: `/a&b<c>`, ConfigPath: `/c&p/config.json`}
	out, err := Render(spec)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, `/a&b<c>`) {
		t.Error("binary path was not XML-escaped")
	}
	if !strings.Contains(out, `/a&amp;b&lt;c&gt;`) {
		t.Errorf("expected escaped binary path in plist\n%s", out)
	}
}

func TestRenderRejectsBadKind(t *testing.T) {
	if _, err := Render(Spec{Kind: "bogus"}); err == nil {
		t.Error("expected an error for a bogus kind")
	}
}

func TestInstallUserLevel(t *testing.T) {
	// fakes replace every launchctl / fs call, so the real launchd is never
	// touched regardless of host OS.
	withDarwin(t)
	f := &fakes{}
	withFakes(t, f)

	plistPath, err := PlistPath(Spec{Kind: KindServe})
	if err != nil {
		t.Fatal(err)
	}
	spec := Spec{Kind: KindServe, BinaryPath: "/x/narthex", ConfigPath: "/c/config.json"}
	st, err := Install(spec)
	if err != nil {
		t.Fatal(err)
	}
	if !st.Installed || !st.Loaded {
		t.Errorf("status = %+v, want installed+loaded", st)
	}
	if st.PlistPath != plistPath {
		t.Errorf("plist path = %q, want %q", st.PlistPath, plistPath)
	}
	if len(f.bootstrapCalls) != 1 {
		t.Fatalf("bootstrap calls = %v, want 1", f.bootstrapCalls)
	}
	if f.bootstrapCalls[0].domain != "gui/501" {
		t.Errorf("domain = %q, want gui/501", f.bootstrapCalls[0].domain)
	}
	if f.bootstrapCalls[0].path != plistPath {
		t.Errorf("bootstrap plist = %q, want %q", f.bootstrapCalls[0].path, plistPath)
	}
	// bootout is called once first (idempotent reinstall path).
	if len(f.bootoutCalls) != 1 {
		t.Fatalf("bootout calls = %v, want 1", f.bootoutCalls)
	}
	if f.bootoutCalls[0].domain != "gui/501" || f.bootoutCalls[0].label != "com.narthex.serve" {
		t.Errorf("bootout = %v", f.bootoutCalls[0])
	}
	data, ok := f.files[plistPath]
	if !ok {
		t.Fatalf("plist file not written at %s", plistPath)
	}
	if !strings.Contains(string(data), "<string>com.narthex.serve</string>") {
		t.Errorf("plist content wrong:\n%s", string(data))
	}
}

func TestInstallRejectsNonDarwin(t *testing.T) {
	// fnIsDarwin is the real default on the host platform.
	if fnIsDarwin() {
		t.Skip("only runs on non-darwin")
	}
	_, err := Install(Spec{Kind: KindServe, BinaryPath: "/x/narthex", ConfigPath: "/c/config.json"})
	if !errors.Is(err, ErrUnsupportedOS) {
		t.Errorf("expected ErrUnsupportedOS, got %v", err)
	}
}

func TestUninstall(t *testing.T) {
	withDarwin(t)
	f := &fakes{}
	withFakes(t, f)

	// Seed an existing plist file.
	plistPath, _ := PlistPath(Spec{Kind: KindServe})
	f.files[plistPath] = []byte("placeholder")

	st, err := Uninstall(Spec{Kind: KindServe})
	if err != nil {
		t.Fatal(err)
	}
	if st.Installed {
		t.Error("after uninstall, installed should be false")
	}
	if _, ok := f.files[plistPath]; ok {
		t.Error("plist file should have been removed")
	}
	if len(f.bootoutCalls) != 1 {
		t.Fatalf("bootout calls = %v, want 1", f.bootoutCalls)
	}
}

func TestUninstallIdempotent(t *testing.T) {
	withDarwin(t)
	f := &fakes{}
	withFakes(t, f)
	// No plist seeded.
	if _, err := Uninstall(Spec{Kind: KindServe}); err != nil {
		t.Errorf("uninstall of missing plist should not error, got %v", err)
	}
}

func TestStatus(t *testing.T) {
	withDarwin(t)
	f := &fakes{loadedResult: true}
	withFakes(t, f)

	plistPath, _ := PlistPath(Spec{Kind: KindServe})
	f.files[plistPath] = []byte("placeholder")

	st, err := Status(Spec{Kind: KindServe})
	if err != nil {
		t.Fatal(err)
	}
	if !st.Installed {
		t.Error("expected installed")
	}
	if !st.Loaded {
		t.Error("expected loaded")
	}
	if len(f.loadedCalls) != 1 || f.loadedCalls[0].label != "com.narthex.serve" {
		t.Fatalf("loaded calls = %v", f.loadedCalls)
	}

	// Remove the file → not installed, no launchctl check.
	delete(f.files, plistPath)
	f.loadedCalls = nil
	st, err = Status(Spec{Kind: KindServe})
	if err != nil {
		t.Fatal(err)
	}
	if st.Installed || st.Loaded {
		t.Errorf("expected not installed, got %+v", st)
	}
	if len(f.loadedCalls) != 0 {
		t.Errorf("launchctl should not be called when plist absent, got %v", f.loadedCalls)
	}
}

func TestRestart(t *testing.T) {
	withDarwin(t)
	f := &fakes{}
	withFakes(t, f)

	if err := Restart(Spec{Kind: KindServe}); err != nil {
		t.Fatal(err)
	}
	if len(f.kickstartCalls) != 1 {
		t.Fatalf("kickstart calls = %v, want 1", f.kickstartCalls)
	}
	want := "gui/" + strconv.Itoa(os.Getuid()) + "/com.narthex.serve"
	if f.kickstartCalls[0] != want {
		t.Errorf("kickstart target = %q, want %q", f.kickstartCalls[0], want)
	}

	// A launchctl failure is surfaced to the caller.
	f.kickstartErr = errors.New("boom")
	if err := Restart(Spec{Kind: KindServe}); err == nil {
		t.Error("expected kickstart failure to be returned")
	}
}

func TestRestartRejectsNonDarwin(t *testing.T) {
	if fnIsDarwin() {
		t.Skip("only runs on non-darwin")
	}
	if err := Restart(Spec{Kind: KindServe}); !errors.Is(err, ErrUnsupportedOS) {
		t.Errorf("expected ErrUnsupportedOS, got %v", err)
	}
}
