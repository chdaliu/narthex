package procman

// Status is the live state of a managed instance.
type Status struct {
	Alive    bool
	Healthy  bool
	MemoryKB int64
}

// Backend starts, stops and inspects managed instances.
type Backend interface {
	// Start launches the app/server for the given kind (see store.Kind*).
	// id identifies the card; the backend is responsible for writing logs
	// to <log dir>/<id>.log.
	Start(kind, dir, id string) (pid int, port int, err error)
	// Stop terminates the instance with pid.
	Stop(pid int) error
	// Status reports the live state of the instance.
	Status(kind string, pid, port int) Status
}
