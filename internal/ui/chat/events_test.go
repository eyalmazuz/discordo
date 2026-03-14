package chat

import (
	"testing"
	"time"

	"github.com/ayn2op/tview"
	"github.com/diamondburned/arikawa/v3/session"
	"github.com/gdamore/tcell/v3"
)

func TestNewLogoutEvent(t *testing.T) {
	before := time.Now()
	event := newLogoutEvent()
	after := time.Now()

	if event == nil {
		t.Fatal("newLogoutEvent returned nil")
	}

	assertEventTimeBetween(t, event.When(), before, after)
}

func TestModelLogout(t *testing.T) {
	m := newMockModel()

	command, ok := m.logout().(tview.EventCommand)
	if !ok {
		t.Fatalf("logout returned %T, want tview.EventCommand", m.logout())
	}

	before := time.Now()
	event := command()
	after := time.Now()

	logoutEvent, ok := event.(*LogoutEvent)
	if !ok {
		t.Fatalf("logout command produced %T, want *LogoutEvent", event)
	}

	assertEventTimeBetween(t, logoutEvent.When(), before, after)
}

func TestNewQuitEvent(t *testing.T) {
	before := time.Now()
	event := NewQuitEvent()
	after := time.Now()

	if event == nil {
		t.Fatal("NewQuitEvent returned nil")
	}

	assertEventTimeBetween(t, event.When(), before, after)
}

func TestModelCloseState(t *testing.T) {
	t.Run("success returns nil event", func(t *testing.T) {
		m := newMockModel()
		m.SetState(nil)

		command, ok := m.closeState().(tview.EventCommand)
		if !ok {
			t.Fatalf("closeState returned %T, want tview.EventCommand", m.closeState())
		}

		if event := command(); event != nil {
			t.Fatalf("closeState success produced %T, want nil", event)
		}
	})

	t.Run("error returns event error", func(t *testing.T) {
		m := newMockModel()

		command, ok := m.closeState().(tview.EventCommand)
		if !ok {
			t.Fatalf("closeState returned %T, want tview.EventCommand", m.closeState())
		}

		before := time.Now()
		event := command()
		after := time.Now()

		eventErr, ok := event.(*tcell.EventError)
		if !ok {
			t.Fatalf("closeState error produced %T, want *tcell.EventError", event)
		}

		if got := eventErr.Error(); got != session.ErrClosed.Error() {
			t.Fatalf("closeState error = %q, want %q", got, session.ErrClosed.Error())
		}

		assertEventTimeBetween(t, eventErr.When(), before, after)
	})
}

func assertEventTimeBetween(t *testing.T, when, before, after time.Time) {
	t.Helper()

	if when.IsZero() {
		t.Fatal("event time was not set")
	}
	if when.Before(before) || when.After(after) {
		t.Fatalf("event time %v outside expected range [%v, %v]", when, before, after)
	}
}
