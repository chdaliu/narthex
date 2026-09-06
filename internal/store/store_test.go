package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	c := DefaultConfig()
	if c.Port != 9090 {
		t.Fatalf("default port = %d, want 9090", c.Port)
	}
	if c.Hostname != "0.0.0.0" {
		t.Fatalf("default hostname = %q, want 0.0.0.0", c.Hostname)
	}
	if c.Language != "en" {
		t.Fatalf("default language = %q, want en", c.Language)
	}
	if c.SessionTTLHours != 720 {
		t.Fatalf("default session TTL = %d, want 720", c.SessionTTLHours)
	}
}

func TestConfigRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ConfigFile)

	c := DefaultConfig()
	c.PasswordHash = "hash"
	c.SessionSecret = "secret"
	if err := SaveConfig(path, c); err != nil {
		t.Fatal(err)
	}

	got, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Port != c.Port || got.SessionSecret != "secret" || got.PasswordHash != "hash" {
		t.Fatalf("roundtrip mismatch: %+v", got)
	}
	if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("config should be 0600, got %v err=%v", fi.Mode().Perm(), err)
	}
}

func TestLoadConfigNormalizesEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ConfigFile)
	if err := os.WriteFile(path, []byte(`{"passwordHash":"x"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Port != 9090 || c.Hostname != "0.0.0.0" {
		t.Fatalf("empty fields should be normalized: %+v", c)
	}
	if c.Language != "en" {
		t.Fatalf("empty language should normalize to en: %q", c.Language)
	}
	if c.SessionTTLHours != 720 {
		t.Fatalf("empty session TTL should normalize to 720: %d", c.SessionTTLHours)
	}
}

func TestLoadConfigMissing(t *testing.T) {
	if _, err := LoadConfig(filepath.Join(t.TempDir(), ConfigFile)); err == nil {
		t.Fatal("missing config should error")
	}
}

func TestStateRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, StateFile)

	s, err := LoadState(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Cards) != 0 {
		t.Fatalf("missing state should be empty, got %+v", s)
	}

	s.Cards = []Card{{ID: "a1", Name: "P", Icon: "code", Background: "bg-01", PID: 42, Port: 8000, Kind: KindComfyUI}}
	if err := SaveState(path, s); err != nil {
		t.Fatal(err)
	}
	got, err := LoadState(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Cards) != 1 || got.Cards[0].PID != 42 || got.Cards[0].Name != "P" {
		t.Fatalf("roundtrip mismatch: %+v", got.Cards)
	}
}

func TestLoadStateKeepsOneCardPerKind(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, StateFile)
	state := `{"cards":[
		{"id":"a","kind":"comfyui","name":"first"},
		{"id":"b","kind":"comfyui","name":"dup"},
		{"id":"c","kind":"opencode","name":"oc"},
		{"id":"d","kind":"transmission","name":"tx"},
		{"id":"e","kind":"opencode","name":"dup-oc"},
		{"id":"f","kind":"weird","name":"obsolete"}
	]}`
	if err := os.WriteFile(path, []byte(state), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := LoadState(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Cards) != 2 {
		t.Fatalf("expected 2 cards (first per kind), got %+v", got.Cards)
	}
	if got.Cards[0].ID != "a" || got.Cards[0].Kind != KindComfyUI {
		t.Fatalf("first comfyui card should win: %+v", got.Cards[0])
	}
	if got.Cards[1].ID != "c" || got.Cards[1].Kind != KindOpencode {
		t.Fatalf("first opencode card should win: %+v", got.Cards[1])
	}
}

func TestRandomID(t *testing.T) {
	a := RandomID(8)
	b := RandomID(8)
	if len(a) != 16 || a == b {
		t.Fatalf("RandomID broken: %q %q", a, b)
	}
}

func TestSloganSemantics(t *testing.T) {
	// Default: nil = use the localized default slogan.
	c := DefaultConfig()
	if c.Slogan != nil {
		t.Fatalf("default slogan should be nil, got %v", *c.Slogan)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, ConfigFile)

	// Missing field → nil (localized default at read time).
	if err := os.WriteFile(path, []byte(`{"passwordHash":"x"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Slogan != nil {
		t.Fatalf("missing slogan should stay nil, got %v", *got.Slogan)
	}

	// Explicit empty string → hidden (off).
	if err := os.WriteFile(path, []byte(`{"passwordHash":"x","slogan":""}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err = LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Slogan == nil || *got.Slogan != "" {
		t.Fatalf("empty slogan should be preserved as off, got %v", got.Slogan)
	}

	// Custom value preserved.
	if err := os.WriteFile(path, []byte(`{"passwordHash":"x","slogan":"custom"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err = LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Slogan == nil || *got.Slogan != "custom" {
		t.Fatalf("custom slogan not preserved, got %v", got.Slogan)
	}
}
