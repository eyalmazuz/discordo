package login

import (
	"log/slog"

	"github.com/ayn2op/discordo/internal/clipboard"
	"github.com/eyalmazuz/tview"
	"github.com/gdamore/tcell/v3"
)

type errMsg struct {
	tcell.EventTime
	err error
}

var writeClipboard = clipboard.Write

func setClipboard(content string) tview.Cmd {
	return func() tview.Msg {
		if err := writeClipboard(clipboard.FmtText, []byte(content)); err != nil {
			slog.Error("failed to copy error message", "err", err)
			return tcell.NewEventError(err)
		}
		return nil
	}
}
