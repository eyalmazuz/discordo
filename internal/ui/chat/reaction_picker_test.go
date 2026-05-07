package chat

import (
	"testing"

	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/eyalmazuz/tview/layers"
	"github.com/eyalmazuz/tview/picker"
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
