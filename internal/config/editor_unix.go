//go:build unix

package config

import (
	"os/exec"
	"strings"
)

func (cfg *Config) CreateEditorCommand(path string) *exec.Cmd {
	parts := strings.Fields(cfg.Editor)
	if len(parts) == 0 {
		return nil
	}
	return exec.Command(parts[0], append(parts[1:], path)...)
}
