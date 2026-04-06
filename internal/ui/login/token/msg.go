package token

import (
	"github.com/ayn2op/tview"
	"github.com/gdamore/tcell/v3"
)

type TokenMsg struct {
	tcell.EventTime
	Token string
}

func tokenCommand(token string) tview.Cmd {
	return func() tview.Msg {
		return &TokenMsg{Token: token}
	}
}
