package chat

import (
	"strings"
	"testing"

	"github.com/ayn2op/discordo/internal/markdown"
	"github.com/ayn2op/tview"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/gdamore/tcell/v3"
)

func TestMentionsListHelpers(t *testing.T) {
	chat := newMockChatModel()
	m := newMentionsList(chat.cfg, chat)

	m.rebuild()
	if got := m.Cursor(); got != -1 {
		t.Fatalf("expected empty list cursor to be -1, got %d", got)
	}
	if text, ok := m.selectedInsertText(); ok || text != "" {
		t.Fatalf("expected no insert text from empty list, got %q ok=%v", text, ok)
	}

	m.append(mentionsListItem{insertText: "alpha", displayText: "Alpha", style: tcell.StyleDefault})
	m.append(mentionsListItem{insertText: "beta", displayText: "BetaUser", style: tcell.StyleDefault})
	m.rebuild()

	if item := m.Builder(-1, 0); item != nil {
		t.Fatal("expected negative builder index to return nil")
	}
	selectedItem, ok := m.Builder(0, 0).(*tview.TextView)
	if !ok {
		t.Fatalf("expected selected builder item to be a text view, got %T", m.Builder(0, 0))
	}
	selectedLines := selectedItem.GetLines()
	if len(selectedLines) != 1 || len(selectedLines[0]) != 1 || selectedLines[0][0].Text != "Alpha" {
		t.Fatalf("expected selected builder item to render Alpha, got %#v", selectedLines)
	}
	if attrs := selectedLines[0][0].Style.GetAttributes(); attrs&tcell.AttrReverse == 0 {
		t.Fatal("expected selected builder item style to be reversed")
	}

	unselectedItem, ok := m.Builder(1, 0).(*tview.TextView)
	if !ok {
		t.Fatalf("expected unselected builder item to be a text view, got %T", m.Builder(1, 0))
	}
	unselectedLines := unselectedItem.GetLines()
	if len(unselectedLines) != 1 || len(unselectedLines[0]) != 1 || unselectedLines[0][0].Text != "BetaUser" {
		t.Fatalf("expected unselected builder item to render BetaUser, got %#v", unselectedLines)
	}
	if attrs := unselectedLines[0][0].Style.GetAttributes(); attrs&tcell.AttrReverse != 0 {
		t.Fatal("expected unselected builder item style to remain non-reversed")
	}
	if item := m.Builder(2, 0); item != nil {
		t.Fatal("expected out-of-range builder index to return nil")
	}

	if got := m.itemCount(); got != 2 {
		t.Fatalf("expected 2 mentions, got %d", got)
	}
	if got := m.Cursor(); got != 0 {
		t.Fatalf("expected rebuilt list to select first item, got %d", got)
	}
	if text, ok := m.selectedInsertText(); !ok || text != "alpha" {
		t.Fatalf("expected selected insert text alpha, got %q ok=%v", text, ok)
	}
	if got := m.maxDisplayWidth(); got < len("BetaUser") {
		t.Fatalf("expected max display width to cover longest item, got %d", got)
	}

	m.clear()
	if got := m.itemCount(); got != 0 {
		t.Fatalf("expected cleared list to be empty, got %d", got)
	}
}

func TestMentionsListAppendEmojiUsesEmojiPreviewLine(t *testing.T) {
	chat := newMockChatModel()
	m := newMentionsList(chat.cfg, chat)

	emoji := discord.Emoji{ID: 123456, Name: "kekw"}
	m.appendEmoji(emoji)
	m.rebuild()

	item, ok := m.Builder(0, 0).(*tview.TextView)
	if !ok {
		t.Fatalf("expected emoji builder item to be a text view, got %T", m.Builder(0, 0))
	}
	lines := item.GetLines()
	if len(lines) != 1 || len(lines[0]) != 2 {
		t.Fatalf("expected emoji suggestion to render preview and label, got %#v", lines)
	}
	if got := lines[0][0].Text; got != markdown.CustomEmojiText("kekw", chat.cfg.InlineImages.Enabled) {
		t.Fatalf("expected emoji preview text %q, got %q", markdown.CustomEmojiText("kekw", chat.cfg.InlineImages.Enabled), got)
	}
	if _, url := lines[0][0].Style.GetUrl(); url != emoji.EmojiURL() {
		t.Fatalf("expected emoji preview URL %q, got %q", emoji.EmojiURL(), url)
	}
	if got := lines[0][1].Text; got != " kekw" {
		t.Fatalf("expected emoji label %q, got %q", " kekw", got)
	}
	if got := m.maxDisplayWidth(); got < len(" kekw")+inlineEmoteWidth {
		t.Fatalf("expected emoji width to include preview and label, got %d", got)
	}
}

func TestMentionsListClearQueuesKittyDeletesForAutocompleteEmoji(t *testing.T) {
	chat := newMockChatModel()
	chat.cfg.InlineImages.Enabled = true
	chat.messagesList.useKitty = true

	m := newMentionsList(chat.cfg, chat)
	lockScreen := &lockingTTYScreen{tty: &mockTty{}}
	m.lastScreen = lockScreen
	m.emoteItemByKey["https://cdn.discordapp.com/emojis/7.png"] = &imageItem{
		kittyID:          7,
		kittyPlaced:      true,
		kittyUploaded:    true,
		pendingPlace:     true,
		kittyCols:        2,
		kittyVisibleRows: 1,
		lastX:            1,
		lastY:            2,
	}

	m.clear()

	if len(m.pendingDeletes) != 1 || m.pendingDeletes[0] != 7 {
		t.Fatalf("expected kitty delete to be queued for autocomplete emoji, got %v", m.pendingDeletes)
	}
	item := m.emoteItemByKey["https://cdn.discordapp.com/emojis/7.png"]
	if item.pendingPlace || item.kittyPlaced || item.kittyUploaded {
		t.Fatal("expected clear to invalidate kitty popup image state")
	}
	if lockScreen.lockCalls == 0 {
		t.Fatal("expected clear to unlock the prior kitty region")
	}
	if !m.hasPendingAfterDraw() {
		t.Fatal("expected clear to keep pending kitty cleanup for AfterDraw")
	}

	tty := &mockTty{}
	m.AfterDraw(&screenWithTty{tty: tty})
	if !strings.Contains(tty.String(), "a=d,d=I,i=7") {
		t.Fatalf("expected AfterDraw to delete stale kitty popup emoji, got %q", tty.String())
	}
	if m.hasPendingAfterDraw() {
		t.Fatal("expected AfterDraw to drain pending kitty cleanup")
	}
}
