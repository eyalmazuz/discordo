package markdown

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"github.com/diamondburned/arikawa/v3/discord"
)

// inlineCustomEmojiPlaceholder reserves layout for an emote.
// We use a full-width space (U+3000) which tview treats as width 2.
const inlineCustomEmojiPlaceholder = "　"

//go:embed emoji.json
var emojiData []byte

var (
	shortcodeToEmoji map[string]string
	emojiToShortcode map[string]string
	StandardEmojis   []discord.Emoji
)

type gemoji struct {
	Emoji   string   `json:"emoji"`
	Aliases []string `json:"aliases"`
}

func init() {
	var list []gemoji
	if err := json.Unmarshal(emojiData, &list); err != nil {
		slog.Error("failed to unmarshal emoji data", "err", err)
		return
	}

	shortcodeToEmoji = make(map[string]string)
	emojiToShortcode = make(map[string]string)
	for _, g := range list {
		for _, alias := range g.Aliases {
			shortcodeToEmoji[alias] = g.Emoji
		}
		// Store the first alias as the primary shortcode.
		// We normalize the emoji character by removing variation selectors for the mapping key.
		emojiToShortcode[normalizeEmoji(g.Emoji)] = g.Aliases[0]
		StandardEmojis = append(StandardEmojis, discord.Emoji{
			Name: g.Emoji,
		})
	}
}

// normalizeEmoji removes variation selectors (U+FE0F) from an emoji string.
func normalizeEmoji(emoji string) string {
	return strings.ReplaceAll(emoji, "\ufe0f", "")
}

// GetShortcode returns the primary shortcode for a standard emoji Unicode character.
func GetShortcode(emoji string) string {
	if s, ok := emojiToShortcode[normalizeEmoji(emoji)]; ok {
		return s
	}
	return ""
}

// TwemojiURL returns the Twemoji CDN URL for a given Unicode emoji character.
func TwemojiURL(emoji string) string {
	if unescaped, err := url.QueryUnescape(emoji); err == nil {
		emoji = unescaped
	} else if unescaped, err := url.PathUnescape(emoji); err == nil {
		emoji = unescaped
	}

	// If it's a shortcode, resolve it to the unicode character first.
	if e, ok := shortcodeToEmoji[strings.Trim(emoji, ":")]; ok {
		emoji = e
	}

	var hexes []string
	for _, r := range emoji {
		if r == 0xfe0f {
			continue // Skip variation selector
		}
		hexes = append(hexes, fmt.Sprintf("%x", r))
	}

	if len(hexes) == 0 {
		return ""
	}

	u := strings.Join(hexes, "-")
	return "https://cdn.jsdelivr.net/gh/jdecked/twemoji@15.0.3/assets/72x72/" + u + ".png"
}

// CustomEmojiText returns the text used to reserve layout for a custom emoji.
// In inline-image mode we use a fixed-width placeholder; otherwise we keep the
// shortcode text for text-only rendering.
func CustomEmojiText(name string, inlineImagesEnabled bool) string {
	if inlineImagesEnabled {
		return inlineCustomEmojiPlaceholder
	}
	return ":" + name + ":"
}

// StandardEmoji returns the Unicode character for a standard emoji shortcode if
// available. If not found, it returns the name wrapped in colons if it looks
// like a shortcode name; otherwise it returns the name as is.
func StandardEmoji(name string) string {
	if e, ok := shortcodeToEmoji[strings.Trim(name, ":")]; ok {
		return e
	}
	if len(name) > 0 && name[0] != ':' && !isUnicodeEmoji(name) {
		return ":" + name + ":"
	}
	return name
}

func isUnicodeEmoji(s string) bool {
	for _, r := range s {
		if r > 127 {
			return true
		}
	}
	return false
}
