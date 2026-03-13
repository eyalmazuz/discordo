package logger

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestLogger(t *testing.T) {
	t.Run("DefaultPath", func(t *testing.T) {
		path := DefaultPath()
		if path == "" {
			t.Errorf("Expected non-empty default path")
		}
	})

	t.Run("Load", func(t *testing.T) {
		tmpDir := t.TempDir()
		logPath := filepath.Join(tmpDir, "test.log")
		
		err := Load(logPath, slog.LevelDebug)
		if err != nil {
			t.Fatalf("Load failed: %v", err)
		}
		
		slog.Info("test message")
		
		if _, err := os.Stat(logPath); os.IsNotExist(err) {
			t.Errorf("Log file not created")
		}
	})

	t.Run("Load_MkdirError", func(t *testing.T) {
		// Try to load in a read-only or invalid path
		err := Load("/root/invalid/log.txt", slog.LevelInfo)
		if err == nil {
			t.Errorf("Expected error for invalid path")
		}
	})
}
