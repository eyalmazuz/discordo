package cmd

import (
	"errors"
	"log/slog"
	"os"
	"testing"

	"github.com/ayn2op/discordo/internal/config"
	rootui "github.com/ayn2op/discordo/internal/ui/root"
	"github.com/ayn2op/tview"
	"github.com/diamondburned/arikawa/v3/utils/ws"
	"github.com/gdamore/tcell/v3"
)

type testScreen struct {
	tcell.Screen
	initErr      error
	mouseEnabled bool
	pasteEnabled bool
	focusEnabled bool
}

func (s *testScreen) Init() error {
	return s.initErr
}

func (s *testScreen) EnableMouse(...tcell.MouseFlags) {
	s.mouseEnabled = true
}

func (s *testScreen) EnablePaste() {
	s.pasteEnabled = true
}

func (s *testScreen) EnableFocus() {
	s.focusEnabled = true
}

func TestParseFlags(t *testing.T) {
	if err := parseFlags([]string{"-config-path", "/tmp/cfg", "-log-path", "/tmp/log", "-log-level", "warn"}); err != nil {
		t.Fatalf("parseFlags() error = %v", err)
	}
	if configPath != "/tmp/cfg" || logPath != "/tmp/log" || logLevel != "warn" {
		t.Fatalf("parsed values = (%q, %q, %q)", configPath, logPath, logLevel)
	}
}

func TestRun(t *testing.T) {
	oldArgs := os.Args
	oldLoadLogger := loadLogger
	oldLoadConfig := loadConfig
	oldNewScreen := newScreen
	oldNewApp := newApp
	oldNewRootModel := newRootModel
	oldRunApp := runApp
	oldRawEvents := ws.EnableRawEvents
	t.Cleanup(func() {
		os.Args = oldArgs
		loadLogger = oldLoadLogger
		loadConfig = oldLoadConfig
		newScreen = oldNewScreen
		newApp = oldNewApp
		newRootModel = oldNewRootModel
		runApp = oldRunApp
		ws.EnableRawEvents = oldRawEvents
	})

	cfg, _ := config.Load("")

	t.Run("Success", func(t *testing.T) {
		os.Args = []string{"discordo", "-log-level", "debug"}
		ws.EnableRawEvents = false

		loadLogger = func(path string, level slog.Level) error {
			if level != slog.LevelDebug {
				t.Fatalf("logger level = %v, want debug", level)
			}
			return nil
		}
		loadConfig = func(string) (*config.Config, error) { return cfg, nil }
		screen := &testScreen{}
		newScreen = func() (tcell.Screen, error) { return screen, nil }
		newApp = tview.NewApplication
		rootCalls := 0
		newRootModel = func(cfg *config.Config, app *tview.Application) tview.Primitive {
			rootCalls++
			return rootui.NewModel(cfg, app)
		}
		runCalls := 0
		runApp = func(*tview.Application) error {
			runCalls++
			return nil
		}

		if err := Run(); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if rootCalls != 1 || runCalls != 1 {
			t.Fatalf("expected one root model call and one run call, got %d and %d", rootCalls, runCalls)
		}
		if !ws.EnableRawEvents {
			t.Fatal("expected debug log level to enable raw websocket events")
		}
		if !screen.pasteEnabled || !screen.focusEnabled {
			t.Fatal("expected Run to enable paste and focus on the screen")
		}
	})

	t.Run("ParseError", func(t *testing.T) {
		os.Args = []string{"discordo", "-unknown"}
		if err := Run(); err == nil {
			t.Fatal("expected unknown flag to fail")
		}
	})

	t.Run("LoggerError", func(t *testing.T) {
		os.Args = []string{"discordo"}
		loadLogger = func(string, slog.Level) error { return errors.New("logger fail") }
		if err := Run(); err == nil || err.Error() != "failed to load logger: logger fail" {
			t.Fatalf("Run() error = %v", err)
		}
	})

	t.Run("ConfigError", func(t *testing.T) {
		os.Args = []string{"discordo"}
		loadLogger = func(string, slog.Level) error { return nil }
		loadConfig = func(string) (*config.Config, error) { return nil, errors.New("config fail") }
		if err := Run(); err == nil || err.Error() != "failed to load config: config fail" {
			t.Fatalf("Run() error = %v", err)
		}
	})

	t.Run("ScreenCreateError", func(t *testing.T) {
		os.Args = []string{"discordo", "-log-level", "warn"}
		loadLogger = func(path string, level slog.Level) error {
			if level != slog.LevelWarn {
				t.Fatalf("logger level = %v, want warn", level)
			}
			return nil
		}
		loadConfig = func(string) (*config.Config, error) { return cfg, nil }
		newScreen = func() (tcell.Screen, error) { return nil, errors.New("screen fail") }
		if err := Run(); err == nil || err.Error() != "failed to create screen: screen fail" {
			t.Fatalf("Run() error = %v", err)
		}
	})

	t.Run("ScreenInitError", func(t *testing.T) {
		os.Args = []string{"discordo"}
		loadLogger = func(string, slog.Level) error { return nil }
		loadConfig = func(string) (*config.Config, error) { return cfg, nil }
		newScreen = func() (tcell.Screen, error) { return &testScreen{initErr: errors.New("init failed")}, nil }
		if err := Run(); err == nil || err.Error() != "failed to init screen: init failed" {
			t.Fatalf("Run() error = %v", err)
		}
	})

	t.Run("RunAppError", func(t *testing.T) {
		os.Args = []string{"discordo", "-log-level", "error"}
		loadLogger = func(path string, level slog.Level) error {
			if level != slog.LevelError {
				t.Fatalf("logger level = %v, want error", level)
			}
			return nil
		}
		loadConfig = func(string) (*config.Config, error) { return cfg, nil }
		newScreen = func() (tcell.Screen, error) { return &testScreen{}, nil }
		newRootModel = func(cfg *config.Config, app *tview.Application) tview.Primitive { return rootui.NewModel(cfg, app) }
		runApp = func(*tview.Application) error { return errors.New("run fail") }
		if err := Run(); err == nil || err.Error() != "run fail" {
			t.Fatalf("Run() error = %v", err)
		}
	})
}

func TestDefaultRunAppWrapperPanicsOnNil(t *testing.T) {
	defaultRunApp := runApp
	defer func() { runApp = defaultRunApp }()

	defer func() {
		if recover() == nil {
			t.Fatal("expected default runApp wrapper to panic on nil app")
		}
	}()

	_ = defaultRunApp(nil)
}
