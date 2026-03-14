package root

import (
	"errors"
	"os"
	"syscall"
	"testing"

	"github.com/ayn2op/discordo/internal/config"
	chatpkg "github.com/ayn2op/discordo/internal/ui/chat"
	loginpkg "github.com/ayn2op/discordo/internal/ui/login"
	qrpkg "github.com/ayn2op/discordo/internal/ui/login/qr"
	tokenpkg "github.com/ayn2op/discordo/internal/ui/login/token"
	"github.com/ayn2op/tview/keybind"
	"github.com/ayn2op/tview"
	"github.com/gdamore/tcell/v3"
)

type stubRootInner struct {
	*tview.Box
	cmd     tview.Command
	handled int
	focused bool
	blurred bool
}

type stubRootInnerKeyMap struct {
	*stubRootInner
	short []keybind.Keybind
	full  [][]keybind.Keybind
}

func (s *stubRootInner) HandleEvent(event tcell.Event) tview.Command {
	s.handled++
	return s.cmd
}

func (s *stubRootInner) Focus(delegate func(tview.Primitive)) {
	s.focused = true
	delegate(s)
}

func (s *stubRootInner) HasFocus() bool {
	return s.focused
}

func (s *stubRootInner) Blur() {
	s.blurred = true
	s.focused = false
}

func (s *stubRootInnerKeyMap) ShortHelp() []keybind.Keybind {
	return s.short
}

func (s *stubRootInnerKeyMap) FullHelp() [][]keybind.Keybind {
	return s.full
}

func newTestRootModel(t *testing.T) *Model {
	t.Helper()

	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	return NewModel(cfg, tview.NewApplication())
}

func TestRootEventCommands(t *testing.T) {
	oldGetStoredToken := getStoredToken
	oldSetStoredToken := setStoredToken
	oldDeleteStoredToken := deleteStoredToken
	oldInitClipboardFn := initClipboardFn
	t.Cleanup(func() {
		getStoredToken = oldGetStoredToken
		setStoredToken = oldSetStoredToken
		deleteStoredToken = oldDeleteStoredToken
		initClipboardFn = oldInitClipboardFn
	})

	tokenEventCmd, ok := tokenCommand("abc").(tview.EventCommand)
	if !ok {
		t.Fatalf("expected tokenCommand to return EventCommand, got %T", tokenCommand("abc"))
	}
	if event, ok := tokenEventCmd().(*tokenEvent); !ok || event.token != "abc" {
		t.Fatalf("expected token event with token %q, got %#v", "abc", event)
	}

	getStoredToken = func() (string, error) { return "stored-token", nil }
	getTokenCmd := getToken().(tview.EventCommand)
	if event, ok := getTokenCmd().(*tokenEvent); !ok || event.token != "stored-token" {
		t.Fatalf("expected stored token event, got %#v", event)
	}

	getStoredToken = func() (string, error) { return "", errors.New("missing") }
	if _, ok := getTokenCmd().(*loginEvent); !ok {
		t.Fatal("expected missing keyring token to fall back to login event")
	}

	var storedToken string
	setStoredToken = func(token string) error {
		storedToken = token
		return nil
	}
	if event := setToken("persist").(tview.EventCommand)(); event != nil {
		t.Fatalf("expected successful setToken to return nil event, got %T", event)
	}
	if storedToken != "persist" {
		t.Fatalf("expected setToken to store %q, got %q", "persist", storedToken)
	}
	setStoredToken = func(string) error { return errors.New("set fail") }
	if event := setToken("persist").(tview.EventCommand)(); event == nil {
		t.Fatal("expected failed setToken to return an error event")
	}

	deleted := false
	deleteStoredToken = func() error {
		deleted = true
		return nil
	}
	if event := deleteToken().(tview.EventCommand)(); event != nil {
		t.Fatalf("expected successful deleteToken to return nil event, got %T", event)
	}
	if !deleted {
		t.Fatal("expected deleteToken to call the delete seam")
	}
	deleteStoredToken = func() error { return errors.New("delete fail") }
	if event := deleteToken().(tview.EventCommand)(); event == nil {
		t.Fatal("expected failed deleteToken to return an error event")
	}

	initClipboardFn = func() error { return nil }
	if event := initClipboard().(tview.EventCommand)(); event != nil {
		t.Fatalf("expected successful initClipboard to return nil event, got %T", event)
	}
	initClipboardFn = func() error { return errors.New("clipboard fail") }
	if event := initClipboard().(tview.EventCommand)(); event == nil {
		t.Fatal("expected failed initClipboard to return an error event")
	}
}

func TestRootModelHandleEventAndHelpers(t *testing.T) {
	m := newTestRootModel(t)

	oldGetStoredToken := getStoredToken
	oldInitClipboardFn := initClipboardFn
	oldSuspendApp := suspendApp
	oldNotifySignal := notifySignal
	oldStopSignal := stopSignal
	oldKillProcess := killProcess
	t.Cleanup(func() {
		getStoredToken = oldGetStoredToken
		initClipboardFn = oldInitClipboardFn
		suspendApp = oldSuspendApp
		notifySignal = oldNotifySignal
		stopSignal = oldStopSignal
		killProcess = oldKillProcess
		os.Unsetenv(tokenEnvVarKey)
	})

	initClipboardFn = func() error { return nil }

	getStoredToken = func() (string, error) { return "from-keyring", nil }
	os.Unsetenv(tokenEnvVarKey)
	cmd := m.HandleEvent(tview.NewInitEvent())
	batch, ok := cmd.(tview.BatchCommand)
	if !ok {
		t.Fatalf("expected init event to return a BatchCommand, got %T", cmd)
	}
	foundTokenEvent := false
	for _, c := range batch {
		if evCmd, ok := c.(tview.EventCommand); ok {
			if ev, ok := evCmd().(*tokenEvent); ok && ev.token == "from-keyring" {
				foundTokenEvent = true
				break
			}
		}
	}
	if !foundTokenEvent {
		t.Fatalf("expected init batch to contain token event with %q, got %#v", "from-keyring", batch)
	}

	os.Setenv(tokenEnvVarKey, "from-env")
	cmd = m.HandleEvent(tview.NewInitEvent())
	batch, ok = cmd.(tview.BatchCommand)
	if !ok {
		t.Fatalf("expected init event with env token to return a BatchCommand, got %T", cmd)
	}
	foundTokenEvent = false
	for _, c := range batch {
		if evCmd, ok := c.(tview.EventCommand); ok {
			if ev, ok := evCmd().(*tokenEvent); ok && ev.token == "from-env" {
				foundTokenEvent = true
				break
			}
		}
	}
	if !foundTokenEvent {
		t.Fatalf("expected init batch to contain env token event with %q, got %#v", "from-env", batch)
	}

	if _, ok := m.HandleEvent(newLoginEvent()).(tview.BatchCommand); !ok {
		t.Fatal("expected login event to show the login view")
	}
	if _, ok := m.inner.(*loginpkg.Model); !ok {
		t.Fatalf("expected login event to install a login model, got %T", m.inner)
	}

	if _, ok := m.HandleEvent(newTokenEvent("chat-token")).(tview.BatchCommand); !ok {
		t.Fatal("expected token event to show the chat view")
	}

	if _, ok := m.HandleEvent(&tokenpkg.TokenEvent{Token: "token-tab"}).(tview.BatchCommand); !ok {
		t.Fatal("expected token tab event to return a batch command")
	}
	if _, ok := m.HandleEvent(&qrpkg.TokenEvent{Token: "qr-tab"}).(tview.BatchCommand); !ok {
		t.Fatal("expected QR tab event to return a batch command")
	}
	if _, ok := m.HandleEvent(&chatpkg.LogoutEvent{}).(tview.BatchCommand); !ok {
		t.Fatal("expected logout event to return a batch command")
	}

	if _, ok := m.HandleEvent(tcell.NewEventKey(tcell.KeyRune, ".", tcell.ModCtrl)).(tview.RedrawCommand); !ok {
		t.Fatal("expected toggle-help key to request a redraw")
	}
	if !m.help.ShowAll() {
		t.Fatal("expected toggle-help key to enable full help")
	}

	suspended := false
	suspendApp = func(_ *tview.Application, fn func()) {
		suspended = true
		fn()
	}
	notifySignal = func(c chan<- os.Signal, _ ...os.Signal) {
		c <- syscall.SIGCONT
	}
	stopSignal = func(chan<- os.Signal) {}
	killProcess = func(int, syscall.Signal) error { return nil }
	if cmd := m.HandleEvent(tcell.NewEventKey(tcell.KeyCtrlZ, "", tcell.ModCtrl)); cmd != nil {
		t.Fatalf("expected suspend key to return nil, got %T", cmd)
	}
	if !suspended {
		t.Fatal("expected suspend key to hit the suspend path")
	}

	inner := &stubRootInner{Box: tview.NewBox(), cmd: tview.RedrawCommand{}}
	m.inner = inner
	if _, ok := m.HandleEvent(tcell.NewEventKey(tcell.KeyRune, "x", tcell.ModNone)).(tview.RedrawCommand); !ok {
		t.Fatal("expected unmatched keys to be forwarded to the inner primitive")
	}
	if inner.handled != 1 {
		t.Fatalf("expected forwarded key to hit inner primitive once, got %d", inner.handled)
	}

	quitBatch, ok := m.HandleEvent(tcell.NewEventKey(tcell.KeyCtrlC, "", tcell.ModCtrl)).(tview.BatchCommand)
	if !ok {
		t.Fatalf("expected quit key to return a batch command, got %T", quitBatch)
	}
	if len(quitBatch) != 2 {
		t.Fatalf("expected quit batch to contain inner and quit commands, got %d entries", len(quitBatch))
	}
	if inner.handled != 2 {
		t.Fatalf("expected quit to forward a quit event to the inner primitive, got %d total calls", inner.handled)
	}

	m.Focus(func(tview.Primitive) {})
	inner.focused = true
	if !m.HasFocus() {
		t.Fatal("expected HasFocus to proxy to the inner primitive")
	}
	m.Blur()
	if !inner.blurred {
		t.Fatal("expected Blur to proxy to the inner primitive")
	}
	if m.activeKeyMap() != nil {
		t.Fatalf("expected non-keymap inner primitive to return nil active key map, got %T", m.activeKeyMap())
	}
	if len(m.ShortHelp()) == 0 || len(m.FullHelp()) == 0 {
		t.Fatal("expected root help to be populated")
	}

	keyed := &stubRootInnerKeyMap{
		stubRootInner: &stubRootInner{Box: tview.NewBox()},
		short:         []keybind.Keybind{keybind.NewKeybind(keybind.WithHelp("x", "inner"))},
		full:          [][]keybind.Keybind{{keybind.NewKeybind(keybind.WithHelp("x", "inner"))}},
	}
	m.inner = keyed
	if m.activeKeyMap() == nil {
		t.Fatal("expected keymap-aware inner primitive to be returned")
	}
	if got := len(m.ShortHelp()); got < 4 {
		t.Fatalf("expected short help to include inner and global bindings, got %d entries", got)
	}
	if got := len(m.FullHelp()); got < 2 {
		t.Fatalf("expected full help to include inner and global groups, got %d groups", got)
	}

	nilInner := newTestRootModel(t)
	if !nilInner.HasFocus() {
		t.Fatal("expected root model without inner primitive to report focus")
	}
	if cmd := nilInner.HandleEvent(tcell.NewEventKey(tcell.KeyRune, "x", tcell.ModNone)); cmd != nil {
		t.Fatalf("expected unmatched key without inner primitive to return nil, got %T", cmd)
	}
	if batch, ok := nilInner.HandleEvent(tcell.NewEventKey(tcell.KeyCtrlC, "", tcell.ModCtrl)).(tview.BatchCommand); !ok || len(batch) != 2 {
		t.Fatalf("expected quit without inner primitive to return a 2-entry batch command, got %#v", batch)
	}
}

func TestRootModelGeometry(t *testing.T) {
	m := newTestRootModel(t)
	m.SetRect(2, 3, 40, 12)
	x, y, w, h := m.GetRect()
	if x != 2 || y != 3 || w != 40 || h != 12 {
		t.Fatalf("unexpected rect: %d %d %d %d", x, y, w, h)
	}
}
