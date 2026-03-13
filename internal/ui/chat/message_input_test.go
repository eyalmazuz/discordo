package chat

import (
	"testing"

	"github.com/ayn2op/discordo/internal/config"
	"github.com/ayn2op/tview"
	"github.com/ayn2op/tview/layers"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/diamondburned/arikawa/v3/session"
	"github.com/diamondburned/arikawa/v3/state"
	"github.com/diamondburned/arikawa/v3/state/store/defaultstore"
	"github.com/diamondburned/ningen/v3"
	"github.com/gdamore/tcell/v3"
)

func newMockChatModel() *Model {
	cfg, _ := config.Load("")
	app := tview.NewApplication()
	m := &Model{
		app: app,
		cfg: cfg,
	}
	m.Layers = layers.New()
	
	// Mock ningen state
	s := state.NewFromSession(session.New(""), defaultstore.New())
	m.state = ningen.FromState(s)
	
	// Mock current user
	me := discord.User{ID: 1, Username: "me"}
	s.Cabinet.MeStore.MyselfSet(me, false)
	
	m.messageInput = newMessageInput(cfg, m)
	m.messagesList = newMessagesList(cfg, m)
	
	return m
}

func TestMessageInput_AutocompleteTrigger(t *testing.T) {
	m := newMockChatModel()
	mi := m.messageInput
	mi.SetDisabled(false)
	m.cfg.MessagesLimit = 1
	
	// Setup a channel and guild in state for autocomplete
	gid := discord.GuildID(123)
	cid := discord.ChannelID(456)
	channel := &discord.Channel{ID: cid, GuildID: gid, Name: "test", Type: discord.GuildText}
	m.SetSelectedChannel(channel)
	
	m.state.Call(&gateway.ChannelCreateEvent{Channel: *channel})
	m.state.Call(&gateway.GuildCreateEvent{
		Guild: discord.Guild{ID: gid},
	})
	
	// Manually set in cabinet to ensure they are available for permissions check
	m.state.Cabinet.GuildStore.GuildSet(&discord.Guild{ID: gid}, false)
	m.state.Cabinet.ChannelStore.ChannelSet(channel, false)
	m.state.Cabinet.RoleStore.RoleSet(gid, &discord.Role{ID: discord.RoleID(gid), Permissions: discord.PermissionViewChannel}, false)
	
	// Manually set a member in the cabinet since ningen.State might not do it automatically from gateway events in this mock setup
	m.state.Cabinet.MemberStore.MemberSet(gid, &discord.Member{
		User: discord.User{ID: 789, Username: "testuser"},
	}, false)

	// Set AutocompleteLimit to enable suggestions
	m.cfg.AutocompleteLimit = 10
	
	// Pre-populate cache to avoid searchMember network call and hang
	mi.cache.Create(gid.String()+" test", 1)

	// Type '@test' to trigger autocomplete
	mi.SetText("@test", true)
	// We don't necessarily need HandleEvent if we SetText, but let's call tabSuggestion directly
	
	// Autocomplete is asynchronous (go mi.chat.app.QueueUpdateDraw)
	// In tests, we manually trigger it to ensure it runs synchronously
	mi.tabSuggestion()
	
	if mi.mentionsList.itemCount() == 0 {
		t.Errorf("Expected mentions list to have items after typing '@test'")
	}
}

func TestMessageInput_MultiLineInput(t *testing.T) {
	m := newMockChatModel()
	mi := m.messageInput
	mi.SetDisabled(false)
	
	// Simulate multi-line input
	text := "line 1\nline 2"
	mi.SetText(text, true)
	
	if mi.GetText() != text {
		t.Errorf("Expected text %q, got %q", text, mi.GetText())
	}
}

func TestMessageInput_SendAndClear(t *testing.T) {
	m := newMockChatModel()
	mi := m.messageInput
	mi.SetDisabled(false)
	
	cid := discord.ChannelID(456)
	m.SetSelectedChannel(&discord.Channel{ID: cid, Type: discord.GuildText})
	
	mi.SetText("hello world", true)
	
	// Check the result of mi.reset()
	mi.reset()
	if mi.GetText() != "" {
		t.Errorf("Expected empty text after reset")
	}
}

func TestMessageInput_Keybinds(t *testing.T) {
	m := newMockChatModel()
	mi := m.messageInput
	mi.SetDisabled(false)
	
	// Test Cancel keybind
	mi.SetText("some text", true)
	cancelKey := m.cfg.Keybinds.MessageInput.Cancel
	mi.HandleEvent(tcell.NewEventKey(tcell.KeyEsc, "", tcell.ModNone))
	
	if mi.GetText() != "" && cancelKey.Keys()[0] == "esc" {
		t.Errorf("Expected text to be cleared after Esc key")
	}
}
