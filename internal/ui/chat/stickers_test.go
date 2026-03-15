package chat

import (
	"strings"
	"testing"

	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/gdamore/tcell/v3"
)

func TestMessagesList_RebuildRows_WithStickers(t *testing.T) {
	m := newMockChatModel()
	ml := m.messagesList
	ml.cfg.InlineImages.Enabled = true

	msg := discord.Message{
		ID: 1,
		Stickers: []discord.StickerItem{
			{
				ID:   123,
				Name: "Test Sticker",
			},
		},
	}

	ml.messages = []discord.Message{msg}
	ml.invalidateRows()
	ml.ensureRows()

	foundSticker := false
	for _, row := range ml.rows {
		if row.kind == messagesListRowSticker {
			foundSticker = true
			if row.messageIndex != 0 {
				t.Errorf("expected messageIndex 0, got %d", row.messageIndex)
			}
		}
	}

	if !foundSticker {
		t.Error("expected to find a sticker row, but didn't")
	}
}

func TestMessagesList_BuildItem_Sticker(t *testing.T) {
	m := newMockChatModel()
	ml := m.messagesList
	ml.cfg.InlineImages.Enabled = true

	msgID := discord.MessageID(1)
	stickerID := discord.StickerID(123)
	msg := discord.Message{
		ID: msgID,
		Stickers: []discord.StickerItem{
			{
				ID:         stickerID,
				Name:       "Test Sticker",
				FormatType: discord.StickerFormatPNG,
			},
		},
	}

	ml.messages = []discord.Message{msg}
	ml.invalidateRows()
	ml.ensureRows()

	for i, row := range ml.rows {
		if row.kind == messagesListRowSticker {
			item := ml.buildItem(i, -1)
			imgItem, ok := item.(*imageItem)
			if !ok {
				t.Fatalf("expected *imageItem for sticker row, got %T", item)
			}

			expectedURL := "https://cdn.discordapp.com/stickers/123.png"
			if imgItem.url != expectedURL {
				t.Errorf("expected URL %q, got %q", expectedURL, imgItem.url)
			}
			return
		}
	}

	t.Fatal("sticker row not found")
}

func TestMessagesList_RenderMessage_WithStickers(t *testing.T) {
	m := newMockChatModel()
	ml := m.messagesList
	ml.cfg.InlineImages.Enabled = false

	msg := discord.Message{
		ID: 1,
		Stickers: []discord.StickerItem{
			{
				ID:   123,
				Name: "Test Sticker",
			},
		},
	}

	lines := ml.renderMessage(msg, tcell.StyleDefault)
	found := false
	for _, line := range lines {
		for _, seg := range line {
			if strings.Contains(seg.Text, "[Sticker: Test Sticker]") {
				found = true
				break
			}
		}
	}

	if !found {
		t.Error("expected to find sticker text in message, but didn't")
	}
}

func TestCompactMessagePreview_WithStickers(t *testing.T) {
	msg := discord.Message{
		Stickers: []discord.StickerItem{
			{Name: "Test Sticker"},
		},
	}
	got := compactMessagePreview(msg)
	want := "[sticker]"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
