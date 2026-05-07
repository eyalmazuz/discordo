package chat

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"testing"

	imgpkg "github.com/ayn2op/discordo/internal/image"
	"github.com/ayn2op/tview/keybind"
	"github.com/ayn2op/tview/layers"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/gdamore/tcell/v3"
)

func TestMessageInputExpandsCustomEmojiOutsideCode(t *testing.T) {
	m := newTestModel()
	channel := &discord.Channel{ID: 10, GuildID: 100, Type: discord.GuildText}
	m.state.Cabinet.EmojiSet(channel.GuildID, []discord.Emoji{
		{ID: 123, Name: "party"},
		{ID: 456, Name: "dance", Animated: true},
	}, false)

	got := m.messageInput.processText(channel, []byte("go :party: :dance: `:party:`"))
	want := "go <:party:123> <a:dance:456> `:party:`"
	if got != want {
		t.Fatalf("unexpected processed text:\n got %q\nwant %q", got, want)
	}
}

func TestMessageInputSuggestsEmojiAutocomplete(t *testing.T) {
	m := newTestModel()
	channel := &discord.Channel{ID: 10, GuildID: 100, Type: discord.GuildText}
	m.SetSelectedChannel(channel)
	m.state.Cabinet.EmojiSet(channel.GuildID, []discord.Emoji{{ID: 123, Name: "party"}}, false)

	cmd := m.messageInput.suggestEmojis(channel, "par")
	if cmd == nil {
		t.Fatal("expected emoji suggestions to show the mentions list")
	}
	if got := m.messageInput.mentionsList.itemCount(); got != 1 {
		t.Fatalf("expected one emoji suggestion, got %d", got)
	}
	item := m.messageInput.mentionsList.items[0]
	if item.insertText != ":party:" || item.displayText != ":party:" || item.previewURL == "" {
		t.Fatalf("unexpected emoji suggestion item: %#v", item)
	}
}

func TestMentionsListEmojiPreviewUsesViewModel(t *testing.T) {
	m := newTestModel()
	m.cfg.InlineImages.Enabled = true
	m.cfg.InlineImages.Renderer = "kitty"
	list := newMentionsList(m.cfg, m)

	url := "https://cdn.discordapp.com/emojis/123.png"
	list.imageCache = imgpkg.NewCache(&http.Client{Transport: pngTransport(t)})
	list.append(mentionsListItem{
		insertText:  ":party:",
		displayText: ":party:",
		style:       tcell.StyleDefault,
		previewURL:  url,
	})
	list.rebuild()

	row, ok := list.Builder()(0, 0).(*mentionsListRowItem)
	if !ok {
		t.Fatalf("expected mentions list row item, got %T", list.Builder()(0, 0))
	}
	if row.preview == nil {
		t.Fatal("expected emoji suggestion row to carry an image preview")
	}
	if !list.imageCache.Requested(url) {
		t.Fatal("expected preview image request to be queued")
	}
}

func TestChatHelpIncludesOverlaysAndPopupHelp(t *testing.T) {
	m := newTestModel()
	channel := &discord.Channel{ID: 10, GuildID: 100, Type: discord.GuildText}
	m.SetSelectedChannel(channel)

	if !hasKeybind(m.baseShortHelp(), m.cfg.Keybinds.ToggleMessageSearch.Keybind) {
		t.Fatal("expected base short help to include message search when a channel is selected")
	}
	if !hasKeybind(m.baseShortHelp(), m.cfg.Keybinds.TogglePinnedMessages.Keybind) {
		t.Fatal("expected base short help to include pinned messages when a channel is selected")
	}

	m.AddLayer(m.messageSearch, layers.WithName(messageSearchLayerName), layers.WithVisible(true))
	if m.activeKeyMap() != m.messageSearch {
		t.Fatal("expected message search overlay to provide active help")
	}
}

func hasKeybind(items []keybind.Keybind, want keybind.Keybind) bool {
	wantHelp := want.Help()
	for _, item := range items {
		if item.Help() == wantHelp {
			return true
		}
	}
	return false
}

func pngTransport(t *testing.T) http.RoundTripper {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png encode: %v", err)
	}
	return &mockTransport{
		roundTrip: func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader(buf.Bytes())),
				Header:     make(http.Header),
			}, nil
		},
	}
}
