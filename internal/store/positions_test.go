package store

import (
	"path/filepath"
	"testing"
)

func TestPositionMethods(t *testing.T) {
	// 0 duration
	p0 := Position{Seconds: 10, Duration: 0}
	if p0.Percent() != 0 {
		t.Errorf("expected 0%% for 0 duration, got %v", p0.Percent())
	}
	if p0.IsPlayed() {
		t.Errorf("expected false for IsPlayed with 0 duration")
	}

	// 50% watched
	p1 := Position{Seconds: 50, Duration: 100}
	if p1.Percent() != 50 {
		t.Errorf("expected 50%%, got %v", p1.Percent())
	}
	if p1.IsPlayed() {
		t.Errorf("expected false for IsPlayed at 50%%")
	}
	if !p1.IsUnfinished() {
		t.Errorf("expected true for IsUnfinished at 50%%")
	}

	// 95% watched (boundary for played)
	p2 := Position{Seconds: 95, Duration: 100}
	if !p2.IsPlayed() {
		t.Errorf("expected true for IsPlayed at 95%%")
	}
	if p2.IsUnfinished() {
		t.Errorf("expected false for IsUnfinished at 95%%")
	}

	// 98% watched
	p3 := Position{Seconds: 98, Duration: 100}
	if !p3.IsPlayed() {
		t.Errorf("expected true for IsPlayed at 98%%")
	}
	if p3.IsUnfinished() {
		t.Errorf("expected false for IsUnfinished at 98%%")
	}

	// 0 seconds watched
	p4 := Position{Seconds: 0, Duration: 100}
	if p4.IsUnfinished() {
		t.Errorf("expected false for IsUnfinished at 0s")
	}
	if p4.IsPlayed() {
		t.Errorf("expected false for IsPlayed at 0s")
	}
}

func TestPerAccountPositionsSeparation(t *testing.T) {
	dir := t.TempDir()
	st, err := NewDeferred(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Default account position
	if err := st.SavePosition("default", "vid1", 30, 100); err != nil {
		t.Fatalf("SavePosition default: %v", err)
	}

	// Another account position
	if err := st.SavePosition("user2", "vid1", 98, 100); err != nil {
		t.Fatalf("SavePosition user2: %v", err)
	}
	if err := st.SavePosition("user2", "vid2", 15, 60); err != nil {
		t.Fatalf("SavePosition user2 vid2: %v", err)
	}

	// Check isolation
	posDefault, ok := st.Position("default", "vid1")
	if !ok || posDefault.Seconds != 30 {
		t.Fatalf("default vid1 want 30s, got %v, ok=%v", posDefault, ok)
	}
	if posDefault.IsPlayed() {
		t.Fatalf("default vid1 should not be played")
	}

	posUser2, ok := st.Position("user2", "vid1")
	if !ok || posUser2.Seconds != 98 {
		t.Fatalf("user2 vid1 want 98s, got %v, ok=%v", posUser2, ok)
	}
	if !posUser2.IsPlayed() {
		t.Fatalf("user2 vid1 should be played")
	}

	// vid2 should not exist in default
	if _, ok := st.Position("default", "vid2"); ok {
		t.Fatalf("vid2 should not exist in default account")
	}

	// Verify file locations
	if st.PositionsFile("default") != filepath.Join(dir, "positions.json") {
		t.Errorf("unexpected default positions file: %s", st.PositionsFile("default"))
	}
	if st.PositionsFile("user2") != filepath.Join(dir, "accounts", "user2", "positions.json") {
		t.Errorf("unexpected user2 positions file: %s", st.PositionsFile("user2"))
	}
}

func TestAmbientSettingPersistence(t *testing.T) {
	dir := t.TempDir()
	st, err := NewDeferred(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Default should be true
	if !st.Settings().AmbientEnabled() {
		t.Fatalf("expected AmbientEnabled to be true by default")
	}

	// Toggle off
	if err := st.SetAmbient(false); err != nil {
		t.Fatalf("SetAmbient false: %v", err)
	}
	if st.Settings().AmbientEnabled() {
		t.Fatalf("expected AmbientEnabled to be false after toggle")
	}

	// Reload from disk
	st2, err := NewWithDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if st2.Settings().AmbientEnabled() {
		t.Fatalf("expected reloaded AmbientEnabled to be false")
	}

	// Toggle back on
	if err := st2.SetAmbient(true); err != nil {
		t.Fatalf("SetAmbient true: %v", err)
	}
	if !st2.Settings().AmbientEnabled() {
		t.Fatalf("expected AmbientEnabled to be true after toggle back on")
	}
}

func TestUnfinishedPositions(t *testing.T) {
	dir := t.TempDir()
	st, err := NewDeferred(dir)
	if err != nil {
		t.Fatal(err)
	}

	// 1 unfinished
	_ = st.SavePosition("acc1", "unfin1", 50, 100)
	// 1 completed (>=95%)
	_ = st.SavePosition("acc1", "done1", 96, 100)
	// 1 zero progress
	_ = st.SavePosition("acc1", "zero1", 0, 100)

	unfin := st.UnfinishedPositions("acc1")
	if len(unfin) != 1 {
		t.Fatalf("expected 1 unfinished position, got %d", len(unfin))
	}
	if unfin[0].VideoID != "unfin1" {
		t.Fatalf("expected unfin1, got %s", unfin[0].VideoID)
	}
}

func TestMediaPositionRoundTripAndLegacy(t *testing.T) {
	dir := t.TempDir()
	st, err := NewWithDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := Position{Seconds: 42, Duration: 600, Space: "Podcasts", MediaKind: "episode", Title: "Fixture episode", Show: "Fixture show"}
	if err := st.SaveMediaPosition("account", "episode", want); err != nil {
		t.Fatal(err)
	}
	if err := st.SavePosition("account", "legacy", 12, 120); err != nil {
		t.Fatal(err)
	}
	reopened, err := NewWithDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := reopened.Position("account", "episode")
	if !ok || got.Seconds != want.Seconds || got.Duration != want.Duration || got.Space != want.Space || got.MediaKind != want.MediaKind || got.Title != want.Title || got.Show != want.Show || got.Updated.IsZero() {
		t.Fatalf("round trip: %+v", got)
	}
	legacy, ok := reopened.Position("account", "legacy")
	if !ok || legacy.Seconds != 12 || legacy.Space != "" || legacy.MediaKind != "" {
		t.Fatalf("legacy record: %+v", legacy)
	}
	if err := reopened.SavePosition("account", "episode", 50, 600); err != nil {
		t.Fatal(err)
	}
	updated, _ := reopened.Position("account", "episode")
	if updated.Seconds != 50 || updated.Space != want.Space || updated.MediaKind != want.MediaKind || updated.Title != want.Title || updated.Show != want.Show {
		t.Fatalf("progress update lost metadata: %+v", updated)
	}
}
