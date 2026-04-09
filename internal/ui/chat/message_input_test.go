package chat

import (
	"strings"
	"testing"

	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/gdamore/tcell/v3"
)

func TestMessageInputTabSuggestAndCompleteDM(t *testing.T) {
	m := newMockChatModel()
	mi := m.messageInput
	mi.SetDisabled(false)
	m.cfg.AutocompleteLimit = 10

	channel := &discord.Channel{
		ID:           10,
		Type:         discord.DirectMessage,
		DMRecipients: []discord.User{{ID: 2, Username: "bob"}, {ID: 3, Username: "alice"}},
	}
	m.SetSelectedChannel(channel)

	mi.SetText("@bo", true)
	mi.tabSuggest()
	if mi.mentionsList.itemCount() == 0 {
		t.Fatal("expected DM autocomplete suggestions")
	}

	mi.tabComplete()
	if got := mi.GetText(); got != "@bob " {
		t.Fatalf("expected tab completion to insert bob, got %q", got)
	}
	if mi.mentionsList.itemCount() != 0 {
		t.Fatal("expected tab completion to clear suggestions")
	}
}

func TestMessageInputTabSuggestAndCompleteEmoji(t *testing.T) {
	m := newMockChatModel()
	mi := m.messageInput
	mi.SetDisabled(false)
	m.cfg.AutocompleteLimit = 10

	channel := &discord.Channel{ID: 12, GuildID: 13, Type: discord.GuildText}
	m.SetSelectedChannel(channel)
	m.state.Cabinet.EmojiSet(channel.GuildID, []discord.Emoji{
		{ID: 1, Name: "mycustomemoji"},
		{ID: 2, Name: "mycustomemojianim", Animated: true},
	}, false)

	mi.SetText(":mycustom", true)
	mi.tabSuggest()
	if mi.mentionsList.itemCount() != 2 {
		t.Fatalf("expected emoji autocomplete suggestions, got %d", mi.mentionsList.itemCount())
	}

	mi.mentionsList.SetCursor(1)
	mi.tabComplete()
	if got := mi.GetText(); got != ":mycustomemojianim: " {
		t.Fatalf("expected emoji completion to insert emoji syntax, got %q", got)
	}
	if mi.mentionsList.itemCount() != 0 {
		t.Fatal("expected emoji tab completion to clear suggestions")
	}
}

func TestMessageInputProcessTextExpandsMentionsOutsideCode(t *testing.T) {
	m := newMockChatModel()
	mi := m.messageInput
	m.state.Cabinet.MeStore.MyselfSet(discord.User{ID: 1, Username: "me"}, true)

	channel := &discord.Channel{
		ID:           11,
		Type:         discord.DirectMessage,
		DMRecipients: []discord.User{{ID: 2, Username: "buddy"}},
	}

	got := mi.processText(channel, []byte("`@buddy` @buddy @me"))
	want := "`@buddy` " + discord.UserID(2).Mention() + " " + discord.UserID(1).Mention()
	if got != want {
		t.Fatalf("expected processed text %q, got %q", want, got)
	}
}

func TestMessageInputSendAndReset(t *testing.T) {
	transport := &mockTransport{}
	m := newTestModelWithTransport(transport)
	mi := m.messageInput
	mi.SetDisabled(false)

	channel := &discord.Channel{ID: 123, Type: discord.DirectMessage}
	m.SetSelectedChannel(channel)
	mi.SetText("hello world", true)

	mi.send()

	if !strings.HasSuffix(transport.path, "/channels/123/messages") {
		t.Fatalf("expected send request to target channel messages, got %q", transport.path)
	}
	if mi.GetText() != "" {
		t.Fatalf("expected input to reset after send, got %q", mi.GetText())
	}
}

func TestMessageInputCancelKeyClearsState(t *testing.T) {
	m := newMockChatModel()
	mi := m.messageInput
	mi.SetDisabled(false)

	mi.SetText("some text", true)
	mi.Update(tcell.NewEventKey(tcell.KeyEsc, "", tcell.ModNone))
	if mi.GetText() != "" {
		t.Fatalf("expected escape to clear text, got %q", mi.GetText())
	}
}
