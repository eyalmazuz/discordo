package chat

import (
	"testing"

	"github.com/ayn2op/tview/layers"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/gdamore/tcell/v3"
)

func TestEscapeClearsSelectedChannelAndFocusesGuildsTree(t *testing.T) {
	m := newTestModel()
	channel := &discord.Channel{ID: 123, Type: discord.DirectMessage}
	m.SetSelectedChannel(channel)
	m.messageInput.SetDisabled(false)
	m.messagesList.setMessages([]discord.Message{{ID: 1, ChannelID: channel.ID, Author: discord.User{ID: 2}}})

	execCmdForTest(m.app, m.Update(tcell.NewEventKey(tcell.KeyEscape, "", tcell.ModNone)))

	if selected := m.SelectedChannel(); selected != nil {
		t.Fatalf("expected selected channel to be cleared, got %v", selected.ID)
	}
	if !m.messageInput.GetDisabled() {
		t.Fatal("expected message input to be disabled after clearing selected channel")
	}
	if m.app.Focused() != m.guildsTree {
		t.Fatalf("expected focus to return to guild tree, got %T", m.app.Focused())
	}
}

func TestEscapeDoesNotClearChannelBehindOverlay(t *testing.T) {
	m := newTestModel()
	channel := &discord.Channel{ID: 123, Type: discord.DirectMessage}
	m.SetSelectedChannel(channel)
	m.AddLayer(m.channelsPicker, layers.WithName(channelsPickerLayerName), layers.WithVisible(true), layers.WithOverlay())

	execCmdForTest(m.app, m.Update(tcell.NewEventKey(tcell.KeyEscape, "", tcell.ModNone)))

	if selected := m.SelectedChannel(); selected == nil || selected.ID != channel.ID {
		t.Fatalf("expected selected channel to remain while overlay handles Escape, got %v", selected)
	}
}
