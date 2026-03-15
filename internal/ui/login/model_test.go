package login

import (
	"errors"
	"testing"

	clipkg "github.com/ayn2op/discordo/internal/clipboard"
	"github.com/ayn2op/discordo/internal/config"
	"github.com/ayn2op/tview"
	"github.com/gdamore/tcell/v3"
)

func newTestLoginModel(t *testing.T) *Model {
	t.Helper()

	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	return NewModel(cfg)
}

func TestLoginModelNewModelAndHelp(t *testing.T) {
	m := newTestLoginModel(t)

	if !m.HasLayer(tabsLayerName) {
		t.Fatal("expected tabs layer to be present")
	}
	if len(m.ShortHelp()) == 0 {
		t.Fatal("expected short help to be populated")
	}
	if len(m.FullHelp()) == 0 {
		t.Fatal("expected full help to be populated")
	}
}

func TestLoginModelHandleEventErrorAndModal(t *testing.T) {
	oldWriteClipboard := writeClipboard
	t.Cleanup(func() {
		writeClipboard = oldWriteClipboard
	})

	m := newTestLoginModel(t)

	if cmd := m.HandleEvent(&tview.ModalDoneEvent{ButtonIndex: 1}); cmd != nil {
		t.Fatalf("expected modal done without an error layer to return nil, got %T", cmd)
	}

	if cmd := m.HandleEvent(tcell.NewEventError(errors.New("boom"))); cmd == nil {
		t.Fatal("expected error event to return a focus command")
	}
	if !m.HasLayer(errorLayerName) {
		t.Fatal("expected error layer to be opened")
	}
	if m.errorModalText != "boom" {
		t.Fatalf("expected stored modal text %q, got %q", "boom", m.errorModalText)
	}

	// Repeated errors while the modal is open should not duplicate the layer.
	if cmd := m.HandleEvent(tcell.NewEventError(errors.New("again"))); cmd != nil {
		t.Fatalf("expected repeated error event to return nil, got %T", cmd)
	}
	if m.errorModalText != "boom" {
		t.Fatalf("expected repeated error to keep original modal text, got %q", m.errorModalText)
	}

	var copied string
	writeClipboard = func(format clipkg.Format, data []byte) error {
		if format != clipkg.FmtText {
			t.Fatalf("expected text clipboard format, got %v", format)
		}
		copied = string(data)
		return nil
	}

	cmd := m.HandleEvent(&tview.ModalDoneEvent{ButtonIndex: 0})
	if event := runCommand(t, cmd); event != nil {
		t.Fatalf("expected successful copy command to return nil event, got %T", event)
	}
	if copied != "boom" {
		t.Fatalf("expected copied modal text %q, got %q", "boom", copied)
	}

	if cmd := m.HandleEvent(&tview.ModalDoneEvent{ButtonIndex: 1}); cmd != nil {
		t.Fatalf("expected close button to return nil, got %T", cmd)
	}
	if m.HasLayer(errorLayerName) {
		t.Fatal("expected close button to remove the error layer")
	}
	if m.errorModalText != "" {
		t.Fatalf("expected close button to clear modal text, got %q", m.errorModalText)
	}
}

func TestSetClipboardError(t *testing.T) {
	oldWriteClipboard := writeClipboard
	t.Cleanup(func() {
		writeClipboard = oldWriteClipboard
	})

	writeClipboard = func(format clipkg.Format, data []byte) error {
		if format != clipkg.FmtText {
			t.Fatalf("expected text clipboard format, got %v", format)
		}
		if string(data) != "boom" {
			t.Fatalf("expected clipboard payload %q, got %q", "boom", string(data))
		}
		return errors.New("copy failed")
	}

	event := runCommand(t, setClipboard("boom"))
	if _, ok := event.(*tcell.EventError); !ok {
		t.Fatalf("expected clipboard failure to surface as EventError, got %T", event)
	}
}

func TestLoginModelHandleEventFallsBackToLayers(t *testing.T) {
	m := newTestLoginModel(t)
	if cmd := m.HandleEvent(tcell.NewEventKey(tcell.KeyTab, "", tcell.ModNone)); cmd != nil {
		t.Fatalf("expected regular key events to be delegated to layers, got %T", cmd)
	}
}
