package logger

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/ayn2op/discordo/internal/consts"
)

const fileName = "logs.txt"

var (
	mkdirAll = os.MkdirAll
	openFile = os.OpenFile
)

func DefaultPath() string {
	return filepath.Join(consts.CacheDir(), fileName)
}

// Load opens the log file and configures default logger.
func Load(path string, level slog.Level) error {
	if err := mkdirAll(filepath.Dir(path), os.ModePerm); err != nil {
		return err
	}

	file, err := openFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.ModePerm)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}

	opts := &slog.HandlerOptions{Level: level}
	handler := slog.NewTextHandler(file, opts)
	slog.SetDefault(slog.New(handler))
	return nil
}
