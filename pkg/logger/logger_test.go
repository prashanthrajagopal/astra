package logger

import (
	"context"
	"log/slog"
	"testing"
)

func TestNewReturnsLogger(t *testing.T) {
	tests := []struct {
		level    string
		wantLevel slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"warn", slog.LevelWarn},
		{"error", slog.LevelError},
		{"info", slog.LevelInfo},
		{"", slog.LevelInfo},        // default
		{"unknown", slog.LevelInfo}, // unknown falls back to info
		{"INFO", slog.LevelInfo},    // case-sensitive: uppercase not matched, falls to default
		{"DEBUG", slog.LevelInfo},   // uppercase not matched
	}

	for _, tt := range tests {
		t.Run("level="+tt.level, func(t *testing.T) {
			l := New(tt.level)
			if l == nil {
				t.Fatal("New() returned nil logger")
			}
			if !l.Enabled(context.TODO(), tt.wantLevel) {
				t.Errorf("logger with level %q should have %v enabled", tt.level, tt.wantLevel)
			}
		})
	}
}

func TestNewDebugLevelExcludesAbove(t *testing.T) {
	l := New("debug")
	// debug level should log everything
	if !l.Enabled(context.TODO(), slog.LevelDebug) {
		t.Error("debug logger should have debug level enabled")
	}
	if !l.Enabled(context.TODO(), slog.LevelInfo) {
		t.Error("debug logger should have info level enabled")
	}
	if !l.Enabled(context.TODO(), slog.LevelWarn) {
		t.Error("debug logger should have warn level enabled")
	}
	if !l.Enabled(context.TODO(), slog.LevelError) {
		t.Error("debug logger should have error level enabled")
	}
}

func TestNewErrorLevelExcludesBelow(t *testing.T) {
	l := New("error")
	if l.Enabled(context.TODO(), slog.LevelDebug) {
		t.Error("error logger should NOT have debug level enabled")
	}
	if l.Enabled(context.TODO(), slog.LevelInfo) {
		t.Error("error logger should NOT have info level enabled")
	}
	if l.Enabled(context.TODO(), slog.LevelWarn) {
		t.Error("error logger should NOT have warn level enabled")
	}
	if !l.Enabled(context.TODO(), slog.LevelError) {
		t.Error("error logger should have error level enabled")
	}
}

func TestNewWarnLevelBoundary(t *testing.T) {
	l := New("warn")
	if l.Enabled(context.TODO(), slog.LevelDebug) {
		t.Error("warn logger should NOT have debug level enabled")
	}
	if l.Enabled(context.TODO(), slog.LevelInfo) {
		t.Error("warn logger should NOT have info level enabled")
	}
	if !l.Enabled(context.TODO(), slog.LevelWarn) {
		t.Error("warn logger should have warn level enabled")
	}
	if !l.Enabled(context.TODO(), slog.LevelError) {
		t.Error("warn logger should have error level enabled")
	}
}
