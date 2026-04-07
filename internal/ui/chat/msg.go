package chat

import (
	"log/slog"

	"github.com/ayn2op/discordo/internal/http"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/diamondburned/arikawa/v3/utils/httputil"
	"github.com/eyalmazuz/tview"
	"github.com/gdamore/tcell/v3"
)

func (m *Model) openState() tview.Cmd {
	return func() tview.Msg {
		if err := openNingenState(m.state); err != nil {
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

func (m *Model) OpenState(token string) error {
	identifyProps := http.IdentifyProperties()
	gateway.DefaultIdentity = identifyProps
	gateway.DefaultPresence = &gateway.UpdatePresenceCommand{
		Status: m.cfg.Status,
	}

	id := gateway.DefaultIdentifier(token)
	id.Compress = false

	m.state = newOpenState(token, id)
	m.events = make(chan gateway.Event)
	m.state.AddHandler(m.events)
	m.state.StateLog = func(err error) {
		slog.Error("state log", "err", err)
	}
	m.state.OnRequest = append(m.state.OnRequest, httputil.WithHeaders(http.Headers()), m.onRequest)
	return openNingenState(m.state)
}

func (m *Model) CloseState() error {
	if m.state == nil {
		return nil
	}
	return m.state.Close()
}
