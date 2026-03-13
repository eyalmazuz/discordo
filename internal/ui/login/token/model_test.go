package token

import (
	"testing"

	"github.com/ayn2op/tview"
	"github.com/gdamore/tcell/v3"
)

func TestNewModel(t *testing.T) {
	m := NewModel()
	if m.Form == nil {
		t.Errorf("Expected Form to be initialized")
	}

	tokenField := m.GetFormItem(0).(*tview.InputField)
	if tokenField.GetLabel() != "Token" {
		t.Errorf("Expected label 'Token', got %q", tokenField.GetLabel())
	}
}

func TestModel_HandleEvent_Submission(t *testing.T) {
	m := NewModel()
	token := "test-token"
	m.GetFormItem(0).(*tview.InputField).SetText(token)

	// Simulate submitting the form
	cmd := m.HandleEvent(&tview.FormSubmitEvent{})
	if cmd == nil {
		t.Errorf("Expected a command for FormSubmitEvent, got nil")
	}
}

func TestModel_Label(t *testing.T) {
	m := NewModel()
	if m.Label() != "Token" {
		t.Errorf("Expected label 'Token', got %q", m.Label())
	}
}

func TestModel_HandleEvent_EmptySubmission(t *testing.T) {
	m := NewModel()
	m.GetFormItem(0).(*tview.InputField).SetText("")

	cmd := m.HandleEvent(&tview.FormSubmitEvent{})
	if cmd != nil {
		t.Errorf("Expected nil command for empty token submission, got %v", cmd)
	}
}

func TestModel_HandleEvent_Fallback(t *testing.T) {
	m := NewModel()
	// Test that it falls back to tview.Form.HandleEvent
	// For example, hitting Enter on a button should trigger something or return a command
	event := tcell.NewEventKey(tcell.KeyEnter, "", tcell.ModNone)
	// We don't necessarily care about the return value, just that it doesn't crash
	// and follows the expected path.
	m.HandleEvent(event)
}
