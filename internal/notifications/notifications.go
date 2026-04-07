package notifications

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"github.com/ayn2op/discordo/internal/config"
	"github.com/ayn2op/discordo/internal/consts"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/diamondburned/ningen/v3"
)

var (
	desktopNotify      = sendDesktopNotificationImpl
	cachedProfileImage = getCachedProfileImageImpl

	cacheDir   = consts.CacheDir
	mkdirAll   = os.MkdirAll
	statFile   = os.Stat
	createFile = os.Create
	httpGet    = http.Get
	copyToFile = io.Copy
)

func Notify(state *ningen.State, message gateway.MessageCreateEvent, cfg *config.Config) error {
	if !cfg.Notifications.Enabled || cfg.Status == discord.DoNotDisturbStatus {
		return nil
	}

	mentions := state.MessageMentions(&message.Message)
	if mentions == 0 {
		return nil
	}

	// Handle sent files
	content := message.Content
	if message.Content == "" && len(message.Attachments) > 0 {
		content = "Uploaded " + message.Attachments[0].Filename
	}

	if content == "" {
		return nil
	}

	title := message.Author.DisplayOrUsername()

	channel, err := state.Cabinet.Channel(message.ChannelID)
	if err != nil {
		return fmt.Errorf("failed to get channel from state: %w", err)
	}

	if channel.GuildID.IsValid() {
		guild, err := state.Cabinet.Guild(channel.GuildID)
		if err != nil {
			return fmt.Errorf("failed to get guild from state: %w", err)
		}

		if member := message.Member; member != nil && member.Nick != "" {
			title = member.Nick
		}

		title += " (#" + channel.Name + ", " + guild.Name + ")"
	}

	hash := message.Author.Avatar
	if hash == "" {
		hash = "default"
	}

	imagePath, err := cachedProfileImage(hash, message.Author.AvatarURLWithType(discord.PNGImage))
	if err != nil {
		slog.Info("failed to get profile image from cache for notification", "err", err, "hash", hash)
	}

	shouldChime := cfg.Notifications.Sound.Enabled && (!cfg.Notifications.Sound.OnlyOnPing || mentions.Has(ningen.MessageMentions|ningen.MessageNotifies))
	if err := desktopNotify(title, content, imagePath, shouldChime, cfg.Notifications.Duration); err != nil {
		return err
	}

	return nil
}

func getCachedProfileImageImpl(avatarHash discord.Hash, url string) (string, error) {
	path := filepath.Join(cacheDir(), "avatars")
	if err := mkdirAll(path, os.ModePerm); err != nil {
		return "", err
	}

	path = filepath.Join(path, avatarHash+".png")
	if _, err := statFile(path); err == nil {
		return path, nil
	}

	file, err := createFile(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	resp, err := httpGet(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if _, err := copyToFile(file, resp.Body); err != nil {
		return "", err
	}

	return path, nil
}
