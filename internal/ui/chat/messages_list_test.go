package chat

import (
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ayn2op/discordo/internal/config"
	"github.com/ayn2op/discordo/internal/markdown"
	"github.com/ayn2op/tview"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/gdamore/tcell/v3"
)

func TestExtractURLs(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected []string
	}{
		{
			name:     "Simple URL",
			content:  "Check this out: https://example.com",
			expected: []string{"https://example.com"},
		},
		{
			name:     "Markdown Link",
			content:  "Click [here](https://example.com/markdown)",
			expected: []string{"https://example.com/markdown"},
		},
		{
			name:     "Multiple URLs",
			content:  "Check https://a.com and https://b.com",
			expected: []string{"https://a.com", "https://b.com"},
		},
		{
			name:     "Nested Link in content",
			content:  "Some text [link](https://nested.com) more text",
			expected: []string{"https://nested.com"},
		},
		{
			name:     "Autolink with brackets",
			content:  "Look at <https://auto.com>",
			expected: []string{"https://auto.com"},
		},
		{
			name:     "No URLs",
			content:  "Just some plain text without links.",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractURLs(tt.content)
			if len(got) != len(tt.expected) {
				t.Fatalf("expected %d URLs, got %d", len(tt.expected), len(got))
			}
			for i := range got {
				if got[i] != tt.expected[i] {
					t.Errorf("expected URL %d to be %q, got %q", i, tt.expected[i], got[i])
				}
			}
		})
	}
}

func TestExtractEmbedURLs(t *testing.T) {
	embeds := []discord.Embed{
		{
			URL: "https://embed-main.com",
			Image: &discord.EmbedImage{
				URL: "https://embed-image.com",
			},
			Video: &discord.EmbedVideo{
				URL: "https://embed-video.com",
			},
		},
	}

	expected := []string{
		"https://embed-main.com",
		"https://embed-image.com",
		"https://embed-video.com",
	}

	got := extractEmbedURLs(embeds)
	if len(got) != len(expected) {
		t.Fatalf("expected %d URLs, got %d", len(expected), len(got))
	}
	for i := range got {
		if got[i] != expected[i] {
			t.Errorf("expected URL %d to be %q, got %q", i, expected[i], got[i])
		}
	}
}

func TestMessageURLs_Deduplication(t *testing.T) {
	msg := discord.Message{
		Content: "https://dup.com and [dup](https://dup.com)",
		Embeds: []discord.Embed{
			{URL: "https://dup.com"},
		},
	}

	expected := []string{"https://dup.com"}
	got := messageURLs(msg)

	if len(got) != len(expected) {
		t.Fatalf("expected %d unique URL, got %d", len(expected), len(got))
	}
	if got[0] != expected[0] {
		t.Errorf("expected %q, got %q", expected[0], got[0])
	}
}

func TestMessagesList_HandleEvent_EnterKey(t *testing.T) {
	cfg, _ := config.Load("")
	ml := &messagesList{
		List: tview.NewList(),
		cfg:  cfg,
	}

	event := tcell.NewEventKey(tcell.KeyEnter, " ", tcell.ModNone)

	// HandleEvent should return RedrawCommand for Enter key
	cmd := ml.HandleEvent(event)
	if _, ok := cmd.(tview.RedrawCommand); !ok {
		t.Errorf("expected RedrawCommand for Enter key")
	}
}

func TestMessagesList_HandleEvent_ReactKeyOpensPicker(t *testing.T) {
	m := newMockChatModel()
	guildID := discord.GuildID(100)
	channelID := discord.ChannelID(200)

	m.SetSelectedChannel(&discord.Channel{ID: channelID, GuildID: guildID, Type: discord.GuildText})
	m.state.Cabinet.GuildStore.GuildSet(&discord.Guild{ID: guildID, Name: "guild"}, false)
	m.state.Cabinet.EmojiSet(guildID, []discord.Emoji{
		{ID: 123456, Name: "kekw"},
	}, false)

	m.messagesList.setMessages([]discord.Message{
		{ID: 1, ChannelID: channelID, GuildID: guildID, Author: discord.User{ID: 2, Username: "user"}},
	})
	m.messagesList.SetCursor(0)

	cmd := m.messagesList.HandleEvent(tcell.NewEventKey(tcell.KeyRune, "+", tcell.ModNone))
	if _, ok := cmd.(tview.RedrawCommand); !ok {
		t.Fatalf("expected RedrawCommand for react key, got %T", cmd)
	}

	if !m.HasLayer(reactionPickerLayerName) {
		t.Fatal("expected reaction picker layer to be visible")
	}

	if m.app.GetFocus() == m.messagesList {
		t.Fatalf("expected focus to move into the reaction picker, got %T", m.app.GetFocus())
	}
}

func TestMessagesList_DrawReactions(t *testing.T) {
	cfg, _ := config.Load("")
	ml := &messagesList{
		cfg: cfg,
	}

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
			reactions: []discord.Reaction{
				{
					Count: 5,
					Me:    false,
					Emoji: discord.Emoji{Name: "👍"},
				},
			},
			expected: []struct {
				emoji string
				count string
			}{{emoji: "👍", count: "5"}},
		},
		{
			name: "Custom Emoji Reaction",
			reactions: []discord.Reaction{
				{
					Count: 10,
					Me:    true,
					Emoji: discord.Emoji{ID: 123456, Name: "custom"},
				},
			},
			expected: []struct {
				emoji string
				count string
			}{{emoji: ":custom:", count: "10"}},
		},
		{
			name:         "Custom Emoji Reaction Inline Images",
			inlineImages: true,
			reactions: []discord.Reaction{
				{
					Count: 7,
					Me:    false,
					Emoji: discord.Emoji{ID: 123456, Name: "custom"},
				},
			},
			expected: []struct {
				emoji string
				count string
			}{{emoji: markdown.CustomEmojiText("custom", true), count: "7"}},
		},
		{
			name: "Multiple Reactions",
			reactions: []discord.Reaction{
				{
					Count: 1,
					Me:    false,
					Emoji: discord.Emoji{Name: "😀"},
				},
				{
					Count: 2,
					Me:    true,
					Emoji: discord.Emoji{Name: "🚀"},
				},
			},
			expected: []struct {
				emoji string
				count string
			}{
				{emoji: "😀", count: "1"},
				{emoji: "🚀", count: "2"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg.InlineImages.Enabled = tt.inlineImages
			builder := tview.NewLineBuilder()
			message := discord.Message{
				Reactions: tt.reactions,
			}

			ml.drawReactions(builder, message, tcell.StyleDefault)
			lines := builder.Finish()

			if len(tt.reactions) > 0 && len(lines) == 0 {
				t.Fatal("expected at least one line for reactions")
			}

			for i, r := range tt.reactions {
				emojiFound := false
				countFound := false
				expected := tt.expected[i]
				for _, line := range lines {
					for _, segment := range line {
						if strings.Contains(segment.Text, expected.emoji) {
							emojiFound = true
							if segment.Style.HasReverse() {
								t.Errorf("reaction %q should not use reverse style", expected.emoji)
							}
							if r.Me && !segment.Style.HasBold() {
								t.Errorf("reaction %q with Me=true should have bold style", expected.emoji)
							} else if !r.Me && segment.Style.HasBold() {
								t.Errorf("reaction %q with Me=false should NOT have bold style", expected.emoji)
							}
						}
						if strings.Contains(segment.Text, expected.count) {
							countFound = true
							if segment.Style.HasReverse() {
								t.Errorf("count %q should not use reverse style", expected.count)
							}
							if r.Me && !segment.Style.HasBold() {
								t.Errorf("count %q with Me=true should have bold style", expected.count)
							} else if !r.Me && segment.Style.HasBold() {
								t.Errorf("count %q with Me=false should NOT have bold style", expected.count)
							}
						}
					}
				}
				if !emojiFound {
					t.Errorf("expected reaction emoji %q not found", expected.emoji)
				}
				if !countFound {
					t.Errorf("expected reaction count %q not found", expected.count)
				}
			}
		})
	}
}

func TestMessagesList_DrawReactions_CustomEmojiURLOnlyOnEmoji(t *testing.T) {
	cfg, _ := config.Load("")
	cfg.InlineImages.Enabled = true
	ml := &messagesList{
		cfg: cfg,
	}

	builder := tview.NewLineBuilder()
	message := discord.Message{
		Reactions: []discord.Reaction{
			{
				Count: 1,
				Emoji: discord.Emoji{ID: 123456, Name: "kekw"},
			},
		},
	}

	ml.drawReactions(builder, message, tcell.StyleDefault)
	lines := builder.Finish()

	var emojiSegmentFound bool
	var countSegmentFound bool
	for _, line := range lines {
		for _, segment := range line {
			_, url := segment.Style.GetUrl()
			switch {
			case segment.Text == markdown.CustomEmojiText("kekw", true):
				emojiSegmentFound = true
				if url != message.Reactions[0].Emoji.EmojiURL() {
					t.Fatalf("expected emoji segment URL %q, got %q", message.Reactions[0].Emoji.EmojiURL(), url)
				}
			case strings.Contains(segment.Text, "1"):
				countSegmentFound = true
				if url != "" {
					t.Fatalf("expected count segment to have no URL, got %q", url)
				}
			}
		}
	}

	if !emojiSegmentFound {
		t.Fatal("expected to find custom emoji segment")
	}
	if !countSegmentFound {
		t.Fatal("expected to find reaction count segment")
	}
}

func TestReactionPicker_ItemForEmoji(t *testing.T) {
	cfg, _ := config.Load("")
	m := newMockChatModel()
	rp := newReactionPicker(cfg, m, m.messagesList, m.messagesList.imageCache)

	emoji := discord.Emoji{ID: 123456, Name: "kekw"}
	preview := rp.previewItem(emoji)
	if preview == nil {
		t.Fatal("expected preview item for custom emoji")
	}

	if preview.maxW != reactionPickerEmojiWidth || preview.maxH != reactionPickerEmojiHeight {
		t.Fatalf("expected preview dimensions %dx%d, got %dx%d", reactionPickerEmojiWidth, reactionPickerEmojiHeight, preview.maxW, preview.maxH)
	}

	if preview.url != emoji.EmojiURL() {
		t.Fatalf("expected preview URL %q, got %q", emoji.EmojiURL(), preview.url)
	}

	line := rp.lineForEmoji(emoji)
	if len(line) != 2 {
		t.Fatalf("expected 2 line segments, got %d", len(line))
	}
	if line[0].Text != markdown.CustomEmojiText(emoji.Name, cfg.InlineImages.Enabled) {
		t.Fatalf("expected placeholder text %q, got %q", markdown.CustomEmojiText(emoji.Name, cfg.InlineImages.Enabled), line[0].Text)
	}
	if _, url := line[0].Style.GetUrl(); url != emoji.EmojiURL() {
		t.Fatalf("expected emoji segment URL %q, got %q", emoji.EmojiURL(), url)
	}
	if line[1].Text != " "+emoji.Name {
		t.Fatalf("expected label %q, got %q", " "+emoji.Name, line[1].Text)
	}
}

func TestReactionPicker_PrepareKittyItemsForFrame(t *testing.T) {
	cfg, _ := config.Load("")
	cfg.InlineImages.Enabled = true
	m := newMockChatModel()
	m.messagesList.useKitty = true
	m.messagesList.cellW = 10
	m.messagesList.cellH = 20

	rp := newReactionPicker(cfg, m, m.messagesList, m.messagesList.imageCache)
	emoji := discord.Emoji{ID: 123456, Name: "kekw"}
	preview := rp.previewItem(emoji)
	if preview == nil {
		t.Fatal("expected preview item for custom emoji")
	}

	preview.kittyPlaced = true
	preview.kittyUploaded = true
	preview.pendingPlace = true

	rp.prepareKittyItemsForFrame(&MockScreen{})

	if preview.pendingPlace {
		t.Fatal("expected pending placement to be cleared before redraw")
	}
	if preview.kittyPlaced {
		t.Fatal("expected kitty placement to be re-armed for redraw")
	}
	if preview.kittyUploaded {
		t.Fatal("expected kitty upload state to be reset after global clear")
	}
	if preview.cellW != 10 || preview.cellH != 20 {
		t.Fatalf("expected cell dimensions 10x20, got %dx%d", preview.cellW, preview.cellH)
	}
}

func TestMessagesList_ScheduleAnimatedRedraw_RearmsDuringQueuedDraw(t *testing.T) {
	ml := &messagesList{}
	var draws atomic.Int32
	done := make(chan struct{})

	ml.queueDraw = func() {
		switch draws.Add(1) {
		case 1:
			ml.scheduleAnimatedRedraw(5 * time.Millisecond)
		case 2:
			select {
			case <-done:
			default:
				close(done)
			}
		}
	}

	ml.scheduleAnimatedRedraw(5 * time.Millisecond)
	defer ml.stopAnimatedRedraw()

	select {
	case <-done:
	case <-time.After(300 * time.Millisecond):
		t.Fatalf("expected animation redraw to re-arm during queued draw, got %d draw(s)", draws.Load())
	}
}

type mockEmoteScreen struct {
	MockScreen
	cells map[string]string // "x,y" -> url
}

func (m *mockEmoteScreen) Get(x, y int) (string, tcell.Style, int) {
	style := tcell.StyleDefault
	if url, ok := m.cells[fmt.Sprintf("%d,%d", x, y)]; ok {
		style = style.Url(url)
	}
	return " ", style, 1
}

func TestMessagesList_EmoteScanning(t *testing.T) {
	cfg, _ := config.Load("")
	cfg.InlineImages.Enabled = true
	ml := newMessagesList(cfg, nil)
	ml.SetRect(0, 0, 100, 100)

	screen := &mockEmoteScreen{
		cells: map[string]string{
			"10,10": "https://cdn.discordapp.com/emojis/1.png?v=1",
			"11,10": "https://cdn.discordapp.com/emojis/1.png?v=1",
			"12,10": "https://cdn.discordapp.com/emojis/1.png?v=1",
			"13,10": "https://cdn.discordapp.com/emojis/1.png?v=1",
			"20,10": "https://cdn.discordapp.com/emojis/2.png?v=1",
			"21,10": "https://cdn.discordapp.com/emojis/2.png?v=1",
		},
	}

	ml.scanAndDrawEmotes(screen)

	if count := len(ml.emoteItemByKey); count != 3 {
		t.Errorf("expected 3 emote items in cache, got %d", count)
	}

	key1 := "https://cdn.discordapp.com/emojis/1.png?v=1@10,10"
	if _, ok := ml.emoteItemByKey[key1]; !ok {
		t.Errorf("expected key %q to exist", key1)
	}

	key2 := "https://cdn.discordapp.com/emojis/1.png?v=1@12,10"
	if _, ok := ml.emoteItemByKey[key2]; !ok {
		t.Errorf("expected key %q to exist", key2)
	}
}

func TestMessagesList_EmoteCacheRequest(t *testing.T) {
	cfg, _ := config.Load("")
	cfg.InlineImages.Enabled = true
	ml := newMessagesList(cfg, nil)
	ml.SetRect(0, 0, 100, 100)

	url := "https://cdn.discordapp.com/emojis/99.png?v=1"
	screen := &mockEmoteScreen{
		cells: map[string]string{
			"5,5": url,
			"6,5": url,
		},
	}

	ml.scanAndDrawEmotes(screen)

	// The image cache should have a request in-flight for the emote URL.
	if !ml.imageCache.Requested(url) {
		t.Error("expected image cache request to be triggered for new emote URL")
	}
}

func TestMessagesList_EmoteClearTrailingCells(t *testing.T) {
	cfg, _ := config.Load("")
	cfg.InlineImages.Enabled = true
	ml := newMessagesList(cfg, nil)
	ml.SetRect(0, 0, 100, 100)

	url := "https://cdn.discordapp.com/emojis/42.png?v=1"
	screen := &mockEmoteScreen{
		MockScreen: MockScreen{},
		cells: map[string]string{
			// Two adjacent emotes with the same URL should keep both leading cells.
			"10,5": url,
			"11,5": url,
			"12,5": url,
			"13,5": url,
		},
	}

	ml.scanAndDrawEmotes(screen)

	// Cells 11 and 13 should have been cleared to ' ', while 10 and 12 remain
	// reserved for the two distinct emote instances.
	for _, key := range []string{"11,5", "13,5"} {
		r, ok := screen.Content[key]
		if !ok {
			t.Errorf("cell %s was not written to (expected blank)", key)
		} else if r != ' ' {
			t.Errorf("cell %s = %q, want ' '", key, string(r))
		}
	}

	for _, key := range []string{"10,5", "12,5"} {
		if _, ok := screen.Content[key]; !ok {
			t.Errorf("cell %s should remain the leading cell of an emote instance", key)
		}
	}
}
