package version

import (
	"runtime"
	"strings"
	"testing"
)

func TestVersionDefaults(t *testing.T) {
	// Package-level vars have their zero/default values when not overridden via ldflags.
	// We just verify they are accessible strings (may be "dev" or "unknown" by default).
	if Version == "" {
		t.Error("Version should not be empty")
	}
	if GitCommit == "" {
		t.Error("GitCommit should not be empty")
	}
	if BuildDate == "" {
		t.Error("BuildDate should not be empty")
	}
}

func TestString(t *testing.T) {
	got := String()

	if got == "" {
		t.Fatal("String() returned empty string")
	}
	if !strings.HasPrefix(got, "astra ") {
		t.Errorf("String() should start with 'astra ', got %q", got)
	}
	if !strings.Contains(got, Version) {
		t.Errorf("String() should contain Version %q, got %q", Version, got)
	}
	if !strings.Contains(got, GitCommit) {
		t.Errorf("String() should contain GitCommit %q, got %q", GitCommit, got)
	}
	if !strings.Contains(got, BuildDate) {
		t.Errorf("String() should contain BuildDate %q, got %q", BuildDate, got)
	}
	if !strings.Contains(got, runtime.Version()) {
		t.Errorf("String() should contain Go version %q, got %q", runtime.Version(), got)
	}
}

func TestStringFormat(t *testing.T) {
	// Save and restore original values.
	origVersion := Version
	origCommit := GitCommit
	origDate := BuildDate
	defer func() {
		Version = origVersion
		GitCommit = origCommit
		BuildDate = origDate
	}()

	Version = "1.2.3"
	GitCommit = "abc1234"
	BuildDate = "2024-01-15"

	got := String()

	expected := "astra 1.2.3 (commit=abc1234 date=2024-01-15 go=" + runtime.Version() + ")"
	if got != expected {
		t.Errorf("String() = %q, want %q", got, expected)
	}
}
