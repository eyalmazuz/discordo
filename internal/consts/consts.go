package consts

import (
	"log/slog"
	"os"
	"path/filepath"
)

const Name = "discordo"

var (
	osUserCacheDir     = os.UserCacheDir
	osTempDir          = os.TempDir
	osMkdirAllCacheDir = os.MkdirAll
)

var cacheDir string

func CacheDir() string {
	return cacheDir
}

func init() {
	initCacheDir()
}

func initCacheDir() string {
	userCacheDir, err := osUserCacheDir()
	if err != nil {
		userCacheDir = osTempDir()
		slog.Warn("failed to get user cache dir; falling back to temp dir", "err", err, "path", userCacheDir)
	}

	cacheDir = filepath.Join(userCacheDir, Name)
	if err := osMkdirAllCacheDir(cacheDir, os.ModePerm); err != nil {
		slog.Error("failed to create cache dir", "err", err, "path", cacheDir)
	}

	return cacheDir
}
