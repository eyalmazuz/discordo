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
	t.Run("TokenMsg", func(t *testing.T) {
		e := &TokenMsg{Token: "token"}
		if e.Token != "token" {
			t.Errorf("got %q", e.Token)
		}
	})
	t.Run("connCreateMsg", func(t *testing.T) {
		_ = &connCreateMsg{}
	})
	t.Run("connCloseMsg", func(t *testing.T) {
		_ = &connCloseMsg{}
	})
	t.Run("helloMsg", func(t *testing.T) {
		_ = &helloMsg{heartbeatInterval: 1, timeoutMS: 2}
	})
	t.Run("nonceProofMsg", func(t *testing.T) {
		_ = &nonceProofMsg{encryptedNonce: "nonce"}
	})
	t.Run("pendingRemoteInitMsg", func(t *testing.T) {
		_ = &pendingRemoteInitMsg{fingerprint: "fp"}
	})
	t.Run("pendingTicketMsg", func(t *testing.T) {
		_ = &pendingTicketMsg{encryptedUserPayload: "payload"}
	})
	t.Run("pendingLoginMsg", func(t *testing.T) {
		_ = &pendingLoginMsg{ticket: "ticket"}
	})
	t.Run("cancelMsg", func(t *testing.T) {
		_ = &cancelMsg{}
	})
	t.Run("heartbeatTickMsg", func(t *testing.T) {
		_ = &heartbeatTickMsg{}
	})
	t.Run("privateKeyMsg", func(t *testing.T) {
		_ = &privateKeyMsg{}
	})
	t.Run("qrCodeMsg", func(t *testing.T) {
		_ = &qrCodeMsg{}
	})
	t.Run("userMsg", func(t *testing.T) {
		_ = &userMsg{discriminator: "disc", username: "user"}
	})
}

func TestModel_HandleEvent_Lifecycle(t *testing.T) {
	app := tview.NewApplication()
	m := NewModel(app)

	t.Run("InitEvent", func(t *testing.T) {
		m.Update(tview.NewInitMsg())
		if m.msg != "Connecting to Remote Auth Gateway..." {
			t.Errorf("Unexpected message: %q", m.msg)
		}
	})

	t.Run("KeyEvent_Escape", func(t *testing.T) {
		m.Update(tcell.NewEventKey(tcell.KeyEsc, "", tcell.ModNone))
		if m.msg != "Canceled" {
			t.Errorf("Expected message 'Canceled', got %q", m.msg)
		}
	})

	t.Run("connCreateMsg", func(t *testing.T) {
		conn := &websocket.Conn{}
		m.Update(&connCreateMsg{conn: conn})
		if m.msg != "Connected. Handshaking..." {
			t.Errorf("Unexpected message: %q", m.msg)
		}
		if m.conn != conn {
			t.Error("expected connection to be set")
		}
	})

	t.Run("connCloseMsg", func(t *testing.T) {
		m.conn = &websocket.Conn{}
		m.Update(&connCloseMsg{})
		if m.conn != nil {
			t.Error("expected connection to be cleared")
		}
	})

	t.Run("helloMsg", func(t *testing.T) {
		m.Update(&helloMsg{heartbeatInterval: 100})
	})

	t.Run("qrCodeMsg", func(t *testing.T) {
		m.Update(&qrCodeMsg{qrCode: nil})
		if m.msg != "Scan this with the Discord mobile app to log in instantly." {
			t.Errorf("Unexpected message: %q", m.msg)
		}
	})

	t.Run("userMsg_Branches", func(t *testing.T) {
		m.Update(&userMsg{username: "user", discriminator: "1234"})
		m.Update(&userMsg{username: "user2", discriminator: "0"})
	})

	t.Run("heartbeatTickMsg_WithConn", func(t *testing.T) {
		m.conn = &websocket.Conn{} // Dummy
		m.Update(&heartbeatTickMsg{})
	})

	t.Run("EventError", func(t *testing.T) {
		m.Update(tcell.NewEventError(errors.New("fail")))
		if m.msg != "fail" {
			t.Errorf("Expected message 'fail', got %q", m.msg)
		}
	})

	t.Run("pendingRemoteInitMsg", func(t *testing.T) {
		m.Update(&pendingRemoteInitMsg{fingerprint: "test-fp"})
		if m.fingerprint != "test-fp" {
			t.Errorf("Expected fingerprint to be set")
		}
	})

	t.Run("pendingTicketMsg", func(t *testing.T) {
		m.Update(&pendingTicketMsg{encryptedUserPayload: "test-payload"})
	})

	t.Run("pendingLoginMsg", func(t *testing.T) {
		m.Update(&pendingLoginMsg{ticket: "test-ticket"})
		if m.msg != "Authenticating..." {
			t.Errorf("Unexpected message: %q", m.msg)
		}
	})

	t.Run("cancelMsg", func(t *testing.T) {
		m.Update(&cancelMsg{})
		if m.msg != "Login canceled on mobile" {
			t.Errorf("Unexpected message: %q", m.msg)
		}
	})

	t.Run("privateKeyMsg", func(t *testing.T) {
		key, _ := rsa.GenerateKey(rand.Reader, 2048)
		m.Update(&privateKeyMsg{privateKey: key})
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
		if _, ok := event.(*connCreateMsg); !ok {
			t.Errorf("Expected connCreateMsg, got %T", event)
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
			{"Hello", "hello", map[string]any{"heartbeat_interval": 100, "timeout_ms": 1000}, &helloMsg{}},
			{"NonceProof", "nonce_proof", map[string]any{"encrypted_nonce": "nonce"}, &nonceProofMsg{}},
			{"PendingRemoteInit", "pending_remote_init", map[string]any{"fingerprint": "fp"}, &pendingRemoteInitMsg{}},
			{"PendingTicket", "pending_ticket", map[string]any{"encrypted_user_payload": "payload"}, &pendingTicketMsg{}},
			{"PendingLogin", "pending_login", map[string]any{"ticket": "ticket"}, &pendingLoginMsg{}},
			{"Cancel", "cancel", map[string]any{}, &cancelMsg{}},
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
		if _, ok := event.(*heartbeatTickMsg); !ok {
			t.Errorf("Expected heartbeatTickMsg, got %T", event)
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
		if _, ok := event.(*privateKeyMsg); !ok {
			t.Errorf("Expected privateKeyMsg, got %T", event)
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
		if e, ok := event.(*userMsg); !ok || e.username != "user" || e.discriminator != "1234" {
			t.Errorf("Expected userMsg user:user disc:1234, got %v", event)
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
		if e, ok := event.(*TokenMsg); !ok || e.Token != token {
			t.Errorf("Expected TokenMsg with token, got %v", event)
		}
	})

	t.Run("generateQRCode_Success", func(t *testing.T) {
		cmd := m.generateQRCode("fp")
		event := runCommand(t, cmd)
		if _, ok := event.(*qrCodeMsg); !ok {
			t.Errorf("Expected qrCodeMsg, got %T", event)
		}
	})
}

func reflectTypeMatch(got, want any) (any, bool) {
	switch want.(type) {
	case *helloMsg:
		_, ok := got.(*helloMsg)
		return got, ok
	case *nonceProofMsg:
		_, ok := got.(*nonceProofMsg)
		return got, ok
	case *pendingRemoteInitMsg:
		_, ok := got.(*pendingRemoteInitMsg)
		return got, ok
	case *pendingTicketMsg:
		_, ok := got.(*pendingTicketMsg)
		return got, ok
	case *pendingLoginMsg:
		_, ok := got.(*pendingLoginMsg)
		return got, ok
	case *cancelMsg:
		_, ok := got.(*cancelMsg)
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
