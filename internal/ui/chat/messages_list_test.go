package chat

import (
	"slices"
	"strings"
	"testing"

	"github.com/ayn2op/discordo/internal/config"
	"github.com/ayn2op/discordo/internal/markdown"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/eyalmazuz/tview"
	"github.com/eyalmazuz/tview/list"
	"github.com/gdamore/tcell/v3"
)

func TestExtractURLs(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected []string
	}{
		{name: "Simple URL", content: "Check this out: https://example.com", expected: []string{"https://example.com"}},
		{name: "Markdown Link", content: "Click [here](https://example.com/markdown)", expected: []string{"https://example.com/markdown"}},
		{name: "Multiple URLs", content: "Check https://a.com and https://b.com", expected: []string{"https://a.com", "https://b.com"}},
		{name: "No URLs", content: "Just some plain text without links.", expected: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractURLs(tt.content)
			if !slices.Equal(got, tt.expected) {
				t.Fatalf("extractURLs(%q) = %#v, want %#v", tt.content, got, tt.expected)
			}
		})
	}
}

func TestMessageURLsDeduplication(t *testing.T) {
	msg := discord.Message{
		Content: "https://dup.com and [dup](https://dup.com)",
		Embeds:  []discord.Embed{{URL: "https://dup.com"}},
	}

	got := messageURLs(msg)
	want := []string{"https://dup.com"}
	if !slices.Equal(got, want) {
		t.Fatalf("messageURLs() = %#v, want %#v", got, want)
	}
}

func TestMessagesListUpdateReactKeyOpensPicker(t *testing.T) {
	m := newMockChatModel()
	guildID := discord.GuildID(100)
	channelID := discord.ChannelID(200)

	m.SetSelectedChannel(&discord.Channel{ID: channelID, GuildID: guildID, Type: discord.GuildText})
	m.state.Cabinet.GuildStore.GuildSet(&discord.Guild{ID: guildID, Name: "guild"}, false)
	m.state.Cabinet.EmojiSet(guildID, []discord.Emoji{{ID: 123456, Name: "kekw"}}, false)

	m.messagesList.setMessages([]discord.Message{
		{ID: 1, ChannelID: channelID, GuildID: guildID, Author: discord.User{ID: 2, Username: "user"}},
	})
	m.messagesList.SetCursor(0)

	m.messagesList.Update(tcell.NewEventKey(tcell.KeyRune, "+", tcell.ModNone))

	if !m.HasLayer(reactionPickerLayerName) {
		t.Fatal("expected reaction picker layer to be visible")
	}
	if m.app.Focused() == m.messagesList {
		t.Fatalf("expected focus to move into the reaction picker, got %T", m.app.Focused())
	}
}

func TestMessagesListDrawReactions(t *testing.T) {
	cfg, _ := config.Load("")
	ml := &messagesList{cfg: cfg}

	tests := []struct {
		name         string
		inlineImages bool
		reactions    []discord.Reaction
		expected     []struct {
			emoji string
			count string
		}
	}{
		{
			name: "Unicode Reaction",
			reactions: []discord.Reaction{{
				Count: 5,
				Emoji: discord.Emoji{Name: "👍"},
			}},
			expected: []struct {
				emoji string
				count string
			}{{emoji: "👍", count: "5"}},
		},
		{
			name:         "Custom Emoji Reaction Inline Images",
			inlineImages: true,
			reactions: []discord.Reaction{{
				Count: 7,
				Emoji: discord.Emoji{ID: 123456, Name: "custom"},
			}},
			expected: []struct {
				emoji string
				count string
			}{{emoji: markdown.CustomEmojiText("custom", true), count: "7"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ml.cfg.InlineImages.Enabled = tt.inlineImages
			builder := tview.NewLineBuilder()
			ml.drawReactions(builder, discord.Message{Reactions: tt.reactions}, tcell.StyleDefault)
			lines := builder.Finish()
			got := joinedLinesText(lines)
			for _, expected := range tt.expected {
				if !strings.Contains(got, expected.emoji) || !strings.Contains(got, expected.count) {
					t.Fatalf("expected rendered reactions to contain %q and %q, got %q", expected.emoji, expected.count, got)
				}
			}
		})
	}
}

func TestMessagesListUpdateEnterKey(t *testing.T) {
	cfg, _ := config.Load("")
	ml := &messagesList{
		Model: list.NewModel(),
		cfg:   cfg,
	}

	ml.Update(tcell.NewEventKey(tcell.KeyEnter, "", tcell.ModNone))
}

func joinedLineText(line tview.Line) string {
	var b strings.Builder
	for _, segment := range line {
		b.WriteString(segment.Text)
	}
	return b.String()
}
