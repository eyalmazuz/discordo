package markdown

import "testing"

func TestCustomEmojiText(t *testing.T) {
	if got := CustomEmojiText("kekw", true); got != inlineCustomEmojiPlaceholder {
		t.Fatalf("expected inline emoji placeholder %q, got %q", inlineCustomEmojiPlaceholder, got)
	}

	if got := CustomEmojiText("kekw", false); got != ":kekw:" {
		t.Fatalf("expected text fallback %q, got %q", ":kekw:", got)
	}
}
