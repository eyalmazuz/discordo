package markdown

import (
	"testing"

	"github.com/ayn2op/discordo/internal/config"
	"github.com/diamondburned/ningen/v3/discordmd"
	"github.com/gdamore/tcell/v3"
	"github.com/yuin/goldmark/ast"
)

func TestRenderer_Emoji(t *testing.T) {
	cfg, _ := config.Load("")
	r := NewRenderer(cfg)

	tests := []struct {
		name         string
		inlineImages bool
		emoji        *discordmd.Emoji
		expected     string
		hasURL       bool
	}{
		{
			name: "Standard Emoji",
			emoji: &discordmd.Emoji{
				Name: "smile",
			},
			expected: ":smile:",
			hasURL:   false,
		},
		{
			name:         "Custom Emoji Text Fallback",
			inlineImages: false,
			emoji: &discordmd.Emoji{
				ID:   "123456",
				Name: "custom",
			},
			expected: ":custom:",
			hasURL:   true,
		},
		{
			name:         "Custom Emoji Inline Placeholder",
			inlineImages: true,
			emoji: &discordmd.Emoji{
				ID:   "123456",
				Name: "custom",
			},
			expected: inlineCustomEmojiPlaceholder,
			hasURL:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg.InlineImages.Enabled = tt.inlineImages
			doc := ast.NewDocument()
			doc.AppendChild(doc, tt.emoji)

			lines := r.RenderLines(nil, doc, tcell.StyleDefault)
			if len(lines) == 0 {
				t.Fatal("expected at least one line")
			}

			found := false
			for _, segment := range lines[0] {
				if segment.Text == tt.expected {
					found = true
					_, url := segment.Style.GetUrl()
					if tt.hasURL && url == "" {
						t.Error("expected URL metadata, got none")
					} else if !tt.hasURL && url != "" {
						t.Errorf("expected no URL metadata, got %q", url)
					}

					if tt.hasURL {
						expectedURL := tt.emoji.EmojiURL()
						if url != expectedURL {
							t.Errorf("expected URL %q, got %q", expectedURL, url)
						}
					}
					break
				}
			}

			if !found {
				t.Errorf("expected text %q not found in segments", tt.expected)
			}
		})
	}
}
