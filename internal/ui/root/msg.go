package root

import (
	"log/slog"

	"github.com/ayn2op/discordo/internal/clipboard"
	"github.com/ayn2op/discordo/internal/keyring"
	"github.com/eyalmazuz/tview"
	"github.com/gdamore/tcell/v3"
)

var (
	getStoredToken    = keyring.GetToken
	setStoredToken    = keyring.SetToken
	deleteStoredToken = keyring.DeleteToken
	initClipboardFn   = clipboard.Init
)

type tokenMsg struct {
	tcell.EventTime
	token string
}

func tokenCommand(token string) tview.Cmd {
	return func() tview.Msg {
		return &tokenMsg{token: token}
	}
}

type loginMsg struct{ tcell.EventTime }

func getToken() tview.Cmd {
	return func() tview.Msg {
		token, err := getStoredToken()
		if err != nil {
			slog.Info("failed to retrieve token from keyring", "err", err)
			return &loginMsg{}
		}
		return &tokenMsg{token: token}
	}
}

func setToken(token string) tview.Cmd {
	return func() tview.Msg {
		if err := setStoredToken(token); err != nil {
			slog.Error("failed to set token to keyring", "err", err)
			return tcell.NewEventError(err)
		}
		return nil
	}
}

func deleteToken() tview.Cmd {
	return func() tview.Msg {
		if err := deleteStoredToken(); err != nil {
			slog.Error("failed to delete token from keyring", "err", err)
			return tcell.NewEventError(err)
		}
		return nil
	}
}

func initClipboard() tview.Cmd {
	return func() tview.Msg {
		if err := initClipboardFn(); err != nil {
			slog.Error("failed to init clipboard", "err", err)
			return tcell.NewEventError(err)
		}
		return nil
	}
}
