package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/ayn2op/discordo/internal/consts"
	tviewkeybind "github.com/ayn2op/tview/keybind"
	"github.com/gdamore/tcell/v3"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestDefaultPath(t *testing.T) {
	t.Run("user config dir fallback", func(t *testing.T) {
		t.Setenv("AppData", "")
		t.Setenv("HOME", "")
		t.Setenv("home", "")
		t.Setenv("XDG_CONFIG_HOME", "")

		// filepath.Join strips the leading dot.
		got := DefaultPath()
		want := filepath.Join(".", consts.Name, fileName)
		if got != want {
			t.Fatalf("got = %v, want = %v", got, want)
		}
	})
}

func TestLoad(t *testing.T) {
	t.Run("invalid default config returns error", func(t *testing.T) {
		orig := defaultCfg
		defaultCfg = []byte("invalid =")
		t.Cleanup(func() { defaultCfg = orig })
		if _, err := Load("does-not-matter.toml"); err == nil {
			t.Fatal(err)
		}
	})

	t.Run("invalid config returns error", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "bad.toml")
		if err := os.WriteFile(path, []byte("invalid ="), os.ModePerm); err != nil {
			t.Fatal(err)
		}

		if _, err := Load(path); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("valid config does not return error", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "good.toml")
		if err := os.WriteFile(path, []byte("mouse = false"), os.ModePerm); err != nil {
			t.Fatal(err)
		}

		cfg, err := Load(path)
		if err != nil {
			t.Fatal(err)
		}

		if cfg.Mouse != false {
			t.Fatalf("got = %v, want = false", cfg.Mouse)
		}
	})

	t.Run("open with bad path returns error (!= ErrNotExist)", func(t *testing.T) {
		if _, err := Load("bad\x00path"); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("missing file uses defaults", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "missing.toml")
		cfg, err := Load(path)
		if err != nil {
			t.Fatal(err)
		}

		var defCfg Config
		if err := toml.Unmarshal(defaultCfg, &defCfg); err != nil {
			t.Fatal(err)
		}
		applyDefaults(&defCfg)

		if diff := cmp.Diff(
			defCfg,
			*cfg,
			cmpopts.EquateComparable(tcell.Style{}),
			cmpopts.IgnoreUnexported(tviewkeybind.Keybind{}),
		); diff != "" {
			t.Fatalf("got = -, want = +, diff=%s", diff)
		}
	})
}

func TestApplyDefaultsDateSeparatorFallbacks(t *testing.T) {
	t.Run("default editor uses EDITOR env", func(t *testing.T) {
		t.Setenv("EDITOR", "nvim")
		cfg := Config{Editor: "default"}
		applyDefaults(&cfg)
		if cfg.Editor != "nvim" {
			t.Fatalf("editor = %q, want %q", cfg.Editor, "nvim")
		}
	})

	t.Run("empty date separator character falls back to default rune", func(t *testing.T) {
		cfg := Config{}
		applyDefaults(&cfg)
		if cfg.DateSeparator.Character != "─" {
			t.Fatalf("character = %q, want default separator", cfg.DateSeparator.Character)
		}
	})

	t.Run("invalid date separator rune falls back to default rune", func(t *testing.T) {
		cfg := Config{}
		cfg.DateSeparator.Character = string([]byte{0xff})
		applyDefaults(&cfg)
		if cfg.DateSeparator.Character != "─" {
			t.Fatalf("character = %q, want default separator", cfg.DateSeparator.Character)
		}
	})
}
