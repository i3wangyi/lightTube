package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"lighttube/internal/platform"
	"lighttube/internal/player"
)

const (
	PlayerInplace  = "inplace"
	PlayerExternal = "external"
)

// Settings holds persistent user configuration.
type Settings struct {
	BrowserSpec string `json:"browser_spec"`
	CodecPolicy string `json:"codec_policy"`
	Player      string `json:"player"`
}

// DefaultSettings returns configuration defaults.
func DefaultSettings() Settings {
	return Settings{
		BrowserSpec: platform.DefaultBrowserSpec(),
		CodecPolicy: string(player.CodecPolicyAuto),
		Player:      PlayerInplace,
	}
}

// Normalize ensures missing or unknown configuration values fall back to defaults.
func (s Settings) Normalize() Settings {
	def := DefaultSettings()
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
	return s
}

// LoadSettings reads settings from the given directory, falling back to defaults if missing or corrupted.
func LoadSettings(dir string) (Settings, error) {
	path := filepath.Join(dir, "settings.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return DefaultSettings(), nil
		}
		return DefaultSettings(), err
	}
	var s Settings
	if err := json.Unmarshal(data, &s); err != nil {
		return DefaultSettings(), nil
	}
	return s.Normalize(), nil
}

// SaveSettings writes settings atomically to the given directory.
func SaveSettings(dir string, s Settings) error {
	s = s.Normalize()
	return atomicWriteJSON(filepath.Join(dir, "settings.json"), s)
}
