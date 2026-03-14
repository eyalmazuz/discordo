package markdown

import (
	"errors"
	"strings"
	"testing"

	"github.com/alecthomas/chroma/v2"
	"github.com/ayn2op/discordo/internal/config"
	"github.com/ayn2op/tview"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/ningen/v3/discordmd"
	"github.com/gdamore/tcell/v3"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
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
					break
				}
			}

			if !found {
				t.Errorf("expected text %q not found in segments", tt.expected)
			}
		})
	}
}

func TestRendererRenderLinesMarkdownStructures(t *testing.T) {
	cfg, _ := config.Load("")
	r := NewRenderer(cfg)
	source := []byte("# Title\n- first\n- second\n[site](https://example.com)\n<https://golang.org>\n")
	root := goldmark.New().Parser().Parse(text.NewReader(source))

	lines := r.RenderLines(source, root, tcell.StyleDefault)
	flat := flattenRenderedText(lines)

	for _, want := range []string{"# Title", "- first", "- second", "site", "https://golang.org"} {
		if !strings.Contains(flat, want) {
			t.Fatalf("rendered output %q does not contain %q", flat, want)
		}
	}
}

func TestRendererRenderLinesFencedCodeBlockHeaders(t *testing.T) {
	cfg, _ := config.Load("")
	r := NewRenderer(cfg)

	tests := []struct {
		name        string
		source      string
		wantContain string
		wantAbsent  string
	}{
		{
			name:        "missing language shows generic header",
			source:      "```\nplain text\n```",
			wantContain: codeBlockIndent + "code",
		},
		{
			name:        "unknown language shows declared header",
			source:      "```notreal\nplain text\n```",
			wantContain: codeBlockIndent + "code: notreal",
		},
		{
			name:        "analyzed language shows analyzed header",
			source:      "```notreal\npackage main\n```",
			wantContain: codeBlockIndent + "code: analyzed",
		},
		{
			name:       "known language omits fallback header",
			source:     "```go\npackage main\n```",
			wantAbsent: codeBlockIndent + "code",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := []byte(tt.source)
			root := goldmark.New().Parser().Parse(text.NewReader(source))
			lines := r.RenderLines(source, root, tcell.StyleDefault)
			flat := flattenRenderedText(lines)
			if tt.wantContain != "" && !strings.Contains(flat, tt.wantContain) {
				t.Fatalf("rendered output %q does not contain %q", flat, tt.wantContain)
			}
			if tt.wantAbsent != "" && strings.Contains(flat, tt.wantAbsent) {
				t.Fatalf("rendered output %q unexpectedly contains %q", flat, tt.wantAbsent)
			}
		})
	}
}

func TestRendererRenderLinesFencedCodeBlockFallbacks(t *testing.T) {
	cfg, _ := config.Load("")
	r := NewRenderer(cfg)

	t.Run("tokenise failure falls back to plain indented lines", func(t *testing.T) {
		oldTokeniseCodeBlock := tokeniseCodeBlock
		t.Cleanup(func() { tokeniseCodeBlock = oldTokeniseCodeBlock })
		tokeniseCodeBlock = func(chroma.Lexer, string) (chroma.Iterator, error) {
			return nil, errors.New("boom")
		}

		source := []byte("```go\nfmt.Println(\"hi\")\n```")
		root := goldmark.New().Parser().Parse(text.NewReader(source))
		lines := r.RenderLines(source, root, tcell.StyleDefault)
		flat := flattenRenderedText(lines)
		if !strings.Contains(flat, codeBlockIndent+"fmt.Println(\"hi\")") {
			t.Fatalf("expected tokenise fallback to emit raw indented code, got %q", flat)
		}
	})

	t.Run("missing theme falls back to chroma fallback theme", func(t *testing.T) {
		oldGetMarkdownTheme := getMarkdownTheme
		t.Cleanup(func() { getMarkdownTheme = oldGetMarkdownTheme })
		getMarkdownTheme = func(string) *chroma.Style { return nil }

		source := []byte("```go\npackage main\n```")
		root := goldmark.New().Parser().Parse(text.NewReader(source))
		lines := r.RenderLines(source, root, tcell.StyleDefault)
		flat := flattenRenderedText(lines)
		if !strings.Contains(flat, "package main") {
			t.Fatalf("expected fallback theme rendering to preserve code text, got %q", flat)
		}
	})
}

func TestApplyChromaStyle(t *testing.T) {
	base := tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorBlack)
	entry := chroma.MustParseStyleEntry("#112233 bold italic underline")

	got := applyChromaStyle(base, entry)
	wantFG := tcell.NewRGBColor(0x11, 0x22, 0x33)
	if got.GetForeground() != wantFG {
		t.Fatalf("foreground = %v, want %v", got.GetForeground(), wantFG)
	}
	if !got.HasBold() || !got.HasItalic() || !got.HasUnderline() {
		t.Fatalf("style flags = bold:%v italic:%v underline:%v, want all true", got.HasBold(), got.HasItalic(), got.HasUnderline())
	}
	
	// Test the 'chroma.No' branch
	entryNo := chroma.MustParseStyleEntry("nobold noitalic nounderline")
	gotNo := applyChromaStyle(base.Bold(true).Italic(true).Underline(true), entryNo)
	if gotNo.HasBold() || gotNo.HasItalic() || gotNo.HasUnderline() {
		t.Fatalf("expected style flags to be removed")
	}
}

func TestMentionText(t *testing.T) {
	tests := []struct {
		name string
		node *discordmd.Mention
		want string
	}{
		{
			name: "channel",
			node: &discordmd.Mention{Channel: &discord.Channel{Name: "general"}},
			want: "#general",
		},
		{
			name: "guild user prefers nick",
			node: &discordmd.Mention{GuildUser: &discord.GuildUser{
				User:   discord.User{Username: "user"},
				Member: &discord.Member{Nick: "nick"},
			}},
			want: "@nick",
		},
		{
			name: "role",
			node: &discordmd.Mention{GuildRole: &discord.Role{Name: "admins"}},
			want: "@admins",
		},
		{
			name: "empty",
			node: &discordmd.Mention{},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mentionText(tt.node); got != tt.want {
				t.Fatalf("mentionText() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestApplyInlineAttr(t *testing.T) {
	base := tcell.StyleDefault

	if got := applyInlineAttr(base, discordmd.AttrBold, false); !got.HasBold() {
		t.Fatal("bold attribute should enable bold")
	}
	if got := applyInlineAttr(base, discordmd.AttrItalics, false); !got.HasItalic() {
		t.Fatal("italics attribute should enable italic")
	}
	if got := applyInlineAttr(base, discordmd.AttrUnderline, false); !got.HasUnderline() {
		t.Fatal("underline attribute should enable underline")
	}
	if got := applyInlineAttr(base, discordmd.AttrStrikethrough, false); !got.HasStrikeThrough() {
		t.Fatal("strikethrough attribute should enable strike-through")
	}
	if got := applyInlineAttr(base, discordmd.AttrMonospace, false); !got.HasReverse() {
		t.Fatal("monospace outside links should enable reverse video")
	}
	if got := applyInlineAttr(base, discordmd.AttrMonospace, true); got.HasReverse() {
		t.Fatal("monospace inside links should not enable reverse video")
	}
	
	// Default branch
	if got := applyInlineAttr(base, 999, false); got != base {
		t.Fatal("unknown attribute should not modify style")
	}
}

func TestRenderer_OrderedListCounter(t *testing.T) {
	cfg, _ := config.Load("")
	r := NewRenderer(cfg)
	source := []byte("1. first\n2. second\n3. third\n")
	root := goldmark.New().Parser().Parse(text.NewReader(source))

	lines := r.RenderLines(source, root, tcell.StyleDefault)
	flat := flattenRenderedText(lines)

	for _, want := range []string{"1. first", "2. second", "3. third"} {
		if !strings.Contains(flat, want) {
			t.Fatalf("rendered output %q does not contain %q", flat, want)
		}
	}
}

func TestRenderer_LineBreaks(t *testing.T) {
	cfg, _ := config.Load("")
	r := NewRenderer(cfg)

	tests := []struct {
		name     string
		source   string
		expected string
	}{
		{
			name:     "Soft Line Break",
			source:   "line 1\nline 2",
			expected: "line 1\nline 2",
		},
		{
			name:     "Hard Line Break",
			source:   "line 1  \nline 2",
			expected: "line 1\n\nline 2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := []byte(tt.source)
			root := goldmark.New().Parser().Parse(text.NewReader(src))
			lines := r.RenderLines(src, root, tcell.StyleDefault)
			flat := flattenRenderedText(lines)
			if !strings.Contains(flat, tt.expected) {
				t.Errorf("expected %q to contain %q", flat, tt.expected)
			}
		})
	}
}

func TestRenderer_UnknownTheme(t *testing.T) {
	cfg, _ := config.Load("")
	cfg.Markdown.Theme = "unknown-nonexistent-theme"
	r := NewRenderer(cfg)

	source := []byte("```go\npackage main\n```")
	root := goldmark.New().Parser().Parse(text.NewReader(source))
	
	lines := r.RenderLines(source, root, tcell.StyleDefault)
	if len(lines) == 0 {
		t.Fatal("expected rendered lines")
	}
}

func FuzzRenderLines(f *testing.F) {
	seeds := []string{
		"hello **world**",
		"```go\npackage main\n```",
		"- item one\n- item two\n",
		"[link](https://example.com)",
		"~~strikethrough~~ and _italic_",
		"",
		"   ",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	cfg, _ := config.Load("")
	r := NewRenderer(cfg)

	f.Fuzz(func(t *testing.T, input string) {
		src := []byte(input)
		root := goldmark.New().Parser().Parse(text.NewReader(src))
		_ = r.RenderLines(src, root, tcell.StyleDefault)
	})
}

func TestRenderer_NestedListIndentation(t *testing.T) {
	cfg, _ := config.Load("")
	r := NewRenderer(cfg)
	source := []byte("- top\n  - child\n")
	root := goldmark.New().Parser().Parse(text.NewReader(source))

	lines := r.RenderLines(source, root, tcell.StyleDefault)
	flat := flattenRenderedText(lines)
	if !strings.Contains(flat, "  - child") {
		t.Fatalf("expected indentation for nested list item, got %q", flat)
	}
}

func TestRenderer_LinkWithInline(t *testing.T) {
	cfg, _ := config.Load("")
	r := NewRenderer(cfg)
	source := []byte("[**bold link**](https://example.com)")
	// discordmd.NewExtension() might not be readily available for simple goldmark, 
	// but RenderLines handles the discordmd node types if they exist in the AST.
	doc := ast.NewDocument()
	para := ast.NewParagraph()
	doc.AppendChild(doc, para)
	link := ast.NewLink()
	link.Destination = []byte("https://example.com")
	para.AppendChild(para, link)
	inline := &discordmd.Inline{Attr: discordmd.AttrBold}
	link.AppendChild(link, inline)
	inline.AppendChild(inline, ast.NewTextSegment(text.NewSegment(3, 12))) // "**bold link**"
	
	r.RenderLines(source, doc, tcell.StyleDefault)
}

func TestRenderer_EmojiID_Branch(t *testing.T) {
	cfg, _ := config.Load("")
	r := NewRenderer(cfg)
	doc := ast.NewDocument()
	emoji := &discordmd.Emoji{ID: "123", Name: "custom"}
	doc.AppendChild(doc, emoji)
	
	lines := r.RenderLines(nil, doc, tcell.StyleDefault)
	if len(lines) == 0 {
		t.Fatal("expected rendered line for emoji")
	}
}

func TestRenderer_Mention_Branch(t *testing.T) {
	cfg, _ := config.Load("")
	r := NewRenderer(cfg)
	doc := ast.NewDocument()
	m := &discordmd.Mention{Channel: &discord.Channel{Name: "general"}}
	doc.AppendChild(doc, m)
	
	lines := r.RenderLines(nil, doc, tcell.StyleDefault)
	if len(lines) == 0 {
		t.Fatal("expected rendered line for mention")
	}
}

func TestRenderer_Document_Branch(t *testing.T) {
	cfg, _ := config.Load("")
	r := NewRenderer(cfg)
	doc := ast.NewDocument()
	// Document node itself is visited.
	r.RenderLines(nil, doc, tcell.StyleDefault)
}

func TestRenderer_MultiLineToken(t *testing.T) {
	cfg, _ := config.Load("")
	r := NewRenderer(cfg)
	source := []byte("```go\n`multi\nline`\n```")
	root := goldmark.New().Parser().Parse(text.NewReader(source))
	
	r.RenderLines(source, root, tcell.StyleDefault)
}

func flattenRenderedText(lines []tview.Line) string {
	var b strings.Builder
	for i, line := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		for _, segment := range line {
			b.WriteString(segment.Text)
		}
	}
	return b.String()
}
