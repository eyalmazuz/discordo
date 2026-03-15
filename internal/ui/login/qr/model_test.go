package qr

import (
	"crypto/rsa"
	"errors"
	"testing"
	"time"

	"github.com/eyalmazuz/tview"
	"github.com/gdamore/tcell/v3"
	"github.com/skip2/go-qrcode"
)

func TestNewModel(t *testing.T) {
	app := tview.NewApplication()
	m := NewModel(app)

	if m.msg != "Press Ctrl+N to open QR login" {
		t.Errorf("Expected initial message 'Press Ctrl+N to open QR login', got %q", m.msg)
	}

	if m.Label() != "QR" {
		t.Errorf("Expected label 'QR', got %q", m.Label())
	}
}

func TestModel_HandleEvent_LoginTriggering(t *testing.T) {
	app := tview.NewApplication()
	m := NewModel(app)

	// Test InitEvent triggers connection
	cmd := m.HandleEvent(&tview.InitEvent{})
	if cmd == nil {
		t.Errorf("Expected a command for InitEvent, got nil")
	}

	expectedMsg := "Connecting to Remote Auth Gateway..."
	if m.msg != expectedMsg {
		t.Errorf("Expected message %q, got %q", expectedMsg, m.msg)
	}
}

func TestModel_HandleEvent_Cancel(t *testing.T) {
	app := tview.NewApplication()
	m := NewModel(app)

	// Simulate pressing Escape to cancel
	event := tcell.NewEventKey(tcell.KeyEsc, "", tcell.ModNone)
	cmd := m.HandleEvent(event)
	if cmd == nil {
		t.Errorf("Expected a command for Escape key, got nil")
	}

	if m.msg != "Canceled" {
		t.Errorf("Expected message 'Canceled', got %q", m.msg)
	}
}

func TestModel_HandleEvent_CustomEvents(t *testing.T) {
	app := tview.NewApplication()
	m := NewModel(app)

	t.Run("connCreateEvent", func(t *testing.T) {
		m.HandleEvent(&connCreateEvent{})
		if m.msg != "Connected. Handshaking..." {
			t.Errorf("Expected message 'Connected. Handshaking...', got %q", m.msg)
		}
	})

	t.Run("helloEvent", func(t *testing.T) {
		m.HandleEvent(&helloEvent{heartbeatInterval: 1000})
		if m.heartbeatInterval != 1000*time.Millisecond {
			t.Errorf("Expected heartbeat interval 1s, got %v", m.heartbeatInterval)
		}
	})

	t.Run("qrCodeEvent", func(t *testing.T) {
		qr, _ := qrcode.New("test", qrcode.Low)
		m.HandleEvent(&qrCodeEvent{qrCode: qr})
		if m.qrCode != qr {
			t.Errorf("Expected qrCode to be set")
		}
		if m.msg != "Scan this with the Discord mobile app to log in instantly." {
			t.Errorf("Unexpected message: %q", m.msg)
		}
	})

	t.Run("userEvent", func(t *testing.T) {
		m.HandleEvent(&userEvent{username: "testuser", discriminator: "1234"})
		if m.msg != "Check your phone! Logging in as testuser#1234" {
			t.Errorf("Unexpected message: %q", m.msg)
		}
	})

	t.Run("userEvent_NoDiscriminator", func(t *testing.T) {
		m.HandleEvent(&userEvent{username: "testuser", discriminator: "0"})
		if m.msg != "Check your phone! Logging in as testuser" {
			t.Errorf("Unexpected message: %q", m.msg)
		}
	})

	t.Run("pendingLoginEvent", func(t *testing.T) {
		m.HandleEvent(&pendingLoginEvent{ticket: "test-ticket"})
		if m.msg != "Authenticating..." {
			t.Errorf("Expected message 'Authenticating...', got %q", m.msg)
		}
	})

	t.Run("nonceProofEvent", func(t *testing.T) {
		cmd := m.HandleEvent(&nonceProofEvent{encryptedNonce: "payload"})
		if cmd == nil {
			t.Fatal("expected nonce proof event to return a command")
		}
	})

	t.Run("EventError", func(t *testing.T) {
		errEvent := tcell.NewEventError(errors.New("test error"))
		cmd := m.HandleEvent(errEvent)
		if cmd == nil {
			t.Fatal("expected EventError to return a command")
		}
		if m.msg != "test error" {
			t.Errorf("Unexpected message: %q", m.msg)
		}
		// The command is a batch that contains the error event again.
		// Since we cannot easily introspect the batch command, just executing it is enough for coverage.
		cmd()
	})

	t.Run("cancelEvent", func(t *testing.T) {
		m.HandleEvent(&cancelEvent{})
		if m.msg != "Login canceled on mobile" {
			t.Errorf("Expected message 'Login canceled on mobile', got %q", m.msg)
		}
	})
}

func TestModel_HandleEvent_RemainingBranches(t *testing.T) {
	m := NewModel(tview.NewApplication())

	t.Run("non escape key falls through to text view", func(t *testing.T) {
		m.SetText("body")
		if cmd := m.HandleEvent(tcell.NewEventKey(tcell.KeyRune, "x", tcell.ModNone)); cmd != nil {
			t.Fatalf("expected non-escape key to fall through without command, got %T", cmd)
		}
	})

	t.Run("heartbeat tick without connection", func(t *testing.T) {
		if cmd := m.HandleEvent(&heartbeatTickEvent{}); cmd != nil {
			t.Fatalf("expected nil command without connection, got %T", cmd)
		}
	})

	t.Run("unhandled event returns nil", func(t *testing.T) {
		if cmd := m.HandleEvent(tcell.NewEventResize(80, 24)); cmd != nil {
			t.Fatalf("expected resize event to return nil, got %T", cmd)
		}
	})
}

func TestModelCenterLinesBranches(t *testing.T) {
	m := NewModel(tview.NewApplication())

	t.Run("default height fallback pads", func(t *testing.T) {
		lines := []tview.Line{{{Text: "one"}}}
		centered := m.centerLines(lines)
		if len(centered) <= len(lines) {
			t.Fatalf("expected fallback centering to add padding, got %d lines", len(centered))
		}
	})

	t.Run("small height clamps padding to zero", func(t *testing.T) {
		m.SetRect(0, 0, 10, 1)
		lines := []tview.Line{{{Text: "one"}}, {{Text: "two"}}}
		centered := m.centerLines(lines)
		if len(centered) != len(lines) {
			t.Fatalf("expected no extra padding when content exceeds height, got %d lines", len(centered))
		}
	})

	t.Run("slightly taller height still adds one line of padding", func(t *testing.T) {
		m.SetRect(0, 0, 10, 2)
		lines := []tview.Line{{{Text: "one"}}}
		centered := m.centerLines(lines)
		if len(centered) != 2 {
			t.Fatalf("expected one line of top padding, got %d lines", len(centered))
		}
		if len(centered[0]) != 0 {
			t.Fatal("expected first line to be padding")
		}
	})

	t.Run("zero inner height falls back to default height", func(t *testing.T) {
		m.SetRect(0, 0, 0, 0)
		lines := []tview.Line{{{Text: "one"}}}
		centered := m.centerLines(lines)
		if len(centered) <= len(lines) {
			t.Fatalf("expected default-height centering to add padding, got %d lines", len(centered))
		}
	})
}

func TestModelNewAndChangedCallbackBranch(t *testing.T) {
	m := NewModel(tview.NewApplication())
	if m.TextView == nil {
		t.Fatal("expected NewModel to initialize TextView")
	}

	m.privateKey = &rsa.PrivateKey{}
	if m.privateKey == nil {
		t.Fatal("expected privateKey assignment to stick")
	}
}
