package api

import (
	"context"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"narthex/internal/procman"
	"narthex/internal/store"
)

const installTimeout = 30 * time.Minute
const installOutputLimit = 64 << 10

// installRun is the in-memory state of one install command. It is not
// persisted: an install interrupted by a narthex restart is reported as gone
// rather than "running" forever.
type installRun struct {
	cardID     string
	running    bool
	command    string
	dir        string
	logPath    string
	pathDirs   []string
	autoStart  bool
	startedAt  time.Time
	finishedAt time.Time
	exitCode   int
	errText    string
	ctx        context.Context
	cancel     context.CancelFunc
}

// HandleInstallCard runs a custom card's install command asynchronously.
func (s *Service) HandleInstallCard(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.mu.Lock()
	idx := s.findCard(id)
	if idx < 0 {
		s.mu.Unlock()
		s.writeError(w, http.StatusNotFound, "err.cardNotFound")
		return
	}
	card := &s.State.Cards[idx]
	st, errKey := s.beginInstallLocked(card, false)
	if errKey != "" {
		s.mu.Unlock()
		status := http.StatusBadRequest
		if errKey == "err.installRunning" || errKey == "err.installCardRunning" {
			status = http.StatusConflict
		}
		s.writeError(w, status, errKey)
		return
	}
	s.mu.Unlock()
	go s.runInstall(st, card.ID, false, r.Host)
	WriteJSON(w, http.StatusAccepted, s.installStatus(st))
}

// HandleInstallStatus reports the current/last install run and its log tail.
func (s *Service) HandleInstallStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.mu.Lock()
	st := s.installs[id]
	s.mu.Unlock()
	if st == nil {
		WriteJSON(w, http.StatusOK, map[string]any{"running": false, "output": "", "action": "install"})
		return
	}
	WriteJSON(w, http.StatusOK, s.installStatus(st))
}

// HandleCancelInstall cancels a running install command.
func (s *Service) HandleCancelInstall(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.mu.Lock()
	st := s.installs[id]
	if st == nil || !st.running {
		s.mu.Unlock()
		s.writeError(w, http.StatusConflict, "err.installNotRunning")
		return
	}
	if st.cancel != nil {
		st.cancel()
	}
	s.mu.Unlock()
	WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// HandleUpdateCard runs a card's update command asynchronously. Unlike
// install, it is allowed while the card is running; the UI asks for
// confirmation first.
func (s *Service) HandleUpdateCard(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.mu.Lock()
	idx := s.findCard(id)
	if idx < 0 {
		s.mu.Unlock()
		s.writeError(w, http.StatusNotFound, "err.cardNotFound")
		return
	}
	card := &s.State.Cards[idx]
	st, errKey := s.beginUpdateLocked(card)
	if errKey != "" {
		s.mu.Unlock()
		status := http.StatusBadRequest
		if errKey == "err.updateRunning" {
			status = http.StatusConflict
		}
		s.writeError(w, status, errKey)
		return
	}
	s.mu.Unlock()
	go s.runUpdate(st)
	WriteJSON(w, http.StatusAccepted, s.updateStatus(st))
}

// HandleUpdateStatus reports the current/last update run and its log tail.
func (s *Service) HandleUpdateStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.mu.Lock()
	st := s.updates[id]
	s.mu.Unlock()
	if st == nil {
		WriteJSON(w, http.StatusOK, map[string]any{"running": false, "output": "", "action": "update"})
		return
	}
	WriteJSON(w, http.StatusOK, s.updateStatus(st))
}

// HandleCancelUpdate cancels a running update command.
func (s *Service) HandleCancelUpdate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.mu.Lock()
	st := s.updates[id]
	if st == nil || !st.running {
		s.mu.Unlock()
		s.writeError(w, http.StatusConflict, "err.updateNotRunning")
		return
	}
	if st.cancel != nil {
		st.cancel()
	}
	s.mu.Unlock()
	WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// beginUpdateLocked validates and registers an update run. Caller holds s.mu.
func (s *Service) beginUpdateLocked(card *store.Card) (*installRun, string) {
	if card.Update == nil || !card.Update.Enabled {
		return nil, "err.updateDisabled"
	}
	if strings.TrimSpace(card.Update.Command) == "" {
		return nil, "err.updateNoCommand"
	}
	if s.maintenanceRunningLocked(card.ID) {
		return nil, "err.updateRunning"
	}
	if s.RunInstall == nil {
		return nil, "err.installUnsupported"
	}
	ctx, cancel := context.WithTimeout(context.Background(), installTimeout)
	dir := s.installDir(card.Kind, card.Dir)
	st := &installRun{
		cardID:    card.ID,
		running:   true,
		command:   card.Update.Command,
		dir:       dir,
		logPath:   filepath.Join(s.Dir, store.LogsDir, card.ID+"-update.log"),
		pathDirs:  append([]string(nil), s.Config.Custom.PathDirs...),
		startedAt: time.Now(),
		ctx:       ctx,
		cancel:    cancel,
	}
	if card.Kind == store.KindCustom && card.Custom != nil && card.Custom.WorkDir != "" {
		st.dir = store.ExpandPath(s.Dir, card.Custom.WorkDir)
	}
	s.updates[card.ID] = st
	delete(s.lastErrors, card.ID)
	return st, ""
}

// runUpdate executes a registered update. It never holds s.mu while the
// command runs.
func (s *Service) runUpdate(st *installRun) {
	select {
	case s.installSem <- struct{}{}:
		defer func() { <-s.installSem }()
	case <-st.ctx.Done():
		s.finishUpdate(st, st.ctx.Err())
		return
	}
	err := s.RunInstall(st.ctx, st.command, st.dir, st.logPath, st.pathDirs)
	s.finishUpdate(st, err)
}

func (s *Service) finishUpdate(st *installRun, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if st.cancel != nil {
		st.cancel()
	}
	st.running = false
	st.finishedAt = time.Now()
	if err != nil {
		st.errText = err.Error()
		if ee, ok := err.(*exec.ExitError); ok {
			st.exitCode = ee.ExitCode()
		}
		return
	}
	st.errText = ""
}

// maintenanceRunningLocked reports whether install or update is running for
// the card. Caller holds s.mu.
func (s *Service) maintenanceRunningLocked(cardID string) bool {
	if r := s.installs[cardID]; r != nil && r.running {
		return true
	}
	if r := s.updates[cardID]; r != nil && r.running {
		return true
	}
	return false
}

// beginInstallLocked validates and registers an install run. Caller holds
// s.mu.
func (s *Service) beginInstallLocked(card *store.Card, autoStart bool) (*installRun, string) {
	if card.Kind != store.KindCustom || card.Custom == nil {
		return nil, "err.installDisabled"
	}
	if !card.Custom.InstallEnabled {
		return nil, "err.installDisabled"
	}
	if strings.TrimSpace(card.Custom.InstallCommand) == "" {
		return nil, "err.installNoCommand"
	}
	if s.Backend.Status(card.Kind, card.PID, card.Port).Alive {
		return nil, "err.installCardRunning"
	}
	if s.maintenanceRunningLocked(card.ID) {
		return nil, "err.installRunning"
	}
	if s.RunInstall == nil {
		return nil, "err.installUnsupported"
	}
	ctx, cancel := context.WithTimeout(context.Background(), installTimeout)
	st := &installRun{
		cardID:    card.ID,
		running:   true,
		command:   card.Custom.InstallCommand,
		dir:       s.installDir(card.Kind, card.Dir),
		logPath:   filepath.Join(s.Dir, store.LogsDir, card.ID+"-install.log"),
		pathDirs:  append([]string(nil), s.Config.Custom.PathDirs...),
		autoStart: autoStart,
		startedAt: time.Now(),
		ctx:       ctx,
		cancel:    cancel,
	}
	if card.Custom.WorkDir != "" {
		st.dir = store.ExpandPath(s.Dir, card.Custom.WorkDir)
	}
	s.installs[card.ID] = st
	delete(s.autoInstallFailed, card.ID)
	delete(s.lastErrors, card.ID)
	return st, ""
}

// runInstall executes a registered install and, for auto installs, starts
// the card when it succeeds. It never holds s.mu while the command runs.
func (s *Service) runInstall(st *installRun, cardID string, autoStart bool, reqHost string) {
	select {
	case s.installSem <- struct{}{}:
		defer func() { <-s.installSem }()
	case <-st.ctx.Done():
		s.finishInstall(st, st.ctx.Err(), false, reqHost)
		return
	}

	err := s.RunInstall(st.ctx, st.command, st.dir, st.logPath, st.pathDirs)
	s.finishInstall(st, err, autoStart, reqHost)
}

func (s *Service) finishInstall(st *installRun, err error, autoStart bool, reqHost string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if st.cancel != nil {
		st.cancel()
	}
	st.running = false
	st.finishedAt = time.Now()
	if err != nil {
		st.errText = err.Error()
		if ee, ok := err.(*exec.ExitError); ok {
			st.exitCode = ee.ExitCode()
		}
		s.autoInstallFailed[st.cardID] = autoStart
		return
	}
	st.errText = ""
	delete(s.autoInstallFailed, st.cardID)
	if !autoStart {
		return
	}
	idx := s.findCard(st.cardID)
	if idx < 0 {
		return
	}
	card := &s.State.Cards[idx]
	if s.Backend.Status(card.Kind, card.PID, card.Port).Alive {
		return
	}
	if _, errKey, startErr := s.startCardLocked(card, reqHost); errKey != "" {
		s.autoInstallFailed[st.cardID] = true
		if startErr != nil {
			s.lastErrors[st.cardID] = startErr.Error()
		}
	}
}

func (s *Service) installStatus(st *installRun) map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := readTail(st.logPath, installOutputLimit)
	return map[string]any{
		"running":  st.running,
		"exitCode": st.exitCode,
		"error":    st.errText,
		"output":   out,
		"logPath":  st.logPath,
		"action":   "install",
	}
}

// needsAutoInstallLocked reports whether a custom start should run the
// install command first because its configured program is missing.
func (s *Service) needsAutoInstallLocked(card *store.Card) bool {
	if card.Kind != store.KindCustom || card.Custom == nil {
		return false
	}
	c := card.Custom
	if !c.InstallEnabled || !c.InstallAuto {
		return false
	}
	if strings.TrimSpace(c.Exec) == "" {
		// Shell mode without an executable has no install detection.
		return false
	}
	_, err := procman.ResolveExecutable(c.Exec, s.Config.Custom.PathDirs)
	return err != nil
}

func (s *Service) updateStatus(st *installRun) map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := readTail(st.logPath, installOutputLimit)
	return map[string]any{
		"running":  st.running,
		"exitCode": st.exitCode,
		"error":    st.errText,
		"output":   out,
		"logPath":  st.logPath,
		"action":   "update",
	}
}

func readTail(path string, limit int) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return ""
	}
	off := info.Size() - int64(limit)
	if off < 0 {
		off = 0
	}
	if _, err := f.Seek(off, io.SeekStart); err != nil {
		return ""
	}
	b, err := io.ReadAll(io.LimitReader(f, int64(limit)))
	if err != nil {
		return ""
	}
	return string(b)
}
