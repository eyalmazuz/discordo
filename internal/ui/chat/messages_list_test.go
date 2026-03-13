package chat

import (
	"testing"

	"github.com/ayn2op/discordo/internal/config"
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

type mockMessagesScreen struct {
	tcell.Screen
	urlAtXY string
}

func (m *mockMessagesScreen) Get(x, y int) (string, tcell.Style, int) {
	style := tcell.StyleDefault
	if m.urlAtXY != "" {
		style = style.Url(m.urlAtXY)
	}
	return " ", style, 1
}

func TestMessagesList_HandleEvent_MouseClick(t *testing.T) {
	ml := &messagesList{
		List: tview.NewList(),
	}
	// Mock screen with a URL at 10,10
	screen := &mockMessagesScreen{urlAtXY: "https://google.com"}
	ml.lastScreen = screen
	ml.SetRect(0, 0, 100, 100) // Ensure 10,10 is in rect

	// Simulate mouse click
	event := &tview.MouseEvent{
		EventMouse: *tcell.NewEventMouse(10, 10, tcell.ButtonPrimary, 0),
		Action:     tview.MouseLeftClick,
	}
	
	// We can't easily verify if openURL was called because it's a goroutine and openURL is a method.
	// But we can check if it returns RedrawCommand.
	cmd := ml.HandleEvent(event)
	if _, ok := cmd.(tview.RedrawCommand); !ok {
		t.Errorf("expected RedrawCommand after clicking a link")
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
