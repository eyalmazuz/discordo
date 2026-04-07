package chat

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"testing"
	"time"

	imgpkg "github.com/ayn2op/discordo/internal/image"
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

	builder := m.Builder()
	if item := builder(-1, 0); item != nil {
		t.Fatal("expected negative builder index to return nil")
	}

	selectedItem, ok := builder(0, 0).(*mentionsListRowItem)
	if !ok {
		t.Fatalf("expected selected builder item to be a row item, got %T", builder(0, 0))
	}
	if selectedItem.item.displayText != "Alpha" {
		t.Fatalf("expected selected row item to carry Alpha, got %#v", selectedItem.item)
	}
	if attrs := selectedItem.style.GetAttributes(); attrs&tcell.AttrReverse == 0 {
		t.Fatal("expected selected row item style to be reversed")
	}

	unselectedItem, ok := builder(1, 0).(*mentionsListRowItem)
	if !ok {
		t.Fatalf("expected unselected builder item to be a row item, got %T", builder(1, 0))
	}
	if unselectedItem.item.displayText != "BetaUser" {
		t.Fatalf("expected unselected row item to carry BetaUser, got %#v", unselectedItem.item)
	}
	if attrs := unselectedItem.style.GetAttributes(); attrs&tcell.AttrReverse != 0 {
		t.Fatal("expected unselected row item style to remain non-reversed")
	}
	if item := builder(2, 0); item != nil {
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

func TestMentionsListRebuildPreservesStyling(t *testing.T) {
	chat := newMockChatModel()
	baseStyle := tcell.StyleDefault.Bold(true)
	m := newMentionsList(chat.cfg, chat)
	m.append(mentionsListItem{insertText: "gamma", displayText: "Gamma", style: baseStyle})
	m.rebuild()

	item, ok := m.Builder()(0, 1).(*mentionsListRowItem)
	if !ok {
		t.Fatalf("expected builder item to be a row item, got %T", m.Builder()(0, 1))
	}
	if attrs := item.style.GetAttributes(); attrs&tcell.AttrBold == 0 {
		t.Fatal("expected mention row style to preserve bold attribute")
	}
	if attrs := item.style.GetAttributes(); attrs&tcell.AttrReverse != 0 {
		t.Fatal("expected non-selected mention row to avoid reverse style")
	}
}

func TestMentionsListEmojiPreview(t *testing.T) {
	chat := newMockChatModel()
	chat.cfg.InlineImages.Enabled = true
	m := newMentionsList(chat.cfg, chat)
	url := "https://cdn.discordapp.com/emojis/123.png"
	m.append(mentionsListItem{
		insertText:  "<:kekw:123>",
		displayText: ":kekw:",
		style:       tcell.StyleDefault,
		previewURL:  url,
	})
	m.rebuild()

	item, ok := m.Builder()(0, 0).(*mentionsListRowItem)
	if !ok {
		t.Fatalf("expected builder item to be a row item, got %T", m.Builder()(0, 0))
	}
	if item.preview == nil {
		t.Fatal("expected overlay emoji preview to be enabled")
	}
	if item.item.previewURL != url {
		t.Fatalf("expected emoji preview URL metadata %q, got %q", url, item.item.previewURL)
	}
}

func TestMentionsListScanAndDrawEmotesRequestsCache(t *testing.T) {
	chat := newTestModel()
	chat.cfg.InlineImages.Enabled = true
	m := newMentionsList(chat.cfg, chat)
	m.cfg.InlineImages.Enabled = true
	m.SetRect(0, 0, 20, 3)
	m.append(mentionsListItem{
		insertText:  "<:kekw:123>",
		displayText: ":kekw:",
		style:       tcell.StyleDefault,
		previewURL:  "https://cdn.discordapp.com/emojis/123.png",
	})
	m.rebuild()

	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png.Encode: %v", err)
	}
	url := "https://cdn.discordapp.com/emojis/123.png"
	rt := &mockTransport{
		roundTrip: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader(buf.Bytes())),
				Header:     make(http.Header),
			}, nil
		},
	}
	m.imageCache = imgpkg.NewCache(&http.Client{Transport: rt})

	screen := &completeMockScreen{}
	m.Draw(screen)

	deadline := time.Now().Add(300 * time.Millisecond)
	for !m.imageCache.Requested(url) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !m.imageCache.Requested(url) {
		t.Fatal("expected emoji preview cache request")
	}
}
