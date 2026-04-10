package markdown

import (
	"strconv"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/ayn2op/discordo/internal/config"
	"github.com/ayn2op/discordo/internal/ui"
	"github.com/eyalmazuz/tview"
	"github.com/diamondburned/ningen/v3/discordmd"
	"github.com/gdamore/tcell/v3"
	"github.com/yuin/goldmark/ast"
)

type Renderer struct {
	cfg *config.Config

	HideSpoilers bool

	listIx       *int
	listNested   int
	inSpoiler    int
	inBlockquote int
}

const codeBlockIndent = "    "

var (
	tokeniseCodeBlock = func(lexer chroma.Lexer, code string) (chroma.Iterator, error) {
		return lexer.Tokenise(nil, code)
	}
	getMarkdownTheme = styles.Get
)

func NewRenderer(cfg *config.Config) *Renderer {
	return &Renderer{cfg: cfg}
}

func (r *Renderer) writeObscured(builder *tview.LineBuilder, text string, style tcell.Style) {
	if r.HideSpoilers && r.inSpoiler > 0 {
		var sb strings.Builder
		for range text {
			sb.WriteString("█")
		}
		text = sb.String()
	}
	builder.Write(text, style)
}

func (r *Renderer) RenderLines(source []byte, node ast.Node, base tcell.Style) []tview.Line {
	r.listIx = nil
	r.listNested = 0
	r.inSpoiler = 0
	r.inBlockquote = 0

	builder := tview.NewLineBuilder()
	styleStack := []tcell.Style{base}
	linkDepth := 0

	currentStyle := func() tcell.Style {
		return styleStack[len(styleStack)-1]
	}
	pushStyle := func(style tcell.Style) {
		styleStack = append(styleStack, style)
	}
	popStyle := func() {
		if len(styleStack) > 1 {
			styleStack = styleStack[:len(styleStack)-1]
		}
	}

	newLine := func() {
		builder.NewLine()
		if r.inBlockquote > 0 {
			builder.Write(" ▎ ", currentStyle().Dim(true))
		}
	}

	theme := r.cfg.Theme.MessagesList
	_ = ast.Walk(node, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		switch node := node.(type) {
		case *ast.Document:
			// noop
		case *ast.Heading:
			if entering {
				builder.Write(strings.Repeat("#", node.Level)+" ", currentStyle())
			} else {
				newLine()
			}
		case *ast.Blockquote:
			if entering {
				if r.inBlockquote == 0 {
					builder.NewLine()
				} else {
					newLine()
				}
				r.inBlockquote++
				builder.Write(" ▎ ", currentStyle().Dim(true))
				pushStyle(currentStyle().Dim(true))
			} else {
				r.inBlockquote--
				popStyle()
				if r.inBlockquote == 0 {
					builder.NewLine()
				} else {
					newLine()
				}
			}
		case *ast.Text:
			if entering {
				r.renderTextWithEmojis(builder, string(node.Segment.Value(source)), currentStyle())
				switch {
				case node.HardLineBreak():
					newLine()
					newLine()
				case node.SoftLineBreak():
					newLine()
				}
			}
		case *ast.FencedCodeBlock:
			if entering {
				newLine()
				r.renderFencedCodeBlock(builder, source, node, currentStyle(), newLine)
			}
		case *ast.AutoLink:
			if entering {
				url := string(node.URL(source))
				style := ui.MergeStyle(currentStyle(), theme.URLStyle.Style).Url(url)
				r.writeObscured(builder, url, style)
			}
		case *ast.Link:
			if entering {
				url := string(node.Destination)
				linkDepth++
				pushStyle(ui.MergeStyle(currentStyle(), theme.URLStyle.Style).Url(url))
			} else {
				if linkDepth > 0 {
					linkDepth--
				}
				popStyle()
			}
		case *ast.List:
			if node.IsOrdered() {
				start := node.Start
				r.listIx = &start
			} else {
				r.listIx = nil
			}

			if entering {
				newLine()
				r.listNested++
			} else {
				r.listNested--
			}
		case *ast.ListItem:
			if entering {
				builder.Write(strings.Repeat("  ", r.listNested-1), currentStyle())
				if r.listIx != nil {
					builder.Write(strconv.Itoa(*r.listIx)+". ", currentStyle())
					*r.listIx++
				} else {
					builder.Write("- ", currentStyle())
				}
			} else {
				newLine()
			}
		case *discordmd.Inline:
			if entering {
				if (node.Attr & discordmd.AttrSpoiler) != 0 {
					r.inSpoiler++
				}
				pushStyle(applyInlineAttr(currentStyle(), node.Attr, linkDepth > 0))
			} else {
				if (node.Attr & discordmd.AttrSpoiler) != 0 {
					r.inSpoiler--
				}
				popStyle()
			}
		case *discordmd.Mention:
			if entering {
				r.writeObscured(builder, mentionText(node), ui.MergeStyle(currentStyle(), theme.MentionStyle.Style))
			}
		case *discordmd.Emoji:
			if entering {
				style := ui.MergeStyle(currentStyle(), theme.EmojiStyle.Style)
				if node.ID != "" {
					style = style.Url(node.EmojiURL())
					r.writeObscured(builder, CustomEmojiText(node.Name, r.cfg.InlineImages.Enabled), style)
					break
				}
				if r.cfg.InlineImages.Enabled {
					style = style.Url(TwemojiURL(node.Name))
					r.writeObscured(builder, CustomEmojiText(node.Name, r.cfg.InlineImages.Enabled), style)
				} else {
					r.writeObscured(builder, StandardEmoji(node.Name), style)
				}
			}
		}
		return ast.WalkContinue, nil
	})

	return builder.Finish()
}

func (r *Renderer) renderFencedCodeBlock(builder *tview.LineBuilder, source []byte, node *ast.FencedCodeBlock, base tcell.Style, newLine func()) {
	var code strings.Builder
	lines := node.Lines()
	for i := range lines.Len() {
		line := lines.At(i)
		code.Write(line.Value(source))
	}

	language := strings.TrimSpace(string(node.Language(source)))
	lexer := lexers.Get(language)
	declaredLanguageSupported := lexer != nil

	// Detect the language from its content.
	var analyzed bool
	if lexer == nil {
		lexer = lexers.Analyse(code.String())
		analyzed = lexer != nil
	}
	if lexer == nil {
		lexer = lexers.Fallback
	}

	// At this point, it should be noted that some lexers can be extremely chatty.
	// To mitigate this, use the coalescing lexer to coalesce runs of identical token types into a single token.
	lexer = chroma.Coalesce(lexer)

	// Show a fallback header when the language is omitted or unknown.
	headerStyle := base.Dim(true)
	if analyzed {
		builder.Write(codeBlockIndent+"code: analyzed", headerStyle)
		newLine()
	} else if language == "" {
		builder.Write(codeBlockIndent+"code", headerStyle)
		newLine()
	} else if !declaredLanguageSupported {
		builder.Write(codeBlockIndent+"code: "+language, headerStyle)
		newLine()
	}

	iterator, err := tokeniseCodeBlock(lexer, code.String())
	if err != nil {
		for i := range lines.Len() {
			line := lines.At(i)
			builder.Write(codeBlockIndent+string(line.Value(source)), base)
		}
		return
	}

	theme := getMarkdownTheme(r.cfg.Markdown.Theme)
	if theme == nil {
		theme = styles.Fallback
	}

	builder.Write(codeBlockIndent, base)
	for token := iterator(); token != chroma.EOF; token = iterator() {
		style := applyChromaStyle(base, theme.Get(token.Type))
		// Chroma tokens may include embedded newlines, so split and re-emit with indentation on each visual line.
		parts := strings.Split(token.Value, "\n")
		for i, part := range parts {
			if i > 0 {
				newLine()
				builder.Write(codeBlockIndent, base)
			}
			if part != "" {
				builder.Write(part, style)
			}
		}
	}
}

func applyChromaStyle(base tcell.Style, entry chroma.StyleEntry) tcell.Style {
	style := base
	if entry.Colour.IsSet() {
		style = style.Foreground(tcell.NewRGBColor(
			int32(entry.Colour.Red()),
			int32(entry.Colour.Green()),
			int32(entry.Colour.Blue()),
		))
	}
	// Intentionally do not apply token background colors so code blocks keep the user's terminal/chat background.
	// if entry.Background.IsSet() {
	// 	style = style.Background(tcell.NewRGBColor(
	// 		int32(entry.Background.Red()),
	// 		int32(entry.Background.Green()),
	// 		int32(entry.Background.Blue()),
	// 	))
	// }
	switch entry.Bold {
	case chroma.Yes:
		style = style.Bold(true)
	case chroma.No:
		style = style.Bold(false)
	}
	switch entry.Italic {
	case chroma.Yes:
		style = style.Italic(true)
	case chroma.No:
		style = style.Italic(false)
	}
	switch entry.Underline {
	case chroma.Yes:
		style = style.Underline(true)
	case chroma.No:
		style = style.Underline(false)
	}
	return style
}

func mentionText(node *discordmd.Mention) string {
	switch {
	case node.Channel != nil:
		return "#" + node.Channel.Name
	case node.GuildUser != nil:
		name := node.GuildUser.DisplayOrUsername()
		if member := node.GuildUser.Member; member != nil && member.Nick != "" {
			name = member.Nick
		}
		return "@" + name
	case node.GuildRole != nil:
		return "@" + node.GuildRole.Name
	default:
		return ""
	}
}

func applyInlineAttr(style tcell.Style, attr discordmd.Attribute, inLink bool) tcell.Style {
	switch attr {
	case discordmd.AttrBold:
		return style.Bold(true)
	case discordmd.AttrItalics:
		return style.Italic(true)
	case discordmd.AttrUnderline:
		return style.Underline(true)
	case discordmd.AttrStrikethrough:
		return style.StrikeThrough(true)
	case discordmd.AttrMonospace:
		// Avoid reverse-video inside links. Link labels like `hash` should still
		// look like links, not highlighted blocks.
		if inLink {
			return style
		}
		return style.Reverse(true)
	}
	return style
}

func (r *Renderer) renderTextWithEmojis(builder *tview.LineBuilder, text string, style tcell.Style) {
	if !r.cfg.InlineImages.Enabled {
		r.writeObscured(builder, text, style)
		return
	}

	runes := []rune(text)
	for i := 0; i < len(runes); {
		found := false
		// Try to match the longest emoji first (e.g. skin tones, ZWJ sequences)
		// Standard emojis in emoji.json are mostly 1-2 runes, but some are more.
		// Flag emojis are 2 runes.
		for l := 12; l > 0; l-- {
			if i+l > len(runes) {
				continue
			}
			part := string(runes[i : i+l])
			if _, ok := emojiToShortcode[normalizeEmoji(part)]; ok {
				// Found an emoji!
				r.writeObscured(builder, CustomEmojiText(part, true), style.Url(TwemojiURL(part)))
				i += l
				found = true
				break
			}
		}

		if !found {
			r.writeObscured(builder, string(runes[i]), style)
			i++
		}
	}
}
