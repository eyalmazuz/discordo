package chat

import (
	"testing"
	"time"

	"github.com/eyalmazuz/tview"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/diamondburned/ningen/v3/states/read"
	"github.com/gdamore/tcell/v3"
)

func TestMessagesList_drawForwardedMessage_Parity(t *testing.T) {
	m := newTestModel()
	ml := m.messagesList
	builder := tview.NewLineBuilder()
	baseStyle := tcell.StyleDefault

	msg := discord.Message{
		ID:        1,
		Timestamp: discord.NewTimestamp(time.Now()),
		Author:    discord.User{Username: "sender"},
		MessageSnapshots: []discord.MessageSnapshot{
			{
				Message: discord.MessageSnapshotMessage{
					Content:   "forwarded content",
					Timestamp: discord.NewTimestamp(time.Now().Add(-time.Hour)),
				},
			},
		},
	}

	// This should cover drawForwardedMessage and drawSnapshotContent
	ml.drawForwardedMessage(builder, msg, baseStyle)
}

func TestMessagesList_SubscriptionMessages_Parity(t *testing.T) {
	m := newTestModel()
	ml := m.messagesList
	builder := tview.NewLineBuilder()
	baseStyle := tcell.StyleDefault

	subTypes := []discord.MessageType{
		discord.NitroBoostMessage,
		discord.NitroTier1Message,
		discord.NitroTier2Message,
		discord.NitroTier3Message,
	}

	for _, mt := range subTypes {
		msg := discord.Message{
			Type:   mt,
			Author: discord.User{Username: "booster"},
		}
		ml.writeMessage(builder, msg, baseStyle)
	}
}

func TestMessagesList_PermissionGatedActions_Parity(t *testing.T) {
	m := newTestModel()
	ml := m.messagesList
	
	t.Run("DeleteOthersMessage_WithoutPermission", func(t *testing.T) {
		m.state.Cabinet.MeStore.MyselfSet(discord.User{ID: 1}, false)
		msg := discord.Message{
			ID:        100,
			Author:    discord.User{ID: 2}, // Not me
			GuildID:   10,
			ChannelID: 20,
		}
		
		ml.messages = []discord.Message{msg}
		ml.rebuildRows()
		ml.SetCursor(0)
		
		// Attempt to delete. Should fail permission check.
		ml.delete()
		// Since we can't easily check slog output, we just ensure no panic.
	})

	t.Run("EditOthersMessage_AlwaysFails", func(t *testing.T) {
		m.state.Cabinet.MeStore.MyselfSet(discord.User{ID: 1}, false)
		msg := discord.Message{
			ID:        101,
			Author:    discord.User{ID: 2}, // Not me
			GuildID:   10,
			ChannelID: 20,
		}
		
		ml.messages = []discord.Message{msg}
		ml.rebuildRows()
		ml.SetCursor(0)
		
		ml.edit()
		if m.app.Focused() == m.messageInput {
			t.Errorf("Expected focus NOT to move to message input for someone else's message")
		}
	})
}

func TestModel_onReadUpdate_Parity(t *testing.T) {
	m := newTestModel()
	gt := m.guildsTree
	
	guildID := discord.GuildID(10)
	chanID := discord.ChannelID(20)
	
	// Create nodes
	gn := tview.NewTreeNode("Guild").SetReference(guildID)
	cn := tview.NewTreeNode("Channel").SetReference(chanID)
	gt.GetRoot().AddChild(gn)
	gn.AddChild(cn)
	
	gt.guildNodeByID[guildID] = gn
	gt.channelNodeByID[chanID] = cn
	
	// Mock ningen state for unread check
	// Ningen state methods like GuildIsUnread are hard to mock without full state control,
	// but we can at least ensure the handler executes and calls style methods.
	m.onReadUpdate(&read.UpdateEvent{
		ReadState: gateway.ReadState{ChannelID: chanID},
		GuildID:   guildID,
	})
}
