package chat

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/ayn2op/tview/layers"
	"github.com/ayn2op/tview/picker"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/gdamore/tcell/v3"
	tcellcolor "github.com/gdamore/tcell/v3/color"
)

func TestReactionPickerSetItemsAndHelp(t *testing.T) {
	m := newTestModel()
	rp := newReactionPicker(m.cfg, m, m.messagesList)

	items := []discord.Emoji{
		{Name: "smile"},
		{ID: 123456, Name: "kekw"},
	}
	rp.SetItems(items)

	if len(rp.items) != len(items) {
		t.Fatalf("expected %d reaction items, got %d", len(items), len(rp.items))
	}
	if len(rp.ShortHelp()) == 0 || len(rp.FullHelp()) == 0 {
		t.Fatal("expected picker help to be populated")
	}
}

func TestReactionPickerSelectionAndCancel(t *testing.T) {
	m := newTestModel()
	rp := newReactionPicker(m.cfg, m, m.messagesList)
	m.messagesList.reactionPicker = rp
	rp.SetItems([]discord.Emoji{{Name: "smile"}})

	m.AddLayer(rp, layers.WithName(reactionPickerLayerName), layers.WithVisible(true))

	rp.Update(&picker.SelectedMsg{Item: picker.Item{Reference: "bad"}})
	rp.Update(&picker.SelectedMsg{Item: picker.Item{Reference: 99}})
	if !m.HasLayer(reactionPickerLayerName) {
		t.Fatal("expected invalid selection to keep picker open")
	}

	channel := &discord.Channel{ID: 200, GuildID: 100, Type: discord.GuildText}
	m.SetSelectedChannel(channel)
	m.messagesList.setMessages([]discord.Message{
		{ID: 300, ChannelID: channel.ID, GuildID: channel.GuildID, Author: discord.User{ID: 2, Username: "user"}},
	})
	m.messagesList.SetCursor(0)

	execCmdForTest(m.app, rp.Update(&picker.SelectedMsg{Item: picker.Item{Reference: 0}}))
	if m.HasLayer(reactionPickerLayerName) {
		t.Fatal("expected successful reaction to close picker")
	}
	if m.app.Focused() != m.messagesList {
		t.Fatalf("expected focus to return to messages list, got %T", m.app.Focused())
	}

	m.AddLayer(rp, layers.WithName(reactionPickerLayerName), layers.WithVisible(true))
	execCmdForTest(m.app, rp.Update(&picker.CancelMsg{}))
	if m.HasLayer(reactionPickerLayerName) {
		t.Fatal("expected cancel to close picker")
	}
}

func TestReactionPickerCloseDeletesKittyPreviews(t *testing.T) {
	m := newTestModel()
	m.cfg.InlineImages.Enabled = true
	m.cfg.InlineImages.Renderer = "kitty"
	rp := newReactionPicker(m.cfg, m, m.messagesList)
	m.messagesList.reactionPicker = rp
	m.AddLayer(rp, layers.WithName(reactionPickerLayerName), layers.WithVisible(true))

	item := newImageItem(rp.imageCache, "https://cdn.discordapp.com/emojis/123.png", inlineEmoteWidth, 1, true, 200123, nil, nil)
	item.kittyPlaced = true
	rp.emoteItemByKey["picker:test"] = item

	tty := &fakeTTY{}
	rp.lastScreen = &fakeScreen{tty: tty}

	execCmdForTest(m.app, rp.close())

	output := tty.String()
	if !strings.Contains(output, "\x1b_Gq=2,a=d,d=I,i=200123\x1b\\") {
		t.Fatalf("expected close to delete Kitty image id, got %q", output)
	}
	if len(rp.pendingDeletes) != 0 {
		t.Fatalf("expected pending deletes to be drained, got %v", rp.pendingDeletes)
	}
	if len(rp.emoteItemByKey) != 0 {
		t.Fatalf("expected preview item cache to be cleared, got %d entries", len(rp.emoteItemByKey))
	}
	if m.HasLayer(reactionPickerLayerName) {
		t.Fatal("expected close to remove reaction picker layer")
	}
}

type fakeTTY struct {
	bytes.Buffer
}

func (t *fakeTTY) Start() error             { return nil }
func (t *fakeTTY) Stop() error              { return nil }
func (t *fakeTTY) Drain() error             { return nil }
func (t *fakeTTY) NotifyResize(chan<- bool) {}
func (t *fakeTTY) WindowSize() (tcell.WindowSize, error) {
	return tcell.WindowSize{Width: 80, Height: 24, PixelWidth: 800, PixelHeight: 480}, nil
}
func (t *fakeTTY) Read([]byte) (int, error) { return 0, io.EOF }
func (t *fakeTTY) Close() error             { return nil }

type fakeScreen struct {
	tty tcell.Tty
}

func (s *fakeScreen) Init() error                                           { return nil }
func (s *fakeScreen) Fini()                                                 {}
func (s *fakeScreen) Clear()                                                {}
func (s *fakeScreen) Fill(rune, tcell.Style)                                {}
func (s *fakeScreen) Put(int, int, string, tcell.Style) (string, int)       { return "", 0 }
func (s *fakeScreen) PutStr(int, int, string)                               {}
func (s *fakeScreen) PutStrStyled(int, int, string, tcell.Style)            {}
func (s *fakeScreen) Get(int, int) (string, tcell.Style, int)               { return "", tcell.StyleDefault, 1 }
func (s *fakeScreen) SetContent(int, int, rune, []rune, tcell.Style)        {}
func (s *fakeScreen) SetStyle(tcell.Style)                                  {}
func (s *fakeScreen) ShowCursor(int, int)                                   {}
func (s *fakeScreen) HideCursor()                                           {}
func (s *fakeScreen) SetCursorStyle(tcell.CursorStyle, ...tcellcolor.Color) {}
func (s *fakeScreen) Size() (int, int)                                      { return 80, 24 }
func (s *fakeScreen) EventQ() chan tcell.Event                              { return make(chan tcell.Event) }
func (s *fakeScreen) EnableMouse(...tcell.MouseFlags)                       {}
func (s *fakeScreen) DisableMouse()                                         {}
func (s *fakeScreen) EnablePaste()                                          {}
func (s *fakeScreen) DisablePaste()                                         {}
func (s *fakeScreen) EnableFocus()                                          {}
func (s *fakeScreen) DisableFocus()                                         {}
func (s *fakeScreen) Colors() int                                           { return 256 }
func (s *fakeScreen) Show()                                                 {}
func (s *fakeScreen) Sync()                                                 {}
func (s *fakeScreen) CharacterSet() string                                  { return "UTF-8" }
func (s *fakeScreen) RegisterRuneFallback(rune, string)                     {}
func (s *fakeScreen) UnregisterRuneFallback(rune)                           {}
func (s *fakeScreen) Resize(int, int, int, int)                             {}
func (s *fakeScreen) Suspend() error                                        { return nil }
func (s *fakeScreen) Resume() error                                         { return nil }
func (s *fakeScreen) Beep() error                                           { return nil }
func (s *fakeScreen) SetSize(int, int)                                      {}
func (s *fakeScreen) LockRegion(int, int, int, int, bool)                   {}
func (s *fakeScreen) Tty() (tcell.Tty, bool)                                { return s.tty, s.tty != nil }
func (s *fakeScreen) SetTitle(string)                                       {}
func (s *fakeScreen) SetClipboard([]byte)                                   {}
func (s *fakeScreen) GetClipboard()                                         {}
func (s *fakeScreen) HasClipboard() bool                                    { return false }
func (s *fakeScreen) ShowNotification(string, string)                       {}
func (s *fakeScreen) Terminal() (string, string)                            { return "", "" }
