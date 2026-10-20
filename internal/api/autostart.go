package api

import (
	"errors"
	"net/http"
	"path/filepath"

	"narthex/internal/autostart"
	"narthex/internal/store"
)

// autostartResponse is the JSON shape returned by /api/autostart. The
// supported flag is false on non-macOS hosts so the UI can render a
// disabled toggle with a hint instead of an error. brewManaged marks a
// `brew services` job for narthex, which takes precedence over the
// built-in autostart.
type autostartResponse struct {
	Supported   bool   `json:"supported"`
	Installed   bool   `json:"installed"`
	Loaded      bool   `json:"loaded"`
	BrewManaged bool   `json:"brewManaged"`
	Label       string `json:"label"`
	PlistPath   string `json:"plistPath"`
}

func autostartResp(st autostart.InstallStatus, err error, brewManaged bool) autostartResponse {
	resp := autostartResponse{BrewManaged: brewManaged}
	if err != nil {
		resp.Supported = !errors.Is(err, autostart.ErrUnsupportedOS)
		return resp
	}
	resp.Supported = true
	resp.Installed = st.Installed
	resp.Loaded = st.Loaded
	resp.Label = st.Label
	resp.PlistPath = st.PlistPath
	return resp
}

// HandleAutostart reports (GET) or toggles (POST) the macOS launchd job
// that auto-starts narthex serve at login (user-level LaunchAgent).
func (s *Service) HandleAutostart(w http.ResponseWriter, r *http.Request) {
	spec := autostart.Spec{
		Kind:       autostart.KindServe,
		ConfigPath: filepath.Join(s.Dir, store.ConfigFile),
	}

	if r.Method == http.MethodGet {
		st, err := autostart.Status(spec)
		WriteJSON(w, http.StatusOK, autostartResp(st, err, s.BrewManaged()))
		return
	}

	var req struct {
		Action string `json:"action"`
	}
	if err := readJSON(r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, "err.invalidRequest")
		return
	}
	switch req.Action {
	case "install":
		// While a brew services job is installed, ours would double-start
		// serve at login — refuse and let the UI surface the reason.
		if s.BrewManaged() {
			s.writeError(w, http.StatusConflict, "err.autostartBrewManaged")
			return
		}
		st, err := autostart.Install(spec)
		if err != nil {
			s.autostartWriteErr(w, err)
			return
		}
		WriteJSON(w, http.StatusOK, autostartResp(st, err, s.BrewManaged()))
	case "uninstall":
		// Uninstall stays allowed under brew management so a previously
		// installed built-in job can be cleaned up.
		st, err := autostart.Uninstall(spec)
		if err != nil {
			s.autostartWriteErr(w, err)
			return
		}
		WriteJSON(w, http.StatusOK, autostartResp(st, err, s.BrewManaged()))
	default:
		s.writeError(w, http.StatusBadRequest, "err.autostartUnknownAction", req.Action)
	}
}

func (s *Service) autostartWriteErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, autostart.ErrUnsupportedOS):
		s.writeError(w, http.StatusBadRequest, "err.autostartUnsupportedOS")
	default:
		s.writeError(w, http.StatusInternalServerError, "err.autostartInstallFailed", err)
	}
}
