package qr

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/ayn2op/discordo/internal/http"
	"github.com/eyalmazuz/tview"
	"github.com/diamondburned/arikawa/v3/utils/httputil"
	"github.com/gdamore/tcell/v3"
	"github.com/gorilla/websocket"
	"github.com/skip2/go-qrcode"
)

type errMsg struct {
	tcell.EventTime
	err error
}

func newErrMsg(err error) *errMsg {
	return &errMsg{err: err}
}

type TokenMsg struct {
	tcell.EventTime
	Token string
}

const remoteAuthGatewayURL = "wss://remote-auth-gateway.discord.gg/?v=2"

type connCreateMsg struct {
	tcell.EventTime
	conn *websocket.Conn
}

type connCloseMsg struct{ tcell.EventTime }

func (m *Model) connect() tview.Cmd {
	return func() tview.Msg {
		headers := http.Headers()
		headers.Set("User-Agent", http.BrowserUserAgent)
		conn, _, err := websocket.DefaultDialer.Dial(remoteAuthGatewayURL, headers)
		if err != nil {
			return newErrMsg(err)
		}
		return &connCreateMsg{conn: conn}
	}
}

func (m *Model) close() tview.Cmd {
	return func() tview.Msg {
		if m.conn != nil {
			if err := m.conn.Close(); err != nil {
				return newErrMsg(err)
			}
		}
		return &connCloseMsg{}
	}
}

type helloMsg struct {
	tcell.EventTime
	heartbeatInterval int
	timeoutMS         int
}

type nonceProofMsg struct {
	tcell.EventTime
	encryptedNonce string
}

type pendingRemoteInitMsg struct {
	tcell.EventTime
	fingerprint string
}

type pendingTicketMsg struct {
	tcell.EventTime
	encryptedUserPayload string
}

type pendingLoginMsg struct {
	tcell.EventTime
	ticket string
}

type cancelMsg struct{ tcell.EventTime }

func (m *Model) listen() tview.Cmd {
	return func() tview.Msg {
		if m.conn == nil {
			return nil
		}

		_, data, err := m.conn.ReadMessage()
		if err != nil {
			return newErrMsg(err)
		}

		var payload struct {
			Op string `json:"op"`
		}
		if err := json.Unmarshal(data, &payload); err != nil {
			return newErrMsg(err)
		}

		switch payload.Op {
		case "hello":
			var payload struct {
				HeartbeatInterval int `json:"heartbeat_interval"`
				TimeoutMS         int `json:"timeout_ms"`
			}
			if err := json.Unmarshal(data, &payload); err != nil {
				return newErrMsg(err)
			}
			return &helloMsg{heartbeatInterval: payload.HeartbeatInterval, timeoutMS: payload.TimeoutMS}
		case "nonce_proof":
			var payload struct {
				EncryptedNonce string `json:"encrypted_nonce"`
			}
			if err := json.Unmarshal(data, &payload); err != nil {
				return newErrMsg(err)
			}
			return &nonceProofMsg{encryptedNonce: payload.EncryptedNonce}
		case "pending_remote_init":
			var payload struct {
				Fingerprint string `json:"fingerprint"`
			}
			if err := json.Unmarshal(data, &payload); err != nil {
				return newErrMsg(err)
			}
			return &pendingRemoteInitMsg{fingerprint: payload.Fingerprint}
		case "pending_ticket":
			var payload struct {
				EncryptedUserPayload string `json:"encrypted_user_payload"`
			}
			if err := json.Unmarshal(data, &payload); err != nil {
				return newErrMsg(err)
			}
			return &pendingTicketMsg{encryptedUserPayload: payload.EncryptedUserPayload}
		case "cancel":
			return &cancelMsg{}
		case "pending_login":
			var payload struct {
				Ticket string `json:"ticket"`
			}
			if err := json.Unmarshal(data, &payload); err != nil {
				return newErrMsg(err)
			}
			return &pendingLoginMsg{ticket: payload.Ticket}
		default:
			return nil
		}
	}
}

type heartbeatTickMsg struct{ tcell.EventTime }

func (m *Model) heartbeat() tview.Cmd {
	return func() tview.Msg {
		time.Sleep(m.heartbeatInterval)
		return &heartbeatTickMsg{}
	}
}

func (m *Model) sendHeartbeat() tview.Cmd {
	return func() tview.Msg {
		if m.conn == nil {
			return nil
		}
		data := struct {
			Op string `json:"op"`
		}{"heartbeat"}
		if err := m.conn.WriteJSON(data); err != nil {
			return newErrMsg(err)
		}
		return nil
	}
}

type privateKeyMsg struct {
	tcell.EventTime
	privateKey *rsa.PrivateKey
}

func (m *Model) generatePrivateKey() tview.Cmd {
	return func() tview.Msg {
		privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			return newErrMsg(err)
		}
		return &privateKeyMsg{privateKey: privateKey}
	}
}

func (m *Model) sendInit() tview.Cmd {
	return func() tview.Msg {
		if m.privateKey == nil {
			return newErrMsg(errors.New("missing private key"))
		}
		spki, err := x509.MarshalPKIXPublicKey(m.privateKey.Public())
		if err != nil {
			return newErrMsg(err)
		}
		encodedPublicKey := base64.StdEncoding.EncodeToString(spki)
		data := struct {
			Op               string `json:"op"`
			EncodedPublicKey string `json:"encoded_public_key"`
		}{"init", encodedPublicKey}
		if err := m.conn.WriteJSON(data); err != nil {
			return newErrMsg(err)
		}
		return nil
	}
}

func (m *Model) sendNonceProof(encryptedNonce string) tview.Cmd {
	return func() tview.Msg {
		decodedNonce, err := base64.StdEncoding.DecodeString(encryptedNonce)
		if err != nil {
			return newErrMsg(err)
		}

		decryptedNonce, err := rsa.DecryptOAEP(sha256.New(), nil, m.privateKey, decodedNonce, nil)
		if err != nil {
			return newErrMsg(err)
		}

		encodedNonce := base64.RawURLEncoding.EncodeToString(decryptedNonce)
		data := struct {
			Op    string `json:"op"`
			Nonce string `json:"nonce"`
		}{"nonce_proof", encodedNonce}
		if err := m.conn.WriteJSON(data); err != nil {
			return newErrMsg(err)
		}
		return nil
	}
}

type qrCodeMsg struct {
	tcell.EventTime
	qrCode *qrcode.QRCode
}

func (m *Model) generateQRCode(fingerprint string) tview.Cmd {
	return func() tview.Msg {
		content := "https://discord.com/ra/" + fingerprint
		qrCode, err := qrcode.New(content, qrcode.Low)
		if err != nil {
			return newErrMsg(err)
		}
		qrCode.DisableBorder = true
		return &qrCodeMsg{qrCode: qrCode}
	}
}

type userMsg struct {
	tcell.EventTime
	discriminator string
	username      string
}

func (m *Model) decryptUserPayload(encryptedPayload string) tview.Cmd {
	return func() tview.Msg {
		decodedPayload, err := base64.StdEncoding.DecodeString(encryptedPayload)
		if err != nil {
			return newErrMsg(err)
		}

		decryptedPayload, err := rsa.DecryptOAEP(sha256.New(), nil, m.privateKey, decodedPayload, nil)
		if err != nil {
			return newErrMsg(err)
		}

		parts := strings.Split(string(decryptedPayload), ":")
		if len(parts) != 4 {
			return newErrMsg(errors.New("invalid user payload"))
		}

		return &userMsg{discriminator: parts[1], username: parts[3]}
	}
}

func (m *Model) exchangeTicket(ticket string) tview.Cmd {
	return func() tview.Msg {
		headers := http.Headers()
		headers.Set("Referer", "https://discord.com/login")
		if m.fingerprint != "" {
			headers.Set("X-Fingerprint", m.fingerprint)
		}

		client := http.NewClient("")
		client.OnRequest = append(client.OnRequest, httputil.WithHeaders(headers))

		encryptedToken, err := client.ExchangeRemoteAuthTicket(ticket)
		if err != nil {
			return newErrMsg(err)
		}

		decodedToken, err := base64.StdEncoding.DecodeString(encryptedToken)
		if err != nil {
			return newErrMsg(err)
		}

		decryptedToken, err := rsa.DecryptOAEP(sha256.New(), nil, m.privateKey, decodedToken, nil)
		if err != nil {
			return newErrMsg(err)
		}
		return &TokenMsg{Token: string(decryptedToken)}
	}
}
