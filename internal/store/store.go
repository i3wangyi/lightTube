package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"lighttube/internal/platform"
	"lighttube/internal/yt"
)

const maxHistory = 500

// DefaultDataDir returns the standard data directory under platform DataDir.
func DefaultDataDir() string {
	return platform.DataDir()
}

// Store provides thread-safe persistence for subscriptions, watch later, and playback history.
type Store struct {
	dir string
	mu  sync.RWMutex

	subs       []yt.Subscription
	watchLater []yt.Video
	history    []yt.Video
	settings   Settings
}

// New creates a Store in the default XDG data directory.
func New() (*Store, error) {
	return NewWithDir(DefaultDataDir())
}

// NewWithDir creates a Store in a specified directory.
func NewWithDir(dir string) (*Store, error) {
	s, err := NewDeferred(dir)
	if err != nil {
		return nil, err
	}

	if err := s.loadAll(); err != nil {
		return nil, err
	}

	return s, nil
}

// NewDeferred creates a Store in a specified directory without reading files immediately.
func NewDeferred(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating store directory: %w", err)
	}

	return &Store{
		dir:        dir,
		subs:       []yt.Subscription{},
		watchLater: []yt.Video{},
		history:    []yt.Video{},
		settings:   DefaultSettingsWithDir(dir),
	}, nil
}

// DataDir returns the data directory used by the store.
func (s *Store) DataDir() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.dir
}

// Dir returns the data directory used by the store (alias for DataDir).
func (s *Store) Dir() string {
	return s.DataDir()
}

// LoadAll reads subscriptions, watch later, and playback history from disk.
func (s *Store) LoadAll() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadAll()
}

func (s *Store) loadAll() error {
	subsFile := filepath.Join(s.dir, "subscriptions.json")
	if err := readJSONFile(subsFile, &s.subs); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("loading subscriptions: %w", err)
	}

	wlFile := filepath.Join(s.dir, "watchlater.json")
	if err := readJSONFile(wlFile, &s.watchLater); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("loading watch later: %w", err)
	}

	histFile := filepath.Join(s.dir, "history.json")
	if err := readJSONFile(histFile, &s.history); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("loading history: %w", err)
	}
	if len(s.history) > maxHistory {
		s.history = s.history[:maxHistory]
	}

	settingsFile := filepath.Join(s.dir, "settings.json")
	var loaded Settings
	if err := readJSONFile(settingsFile, &loaded); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("loading settings: %w", err)
	}
	s.settings = loaded.NormalizeWithDir(s.dir)

	return nil
}

// Settings returns the active persistent settings.
func (s *Store) Settings() Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.settings
}

// ActiveAccount returns the configuration of the currently active account.
func (s *Store) ActiveAccount() AccountConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.settings.GetActiveAccount()
}

// SaveSettings persists settings to settings.json atomically.
func (s *Store) SaveSettings(cfg Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cfg = cfg.NormalizeWithDir(s.dir)
	if err := atomicWriteJSON(filepath.Join(s.dir, "settings.json"), cfg); err != nil {
		return err
	}
	s.settings = cfg
	return nil
}

// SetAmbient updates the persistent ambient glow setting.
func (s *Store) SetAmbient(enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.settings.Ambient = &enabled
	return s.saveSettingsLocked()
}

func (s *Store) saveSettingsLocked() error {
	cfg := s.settings.NormalizeWithDir(s.dir)
	if err := atomicWriteJSON(filepath.Join(s.dir, "settings.json"), cfg); err != nil {
		return err
	}
	s.settings = cfg
	return nil
}

// AddAccount adds or updates an account in settings.json.
func (s *Store) AddAccount(acc AccountConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if acc.ID == "" {
		acc.ID = fmt.Sprintf("acc_%d", time.Now().Unix())
	}
	if acc.Label == "" {
		acc.Label = acc.ID
	}
	if acc.CookieFile == "" {
		if acc.ID == "default" {
			acc.CookieFile = filepath.Join(s.dir, "cookies.txt")
		} else {
			acc.CookieFile = filepath.Join(s.dir, "accounts", acc.ID, "cookies.txt")
		}
	}
	if acc.CacheDir == "" {
		if acc.ID == "default" {
			acc.CacheDir = filepath.Join(platform.CacheDir(), "account")
		} else {
			acc.CacheDir = filepath.Join(platform.CacheDir(), "accounts", acc.ID)
		}
	}

	found := false
	for i, existing := range s.settings.Accounts {
		if existing.ID == acc.ID {
			s.settings.Accounts[i] = acc
			found = true
			break
		}
	}
	if !found {
		s.settings.Accounts = append(s.settings.Accounts, acc)
	}

	return s.saveSettingsLocked()
}

// SwitchAccount changes the active account and saves settings.
func (s *Store) SwitchAccount(id string) (AccountConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, acc := range s.settings.Accounts {
		if acc.ID == id {
			s.settings.ActiveAccount = id
			s.settings.BrowserSpec = acc.BrowserSpec
			if err := s.saveSettingsLocked(); err != nil {
				return AccountConfig{}, err
			}
			return acc, nil
		}
	}
	return AccountConfig{}, fmt.Errorf("account %q not found", id)
}

// RemoveAccount deletes an account and cleans up its data directories.
func (s *Store) RemoveAccount(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var target *AccountConfig
	newAccounts := make([]AccountConfig, 0, len(s.settings.Accounts))
	for _, acc := range s.settings.Accounts {
		if acc.ID == id {
			copyAcc := acc
			target = &copyAcc
		} else {
			newAccounts = append(newAccounts, acc)
		}
	}

	if target == nil {
		return fmt.Errorf("account %q not found", id)
	}

	if target.UserDataDir != "" {
		_ = os.RemoveAll(target.UserDataDir)
	}
	if target.ID != "default" {
		if target.CookieFile != "" {
			_ = os.Remove(target.CookieFile)
			_ = os.Remove(target.CookieFile + ".play")
		}
		if target.CacheDir != "" {
			_ = os.RemoveAll(target.CacheDir)
		}
	}

	s.settings.Accounts = newAccounts
	if s.settings.ActiveAccount == id {
		if len(newAccounts) > 0 {
			s.settings.ActiveAccount = newAccounts[0].ID
			s.settings.BrowserSpec = newAccounts[0].BrowserSpec
		} else {
			def := DefaultAccountConfig(s.dir, s.settings.BrowserSpec)
			s.settings.Accounts = []AccountConfig{def}
			s.settings.ActiveAccount = def.ID
		}
	}

	return s.saveSettingsLocked()
}

// Subscriptions returns a copy of all subscribed channels.
func (s *Store) Subscriptions() []yt.Subscription {
	s.mu.RLock()
	defer s.mu.RUnlock()
	res := make([]yt.Subscription, len(s.subs))
	copy(res, s.subs)
	return res
}

// HasSubscription reports whether a channel ID is in the subscription list.
func (s *Store) HasSubscription(channelID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, sub := range s.subs {
		if sub.ChannelID == channelID {
			return true
		}
	}
	return false
}

// AddSubscription adds a channel to subscriptions if not already present.
func (s *Store) AddSubscription(sub yt.Subscription) error {
	if sub.ChannelID == "" {
		return errors.New("empty channel ID")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for i, existing := range s.subs {
		if existing.ChannelID == sub.ChannelID {
			if sub.Title != "" {
				s.subs[i].Title = sub.Title
			}
			if sub.URL != "" {
				s.subs[i].URL = sub.URL
			}
			return s.saveSubsLocked()
		}
	}

	s.subs = append(s.subs, sub)
	return s.saveSubsLocked()
}

// AddSubscriptions adds multiple subscriptions, skipping duplicates.
func (s *Store) AddSubscriptions(subs []yt.Subscription) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	existingMap := make(map[string]int, len(s.subs))
	for i, sub := range s.subs {
		existingMap[sub.ChannelID] = i
	}

	for _, sub := range subs {
		if sub.ChannelID == "" {
			continue
		}
		if idx, ok := existingMap[sub.ChannelID]; ok {
			if sub.Title != "" {
				s.subs[idx].Title = sub.Title
			}
			if sub.URL != "" {
				s.subs[idx].URL = sub.URL
			}
		} else {
			existingMap[sub.ChannelID] = len(s.subs)
			s.subs = append(s.subs, sub)
		}
	}

	return s.saveSubsLocked()
}

// RemoveSubscription removes a channel from subscriptions.
func (s *Store) RemoveSubscription(channelID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	found := false
	filtered := make([]yt.Subscription, 0, len(s.subs))
	for _, sub := range s.subs {
		if sub.ChannelID == channelID {
			found = true
			continue
		}
		filtered = append(filtered, sub)
	}

	if !found {
		return nil
	}

	s.subs = filtered
	return s.saveSubsLocked()
}

func (s *Store) saveSubsLocked() error {
	return atomicWriteJSON(filepath.Join(s.dir, "subscriptions.json"), s.subs)
}

// WatchLater returns a copy of watch later videos (newest first).
func (s *Store) WatchLater() []yt.Video {
	s.mu.RLock()
	defer s.mu.RUnlock()
	res := make([]yt.Video, len(s.watchLater))
	copy(res, s.watchLater)
	return res
}

// IsInWatchLater reports whether a video ID is present in watch later.
func (s *Store) IsInWatchLater(id string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, v := range s.watchLater {
		if v.ID == id {
			return true
		}
	}
	return false
}

// AddWatchLater prepends a video to watch later.
func (s *Store) AddWatchLater(v yt.Video) error {
	if v.ID == "" {
		return errors.New("empty video ID")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Remove if already present so it moves to front.
	filtered := make([]yt.Video, 0, len(s.watchLater)+1)
	for _, item := range s.watchLater {
		if item.ID != v.ID {
			filtered = append(filtered, item)
		}
	}

	s.watchLater = append([]yt.Video{v}, filtered...)
	return s.saveWatchLaterLocked()
}

// RemoveWatchLater removes a video from watch later.
func (s *Store) RemoveWatchLater(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	filtered := make([]yt.Video, 0, len(s.watchLater))
	for _, item := range s.watchLater {
		if item.ID != id {
			filtered = append(filtered, item)
		}
	}

	s.watchLater = filtered
	return s.saveWatchLaterLocked()
}

// ToggleWatchLater adds the video if not in watch later, or removes it if already present.
// It returns true if the video was added, or false if removed.
func (s *Store) ToggleWatchLater(v yt.Video) (bool, error) {
	if v.ID == "" {
		return false, errors.New("empty video ID")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	idx := -1
	for i, item := range s.watchLater {
		if item.ID == v.ID {
			idx = i
			break
		}
	}

	if idx >= 0 {
		s.watchLater = append(s.watchLater[:idx], s.watchLater[idx+1:]...)
		return false, s.saveWatchLaterLocked()
	}

	s.watchLater = append([]yt.Video{v}, s.watchLater...)
	return true, s.saveWatchLaterLocked()
}

func (s *Store) saveWatchLaterLocked() error {
	return atomicWriteJSON(filepath.Join(s.dir, "watchlater.json"), s.watchLater)
}

// SavedCollections reads user-saved music collections for an account.
func (s *Store) SavedCollections(accountID string) []yt.Album {
	path := s.collectionsFile(accountID)
	s.mu.RLock()
	defer s.mu.RUnlock()
	var albums []yt.Album
	if readJSONFile(path, &albums) != nil {
		return nil
	}
	return albums
}

// SaveCollection prepends a music collection to the account's saved list.
func (s *Store) SaveCollection(accountID string, album yt.Album) error {
	if album.ID == "" && album.Title == "" {
		return errors.New("empty music collection")
	}
	path := s.collectionsFile(accountID)
	s.mu.Lock()
	defer s.mu.Unlock()
	var albums []yt.Album
	_ = readJSONFile(path, &albums)
	filtered := make([]yt.Album, 0, len(albums)+1)
	for _, item := range albums {
		if album.ID != "" && item.ID == album.ID || album.ID == "" && item.Title == album.Title {
			continue
		}
		filtered = append(filtered, item)
	}
	return atomicWriteJSON(path, append([]yt.Album{album}, filtered...))
}

func (s *Store) collectionsFile(accountID string) string {
	s.mu.RLock()
	dir := s.dir
	s.mu.RUnlock()
	if accountID == "" || accountID == "default" {
		return filepath.Join(dir, "collections.json")
	}
	return filepath.Join(dir, "accounts", filepath.Base(accountID), "collections.json")
}

// History returns playback history (newest first, max 500).
func (s *Store) History() []yt.Video {
	s.mu.RLock()
	defer s.mu.RUnlock()
	res := make([]yt.Video, len(s.history))
	copy(res, s.history)
	return res
}

// AddHistory prepends a video to playback history, moving it to front if already present,
// and capping the total history list to 500 items.
func (s *Store) AddHistory(v yt.Video) error {
	if v.ID == "" {
		return errors.New("empty video ID")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	filtered := make([]yt.Video, 0, len(s.history)+1)
	filtered = append(filtered, v)
	for _, item := range s.history {
		if item.ID != v.ID {
			filtered = append(filtered, item)
			if len(filtered) >= maxHistory {
				break
			}
		}
	}

	s.history = filtered
	return s.saveHistoryLocked()
}

// ClearHistory removes all items from playback history.
func (s *Store) ClearHistory() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.history = []yt.Video{}
	return s.saveHistoryLocked()
}

func (s *Store) saveHistoryLocked() error {
	return atomicWriteJSON(filepath.Join(s.dir, "history.json"), s.history)
}

func readJSONFile(path string, target interface{}) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

func atomicWriteJSON(dest string, v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding JSON: %w", err)
	}

	dir := filepath.Dir(dest)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, filepath.Base(dest)+".tmp.*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		_ = os.Remove(tmpName)
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	return os.Rename(tmpName, dest)
}
