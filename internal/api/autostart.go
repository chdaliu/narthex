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
// disabled toggle with a hint instead of an error.
type autostartResponse struct {
	Supported bool   `json:"supported"`
	Installed bool   `json:"installed"`
	Loaded    bool   `json:"loaded"`
	Label     string `json:"label"`
	PlistPath string `json:"plistPath"`
}

func autostartResp(st autostart.InstallStatus, err error) autostartResponse {
	resp := autostartResponse{}
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
		WriteJSON(w, http.StatusOK, autostartResp(st, err))
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
		st, err := autostart.Install(spec)
		if err != nil {
			s.autostartWriteErr(w, err)
			return
		}
		WriteJSON(w, http.StatusOK, autostartResp(st, err))
	case "uninstall":
		st, err := autostart.Uninstall(spec)
		if err != nil {
			s.autostartWriteErr(w, err)
			return
		}
		WriteJSON(w, http.StatusOK, autostartResp(st, err))
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
