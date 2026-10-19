package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSettingsDefaults(t *testing.T) {
	def := DefaultSettings()
	if def.CodecPolicy != "auto" {
		t.Errorf("expected default codec policy 'auto', got %q", def.CodecPolicy)
	}
	if def.Player != "inplace" {
		t.Errorf("expected default player 'inplace', got %q", def.Player)
	}
	if def.BrowserSpec == "" {
		t.Errorf("expected non-empty default browser spec")
	}
}

func TestSettingsSaveAndLoad(t *testing.T) {
	dir := t.TempDir()

	// Loading from non-existent directory returns defaults
	s, err := LoadSettings(dir)
	if err != nil {
		t.Fatalf("LoadSettings failed: %v", err)
	}
	def := DefaultSettings()
	if s != def {
		t.Errorf("LoadSettings on missing file = %+v, want %+v", s, def)
	}

	// Save custom settings
	custom := Settings{
		BrowserSpec: "firefox",
		CodecPolicy: "old-gpu",
		Player:      "external",
	}
	if err := SaveSettings(dir, custom); err != nil {
		t.Fatalf("SaveSettings failed: %v", err)
	}

	loaded, err := LoadSettings(dir)
	if err != nil {
		t.Fatalf("LoadSettings failed: %v", err)
	}
	if loaded != custom {
		t.Errorf("loaded = %+v, want %+v", loaded, custom)
	}
}

func TestSettingsNormalization(t *testing.T) {
	invalid := Settings{
		BrowserSpec: "",
		CodecPolicy: "invalid-codec-policy",
		Player:      "vlc",
	}
	norm := invalid.Normalize()
	def := DefaultSettings()

	if norm.BrowserSpec != def.BrowserSpec {
		t.Errorf("normalized BrowserSpec = %q, want %q", norm.BrowserSpec, def.BrowserSpec)
	}
	if norm.CodecPolicy != "auto" {
		t.Errorf("normalized CodecPolicy = %q, want %q", norm.CodecPolicy, "auto")
	}
	if norm.Player != "inplace" {
		t.Errorf("normalized Player = %q, want %q", norm.Player, "inplace")
	}
}

func TestStoreSettings(t *testing.T) {
	dir := t.TempDir()
	st, err := NewWithDir(dir)
	if err != nil {
		t.Fatalf("NewWithDir failed: %v", err)
	}

	if st.Settings() != DefaultSettings() {
		t.Errorf("initial store settings = %+v, want defaults", st.Settings())
	}

	custom := Settings{
		BrowserSpec: "chrome+kwallet",
		CodecPolicy: "any",
		Player:      "external",
	}
	if err := st.SaveSettings(custom); err != nil {
		t.Fatalf("SaveSettings failed: %v", err)
	}

	// Verify file was written
	if _, err := os.Stat(filepath.Join(dir, "settings.json")); err != nil {
		t.Fatalf("settings.json was not created: %v", err)
	}

	// Reload from new store instance
	st2, err := NewWithDir(dir)
	if err != nil {
		t.Fatalf("NewWithDir 2 failed: %v", err)
	}
	if st2.Settings() != custom {
		t.Errorf("reloaded store settings = %+v, want %+v", st2.Settings(), custom)
	}
}
