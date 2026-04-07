package chat

import (
	"testing"

	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/gdamore/tcell/v3"
)

func TestMessageInputAddMentionHelpers(t *testing.T) {
	m := newTestModel()
	mi := m.messageInput

	mi.addMentionUser(nil)
	if mi.mentionsList.itemCount() != 0 {
		t.Fatal("expected nil user to be ignored")
	}

	user := &discord.User{ID: 2, Username: "buddy"}
	if err := m.state.Cabinet.PresenceSet(discord.NullGuildID, &discord.Presence{User: *user, Status: discord.OfflineStatus}, false); err != nil {
		t.Fatalf("presence set: %v", err)
	}
	mi.addMentionUser(user)
	if got := mi.mentionsList.itemCount(); got != 1 {
		t.Fatalf("expected one DM mention, got %d", got)
	}
	if attrs := mi.mentionsList.items[0].style.GetAttributes(); attrs&tcell.AttrDim == 0 {
		t.Fatal("expected offline DM mention to be dimmed")
	}

	guildID := discord.GuildID(20)
	member := &discord.Member{
		User: discord.User{ID: 3, Username: "guilduser"},
		Nick: "Guild User",
	}
	if mi.addMentionMember(guildID, member) {
		t.Fatal("expected a single member to stay below the autocomplete limit")
	}
	if got := mi.mentionsList.itemCount(); got != 2 {
		t.Fatalf("expected two mention entries total, got %d", got)
	}
}

func TestMessageInputShowMentionListAndHelp(t *testing.T) {
	m := newTestModel()
	mi := m.messageInput
	mi.SetDisabled(false)
	channel := &discord.Channel{ID: 80, GuildID: 81, Type: discord.GuildText}
	m.SetSelectedChannel(channel)
	setPermissionsForUser(m, channel.GuildID, channel, discord.User{ID: 1, Username: "me"}, discord.PermissionViewChannel|discord.PermissionAttachFiles)

	m.messagesList.SetRect(0, 0, 30, 8)
	mi.SetRect(4, 6, 10, 3)
	mi.mentionsList.append(mentionsListItem{insertText: "averylongname", displayText: "averylongname", style: tcell.StyleDefault})
	mi.mentionsList.rebuild()

	mi.showMentionList()
	if !m.GetVisible(mentionsListLayerName) {
		t.Fatal("expected showMentionList to show the mentions layer")
	}

	if !containsKeybind(mi.ShortHelp(), mi.cfg.Keybinds.MessageInput.OpenFilePicker.Keybind) {
		t.Fatal("expected attach-permitted help to include the file picker keybind")
	}
}

func TestMessageInputSearchMemberCacheShortCircuit(t *testing.T) {
	m := newTestModel()
	mi := m.messageInput
	guildID := discord.GuildID(90)

	exactKey := guildID.String() + " ab"
	mi.cache.Create(exactKey, 1)
	if cmd := mi.searchMember(guildID, "ab"); cmd != nil {
		t.Fatal("expected exact cache hit to skip live search")
	}

	prefixKey := guildID.String() + " a"
	mi.cache.Create(prefixKey, 0)
	if cmd := mi.searchMember(guildID, "ab"); cmd != nil {
		t.Fatal("expected prefix cache short-circuit to skip live search")
	}
	if !mi.cache.Exists(exactKey) {
		t.Fatal("expected prefix cache path to create the specific cache entry")
	}
}
