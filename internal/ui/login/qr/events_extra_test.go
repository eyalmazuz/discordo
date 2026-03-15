package qr

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/eyalmazuz/tview"
	"github.com/diamondburned/arikawa/v3/api"
	"github.com/diamondburned/arikawa/v3/utils/httputil"
	"github.com/diamondburned/arikawa/v3/utils/httputil/httpdriver"
	"github.com/gdamore/tcell/v3"
	"github.com/gorilla/websocket"
	"github.com/skip2/go-qrcode"
)

type qrRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn qrRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestQRCommandsErrorPaths(t *testing.T) {
	t.Run("default delegate wrappers", func(t *testing.T) {
		oldRead := wsReadMessage
		oldWrite := wsWriteJSON
		oldClose := wsClose
		oldExchange := exchangeTicketFn

		assertPanics := func(name string, fn func()) {
			t.Helper()
			defer func() {
				if recover() == nil {
					t.Fatalf("%s did not panic", name)
				}
			}()
			fn()
		}

		assertPanics("wsReadMessage", func() { _, _, _ = oldRead(nil) })
		assertPanics("wsWriteJSON", func() { _ = oldWrite(nil, struct{}{}) })
		assertPanics("wsClose", func() { _ = oldClose(nil) })

		client := api.NewCustomClient("token", httputil.NewClientWithDriver(httpdriver.WrapClient(http.Client{
			Transport: qrRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.Method != http.MethodPost {
					t.Fatalf("method = %s, want POST", req.Method)
				}
				if req.URL.Path != "/api/v9/users/@me/remote-auth/login" {
					t.Fatalf("path = %s, want %s", req.URL.Path, "/api/v9/users/@me/remote-auth/login")
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`{"encrypted_token":"wrapped-token"}`)),
				}, nil
			}),
		})))

		token, err := oldExchange(client, "ticket")
		if err != nil {
			t.Fatalf("default exchange ticket delegate error = %v", err)
		}
		if token != "wrapped-token" {
			t.Fatalf("token = %q, want wrapped-token", token)
		}
	})

	t.Run("connect dial failure", func(t *testing.T) {
		m := NewModel(tview.NewApplication())
		oldWsDial := wsDial
		t.Cleanup(func() { wsDial = oldWsDial })
		wsDial = func(string, http.Header) (*websocket.Conn, *http.Response, error) {
			return nil, nil, errors.New("dial failed")
		}

		event := runCommand(t, m.connect())
		if _, ok := event.(*tcell.EventError); !ok {
			t.Fatalf("expected connect failure to return EventError, got %T", event)
		}
	})

	t.Run("close nil connection", func(t *testing.T) {
		m := NewModel(tview.NewApplication())
		event := runCommand(t, m.close())
		if _, ok := event.(*connCloseEvent); !ok {
			t.Fatalf("expected nil close to return connCloseEvent, got %T", event)
		}
	})

	t.Run("close websocket error", func(t *testing.T) {
		m := NewModel(tview.NewApplication())
		m.conn = &websocket.Conn{}
		oldWsClose := wsClose
		t.Cleanup(func() { wsClose = oldWsClose })
		wsClose = func(*websocket.Conn) error { return errors.New("close failed") }

		event := runCommand(t, m.close())
		if _, ok := event.(*tcell.EventError); !ok {
			t.Fatalf("expected close failure to return EventError, got %T", event)
		}
	})

	t.Run("listen branches", func(t *testing.T) {
		m := NewModel(tview.NewApplication())
		if event := runCommand(t, m.listen()); event != nil {
			t.Fatalf("expected nil listen with no connection, got %T", event)
		}

		m.conn = &websocket.Conn{}
		oldWsReadMessage := wsReadMessage
		t.Cleanup(func() { wsReadMessage = oldWsReadMessage })

		wsReadMessage = func(*websocket.Conn) (int, []byte, error) {
			return websocket.TextMessage, nil, errors.New("read failed")
		}
		if _, ok := runCommand(t, m.listen()).(*tcell.EventError); !ok {
			t.Fatal("expected read failure to return EventError")
		}

		wsReadMessage = func(*websocket.Conn) (int, []byte, error) {
			return websocket.TextMessage, []byte("{"), nil
		}
		if _, ok := runCommand(t, m.listen()).(*tcell.EventError); !ok {
			t.Fatal("expected invalid JSON to return EventError")
		}

		wsReadMessage = func(*websocket.Conn) (int, []byte, error) {
			return websocket.TextMessage, []byte(`{"op":"unknown"}`), nil
		}
		if event := runCommand(t, m.listen()); event != nil {
			t.Fatalf("expected unknown op to be ignored, got %T", event)
		}

		tests := []string{
			`{"op":"hello","heartbeat_interval":"bad"}`,
			`{"op":"nonce_proof","encrypted_nonce":1}`,
			`{"op":"pending_remote_init","fingerprint":1}`,
			`{"op":"pending_ticket","encrypted_user_payload":1}`,
			`{"op":"pending_login","ticket":1}`,
		}
		for _, payload := range tests {
			wsReadMessage = func(*websocket.Conn) (int, []byte, error) {
				return websocket.TextMessage, []byte(payload), nil
			}
			if _, ok := runCommand(t, m.listen()).(*tcell.EventError); !ok {
				t.Fatalf("expected typed payload decode failure for %s", payload)
			}
		}
	})

	t.Run("send heartbeat", func(t *testing.T) {
		m := NewModel(tview.NewApplication())
		if event := runCommand(t, m.sendHeartbeat()); event != nil {
			t.Fatalf("expected nil heartbeat when disconnected, got %T", event)
		}

		m.conn = &websocket.Conn{}
		oldWsWriteJSON := wsWriteJSON
		t.Cleanup(func() { wsWriteJSON = oldWsWriteJSON })
		wsWriteJSON = func(*websocket.Conn, any) error { return errors.New("write failed") }

		if _, ok := runCommand(t, m.sendHeartbeat()).(*tcell.EventError); !ok {
			t.Fatal("expected heartbeat write failure to return EventError")
		}
	})

	t.Run("send init", func(t *testing.T) {
		m := NewModel(tview.NewApplication())
		if _, ok := runCommand(t, m.sendInit()).(*tcell.EventError); !ok {
			t.Fatal("expected missing private key to return EventError")
		}

		m.conn = &websocket.Conn{}
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatalf("generate key: %v", err)
		}
		m.privateKey = key

		oldWsWriteJSON := wsWriteJSON
		t.Cleanup(func() { wsWriteJSON = oldWsWriteJSON })
		wsWriteJSON = func(*websocket.Conn, any) error { return errors.New("write failed") }

		if _, ok := runCommand(t, m.sendInit()).(*tcell.EventError); !ok {
			t.Fatal("expected send init write failure to return EventError")
		}

		m.privateKey = &rsa.PrivateKey{}
		if _, ok := runCommand(t, m.sendInit()).(*tcell.EventError); !ok {
			t.Fatal("expected invalid private key to fail during public key marshaling")
		}
	})

	t.Run("send nonce proof", func(t *testing.T) {
		m := NewModel(tview.NewApplication())
		if _, ok := runCommand(t, m.sendNonceProof("%%%")).(*tcell.EventError); !ok {
			t.Fatal("expected invalid base64 nonce to return EventError")
		}

		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatalf("generate key: %v", err)
		}
		m.privateKey = key
		m.conn = &websocket.Conn{}

		raw := base64.StdEncoding.EncodeToString([]byte("not rsa payload"))
		if _, ok := runCommand(t, m.sendNonceProof(raw)).(*tcell.EventError); !ok {
			t.Fatal("expected invalid encrypted nonce to return EventError")
		}

		nonce := []byte("nonce")
		encryptedNonce, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, &key.PublicKey, nonce, nil)
		if err != nil {
			t.Fatalf("encrypt nonce: %v", err)
		}
		oldWsWriteJSON := wsWriteJSON
		t.Cleanup(func() { wsWriteJSON = oldWsWriteJSON })
		wsWriteJSON = func(*websocket.Conn, any) error { return errors.New("write failed") }
		if _, ok := runCommand(t, m.sendNonceProof(base64.StdEncoding.EncodeToString(encryptedNonce))).(*tcell.EventError); !ok {
			t.Fatal("expected nonce proof write failure to return EventError")
		}
	})

	t.Run("decrypt user payload", func(t *testing.T) {
		m := NewModel(tview.NewApplication())
		if _, ok := runCommand(t, m.decryptUserPayload("%%%")).(*tcell.EventError); !ok {
			t.Fatal("expected invalid base64 payload to return EventError")
		}

		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatalf("generate key: %v", err)
		}
		m.privateKey = key

		raw := base64.StdEncoding.EncodeToString([]byte("not rsa payload"))
		if _, ok := runCommand(t, m.decryptUserPayload(raw)).(*tcell.EventError); !ok {
			t.Fatal("expected invalid encrypted payload to return EventError")
		}

		encoded, err := rsaEncryptToStdBase64(&key.PublicKey, []byte("missing:parts"))
		if err != nil {
			t.Fatalf("encrypt payload: %v", err)
		}
		if _, ok := runCommand(t, m.decryptUserPayload(encoded)).(*tcell.EventError); !ok {
			t.Fatal("expected malformed user payload to return EventError")
		}
	})

	t.Run("exchange ticket", func(t *testing.T) {
		m := NewModel(tview.NewApplication())
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatalf("generate key: %v", err)
		}
		m.privateKey = key

		oldExchangeTicketFn := exchangeTicketFn
		t.Cleanup(func() { exchangeTicketFn = oldExchangeTicketFn })

		exchangeTicketFn = func(*api.Client, string) (string, error) {
			return "", errors.New("exchange failed")
		}
		if _, ok := runCommand(t, m.exchangeTicket("ticket")).(*tcell.EventError); !ok {
			t.Fatal("expected exchange failure to return EventError")
		}

		exchangeTicketFn = func(*api.Client, string) (string, error) {
			return "%%%", nil
		}
		if _, ok := runCommand(t, m.exchangeTicket("ticket")).(*tcell.EventError); !ok {
			t.Fatal("expected invalid base64 token to return EventError")
		}

		exchangeTicketFn = func(*api.Client, string) (string, error) {
			return base64.StdEncoding.EncodeToString([]byte("not rsa payload")), nil
		}
		if _, ok := runCommand(t, m.exchangeTicket("ticket")).(*tcell.EventError); !ok {
			t.Fatal("expected undecryptable token to return EventError")
		}
	})

	t.Run("generate private key and qr code failures", func(t *testing.T) {
		m := NewModel(tview.NewApplication())

		oldRSAGenerateKey := rsaGenerateKey
		oldQRCodeNew := qrCodeNew
		t.Cleanup(func() {
			rsaGenerateKey = oldRSAGenerateKey
			qrCodeNew = oldQRCodeNew
		})

		rsaGenerateKey = func(io.Reader, int) (*rsa.PrivateKey, error) {
			return nil, errors.New("keygen failed")
		}
		if _, ok := runCommand(t, m.generatePrivateKey()).(*tcell.EventError); !ok {
			t.Fatal("expected generatePrivateKey failure to return EventError")
		}

		qrCodeNew = func(string, qrcode.RecoveryLevel) (*qrcode.QRCode, error) {
			return nil, errors.New("qr failed")
		}
		if _, ok := runCommand(t, m.generateQRCode("fingerprint")).(*tcell.EventError); !ok {
			t.Fatal("expected generateQRCode failure to return EventError")
		}
	})

	t.Run("exchange ticket includes fingerprint header path", func(t *testing.T) {
		m := NewModel(tview.NewApplication())
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatalf("generate key: %v", err)
		}
		m.privateKey = key
		m.fingerprint = "fp"

		token := "token"
		encoded, err := rsaEncryptToStdBase64(&key.PublicKey, []byte(token))
		if err != nil {
			t.Fatalf("encrypt token: %v", err)
		}

		oldExchangeTicketFn := exchangeTicketFn
		t.Cleanup(func() { exchangeTicketFn = oldExchangeTicketFn })
		exchangeTicketFn = func(client *api.Client, ticket string) (string, error) {
			if len(client.OnRequest) == 0 {
				t.Fatal("expected exchange client to install request hooks")
			}
			return encoded, nil
		}

		event := runCommand(t, m.exchangeTicket("ticket"))
		tokenEvent, ok := event.(*TokenEvent)
		if !ok || tokenEvent.Token != token {
			t.Fatalf("expected token event with %q, got %#v", token, event)
		}
	})
}

func rsaEncryptToStdBase64(key *rsa.PublicKey, payload []byte) (string, error) {
	encrypted, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, key, payload, nil)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(encrypted), nil
}
