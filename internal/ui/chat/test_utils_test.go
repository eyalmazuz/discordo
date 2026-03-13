package chat

import (
	"github.com/ayn2op/discordo/internal/config"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/session"
	"github.com/diamondburned/arikawa/v3/state"
	"github.com/diamondburned/arikawa/v3/state/store/defaultstore"
	"github.com/diamondburned/ningen/v3"
	"github.com/ayn2op/tview"
	"github.com/gdamore/tcell/v3"
	"github.com/gdamore/tcell/v3/color"
)

type completeMockScreen struct {
	MockScreen
}

func (m *completeMockScreen) Init() error { return nil }
func (m *completeMockScreen) Fini() {}
func (m *completeMockScreen) Clear() {}
func (m *completeMockScreen) Fill(rune, tcell.Style) {}
func (m *completeMockScreen) Show() {}
func (m *completeMockScreen) CharacterSet() string { return "UTF-8" }
func (m *completeMockScreen) Size() (int, int) { return 80, 24 }
func (m *completeMockScreen) HasMouse() bool { return false }
func (m *completeMockScreen) HasKey(tcell.Key) bool { return true }
func (m *completeMockScreen) Colors() int { return 256 }
func (m *completeMockScreen) SetCursorStyle(tcell.CursorStyle, ...color.Color) {}
func (m *completeMockScreen) ShowCursor(x, y int) {}
func (m *completeMockScreen) HideCursor() {}
func (m *completeMockScreen) SetStyle(tcell.Style) {}
func (m *completeMockScreen) GetContent(x, y int) (rune, []rune, tcell.Style, int) { return ' ', nil, tcell.StyleDefault, 1 }
func (m *completeMockScreen) SetSize(int, int) {}
func (m *completeMockScreen) Channel() chan tcell.Event { return make(chan tcell.Event) }
func (m *completeMockScreen) EventQ() chan tcell.Event { return make(chan tcell.Event) }
func (m *completeMockScreen) PostEvent(tcell.Event) error { return nil }
func (m *completeMockScreen) PostEventWait(tcell.Event) {}
func (m *completeMockScreen) Sync() {}
func (m *completeMockScreen) Register() {}
func (m *completeMockScreen) Unregister() {}
func (m *completeMockScreen) EnableMouse(...tcell.MouseFlags) {}
func (m *completeMockScreen) DisableMouse() {}
func (m *completeMockScreen) EnablePaste() {}
func (m *completeMockScreen) DisablePaste() {}
func (m *completeMockScreen) Reload() {}
func (m *completeMockScreen) SetClip(x, y, w, h int) {}
func (m *completeMockScreen) GetClip() (int, int, int, int) { return 0, 0, 80, 24 }
func (m *completeMockScreen) SetAttributes(tcell.AttrMask) {}
func (m *completeMockScreen) Beep() error { return nil }
func (m *completeMockScreen) SetTitle(string) {}
func (m *completeMockScreen) Stop() {}
func (m *completeMockScreen) Pause() {}
func (m *completeMockScreen) Resume() error { return nil }
func (m *completeMockScreen) IsPaused() bool { return false }
func (m *completeMockScreen) Put(x, y int, s string, style tcell.Style) (string, int) { return s, len(s) }

func newTestModel() *Model {
	cfg := &config.Config{}
	app := tview.NewApplication()
	m := NewView(app, cfg, "token")
	
	s := state.NewFromSession(session.New(""), defaultstore.New())
	m.state = ningen.FromState(s)
	m.state.Cabinet.MeStore.MyselfSet(discord.User{ID: 1}, false)
	
	app.SetScreen(&completeMockScreen{})
	go app.Run()
	
	return m
}
