package chat

import (
	"errors"
	"strings"
	"testing"

	"github.com/ayn2op/discordo/internal/config"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/diamondburned/ningen/v3"
)

func TestStateMessageHandlers(t *testing.T) {
	m := newTestModel()
	channelID := discord.ChannelID(123)
	m.SetSelectedChannel(&discord.Channel{ID: channelID, Type: discord.DirectMessage})

	m.messagesList.setMessages([]discord.Message{{ID: 1, ChannelID: channelID, Content: "before"}})
	m.onMessageUpdate(&gateway.MessageUpdateEvent{
		Message: discord.Message{ID: 1, ChannelID: channelID, Content: "after"},
	})
	if got := m.messagesList.messages[0].Content; got != "after" {
		t.Fatalf("expected updated message content, got %q", got)
	}

	m.onMessageDelete(&gateway.MessageDeleteEvent{ID: 1, ChannelID: channelID})
	if got := len(m.messagesList.messages); got != 0 {
		t.Fatalf("expected message to be deleted, got %d messages", got)
	}
}

func TestStateNotifyPathAndTyping(t *testing.T) {
	oldNotifyMessage := notifyMessage
	t.Cleanup(func() { notifyMessage = oldNotifyMessage })

	called := false
	notifyMessage = func(_ *ningen.State, message gateway.MessageCreateEvent, _ *config.Config) error {
		called = true
		if message.ChannelID != 456 {
			t.Fatalf("expected notify path for channel 456, got %v", message.ChannelID)
		}
		return nil
	}

	m := newTestModel()
	m.SetSelectedChannel(&discord.Channel{ID: 123, Type: discord.DirectMessage})
	cmd := m.onMessageCreate(&gateway.MessageCreateEvent{
		Message: discord.Message{ID: 1, ChannelID: 456, Author: discord.User{ID: 2}},
	})
	if cmd == nil {
		t.Fatal("expected notify branch to return a command")
	}
	if msg := cmd(); msg != nil {
		t.Fatalf("expected notify command to emit nil msg, got %T", msg)
	}
	if !called {
		t.Fatal("expected notify seam to be called")
	}

	m.state.Cabinet.MeStore.MyselfSet(discord.User{ID: 1, Username: "me"}, true)
	m.SetSelectedChannel(&discord.Channel{ID: 789, Type: discord.DirectMessage, DMRecipients: []discord.User{{ID: 2, Username: "other"}}})
	m.onTypingStart(&gateway.TypingStartEvent{ChannelID: 789, UserID: 2})
	if !strings.Contains(m.messagesList.GetFooter(), "other") {
		t.Fatalf("expected typing footer to mention user, got %q", m.messagesList.GetFooter())
	}
}

func TestStateNotifyErrorAndMemberInvalidation(t *testing.T) {
	oldNotifyMessage := notifyMessage
	t.Cleanup(func() { notifyMessage = oldNotifyMessage })
	notifyMessage = func(_ *ningen.State, _ gateway.MessageCreateEvent, _ *config.Config) error {
		return errors.New("notify fail")
	}

	m := newTestModel()
	m.SetSelectedChannel(&discord.Channel{ID: 123, Type: discord.DirectMessage})
	cmd := m.onMessageCreate(&gateway.MessageCreateEvent{
		Message: discord.Message{ID: 1, ChannelID: 456, Author: discord.User{ID: 2}},
	})
	if cmd == nil {
		t.Fatal("expected notify error path to return a command")
	}
	cmd()

	guildID := discord.GuildID(10)
	key := guildID.String() + " alice"
	m.messageInput.cache.Create(key, 50)
	m.onGuildMemberRemove(&gateway.GuildMemberRemoveEvent{
		GuildID: guildID,
		User:    discord.User{Username: "alice"},
	})
	if m.messageInput.cache.Exists(key) {
		t.Fatal("expected guild member removal to invalidate cache entry")
	}
}
