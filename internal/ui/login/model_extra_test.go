package login

import (
	"errors"
	"testing"

	clipkg "github.com/ayn2op/discordo/internal/clipboard"
	"github.com/ayn2op/discordo/internal/config"
	"github.com/eyalmazuz/tview"
	"github.com/gdamore/tcell/v3"
)

func TestLoginModelCopyError(t *testing.T) {
	oldWriteClipboard := writeClipboard
	t.Cleanup(func() {
		writeClipboard = oldWriteClipboard
	})

	m := newTestLoginModel(t)
	m.HandleEvent(tcell.NewEventError(errors.New("boom")))

	writeClipboard = func(format clipkg.Format, data []byte) error {
		if format != clipkg.FmtText {
			t.Fatalf("unexpected clipboard format: %v", format)
		}
		if string(data) != "boom" {
			t.Fatalf("unexpected clipboard contents: %q", string(data))
		}
		return errors.New("copy failed")
	}

	cmd := m.HandleEvent(&tview.ModalDoneEvent{ButtonIndex: 0})
	event := runCommand(t, cmd)
	if _, ok := event.(*tcell.EventError); !ok {
		t.Fatalf("expected clipboard write failure to return EventError, got %T", event)
	}
}

func TestLoginModelOnErrorWithDefaultDialogStyle(t *testing.T) {
	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	cfg.Theme.Dialog.Style.Style = tcell.StyleDefault
	cfg.Theme.Dialog.BackgroundStyle.Style = tcell.StyleDefault

	m := NewModel(cfg)
	m.onError(errors.New("plain"))

	if !m.HasLayer(errorLayerName) {
		t.Fatal("expected onError to open the error layer")
	}
	if m.errorModalText != "plain" {
		t.Fatalf("expected stored modal text %q, got %q", "plain", m.errorModalText)
	}
}

func TestLoginModelOnErrorWithStyledDialog(t *testing.T) {
	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	cfg.Theme.Dialog.Style.Style = tcell.StyleDefault.Foreground(tcell.ColorBlue).Background(tcell.ColorRed)
	cfg.Theme.Dialog.BackgroundStyle.Style = tcell.StyleDefault.Background(tcell.ColorGreen)

	m := NewModel(cfg)
	m.onError(errors.New("styled"))

	if !m.HasLayer(errorLayerName) {
		t.Fatal("expected styled onError to open the error layer")
	}
	if m.errorModalText != "styled" {
		t.Fatalf("expected stored modal text %q, got %q", "styled", m.errorModalText)
	}
}
