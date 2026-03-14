package chat

import (
	"reflect"
	"testing"
	"unsafe"

	imgpkg "github.com/ayn2op/discordo/internal/image"
	"github.com/ayn2op/discordo/internal/markdown"
	"github.com/ayn2op/discordo/pkg/picker"
	"github.com/ayn2op/tview"
	"github.com/ayn2op/tview/layers"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/gdamore/tcell/v3"
)

type emojiURLScreen struct {
	completeMockScreen
	url string
}

func reactionPickerPrivateField[T any](t *testing.T, rp *reactionPicker, name string) T {
	t.Helper()

	field := reflect.ValueOf(rp.Picker).Elem().FieldByName(name)
	if !field.IsValid() {
		t.Fatalf("picker field %q not found", name)
	}
	if !field.CanAddr() {
		t.Fatalf("picker field %q is not addressable", name)
	}

	return *(*T)(unsafe.Pointer(field.UnsafeAddr()))
}

func (s *emojiURLScreen) Get(x, y int) (string, tcell.Style, int) {
	if s.url != "" {
		return " ", tcell.StyleDefault.Url(s.url), 1
	}
	return " ", tcell.StyleDefault, 1
}

func TestReactionPickerSetItemsAndHelp(t *testing.T) {
	m := newTestModel()
	rp := newReactionPicker(m.cfg, m, m.messagesList, imgpkg.NewCache(nil))

	items := []discord.Emoji{
		{Name: "🙂"},
		{ID: 123456, Name: "kekw"},
	}
	rp.SetItems(items)

	if len(rp.items) != len(items) {
		t.Fatalf("expected %d picker items, got %d", len(items), len(rp.items))
	}
	if got := rp.GetFooter(); got != "Tab focus" {
		t.Fatalf("expected footer %q, got %q", "Tab focus", got)
	}
	if len(rp.ShortHelp()) == 0 || len(rp.FullHelp()) == 0 {
		t.Fatal("expected picker help to be populated")
	}

	unicodeLine := rp.lineForEmoji(items[0])
	if len(unicodeLine) != 1 || unicodeLine[0].Text != items[0].Name {
		t.Fatalf("unexpected unicode emoji line: %#v", unicodeLine)
	}

	m.cfg.InlineImages.Enabled = true
	customLine := rp.lineForEmoji(items[1])
	if len(customLine) != 2 {
		t.Fatalf("expected preview and label segments for custom emoji, got %#v", customLine)
	}
	if customLine[0].Text != markdown.CustomEmojiText(items[1].Name, true) {
		t.Fatalf("unexpected custom emoji preview text %q", customLine[0].Text)
	}
	if customLine[1].Text != " "+items[1].Name {
		t.Fatalf("unexpected custom emoji label %q", customLine[1].Text)
	}

	setFocus := reactionPickerPrivateField[func(tview.Primitive)](t, rp, "setFocus")
	list := reactionPickerPrivateField[*tview.List](t, rp, "list")
	setFocus(list)
	if m.app.GetFocus() != list {
		t.Fatalf("expected picker focus callback to delegate to the app")
	}
}

func TestReactionPickerPreviewAndScan(t *testing.T) {
	m := newTestModel()
	m.cfg.InlineImages.Enabled = true
	m.messagesList.useKitty = false
	rp := newReactionPicker(m.cfg, m, m.messagesList, imgpkg.NewCache(nil))

	if item := rp.previewItem(discord.Emoji{Name: "🙂"}); item != nil {
		t.Fatal("expected no preview item for unicode emoji")
	}

	custom := discord.Emoji{ID: 123456, Name: "kekw"}
	first := rp.previewItem(custom)
	if first == nil {
		t.Fatal("expected preview item for custom emoji")
	}
	second := rp.previewItemByURL(custom.EmojiURL())
	if second == nil {
		t.Fatal("expected preview item by URL")
	}
	if first != second {
		t.Fatal("expected preview item lookup by URL to reuse the cached item")
	}

	screen := &emojiURLScreen{url: "https://cdn.discordapp.com/emojis/999999.png"}
	rp.SetRect(0, 0, 10, 3)
	rp.scanAndDrawEmotes(screen)
	if _, ok := rp.emoteItemByKey[screen.url]; !ok {
		t.Fatal("expected scan to create a preview item for visible emoji URL")
	}
	if len(screen.Content) == 0 {
		t.Fatal("expected scan to draw placeholder content for visible emoji URL")
	}

	plainScreen := &emojiURLScreen{url: "https://example.com/not-discord.png"}
	rp.scanAndDrawEmotes(plainScreen)
	if len(rp.emoteItemByKey) != 2 {
		t.Fatalf("expected non-discord URL to be ignored, got %d preview items", len(rp.emoteItemByKey))
	}
}

func TestReactionPickerOnSelectedAndClose(t *testing.T) {
	m := newTestModel()
	rp := newReactionPicker(m.cfg, m, m.messagesList, imgpkg.NewCache(nil))
	m.messagesList.reactionPicker = rp

	rp.SetItems([]discord.Emoji{{Name: "🙂"}})
	m.AddLayer(rp, layers.WithName(reactionPickerLayerName), layers.WithVisible(true))
	m.app.SetFocus(rp)

	rp.onSelected(picker.Item{Reference: "bad"})
	rp.onSelected(picker.Item{Reference: 99})
	if !m.HasLayer(reactionPickerLayerName) {
		t.Fatal("expected invalid selections to keep the picker open")
	}

	rp.onSelected(picker.Item{Reference: 0})
	if !m.HasLayer(reactionPickerLayerName) {
		t.Fatal("expected picker to stay open when no message is selected")
	}

	channel := &discord.Channel{ID: 200, GuildID: 100, Type: discord.GuildText}
	m.SetSelectedChannel(channel)
	m.messagesList.setMessages([]discord.Message{
		{ID: 300, ChannelID: channel.ID, GuildID: channel.GuildID, Author: discord.User{ID: 2, Username: "user"}},
	})
	m.messagesList.SetCursor(0)

	rp.onSelected(picker.Item{Reference: 0})
	if m.HasLayer(reactionPickerLayerName) {
		t.Fatal("expected picker to close after successful reaction")
	}
	if !m.messagesList.pendingFullClear {
		t.Fatal("expected successful close to request a full clear")
	}
	if m.app.GetFocus() != m.messagesList {
		t.Fatalf("expected focus to return to messages list, got %T", m.app.GetFocus())
	}
}

func TestReactionPickerDrawAndAfterDraw(t *testing.T) {
	m := newTestModel()
	m.cfg.InlineImages.Enabled = true
	m.messagesList.useKitty = true
	rp := newReactionPicker(m.cfg, m, m.messagesList, imgpkg.NewCache(nil))
	rp.SetRect(0, 0, 10, 3)

	screen := &emojiURLScreen{url: "https://cdn.discordapp.com/emojis/999999.png"}
	rp.Draw(screen)
	if _, ok := rp.emoteItemByKey[screen.url]; !ok {
		t.Fatal("expected Draw to scan and register emoji previews")
	}

	ttyScreen := &screenWithTty{tty: &mockTty{}}
	rp.emoteItemByKey["kitty"] = &imageItem{
		pendingPlace:     true,
		kittyID:          1,
		kittyPayload:     "payload",
		kittyCols:        1,
		kittyRows:        1,
		kittyVisibleRows: 1,
		cellW:            10,
		cellH:            20,
	}
	rp.AfterDraw(ttyScreen)
	if ttyScreen.tty.Len() == 0 {
		t.Fatal("expected AfterDraw to write kitty placement commands")
	}
}

func TestReactionPickerErrorAndEarlyReturnBranches(t *testing.T) {
	m := newMockChatModel()
	rp := newReactionPicker(m.cfg, m, m.messagesList, imgpkg.NewCache(nil))
	m.messagesList.reactionPicker = rp
	rp.SetItems([]discord.Emoji{{Name: "🙂"}})

	channel := &discord.Channel{ID: 200, GuildID: 100, Type: discord.GuildText}
	m.SetSelectedChannel(channel)
	m.messagesList.setMessages([]discord.Message{
		{ID: 999, ChannelID: channel.ID, GuildID: channel.GuildID, Author: discord.User{ID: 2, Username: "user"}},
	})
	m.messagesList.SetCursor(0)
	m.AddLayer(rp, layers.WithName(reactionPickerLayerName), layers.WithVisible(true))

	rp.onSelected(picker.Item{Reference: 0})
	if !m.HasLayer(reactionPickerLayerName) {
		t.Fatal("expected picker to stay open when reacting fails")
	}

	m.cfg.InlineImages.Enabled = false
	rp.AfterDraw(&completeMockScreen{})

	m.cfg.InlineImages.Enabled = true
	m.messagesList.useKitty = true
	rp.AfterDraw(&completeMockScreen{})

	screen := &emojiURLScreen{url: "https://cdn.discordapp.com/emojis/123.png"}
	before := len(rp.emoteItemByKey)
	m.cfg.InlineImages.Enabled = false
	rp.scanAndDrawEmotes(screen)
	if got := len(rp.emoteItemByKey); got != before {
		t.Fatalf("expected disabled inline images to skip preview creation, got %d items", got)
	}
}
