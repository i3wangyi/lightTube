package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"lighttube/internal/store"
	"lighttube/internal/yt"
)

type mockYTDLP struct {
	searchData []byte
	resolveID  string
}

func (m *mockYTDLP) Search(ctx context.Context, query string) ([]byte, error) {
	return m.searchData, nil
}

func (m *mockYTDLP) ResolveChannel(ctx context.Context, target string) (string, error) {
	if m.resolveID != "" {
		return m.resolveID, nil
	}
	return "UCmock12345678901234567", nil
}

func setupTestEnv(t *testing.T) (*appEnv, *bytes.Buffer, *bytes.Buffer) {
	searchFixture, err := os.ReadFile("../../testdata/search.json")
	if err != nil {
		t.Fatalf("failed reading search fixture: %v", err)
	}

	mock := &mockYTDLP{
		searchData: searchFixture,
		resolveID:  "UC8butISFwT-Wl7EV0hUK0BQ",
	}

	tempDir := t.TempDir()
	st, err := store.NewWithDir(filepath.Join(tempDir, "data"))
	if err != nil {
		t.Fatalf("failed creating test store: %v", err)
	}

	cacheDir := filepath.Join(tempDir, "cache")
	// Pre-populate feed cache for the mock channel.
	feedFixture, err := os.ReadFile("../../testdata/feed.xml")
	if err == nil {
		_ = os.MkdirAll(cacheDir, 0o755)
		_ = os.WriteFile(filepath.Join(cacheDir, "UC8butISFwT-Wl7EV0hUK0BQ.xml"), feedFixture, 0o644)
	}

	ytClient := yt.NewClientWithOptions(mock, nil, cacheDir)

	var stdout, stderr bytes.Buffer
	env := &appEnv{
		ytClient: ytClient,
		store:    st,
		stdout:   &stdout,
		stderr:   &stderr,
		stdin:    strings.NewReader(""),
	}

	return env, &stdout, &stderr
}

func TestRunVersion(t *testing.T) {
	env, stdout, _ := setupTestEnv(t)
	if err := runWithEnv(env, []string{"--version"}); err != nil {
		t.Fatalf("unexpected error for --version: %v", err)
	}
	if !strings.Contains(stdout.String(), "lighttube") {
		t.Errorf("expected version output, got: %s", stdout.String())
	}
}

func TestRunHelp(t *testing.T) {
	env, stdout, _ := setupTestEnv(t)
	if err := runWithEnv(env, []string{"--help"}); err != nil {
		t.Fatalf("unexpected error for --help: %v", err)
	}
	if !strings.Contains(stdout.String(), "Usage:") {
		t.Errorf("expected help output, got: %s", stdout.String())
	}
}

func TestRunNoArgs(t *testing.T) {
	env, _, _ := setupTestEnv(t)
	err := runWithEnv(env, []string{})
	if err == nil {
		t.Fatal("expected error for empty args, got nil")
	}
}

func TestRunPlayMissingURL(t *testing.T) {
	env, _, _ := setupTestEnv(t)
	err := runWithEnv(env, []string{"play"})
	if err == nil {
		t.Fatal("expected error when play is invoked without URL, got nil")
	}
	if !strings.Contains(err.Error(), "usage: lighttube play <url> [--audio]") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestRunUnknownCommand(t *testing.T) {
	env, _, _ := setupTestEnv(t)
	err := runWithEnv(env, []string{"foo"})
	if err == nil {
		t.Fatal("expected error for unknown command, got nil")
	}
	if !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestRunSearch(t *testing.T) {
	env, stdout, _ := setupTestEnv(t)
	if err := runWithEnv(env, []string{"search", "golang"}); err != nil {
		t.Fatalf("runWithEnv search failed: %v", err)
	}

	out := stdout.String()
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 search result lines, got %d:\n%s", len(lines), out)
	}

	// Verify id, title, channel, duration format.
	parts := strings.Split(lines[0], "\t")
	if len(parts) != 4 {
		t.Fatalf("expected 4 tab-separated fields, got %d: %q", len(parts), lines[0])
	}
	if parts[0] != "yyUHQIec83I" {
		t.Errorf("expected first video ID yyUHQIec83I, got %q", parts[0])
	}
	if parts[3] != "3:24:59" {
		t.Errorf("expected duration 3:24:59, got %q", parts[3])
	}
}

func TestRunSubAddAndList(t *testing.T) {
	env, stdout, _ := setupTestEnv(t)

	// Add channel.
	if err := runWithEnv(env, []string{"sub", "add", "@freecodecamp"}); err != nil {
		t.Fatalf("sub add failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "added") {
		t.Errorf("expected added message, got: %s", stdout.String())
	}

	// List channels.
	stdout.Reset()
	if err := runWithEnv(env, []string{"sub", "list"}); err != nil {
		t.Fatalf("sub list failed: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "UC8butISFwT-Wl7EV0hUK0BQ") {
		t.Errorf("expected UC8butISFwT-Wl7EV0hUK0BQ in sub list, got: %s", out)
	}
}

func TestRunFeed(t *testing.T) {
	env, stdout, _ := setupTestEnv(t)

	// Initially empty subscriptions -> empty feed.
	if err := runWithEnv(env, []string{"feed"}); err != nil {
		t.Fatalf("feed on empty subs failed: %v", err)
	}
	if stdout.Len() != 0 {
		t.Errorf("expected empty feed output when no subscriptions, got: %s", stdout.String())
	}

	// Add subscription.
	_ = env.store.AddSubscription(yt.Subscription{
		ChannelID: "UC8butISFwT-Wl7EV0hUK0BQ",
		Title:     "freeCodeCamp.org",
	})

	stdout.Reset()
	if err := runWithEnv(env, []string{"feed"}); err != nil {
		t.Fatalf("feed with subscription failed: %v", err)
	}

	out := stdout.String()
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) == 0 || lines[0] == "" {
		t.Fatalf("expected feed lines, got empty output")
	}

	parts := strings.Split(lines[0], "\t")
	if len(parts) != 4 {
		t.Fatalf("expected 4 tab-separated fields in feed output, got %d: %q", len(parts), lines[0])
	}
}
