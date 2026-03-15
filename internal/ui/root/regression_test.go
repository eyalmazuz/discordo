package root

import (
	"errors"
	"testing"
	"time"

	"github.com/ayn2op/discordo/internal/config"
	"github.com/ayn2op/discordo/internal/ui/chat"
	"github.com/ayn2op/discordo/internal/ui/login"
	"github.com/ayn2op/discordo/internal/ui/login/token"
	"github.com/eyalmazuz/tview"
	"github.com/gdamore/tcell/v3"
	"github.com/gdamore/tcell/v3/color"
)

type mockScreen struct {
	tcell.Screen
}

func (m *mockScreen) Init() error { return nil }
func (m *mockScreen) Fini() {}
func (m *mockScreen) Clear() {}
func (m *mockScreen) Fill(rune, tcell.Style) {}
func (m *mockScreen) Show() {}
func (m *mockScreen) CharacterSet() string { return "UTF-8" }
func (m *mockScreen) Size() (int, int) { return 80, 24 }
func (m *mockScreen) HasMouse() bool { return false }
func (m *mockScreen) HasKey(tcell.Key) bool { return true }
func (m *mockScreen) Colors() int { return 256 }
func (m *mockScreen) SetCursorStyle(tcell.CursorStyle, ...color.Color) {}
func (m *mockScreen) ShowCursor(x, y int) {}
func (m *mockScreen) HideCursor() {}
func (m *mockScreen) SetStyle(tcell.Style) {}
func (m *mockScreen) GetContent(x, y int) (rune, []rune, tcell.Style, int) { return ' ', nil, tcell.StyleDefault, 1 }
func (m *mockScreen) Get(x, y int) (string, tcell.Style, int) { return " ", tcell.StyleDefault, 1 }
func (m *mockScreen) SetSize(int, int) {}
func (m *mockScreen) Channel() chan tcell.Event { return make(chan tcell.Event) }
func (m *mockScreen) EventQ() chan tcell.Event { return make(chan tcell.Event) }
func (m *mockScreen) PostEvent(tcell.Event) error { return nil }
func (m *mockScreen) PostEventWait(tcell.Event) {}
func (m *mockScreen) Sync() {}
func (m *mockScreen) Register() {}
func (m *mockScreen) Unregister() {}
func (m *mockScreen) EnableMouse(...tcell.MouseFlags) {}
func (m *mockScreen) DisableMouse() {}
func (m *mockScreen) EnablePaste() {}
func (m *mockScreen) DisablePaste() {}
func (m *mockScreen) Reload() {}
func (m *mockScreen) SetClip(x, y, w, h int) {}
func (m *mockScreen) GetClip() (int, int, int, int) { return 0, 0, 80, 24 }
func (m *mockScreen) SetAttributes(tcell.AttrMask) {}
func (m *mockScreen) Beep() error { return nil }
func (m *mockScreen) SetTitle(string) {}
func (m *mockScreen) Suspend() error { return nil }
func (m *mockScreen) Pause() {}
func (m *mockScreen) Resume() error { return nil }
func (m *mockScreen) IsPaused() bool { return false }
func (m *mockScreen) Put(x, y int, s string, style tcell.Style) (string, int) { return s, len(s) }
func (m *mockScreen) PutStrStyled(x, y int, s string, style tcell.Style)      {}
func (m *mockScreen) SetContent(int, int, rune, []rune, tcell.Style)          {}
func (m *mockScreen) LockRegion(int, int, int, int, bool)                     {}

func init() {
	// Stub external dependencies
	getStoredToken = func() (string, error) { return "", errors.New("no token") }
	setStoredToken = func(string) error { return nil }
	deleteStoredToken = func() error { return nil }
	initClipboardFn = func() error { return nil }
}

func TestRegression_AppLifecycle(t *testing.T) {
	cfg, _ := config.Load("")
	app := tview.NewApplication()
	app.SetScreen(&mockScreen{})

	m := NewModel(cfg, app)
	app.SetRoot(m)

	// Run app in background
	go func() {
		_ = app.Run()
	}()

	// Give it time to initialize
	time.Sleep(200 * time.Millisecond)

	// 1. Initial state - should show login
	if _, ok := m.inner.(*login.Model); !ok {
		t.Errorf("expected login view (*login.Model), got %T", m.inner)
	}

	// 2. Simulate login
	testToken := "test-token"
	app.QueueEvent(&token.TokenEvent{Token: testToken})
	time.Sleep(200 * time.Millisecond)

	if _, ok := m.inner.(*chat.Model); !ok {
		t.Errorf("expected chat view after login, got %T", m.inner)
	}

	// 3. Simulate logout
	app.QueueEvent(&chat.LogoutEvent{})
	time.Sleep(200 * time.Millisecond)

	if _, ok := m.inner.(*login.Model); !ok {
		t.Errorf("expected back to login view after logout, got %T", m.inner)
	}
}

func TestRegression_GlobalKeybinds(t *testing.T) {
	cfg, _ := config.Load("")
	app := tview.NewApplication()
	app.SetScreen(&mockScreen{})

	m := NewModel(cfg, app)
	app.SetRoot(m)

	go func() {
		_ = app.Run()
	}()
	time.Sleep(200 * time.Millisecond)

	// Test ToggleHelp (Ctrl+.)
	app.QueueEvent(tcell.NewEventKey(tcell.KeyRune, ".", tcell.ModCtrl))
	time.Sleep(200 * time.Millisecond)

	if !m.help.ShowAll() {
		t.Error("expected help to be expanded after toggle")
	}

	app.QueueEvent(tcell.NewEventKey(tcell.KeyRune, ".", tcell.ModCtrl))
	time.Sleep(200 * time.Millisecond)

	if m.help.ShowAll() {
		t.Error("expected help to be collapsed after second toggle")
	}
}
