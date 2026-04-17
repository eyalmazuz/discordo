package chat

import (
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/ningen/v3"
)

func channelUnreadIndication(state *ningen.State, channel discord.Channel, opts ningen.UnreadOpts) ningen.UnreadIndication {
	if state == nil {
		return ningen.ChannelRead
	}

	readState := state.ReadState.ReadState(channel.ID)
	if readState == nil || !readState.LastMessageID.IsValid() {
		return ningen.ChannelRead
	}

	if readState.MentionCount > 0 {
		return ningen.ChannelMentioned
	}

	lastMessageID := state.LastMessage(channel.ID)
	if !lastMessageID.IsValid() {
		return ningen.ChannelRead
	}

	if !channel.GuildID.IsValid() {
		if readState.LastMessageID < lastMessageID {
			return ningen.ChannelUnread
		}
		return ningen.ChannelRead
	}

	return state.ChannelIsUnread(channel.ID, opts)
}

func channelUnreadIndicationByID(state *ningen.State, channelID discord.ChannelID, opts ningen.UnreadOpts) ningen.UnreadIndication {
	if state == nil {
		return ningen.ChannelRead
	}

	channel, err := state.Cabinet.Channel(channelID)
	if err != nil {
		return state.ChannelIsUnread(channelID, opts)
	}

	return channelUnreadIndication(state, *channel, opts)
}

func channelUnreadCount(state *ningen.State, channel discord.Channel, opts ningen.UnreadOpts) int {
	if state == nil {
		return 0
	}

	messages, _ := state.Cabinet.Messages(channel.ID)
	readState := state.ReadState.ReadState(channel.ID)
	if readState == nil || !readState.LastMessageID.IsValid() {
		return 0
	}

	unread := 0
	if messages != nil {
		for _, message := range messages {
			if message.ID > readState.LastMessageID {
				unread++
			} else {
				break
			}
		}
	} else if channelUnreadIndication(state, channel, opts) != ningen.ChannelRead {
		unread = 1
	}

	return unread
}
