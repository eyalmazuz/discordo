package chat

import (
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/ayn2op/tview"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/gdamore/tcell/v3"
)

type renderMockScreen struct {
	completeMockScreen
}

func (s *renderMockScreen) PutStr(x int, y int, str string) {
	s.PutStrStyled(x, y, str, tcell.StyleDefault)
}

func (s *renderMockScreen) PutStrStyled(x int, y int, str string, style tcell.Style) {
	for _, r := range str {
		s.SetContent(x, y, r, nil, style)
		x++
	}
}

func (s *renderMockScreen) Put(x int, y int, str string, style tcell.Style) (string, int) {
	s.PutStrStyled(x, y, str, style)
	return "", len(str)
}

func TestMessagesList_HandleEvent_PinKeyOpensConfirmationDialog(t *testing.T) {
	m := newTestModelWithTransport(&mockTransport{})
	ml := m.messagesList
	channel := &discord.Channel{ID: 99, Type: discord.DirectMessage}
	m.SetSelectedChannel(channel)
	ml.setMessages([]discord.Message{
		{ID: 11, ChannelID: channel.ID, Content: "pin me please", Author: discord.User{ID: 2, Username: "other"}},
	})
	ml.SetCursor(0)

	ml.HandleEvent(tcell.NewEventKey(tcell.KeyRune, "p", tcell.ModNone))
	if !m.HasLayer(confirmModalLayerName) {
		t.Fatal("expected pin confirmation dialog to be visible")
	}

	lines := renderPrimitiveLines(t, m.GetLayer(confirmModalLayerName))
	promptIndex := lineIndexContaining(lines, "Do you want to pin this message")
	helperIndex := lineIndexContaining(lines, "please verify again that this is the message you want to pin")
	messageIndex := lineIndexContaining(lines, "pin me please")
	buttonsIndex := lineIndexContaining(lines, "yes")
	if promptIndex < 0 || helperIndex < 0 || messageIndex < 0 || buttonsIndex < 0 {
		t.Fatalf("expected pin dialog text to render, got:\n%s", strings.Join(lines, "\n"))
	}
	if promptIndex >= messageIndex || helperIndex >= messageIndex {
		t.Fatalf("expected question and helper text above the message preview, got:\n%s", strings.Join(lines, "\n"))
	}
	if messageIndex >= buttonsIndex {
		t.Fatalf("expected message preview above the buttons, got:\n%s", strings.Join(lines, "\n"))
	}
	if !strings.Contains(strings.ToLower(lines[buttonsIndex]), "no") {
		t.Fatalf("expected buttons row to include yes/no, got %q", lines[buttonsIndex])
	}
}

func TestMessagesList_PinConfirmation_TabThenEnterCancels(t *testing.T) {
	transport := &mockTransport{}
	m := newTestModelWithTransport(transport)
	ml := m.messagesList
	channel := &discord.Channel{ID: 101, Type: discord.DirectMessage}
	m.SetSelectedChannel(channel)
	ml.setMessages([]discord.Message{
		{ID: 12, ChannelID: channel.ID, Content: "cancel this pin", Author: discord.User{ID: 2, Username: "other"}},
	})
	ml.SetCursor(0)

	ml.HandleEvent(tcell.NewEventKey(tcell.KeyRune, "p", tcell.ModNone))
	if !m.HasLayer(confirmModalLayerName) {
		t.Fatal("expected pin confirmation dialog to be visible")
	}

	m.Focus(func(p tview.Primitive) {
		m.app.SetFocus(p)
	})
	executeModelCommand(m, m.HandleEvent(tcell.NewEventKey(tcell.KeyTab, "", tcell.ModNone)))
	executeModelCommand(m, m.HandleEvent(tcell.NewEventKey(tcell.KeyEnter, "", tcell.ModNone)))

	if m.HasLayer(confirmModalLayerName) {
		t.Fatal("expected pin confirmation dialog to close after cancelling")
	}
	if transport.method != "" || transport.path != "" {
		t.Fatalf("expected cancel not to pin the message, got method=%q path=%q", transport.method, transport.path)
	}
}

func TestMessagesList_PinConfirmation_EnterPinsSelectedMessage(t *testing.T) {
	transport := &mockTransport{
		roundTrip: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusNoContent,
				Body:       io.NopCloser(strings.NewReader("")),
				Header:     make(http.Header),
			}, nil
		},
	}
	m := newTestModelWithTransport(transport)
	ml := m.messagesList
	channel := &discord.Channel{ID: 102, Type: discord.DirectMessage}
	m.SetSelectedChannel(channel)
	ml.setMessages([]discord.Message{
		{ID: 13, ChannelID: channel.ID, Content: "pin this one", Author: discord.User{ID: 2, Username: "other"}},
	})
	ml.SetCursor(0)

	ml.HandleEvent(tcell.NewEventKey(tcell.KeyRune, "p", tcell.ModNone))
	if !m.HasLayer(confirmModalLayerName) {
		t.Fatal("expected pin confirmation dialog to be visible")
	}

	m.Focus(func(p tview.Primitive) {
		m.app.SetFocus(p)
	})
	executeModelCommand(m, m.HandleEvent(tcell.NewEventKey(tcell.KeyEnter, "", tcell.ModNone)))

	if m.HasLayer(confirmModalLayerName) {
		t.Fatal("expected pin confirmation dialog to close after confirming")
	}
	if transport.method != http.MethodPut {
		t.Fatalf("expected confirm to use PUT, got %q", transport.method)
	}
	if !strings.Contains(transport.path, "/channels/102/pins/13") {
		t.Fatalf("expected pin request path, got %q", transport.path)
	}
}

func renderPrimitiveLines(t *testing.T, primitive tview.Primitive) []string {
	t.Helper()

	if primitive == nil {
		t.Fatal("expected primitive to be present")
	}

	screen := &renderMockScreen{}
	primitive.SetRect(0, 0, 80, 24)
	primitive.Draw(screen)

	return screenLines(screen.Content, 80, 24)
}

func screenLines(content map[string]rune, width, height int) []string {
	lines := make([]string, 0, height)
	for y := 0; y < height; y++ {
		row := make([]rune, width)
		for x := 0; x < width; x++ {
			row[x] = ' '
		}
		for key, r := range content {
			parts := strings.Split(key, ",")
			if len(parts) != 2 {
				continue
			}
			x, errX := strconv.Atoi(parts[0])
			yPos, errY := strconv.Atoi(parts[1])
			if errX != nil || errY != nil || yPos != y || x < 0 || x >= width {
				continue
			}
			row[x] = r
		}
		lines = append(lines, strings.TrimRight(string(row), " "))
	}
	return lines
}

func lineIndexContaining(lines []string, needle string) int {
	for index, line := range lines {
		if strings.Contains(line, needle) {
			return index
		}
	}
	return -1
}
