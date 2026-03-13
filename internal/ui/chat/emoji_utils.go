package chat

import (
	"log/slog"

	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/ningen/v3"
)

func availableEmojisForChannel(state *ningen.State, c *discord.Channel) []discord.Emoji {
	if state == nil || c == nil {
		return nil
	}

	var emojis []discord.Emoji
	seen := make(map[discord.EmojiID]struct{})

	appendGuildEmojis := func(guildID discord.GuildID, allowFetch bool) {
		if !guildID.IsValid() {
			return
		}

		guildEmojis, err := state.Cabinet.Emojis(guildID)
		if err != nil && allowFetch {
			guildEmojis, err = state.Emojis(guildID)
		}
		if err != nil {
			if allowFetch {
				slog.Error("failed to fetch emojis", "guild_id", guildID, "err", err)
			}
			return
		}

		for _, emoji := range guildEmojis {
			if _, ok := seen[emoji.ID]; ok {
				continue
			}
			seen[emoji.ID] = struct{}{}
			emojis = append(emojis, emoji)
		}
	}

	appendGuildEmojis(c.GuildID, true)

	me, err := state.Cabinet.Me()
	if err != nil || me.Nitro == discord.NoUserNitro {
		return emojis
	}

	guilds, err := state.Cabinet.Guilds()
	if err != nil {
		return emojis
	}

	for _, guild := range guilds {
		if guild.ID == c.GuildID {
			continue
		}
		appendGuildEmojis(guild.ID, false)
	}

	return emojis
}
