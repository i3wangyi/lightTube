package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Position records playback progress for a video.
type Position struct {
	Seconds   float64   `json:"seconds"`
	Duration  float64   `json:"duration"`
	Updated   time.Time `json:"updated"`
	Space     string    `json:"space,omitempty"`
	MediaKind string    `json:"media_kind,omitempty"`
	Title     string    `json:"title,omitempty"`
	Show      string    `json:"show,omitempty"`
}

// Percent returns playback progress as a percentage between 0 and 100.
func (p Position) Percent() float64 {
	if p.Duration <= 0 {
		return 0
	}
	pct := (p.Seconds / p.Duration) * 100
	if pct < 0 {
		return 0
	}
	if pct > 100 {
		return 100
	}
	return pct
}

// IsPlayed reports whether the video has been watched to at least 95%.
func (p Position) IsPlayed() bool {
	return p.Duration > 0 && (p.Seconds/p.Duration) >= 0.95
}

// IsUnfinished reports whether the video has a stored position greater than zero and is not completed.
func (p Position) IsUnfinished() bool {
	return p.Seconds > 0 && !p.IsPlayed()
}

// PositionsFile returns the path to the positions.json file for a given account.
func (s *Store) PositionsFile(accountID string) string {
	s.mu.RLock()
	dir := s.dir
	s.mu.RUnlock()

	if accountID == "" || accountID == "default" {
		return filepath.Join(dir, "positions.json")
	}
	return filepath.Join(dir, "accounts", accountID, "positions.json")
}

// Positions returns a copy of all stored positions for the specified account.
func (s *Store) Positions(accountID string) (map[string]Position, error) {
	filePath := s.PositionsFile(accountID)

	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := os.ReadFile(filePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return make(map[string]Position), nil
		}
		return nil, fmt.Errorf("reading positions: %w", err)
	}

	var res map[string]Position
	if err := json.Unmarshal(data, &res); err != nil {
		return make(map[string]Position), nil
	}
	if res == nil {
		res = make(map[string]Position)
	}
	return res, nil
}

// Position retrieves the position for a given video in the specified account.
func (s *Store) Position(accountID, videoID string) (Position, bool) {
	positions, err := s.Positions(accountID)
	if err != nil {
		return Position{}, false
	}
	pos, ok := positions[videoID]
	return pos, ok
}

// SavePosition saves or updates a video's position for the specified account.
func (s *Store) SavePosition(accountID, videoID string, seconds, duration float64) error {
	return s.SaveMediaPosition(accountID, videoID, Position{Seconds: seconds, Duration: duration})
}

// SaveMediaPosition stores progress with the source and display metadata.
func (s *Store) SaveMediaPosition(accountID, videoID string, position Position) error {
	if videoID == "" {
		return errors.New("empty video ID")
	}

	filePath := s.PositionsFile(accountID)

	s.mu.Lock()
	defer s.mu.Unlock()

	positions := make(map[string]Position)
	data, err := os.ReadFile(filePath)
	if err == nil {
		_ = json.Unmarshal(data, &positions)
	}
	if positions == nil {
		positions = make(map[string]Position)
	}

	if position.Space == "" && position.MediaKind == "" {
		previous := positions[videoID]
		position.Space, position.MediaKind = previous.Space, previous.MediaKind
		position.Title, position.Show = previous.Title, previous.Show
	}
	position.Updated = time.Now()
	positions[videoID] = position

	return atomicWriteJSON(filePath, positions)
}

// UnfinishedPosition bundles a video ID with its playback position.
type UnfinishedPosition struct {
	VideoID string
	Position
}

// UnfinishedPositions returns all positions with 0 < percent < 95%, sorted by Updated descending.
func (s *Store) UnfinishedPositions(accountID string) []UnfinishedPosition {
	positions, err := s.Positions(accountID)
	if err != nil {
		return nil
	}
	var res []UnfinishedPosition
	for id, p := range positions {
		if p.IsUnfinished() {
			res = append(res, UnfinishedPosition{
				VideoID:  id,
				Position: p,
			})
		}
	}
	sort.Slice(res, func(i, j int) bool {
		return res[i].Updated.After(res[j].Updated)
	})
	return res
}
