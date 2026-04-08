package markdown

import (
	"testing"
)

func TestStandardEmoji(t *testing.T) {
	tests := []struct {
		name     string
		expected string
	}{
		{"thumbsup", "👍"},
		{"smile", "😄"},
		{"+1", "👍"},
		{"heart", "❤️"}, // Note: Gemoji might have variation selector
		{"unknown", ":unknown:"},
		{"👍", "👍"},
	}

	for _, tt := range tests {
		got := StandardEmoji(tt.name)
		// We use contains or similar if variation selectors are an issue, 
		// but let's see what Gemoji has.
		if got != tt.expected && got != tt.expected+"\ufe0f" {
			t.Errorf("StandardEmoji(%q) = %q, expected %q", tt.name, got, tt.expected)
		}
	}
}

func TestTwemojiURL(t *testing.T) {
	tests := []struct {
		emoji    string
		expected string
	}{
		{"👍", "https://cdn.jsdelivr.net/gh/jdecked/twemoji@15.0.3/assets/72x72/1f44d.png"},
		{"smile", "https://cdn.jsdelivr.net/gh/jdecked/twemoji@15.0.3/assets/72x72/1f604.png"},
		{"❤️", "https://cdn.jsdelivr.net/gh/jdecked/twemoji@15.0.3/assets/72x72/2764.png"},
	}

	for _, tt := range tests {
		got := TwemojiURL(tt.emoji)
		if got != tt.expected {
			t.Errorf("TwemojiURL(%q) = %q, expected %q", tt.emoji, got, tt.expected)
		}
	}
}

func TestStandardEmojisList(t *testing.T) {
	if len(StandardEmojis) == 0 {
		t.Fatal("StandardEmojis list is empty")
	}
	t.Logf("Loaded %d standard emojis", len(StandardEmojis))
}
