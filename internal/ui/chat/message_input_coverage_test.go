package chat

import (
	"os/exec"
	"testing"

	"github.com/ayn2op/discordo/internal/config"
	"github.com/gdamore/tcell/v3"
)

func TestMessageInput_Extra(t *testing.T) {
	m := newMockChatModel()
	mi := newMessageInput(m.cfg, m)

	// Mock editor functions to avoid hangs/suspension
	oldCreateEditorCmd := createEditorCmd
	oldRunEditor := runEditorCmd
	createEditorCmd = func(cfg *config.Config, path string) *exec.Cmd {
		return exec.Command("true")
	}
	runEditorCmd = func(cmd *exec.Cmd) error {
		return nil
	}
	defer func() {
		createEditorCmd = oldCreateEditorCmd
		runEditorCmd = oldRunEditor
	}()

	t.Run("HandleEvent_KeyEvent_CtrlU", func(t *testing.T) {
		mi.SetText("some text", true)
		mi.HandleEvent(tcell.NewEventKey(tcell.KeyCtrlU, "", tcell.ModNone))
		// Ctrl+U is Undo, so text might remain if there's nothing to undo or change to previous state.
		// We just ensure no panic.
	})

	t.Run("HandleEvent_KeyEvent_Escape", func(t *testing.T) {
		mi.SetText("some text", true)
		mi.HandleEvent(tcell.NewEventKey(tcell.KeyEsc, "", tcell.ModNone))
		if mi.GetText() != "" {
			t.Errorf("Expected empty text after Escape")
		}
	})

	t.Run("HandleEvent_KeyEvent_Tab", func(t *testing.T) {
		mi.HandleEvent(tcell.NewEventKey(tcell.KeyTab, "", tcell.ModNone))
	})

	t.Run("HandleEvent_KeyEvent_Enter", func(t *testing.T) {
		mi.SetText("hello", true)
		mi.HandleEvent(tcell.NewEventKey(tcell.KeyEnter, "", tcell.ModNone))
	})

	t.Run("HandleEvent_KeyEvent_CtrlS", func(t *testing.T) {
		mi.HandleEvent(tcell.NewEventKey(tcell.KeyCtrlS, "", tcell.ModNone))
	})

	t.Run("HandleEvent_KeyEvent_CtrlE", func(t *testing.T) {
		mi.HandleEvent(tcell.NewEventKey(tcell.KeyCtrlE, "", tcell.ModNone))
	})

	t.Run("HandleEvent_KeyEvent_Up", func(t *testing.T) {
		mi.HandleEvent(tcell.NewEventKey(tcell.KeyUp, "", tcell.ModNone))
	})

	t.Run("HandleEvent_KeyEvent_Down", func(t *testing.T) {
		mi.HandleEvent(tcell.NewEventKey(tcell.KeyDown, "", tcell.ModNone))
	})
}

func TestMessageInput_Autocomplete_Branches(t *testing.T) {
	m := newMockChatModel()
	mi := newMessageInput(m.cfg, m)

	t.Run("tabSuggestion_NoTrigger", func(t *testing.T) {
		mi.SetText("hello", true)
		mi.tabSuggestion()
	})
}
