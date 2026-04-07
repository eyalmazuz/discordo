package qr

import (
	"github.com/gorilla/websocket"
	"github.com/diamondburned/arikawa/v3/api"
	"github.com/skip2/go-qrcode"
)

var (
	wsDial           = websocket.DefaultDialer.Dial
	wsClose          = func(conn *websocket.Conn) error { return conn.Close() }
	wsReadMessage    = func(conn *websocket.Conn) (int, []byte, error) { return conn.ReadMessage() }
	wsWriteJSON      = func(conn *websocket.Conn, v any) error { return conn.WriteJSON(v) }
	exchangeTicketFn = func(client *api.Client, ticket string) (string, error) { return client.ExchangeRemoteAuthTicket(ticket) }
	qrCodeNew        = qrcode.New
)
