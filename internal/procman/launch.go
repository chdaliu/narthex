package procman

import (
	"fmt"
	"os/exec"

	"narthex/internal/store"
)

// PreparedLaunch is the common command description produced by a launcher.
// The manager owns the actual spawn, log, process-group and status handling.
type PreparedLaunch struct {
	Path   string
	Args   []string
	Env    []string
	Dir    string
	Port   int
	OnExit []func(pid int)
	// AfterStart runs immediately after a successful spawn (used by WeTTY to
	// verify the child survived and record its sidecar state).
	AfterStart func(pid int) error
	// Cleanup releases side effects when the spawn or AfterStart fails.
	Cleanup func()
}

type launcher struct {
	prepare func(m *Manager, spec StartSpec) (PreparedLaunch, error)
}

var launchers = map[string]launcher{
	store.KindComfyUI:    {prepare: prepareComfyUI},
	store.KindOpencode:   {prepare: prepareOpencode},
	store.KindMdbook:     {prepare: prepareMdbook},
	store.KindGitvelo:    {prepare: prepareGitvelo},
	store.KindVSCode:     {prepare: prepareVSCode},
	store.KindVSCodium:   {prepare: prepareVSCodium},
	store.KindWetty:      {prepare: prepareWetty},
	store.KindEmulatorJS: {prepare: prepareEmulatorJS},
	store.KindCustom:     {prepare: prepareCustom},
}

// Start launches the instance described by spec through the registered
// launcher. The child runs in its own process group so Stop can terminate the
// whole tree. Output is appended to <LogDir>/<card ID>.log.
func (m *Manager) Start(spec StartSpec) (pid int, port int, err error) {
	l, ok := launchers[spec.Card.Kind]
	if !ok {
		return 0, 0, fmt.Errorf("unsupported service kind: %s", spec.Card.Kind)
	}
	p, err := l.prepare(m, spec)
	if err != nil {
		return 0, 0, err
	}
	cmd := exec.Command(p.Path, p.Args...)
	cmd.Dir = p.Dir
	cmd.Env = p.Env

	pid, err = m.spawn(cmd, spec.Card.ID, p.OnExit...)
	if err != nil {
		if p.Cleanup != nil {
			p.Cleanup()
		}
		return 0, 0, err
	}
	if p.AfterStart != nil {
		if err := p.AfterStart(pid); err != nil {
			Stop(pid)
			if p.Cleanup != nil {
				p.Cleanup()
			}
			return 0, 0, err
		}
	}
	return pid, p.Port, nil
}
