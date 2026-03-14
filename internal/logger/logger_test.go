package logger

import (
	"errors"
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
		oldMkdirAll := mkdirAll
		t.Cleanup(func() { mkdirAll = oldMkdirAll })
		mkdirAll = func(string, os.FileMode) error { return errors.New("mkdir failed") }

		err := Load(filepath.Join(t.TempDir(), "test.log"), slog.LevelInfo)
		if err == nil {
			t.Errorf("Expected error for mkdir failure")
		}
	})

	t.Run("Load_OpenFileError", func(t *testing.T) {
		oldOpenFile := openFile
		t.Cleanup(func() { openFile = oldOpenFile })
		openFile = func(string, int, os.FileMode) (*os.File, error) {
			return nil, errors.New("open failed")
		}

		err := Load(filepath.Join(t.TempDir(), "test.log"), slog.LevelInfo)
		if err == nil {
			t.Fatal("expected open file error")
		}
	})
}
