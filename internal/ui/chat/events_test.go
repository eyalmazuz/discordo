package chat

import (
	"testing"

	"github.com/diamondburned/arikawa/v3/session"
	"github.com/gdamore/tcell/v3"
)

func TestNewLogoutEvent(t *testing.T) {
	event := newLogoutEvent()
	if event == nil {
		t.Fatal("newLogoutEvent returned nil")
	}
}

func TestModelLogout(t *testing.T) {
	m := newMockModel()
	event := executeCommand(requireCommand(t, m.logout()))

	if _, ok := event.(*LogoutEvent); !ok {
		t.Fatalf("logout command produced %T, want *LogoutEvent", event)
	}
}

func TestNewQuitEvent(t *testing.T) {
	event := NewQuitEvent()
	if event == nil {
		t.Fatal("NewQuitEvent returned nil")
	}
}

func TestModelCloseState(t *testing.T) {
	t.Run("success returns nil event", func(t *testing.T) {
		m := newMockModel()
		m.SetState(nil)
		if event := executeCommand(requireCommand(t, m.closeState())); event != nil {
			t.Fatalf("closeState success produced %T, want nil", event)
		}
	})

	t.Run("error returns event error", func(t *testing.T) {
		m := newMockModel()
		event := executeCommand(requireCommand(t, m.closeState()))

		eventErr, ok := event.(*tcell.EventError)
		if !ok {
			t.Fatalf("closeState error produced %T, want *tcell.EventError", event)
		}

		if got := eventErr.Error(); got != session.ErrClosed.Error() {
			t.Fatalf("closeState error = %q, want %q", got, session.ErrClosed.Error())
		}
	})
}
