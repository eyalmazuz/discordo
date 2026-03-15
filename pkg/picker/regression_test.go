package picker

import (
	"testing"

	"github.com/eyalmazuz/tview/keybind"
	"github.com/gdamore/tcell/v3"
)

func TestRegression_PickerFilteringAndSelection(t *testing.T) {
	p := New()
	p.SetKeyMap(&KeyMap{
		Select: keybind.NewKeybind(keybind.WithKeys("enter")),
		Down:   keybind.NewKeybind(keybind.WithKeys("down")),
	})

	p.AddItem(Item{Text: "apple", FilterText: "apple", Reference: "apple"})
	p.AddItem(Item{Text: "apricot", FilterText: "apricot", Reference: "apricot"})
	p.AddItem(Item{Text: "banana", FilterText: "banana", Reference: "banana"})
	p.Update()

	var selectedItem Item
	p.SetSelectedFunc(func(item Item) {
		selectedItem = item
	})

	// 1. Initial state
	if p.FilteredCount() != 3 {
		t.Errorf("expected 3 items, got %d", p.FilteredCount())
	}

	// 2. Type "ap" to filter
	p.input.Focus(nil) // Ensure input is focused
	p.HandleEvent(tcell.NewEventKey(tcell.KeyRune, "a", tcell.ModNone))
	p.HandleEvent(tcell.NewEventKey(tcell.KeyRune, "p", tcell.ModNone))
	
	if p.FilteredCount() != 2 {
		t.Errorf("expected 2 items after filtering 'ap', got %d", p.FilteredCount())
	}

	// 3. Move down to "apricot"
	// We need to focus the list first if it's not focused, OR rely on global 'j' if configured.
	// Actually, picker handles input and list.
	p.list.Focus(nil)
	p.HandleEvent(tcell.NewEventKey(tcell.KeyRune, "j", tcell.ModNone)) // Default 'j' for down

	if p.list.Cursor() != 1 {
		t.Errorf("expected cursor to be 1, got %d", p.list.Cursor())
	}

	// 4. Select "apricot"
	p.HandleEvent(tcell.NewEventKey(tcell.KeyEnter, "", tcell.ModNone))

	if selectedItem.Reference != "apricot" {
		t.Errorf("expected 'apricot' to be selected, got %v", selectedItem.Reference)
	}
}

func TestRegression_PickerCancellation(t *testing.T) {
	p := New()
	p.SetKeyMap(&KeyMap{
		Cancel: keybind.NewKeybind(keybind.WithKeys("esc")),
	})

	canceled := false
	p.SetCancelFunc(func() {
		canceled = true
	})

	p.HandleEvent(tcell.NewEventKey(tcell.KeyEsc, "", tcell.ModNone))

	if !canceled {
		t.Error("expected cancel func to be called on Esc")
	}
}
