package procman

import "narthex/internal/store"

// Status is the live state of a managed instance.
type Status struct {
	Alive    bool
	Healthy  bool
	MemoryKB int64
}

// StartSpec describes one instance launch. It replaces the old positional
// Start arguments so custom cards can carry their own configuration
// (program, command, port mode, proxy mode) without widening the interface
// again.
type StartSpec struct {
	// Card is the card being started (Kind, ID and custom configuration).
	Card store.Card
	// Dir is the working directory resolved by the API for built-in kinds.
	Dir string
	// NoAuth asks the app's server to skip its own login gate so it can be
	// fronted by the narthex gateway (gitvelo proxy mode); it is ignored by
	// kinds whose server has no such switch.
	NoAuth bool
	// Launch is the effective launch configuration. Custom cards use it
	// directly; built-in launchers use their own preset/dynamic values and
	// only consult it for display-only fields (proxy mode).
	Launch *store.LaunchConfig
}

// Backend starts, stops and inspects managed instances.
type Backend interface {
	// Start launches the app/server described by spec. The backend writes
	// logs to <log dir>/<card ID>.log and returns the runtime port (0 for
	// cards that use no port).
	Start(spec StartSpec) (pid int, port int, err error)
	// Stop terminates the instance with pid.
	Stop(pid int) error
	// Status reports the live state of the instance.
	Status(kind string, pid, port int) Status
}
