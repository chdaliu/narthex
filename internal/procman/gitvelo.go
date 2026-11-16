package procman

import (
	"fmt"
	"os"
	"strconv"

	"narthex/internal/store"
)

// startGitvelo launches the GitVelo web service from the `gitvelo` CLI.
//
// GitVelo authenticates the web UI with a cookie session (gv_session), which
// narthex cannot inject the way it does opencode's HTTP basic auth. The card
// therefore has two spawn shapes:
//
//   - proxy mode (noAuth, the default): `gitvelo serve --no-auth --host
//     127.0.0.1 --port <n>`. The server skips its own login gate and binds
//     loopback only, so the port is never reachable unauthenticated from the
//     network; the narthex gateway fronts it and requires the narthex
//     session.
//   - direct mode: `gitvelo serve --host <configured> --port <n>`. The
//     server keeps its own login and is opened on its own port.
//
// The working directory dir is the user's home. The child runs in its own
// process group with output appended to <LogDir>/<id>.log (see spawn).
func prepareGitvelo(m *Manager, spec StartSpec) (PreparedLaunch, error) {
	bin := DetectGitvelo()
	if bin == "" {
		return PreparedLaunch{}, fmt.Errorf("GitVelo CLI (gitvelo) not found in PATH")
	}
	port, err := freePortIn(m.GitveloPortRange)
	if err != nil {
		return PreparedLaunch{}, err
	}
	noAuth := spec.NoAuth
	if spec.Launch != nil && spec.Launch.ProxyMode != "" {
		noAuth = spec.Launch.ProxyMode == store.CustomProxyGateway
	}
	host := m.GitveloHostname
	if host == "" {
		host = "127.0.0.1"
	}
	args := []string{"serve", "--host", host, "--port", strconv.Itoa(port)}
	if noAuth {
		// --no-auth trusts the reverse proxy: keep it loopback-only.
		args = []string{"serve", "--no-auth", "--host", "127.0.0.1", "--port", strconv.Itoa(port)}
	}
	return PreparedLaunch{Path: bin, Args: args, Dir: spec.Dir, Port: port}, nil
}

// DetectGitvelo returns the path to the GitVelo CLI (`gitvelo`), or "" when
// it is not installed. Resolution order: NARTHEX_GITVELO_BIN (test/container
// override), then the `gitvelo` executable on PATH or in a well-known
// Homebrew bin directory (see lookPath).
func DetectGitvelo() string {
	if v := os.Getenv("NARTHEX_GITVELO_BIN"); v != "" {
		return v
	}
	return lookPath("gitvelo")
}
