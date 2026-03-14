package consts

import (
	"errors"
	"os"
	"testing"
)

func TestCacheDir(t *testing.T) {
	if CacheDir() == "" {
		t.Errorf("expected non-empty cache dir")
	}
}

func TestInitCacheDir_ErrorPaths(t *testing.T) {
	oldUserCacheDir := osUserCacheDir
	oldTempDir := osTempDir
	oldMkdirAll := osMkdirAllCacheDir
	defer func() {
		osUserCacheDir = oldUserCacheDir
		osTempDir = oldTempDir
		osMkdirAllCacheDir = oldMkdirAll
	}()

	t.Run("UserCacheDir_Error", func(t *testing.T) {
		osUserCacheDir = func() (string, error) { return "", errors.New("fail") }
		osTempDir = func() string { return "/tmp/discordo-test" }
		osMkdirAllCacheDir = func(path string, perm os.FileMode) error { return nil }
		
		dir := initCacheDir()
		if dir != "/tmp/discordo-test/discordo" {
			t.Errorf("expected fallback to temp dir, got %q", dir)
		}
	})

	t.Run("MkdirAll_Error", func(t *testing.T) {
		osUserCacheDir = func() (string, error) { return "/home/user/.cache", nil }
		osMkdirAllCacheDir = func(path string, perm os.FileMode) error { return errors.New("mkdir fail") }
		
		dir := initCacheDir()
		if dir != "/home/user/.cache/discordo" {
			t.Errorf("expected dir path even if mkdir fails, got %q", dir)
		}
	})
}
