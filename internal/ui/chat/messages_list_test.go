package chat

import (
	"slices"
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

	ml.HandleEvent(event)
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

	m.messagesList.HandleEvent(tcell.NewEventKey(tcell.KeyRune, "+", tcell.ModNone))

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

func TestMessagesListSetAddDeleteAndSelectionHelpers(t *testing.T) {
	m := newMockChatModel()
	ml := m.messagesList

	first := discord.Message{ID: 1, ChannelID: 10, Content: "first"}
	second := discord.Message{ID: 2, ChannelID: 10, Content: "second"}
	ml.itemByID[first.ID] = tview.NewTextView()

	ml.setMessages([]discord.Message{first, second})
	if len(ml.messages) != 2 || ml.messages[0].ID != second.ID || ml.messages[1].ID != first.ID {
		t.Fatalf("setMessages reversed messages = %#v", ml.messages)
	}
	if len(ml.itemByID) != 0 {
		t.Fatal("setMessages should clear cached items")
	}
	if !ml.kittyNeedsFullClear {
		t.Fatal("setMessages should require a full kitty clear")
	}

	ml.addMessage(discord.Message{ID: 3, ChannelID: 10, Content: "third"})
	if got := len(ml.messages); got != 3 {
		t.Fatalf("message count after add = %d, want 3", got)
	}

	ml.setMessage(10, discord.Message{ID: 99})
	ml.deleteMessage(10)
	if got := len(ml.messages); got != 3 {
		t.Fatalf("out-of-range updates should keep message count at 3, got %d", got)
	}

	ml.SetCursor(1)
	if got := ml.Cursor(); got != 1 {
		t.Fatalf("Cursor() = %d, want 1", got)
	}
	if msg, err := ml.selectedMessage(); err != nil || msg.ID != first.ID {
		t.Fatalf("selectedMessage() = (%v, %v), want message ID %d", msg, err, first.ID)
	}

	ml.clearSelection()
	if got := ml.Cursor(); got != -1 {
		t.Fatalf("Cursor() after clear = %d, want -1", got)
	}
	if _, err := ml.selectedMessage(); err == nil {
		t.Fatal("selectedMessage() should fail when no message is selected")
	}

	ml.deleteMessage(1)
	if got := len(ml.messages); got != 2 {
		t.Fatalf("message count after delete = %d, want 2", got)
	}
}

func TestMessagesListRowNavigationHelpers(t *testing.T) {
	m := newMockChatModel()
	ml := m.messagesList
	ml.cfg.DateSeparator.Enabled = true
	ml.cfg.InlineImages.Enabled = true
	now := discord.NewTimestamp(time.Now())
	ml.messages = []discord.Message{
		{
			ID:        1,
			ChannelID: 10,
			Timestamp: now,
			Attachments: []discord.Attachment{
				{Filename: "image.png", ContentType: "image/png"},
			},
		},
		{ID: 2, ChannelID: 10, Timestamp: now},
	}
	ml.invalidateRows()
	ml.ensureRows()

	if got := ml.messageToRowIndex(0); got == -1 {
		t.Fatal("expected first message to map to a row index")
	}

	separatorIndex := 0
	imageIndex := -1
	for i, row := range ml.rows {
		if row.kind == messagesListRowImage {
			imageIndex = i
			break
		}
	}
	if imageIndex == -1 {
		t.Fatal("expected image row to be present")
	}

	if got := ml.nearestMessageRowIndex(separatorIndex); got != 1 {
		t.Fatalf("nearestMessageRowIndex(separator) = %d, want 1", got)
	}
	if got := ml.nearestMessageRowIndex(imageIndex); got != 1 {
		t.Fatalf("nearestMessageRowIndex(image row) = %d, want 1", got)
	}

	ml.List.SetCursor(separatorIndex)
	ml.onRowCursorChanged(separatorIndex)
	if got := ml.List.Cursor(); got != 1 {
		t.Fatalf("cursor after separator snap = %d, want 1", got)
	}

	ml.List.SetCursor(imageIndex)
	ml.onRowCursorChanged(imageIndex)
	if got := ml.List.Cursor(); got != 1 {
		t.Fatalf("cursor after image snap = %d, want 1", got)
	}
}

func TestMessagesListSelectReplyAndEmbedHelpers(t *testing.T) {
	m := newMockChatModel()
	ml := m.messagesList
	ml.messages = []discord.Message{
		{ID: 1, ChannelID: 10, Content: "root"},
		{ID: 2, ChannelID: 10, ReferencedMessage: &discord.Message{ID: 1}},
	}
	ml.invalidateRows()
	ml.SetCursor(1)
	ml.selectReply()
	if got := ml.Cursor(); got != 0 {
		t.Fatalf("cursor after selectReply = %d, want 0", got)
	}

	lines := embedLines(discord.Embed{
		Title:       "Title",
		Description: `hello \. world`,
		URL:         "https://example.com/path",
		Fields: []discord.EmbedField{
			{Name: "Field", Value: "Value"},
			{Name: "Field", Value: "Value"},
		},
		Image: &discord.EmbedImage{URL: "https://cdn.discordapp.com/emojis/123.png"},
	}, map[string]struct{}{}, true)

	if len(lines) == 0 {
		t.Fatal("expected embed lines to be generated")
	}
	if !strings.Contains(lines[1].Text, "hello . world") {
		t.Fatalf("embed description = %q, want markdown escapes removed", lines[1].Text)
	}
	if got := linkDisplayText("https://example.com/" + strings.Repeat("a", 60)); !strings.HasSuffix(got, "...") {
		t.Fatalf("linkDisplayText() = %q, want ellipsis for long path", got)
	}
	if got := unescapeMarkdownEscapes(`a\*b`); got != "a*b" {
		t.Fatalf("unescapeMarkdownEscapes() = %q, want %q", got, "a*b")
	}
}

func TestMessagesList_RebuildRowsAndCursorMapping(t *testing.T) {
	m := newMockChatModel()
	m.cfg.DateSeparator.Enabled = true
	m.cfg.InlineImages.Enabled = true
	ml := m.messagesList

	day1 := discord.NewTimestamp(time.Date(2024, time.January, 1, 12, 0, 0, 0, time.UTC))
	day2 := discord.NewTimestamp(time.Date(2024, time.January, 2, 12, 0, 0, 0, time.UTC))

	ml.messages = []discord.Message{
		{
			ID:        1,
			Timestamp: day1,
			Content:   "one",
			Attachments: []discord.Attachment{
				{Filename: "one.png", URL: "https://cdn.example/one.png", ContentType: "image/png"},
			},
		},
		{
			ID:        2,
			Timestamp: day1,
			Content:   "two",
		},
		{
			ID:        3,
			Timestamp: day2,
			Content:   "three",
			Attachments: []discord.Attachment{
				{Filename: "three.txt", URL: "https://cdn.example/three.txt", ContentType: "text/plain"},
				{Filename: "three.gif", URL: "https://cdn.example/three.gif", ContentType: "image/gif"},
			},
		},
	}

	ml.rebuildRows()

	wantKinds := []messagesListRowKind{
		messagesListRowSeparator,
		messagesListRowMessage,
		messagesListRowImage,
		messagesListRowMessage,
		messagesListRowSeparator,
		messagesListRowMessage,
		messagesListRowImage,
	}
	if got := rowKinds(ml.rows); !slices.Equal(got, wantKinds) {
		t.Fatalf("row kinds = %v, want %v", got, wantKinds)
	}

	if got := ml.messageToRowIndex(0); got != 1 {
		t.Fatalf("messageToRowIndex(0) = %d, want 1", got)
	}
	if got := ml.messageToRowIndex(1); got != 3 {
		t.Fatalf("messageToRowIndex(1) = %d, want 3", got)
	}
	if got := ml.messageToRowIndex(2); got != 5 {
		t.Fatalf("messageToRowIndex(2) = %d, want 5", got)
	}
	if got := ml.messageToRowIndex(99); got != -1 {
		t.Fatalf("messageToRowIndex(99) = %d, want -1", got)
	}

	ml.SetCursor(1)
	if got := ml.List.Cursor(); got != 3 {
		t.Fatalf("row cursor = %d, want 3", got)
	}
	if got := ml.Cursor(); got != 1 {
		t.Fatalf("Cursor() = %d, want 1", got)
	}

	ml.List.SetCursor(0)
	if got := ml.Cursor(); got != 0 {
		t.Fatalf("Cursor() after snapping from separator row = %d, want 0", got)
	}
	if got := ml.List.Cursor(); got != 1 {
		t.Fatalf("onRowCursorChanged(separator) cursor = %d, want 1", got)
	}

	ml.List.SetCursor(2)
	// it also snaps to 1
	if got := ml.List.Cursor(); got != 1 {
		t.Fatalf("onRowCursorChanged(image row) cursor = %d, want 1", got)
	}
}

func TestMessagesList_SelectedMessageErrorsAndSuccess(t *testing.T) {
	m := newMockChatModel()
	m.cfg.DateSeparator.Enabled = true
	ml := m.messagesList

	if _, err := ml.selectedMessage(); err == nil || err.Error() != "no messages available" {
		t.Fatalf("selectedMessage() error = %v, want %q", err, "no messages available")
	}

	ml.messages = []discord.Message{
		{
			ID:        1,
			Timestamp: discord.NewTimestamp(time.Date(2024, time.January, 1, 12, 0, 0, 0, time.UTC)),
			Content:   "hello",
		},
	}
	ml.rebuildRows()

	ml.SetCursor(0)
	msg, err := ml.selectedMessage()
	if err != nil {
		t.Fatalf("selectedMessage() returned error: %v", err)
	}
	if msg.ID != 1 {
		t.Fatalf("selectedMessage() ID = %s, want 1", msg.ID)
	}

	ml.clearSelection()
	if _, err := ml.selectedMessage(); err == nil || err.Error() != "no message is currently selected" {
		t.Fatalf("selectedMessage() after clearSelection error = %v, want %q", err, "no message is currently selected")
	}
}

func TestMessagesList_MessageMutationInvalidation(t *testing.T) {
	t.Run("setMessages clones reverses and clears caches", func(t *testing.T) {
		m := newMockChatModel()
		ml := m.messagesList
		ml.kittyNeedsFullClear = false
		ml.itemByID[1] = tview.NewTextView()

		input := []discord.Message{{ID: 2}, {ID: 1}}
		ml.setMessages(input)
		input[0].ID = 99

		if got := messageIDs(ml.messages); !slices.Equal(got, []discord.MessageID{1, 2}) {
			t.Fatalf("setMessages IDs = %v, want %v", got, []discord.MessageID{1, 2})
		}
		if len(ml.itemByID) != 0 {
			t.Fatalf("itemByID was not cleared: %#v", ml.itemByID)
		}
		if !ml.rowsDirty {
			t.Fatal("setMessages() did not mark rows dirty")
		}
		if !ml.kittyNeedsFullClear {
			t.Fatal("setMessages() did not request kitty full clear")
		}
	})

	t.Run("addMessage appends and invalidates reused cache", func(t *testing.T) {
		m := newMockChatModel()
		ml := m.messagesList
		ml.messages = []discord.Message{{ID: 1}}
		ml.rowsDirty = false
		ml.itemByID[2] = tview.NewTextView()

		ml.addMessage(discord.Message{ID: 2})

		if got := messageIDs(ml.messages); !slices.Equal(got, []discord.MessageID{1, 2}) {
			t.Fatalf("messages = %v, want %v", got, []discord.MessageID{1, 2})
		}
		if _, ok := ml.itemByID[2]; ok {
			t.Fatal("addMessage() did not invalidate cached item for reused ID")
		}
		if !ml.rowsDirty {
			t.Fatal("addMessage() did not mark rows dirty")
		}
	})

	t.Run("setMessage updates valid indices and ignores invalid ones", func(t *testing.T) {
		m := newMockChatModel()
		ml := m.messagesList
		ml.messages = []discord.Message{{ID: 1, Content: "before"}}
		ml.rowsDirty = false
		ml.itemByID[1] = tview.NewTextView()

		ml.setMessage(0, discord.Message{ID: 1, Content: "after"})

		if got := ml.messages[0].Content; got != "after" {
			t.Fatalf("updated content = %q, want %q", got, "after")
		}
		if _, ok := ml.itemByID[1]; ok {
			t.Fatal("setMessage() did not invalidate cached item")
		}
		if !ml.rowsDirty {
			t.Fatal("setMessage() did not mark rows dirty")
		}

		ml.rowsDirty = false
		ml.itemByID[1] = tview.NewTextView()
		ml.setMessage(9, discord.Message{ID: 1, Content: "ignored"})

		if got := ml.messages[0].Content; got != "after" {
			t.Fatalf("invalid setMessage changed content to %q", got)
		}
		if _, ok := ml.itemByID[1]; !ok {
			t.Fatal("invalid setMessage should not invalidate cache")
		}
		if ml.rowsDirty {
			t.Fatal("invalid setMessage should not mark rows dirty")
		}
	})

	t.Run("deleteMessage removes valid indices and ignores invalid ones", func(t *testing.T) {
		m := newMockChatModel()
		ml := m.messagesList
		ml.messages = []discord.Message{{ID: 1}, {ID: 2}}
		ml.rowsDirty = false
		ml.itemByID[1] = tview.NewTextView()

		ml.deleteMessage(0)

		if got := messageIDs(ml.messages); !slices.Equal(got, []discord.MessageID{2}) {
			t.Fatalf("messages after delete = %v, want %v", got, []discord.MessageID{2})
		}
		if _, ok := ml.itemByID[1]; ok {
			t.Fatal("deleteMessage() did not invalidate cached item")
		}
		if !ml.rowsDirty {
			t.Fatal("deleteMessage() did not mark rows dirty")
		}

		ml.rowsDirty = false
		ml.itemByID[2] = tview.NewTextView()
		ml.deleteMessage(9)

		if got := messageIDs(ml.messages); !slices.Equal(got, []discord.MessageID{2}) {
			t.Fatalf("invalid delete changed messages to %v", got)
		}
		if _, ok := ml.itemByID[2]; !ok {
			t.Fatal("invalid deleteMessage should not invalidate cache")
		}
		if ml.rowsDirty {
			t.Fatal("invalid deleteMessage should not mark rows dirty")
		}
	})
}

func TestMessagesList_BuildItemTypesAndCache(t *testing.T) {
	m := newMockChatModel()
	m.cfg.DateSeparator.Enabled = true
	m.cfg.InlineImages.Enabled = true
	ml := m.messagesList
	ml.SetRect(0, 0, 40, 10)

	msg := discord.Message{
		ID:        10,
		Timestamp: discord.NewTimestamp(time.Date(2024, time.January, 1, 12, 0, 0, 0, time.UTC)),
		Content:   "hello",
		Author:    discord.User{ID: 2, Username: "user"},
		Attachments: []discord.Attachment{
			{Filename: "image.png", URL: "https://cdn.example/image.png", ContentType: "image/png", Size: 12},
		},
	}
	ml.messages = []discord.Message{msg}
	ml.rebuildRows()

	separatorItem, ok := ml.buildItem(0, -1).(*tview.TextView)
	if !ok {
		t.Fatalf("buildItem(separator) type = %T, want *tview.TextView", ml.buildItem(0, -1))
	}
	if got := joinedLineText(separatorItem.GetLines()[0]); !strings.Contains(got, "2024") {
		t.Fatalf("separator text = %q, want formatted date", got)
	}

	messageItem, ok := ml.buildItem(1, -1).(*tview.TextView)
	if !ok {
		t.Fatalf("buildItem(message) type = %T, want *tview.TextView", ml.buildItem(1, -1))
	}
	if cached := ml.buildItem(1, -1); cached != messageItem {
		t.Fatal("buildItem(message) did not reuse cached text view")
	}
	if selected := ml.buildItem(1, 1); selected == messageItem {
		t.Fatal("buildItem(selected message) should bypass the unselected cache")
	}

	if _, ok := ml.buildItem(2, -1).(*imageItem); !ok {
		t.Fatalf("buildItem(image) type = %T, want *imageItem", ml.buildItem(2, -1))
	}
	if !ml.imageCache.Requested("https://cdn.example/image.png") {
		t.Fatal("buildItem(image) did not trigger image cache request")
	}
	if got := ml.buildItem(99, -1); got != nil {
		t.Fatalf("buildItem(out of range) = %T, want nil", got)
	}
}

func TestMessagesList_DrawDateSeparatorBranches(t *testing.T) {
	m := newMockChatModel()
	ml := m.messagesList
	m.cfg.DateSeparator.Format = "2006-01-02"
	m.cfg.DateSeparator.Character = "-"
	ts := discord.NewTimestamp(time.Date(2024, time.January, 2, 12, 0, 0, 0, time.UTC))

	ml.SetRect(0, 0, 0, 1)
	builder := tview.NewLineBuilder()
	ml.drawDateSeparator(builder, ts, tcell.StyleDefault)
	if got, want := joinedLineText(builder.Finish()[0]), "-------- 2024-01-02 --------"; got != want {
		t.Fatalf("zero-width separator = %q, want %q", got, want)
	}

	ml.SetRect(0, 0, 8, 1)
	builder = tview.NewLineBuilder()
	ml.drawDateSeparator(builder, ts, tcell.StyleDefault)
	if got, want := joinedLineText(builder.Finish()[0]), "2024-01-02"; got != want {
		t.Fatalf("narrow separator = %q, want %q", got, want)
	}

	ml.SetRect(0, 0, 20, 1)
	_, _, innerWidth, _ := ml.GetInnerRect()
	builder = tview.NewLineBuilder()
	ml.drawDateSeparator(builder, ts, tcell.StyleDefault)
	fillWidth := innerWidth - len(" 2024-01-02 ")
	wantWide := strings.Repeat("-", fillWidth/2) + " 2024-01-02 " + strings.Repeat("-", fillWidth-fillWidth/2)
	if got, want := joinedLineText(builder.Finish()[0]), wantWide; got != want {
		t.Fatalf("wide separator = %q, want %q", got, want)
	}
}

func TestWrapStyledLinePreservesSegmentsAcrossWraps(t *testing.T) {
	bold := tcell.StyleDefault.Bold(true)
	underlined := tcell.StyleDefault.Underline(true)
	line := tview.Line{
		{Text: "ab", Style: bold},
		{Text: "cd", Style: bold},
		{Text: "ef", Style: underlined},
	}

	got := wrapStyledLine(line, 3)
	if gotTexts := []string{joinedLineText(got[0]), joinedLineText(got[1])}; !slices.Equal(gotTexts, []string{"abc", "def"}) {
		t.Fatalf("wrapped texts = %v, want %v", gotTexts, []string{"abc", "def"})
	}
	if len(got[0]) != 1 || got[0][0].Text != "abc" || got[0][0].Style != bold {
		t.Fatalf("first wrapped line = %#v, want one bold segment %q", got[0], "abc")
	}
	if len(got[1]) != 2 || got[1][0].Text != "d" || got[1][0].Style != bold || got[1][1].Text != "ef" || got[1][1].Style != underlined {
		t.Fatalf("second wrapped line = %#v, want bold %q then underlined %q", got[1], "d", "ef")
	}
}

func TestEmbedLines_TextualAndMediaBranches(t *testing.T) {
	embed := discord.Embed{
		URL:         "https://title.example/path",
		Provider:    &discord.EmbedProvider{Name: "Provider"},
		Author:      &discord.EmbedAuthor{Name: "Author"},
		Title:       "Title",
		Description: `escaped \. text`,
		Fields: []discord.EmbedField{
			{Name: "Field", Value: "Value"},
			{Name: "Field", Value: "Value"},
		},
		Footer: &discord.EmbedFooter{Text: "Footer"},
		Image:  &discord.EmbedImage{URL: "https://images.example/path"},
		Video:  &discord.EmbedVideo{URL: "https://videos.example/path"},
	}

	got := embedLines(embed, nil, false)
	want := []embedLine{
		{Text: "Provider", Kind: embedLineProvider},
		{Text: "Author", Kind: embedLineAuthor},
		{Text: "Title", Kind: embedLineTitle, URL: "https://title.example/path"},
		{Text: "escaped . text", Kind: embedLineDescription},
		{Text: "Field", Kind: embedLineFieldName},
		{Text: "Value", Kind: embedLineFieldValue},
		{Text: "Footer", Kind: embedLineFooter},
		{Text: "images.example/path", Kind: embedLineURL, URL: "https://images.example/path"},
		{Text: "videos.example/path", Kind: embedLineURL, URL: "https://videos.example/path"},
	}

	if !slices.Equal(got, want) {
		t.Fatalf("embedLines() = %#v, want %#v", got, want)
	}
}

func TestEmbedLines_InlineEmojiPlaceholderAndContentDedup(t *testing.T) {
	embed := discord.Embed{
		URL:   "https://content.example/dupe",
		Image: &discord.EmbedImage{URL: "https://cdn.discordapp.com/emojis/123456.png"},
		Video: &discord.EmbedVideo{URL: "https://videos.example/path"},
	}

	got := embedLines(embed, map[string]struct{}{"https://content.example/dupe": {}}, true)
	want := []embedLine{
		{Text: " ", Kind: embedLineDescription, URL: "https://cdn.discordapp.com/emojis/123456.png"},
	}

	if !slices.Equal(got, want) {
		t.Fatalf("embedLines() = %#v, want %#v", got, want)
	}
}

func TestMessagesList_OpenShowsAttachmentsPickerForMultipleTargets(t *testing.T) {
	m := newMockChatModel()
	ml := m.messagesList

	ml.setMessages([]discord.Message{
		{ID: 1, Content: "https://one.example https://two.example"},
	})
	ml.SetCursor(0)

	ml.open()

	if !m.HasLayer(attachmentsListLayerName) {
		t.Fatal("expected attachments picker layer to be visible")
	}

	if len(ml.attachmentsPicker.items) != 2 || ml.attachmentsPicker.items[0].label != "https://one.example" || ml.attachmentsPicker.items[1].label != "https://two.example" {
		t.Fatalf("unexpected attachment picker items: %v", ml.attachmentsPicker.items)
	}
}

func TestMessagesList_FormatTimestamp(t *testing.T) {
	cfg, _ := config.Load("")
	ml := &messagesList{cfg: cfg}

	now := time.Now()
	yesterday := now.Add(-30 * time.Hour)

	tsNow := discord.NewTimestamp(now)
	tsYesterday := discord.NewTimestamp(yesterday)

	fmtNow := ml.formatTimestamp(tsNow)
	fmtYesterday := ml.formatTimestamp(tsYesterday)

	// Both should be non-empty strings.
	if fmtNow == "" {
		t.Fatal("formatTimestamp returned empty string for current time")
	}
	if fmtYesterday == "" {
		t.Fatal("formatTimestamp returned empty string for yesterday")
	}

	// Formatting the same moment twice must be deterministic.
	if got := ml.formatTimestamp(tsNow); got != fmtNow {
		t.Fatalf("formatTimestamp non-deterministic: %q vs %q", fmtNow, got)
	}
}

func TestMessagesList_WriteMessageTypes(t *testing.T) {
	m := newMockChatModel()
	ml := m.messagesList

	author := discord.User{ID: 5, Username: "testuser"}
	ts := discord.NewTimestamp(time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC))

	tests := []struct {
		name     string
		msg      discord.Message
		wantText string
	}{
		{
			name: "GuildMemberJoin",
			msg: discord.Message{
				Type:      discord.GuildMemberJoinMessage,
				Author:    author,
				Timestamp: ts,
			},
			wantText: "joined the server.",
		},
		{
			name: "ChannelPinned",
			msg: discord.Message{
				Type:      discord.ChannelPinnedMessage,
				Author:    author,
				Timestamp: ts,
			},
			wantText: "pinned a message.",
		},
		{
			name: "DefaultMessage",
			msg: discord.Message{
				Type:      discord.DefaultMessage,
				Author:    author,
				Timestamp: ts,
				Content:   "hello",
			},
			wantText: "testuser",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := tview.NewLineBuilder()
			ml.writeMessage(builder, tt.msg, tcell.StyleDefault)
			lines := builder.Finish()

			flat := func() string {
				var b strings.Builder
				for _, line := range lines {
					for _, seg := range line {
						b.WriteString(seg.Text)
					}
				}
				return b.String()
			}()

			if len(lines) == 0 {
				t.Fatal("expected non-empty output")
			}
			if !strings.Contains(flat, tt.wantText) {
				t.Fatalf("output %q does not contain %q", flat, tt.wantText)
			}
		})
	}
}

func TestMessagesList_DrawAuthorName(t *testing.T) {
	m := newMockChatModel()
	ml := m.messagesList

	msg := discord.Message{
		Author:    discord.User{ID: 42, Username: "BotUser", Bot: true},
		Timestamp: discord.NewTimestamp(time.Now()),
	}

	builder := tview.NewLineBuilder()
	ml.drawAuthor(builder, msg, tcell.StyleDefault)
	lines := builder.Finish()

	if len(lines) == 0 {
		t.Fatal("expected at least one line")
	}

	flat := func() string {
		var b strings.Builder
		for _, line := range lines {
			for _, seg := range line {
				b.WriteString(seg.Text)
			}
		}
		return b.String()
	}()

	if !strings.Contains(flat, "BotUser") {
		t.Fatalf("expected author name %q in output %q", "BotUser", flat)
	}
}

func TestMessagesList_DrawReplyMessage_NoReference(t *testing.T) {
	m := newMockChatModel()
	ml := m.messagesList

	msg := discord.Message{
		Type:              discord.InlinedReplyMessage,
		Author:            discord.User{ID: 10, Username: "replier"},
		Timestamp:         discord.NewTimestamp(time.Now()),
		Content:           "I replied",
		ReferencedMessage: nil,
	}

	builder := tview.NewLineBuilder()
	// Must not panic.
	ml.drawReplyMessage(builder, msg, tcell.StyleDefault)
	lines := builder.Finish()

	flat := func() string {
		var b strings.Builder
		for _, line := range lines {
			for _, seg := range line {
				b.WriteString(seg.Text)
			}
		}
		return b.String()
	}()

	if !strings.Contains(flat, "deleted") {
		t.Fatalf("expected deleted-message placeholder in output %q", flat)
	}
}

func TestMessagesList_DrawPinnedMessage(t *testing.T) {
	m := newMockChatModel()
	ml := m.messagesList

	msg := discord.Message{
		Type:      discord.ChannelPinnedMessage,
		Author:    discord.User{ID: 7, Username: "pinner"},
		Timestamp: discord.NewTimestamp(time.Now()),
	}

	builder := tview.NewLineBuilder()
	ml.drawPinnedMessage(builder, msg, tcell.StyleDefault)
	lines := builder.Finish()

	if len(lines) == 0 {
		t.Fatal("expected non-empty output for pinned message")
	}

	flat := func() string {
		var b strings.Builder
		for _, line := range lines {
			for _, seg := range line {
				b.WriteString(seg.Text)
			}
		}
		return b.String()
	}()

	if !strings.Contains(flat, "pinned") {
		t.Fatalf("expected %q in pinned message output %q", "pinned", flat)
	}
}

func TestMessagesList_SelectUpDownAtBoundaries(t *testing.T) {
	m := newMockChatModel()
	ml := m.messagesList

	msgs := []discord.Message{
		{ID: 1, Content: "a"},
		{ID: 2, Content: "b"},
		{ID: 3, Content: "c"},
	}
	ml.setMessages(msgs)
	ml.rebuildRows()

	// Start unselected; selectDown should move to last.
	ml.clearSelection()
	ml.selectDown()
	if got := ml.Cursor(); got < 0 || got >= len(msgs) {
		t.Fatalf("selectDown from unselected: cursor = %d, want valid index", got)
	}

	// Select last item; selectDown should not go past it.
	ml.SetCursor(len(msgs) - 1)
	ml.selectDown()
	if got := ml.Cursor(); got != len(msgs)-1 {
		t.Fatalf("selectDown at bottom: cursor = %d, want %d", got, len(msgs)-1)
	}

	// Select first item; selectUp at top with no channel should not panic or move.
	ml.SetCursor(0)
	ml.selectUp() // prependOlderMessages returns 0 because no channel is selected
	if got := ml.Cursor(); got != 0 {
		t.Fatalf("selectUp at top (no channel): cursor = %d, want 0", got)
	}
}

func rowKinds(rows []messagesListRow) []messagesListRowKind {
	kinds := make([]messagesListRowKind, 0, len(rows))
	for _, row := range rows {
		kinds = append(kinds, row.kind)
	}
	return kinds
}

func messageIDs(messages []discord.Message) []discord.MessageID {
	ids := make([]discord.MessageID, 0, len(messages))
	for _, message := range messages {
		ids = append(ids, message.ID)
	}
	return ids
}

func joinedLineText(line tview.Line) string {
	var b strings.Builder
	for _, segment := range line {
		b.WriteString(segment.Text)
	}
	return b.String()
}

func TestMessagesList_SelectTopBottom(t *testing.T) {
	m := newMockChatModel()
	ml := m.messagesList

	// Empty list shouldn't panic
	ml.selectTop()
	ml.selectBottom()

	msgs := []discord.Message{
		{ID: 1, Content: "a"},
		{ID: 2, Content: "b"},
		{ID: 3, Content: "c"},
	}
	ml.setMessages(msgs)
	ml.rebuildRows()

	ml.SetCursor(1)
	ml.selectTop()
	if got := ml.Cursor(); got != 0 {
		t.Fatalf("selectTop: cursor = %d, want 0", got)
	}

	ml.SetCursor(1)
	ml.selectBottom()
	if got := ml.Cursor(); got != len(msgs)-1 {
		t.Fatalf("selectBottom: cursor = %d, want %d", got, len(msgs)-1)
	}
}
