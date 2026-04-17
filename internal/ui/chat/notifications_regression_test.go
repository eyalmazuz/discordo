package chat

import (
	"testing"

	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/diamondburned/ningen/v3/states/read"
	"github.com/eyalmazuz/tview"
)

func TestModel_onMessageCreate_DMAlerts(t *testing.T) {
	m := newMockChatModel()
	dmChannelID := discord.ChannelID(123)
	dmChannel := discord.Channel{ID: dmChannelID, Type: discord.DirectMessage}
	m.state.Cabinet.ChannelStore.ChannelSet(&dmChannel, false)

	me, _ := m.state.Cabinet.Me()

	t.Run("Other user sends message in non-selected DM", func(t *testing.T) {
		m.SetSelectedChannel(nil)
		m.guildsTree.clearDMAlert(dmChannelID)

		event := &gateway.MessageCreateEvent{
			Message: discord.Message{
				ID:        1,
				ChannelID: dmChannelID,
				Author:    discord.User{ID: 456, Username: "OtherUser"},
				Content:   "Hello",
			},
		}

		m.onMessageCreate(event)

		if count := m.guildsTree.dmAlertCounts[dmChannelID]; count != 1 {
			t.Errorf("expected 1 DM alert, got %d", count)
		}
	})

	t.Run("Other user sends message in selected DM and app is focused", func(t *testing.T) {
		m.SetSelectedChannel(&dmChannel)
		m.appFocused = true
		m.guildsTree.clearDMAlert(dmChannelID)

		event := &gateway.MessageCreateEvent{
			Message: discord.Message{
				ID:        2,
				ChannelID: dmChannelID,
				Author:    discord.User{ID: 456, Username: "OtherUser"},
				Content:   "Are you there?",
			},
		}

		m.onMessageCreate(event)

		if count := m.guildsTree.dmAlertCounts[dmChannelID]; count != 0 {
			t.Errorf("expected 0 DM alerts for current channel, got %d", count)
		}
	})

	t.Run("Current user sends message in selected DM", func(t *testing.T) {
		m.SetSelectedChannel(&dmChannel)
		m.guildsTree.clearDMAlert(dmChannelID)

		event := &gateway.MessageCreateEvent{
			Message: discord.Message{
				ID:        3,
				ChannelID: dmChannelID,
				Author:    *me,
				Content:   "Yes, I am.",
			},
		}

		m.onMessageCreate(event)

		if count := m.guildsTree.dmAlertCounts[dmChannelID]; count != 0 {
			t.Errorf("expected 0 DM alerts for self message, got %d", count)
		}
	})
}

func TestModel_onReadUpdate_ClearDMAlert(t *testing.T) {
	m := newMockChatModel()
	dmChannelID := discord.ChannelID(123)
	dmChannel := discord.Channel{ID: dmChannelID, Type: discord.DirectMessage}
	m.state.Cabinet.ChannelStore.ChannelSet(&dmChannel, false)

	t.Run("Clears alert even if channel node is not in tree", func(t *testing.T) {
		m.guildsTree.addDMAlert(dmChannelID)
		if count := m.guildsTree.dmAlertCounts[dmChannelID]; count != 1 {
			t.Fatalf("setup failed: expected 1 DM alert, got %d", count)
		}

		// Ensure channel node is NOT in the tree maps
		delete(m.guildsTree.channelNodeByID, dmChannelID)

		// Simulate read event
		event := &read.UpdateEvent{}
		event.ChannelID = dmChannelID

		// In a real scenario, ningen updates the read state.
		// We mock it so that ChannelIsUnread returns ChannelRead.
		m.onReadUpdate(event)

		if count := m.guildsTree.dmAlertCounts[dmChannelID]; count != 0 {
			t.Errorf("expected DM alert to be cleared, but count is %d", count)
		}
	})
}

func TestModel_onReadUpdate_DMUnreadWithoutMentionCreatesAlert(t *testing.T) {
	m := newMockChatModel()
	dmChannelID := discord.ChannelID(123)
	readMessageID := discord.MessageID(100)
	lastMessageID := discord.MessageID(999)
	dmChannel := discord.Channel{ID: dmChannelID, Type: discord.DirectMessage, LastMessageID: lastMessageID}
	m.state.Cabinet.ChannelStore.ChannelSet(&dmChannel, false)

	root := m.guildsTree.GetRoot()
	dmRoot := tview.NewTreeNode("Direct Messages").SetReference(dmNode{}).SetExpandable(true)
	root.ClearChildren().AddChild(dmRoot)
	m.guildsTree.dmRootNode = dmRoot

	m.state.ReadState.MarkRead(dmChannelID, readMessageID)
	m.onReadUpdate(&read.UpdateEvent{
		ReadState: gateway.ReadState{
			ChannelID:     dmChannelID,
			LastMessageID: readMessageID,
			MentionCount:  0,
		},
		Unread: true,
	})

	if count := m.guildsTree.dmAlertCounts[dmChannelID]; count != 1 {
		t.Fatalf("expected unread DM without mentions to surface a top alert, got %d", count)
	}

	children := root.GetChildren()
	if len(children) == 0 {
		t.Fatal("expected DM alert section to be rendered")
	}
	if ref, ok := children[0].GetReference().(dmAlertRef); !ok || ref.channelID != dmChannelID {
		t.Fatalf("expected unread DM alert to be the first top-level node, got %v", children[0].GetReference())
	}
}

func TestModel_onReady_DMAlertsHydrateFromUnreadState(t *testing.T) {
	m := newMockChatModel()
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
	})

	if count := m.guildsTree.dmAlertCounts[dmChannelID]; count != 1 {
		t.Fatalf("expected startup unread DM alert count 1, got %d", count)
	}

	children := m.guildsTree.GetRoot().GetChildren()
	if len(children) < 2 {
		t.Fatalf("expected top alert + dm root after READY hydration, got %d children", len(children))
	}
	if ref, ok := children[0].GetReference().(dmAlertRef); !ok || ref.channelID != dmChannelID {
		t.Fatalf("expected unread DM alert to be rendered first after READY hydration, got %v", children[0].GetReference())
	}
}

func TestModel_onMessageUpdate_Highlights(t *testing.T) {
	m := newMockChatModel()
	dmChannelID := discord.ChannelID(123)
	dmChannel := discord.Channel{ID: dmChannelID, Type: discord.DirectMessage}
	m.state.Cabinet.ChannelStore.ChannelSet(&dmChannel, false)

	t.Run("Update message in non-selected channel refreshes highlight style", func(t *testing.T) {
		m.SetSelectedChannel(nil)

		// Setup channel node in the tree to simulate expanded DM folder
		channelNode := tview.NewTreeNode("DM").SetReference(dmChannelID)
		m.guildsTree.channelNodeByID[dmChannelID] = channelNode

		event := &gateway.MessageUpdateEvent{
			Message: discord.Message{
				ID:        1,
				ChannelID: dmChannelID,
				Content:   "Updated content (e.g. embed added)",
			},
		}

		m.onMessageUpdate(event)

		if m.guildsTree.channelNodeByID[dmChannelID] != channelNode {
			t.Errorf("channelNode should still be in the tree")
		}
	})
}

func TestModel_onMessageCreate_SelectedDMWhenUnfocusedCreatesAlert(t *testing.T) {
	m := newMockChatModel()
	dmChannelID := discord.ChannelID(123)
	dmChannel := discord.Channel{ID: dmChannelID, Type: discord.DirectMessage}
	m.state.Cabinet.ChannelStore.ChannelSet(&dmChannel, false)
	m.SetSelectedChannel(&dmChannel)
	m.appFocused = false

	event := &gateway.MessageCreateEvent{
		Message: discord.Message{
			ID:        4,
			ChannelID: dmChannelID,
			Author:    discord.User{ID: 456, Username: "OtherUser"},
			Content:   "ping while unfocused",
		},
	}

	m.onMessageCreate(event)

	if count := m.guildsTree.dmAlertCounts[dmChannelID]; count != 1 {
		t.Fatalf("expected current DM to surface an alert while the app is unfocused, got %d", count)
	}
}
