package chat

import (
	"strings"
	"testing"
	"time"

	"github.com/ayn2op/tview"
	"github.com/ayn2op/tview/layers"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/gdamore/tcell/v3"
)

func TestMessageSearchShortcutOpensPopupAndFocusesInput(t *testing.T) {
	m := newTestModel()
	channel := &discord.Channel{ID: 200, GuildID: 100, Type: discord.GuildText, Name: "general"}
	m.SetSelectedChannel(channel)

	execCmdForTest(m.app, m.Update(tcell.NewEventKey(tcell.KeyCtrlF, "", tcell.ModNone)))

	if !m.HasLayer(messageSearchLayerName) {
		t.Fatal("expected message search layer to be visible")
	}
	if m.app.Focused() != m.messageSearch.input {
		t.Fatalf("expected focus on search input, got %T", m.app.Focused())
	}
}

func TestMessageSearchPopupSelectCurrentJumpsToMessage(t *testing.T) {
	m := newTestModel()
	channel := discord.Channel{ID: 200, GuildID: 100, Type: discord.GuildText, Name: "general"}
	m.SetSelectedChannel(&channel)

	sp := m.messageSearch
	sp.Prepare(channel, m.messageInput)
	sp.results = []messageSearchResult{{
		Message: discord.Message{
			ID:        300,
			ChannelID: channel.ID,
			GuildID:   channel.GuildID,
			Content:   "hello world",
			Timestamp: discord.NewTimestamp(time.Unix(0, 0)),
			Author:    discord.User{ID: 10, Username: "user"},
		},
	}}
	sp.list.SetCursor(0)

	var gotChannel discord.ChannelID
	var gotMessage discord.MessageID
	sp.jumpToMessage = func(got discord.Channel, messageID discord.MessageID) error {
		gotChannel = got.ID
		gotMessage = messageID
		return nil
	}

	m.AddLayer(sp, layers.WithName(messageSearchLayerName), layers.WithVisible(true), layers.WithOverlay())
	execCmdForTest(m.app, sp.selectCurrent())

	if gotChannel != channel.ID || gotMessage != 300 {
		t.Fatalf("expected jump to %v/%v, got %v/%v", channel.ID, discord.MessageID(300), gotChannel, gotMessage)
	}
	if m.HasLayer(messageSearchLayerName) {
		t.Fatal("expected search popup to close after selecting a result")
	}
	if m.app.Focused() != m.messagesList {
		t.Fatalf("expected focus to move to messages list, got %T", m.app.Focused())
	}
}

func TestPinnedMessagesPopupRendersPins(t *testing.T) {
	m := newTestModel()
	channel := discord.Channel{ID: 200, Type: discord.DirectMessage}
	pp := m.pinnedMessages
	pp.fetchPinnedMessages = func(got discord.Channel) ([]discord.Message, error) {
		if got.ID != channel.ID {
			t.Fatalf("expected channel %v, got %v", channel.ID, got.ID)
		}
		return []discord.Message{
			{ID: 301, ChannelID: channel.ID, Content: "first pinned message", Pinned: true, Author: discord.User{ID: 2, Username: "alice"}},
			{ID: 302, ChannelID: channel.ID, Content: "second pinned message", Pinned: true, Author: discord.User{ID: 3, Username: "bob"}},
		}, nil
	}

	pp.Prepare(channel, m.messageInput)
	if len(pp.pins) != 2 {
		t.Fatalf("expected 2 pins, got %d", len(pp.pins))
	}
	first := pp.buildItem(0, 0).(*tview.TextView)
	second := pp.buildItem(1, 0).(*tview.TextView)
	flat := linesText(append(first.GetLines(), second.GetLines()...))
	if !strings.Contains(flat, "first pinned message") || !strings.Contains(flat, "second pinned message") {
		t.Fatalf("expected pinned messages to render, got %q", flat)
	}
}

func TestMessagesListDrawReactions(t *testing.T) {
	m := newTestModel()
	builder := tview.NewLineBuilder()
	m.messagesList.drawReactions(builder, discord.Message{Reactions: []discord.Reaction{{
		Count: 5,
		Emoji: discord.Emoji{Name: "👍"},
	}}}, tcell.StyleDefault)

	got := linesText(builder.Finish())
	if !strings.Contains(got, "👍") || !strings.Contains(got, "5") {
		t.Fatalf("expected rendered reaction emoji and count, got %q", got)
	}
}

func TestExtractEmbedURLsIncludesRichMedia(t *testing.T) {
	urls := extractEmbedURLs([]discord.Embed{{
		URL:       "https://example.com",
		Image:     &discord.EmbedImage{URL: "https://example.com/image.png"},
		Thumbnail: &discord.EmbedThumbnail{URL: "https://example.com/thumb.png"},
		Video:     &discord.EmbedVideo{URL: "https://example.com/video.mp4"},
	}})
	got := strings.Join(urls, "\n")
	for _, want := range []string{"https://example.com", "https://example.com/image.png", "https://example.com/thumb.png", "https://example.com/video.mp4"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected embed URLs to include %q, got %q", want, got)
		}
	}
}
