package login

import (
	"errors"
	"reflect"
	"testing"

	"github.com/ayn2op/discordo/internal/config"
	"github.com/eyalmazuz/tview"
	"github.com/gdamore/tcell/v3"
)

func getActiveTab(m *Model) int {
	v := reflect.ValueOf(m.tabs).Elem()
	f := v.FieldByName("active")
	return int(f.Int())
}

func TestRegression_LoginTabs(t *testing.T) {
	cfg, _ := config.Load("")
	m := NewModel(cfg)

	// Initially, it should be on the first tab (Token)
	if getActiveTab(m) != 0 {
		t.Errorf("expected initial tab to be 0 (Token), got %d", getActiveTab(m))
	}

	// Simulate 'Ctrl+L' to move to next tab (QR)
	m.Update(tcell.NewEventKey(tcell.KeyCtrlL, "", tcell.ModNone))
	if getActiveTab(m) != 1 {
		t.Errorf("expected tab to be 1 (QR) after 'Ctrl+L', got %d", getActiveTab(m))
	}

	// Switch back to Token tab
	m.Update(tcell.NewEventKey(tcell.KeyCtrlH, "", tcell.ModNone))
	if getActiveTab(m) != 0 {
		t.Errorf("expected tab to be 0 (Token) after 'Ctrl+H', got %d", getActiveTab(m))
	}
}

func TestRegression_LoginErrorFlow(t *testing.T) {
	cfg, _ := config.Load("")
	m := NewModel(cfg)

	// Simulate an error event
	errEv := tcell.NewEventError(errors.New("test error"))
	m.Update(errEv)

	if !m.HasLayer(errorLayerName) {
		t.Fatal("expected error layer to be visible")
	}

	// Close the error dialog
	m.Update(&tview.ModalDoneMsg{ButtonIndex: 1}) // Close button
	if m.HasLayer(errorLayerName) {
		t.Error("expected error layer to be removed after closing")
	}
}
