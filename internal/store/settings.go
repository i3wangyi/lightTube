package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"lighttube/internal/platform"
	"lighttube/internal/player"
)

const (
	PlayerInplace  = "inplace"
	PlayerExternal = "external"
)

// AccountConfig represents an individual account configuration in settings.json.
type AccountConfig struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	BrowserSpec string `json:"browser_spec"`
	CookieFile  string `json:"cookie_file"`
	CacheDir    string `json:"cache_dir"`
	UserName    string `json:"user_name,omitempty"`
	UserDataDir string `json:"user_data_dir,omitempty"`
}

// Settings holds persistent user configuration.
type Settings struct {
	BrowserSpec   string          `json:"browser_spec"`
	CodecPolicy   string          `json:"codec_policy"`
	Player        string          `json:"player"`
	ActiveAccount string          `json:"active_account"`
	Accounts      []AccountConfig `json:"accounts,omitempty"`
}

// DefaultAccountConfig returns the default migrated account pointing to legacy paths.
func DefaultAccountConfig(dataDir, browserSpec string) AccountConfig {
	if dataDir == "" {
		dataDir = platform.DataDir()
	}
	if browserSpec == "" {
		browserSpec = platform.DefaultBrowserSpec()
	}
	return AccountConfig{
		ID:          "default",
		Label:       "default",
		BrowserSpec: browserSpec,
		CookieFile:  filepath.Join(dataDir, "cookies.txt"),
		CacheDir:    filepath.Join(platform.CacheDir(), "account"),
	}
}

// DefaultSettings returns configuration defaults.
func DefaultSettings() Settings {
	return DefaultSettingsWithDir(platform.DataDir())
}

// DefaultSettingsWithDir returns configuration defaults using the specified data directory.
func DefaultSettingsWithDir(dataDir string) Settings {
	defBrowser := platform.DefaultBrowserSpec()
	defAcc := DefaultAccountConfig(dataDir, defBrowser)
	return Settings{
		BrowserSpec:   defBrowser,
		CodecPolicy:   string(player.CodecPolicyAuto),
		Player:        PlayerInplace,
		ActiveAccount: "default",
		Accounts:      []AccountConfig{defAcc},
	}
}

// Normalize ensures missing or unknown configuration values fall back to defaults.
func (s Settings) Normalize() Settings {
	return s.NormalizeWithDir(platform.DataDir())
}

// NormalizeWithDir ensures missing configuration values fall back to defaults with the given data directory.
func (s Settings) NormalizeWithDir(dataDir string) Settings {
	def := DefaultSettingsWithDir(dataDir)
	if s.BrowserSpec == "" {
		s.BrowserSpec = def.BrowserSpec
	}
	switch s.CodecPolicy {
	case string(player.CodecPolicyAuto), string(player.CodecPolicyOldGPU), string(player.CodecPolicyAny):
		// valid
	default:
		s.CodecPolicy = def.CodecPolicy
	}
	switch s.Player {
	case PlayerInplace, PlayerExternal:
		// valid
	default:
		s.Player = def.Player
	}

	// Migrate accounts: if accounts list is empty, initialize account "default"
	// with existing single account paths so nothing moves.
	if len(s.Accounts) == 0 {
		s.Accounts = []AccountConfig{DefaultAccountConfig(dataDir, s.BrowserSpec)}
		s.ActiveAccount = "default"
	} else {
		for i := range s.Accounts {
			acc := &s.Accounts[i]
			if acc.ID == "" {
				acc.ID = fmt.Sprintf("acc_%d", i)
			}
			if acc.Label == "" {
				acc.Label = acc.ID
			}
			if acc.BrowserSpec == "" {
				acc.BrowserSpec = s.BrowserSpec
			}
			if acc.CookieFile == "" {
				if acc.ID == "default" {
					acc.CookieFile = filepath.Join(dataDir, "cookies.txt")
				} else {
					acc.CookieFile = filepath.Join(dataDir, "accounts", acc.ID, "cookies.txt")
				}
			}
			if acc.CacheDir == "" {
				if acc.ID == "default" {
					acc.CacheDir = filepath.Join(platform.CacheDir(), "account")
				} else {
					acc.CacheDir = filepath.Join(platform.CacheDir(), "accounts", acc.ID)
				}
			}
		}

		// Ensure ActiveAccount is valid.
		activeFound := false
		for _, acc := range s.Accounts {
			if acc.ID == s.ActiveAccount {
				activeFound = true
				break
			}
		}
		if !activeFound {
			s.ActiveAccount = s.Accounts[0].ID
		}
	}

	return s
}

// GetActiveAccount returns the active AccountConfig.
func (s Settings) GetActiveAccount() AccountConfig {
	for _, acc := range s.Accounts {
		if acc.ID == s.ActiveAccount {
			return acc
		}
	}
	if len(s.Accounts) > 0 {
		return s.Accounts[0]
	}
	return DefaultAccountConfig(platform.DataDir(), s.BrowserSpec)
}

// LoadSettings reads settings from the given directory, falling back to defaults if missing or corrupted.
func LoadSettings(dir string) (Settings, error) {
	path := filepath.Join(dir, "settings.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return DefaultSettingsWithDir(dir), nil
		}
		return DefaultSettingsWithDir(dir), err
	}
	var s Settings
	if err := json.Unmarshal(data, &s); err != nil {
		return DefaultSettingsWithDir(dir), nil
	}
	return s.NormalizeWithDir(dir), nil
}

// SaveSettings writes settings atomically to the given directory.
func SaveSettings(dir string, s Settings) error {
	s = s.NormalizeWithDir(dir)
	return atomicWriteJSON(filepath.Join(dir, "settings.json"), s)
}
