package picker

import (
	"testing"

	"github.com/ayn2op/tview"
	"github.com/ayn2op/tview/keybind"
	"github.com/gdamore/tcell/v3"
)

func TestPicker_ToggleFocus(t *testing.T) {
	p := New()

	var lastFocused tview.Primitive
	// Mock Application behavior: setting focus blurs previous and focuses new
	setFocus := func(pr tview.Primitive) {
		if pr == p.list {
			// In a real app, Application would call Blur on input and Focus on list
			// But here we just need HasFocus() to be correct for the next call.
			// tview.Box (which both inherit) has internal focus state.
			p.input.Blur()
			p.list.Focus(nil)
		} else {
			p.list.Blur()
			p.input.Focus(nil)
		}
		lastFocused = pr
	}
	p.SetFocusFunc(setFocus)

	p.SetKeyMap(&KeyMap{
		ToggleFocus: keybind.NewKeybind(keybind.WithKeys("tab")),
	})

	// Start with input focused
	p.input.Focus(nil)
	lastFocused = p.input

	event := tcell.NewEventKey(tcell.KeyTab, " ", tcell.ModNone)
	p.HandleEvent(event)

	if lastFocused != p.list {
		t.Errorf("Expected list to be focused after Tab on input")
	}

	p.HandleEvent(event)

	if lastFocused != p.input {
		t.Errorf("Expected input to be focused after Tab on list")
	}
}

func TestPicker_ListNavigation_HJKL(t *testing.T) {
	p := New()
	p.AddItem(Item{Text: "Item 1"})
	p.AddItem(Item{Text: "Item 2"})
	p.AddItem(Item{Text: "Item 3"})
	p.Update()

	p.list.Focus(nil)
	if p.list.Cursor() != 0 {
		t.Errorf("Initial cursor should be 0")
	}

	// Simulate 'j' when list is focused
	eventJ := tcell.NewEventKey(tcell.KeyRune, "j", tcell.ModNone)
	p.HandleEvent(eventJ)

	if p.list.Cursor() != 1 {
		t.Errorf("Expected cursor 1 after 'j', got %d", p.list.Cursor())
	}

	// Simulate 'k' when list is focused
	eventK := tcell.NewEventKey(tcell.KeyRune, "k", tcell.ModNone)
	p.HandleEvent(eventK)

	if p.list.Cursor() != 0 {
		t.Errorf("Expected cursor 0 after 'k', got %d", p.list.Cursor())
	}
}

func TestPicker_StyledLineItem(t *testing.T) {
	p := New()
	line := tview.Line{
		{Text: "preview", Style: tcell.StyleDefault.Url("https://example.com")},
		{Text: " - label", Style: tcell.StyleDefault},
	}
	p.AddItem(Item{Line: line, FilterText: "label"})
	p.Update()

	got := p.list.Builder(0, p.list.Cursor())
	tv, ok := got.(*tview.TextView)
	if !ok {
		t.Fatalf("expected TextView item, got %T", got)
	}

	lines := tv.GetLines()
	if len(lines) != 1 || len(lines[0]) != len(line) {
		t.Fatalf("unexpected rendered line shape: %#v", lines)
	}

	if lines[0][0].Text != "preview" {
		t.Fatalf("expected first segment text %q, got %q", "preview", lines[0][0].Text)
	}

	_, url := lines[0][0].Style.GetUrl()
	if url != "https://example.com" {
		t.Fatalf("expected URL metadata to be preserved, got %q", url)
	}
}
