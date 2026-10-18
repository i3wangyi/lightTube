package main

import (
	"strings"
	"testing"
)

func TestRunVersion(t *testing.T) {
	if err := run([]string{"--version"}); err != nil {
		t.Fatalf("unexpected error for --version: %v", err)
	}
}

func TestRunHelp(t *testing.T) {
	if err := run([]string{"--help"}); err != nil {
		t.Fatalf("unexpected error for --help: %v", err)
	}
}

func TestRunNoArgs(t *testing.T) {
	err := run([]string{})
	if err == nil {
		t.Fatal("expected error for empty args, got nil")
	}
}

func TestRunPlayMissingURL(t *testing.T) {
	err := run([]string{"play"})
	if err == nil {
		t.Fatal("expected error when play is invoked without URL, got nil")
	}
	if !strings.Contains(err.Error(), "usage: lighttube play <url> [--audio]") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestRunUnknownCommand(t *testing.T) {
	err := run([]string{"foo"})
	if err == nil {
		t.Fatal("expected error for unknown command, got nil")
	}
	if !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("unexpected error message: %v", err)
	}
}
