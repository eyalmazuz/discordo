package markdown

// inlineCustomEmojiPlaceholder is a single grapheme with width 2, so wrapped
// layout reserves exactly one emote slot without introducing extra spacing.
const inlineCustomEmojiPlaceholder = "　"

// CustomEmojiText returns the text used to reserve layout for a custom emoji.
// In inline-image mode we use a fixed-width placeholder; otherwise we keep the
// shortcode text for text-only rendering.
func CustomEmojiText(name string, inlineImagesEnabled bool) string {
	if inlineImagesEnabled {
		return inlineCustomEmojiPlaceholder
	}
	return ":" + name + ":"
}
