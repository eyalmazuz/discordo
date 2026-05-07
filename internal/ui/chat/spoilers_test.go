package chat

import (
	"strings"
	"testing"

	"github.com/ayn2op/tview"
	"github.com/diamondburned/arikawa/v3/discord"
)

func linesText(lines []tview.Line) string {
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

func TestSpoilersHiddenUntilSelected(t *testing.T) {
	m := newTestModel()
	msg := discord.Message{ID: 1, Content: "before ||secret|| after", Author: discord.User{Username: "u"}}

	hidden := linesText(m.messagesList.renderMessage(msg, m.cfg.Theme.MessagesList.MessageStyle.Style, true))
	if strings.Contains(hidden, "secret") {
		t.Fatalf("expected spoiler content to be hidden, got %q", hidden)
	}
	if !strings.Contains(hidden, "██████") {
		t.Fatalf("expected hidden spoiler blocks, got %q", hidden)
	}

	revealed := linesText(m.messagesList.renderMessage(msg, m.cfg.Theme.MessagesList.SelectedMessageStyle.Style, false))
	if !strings.Contains(revealed, "secret") {
		t.Fatalf("expected selected message to reveal spoiler, got %q", revealed)
	}
}

func TestSpoilersHiddenWhenMarkdownDisabled(t *testing.T) {
	m := newTestModel()
	m.cfg.Markdown.Enabled = false
	msg := discord.Message{ID: 1, Content: "plain ||secret||", Author: discord.User{Username: "u"}}

	hidden := linesText(m.messagesList.renderMessage(msg, m.cfg.Theme.MessagesList.MessageStyle.Style, true))
	if strings.Contains(hidden, "secret") {
		t.Fatalf("expected markdown-disabled spoiler content to be hidden, got %q", hidden)
	}

	revealed := linesText(m.messagesList.renderMessage(msg, m.cfg.Theme.MessagesList.SelectedMessageStyle.Style, false))
	if !strings.Contains(revealed, "secret") {
		t.Fatalf("expected selected markdown-disabled message to reveal spoiler, got %q", revealed)
	}
}
