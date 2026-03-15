package picker

import (
	"errors"
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
	p.SetRect(0, 0, 100, 100)

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

	// Simulate 'G' when list is focused
	p.HandleEvent(tcell.NewEventKey(tcell.KeyRune, "G", tcell.ModNone))
	if p.list.Cursor() != 2 {
		t.Errorf("Expected cursor 2 after 'G', got %d", p.list.Cursor())
	}

	// Simulate 'g' when list is focused
	p.HandleEvent(tcell.NewEventKey(tcell.KeyRune, "g", tcell.ModNone))
	if p.list.Cursor() != 0 {
		t.Errorf("Expected cursor 0 after 'g', got %d", p.list.Cursor())
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

func TestPicker_SettersAndClears(t *testing.T) {
	p := New()
	p.SetScrollBarVisibility(tview.ScrollBarVisibilityAlways)
	p.SetScrollBar(tview.NewScrollBar())

	p.SetSelectedFunc(func(item Item) {})
	p.SetCancelFunc(func() {})

	p.AddItem(Item{Text: "test"})
	p.Update()
	if p.FilteredCount() != 1 {
		t.Fatal("expected 1 item")
	}

	p.ClearItems()
	p.Update()
	if p.FilteredCount() != 0 {
		t.Fatal("expected 0 items after ClearItems")
	}

	p.AddItem(Item{Text: "test2"})
	p.Update()
	p.ClearList()
	if p.FilteredCount() != 0 {
		t.Fatal("expected 0 items after ClearList")
	}
}

func TestPicker_HandleEvent_Full(t *testing.T) {
	p := New()
	p.SetKeyMap(&KeyMap{
		Select: keybind.NewKeybind(keybind.WithKeys("enter")),
		Cancel: keybind.NewKeybind(keybind.WithKeys("esc")),
		Up:     keybind.NewKeybind(keybind.WithKeys("up")),
		Down:   keybind.NewKeybind(keybind.WithKeys("down")),
		Top:    keybind.NewKeybind(keybind.WithKeys("home")),
		Bottom: keybind.NewKeybind(keybind.WithKeys("end")),
	})
	p.AddItem(Item{Text: "item1"})
	p.AddItem(Item{Text: "item2"})
	p.AddItem(Item{Text: "item3"})
	p.Update()
	p.SetRect(0, 0, 100, 100)

	t.Run("KeyMapNavigation", func(t *testing.T) {
		p.list.Focus(nil)
		p.list.SetCursor(0)

		p.HandleEvent(tcell.NewEventKey(tcell.KeyDown, "", tcell.ModNone))
		if p.list.Cursor() != 1 {
			t.Errorf("expected cursor 1, got %d", p.list.Cursor())
		}

		p.HandleEvent(tcell.NewEventKey(tcell.KeyEnd, "", tcell.ModNone))
		if p.list.Cursor() != 2 {
			t.Errorf("expected cursor 2, got %d", p.list.Cursor())
		}

		p.HandleEvent(tcell.NewEventKey(tcell.KeyUp, "", tcell.ModNone))
		if p.list.Cursor() != 1 {
			t.Errorf("expected cursor 1, got %d", p.list.Cursor())
		}

		p.HandleEvent(tcell.NewEventKey(tcell.KeyHome, "", tcell.ModNone))
		if p.list.Cursor() != 0 {
			t.Errorf("expected cursor 0, got %d", p.list.Cursor())
		}
	})

	t.Run("Selection", func(t *testing.T) {
		selected := false
		p.SetSelectedFunc(func(item Item) { selected = true })
		p.list.Focus(nil)
		p.HandleEvent(tcell.NewEventKey(tcell.KeyEnter, " ", tcell.ModNone))
		if !selected {
			t.Fatal("expected selected func to be called")
		}
	})

	t.Run("Cancel", func(t *testing.T) {
		canceled := false
		p.SetCancelFunc(func() { canceled = true })
		p.HandleEvent(tcell.NewEventKey(tcell.KeyEsc, " ", tcell.ModNone))
		if !canceled {
			t.Fatal("expected cancel func to be called")
		}
	})
}

func TestPicker_onInputChanged(t *testing.T) {
	p := New()
	p.AddItem(Item{Text: "apple", FilterText: "apple"})
	p.AddItem(Item{Text: "banana", FilterText: "banana"})
	p.Update()

	p.onInputChanged("ap")
	if p.FilteredCount() != 1 {
		t.Fatalf("expected 1 item, got %d", p.FilteredCount())
	}

	p.onInputChanged("")
	if p.FilteredCount() != 2 {
		t.Fatalf("expected 2 items, got %d", p.FilteredCount())
	}
}

func TestPicker_SetFilteredItemsBuilderBranches(t *testing.T) {
	p := New()
	p.setFilteredItems(Items{
		{
			Builder: func(selected bool) tview.ListItem {
				return tview.NewTextView().SetText("built")
			},
		},
	})

	if got := p.list.Builder(0, 0); got == nil {
		t.Fatal("expected custom builder item")
	}
	if got := p.list.Builder(99, 0); got != nil {
		t.Fatalf("expected out-of-range builder to return nil, got %T", got)
	}
}

func TestPicker_HandleEvent_ExtraBranches(t *testing.T) {
	p := New()
	p.SetKeyMap(&KeyMap{
		Select:      keybind.NewKeybind(keybind.WithKeys("enter")),
		Cancel:      keybind.NewKeybind(keybind.WithKeys("esc")),
		ToggleFocus: keybind.NewKeybind(keybind.WithKeys("tab")),
	})

	if cmd := p.HandleEvent(tcell.NewEventKey(tcell.KeyTab, "", tcell.ModNone)); cmd != nil {
		t.Fatalf("expected toggle focus with nil focus func to return nil, got %T", cmd)
	}

	if cmd := p.HandleEvent(tcell.NewEventKey(tcell.KeyEnter, "", tcell.ModNone)); cmd != nil {
		t.Fatalf("expected select key with no selection callback to return nil, got %T", cmd)
	}

	if cmd := p.HandleEvent(tcell.NewEventKey(tcell.KeyEsc, "", tcell.ModNone)); cmd != nil {
		t.Fatalf("expected cancel key with no cancel callback to return nil, got %T", cmd)
	}

	p.input.Focus(nil)
	if cmd := p.HandleEvent(tcell.NewEventKey(tcell.KeyRune, "j", tcell.ModNone)); cmd != nil {
		t.Fatalf("expected rune navigation outside list focus to return nil, got %T", cmd)
	}
	if got := p.input.GetText(); got != "j" {
		t.Fatalf("expected rune navigation outside list focus to update input text, got %q", got)
	}

	if cmd := p.HandleEvent(tcell.NewEventError(errors.New("boom"))); cmd != nil {
		t.Fatalf("expected non-key event to fall through without command, got %T", cmd)
	}
}
