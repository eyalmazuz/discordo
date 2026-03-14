package chat

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/ayn2op/tview"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/gdamore/tcell/v3"
)

const testPinnedMessagesLayerName = "pinnedMessages"

func TestModel_HandleEvent_PinnedMessagesKeyOpensPopup(t *testing.T) {
	channel := &discord.Channel{ID: 200, Type: discord.DirectMessage}
	pins := []discord.Message{
		{ID: 301, ChannelID: channel.ID, Content: "first pinned message", Pinned: true, Author: discord.User{ID: 2, Username: "alice"}},
		{ID: 302, ChannelID: channel.ID, Content: "second pinned message", Pinned: true, Author: discord.User{ID: 3, Username: "bob"}},
	}
	transport := &mockTransport{
		roundTrip: func(req *http.Request) (*http.Response, error) {
			if req.Method == http.MethodGet && strings.HasSuffix(req.URL.Path, "/channels/200/pins") {
				return jsonHTTPResponse(t, pins), nil
			}
			return jsonHTTPResponse(t, []discord.Message{}), nil
		},
	}
	m := newTestModelWithTransport(transport)
	m.SetSelectedChannel(channel)

	cmd := m.HandleEvent(tcell.NewEventKey(tcell.KeyCtrlP, "", tcell.ModNone))
	if _, ok := cmd.(tview.RedrawCommand); !ok {
		t.Fatalf("expected redraw command for pinned messages key, got %T", cmd)
	}
	if !m.HasLayer(testPinnedMessagesLayerName) {
		t.Fatal("expected pinned messages popup layer to be visible")
	}
	if transport.method != http.MethodGet || !strings.Contains(transport.path, "/channels/200/pins") {
		t.Fatalf("expected pinned messages fetch, got %s %s", transport.method, transport.path)
	}

	lines := renderPrimitiveLines(t, m.GetLayer(testPinnedMessagesLayerName))
	flat := strings.Join(lines, "\n")
	if !strings.Contains(flat, "first pinned message") || !strings.Contains(flat, "second pinned message") {
		t.Fatalf("expected pinned messages to render in the popup, got:\n%s", flat)
	}
}

func TestModel_HandleEvent_PinnedMessagesKeyIgnoresVisibleMentionsList(t *testing.T) {
	m := newTestModel()
	channel := &discord.Channel{ID: 200, Type: discord.DirectMessage}
	m.SetSelectedChannel(channel)
	m.app.SetFocus(m.messageInput)

	m.messageInput.mentionsList.append(mentionsListItem{insertText: "alice", displayText: "Alice", style: tcell.StyleDefault})
	m.messageInput.mentionsList.append(mentionsListItem{insertText: "bob", displayText: "Bob", style: tcell.StyleDefault})
	m.messageInput.mentionsList.rebuild()
	m.ShowLayer(mentionsListLayerName).SendToFront(mentionsListLayerName)
	m.messageInput.mentionsList.SetCursor(1)

	cmd := m.HandleEvent(tcell.NewEventKey(tcell.KeyCtrlP, "", tcell.ModNone))
	if _, ok := cmd.(tview.RedrawCommand); !ok {
		t.Fatalf("expected ctrl+p with visible mentions list to redraw, got %T", cmd)
	}
	if m.HasLayer(testPinnedMessagesLayerName) {
		t.Fatal("expected ctrl+p not to open pinned messages while mentions list is visible")
	}
	if got := m.messageInput.mentionsList.Cursor(); got != 0 {
		t.Fatalf("expected ctrl+p to keep navigating mentions list, got cursor %d", got)
	}
}

func TestModel_HandleEvent_PinnedMessagesKeyIgnoresVisibleEmojiAutocomplete(t *testing.T) {
	m := newTestModel()
	channel := &discord.Channel{ID: 210, GuildID: 211, Type: discord.GuildText}
	m.SetSelectedChannel(channel)
	m.app.SetFocus(m.messageInput)
	m.messageInput.SetDisabled(false)
	m.state.Cabinet.EmojiSet(channel.GuildID, []discord.Emoji{
		{ID: 1, Name: "kekw"},
		{ID: 2, Name: "kekwait"},
	}, false)

	m.messageInput.SetText(":kek", true)
	m.messageInput.tabSuggestion()
	if !m.GetVisible(mentionsListLayerName) {
		t.Fatal("expected emoji autocomplete popup to be visible")
	}
	m.messageInput.mentionsList.SetCursor(1)

	cmd := m.HandleEvent(tcell.NewEventKey(tcell.KeyCtrlP, "", tcell.ModNone))
	if _, ok := cmd.(tview.RedrawCommand); !ok {
		t.Fatalf("expected ctrl+p with visible emoji autocomplete to redraw, got %T", cmd)
	}
	if m.HasLayer(testPinnedMessagesLayerName) {
		t.Fatal("expected ctrl+p not to open pinned messages while emoji autocomplete is visible")
	}
	if got := m.messageInput.mentionsList.Cursor(); got != 0 {
		t.Fatalf("expected ctrl+p to keep navigating emoji autocomplete, got cursor %d", got)
	}
}

func TestModel_HandleEvent_PinnedMessagesPopupEnterJumpsToMessage(t *testing.T) {
	channel := &discord.Channel{ID: 200, Type: discord.DirectMessage}
	pins := []discord.Message{
		{ID: 301, ChannelID: channel.ID, Content: "jump target", Pinned: true, Author: discord.User{ID: 2, Username: "alice"}},
	}
	window := []discord.Message{
		{ID: 300, ChannelID: channel.ID, Content: "before", Author: discord.User{ID: 2, Username: "alice"}},
		{ID: 301, ChannelID: channel.ID, Content: "jump target", Pinned: true, Author: discord.User{ID: 2, Username: "alice"}},
		{ID: 302, ChannelID: channel.ID, Content: "after", Author: discord.User{ID: 2, Username: "alice"}},
	}
	transport := &mockTransport{
		roundTrip: func(req *http.Request) (*http.Response, error) {
			switch {
			case req.Method == http.MethodGet && strings.HasSuffix(req.URL.Path, "/channels/200/pins"):
				return jsonHTTPResponse(t, pins), nil
			case req.Method == http.MethodGet && strings.Contains(req.URL.Path, "/channels/200/messages"):
				return jsonHTTPResponse(t, window), nil
			default:
				return jsonHTTPResponse(t, []discord.Message{}), nil
			}
		},
	}
	m := newTestModelWithTransport(transport)
	m.SetSelectedChannel(channel)

	m.HandleEvent(tcell.NewEventKey(tcell.KeyCtrlP, "", tcell.ModNone))
	executeModelCommand(m, m.HandleEvent(tcell.NewEventKey(tcell.KeyEnter, "", tcell.ModNone)))

	if m.HasLayer(testPinnedMessagesLayerName) {
		t.Fatal("expected pinned messages popup to close after selecting a pin")
	}

	selected, err := m.messagesList.selectedMessage()
	if err != nil {
		t.Fatalf("expected selected message after jumping to pin, got %v", err)
	}
	if selected.ID != 301 {
		t.Fatalf("expected jump to select message 301, got %v", selected.ID)
	}
}

func TestModel_HandleEvent_PinnedMessagesPopupDeleteFlows(t *testing.T) {
	channel := &discord.Channel{ID: 200, Type: discord.DirectMessage}
	pins := []discord.Message{
		{ID: 301, ChannelID: channel.ID, Content: "remove me", Pinned: true, Author: discord.User{ID: 2, Username: "alice"}},
	}

	t.Run("lowercase d opens confirm and no cancels", func(t *testing.T) {
		transport := &mockTransport{
			roundTrip: func(req *http.Request) (*http.Response, error) {
				if req.Method == http.MethodGet && strings.HasSuffix(req.URL.Path, "/channels/200/pins") {
					return jsonHTTPResponse(t, pins), nil
				}
				return jsonHTTPResponse(t, []discord.Message{}), nil
			},
		}
		m := newTestModelWithTransport(transport)
		m.SetSelectedChannel(channel)

		m.HandleEvent(tcell.NewEventKey(tcell.KeyCtrlP, "", tcell.ModNone))
		cmd := m.HandleEvent(tcell.NewEventKey(tcell.KeyRune, "d", tcell.ModNone))
		if _, ok := cmd.(tview.RedrawCommand); !ok {
			t.Fatalf("expected redraw command for delete-confirm pin key, got %T", cmd)
		}
		if !m.HasLayer(confirmModalLayerName) {
			t.Fatal("expected unpin confirmation dialog to be visible")
		}

		lines := renderPrimitiveLines(t, m.GetLayer(confirmModalLayerName))
		flat := strings.Join(lines, "\n")
		if !strings.Contains(flat, "remove") || !strings.Contains(flat, "pin") {
			t.Fatalf("expected unpin confirmation prompt, got:\n%s", flat)
		}

		m.Focus(func(p tview.Primitive) {
			m.app.SetFocus(p)
		})
		executeModelCommand(m, m.HandleEvent(tcell.NewEventKey(tcell.KeyTab, "", tcell.ModNone)))
		executeModelCommand(m, m.HandleEvent(tcell.NewEventKey(tcell.KeyEnter, "", tcell.ModNone)))

		if m.HasLayer(confirmModalLayerName) {
			t.Fatal("expected unpin confirmation dialog to close after cancellation")
		}
		if !m.HasLayer(testPinnedMessagesLayerName) {
			t.Fatal("expected pinned messages popup to remain open after cancelling unpin")
		}
		if transport.method == http.MethodDelete && strings.Contains(transport.path, "/pins/301") {
			t.Fatalf("expected cancel not to unpin, got %s %s", transport.method, transport.path)
		}
	})

	t.Run("lowercase d then enter removes the pin", func(t *testing.T) {
		transport := &mockTransport{
			roundTrip: func(req *http.Request) (*http.Response, error) {
				switch {
				case req.Method == http.MethodGet && strings.HasSuffix(req.URL.Path, "/channels/200/pins"):
					return jsonHTTPResponse(t, pins), nil
				case req.Method == http.MethodDelete && strings.HasSuffix(req.URL.Path, "/channels/200/pins/301"):
					return &http.Response{
						StatusCode: http.StatusNoContent,
						Body:       io.NopCloser(strings.NewReader("")),
						Header:     make(http.Header),
					}, nil
				default:
					return jsonHTTPResponse(t, []discord.Message{}), nil
				}
			},
		}
		m := newTestModelWithTransport(transport)
		m.SetSelectedChannel(channel)

		m.HandleEvent(tcell.NewEventKey(tcell.KeyCtrlP, "", tcell.ModNone))
		m.HandleEvent(tcell.NewEventKey(tcell.KeyRune, "d", tcell.ModNone))
		m.Focus(func(p tview.Primitive) {
			m.app.SetFocus(p)
		})
		executeModelCommand(m, m.HandleEvent(tcell.NewEventKey(tcell.KeyEnter, "", tcell.ModNone)))

		if transport.method != http.MethodDelete || !strings.Contains(transport.path, "/channels/200/pins/301") {
			t.Fatalf("expected confirm unpin to delete the pin, got %s %s", transport.method, transport.path)
		}
	})

	t.Run("uppercase D removes the pin without a prompt", func(t *testing.T) {
		transport := &mockTransport{
			roundTrip: func(req *http.Request) (*http.Response, error) {
				switch {
				case req.Method == http.MethodGet && strings.HasSuffix(req.URL.Path, "/channels/200/pins"):
					return jsonHTTPResponse(t, pins), nil
				case req.Method == http.MethodDelete && strings.HasSuffix(req.URL.Path, "/channels/200/pins/301"):
					return &http.Response{
						StatusCode: http.StatusNoContent,
						Body:       io.NopCloser(strings.NewReader("")),
						Header:     make(http.Header),
					}, nil
				default:
					return jsonHTTPResponse(t, []discord.Message{}), nil
				}
			},
		}
		m := newTestModelWithTransport(transport)
		m.SetSelectedChannel(channel)

		m.HandleEvent(tcell.NewEventKey(tcell.KeyCtrlP, "", tcell.ModNone))
		cmd := m.HandleEvent(tcell.NewEventKey(tcell.KeyRune, "D", tcell.ModNone))
		if _, ok := cmd.(tview.RedrawCommand); !ok {
			t.Fatalf("expected redraw command for force-unpin key, got %T", cmd)
		}
		if m.HasLayer(confirmModalLayerName) {
			t.Fatal("expected force-unpin to skip the confirmation dialog")
		}
		if transport.method != http.MethodDelete || !strings.Contains(transport.path, "/channels/200/pins/301") {
			t.Fatalf("expected force-unpin to delete the pin, got %s %s", transport.method, transport.path)
		}
	})
}

func jsonHTTPResponse(t *testing.T, v any) *http.Response {
	t.Helper()

	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("failed to marshal response body: %v", err)
	}

	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(data)),
		Header:     make(http.Header),
	}
}
