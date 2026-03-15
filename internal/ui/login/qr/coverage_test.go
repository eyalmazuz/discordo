package qr

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	stdhttp "net/http"
	"testing"
	"time"

	"github.com/eyalmazuz/tview"
	"github.com/diamondburned/arikawa/v3/api"
	"github.com/gdamore/tcell/v3"
	"github.com/gorilla/websocket"
	"github.com/skip2/go-qrcode"
)

func TestEventConstructors(t *testing.T) {
	t.Run("newTokenEvent", func(t *testing.T) {
		e := newTokenEvent("token")
		if e.Token != "token" {
			t.Errorf("got %q", e.Token)
		}
	})
	t.Run("newConnCreateEvent", func(t *testing.T) {
		newConnCreateEvent(nil)
	})
	t.Run("newConnCloseEvent", func(t *testing.T) {
		newConnCloseEvent()
	})
	t.Run("newHelloEvent", func(t *testing.T) {
		newHelloEvent(1, 2)
	})
	t.Run("newNonceProofEvent", func(t *testing.T) {
		newNonceProofEvent("nonce")
	})
	t.Run("newPendingRemoteInitEvent", func(t *testing.T) {
		newPendingRemoteInitEvent("fp")
	})
	t.Run("newPendingTicketEvent", func(t *testing.T) {
		newPendingTicketEvent("payload")
	})
	t.Run("newPendingLoginEvent", func(t *testing.T) {
		newPendingLoginEvent("ticket")
	})
	t.Run("newCancelEvent", func(t *testing.T) {
		newCancelEvent()
	})
	t.Run("newHeartbeatTickEvent", func(t *testing.T) {
		newHeartbeatTickEvent()
	})
	t.Run("newPrivateKeyEvent", func(t *testing.T) {
		newPrivateKeyEvent(nil)
	})
	t.Run("newQRCodeEvent", func(t *testing.T) {
		newQRCodeEvent(nil)
	})
	t.Run("newUserEvent", func(t *testing.T) {
		newUserEvent("disc", "user")
	})
}

func TestModel_HandleEvent_Lifecycle(t *testing.T) {
	app := tview.NewApplication()
	m := NewModel(app)

	t.Run("InitEvent", func(t *testing.T) {
		m.HandleEvent(tview.NewInitEvent())
		if m.msg != "Connecting to Remote Auth Gateway..." {
			t.Errorf("Unexpected message: %q", m.msg)
		}
	})

	t.Run("KeyEvent_Escape", func(t *testing.T) {
		m.HandleEvent(tcell.NewEventKey(tcell.KeyEsc, "", tcell.ModNone))
		if m.msg != "Canceled" {
			t.Errorf("Expected message 'Canceled', got %q", m.msg)
		}
	})

	t.Run("connCreateEvent", func(t *testing.T) {
		conn := &websocket.Conn{}
		m.HandleEvent(&connCreateEvent{conn: conn})
		if m.msg != "Connected. Handshaking..." {
			t.Errorf("Unexpected message: %q", m.msg)
		}
		if m.conn != conn {
			t.Error("expected connection to be set")
		}
	})

	t.Run("connCloseEvent", func(t *testing.T) {
		m.conn = &websocket.Conn{}
		m.HandleEvent(&connCloseEvent{})
		if m.conn != nil {
			t.Error("expected connection to be cleared")
		}
	})

	t.Run("helloEvent", func(t *testing.T) {
		m.HandleEvent(&helloEvent{heartbeatInterval: 100})
	})

	t.Run("qrCodeEvent", func(t *testing.T) {
		m.HandleEvent(&qrCodeEvent{qrCode: nil})
		if m.msg != "Scan this with the Discord mobile app to log in instantly." {
			t.Errorf("Unexpected message: %q", m.msg)
		}
	})

	t.Run("userEvent_Branches", func(t *testing.T) {
		m.HandleEvent(&userEvent{username: "user", discriminator: "1234"})
		m.HandleEvent(&userEvent{username: "user2", discriminator: "0"})
	})

	t.Run("heartbeatTickEvent_WithConn", func(t *testing.T) {
		m.conn = &websocket.Conn{} // Dummy
		m.HandleEvent(&heartbeatTickEvent{})
	})

	t.Run("EventError", func(t *testing.T) {
		m.HandleEvent(tcell.NewEventError(errors.New("fail")))
		if m.msg != "fail" {
			t.Errorf("Expected message 'fail', got %q", m.msg)
		}
	})

	t.Run("pendingRemoteInitEvent", func(t *testing.T) {
		m.HandleEvent(&pendingRemoteInitEvent{fingerprint: "test-fp"})
		if m.fingerprint != "test-fp" {
			t.Errorf("Expected fingerprint to be set")
		}
	})

	t.Run("pendingTicketEvent", func(t *testing.T) {
		m.HandleEvent(&pendingTicketEvent{encryptedUserPayload: "test-payload"})
	})

	t.Run("pendingLoginEvent", func(t *testing.T) {
		m.HandleEvent(&pendingLoginEvent{ticket: "test-ticket"})
		if m.msg != "Authenticating..." {
			t.Errorf("Unexpected message: %q", m.msg)
		}
	})

	t.Run("cancelEvent", func(t *testing.T) {
		m.HandleEvent(&cancelEvent{})
		if m.msg != "Login canceled on mobile" {
			t.Errorf("Unexpected message: %q", m.msg)
		}
	})

	t.Run("privateKeyEvent", func(t *testing.T) {
		key, _ := rsa.GenerateKey(rand.Reader, 2048)
		m.HandleEvent(&privateKeyEvent{privateKey: key})
		if m.privateKey != key {
			t.Error("expected private key to be set")
		}
	})
}

func TestModel_Commands_Mocks(t *testing.T) {
	app := tview.NewApplication()
	m := NewModel(app)

	t.Run("connect_Success", func(t *testing.T) {
		oldWsDial := wsDial
		wsDial = func(url string, headers stdhttp.Header) (*websocket.Conn, *stdhttp.Response, error) {
			return &websocket.Conn{}, nil, nil
		}
		defer func() { wsDial = oldWsDial }()

		cmd := m.connect()
		event := runCommand(t, cmd)
		if _, ok := event.(*connCreateEvent); !ok {
			t.Errorf("Expected connCreateEvent, got %T", event)
		}
	})

	t.Run("close_WithConn", func(t *testing.T) {
		m.conn = &websocket.Conn{}
		oldWsClose := wsClose
		wsClose = func(conn *websocket.Conn) error { return nil }
		defer func() { wsClose = oldWsClose }()

		cmd := m.close()
		runCommand(t, cmd)
	})

	t.Run("listen_Branches", func(t *testing.T) {
		m.conn = &websocket.Conn{}
		oldWsRead := wsReadMessage
		defer func() { wsReadMessage = oldWsRead }()

		tests := []struct {
			name     string
			op       string
			data     map[string]any
			wantType any
		}{
			{"Hello", "hello", map[string]any{"heartbeat_interval": 100, "timeout_ms": 1000}, &helloEvent{}},
			{"NonceProof", "nonce_proof", map[string]any{"encrypted_nonce": "nonce"}, &nonceProofEvent{}},
			{"PendingRemoteInit", "pending_remote_init", map[string]any{"fingerprint": "fp"}, &pendingRemoteInitEvent{}},
			{"PendingTicket", "pending_ticket", map[string]any{"encrypted_user_payload": "payload"}, &pendingTicketEvent{}},
			{"PendingLogin", "pending_login", map[string]any{"ticket": "ticket"}, &pendingLoginEvent{}},
			{"Cancel", "cancel", map[string]any{}, &cancelEvent{}},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				wsReadMessage = func(conn *websocket.Conn) (int, []byte, error) {
					payload := tt.data
					payload["op"] = tt.op
					data, _ := json.Marshal(payload)
					return websocket.TextMessage, data, nil
				}
				cmd := m.listen()
				event := runCommand(t, cmd)
				if _, ok := reflectTypeMatch(event, tt.wantType); !ok {
					t.Errorf("Expected %T, got %T", tt.wantType, event)
				}
			})
		}
	})

	t.Run("heartbeat_Wait", func(t *testing.T) {
		m.heartbeatInterval = 10 * time.Millisecond
		cmd := m.heartbeat()
		event := runCommand(t, cmd)
		if _, ok := event.(*heartbeatTickEvent); !ok {
			t.Errorf("Expected heartbeatTickEvent, got %T", event)
		}
	})

	t.Run("sendHeartbeat_WithConn", func(t *testing.T) {
		m.conn = &websocket.Conn{}
		oldWsWrite := wsWriteJSON
		wsWriteJSON = func(conn *websocket.Conn, v any) error { return nil }
		defer func() { wsWriteJSON = oldWsWrite }()

		cmd := m.sendHeartbeat()
		event := runCommand(t, cmd)
		if event != nil {
			t.Errorf("Expected nil event, got %T", event)
		}
	})

	t.Run("generatePrivateKey_Success", func(t *testing.T) {
		cmd := m.generatePrivateKey()
		event := runCommand(t, cmd)
		if _, ok := event.(*privateKeyEvent); !ok {
			t.Errorf("Expected privateKeyEvent, got %T", event)
		}
	})

	t.Run("sendInit_WithKey", func(t *testing.T) {
		m.conn = &websocket.Conn{}
		m.privateKey, _ = rsa.GenerateKey(rand.Reader, 2048)
		oldWsWrite := wsWriteJSON
		wsWriteJSON = func(conn *websocket.Conn, v any) error { return nil }
		defer func() { wsWriteJSON = oldWsWrite }()

		cmd := m.sendInit()
		event := runCommand(t, cmd)
		if event != nil {
			t.Errorf("Expected nil event, got %T", event)
		}
	})

	t.Run("sendNonceProof_Success", func(t *testing.T) {
		m.conn = &websocket.Conn{}
		key, _ := rsa.GenerateKey(rand.Reader, 2048)
		m.privateKey = key

		nonce := []byte("nonce")
		encryptedNonce, _ := rsa.EncryptOAEP(sha256.New(), rand.Reader, &key.PublicKey, nonce, nil)
		encodedNonce := base64.StdEncoding.EncodeToString(encryptedNonce)

		oldWsWrite := wsWriteJSON
		wsWriteJSON = func(conn *websocket.Conn, v any) error { return nil }
		defer func() { wsWriteJSON = oldWsWrite }()

		cmd := m.sendNonceProof(encodedNonce)
		event := runCommand(t, cmd)
		if event != nil {
			t.Errorf("Expected nil event, got %T", event)
		}
	})

	t.Run("decryptUserPayload_Success", func(t *testing.T) {
		key, _ := rsa.GenerateKey(rand.Reader, 2048)
		m.privateKey = key

		payload := "id:1234:avatar:user"
		encryptedPayload, _ := rsa.EncryptOAEP(sha256.New(), rand.Reader, &key.PublicKey, []byte(payload), nil)
		encodedPayload := base64.StdEncoding.EncodeToString(encryptedPayload)

		cmd := m.decryptUserPayload(encodedPayload)
		event := runCommand(t, cmd)
		if e, ok := event.(*userEvent); !ok || e.username != "user" || e.discriminator != "1234" {
			t.Errorf("Expected userEvent user:user disc:1234, got %v", event)
		}
	})

	t.Run("exchangeTicket_Success", func(t *testing.T) {
		key, _ := rsa.GenerateKey(rand.Reader, 2048)
		m.privateKey = key

		token := "my-secret-token"
		encryptedToken, _ := rsa.EncryptOAEP(sha256.New(), rand.Reader, &key.PublicKey, []byte(token), nil)
		encodedToken := base64.StdEncoding.EncodeToString(encryptedToken)

		oldExchange := exchangeTicketFn
		exchangeTicketFn = func(client *api.Client, ticket string) (string, error) {
			return encodedToken, nil
		}
		defer func() { exchangeTicketFn = oldExchange }()

		cmd := m.exchangeTicket("ticket")
		event := runCommand(t, cmd)
		if e, ok := event.(*TokenEvent); !ok || e.Token != token {
			t.Errorf("Expected TokenEvent with token, got %v", event)
		}
	})

	t.Run("generateQRCode_Success", func(t *testing.T) {
		cmd := m.generateQRCode("fp")
		event := runCommand(t, cmd)
		if _, ok := event.(*qrCodeEvent); !ok {
			t.Errorf("Expected qrCodeEvent, got %T", event)
		}
	})
}

func reflectTypeMatch(got, want any) (any, bool) {
	switch want.(type) {
	case *helloEvent:
		_, ok := got.(*helloEvent)
		return got, ok
	case *nonceProofEvent:
		_, ok := got.(*nonceProofEvent)
		return got, ok
	case *pendingRemoteInitEvent:
		_, ok := got.(*pendingRemoteInitEvent)
		return got, ok
	case *pendingTicketEvent:
		_, ok := got.(*pendingTicketEvent)
		return got, ok
	case *pendingLoginEvent:
		_, ok := got.(*pendingLoginEvent)
		return got, ok
	case *cancelEvent:
		_, ok := got.(*cancelEvent)
		return got, ok
	default:
		return nil, false
	}
}

type mockScreen struct {
	tcell.Screen
}

func (m *mockScreen) SetContent(int, int, rune, []rune, tcell.Style)          {}
func (m *mockScreen) Size() (int, int)                                        { return 80, 24 }
func (m *mockScreen) Put(x, y int, s string, style tcell.Style) (string, int) { return s, len(s) }
func (m *mockScreen) PutStrStyled(x, y int, s string, style tcell.Style)      {}

func TestModel_Draw_Extra(t *testing.T) {
	app := tview.NewApplication()
	m := NewModel(app)
	screen := &mockScreen{}

	t.Run("WithoutQRCode", func(t *testing.T) {
		m.Draw(screen)
	})

	t.Run("WithQRCode", func(t *testing.T) {
		q, _ := qrcode.New("test", qrcode.Low)
		m.qrCode = q
		m.Draw(screen)
	})
}
