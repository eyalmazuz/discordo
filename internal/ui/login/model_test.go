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
	return NewModel(tview.NewApplication(), cfg)
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

	if _, ok := m.HandleEvent(tcell.NewEventError(errors.New("boom"))).(tview.RedrawCommand); !ok {
		t.Fatal("expected error event to request a redraw")
	}
	if !m.HasLayer(errorLayerName) {
		t.Fatal("expected error layer to be opened")
	}
	if m.errorModalText != "boom" {
		t.Fatalf("expected stored modal text %q, got %q", "boom", m.errorModalText)
	}

	// Repeated errors while the modal is open should not duplicate the layer.
	if _, ok := m.HandleEvent(tcell.NewEventError(errors.New("again"))).(tview.RedrawCommand); !ok {
		t.Fatal("expected repeated error event to request a redraw")
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
	eventCmd, ok := cmd.(tview.EventCommand)
	if !ok {
		t.Fatalf("expected copy button to return EventCommand, got %T", cmd)
	}
	if event := eventCmd(); event != nil {
		t.Fatalf("expected successful copy command to return nil event, got %T", event)
	}
	if copied != "boom" {
		t.Fatalf("expected copied modal text %q, got %q", "boom", copied)
	}

	if _, ok := m.HandleEvent(&tview.ModalDoneEvent{ButtonIndex: 1}).(tview.RedrawCommand); !ok {
		t.Fatal("expected close button to request a redraw")
	}
	if m.HasLayer(errorLayerName) {
		t.Fatal("expected close button to remove the error layer")
	}
	if m.errorModalText != "" {
		t.Fatalf("expected close button to clear modal text, got %q", m.errorModalText)
	}
}

