package chat

import (
	"errors"
	"testing"

	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/diamondburned/arikawa/v3/session"
	"github.com/diamondburned/arikawa/v3/state"
	"github.com/diamondburned/arikawa/v3/state/store/defaultstore"
	"github.com/diamondburned/ningen/v3"
)

func TestModelOpenState(t *testing.T) {
	oldNewOpenState := newOpenState
	oldOpenNingenState := openNingenState
	oldDefaultIdentity := gateway.DefaultIdentity
	oldDefaultPresence := gateway.DefaultPresence
	t.Cleanup(func() {
		newOpenState = oldNewOpenState
		openNingenState = oldOpenNingenState
		gateway.DefaultIdentity = oldDefaultIdentity
		gateway.DefaultPresence = oldDefaultPresence
	})

	var (
		gotToken string
		gotID    gateway.Identifier
	)
	built := ningen.FromState(state.NewFromSession(session.New(""), defaultstore.New()))
	newOpenState = func(token string, id gateway.Identifier) *ningen.State {
		gotToken = token
		gotID = id
		return built
	}

	openCalls := 0
	openNingenState = func(st *ningen.State) error {
		openCalls++
		if st != built {
			t.Fatalf("expected OpenState to pass the built state to open, got %p want %p", st, built)
		}
		return nil
	}

	m := newMockChatModel()
	m.cfg.Status = "online"
	m.cfg.TypingIndicator.Receive = true

	if err := m.OpenState("token-123"); err != nil {
		t.Fatalf("OpenState returned error: %v", err)
	}

	if gotToken != "token-123" {
		t.Fatalf("expected builder token %q, got %q", "token-123", gotToken)
	}
	if gotID.Token != "token-123" {
		t.Fatalf("expected gateway identifier token %q, got %q", "token-123", gotID.Token)
	}
	if gotID.Compress {
		t.Fatal("expected OpenState to disable compression on the gateway identifier")
	}
	if m.state != built {
		t.Fatal("expected model state to be replaced with the built state")
	}
	if gateway.DefaultPresence == nil || gateway.DefaultPresence.Status != m.cfg.Status {
		t.Fatalf("expected default presence status %q, got %+v", m.cfg.Status, gateway.DefaultPresence)
	}
	if built.StateLog == nil {
		t.Fatal("expected OpenState to install a state logger")
	}
	if len(built.OnRequest) == 0 {
		t.Fatal("expected OpenState to register request middleware")
	}
	if openCalls != 1 {
		t.Fatalf("expected OpenState to invoke state open once, got %d", openCalls)
	}
}

func TestModelOpenStateReturnsOpenError(t *testing.T) {
	oldNewOpenState := newOpenState
	oldOpenNingenState := openNingenState
	t.Cleanup(func() {
		newOpenState = oldNewOpenState
		openNingenState = oldOpenNingenState
	})

	built := ningen.FromState(state.NewFromSession(session.New(""), defaultstore.New()))
	newOpenState = func(string, gateway.Identifier) *ningen.State {
		return built
	}

	sentinel := errors.New("open failed")
	openNingenState = func(st *ningen.State) error {
		if st != built {
			t.Fatalf("expected open to receive the built state, got %p want %p", st, built)
		}
		return sentinel
	}

	m := newMockChatModel()
	if err := m.OpenState("token-123"); !errors.Is(err, sentinel) {
		t.Fatalf("expected OpenState to return %v, got %v", sentinel, err)
	}
}
