package chat

import (
	"testing"

	"github.com/ayn2op/tview"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/diamondburned/ningen/v3/states/read"
)

func TestModelOnMessageCreateDMAlerts(t *testing.T) {
	m := newTestModel()
	dmChannelID := discord.ChannelID(123)
	dmChannel := discord.Channel{ID: dmChannelID, Type: discord.DirectMessage}
	m.state.Cabinet.ChannelStore.ChannelSet(&dmChannel, false)
	me, _ := m.state.Cabinet.Me()

	t.Run("other user in non-selected DM", func(t *testing.T) {
		m.SetSelectedChannel(nil)
		m.guildsTree.clearDMAlert(dmChannelID)

		m.onMessageCreate(&gateway.MessageCreateEvent{Message: discord.Message{
			ID:        1,
			ChannelID: dmChannelID,
			Author:    discord.User{ID: 456, Username: "OtherUser"},
			Content:   "Hello",
		}})

		if count := m.guildsTree.dmAlertCounts[dmChannelID]; count != 1 {
			t.Fatalf("expected 1 DM alert, got %d", count)
		}
	})

	t.Run("selected focused DM does not alert", func(t *testing.T) {
		m.SetSelectedChannel(&dmChannel)
		m.appFocused = true
		m.guildsTree.clearDMAlert(dmChannelID)

		m.onMessageCreate(&gateway.MessageCreateEvent{Message: discord.Message{
			ID:        2,
			ChannelID: dmChannelID,
			Author:    discord.User{ID: 456, Username: "OtherUser"},
			Content:   "Are you there?",
		}})

		if count := m.guildsTree.dmAlertCounts[dmChannelID]; count != 0 {
			t.Fatalf("expected 0 DM alerts for current focused channel, got %d", count)
		}
	})

	t.Run("self message does not alert", func(t *testing.T) {
		m.SetSelectedChannel(&dmChannel)
		m.appFocused = false
		m.guildsTree.clearDMAlert(dmChannelID)

		m.onMessageCreate(&gateway.MessageCreateEvent{Message: discord.Message{
			ID:        3,
			ChannelID: dmChannelID,
			Author:    *me,
			Content:   "Yes, I am.",
		}})

		if count := m.guildsTree.dmAlertCounts[dmChannelID]; count != 0 {
			t.Fatalf("expected 0 DM alerts for self message, got %d", count)
		}
	})
}

func TestModelOnReadUpdateClearsDMAlertWithoutChannelNode(t *testing.T) {
	m := newTestModel()
	dmChannelID := discord.ChannelID(123)
	dmChannel := discord.Channel{ID: dmChannelID, Type: discord.DirectMessage}
	m.state.Cabinet.ChannelStore.ChannelSet(&dmChannel, false)

	m.guildsTree.addDMAlert(dmChannelID)
	delete(m.guildsTree.channelNodeByID, dmChannelID)

	m.onReadUpdate(&read.UpdateEvent{ReadState: gateway.ReadState{ChannelID: dmChannelID}})

	if count := m.guildsTree.dmAlertCounts[dmChannelID]; count != 0 {
		t.Fatalf("expected DM alert to be cleared, got %d", count)
	}
}

func TestModelOnReadyHydratesUnreadDMAlerts(t *testing.T) {
	m := newTestModel()
	dmChannelID := discord.ChannelID(123)
	readMessageID := discord.MessageID(100)
	lastMessageID := discord.MessageID(999)
	dmChannel := discord.Channel{
		ID:            dmChannelID,
		Type:          discord.DirectMessage,
		LastMessageID: lastMessageID,
		DMRecipients:  []discord.User{{ID: 456, Username: "OtherUser"}},
	}
	m.state.Cabinet.ChannelStore.ChannelSet(&dmChannel, false)
	m.state.ReadState.MarkRead(dmChannelID, readMessageID)

	m.onReady(&gateway.ReadyEvent{
		ReadyEventExtras: gateway.ReadyEventExtras{
			UserSettings: &gateway.UserSettings{},
		},
		PrivateChannels: []discord.Channel{dmChannel},
	})

	if count := m.guildsTree.dmAlertCounts[dmChannelID]; count != 1 {
		t.Fatalf("expected startup unread DM alert count 1, got %d", count)
	}
	children := m.guildsTree.GetRoot().GetChildren()
	if len(children) < 2 {
		t.Fatalf("expected top alert + dm root after READY hydration, got %d children", len(children))
	}
	if ref, ok := children[0].GetReference().(dmAlertRef); !ok || ref.channelID != dmChannelID {
		t.Fatalf("expected unread DM alert first, got %v", children[0].GetReference())
	}
}

func TestGuildsTreeReordersExpandedDMChannel(t *testing.T) {
	m := newTestModel()
	first := discord.Channel{ID: 1, Type: discord.DirectMessage}
	second := discord.Channel{ID: 2, Type: discord.DirectMessage}
	m.state.Cabinet.ChannelStore.ChannelSet(&first, false)
	m.state.Cabinet.ChannelStore.ChannelSet(&second, false)

	root := tview.NewTreeNode("Direct Messages").SetReference(dmNode{}).SetExpanded(true)
	m.guildsTree.dmRootNode = root
	m.guildsTree.createChannelNode(root, first)
	m.guildsTree.createChannelNode(root, second)

	m.guildsTree.reorderDMChannel(second.ID)
	if got := root.GetChildren()[0].GetReference(); got != second.ID {
		t.Fatalf("expected newest DM first, got %v", got)
	}
}
