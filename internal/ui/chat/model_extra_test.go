package chat

import (
	"strings"
	"testing"
	"time"

	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/diamondburned/ningen/v3/states/read"
	"github.com/eyalmazuz/tview"
	"github.com/eyalmazuz/tview/layers"
	"github.com/gdamore/tcell/v3"
)

func TestModelOverlayAndSearchHelpers(t *testing.T) {
	m := newTestModel()
	channel := &discord.Channel{ID: 123, Type: discord.DirectMessage}
	m.SetSelectedChannel(channel)

	if m.hasPopupOverlay() {
		t.Fatal("expected no popup overlay initially")
	}

	m.openPicker()
	if !m.hasPopupOverlay() {
		t.Fatal("expected channels picker to count as popup overlay")
	}
	m.closePicker()

	m.openMessageSearch()
	if !m.HasLayer(messageSearchLayerName) {
		t.Fatal("expected message search popup to open")
	}
	if m.app.Focused() != m.messageSearch.input {
		t.Fatalf("expected focus on search input, got %T", m.app.Focused())
	}

	m.RemoveLayer(messageSearchLayerName)
	m.openPinnedMessages()
	if !m.HasLayer(pinnedMessagesLayerName) {
		t.Fatal("expected pinned messages popup to open")
	}
}

func TestModelCloseLayerEventAndConfirmModal(t *testing.T) {
	m := newTestModel()
	m.AddLayer(tview.NewBox(), layers.WithName("test"), layers.WithVisible(true))

	if cmd := m.Update(&closeLayerEvent{name: "test"}); cmd != nil {
		t.Fatalf("expected close-layer event to return nil, got %T", cmd)
	}
	if m.GetVisible("test") {
		t.Fatal("expected close-layer event to hide the layer")
	}

	setFocusForTest(m.app, m.messageInput)
	called := ""
	m.showConfirmModal("confirm", []string{"Yes", "No"}, func(label string) { called = label })
	m.Update(&tview.ModalDoneMsg{ButtonLabel: "No"})
	if called != "No" {
		t.Fatalf("expected modal callback label %q, got %q", "No", called)
	}
}

func TestModelTyperFooterAndExpiry(t *testing.T) {
	oldTypingAfterFunc := typingAfterFunc
	t.Cleanup(func() { typingAfterFunc = oldTypingAfterFunc })

	var callback func()
	typingAfterFunc = func(_ time.Duration, fn func()) *time.Timer {
		callback = fn
		return time.NewTimer(time.Hour)
	}

	m := newTestModel()
	channel := &discord.Channel{ID: 100, Type: discord.DirectMessage, DMRecipients: []discord.User{{ID: 3, Username: "user3"}}}
	m.SetSelectedChannel(channel)
	m.addTyper(3)
	if !strings.Contains(m.messagesList.GetFooter(), "user3") {
		t.Fatalf("expected footer to mention active typer, got %q", m.messagesList.GetFooter())
	}
	if callback == nil {
		t.Fatal("expected typer expiry callback to be installed")
	}
	callback()
	time.Sleep(20 * time.Millisecond)
	if _, ok := m.typers[3]; ok {
		t.Fatal("expected expiry callback to remove typer")
	}
}

func TestModelKeyRoutingAndReadUpdate(t *testing.T) {
	m := newTestModel()
	m.messageInput.SetDisabled(false)

	for _, key := range []*tcell.EventKey{
		tcell.NewEventKey(tcell.KeyCtrlG, "", tcell.ModNone),
		tcell.NewEventKey(tcell.KeyCtrlT, "", tcell.ModNone),
		tcell.NewEventKey(tcell.KeyCtrlI, "", tcell.ModNone),
		tcell.NewEventKey(tcell.KeyCtrlF, "", tcell.ModNone),
		tcell.NewEventKey(tcell.KeyCtrlP, "", tcell.ModNone),
	} {
		m.Update(key)
	}

	m.guildsTree.guildNodeByID[1] = tview.NewTreeNode("G")
	m.guildsTree.channelNodeByID[2] = tview.NewTreeNode("C")
	m.onReadUpdate(&read.UpdateEvent{
		ReadState: gateway.ReadState{ChannelID: 2},
		GuildID:   1,
	})
}
