package chat

import (
	"context"
	"log/slog"

	"github.com/ayn2op/tview"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/gdamore/tcell/v3"
)

func (m *Model) openState() tview.Cmd {
	return func() tview.Msg {
		if err := m.state.Open(context.Background()); err != nil {
			slog.Error("failed to open chat state", "err", err)
			return nil
		}
		return nil
	}
}

func (m *Model) closeState() tview.Cmd {
	return func() tview.Msg {
		if m.state != nil {
			if err := m.state.Close(); err != nil {
				slog.Error("failed to close the session", "err", err)
				return nil
			}
		}
		return nil
	}
}

type gatewayEventMsg struct {
	tcell.EventTime
	gateway.Event
}

func (m *Model) listen() tview.Cmd {
	return func() tview.Msg {
		return &gatewayEventMsg{Event: <-m.events}
	}
}

type channelLoadedMsg struct {
	tcell.EventTime
	Channel  discord.Channel
	Messages []discord.Message
}

type olderMessagesLoadedMsg struct {
	tcell.EventTime
	ChannelID discord.ChannelID
	Older     []discord.Message
}

func newOlderMessagesLoadedMsg(channelID discord.ChannelID, older []discord.Message) *olderMessagesLoadedMsg {
	return &olderMessagesLoadedMsg{ChannelID: channelID, Older: older}
}

type LogoutMsg struct{ tcell.EventTime }

func (m *Model) logout() tview.Cmd {
	return func() tview.Msg {
		return &LogoutMsg{}
	}
}

type QuitMsg struct{ tcell.EventTime }
