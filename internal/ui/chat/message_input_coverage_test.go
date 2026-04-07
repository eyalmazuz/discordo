package chat

import (
	"testing"

	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/gdamore/tcell/v3"
)

func TestMessageInputUpdateRoutesMentionNavigation(t *testing.T) {
	m := newMockChatModel()
	mi := m.messageInput
	mi.SetDisabled(false)
	m.cfg.AutocompleteLimit = 5
	m.SetSelectedChannel(&discord.Channel{ID: 10, Type: discord.DirectMessage})

	mi.mentionsList.append(mentionsListItem{insertText: "alice", displayText: "Alice", style: tcell.StyleDefault})
	mi.mentionsList.append(mentionsListItem{insertText: "bob", displayText: "Bob", style: tcell.StyleDefault})
	mi.mentionsList.rebuild()
	m.ShowLayer(mentionsListLayerName)

	mi.Update(tcell.NewEventKey(tcell.KeyCtrlN, "", tcell.ModNone))
	if got := mi.mentionsList.Cursor(); got != 1 {
		t.Fatalf("expected ctrl+n to move cursor down, got %d", got)
	}

	mi.Update(tcell.NewEventKey(tcell.KeyCtrlP, "", tcell.ModNone))
	if got := mi.mentionsList.Cursor(); got != 0 {
		t.Fatalf("expected ctrl+p to move cursor up, got %d", got)
	}
}

func TestMessageInputStopTabCompletionClearsOverlay(t *testing.T) {
	m := newMockChatModel()
	mi := m.messageInput
	mi.SetDisabled(false)
	m.cfg.AutocompleteLimit = 5

	mi.mentionsList.append(mentionsListItem{insertText: "alice", displayText: "Alice", style: tcell.StyleDefault})
	mi.mentionsList.rebuild()
	m.ShowLayer(mentionsListLayerName)

	cmd := mi.stopTabCompletion()
	if cmd == nil {
		t.Fatal("expected stopTabCompletion to return a focus command")
	}
	if mi.mentionsList.itemCount() != 0 {
		t.Fatal("expected stopTabCompletion to clear suggestions")
	}
	if m.GetVisible(mentionsListLayerName) {
		t.Fatal("expected stopTabCompletion to hide the mentions layer")
	}
}
