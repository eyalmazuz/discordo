package chat

import (
	"testing"

	"github.com/ayn2op/tview"
	"github.com/gdamore/tcell/v3"
)

func TestMentionsListHelpers(t *testing.T) {
	m := newMentionsList(newMockChatModel().cfg)

	m.rebuild()
	if got := m.Cursor(); got != -1 {
		t.Fatalf("expected empty list cursor to be -1, got %d", got)
	}
	if text, ok := m.selectedInsertText(); ok || text != "" {
		t.Fatalf("expected no insert text from empty list, got %q ok=%v", text, ok)
	}

	m.append(mentionsListItem{insertText: "alpha", displayText: "Alpha", style: tcell.StyleDefault})
	m.append(mentionsListItem{insertText: "beta", displayText: "BetaUser", style: tcell.StyleDefault})
	m.rebuild()

	if item := m.Builder(-1, 0); item != nil {
		t.Fatal("expected negative builder index to return nil")
	}
	selectedItem, ok := m.Builder(0, 0).(*tview.TextView)
	if !ok {
		t.Fatalf("expected selected builder item to be a text view, got %T", m.Builder(0, 0))
	}
	selectedLines := selectedItem.GetLines()
	if len(selectedLines) != 1 || len(selectedLines[0]) != 1 || selectedLines[0][0].Text != "Alpha" {
		t.Fatalf("expected selected builder item to render Alpha, got %#v", selectedLines)
	}
	if attrs := selectedLines[0][0].Style.GetAttributes(); attrs&tcell.AttrReverse == 0 {
		t.Fatal("expected selected builder item style to be reversed")
	}

	unselectedItem, ok := m.Builder(1, 0).(*tview.TextView)
	if !ok {
		t.Fatalf("expected unselected builder item to be a text view, got %T", m.Builder(1, 0))
	}
	unselectedLines := unselectedItem.GetLines()
	if len(unselectedLines) != 1 || len(unselectedLines[0]) != 1 || unselectedLines[0][0].Text != "BetaUser" {
		t.Fatalf("expected unselected builder item to render BetaUser, got %#v", unselectedLines)
	}
	if attrs := unselectedLines[0][0].Style.GetAttributes(); attrs&tcell.AttrReverse != 0 {
		t.Fatal("expected unselected builder item style to remain non-reversed")
	}
	if item := m.Builder(2, 0); item != nil {
		t.Fatal("expected out-of-range builder index to return nil")
	}

	if got := m.itemCount(); got != 2 {
		t.Fatalf("expected 2 mentions, got %d", got)
	}
	if got := m.Cursor(); got != 0 {
		t.Fatalf("expected rebuilt list to select first item, got %d", got)
	}
	if text, ok := m.selectedInsertText(); !ok || text != "alpha" {
		t.Fatalf("expected selected insert text alpha, got %q ok=%v", text, ok)
	}
	if got := m.maxDisplayWidth(); got < len("BetaUser") {
		t.Fatalf("expected max display width to cover longest item, got %d", got)
	}

	m.clear()
	if got := m.itemCount(); got != 0 {
		t.Fatalf("expected cleared list to be empty, got %d", got)
	}
}
