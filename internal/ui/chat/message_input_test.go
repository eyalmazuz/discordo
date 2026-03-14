package chat

import (
	"testing"

	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/gdamore/tcell/v3"
)

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

func TestReplaceEmojis(t *testing.T) {
	emojis := []discord.Emoji{
		{ID: 123456, Name: "kekw"},
		{ID: 789012, Name: "pepe", Animated: true},
	}

	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "known emoji is expanded",
			src:  "hello :kekw: world",
			want: "hello <:kekw:123456> world",
		},
		{
			name: "animated emoji uses a: prefix",
			src:  "nice :pepe:",
			want: "nice <a:pepe:789012>",
		},
		{
			name: "unknown emoji left alone",
			src:  ":unknown: stays",
			want: ":unknown: stays",
		},
		{
			name: "no emojis in text",
			src:  "just plain text",
			want: "just plain text",
		},
		{
			name: "multiple emojis",
			src:  ":kekw: and :pepe:",
			want: "<:kekw:123456> and <a:pepe:789012>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := string(replaceEmojis(emojis, []byte(tt.src)))
			if got != tt.want {
				t.Errorf("replaceEmojis() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestReplaceEmojis_NoGuildEmojis(t *testing.T) {
	src := ":kekw: stays as is"
	got := string(replaceEmojis(nil, []byte(src)))
	if got != src {
		t.Errorf("replaceEmojis(nil) = %q, want %q", got, src)
	}
}

func TestProcessText_EmojiInCodeBlock(t *testing.T) {
	m := newMockChatModel()
	mi := m.messageInput

	gid := discord.GuildID(100)
	cid := discord.ChannelID(200)
	channel := &discord.Channel{ID: cid, GuildID: gid, Type: discord.GuildText}

	// Seed emojis into the state cabinet.
	m.state.Cabinet.EmojiSet(gid, []discord.Emoji{
		{ID: 123456, Name: "kekw"},
	}, false)

	// Emoji inside a code block should NOT be expanded.
	got := mi.processText(channel, []byte("look: `:kekw:`"))
	if got != "look: `:kekw:`" {
		t.Errorf("processText code block = %q, want emoji left alone", got)
	}

	// Emoji outside code block should be expanded.
	got = mi.processText(channel, []byte(":kekw:"))
	if got != "<:kekw:123456>" {
		t.Errorf("processText plain = %q, want <:kekw:123456>", got)
	}
}

func TestProcessText_EmojiAvailableInDMWithNitro(t *testing.T) {
	m := newMockChatModel()
	mi := m.messageInput

	m.state.Cabinet.MeStore.MyselfSet(discord.User{
		ID:       1,
		Username: "me",
		Nitro:    discord.NitroFull,
	}, true)

	guildID := discord.GuildID(100)
	dmChannel := &discord.Channel{ID: 200, Type: discord.DirectMessage}
	m.state.Cabinet.GuildStore.GuildSet(&discord.Guild{ID: guildID, Name: "guild"}, false)
	m.state.Cabinet.EmojiSet(guildID, []discord.Emoji{
		{ID: 123456, Name: "kekw"},
	}, false)

	got := mi.processText(dmChannel, []byte(":kekw:"))
	if got != "<:kekw:123456>" {
		t.Errorf("processText dm = %q, want <:kekw:123456>", got)
	}
}

func TestProcessText_EmojiFromOtherGuildWithNitro(t *testing.T) {
	m := newMockChatModel()
	mi := m.messageInput

	m.state.Cabinet.MeStore.MyselfSet(discord.User{
		ID:       1,
		Username: "me",
		Nitro:    discord.NitroFull,
	}, true)

	currentGuildID := discord.GuildID(100)
	otherGuildID := discord.GuildID(101)
	channel := &discord.Channel{ID: 200, GuildID: currentGuildID, Type: discord.GuildText}

	m.state.Cabinet.GuildStore.GuildSet(&discord.Guild{ID: currentGuildID, Name: "current"}, false)
	m.state.Cabinet.GuildStore.GuildSet(&discord.Guild{ID: otherGuildID, Name: "other"}, false)
	m.state.Cabinet.EmojiSet(currentGuildID, []discord.Emoji{}, false)
	m.state.Cabinet.EmojiSet(otherGuildID, []discord.Emoji{
		{ID: 123456, Name: "kekw"},
	}, false)

	got := mi.processText(channel, []byte(":kekw:"))
	if got != "<:kekw:123456>" {
		t.Errorf("processText cross-guild = %q, want <:kekw:123456>", got)
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
