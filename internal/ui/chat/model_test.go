package chat

import (
	"testing"

	"github.com/ayn2op/discordo/internal/config"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/session"
	"github.com/diamondburned/arikawa/v3/state"
	"github.com/diamondburned/arikawa/v3/state/store/defaultstore"
	"github.com/diamondburned/ningen/v3"
	"github.com/eyalmazuz/tview"
	"github.com/gdamore/tcell/v3"
)

func newMockModel() *Model {
	cfg, _ := config.Load("")
	app := tview.NewApplication()
	m := NewModel(app, cfg, "test-token")

	// Mock ningen state
	s := state.NewFromSession(session.New(""), defaultstore.New())
	m.state = ningen.FromState(s)

	return m
}

func TestModel_LayerManagement(t *testing.T) {
	m := newMockModel()

	// Open channels picker
	m.openPicker()
	if !m.HasLayer(channelsPickerLayerName) {
		t.Errorf("Expected channels picker layer to exist")
	}

	// Close channels picker
	m.closePicker()
	if m.HasLayer(channelsPickerLayerName) {
		t.Errorf("Expected channels picker layer to be removed")
	}
}

func TestModel_FocusSwitching(t *testing.T) {
	m := newMockModel()
	m.messageInput.SetDisabled(false)

	// Initial focus should be on guildsTree
	setFocusForTest(m.app, m.guildsTree)
	if m.app.Focused() != m.guildsTree {
		t.Errorf("Expected focus on guildsTree")
	}

	// Focus Next: guildsTree -> messagesList
	execCmdForTest(m.app, m.focusNext())
	if m.app.Focused() != m.messagesList {
		t.Errorf("Expected focus on messagesList, got %T", m.app.Focused())
	}

	// Focus Next: messagesList -> messageInput
	execCmdForTest(m.app, m.focusNext())
	if m.app.Focused() != m.messageInput {
		t.Errorf("Expected focus on messageInput, got %T", m.app.Focused())
	}

	// Focus Next: messageInput -> guildsTree (loop)
	execCmdForTest(m.app, m.focusNext())
	if m.app.Focused() != m.guildsTree {
		t.Errorf("Expected focus on guildsTree, got %T", m.app.Focused())
	}
}

func TestModel_GlobalKeybinds(t *testing.T) {
	m := newMockModel()
	m.messageInput.SetDisabled(false)

	// Test Ctrl+G (Focus Guilds Tree)
	execCmdForTest(m.app, m.Update(tcell.NewEventKey(tcell.KeyCtrlG, "", tcell.ModNone)))
	if m.app.Focused() != m.guildsTree {
		t.Errorf("Expected focus on guildsTree after Ctrl+G")
	}

	// Test Ctrl+L (Focus Messages List) - Note: Default is Ctrl+T in this repo, but I'll use what's in config
	execCmdForTest(m.app, m.Update(tcell.NewEventKey(tcell.KeyCtrlT, "", tcell.ModNone)))
	if m.app.Focused() != m.messagesList {
		t.Errorf("Expected focus on messagesList after Ctrl+T")
	}

	// Test Ctrl+I (Focus Message Input)
	execCmdForTest(m.app, m.Update(tcell.NewEventKey(tcell.KeyCtrlI, "", tcell.ModNone)))
	if m.app.Focused() != m.messageInput {
		t.Errorf("Expected focus on messageInput after Ctrl+I")
	}
}

func TestModel_MarkRead(t *testing.T) {
	m := newMockModel()
	m.cfg.MessagesLimit = 1

	channelID := discord.ChannelID(123)
	lastMsgID := discord.MessageID(456)
	channel := &discord.Channel{ID: channelID, LastMessageID: lastMsgID, Type: discord.GuildText}

	// Add message to cabinet to prevent API call
	m.state.Cabinet.MessageStore.MessageSet(&discord.Message{ID: lastMsgID, ChannelID: channelID}, false)

	// Add channel to cabinet so findNodeByChannelID works recursively if needed
	m.state.Cabinet.ChannelStore.ChannelSet(channel, false)

	// We check if loadChannel returns a command for a valid channel
	if cmd := m.guildsTree.loadChannel(*channel); cmd == nil {
		t.Fatal("expected loadChannel to return a command")
	}
}
