package api

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"narthex/internal/store"
)

const maxUploadSize = 15 << 20

// allowedUploadExts maps file extensions to content types.
var allowedUploadExts = map[string]string{
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".png":  "image/png",
	".webp": "image/webp",
}

// UploadExt reports whether ext (without dot) is an allowed upload
// extension.
func UploadExt(ext string) bool {
	_, ok := allowedUploadExts["."+ext]
	return ok
}

// UploadsDir returns the upload storage directory (inside the config dir).
func (s *Service) UploadsDir() string {
	return filepath.Join(s.Dir, "uploads")
}

// Background describes one selectable background (preset or upload).
type Background struct {
	ID     string `json:"id"`
	URL    string `json:"url"`
	Preset bool   `json:"preset"`
}

// backgrounds lists preset backgrounds plus uploaded images.
func (s *Service) backgrounds() []Background {
	list := []Background{}
	for _, name := range assetNames("assets/backgrounds", ".jpg") {
		list = append(list, Background{ID: name, URL: "/assets/backgrounds/" + name + ".jpg", Preset: true})
	}
	entries, err := os.ReadDir(s.UploadsDir())
	if err == nil {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			ext := strings.ToLower(filepath.Ext(e.Name()))
			if _, ok := allowedUploadExts[ext]; !ok {
				continue
			}
			id := strings.TrimSuffix(e.Name(), ext)
			list = append(list, Background{ID: id, URL: "/uploads/" + e.Name(), Preset: false})
		}
	}
	return list
}

// uploadExists reports whether id matches an uploaded image file.
func (s *Service) uploadExists(id string) bool {
	if !isHexID(id) {
		return false
	}
	for ext := range allowedUploadExts {
		if _, err := os.Stat(filepath.Join(s.UploadsDir(), id+ext)); err == nil {
			return true
		}
	}
	return false
}

// isHexID reports whether id is a lowercase hex string (16 chars).
func isHexID(id string) bool {
	if len(id) != 16 {
		return false
	}
	for _, c := range id {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// backgroundAllowed checks a pageBackground value: a preset name or an
// existing upload id.
func (s *Service) backgroundAllowed(id string) bool {
	for _, name := range assetNames("assets/backgrounds", ".jpg") {
		if name == id {
			return true
		}
	}
	return s.uploadExists(id)
}

// HandleUploadCreate stores an uploaded image and returns its id/url.
func (s *Service) HandleUploadCreate(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)
	file, header, err := r.FormFile("file")
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "err.uploadInvalid")
		return
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if _, ok := allowedUploadExts[ext]; !ok {
		s.writeError(w, http.StatusBadRequest, "err.uploadType")
		return
	}

	dir := s.UploadsDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		s.writeError(w, http.StatusInternalServerError, "err.saveFailed", err.Error())
		return
	}
	id := store.RandomID(8)
	dst := filepath.Join(dir, id+ext)
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "err.saveFailed", err.Error())
		return
	}
	written, err := io.Copy(out, io.LimitReader(file, maxUploadSize+1))
	out.Close()
	if err != nil || written > maxUploadSize {
		os.Remove(dst)
		if written > maxUploadSize {
			s.writeError(w, http.StatusRequestEntityTooLarge, "err.uploadTooLarge")
		} else {
			s.writeError(w, http.StatusInternalServerError, "err.saveFailed", "write failed")
		}
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{
		"id":   id,
		"url":  "/uploads/" + id + ext,
		"name": header.Filename,
	})
}

// HandleUploadDelete removes an uploaded image by id.
func (s *Service) HandleUploadDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !s.uploadExists(id) {
		s.writeError(w, http.StatusNotFound, "err.uploadNotFound")
		return
	}
	for ext := range allowedUploadExts {
		p := filepath.Join(s.UploadsDir(), id+ext)
		if err := os.Remove(p); err == nil {
			break
		}
	}
	WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
