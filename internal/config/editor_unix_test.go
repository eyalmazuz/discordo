//go:build unix

package config

import (
	"testing"
)

func TestCreateEditorCommand(t *testing.T) {
	tests := []struct {
		name     string
		editor   string
		path     string
		wantNil  bool
		wantArgs []string
	}{
		{
			name:     "simple editor",
			editor:   "nvim",
			path:     "/tmp/file.md",
			wantArgs: []string{"nvim", "/tmp/file.md"},
		},
		{
			name:     "editor with flags",
			editor:   "code --wait",
			path:     "/tmp/file.md",
			wantArgs: []string{"code", "--wait", "/tmp/file.md"},
		},
		{
			name:    "empty editor",
			editor:  "",
			path:    "/tmp/file.md",
			wantNil: true,
		},
		{
			name:     "injection attempt passed as literal args",
			editor:   "vim ; rm -rf /",
			path:     "/tmp/file.md",
			wantArgs: []string{"vim", ";", "rm", "-rf", "/", "/tmp/file.md"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Editor: tt.editor}
			cmd := cfg.CreateEditorCommand(tt.path)

			if tt.wantNil {
				if cmd != nil {
					t.Fatalf("expected nil cmd, got %v", cmd.Args)
				}
				return
			}

			if cmd == nil {
				t.Fatal("expected non-nil cmd")
			}

			if len(cmd.Args) != len(tt.wantArgs) {
				t.Fatalf("expected %d args, got %d: %v", len(tt.wantArgs), len(cmd.Args), cmd.Args)
			}
			for i, want := range tt.wantArgs {
				if cmd.Args[i] != want {
					t.Fatalf("arg %d = %q, want %q", i, cmd.Args[i], want)
				}
			}
		})
	}
}
