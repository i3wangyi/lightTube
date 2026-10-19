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
	if def.ActiveAccount != "default" {
		t.Errorf("expected active account 'default', got %q", def.ActiveAccount)
	}
	if len(def.Accounts) != 1 || def.Accounts[0].ID != "default" {
		t.Errorf("expected default account in accounts list, got %+v", def.Accounts)
	}
}

func TestSettingsSaveAndLoad(t *testing.T) {
	dir := t.TempDir()

	// Loading from non-existent directory returns defaults
	s, err := LoadSettings(dir)
	if err != nil {
		t.Fatalf("LoadSettings failed: %v", err)
	}
	def := DefaultSettingsWithDir(dir)
	if s.ActiveAccount != def.ActiveAccount || s.CodecPolicy != def.CodecPolicy || len(s.Accounts) != len(def.Accounts) {
		t.Errorf("LoadSettings on missing file = %+v, want %+v", s, def)
	}

	// Save custom settings
	custom := Settings{
		BrowserSpec:   "firefox",
		CodecPolicy:   "old-gpu",
		Player:        "external",
		ActiveAccount: "default",
		Accounts: []AccountConfig{
			{
				ID:          "default",
				Label:       "default",
				BrowserSpec: "firefox",
				CookieFile:  filepath.Join(dir, "cookies.txt"),
				CacheDir:    filepath.Join(dir, "cache", "account"),
			},
		},
	}
	if err := SaveSettings(dir, custom); err != nil {
		t.Fatalf("SaveSettings failed: %v", err)
	}

	loaded, err := LoadSettings(dir)
	if err != nil {
		t.Fatalf("LoadSettings failed: %v", err)
	}
	if loaded.CodecPolicy != custom.CodecPolicy || loaded.ActiveAccount != custom.ActiveAccount || len(loaded.Accounts) != 1 {
		t.Errorf("loaded = %+v, want %+v", loaded, custom)
	}
}

func TestAccountMigrationWithoutMovingFiles(t *testing.T) {
	dir := t.TempDir()

	// Create legacy single-account cookies.txt
	cookieFile := filepath.Join(dir, "cookies.txt")
	if err := os.WriteFile(cookieFile, []byte("fake cookies"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Legacy settings.json without accounts array
	legacySettingsJSON := `{"browser_spec":"chrome+gnomekeyring","codec_policy":"auto","player":"inplace"}`
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(legacySettingsJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	st, err := NewWithDir(dir)
	if err != nil {
		t.Fatalf("NewWithDir failed: %v", err)
	}

	settings := st.Settings()
	if settings.ActiveAccount != "default" {
		t.Errorf("expected migrated active account 'default', got %q", settings.ActiveAccount)
	}
	if len(settings.Accounts) != 1 {
		t.Fatalf("expected 1 migrated account, got %d", len(settings.Accounts))
	}

	acc := settings.Accounts[0]
	if acc.ID != "default" || acc.Label != "default" {
		t.Errorf("unexpected account ID/Label: %+v", acc)
	}
	if acc.CookieFile != cookieFile {
		t.Errorf("expected cookie file %s, got %s (files must not move!)", cookieFile, acc.CookieFile)
	}

	// Verify the original cookie file is untouched and exists
	if _, err := os.Stat(cookieFile); err != nil {
		t.Errorf("cookies.txt was moved or deleted: %v", err)
	}
}

func TestStoreAccountManagement(t *testing.T) {
	dir := t.TempDir()
	st, err := NewWithDir(dir)
	if err != nil {
		t.Fatalf("NewWithDir failed: %v", err)
	}

	// Verify initial default account
	active := st.ActiveAccount()
	if active.ID != "default" {
		t.Fatalf("expected active 'default', got %q", active.ID)
	}

	// Add second account
	workAcc := AccountConfig{
		ID:          "work",
		Label:       "Work Account",
		BrowserSpec: "chrome+gnomekeyring:Profile 1",
		UserName:    "work@example.com",
	}
	if err := st.AddAccount(workAcc); err != nil {
		t.Fatalf("AddAccount failed: %v", err)
	}

	if len(st.Settings().Accounts) != 2 {
		t.Fatalf("expected 2 accounts, got %d", len(st.Settings().Accounts))
	}

	// Switch to work account
	switched, err := st.SwitchAccount("work")
	if err != nil {
		t.Fatalf("SwitchAccount failed: %v", err)
	}
	if switched.ID != "work" || st.ActiveAccount().ID != "work" {
		t.Errorf("expected active account 'work', got %q", st.ActiveAccount().ID)
	}
	if st.Settings().BrowserSpec != "chrome+gnomekeyring:Profile 1" {
		t.Errorf("expected browser spec to update to work account's spec, got %s", st.Settings().BrowserSpec)
	}

	// Remove work account
	if err := st.RemoveAccount("work"); err != nil {
		t.Fatalf("RemoveAccount failed: %v", err)
	}
	if len(st.Settings().Accounts) != 1 {
		t.Fatalf("expected 1 account after removal, got %d", len(st.Settings().Accounts))
	}
	if st.ActiveAccount().ID != "default" {
		t.Errorf("expected active account to revert to default, got %q", st.ActiveAccount().ID)
	}
}
