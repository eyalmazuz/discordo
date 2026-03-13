package chat

import (
	"testing"

	"github.com/diamondburned/arikawa/v3/discord"
)

func TestModel_OnMessageReactionAdd(t *testing.T) {
	m := newMockChatModel()
	m.messagesList.messages = []discord.Message{
		{ID: 1, ChannelID: 123},
	}
	m.SetSelectedChannel(&discord.Channel{ID: 123})

	// Setup state cabinet
	msg := &discord.Message{
		ID: 1, 
		ChannelID: 123, 
		Reactions: []discord.Reaction{{Count: 1, Emoji: discord.Emoji{Name: "👍"}}},
	}
	m.state.Cabinet.MessageStore.MessageSet(msg, false)

	// In a real scenario, ningen updates the cabinet.
	// We just want to ensure that our handler (if called) would trigger an update.
	// Since we can't easily test QueueUpdateDraw, we verify the logic of setMessage.
	m.messagesList.setMessage(0, *msg)

	if len(m.messagesList.messages[0].Reactions) == 0 {
		t.Errorf("expected reaction to be updated in messagesList")
	}
	if m.messagesList.messages[0].Reactions[0].Count != 1 {
		t.Errorf("expected reaction count 1, got %d", m.messagesList.messages[0].Reactions[0].Count)
	}
}

func TestModel_OnMessageReactionRemove(t *testing.T) {
	m := newMockChatModel()
	m.messagesList.messages = []discord.Message{
		{
			ID: 1, 
			ChannelID: 123, 
			Reactions: []discord.Reaction{{Count: 1, Emoji: discord.Emoji{Name: "👍"}}},
		},
	}
	m.SetSelectedChannel(&discord.Channel{ID: 123})

	// Setup state cabinet (reaction removed)
	msg := &discord.Message{
		ID: 1, 
		ChannelID: 123, 
		Reactions: []discord.Reaction{},
	}
	m.state.Cabinet.MessageStore.MessageSet(msg, false)

	// Manually call setMessage to verify logic
	m.messagesList.setMessage(0, *msg)

	if len(m.messagesList.messages[0].Reactions) != 0 {
		t.Errorf("expected reaction to be removed from messagesList")
	}
}
