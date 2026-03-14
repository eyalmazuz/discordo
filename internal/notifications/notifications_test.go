package notifications

import (
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/ayn2op/discordo/internal/config"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/diamondburned/arikawa/v3/session"
	arikawastate "github.com/diamondburned/arikawa/v3/state"
	"github.com/diamondburned/arikawa/v3/state/store/defaultstore"
	"github.com/diamondburned/ningen/v3"
)

func newNotificationState() *ningen.State {
	s := arikawastate.NewFromSession(session.New(""), defaultstore.New())
	st := ningen.FromState(s)
	st.Cabinet.MeStore.MyselfSet(discord.User{ID: 1, Username: "me"}, false)
	return st
}

func newNotificationConfig(t *testing.T) *config.Config {
	t.Helper()

	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	cfg.Notifications.Enabled = true
	return cfg
}

func messageEvent(channelID discord.ChannelID, guildID discord.GuildID, content string) *gateway.MessageCreateEvent {
	return &gateway.MessageCreateEvent{
		Message: discord.Message{
			ID:        55,
			ChannelID: channelID,
			GuildID:   guildID,
			Content:   content,
			Author:    discord.User{ID: 2, Username: "sender", Avatar: "avatar"},
			Mentions:  []discord.GuildUser{{User: discord.User{ID: 1, Username: "me"}}},
		},
	}
}

func TestNotify_GatingAndErrors(t *testing.T) {
	oldDesktopNotify := desktopNotify
	oldCachedProfileImage := cachedProfileImage
	t.Cleanup(func() {
		desktopNotify = oldDesktopNotify
		cachedProfileImage = oldCachedProfileImage
	})

	state := newNotificationState()
	cfg := newNotificationConfig(t)
	channelID := discord.ChannelID(100)
	state.Cabinet.ChannelStore.ChannelSet(&discord.Channel{ID: channelID, Type: discord.DirectMessage}, false)

	var calls int
	desktopNotify = func(title, message, image string, playSound bool, duration int) error {
		calls++
		return nil
	}
	cachedProfileImage = func(discord.Hash, string) (string, error) { return "", nil }

	cfg.Notifications.Enabled = false
	if err := Notify(state, messageEvent(channelID, 0, "hello"), cfg); err != nil {
		t.Fatalf("Notify with disabled notifications: %v", err)
	}
	cfg.Notifications.Enabled = true
	if calls != 0 {
		t.Fatalf("expected no desktop notification when disabled, got %d", calls)
	}

	cfg.Status = discord.DoNotDisturbStatus
	if err := Notify(state, messageEvent(channelID, 0, "hello"), cfg); err != nil {
		t.Fatalf("Notify with DND status: %v", err)
	}
	cfg.Status = ""
	if calls != 0 {
		t.Fatalf("expected no desktop notification in DND mode, got %d", calls)
	}

	guildChannelID := discord.ChannelID(101)
	state.Cabinet.ChannelStore.ChannelSet(&discord.Channel{ID: guildChannelID, GuildID: 88, Type: discord.GuildText}, false)
	if err := Notify(state, messageEvent(guildChannelID, 88, "hello"), cfg); err == nil {
		t.Fatal("expected missing guild lookup to fail")
	}

	if err := Notify(state, messageEvent(discord.ChannelID(999), 0, "hello"), cfg); err == nil {
		t.Fatal("expected missing channel lookup to fail")
	}
}

func TestNotify_NoMentionEmptyContentAndDesktopError(t *testing.T) {
	oldDesktopNotify := desktopNotify
	oldCachedProfileImage := cachedProfileImage
	t.Cleanup(func() {
		desktopNotify = oldDesktopNotify
		cachedProfileImage = oldCachedProfileImage
	})

	state := newNotificationState()
	cfg := newNotificationConfig(t)
	cachedProfileImage = func(discord.Hash, string) (string, error) { return "", nil }

	guildID := discord.GuildID(400)
	guildChannelID := discord.ChannelID(401)
	state.Cabinet.ChannelStore.ChannelSet(&discord.Channel{ID: guildChannelID, GuildID: guildID, Type: discord.GuildText, Name: "general"}, false)
	state.Cabinet.GuildStore.GuildSet(&discord.Guild{ID: guildID, Name: "guild"}, false)

	noMention := messageEvent(guildChannelID, guildID, "hello")
	noMention.Author.ID = 1
	called := false
	desktopNotify = func(string, string, string, bool, int) error {
		called = true
		return nil
	}
	if err := Notify(state, noMention, cfg); err != nil {
		t.Fatalf("Notify without mentions returned error: %v", err)
	}
	if called {
		t.Fatal("expected no desktop notification when the message does not mention the user")
	}

	called = false
	empty := messageEvent(guildChannelID, guildID, "")
	if err := Notify(state, empty, cfg); err != nil {
		t.Fatalf("Notify with empty content returned error: %v", err)
	}
	if called {
		t.Fatal("expected empty-content message without attachments to be ignored")
	}

	dmID := discord.ChannelID(402)
	state.Cabinet.ChannelStore.ChannelSet(&discord.Channel{ID: dmID, Type: discord.DirectMessage}, false)
	desktopNotify = func(string, string, string, bool, int) error { return errors.New("notify") }
	if err := Notify(state, messageEvent(dmID, 0, "hello"), cfg); err == nil {
		t.Fatal("expected desktop notification failure to bubble up")
	}
}

func TestNotify_ContentTitleSoundAndImageFallback(t *testing.T) {
	oldDesktopNotify := desktopNotify
	oldCachedProfileImage := cachedProfileImage
	t.Cleanup(func() {
		desktopNotify = oldDesktopNotify
		cachedProfileImage = oldCachedProfileImage
	})

	state := newNotificationState()
	cfg := newNotificationConfig(t)
	cfg.Notifications.Sound.Enabled = true
	cfg.Notifications.Sound.OnlyOnPing = true
	cfg.Notifications.Duration = 7

	channelID := discord.ChannelID(200)
	guildID := discord.GuildID(300)
	state.Cabinet.ChannelStore.ChannelSet(&discord.Channel{ID: channelID, GuildID: guildID, Name: "general", Type: discord.GuildText}, false)
	state.Cabinet.GuildStore.GuildSet(&discord.Guild{ID: guildID, Name: "guild"}, false)

	ev := messageEvent(channelID, guildID, "")
	ev.Member = &discord.Member{Nick: "nick"}
	ev.Attachments = []discord.Attachment{{Filename: "report.txt"}}

	var gotTitle, gotMessage, gotImage string
	var gotSound bool
	var gotDuration int
	desktopNotify = func(title, message, image string, playSound bool, duration int) error {
		gotTitle = title
		gotMessage = message
		gotImage = image
		gotSound = playSound
		gotDuration = duration
		return nil
	}
	cachedProfileImage = func(hash discord.Hash, url string) (string, error) {
		if hash != "avatar" {
			t.Fatalf("expected avatar hash, got %q", hash)
		}
		if url == "" {
			t.Fatal("expected avatar URL")
		}
		return "", errors.New("cache miss")
	}

	if err := Notify(state, ev, cfg); err != nil {
		t.Fatalf("Notify returned error: %v", err)
	}
	if gotTitle != "nick (#general, guild)" {
		t.Fatalf("unexpected title: %q", gotTitle)
	}
	if gotMessage != "Uploaded report.txt" {
		t.Fatalf("unexpected notification message: %q", gotMessage)
	}
	if gotImage != "" {
		t.Fatalf("expected empty image path on cache error, got %q", gotImage)
	}
	if !gotSound {
		t.Fatal("expected ping notification to chime")
	}
	if gotDuration != 7 {
		t.Fatalf("expected duration 7, got %d", gotDuration)
	}

	cfg.Notifications.Sound.OnlyOnPing = false
	ev = messageEvent(channelID, guildID, "body")
	ev.Author.Avatar = ""
	cachedProfileImage = func(hash discord.Hash, url string) (string, error) {
		if hash != "default" {
			t.Fatalf("expected default avatar hash, got %q", hash)
		}
		return "/tmp/avatar.png", nil
	}

	if err := Notify(state, ev, cfg); err != nil {
		t.Fatalf("Notify with default avatar returned error: %v", err)
	}
	if gotMessage != "body" {
		t.Fatalf("expected explicit content to win over attachment fallback, got %q", gotMessage)
	}
	if gotImage != "/tmp/avatar.png" {
		t.Fatalf("expected cached image path, got %q", gotImage)
	}
}

func TestGetCachedProfileImage(t *testing.T) {
	oldCacheDir := cacheDir
	oldMkdirAll := mkdirAll
	oldStatFile := statFile
	oldCreateFile := createFile
	oldHTTPGet := httpGet
	oldCopyToFile := copyToFile
	t.Cleanup(func() {
		cacheDir = oldCacheDir
		mkdirAll = oldMkdirAll
		statFile = oldStatFile
		createFile = oldCreateFile
		httpGet = oldHTTPGet
		copyToFile = oldCopyToFile
	})

	dir := t.TempDir()
	cacheDir = func() string { return dir }
	url := "https://example.com/avatar.png"

	httpGet = func(got string) (*http.Response, error) {
		if got != url {
			t.Fatalf("unexpected avatar URL %q", got)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("avatar-data")),
		}, nil
	}

	path, err := getCachedProfileImage("hash", url)
	if err != nil {
		t.Fatalf("getCachedProfileImage returned error: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read cached avatar: %v", err)
	}
	if string(data) != "avatar-data" {
		t.Fatalf("unexpected cached avatar contents: %q", string(data))
	}

	httpCalled := false
	httpGet = func(string) (*http.Response, error) {
		httpCalled = true
		return nil, errors.New("should not fetch cached avatar")
	}
	secondPath, err := getCachedProfileImage("hash", url)
	if err != nil {
		t.Fatalf("getCachedProfileImage cache hit returned error: %v", err)
	}
	if secondPath != path {
		t.Fatalf("expected cache hit path %q, got %q", path, secondPath)
	}
	if httpCalled {
		t.Fatal("expected cache hit to skip HTTP fetch")
	}

	mkdirAll = func(string, os.FileMode) error { return errors.New("mkdir") }
	if _, err := getCachedProfileImage("hash2", url); err == nil {
		t.Fatal("expected mkdir failure")
	}

	mkdirAll = oldMkdirAll
	statFile = func(string) (os.FileInfo, error) { return nil, os.ErrNotExist }
	createFile = func(string) (*os.File, error) { return nil, errors.New("create") }
	if _, err := getCachedProfileImage("hash3", url); err == nil {
		t.Fatal("expected create failure")
	}

	createFile = oldCreateFile
	httpGet = func(string) (*http.Response, error) { return nil, errors.New("get") }
	if _, err := getCachedProfileImage("hash4", url); err == nil {
		t.Fatal("expected HTTP failure")
	}

	httpGet = func(string) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("avatar-data")),
		}, nil
	}
	copyToFile = func(io.Writer, io.Reader) (int64, error) { return 0, errors.New("copy") }
	if _, err := getCachedProfileImage("hash5", url); err == nil {
		t.Fatal("expected copy failure")
	}
}
