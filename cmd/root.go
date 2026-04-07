package cmd

import (
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/ayn2op/discordo/internal/config"
	"github.com/ayn2op/discordo/internal/logger"
	"github.com/ayn2op/discordo/internal/ui/root"
	"github.com/diamondburned/arikawa/v3/utils/ws"
	"github.com/eyalmazuz/tview"
	"github.com/gdamore/tcell/v3"
)

var (
	configPath string
	logPath    string
	logLevel   string
)

var (
	parseFlags = func(args []string) error {
		fs := flag.NewFlagSet("discordo", flag.ContinueOnError)
		fs.StringVar(&configPath, "config-path", config.DefaultPath(), "path of the configuration file")
		fs.StringVar(&logPath, "log-path", logger.DefaultPath(), "path of the log file")
		fs.StringVar(&logLevel, "log-level", "info", "log level")
		return fs.Parse(args)
	}
	loadLogger   = logger.Load
	loadConfig   = config.Load
	newScreen    = tcell.NewScreen
	newApp       = tview.NewApplication
	newRootModel = func(cfg *config.Config, app *tview.Application) tview.Model {
		return root.NewModel(cfg, app)
	}
	runApp = func(app *tview.Application) error { return app.Run() }
)

func Run() error {
	if err := parseFlags(os.Args[1:]); err != nil {
		return err
	}

	var level slog.Level
	switch logLevel {
	case "debug":
		ws.EnableRawEvents = true
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}

	if err := loadLogger(logPath, level); err != nil {
		return fmt.Errorf("failed to load logger: %w", err)
	}

	cfg, err := loadConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	screen, err := newScreen()
	if err != nil {
		return fmt.Errorf("failed to create screen: %w", err)
	}

	if err := screen.Init(); err != nil {
		return fmt.Errorf("failed to init screen: %w", err)
	}

	if cfg.Mouse {
		screen.EnableMouse()
	}
	screen.EnablePaste()
	screen.EnableFocus()

	tview.Styles = tview.Theme{}
	app := newApp(tview.WithScreen(screen))
	app.SetRoot(newRootModel(cfg, app))
	return runApp(app)
}
