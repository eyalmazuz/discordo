package root

import (
	"log/slog"

	"github.com/ayn2op/discordo/internal/clipboard"
	"github.com/ayn2op/discordo/internal/keyring"
	"github.com/ayn2op/tview"
	"github.com/gdamore/tcell/v3"
)

type tokenEvent struct {
	tcell.EventTime
	token string
}

func newTokenEvent(token string) *tokenEvent {
	return &tokenEvent{token: token}
}

func tokenCommand(token string) tview.Command {
	return func() tcell.Event {
		return newTokenEvent(token)
	}
}

type loginEvent struct{ tcell.EventTime }

var (
	getStoredToken    = keyring.GetToken
	setStoredToken    = keyring.SetToken
	deleteStoredToken = keyring.DeleteToken
	initClipboardFn   = clipboard.Init
)

func newLoginEvent() *loginEvent {
	return &loginEvent{}
}

func getToken() tview.Command {
	return func() tcell.Event {
		token, err := getStoredToken()
		if err != nil {
			slog.Info("failed to retrieve token from keyring", "err", err)
			return newLoginEvent()
		}
		return newTokenEvent(token)
	}
}

func setToken(token string) tview.Command {
	return func() tcell.Event {
		if err := setStoredToken(token); err != nil {
			slog.Error("failed to set token to keyring", "err", err)
			return tcell.NewEventError(err)
		}
		return nil
	}
}

func deleteToken() tview.Command {
	return func() tcell.Event {
		if err := deleteStoredToken(); err != nil {
			slog.Error("failed to delete token from keyring", "err", err)
			return tcell.NewEventError(err)
		}
		return nil
	}
}

func initClipboard() tview.Command {
	return func() tcell.Event {
		if err := initClipboardFn(); err != nil {
			slog.Error("failed to init clipboard", "err", err)
			return tcell.NewEventError(err)
		}
		return nil
	}
}
