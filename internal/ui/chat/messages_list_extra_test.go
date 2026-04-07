package chat

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
	"unsafe"

	clipkg "github.com/ayn2op/discordo/internal/clipboard"
	"github.com/ayn2op/discordo/internal/config"
	imgpkg "github.com/ayn2op/discordo/internal/image"
	"github.com/diamondburned/arikawa/v3/api"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/diamondburned/arikawa/v3/state"
	"github.com/diamondburned/arikawa/v3/utils/ws"
	"github.com/diamondburned/ningen/v3/states/relationship"
	"github.com/eyalmazuz/tview"
	"github.com/eyalmazuz/tview/layers"
	"github.com/eyalmazuz/tview/picker"
	"github.com/gdamore/tcell/v3"
)

type lockingTTYScreen struct {
	completeMockScreen
	lockCalls int
	tty       tcell.Tty
}

func (s *lockingTTYScreen) LockRegion(x, y, width, height int, lock bool) {
	s.lockCalls++
}

func (s *lockingTTYScreen) Tty() (tcell.Tty, bool) { return s.tty, true }

type windowSizeErrTty struct{}

func (windowSizeErrTty) Close() error                { return nil }
func (windowSizeErrTty) Read([]byte) (int, error)    { return 0, nil }
func (windowSizeErrTty) Write(p []byte) (int, error) { return len(p), nil }
func (windowSizeErrTty) Size() (int, int, error)     { return 80, 24, nil }
func (windowSizeErrTty) Drain() error                { return nil }
func (windowSizeErrTty) NotifyResize(chan<- bool)    {}
func (windowSizeErrTty) Stop() error                 { return nil }
func (windowSizeErrTty) Start() error                { return nil }
func (windowSizeErrTty) WindowSize() (tcell.WindowSize, error) {
	return tcell.WindowSize{}, errors.New("window size")
}

type zeroCellTty struct{}

func (zeroCellTty) Close() error                { return nil }
func (zeroCellTty) Read([]byte) (int, error)    { return 0, nil }
func (zeroCellTty) Write(p []byte) (int, error) { return len(p), nil }
func (zeroCellTty) Size() (int, int, error)     { return 80, 24, nil }
func (zeroCellTty) Drain() error                { return nil }
func (zeroCellTty) NotifyResize(chan<- bool)    {}
func (zeroCellTty) Stop() error                 { return nil }
func (zeroCellTty) Start() error                { return nil }
func (zeroCellTty) WindowSize() (tcell.WindowSize, error) {
	return tcell.WindowSize{Width: 80, Height: 24}, nil
}

func TestMessagesList_Branches(t *testing.T) {
	m := newTestModel()
	ml := m.messagesList

	t.Run("Draw", func(t *testing.T) {
		screen := &completeMockScreen{}
		ml.Draw(screen)
	})

	t.Run("writeMessage_Branches", func(t *testing.T) {
		builder := tview.NewLineBuilder()
		baseStyle := tcell.StyleDefault

		// 1. Blocked user
		m.state.Cabinet.MeStore.MyselfSet(discord.User{ID: 1}, false)
		m.cfg.HideBlockedUsers = true

		// 2. GuildMemberJoin
		msgJoin := discord.Message{Type: discord.GuildMemberJoinMessage, Author: discord.User{Username: "joiner"}}
		ml.writeMessage(builder, msgJoin, baseStyle, false)

		// 3. ChannelPinned
		msgPinned := discord.Message{Type: discord.ChannelPinnedMessage, Author: discord.User{Username: "pinner"}}
		ml.writeMessage(builder, msgPinned, baseStyle, false)
	})

	t.Run("drawAuthor_Member", func(t *testing.T) {
		builder := tview.NewLineBuilder()
		msg := discord.Message{
			Author:  discord.User{ID: 2, Username: "user2"},
			GuildID: 10,
		}
		// Set member in cabinet
		m.state.Cabinet.MemberStore.MemberSet(10, &discord.Member{
			User: discord.User{ID: 2},
			Nick: "nick2",
		}, false)

		ml.drawAuthor(builder, msg, tcell.StyleDefault)
	})

	t.Run("drawContent_MarkdownBranches", func(t *testing.T) {
		builder := tview.NewLineBuilder()
		msg := discord.Message{Content: "```go\nfunc main() {}\n```"}
		m.cfg.Markdown.Enabled = true
		ml.drawContent(builder, msg, tcell.StyleDefault, false)
	})

	t.Run("drawDefaultMessage_Attachments", func(t *testing.T) {
		builder := tview.NewLineBuilder()
		msg := discord.Message{
			Attachments: []discord.Attachment{
				{Filename: "test.txt", URL: "https://example.com/test.txt"},
			},
		}
		ml.drawDefaultMessage(builder, msg, tcell.StyleDefault, false)
	})
}

func TestMessagesList_Navigation_Branches(t *testing.T) {
	m := newTestModel()
	ml := m.messagesList

	t.Run("selectUp_Prepend", func(t *testing.T) {
		ml.messages = []discord.Message{{ID: 100}}
		ml.SetCursor(0)
		ml.selectUp()
	})

	t.Run("selectDown_End", func(t *testing.T) {
		ml.messages = []discord.Message{{ID: 100}}
		ml.SetCursor(0)
		ml.selectDown()
	})

	t.Run("selectReply_Branches", func(t *testing.T) {
		ml.messages = []discord.Message{
			{ID: 1},
			{ID: 2, ReferencedMessage: &discord.Message{ID: 1}},
		}
		ml.SetCursor(1)
		ml.selectReply()

		// Not found
		ml.messages[1].ReferencedMessage.ID = 99
		ml.selectReply()
	})
}

func TestMessagesList_Embeds_AllLines(t *testing.T) {
	m := newTestModel()
	ml := m.messagesList
	builder := tview.NewLineBuilder()

	embed := discord.Embed{
		Title:       "Title",
		URL:         "https://url.com",
		Provider:    &discord.EmbedProvider{Name: "Provider"},
		Author:      &discord.EmbedAuthor{Name: "Author"},
		Description: "Description with \\*escape\\*",
		Fields: []discord.EmbedField{
			{Name: "F1", Value: "V1"},
			{Name: "F2"},
			{Value: "V3"},
		},
		Footer: &discord.EmbedFooter{Text: "Footer"},
		Color:  0xFF0000,
	}

	msg := discord.Message{Embeds: []discord.Embed{embed}}
	ml.drawEmbeds(builder, msg, tcell.StyleDefault)
}

func TestMessagesList_Yank_And_Help(t *testing.T) {
	m := newTestModel()
	ml := m.messagesList
	ml.setMessages([]discord.Message{{ID: 1, ChannelID: 2, Content: "content", Author: discord.User{Username: "user"}}})
	ml.SetCursor(0)

	t.Run("yankID", func(t *testing.T) {
		executeCommand(requireCommand(t, ml.yankMessageID()))
	})
	t.Run("yankContent", func(t *testing.T) {
		ml.yankContent()
	})
	t.Run("yankURL", func(t *testing.T) {
		ml.yankURL()
	})

	t.Run("Help", func(t *testing.T) {
		ml.ShortHelp()
		ml.FullHelp()
	})
}

func TestMessagesList_Actions(t *testing.T) {
	m := newTestModel()
	ml := m.messagesList
	ml.messages = []discord.Message{{ID: 1, Content: "content", Author: discord.User{ID: 1}}} // me.ID=1
	ml.SetCursor(0)

	t.Run("reply", func(t *testing.T) {
		ml.reply(false)
		ml.reply(true)
	})
	t.Run("edit", func(t *testing.T) {
		ml.edit()
	})
	t.Run("confirmDelete", func(t *testing.T) {
		ml.confirmDelete()
	})
	t.Run("delete", func(t *testing.T) {
		ml.delete()
	})
}

func TestMessagesList_JumpAndMembers(t *testing.T) {
	m := newTestModel()
	ml := m.messagesList

	t.Run("jumpToMessage", func(t *testing.T) {
		ml.messages = []discord.Message{{ID: 1}}
		ch := discord.Channel{ID: 123}
		ml.jumpToMessage(ch, 1)
		ml.jumpToMessage(ch, 99) // not found
	})

	t.Run("requestGuildMembers_NoFetch", func(t *testing.T) {
		// If all members in cabinet, no fetch
		msg := discord.Message{Author: discord.User{ID: 1}, GuildID: 10}
		m.state.Cabinet.MemberStore.MemberSet(10, &discord.Member{User: discord.User{ID: 1}}, false)
		ml.requestGuildMembers(10, []discord.Message{msg})
	})
}

func TestMessagesList_Draw_ExtraBranches(t *testing.T) {
	m := newTestModel()
	ml := m.messagesList

	t.Run("drawReplyMessage_NotFound", func(t *testing.T) {
		builder := tview.NewLineBuilder()
		msg := discord.Message{ReferencedMessage: &discord.Message{ID: 99}}
		ml.drawReplyMessage(builder, msg, tcell.StyleDefault, false)
	})
}

func TestMessagesList_KittyHelpers(t *testing.T) {
	m := newTestModel()
	ml := m.messagesList
	ml.cfg.InlineImages.Enabled = true
	ml.useKitty = true

	t.Run("AfterDrawPlacesAndDeletes", func(t *testing.T) {
		tty := &mockTty{}
		screen := &screenWithTty{tty: tty}

		ml.kittyNeedsFullClear = true
		ml.pendingDeletes = []uint32{7}
		ml.imageItemByKey = map[string]*imageItem{
			"img": {
				kittyID:          2,
				pendingPlace:     true,
				kittyPayload:     "AAAA",
				kittyCols:        1,
				kittyRows:        1,
				kittyVisibleRows: 1,
				kittyCropH:       1,
				cellW:            1,
				cellH:            1,
			},
		}

		ml.Draw(screen)
		ml.AfterDraw(screen)

		out := tty.String()
		if !strings.Contains(out, "a=d,d=I,i=2") {
			t.Fatalf("expected per-image kitty delete command for id=2, got %q", out)
		}
		if !strings.Contains(out, "a=d,d=I,i=7") {
			t.Fatalf("expected delete-by-id command, got %q", out)
		}
		if ml.kittyNeedsFullClear || len(ml.pendingDeletes) != 0 {
			t.Fatal("expected AfterDraw to clear pending kitty work")
		}
	})

	t.Run("AfterDrawSuspended", func(t *testing.T) {
		tty := &mockTty{}
		screen := &screenWithTty{tty: tty}
		ml.kittySuspended = true
		ml.kittyNeedsFullClear = true
		ml.pendingDeletes = []uint32{9}

		ml.Draw(screen)
		ml.AfterDraw(screen)

		if !strings.Contains(tty.String(), "a=d,d=I,i=9") {
			t.Fatalf("expected suspended draw to delete pending kitty image by id, got %q", tty.String())
		}
		if ml.kittyNeedsFullClear || len(ml.pendingDeletes) != 0 {
			t.Fatal("expected suspended draw to clear pending state")
		}
		ml.kittySuspended = false
	})

	t.Run("SetKittySuspendedAndUpdateCellDimensions", func(t *testing.T) {
		screen := &ttyScreen{tty: cellSizeTty{}}
		lockScreen := &lockingScreen{}

		ml.imageItemByKey = map[string]*imageItem{
			"img": {
				useKitty:         true,
				kittyPlaced:      true,
				kittyUploaded:    true,
				pendingPlace:     true,
				lockKittyRegion:  true,
				kittyCols:        2,
				kittyVisibleRows: 1,
			},
		}
		ml.emoteItemByKey = map[string]*imageItem{
			"emote": {
				useKitty:         true,
				kittyPlaced:      true,
				kittyUploaded:    true,
				pendingPlace:     true,
				lockKittyRegion:  true,
				kittyCols:        2,
				kittyVisibleRows: 1,
			},
		}

		ml.setKittySuspended(lockScreen, true)
		for _, item := range ml.imageItemByKey {
			if item.useKitty || item.pendingPlace || item.kittyPlaced {
				t.Fatalf("expected suspension to invalidate placed kitty image items, got %+v", item)
			}
		}
		for _, item := range ml.emoteItemByKey {
			if item.useKitty || item.pendingPlace || item.kittyPlaced {
				t.Fatalf("expected suspension to invalidate placed kitty emote items, got %+v", item)
			}
		}
		if len(ml.pendingDeletes) != 2 {
			t.Fatalf("expected 2 pending deletes on suspension, got %d", len(ml.pendingDeletes))
		}
		if lockScreen.lockCalls == 0 {
			t.Fatal("expected suspension to unlock prior kitty regions")
		}

		for _, item := range ml.imageItemByKey {
			item.kittyPayload = "payload"
		}
		ml.updateCellDimensions(screen)
		if ml.cellW != 10 || ml.cellH != 20 {
			t.Fatalf("expected cached cell dimensions 10x20, got %dx%d", ml.cellW, ml.cellH)
		}
		for _, item := range ml.imageItemByKey {
			if item.kittyPayload != "" {
				t.Fatal("expected cell dimension change to invalidate kitty payload")
			}
		}

		ml.setKittySuspended(lockScreen, false)
		for _, item := range ml.imageItemByKey {
			if !item.useKitty {
				t.Fatal("expected unsuspension to restore kitty mode")
			}
		}
	})
}

func TestMessagesList_MiscHelpers(t *testing.T) {
	var nilList *messagesList
	nilList.queueAnimatedDraw()

	empty := &messagesList{}
	empty.queueAnimatedDraw()
	empty.stopAnimatedRedraw()

	if !resolveKittyMode("kitty") {
		t.Fatal("expected kitty renderer to force kitty mode")
	}
	if resolveKittyMode("halfblock") {
		t.Fatal("expected halfblock renderer to disable kitty mode")
	}
}

func TestMessagesListOpenVariantsAndMouseURL(t *testing.T) {
	m := newTestModel()
	ml := m.messagesList

	oldOpenStart := openStart
	oldHTTPGetAttachment := httpGetAttachment
	oldMkdirAllAttachment := mkdirAllAttachment
	oldCreateAttachmentFile := createAttachmentFile
	oldCopyAttachmentData := copyAttachmentData
	t.Cleanup(func() {
		openStart = oldOpenStart
		httpGetAttachment = oldHTTPGetAttachment
		mkdirAllAttachment = oldMkdirAllAttachment
		createAttachmentFile = oldCreateAttachmentFile
		copyAttachmentData = oldCopyAttachmentData
	})

	opened := make(chan string, 8)
	openStart = func(target string) error {
		opened <- target
		return nil
	}

	httpGetAttachment = func(string) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("attachment")),
			Header:     make(http.Header),
		}, nil
	}
	mkdirAllAttachment = func(string, os.FileMode) error { return nil }
	createAttachmentFile = func(string) (*os.File, error) {
		return os.CreateTemp(t.TempDir(), "attachment-*")
	}
	copyAttachmentData = func(dst io.Writer, src io.Reader) (int64, error) {
		return io.Copy(dst, src)
	}

	t.Run("open url", func(t *testing.T) {
		ml.setMessages([]discord.Message{
			{ID: 1, Content: "https://example.com", Author: discord.User{ID: 2}},
		})
		ml.SetCursor(0)
		ml.open()

		select {
		case got := <-opened:
			if got != "https://example.com" {
				t.Fatalf("expected opened URL %q, got %q", "https://example.com", got)
			}
		case <-time.After(300 * time.Millisecond):
			t.Fatal("timed out waiting for opened URL")
		}
	})

	t.Run("open non image attachment", func(t *testing.T) {
		ml.setMessages([]discord.Message{
			{
				ID: 2,
				Attachments: []discord.Attachment{
					{Filename: "report.txt", URL: "https://example.com/report.txt", ContentType: "text/plain"},
				},
				Author: discord.User{ID: 2},
			},
		})
		ml.SetCursor(0)
		ml.open()

		select {
		case got := <-opened:
			if got != "https://example.com/report.txt" {
				t.Fatalf("expected attachment URL %q, got %q", "https://example.com/report.txt", got)
			}
		case <-time.After(300 * time.Millisecond):
			t.Fatal("timed out waiting for attachment URL")
		}
	})

	t.Run("open image attachment", func(t *testing.T) {
		ml.setMessages([]discord.Message{
			{
				ID: 3,
				Attachments: []discord.Attachment{
					{Filename: "image.png", URL: "https://example.com/image.png", ContentType: "image/png"},
				},
				Author: discord.User{ID: 2},
			},
		})
		ml.SetCursor(0)
		ml.open()

		select {
		case got := <-opened:
			if !strings.HasSuffix(got, "/attachments/image.png") && !strings.HasSuffix(got, "\\attachments\\image.png") {
				t.Fatalf("expected cached attachment path to end with attachments/image.png, got %q", got)
			}
		case <-time.After(300 * time.Millisecond):
			t.Fatal("timed out waiting for opened attachment path")
		}
	})

	t.Run("open multiple resources shows picker", func(t *testing.T) {
		ml.setMessages([]discord.Message{
			{
				ID:      4,
				Content: "https://one.example https://two.example",
				Author:  discord.User{ID: 2},
			},
		})
		ml.SetCursor(0)
		ml.open()
		if !m.HasLayer(attachmentsPickerLayerName) {
			t.Fatal("expected attachments layer to be opened for multiple resources")
		}
	})

	t.Run("mouse click on url opens link", func(t *testing.T) {
		ml.SetRect(0, 0, 20, 5)
		ml.lastScreen = &mockEmoteScreen{
			cells: map[string]string{"1,1": "https://clicked.example"},
		}
		event := &tview.MouseMsg{
			EventMouse: *tcell.NewEventMouse(1, 1, tcell.ButtonPrimary, tcell.ModNone),
			Action:     tview.MouseLeftClick,
		}

		ml.Update(event)

		select {
		case got := <-opened:
			if got != "https://clicked.example" {
				t.Fatalf("expected clicked URL %q, got %q", "https://clicked.example", got)
			}
		case <-time.After(300 * time.Millisecond):
			t.Fatal("timed out waiting for clicked URL")
		}
	})
}

func TestMessagesListHandleEventActionBranches(t *testing.T) {
	t.Run("unmatched key returns nil", func(t *testing.T) {
		m := newTestModelWithTransport(&mockTransport{})
		ml := m.messagesList
		if cmd := ml.Update(tcell.NewEventKey(tcell.KeyRune, "x", tcell.ModNone)); cmd != nil {
			t.Fatalf("expected unmatched key to return nil, got %T", cmd)
		}
	})

	t.Run("cancel key clears selection", func(t *testing.T) {
		m := newTestModelWithTransport(&mockTransport{})
		ml := m.messagesList
		ml.setMessages([]discord.Message{{ID: 10, ChannelID: 99, Content: "body", Author: discord.User{ID: 2, Username: "other"}}})
		ml.SetCursor(0)
		ml.Update(tcell.NewEventKey(tcell.KeyEsc, "", tcell.ModNone))
		if ml.Cursor() != -1 {
			t.Fatalf("expected cursor to be cleared, got %d", ml.Cursor())
		}
	})

	t.Run("yank keybinds", func(t *testing.T) {
		m := newTestModelWithTransport(&mockTransport{})
		ml := m.messagesList
		ml.setMessages([]discord.Message{{ID: 10, ChannelID: 99, Content: "body", Author: discord.User{ID: 2, Username: "other"}}})
		copied := stubClipboardWrite(t)
		ml.SetCursor(0)
		executeCommand(requireCommand(t, ml.Update(tcell.NewEventKey(tcell.KeyRune, "i", tcell.ModNone))))
		if got := waitForCopiedText(t, copied); got != "10" {
			t.Fatalf("expected copied id %q, got %q", "10", got)
		}

		executeCommand(requireCommand(t, ml.Update(tcell.NewEventKey(tcell.KeyRune, "y", tcell.ModNone))))
		if got := waitForCopiedText(t, copied); got != "body" {
			t.Fatalf("expected copied content %q, got %q", "body", got)
		}

		executeCommand(requireCommand(t, ml.Update(tcell.NewEventKey(tcell.KeyRune, "u", tcell.ModNone))))
		if got := waitForCopiedText(t, copied); got == "" || !strings.Contains(got, "/channels/") {
			t.Fatalf("expected copied message URL, got %q", got)
		}
	})

	t.Run("reply key focuses composer", func(t *testing.T) {
		m := newTestModelWithTransport(&mockTransport{})
		ml := m.messagesList
		ml.chatView.SetSelectedChannel(&discord.Channel{ID: 99, Type: discord.DirectMessage})
		ml.setMessages([]discord.Message{{ID: 10, ChannelID: 99, Content: "body", Author: discord.User{ID: 2, Username: "other"}}})
		ml.SetCursor(0)
		ml.Update(tcell.NewEventKey(tcell.KeyRune, "R", tcell.ModNone))
		if ml.chatView.messageInput.sendMessageData.Reference == nil || ml.chatView.messageInput.sendMessageData.Reference.MessageID != 10 {
			t.Fatal("expected reply to set message reference")
		}
		if ml.chatView.messageInput.Title() == "" {
			t.Fatal("expected reply to set composer title")
		}
	})

	t.Run("edit key only works for own message", func(t *testing.T) {
		t.Run("foreign message", func(t *testing.T) {
			m := newTestModelWithTransport(&mockTransport{})
			ml := m.messagesList
			ml.chatView.SetSelectedChannel(&discord.Channel{ID: 99, Type: discord.DirectMessage})
			ml.setMessages([]discord.Message{{ID: 10, ChannelID: 99, Content: "body", Author: discord.User{ID: 2, Username: "other"}}})
			ml.SetCursor(0)
			ml.Update(tcell.NewEventKey(tcell.KeyRune, "e", tcell.ModNone))
			if ml.chatView.messageInput.edit {
				t.Fatal("expected foreign message not to enter edit mode")
			}
		})

		t.Run("own message", func(t *testing.T) {
			m := newTestModelWithTransport(&mockTransport{})
			ml := m.messagesList
			ml.chatView.SetSelectedChannel(&discord.Channel{ID: 99, Type: discord.DirectMessage})
			ml.setMessages([]discord.Message{{ID: 11, ChannelID: 99, Content: "mine", Author: discord.User{ID: 1, Username: "me"}}})
			ml.SetCursor(0)
			ml.Update(tcell.NewEventKey(tcell.KeyRune, "e", tcell.ModNone))
			if !ml.chatView.messageInput.edit {
				t.Fatal("expected own message to enter edit mode")
			}
		})
	})

	t.Run("delete confirm opens modal", func(t *testing.T) {
		m := newTestModelWithTransport(&mockTransport{})
		ml := m.messagesList
		ml.chatView.SetSelectedChannel(&discord.Channel{ID: 99, Type: discord.DirectMessage})
		ml.setMessages([]discord.Message{{ID: 11, ChannelID: 99, Content: "mine", Author: discord.User{ID: 1, Username: "me"}}})
		ml.SetCursor(1)
		ml.SetCursor(0)
		ml.Update(tcell.NewEventKey(tcell.KeyRune, "d", tcell.ModNone))
		if !m.HasLayer(confirmModalLayerName) {
			t.Fatal("expected confirm modal layer to be visible")
		}
	})
}

func TestMessagesListDeleteReplyAndHelpers(t *testing.T) {
	transport := &mockTransport{}
	m := newTestModelWithTransport(transport)
	ml := m.messagesList
	channel := &discord.Channel{ID: 321, Type: discord.DirectMessage}
	m.SetSelectedChannel(channel)

	t.Run("reply mention toggles allowed mentions", func(t *testing.T) {
		ml.setMessages([]discord.Message{{ID: 21, ChannelID: channel.ID, Author: discord.User{ID: 2, Username: "other"}}})
		ml.SetCursor(0)
		ml.reply(true)
		if ml.chatView.messageInput.sendMessageData.AllowedMentions == nil || ml.chatView.messageInput.sendMessageData.AllowedMentions.RepliedUser == nil {
			t.Fatal("expected allowed mentions to be configured for reply")
		}
		if !*ml.chatView.messageInput.sendMessageData.AllowedMentions.RepliedUser {
			t.Fatal("expected reply mention to ping the replied user")
		}
	})

	t.Run("reply without mention prefers guild nickname in title", func(t *testing.T) {
		guildID := discord.GuildID(999)
		guildChannel := &discord.Channel{ID: 333, GuildID: guildID, Type: discord.GuildText}
		m.SetSelectedChannel(guildChannel)
		m.state.Cabinet.MemberStore.MemberSet(guildID, &discord.Member{
			User: discord.User{ID: 2, Username: "other"},
			Nick: "nickname",
		}, false)

		ml.setMessages([]discord.Message{{ID: 22, ChannelID: guildChannel.ID, GuildID: guildID, Author: discord.User{ID: 2, Username: "other"}}})
		ml.SetCursor(0)
		ml.reply(false)

		if got := ml.chatView.messageInput.Title(); !strings.Contains(got, "nickname") {
			t.Fatalf("expected reply title to prefer member nick, got %q", got)
		}
		if ml.chatView.messageInput.sendMessageData.AllowedMentions == nil || *ml.chatView.messageInput.sendMessageData.AllowedMentions.RepliedUser {
			t.Fatal("expected non-mention reply to keep replied-user ping disabled")
		}
	})

	t.Run("delete without selected channel returns", func(t *testing.T) {
		ml.setMessages([]discord.Message{{ID: 22, ChannelID: channel.ID, Author: discord.User{ID: 1, Username: "me"}}})
		ml.SetCursor(0)
		m.SetSelectedChannel(nil)
		ml.delete()
		m.SetSelectedChannel(channel)
	})

	t.Run("delete in dm succeeds", func(t *testing.T) {
		ml.setMessages([]discord.Message{{ID: 23, ChannelID: channel.ID, Author: discord.User{ID: 1, Username: "me"}}})
		ml.SetCursor(0)
		m.state.Cabinet.MessageStore.MessageSet(&discord.Message{ID: 23, ChannelID: channel.ID}, false)
		ml.delete()
		if transport.method != http.MethodDelete || !strings.Contains(transport.path, "/messages/23") {
			t.Fatalf("expected delete request for message 23, got method=%q path=%q", transport.method, transport.path)
		}
	})

	t.Run("delete denied without guild permission", func(t *testing.T) {
		guildChannel := &discord.Channel{ID: 654, GuildID: 777, Type: discord.GuildText}
		m.SetSelectedChannel(guildChannel)
		m.state.Cabinet.ChannelStore.ChannelSet(guildChannel, false)
		ml.setMessages([]discord.Message{{ID: 24, ChannelID: guildChannel.ID, GuildID: guildChannel.GuildID, Author: discord.User{ID: 2, Username: "other"}}})
		ml.SetCursor(0)

		transport.method = ""
		transport.path = ""
		ml.delete()
		if strings.Contains(transport.path, "/messages/24") {
			t.Fatalf("expected permission-denied delete not to hit delete endpoint, got method=%q path=%q", transport.method, transport.path)
		}
	})

	t.Run("request guild members hits gateway path", func(t *testing.T) {
		ml.requestGuildMembers(discord.GuildID(777), []discord.Message{
			{Author: discord.User{ID: 30}},
			{Author: discord.User{ID: 30}},
			{Author: discord.User{ID: 31}, WebhookID: discord.WebhookID(55)},
		})
	})

	t.Run("short and full help with selection", func(t *testing.T) {
		m.SetSelectedChannel(channel)
		ml.setMessages([]discord.Message{{ID: 25, ChannelID: channel.ID, Attachments: []discord.Attachment{{Filename: "file.txt", URL: "https://example.com/file.txt"}}, Author: discord.User{ID: 2, Username: "other"}}})
		ml.SetCursor(0)
		if got := len(ml.ShortHelp()); got == 0 {
			t.Fatal("expected short help entries")
		}
		if got := len(ml.FullHelp()); got == 0 {
			t.Fatal("expected full help groups")
		}
	})
}

func TestMessagesListOpenAttachmentFailures(t *testing.T) {
	ml := newTestModel().messagesList

	oldHTTPGetAttachment := httpGetAttachment
	oldMkdirAllAttachment := mkdirAllAttachment
	oldCreateAttachmentFile := createAttachmentFile
	oldCopyAttachmentData := copyAttachmentData
	oldOpenStart := openStart
	t.Cleanup(func() {
		httpGetAttachment = oldHTTPGetAttachment
		mkdirAllAttachment = oldMkdirAllAttachment
		createAttachmentFile = oldCreateAttachmentFile
		copyAttachmentData = oldCopyAttachmentData
		openStart = oldOpenStart
	})

	t.Run("http failure", func(t *testing.T) {
		httpGetAttachment = func(string) (*http.Response, error) { return nil, errors.New("http fail") }
		ml.openAttachment(discord.Attachment{Filename: "file.png", URL: "https://example.com/file.png"})
	})

	t.Run("mkdir failure", func(t *testing.T) {
		httpGetAttachment = func(string) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("x")), Header: make(http.Header)}, nil
		}
		mkdirAllAttachment = func(string, os.FileMode) error { return errors.New("mkdir fail") }
		ml.openAttachment(discord.Attachment{Filename: "file.png", URL: "https://example.com/file.png"})
	})

	t.Run("create failure", func(t *testing.T) {
		mkdirAllAttachment = func(string, os.FileMode) error { return nil }
		createAttachmentFile = func(string) (*os.File, error) { return nil, errors.New("create fail") }
		ml.openAttachment(discord.Attachment{Filename: "file.png", URL: "https://example.com/file.png"})
	})

	t.Run("copy failure", func(t *testing.T) {
		createAttachmentFile = func(string) (*os.File, error) { return os.CreateTemp(t.TempDir(), "file-*") }
		copyAttachmentData = func(io.Writer, io.Reader) (int64, error) { return 0, errors.New("copy fail") }
		ml.openAttachment(discord.Attachment{Filename: "file.png", URL: "https://example.com/file.png"})
	})

	t.Run("open failure", func(t *testing.T) {
		copyAttachmentData = func(dst io.Writer, src io.Reader) (int64, error) { return io.Copy(dst, src) }
		openStart = func(string) error { return errors.New("open fail") }
		ml.openAttachment(discord.Attachment{Filename: "file.png", URL: "https://example.com/file.png"})
	})

	t.Run("url open failure", func(t *testing.T) {
		openStart = func(string) error { return errors.New("open fail") }
		ml.openURL("https://example.com")
	})
}

func TestMessagesListWaitForChunkAndShowAttachments(t *testing.T) {
	m := newTestModel()
	ml := m.messagesList

	t.Run("waitForChunkEvent when not fetching", func(t *testing.T) {
		if got := ml.waitForChunkEvent(); got != 0 {
			t.Fatalf("expected zero chunk count when idle, got %d", got)
		}
	})

	t.Run("showAttachmentsList opens overlay", func(t *testing.T) {
		ml.showAttachmentsList([]string{"https://one.example"}, []discord.Attachment{{Filename: "file.txt", URL: "https://two.example", ContentType: "text/plain"}})
		if !m.HasLayer(attachmentsPickerLayerName) {
			t.Fatal("expected attachments overlay to be visible")
		}
	})
}

func TestMessagesListDeleteAllowedMentionsReset(t *testing.T) {
	m := newTestModel()
	ml := m.messagesList
	ml.chatView.messageInput.sendMessageData = &api.SendMessageData{}
	ml.setMessages([]discord.Message{{ID: 40, Author: discord.User{ID: 2, Username: "other"}}})
	ml.SetCursor(0)
	ml.reply(false)
	if ml.chatView.messageInput.sendMessageData.AllowedMentions == nil {
		t.Fatal("expected reply to install allowed mentions")
	}
}

func TestMessagesListConfirmDeleteFlow(t *testing.T) {
	transport := &mockTransport{}
	m := newTestModelWithTransport(transport)
	ml := m.messagesList
	channel := &discord.Channel{ID: 500, Type: discord.DirectMessage}
	m.SetSelectedChannel(channel)
	ml.setMessages([]discord.Message{{ID: 88, ChannelID: channel.ID, Author: discord.User{ID: 1, Username: "me"}}})
	ml.SetCursor(0)
	m.state.Cabinet.MessageStore.MessageSet(&discord.Message{ID: 88, ChannelID: channel.ID}, false)

	ml.confirmDelete()
	if !m.HasLayer(confirmModalLayerName) {
		t.Fatal("expected confirm modal to be visible")
	}

	m.Update(&tview.ModalDoneMsg{ButtonLabel: "Yes"})
	if !strings.Contains(transport.path, "/messages/88") {
		t.Fatalf("expected confirm delete to hit delete endpoint, got %q", transport.path)
	}
}

func TestMessagesListUpdateMoreKeybinds(t *testing.T) {
	m := newTestModelWithTransport(&mockTransport{})
	ml := m.messagesList
	channel := &discord.Channel{ID: 600, Type: discord.DirectMessage}
	m.SetSelectedChannel(channel)
	m.state.Cabinet.MessageStore.MessageSet(&discord.Message{ID: 91, ChannelID: channel.ID}, false)
	ml.setMessages([]discord.Message{
		{ID: 90, ChannelID: channel.ID, Content: "https://example.com", Author: discord.User{ID: 2, Username: "other"}},
		{ID: 91, ChannelID: channel.ID, Content: "mine", Author: discord.User{ID: 1, Username: "me"}},
	})

	oldOpenStart := openStart
	t.Cleanup(func() { openStart = oldOpenStart })
	openStart = func(string) error { return nil }

	t.Run("navigation keys redraw", func(t *testing.T) {
		keys := []*tcell.EventKey{
			tcell.NewEventKey(tcell.KeyRune, "k", tcell.ModNone),
			tcell.NewEventKey(tcell.KeyRune, "j", tcell.ModNone),
			tcell.NewEventKey(tcell.KeyRune, "g", tcell.ModNone),
			tcell.NewEventKey(tcell.KeyRune, "G", tcell.ModNone),
			tcell.NewEventKey(tcell.KeyRune, "s", tcell.ModNone),
			tcell.NewEventKey(tcell.KeyHome, "", tcell.ModNone),
			tcell.NewEventKey(tcell.KeyEnd, "", tcell.ModNone),
		}
		for _, key := range keys {
			ml.Update(key)
		}
	})

	t.Run("enter key opens selection", func(t *testing.T) {
		ml.SetCursor(0)
		ml.Update(tcell.NewEventKey(tcell.KeyEnter, "", tcell.ModNone))
	})

	t.Run("reply mention key redraws", func(t *testing.T) {
		ml.SetCursor(0)
		ml.Update(tcell.NewEventKey(tcell.KeyRune, "r", tcell.ModNone))
	})

	t.Run("force delete key redraws", func(t *testing.T) {
		ml.SetCursor(1)
		executeCommand(requireCommand(t, ml.Update(tcell.NewEventKey(tcell.KeyRune, "D", tcell.ModNone))))
	})

	t.Run("non click mouse falls through", func(t *testing.T) {
		event := &tview.MouseMsg{
			EventMouse: *tcell.NewEventMouse(1, 1, tcell.ButtonPrimary, tcell.ModNone),
			Action:     tview.MouseRightClick,
		}
		ml.Update(event)
	})
}

func TestMessagesListFetchAndJumpBranches(t *testing.T) {
	t.Run("prepend older messages without selection returns zero", func(t *testing.T) {
		ml := newTestModel().messagesList
		if got := ml.prependOlderMessages(); got != 0 {
			t.Fatalf("expected no prepend without selected channel, got %d", got)
		}
	})

	t.Run("prepend older messages handles transport error", func(t *testing.T) {
		transport := &mockTransport{
			roundTrip: func(req *http.Request) (*http.Response, error) {
				if req.Method == http.MethodGet && strings.Contains(req.URL.Path, "/messages") {
					return nil, errors.New("fetch fail")
				}
				return (&mockTransport{}).RoundTrip(req)
			},
		}
		m := newTestModelWithTransport(transport)
		ml := m.messagesList
		m.SetSelectedChannel(&discord.Channel{ID: 55, Type: discord.DirectMessage})
		ml.messages = []discord.Message{{ID: 200, ChannelID: 55}}
		if got := ml.prependOlderMessages(); got != 0 {
			t.Fatalf("expected failed prepend to return zero, got %d", got)
		}
	})

	t.Run("prepend older messages handles empty fetch", func(t *testing.T) {
		m := newTestModelWithTransport(&mockTransport{messages: nil})
		ml := m.messagesList
		m.SetSelectedChannel(&discord.Channel{ID: 56, Type: discord.DirectMessage})
		ml.messages = []discord.Message{{ID: 201, ChannelID: 56}}
		if got := ml.prependOlderMessages(); got != 0 {
			t.Fatalf("expected empty prepend to return zero, got %d", got)
		}
	})

	t.Run("prepend older messages prepends fetched window", func(t *testing.T) {
		transport := &mockTransport{messages: []discord.Message{
			{ID: 110, ChannelID: 57, Author: discord.User{ID: 1}},
			{ID: 120, ChannelID: 57, Author: discord.User{ID: 1}},
		}}
		m := newTestModelWithTransport(transport)
		ml := m.messagesList
		channel := &discord.Channel{ID: 57, Type: discord.DirectMessage}
		m.SetSelectedChannel(channel)
		ml.messages = []discord.Message{{ID: 300, ChannelID: 57, Author: discord.User{ID: 1}}}

		if got := ml.prependOlderMessages(); got != 2 {
			t.Fatalf("expected two prepended messages, got %d", got)
		}
		if len(ml.messages) != 3 {
			t.Fatalf("expected three messages after prepend, got %d", len(ml.messages))
		}
		if ml.messages[0].ID != 120 || ml.messages[1].ID != 110 || ml.messages[2].ID != 300 {
			t.Fatalf("unexpected prepended order: %+v", []discord.MessageID{ml.messages[0].ID, ml.messages[1].ID, ml.messages[2].ID})
		}
	})

	t.Run("jump to message validates ids", func(t *testing.T) {
		ml := newTestModel().messagesList
		if err := ml.jumpToMessage(discord.Channel{}, 0); err == nil {
			t.Fatal("expected invalid ids to fail")
		}
	})

	t.Run("jump to message surfaces fetch error", func(t *testing.T) {
		transport := &mockTransport{
			roundTrip: func(req *http.Request) (*http.Response, error) {
				if req.Method == http.MethodGet && strings.Contains(req.URL.Path, "/messages") {
					return nil, errors.New("around fail")
				}
				return (&mockTransport{}).RoundTrip(req)
			},
		}
		m := newTestModelWithTransport(transport)
		ml := m.messagesList
		err := ml.jumpToMessage(discord.Channel{ID: 58, Type: discord.DirectMessage}, 10)
		if err == nil || !strings.Contains(err.Error(), "around fail") {
			t.Fatalf("expected fetch error, got %v", err)
		}
	})

	t.Run("jump to message handles not found and missing target", func(t *testing.T) {
		missingWindow := newTestModelWithTransport(&mockTransport{messages: nil})
		if err := missingWindow.messagesList.jumpToMessage(discord.Channel{ID: 59, Type: discord.DirectMessage}, 99); err == nil || !strings.Contains(err.Error(), "not found") {
			t.Fatalf("expected message-not-found error, got %v", err)
		}

		transport := &mockTransport{messages: []discord.Message{
			{ID: 1, ChannelID: 60, Author: discord.User{ID: 1}},
			{ID: 3, ChannelID: 60, Author: discord.User{ID: 1}},
		}}
		m := newTestModelWithTransport(transport)
		err := m.messagesList.jumpToMessage(discord.Channel{ID: 60, Type: discord.DirectMessage}, 2)
		if err == nil || !strings.Contains(err.Error(), "not present") {
			t.Fatalf("expected missing-target error, got %v", err)
		}
	})

	t.Run("jump to message loads window and selects target", func(t *testing.T) {
		transport := &mockTransport{messages: []discord.Message{
			{ID: 401, ChannelID: 61, Content: "older", Author: discord.User{ID: 1}},
			{ID: 402, ChannelID: 61, Content: "target", Author: discord.User{ID: 1}},
			{ID: 403, ChannelID: 61, Content: "newer", Author: discord.User{ID: 1}},
		}}
		m := newTestModelWithTransport(transport)
		ml := m.messagesList
		channel := discord.Channel{ID: 61, Type: discord.DirectMessage, Topic: "search target"}
		m.addTyper(999)

		if err := ml.jumpToMessage(channel, 402); err != nil {
			t.Fatalf("expected jump to succeed, got %v", err)
		}
		if m.SelectedChannel() == nil || m.SelectedChannel().ID != channel.ID {
			t.Fatal("expected jump to set selected channel")
		}
		if ml.Cursor() == -1 || ml.messages[ml.Cursor()].ID != 402 {
			t.Fatalf("expected cursor on target message, got cursor=%d", ml.Cursor())
		}
		if got := ml.Title(); !strings.Contains(got, "search target") {
			t.Fatalf("expected title to include topic, got %q", got)
		}
		if len(m.typers) != 0 {
			t.Fatal("expected jump to clear typers")
		}
	})
}

func TestMessagesListHelperAndFailureBranches(t *testing.T) {
	t.Run("link display text handles invalid and root URLs", func(t *testing.T) {
		if got := linkDisplayText("not a url"); got != "not a url" {
			t.Fatalf("expected invalid URL to pass through, got %q", got)
		}
		if got := linkDisplayText("https://example.com/"); got != "example.com" {
			t.Fatalf("expected root path to collapse to host, got %q", got)
		}
	})

	t.Run("wrapStyledLine handles empty and zero-width input", func(t *testing.T) {
		line := tview.Line{{Text: "abc", Style: tcell.StyleDefault.Bold(true)}}
		if got := wrapStyledLine(line, 0); len(got) != 1 || joinedLinesText(got) != "abc" {
			t.Fatalf("expected zero-width wrap to return original line, got %#v", got)
		}
		if got := wrapStyledLine(nil, 4); len(got) != 1 || len(got[0]) != 0 {
			t.Fatalf("expected empty line to stay empty, got %#v", got)
		}
		wide := tview.Line{{Text: "a🙂b", Style: tcell.StyleDefault}}
		if got := wrapStyledLine(wide, 2); len(got) < 2 {
			t.Fatalf("expected wide graphemes to wrap across lines, got %#v", got)
		}
	})

	t.Run("messageURLs deduplicates embeds and trims blanks", func(t *testing.T) {
		msg := discord.Message{
			Content: " https://dup.example https://dup.example ",
			Embeds: []discord.Embed{
				{URL: "https://dup.example"},
				{Image: &discord.EmbedImage{URL: "https://img.example/1.png"}},
				{Video: &discord.EmbedVideo{URL: "https://video.example/1.mp4"}},
				{Image: &discord.EmbedImage{URL: "https://img.example/1.png"}},
			},
		}
		got := messageURLs(msg)
		want := []string{"https://dup.example", "https://img.example/1.png", "https://video.example/1.mp4"}
		if len(got) != len(want) {
			t.Fatalf("expected %d URLs, got %d: %#v", len(want), len(got), got)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("url %d = %q, want %q", i, got[i], want[i])
			}
		}
	})

	t.Run("drawEmbeds skips duplicate media-only embeds already in content", func(t *testing.T) {
		m := newTestModel()
		ml := m.messagesList
		ml.SetRect(0, 0, 80, 20)
		ml.cfg.InlineImages.Enabled = false
		builder := tview.NewLineBuilder()
		msg := discord.Message{
			Content: "https://img.example/x.png https://video.example/x.mp4",
			Embeds: []discord.Embed{{
				Image: &discord.EmbedImage{URL: "https://img.example/x.png"},
				Video: &discord.EmbedVideo{URL: "https://video.example/x.mp4"},
			}},
		}
		ml.drawEmbeds(builder, msg, tcell.StyleDefault)
		if got := joinedLinesText(builder.Finish()); got != "" {
			t.Fatalf("expected duplicate media-only embed to render nothing, got %q", got)
		}
	})

	t.Run("delete and guild-member-request failure paths", func(t *testing.T) {
		oldDeleteMessageFunc := deleteMessageFunc
		oldMessageRemoveFunc := messageRemoveFunc
		oldSendGatewayFunc := sendGatewayFunc
		t.Cleanup(func() {
			deleteMessageFunc = oldDeleteMessageFunc
			messageRemoveFunc = oldMessageRemoveFunc
			sendGatewayFunc = oldSendGatewayFunc
		})

		m := newTestModel()
		ml := m.messagesList
		channel := &discord.Channel{ID: 700, GuildID: 701, Type: discord.GuildText}
		m.SetSelectedChannel(channel)
		m.state.Cabinet.ChannelStore.ChannelSet(channel, false)
		m.state.Cabinet.MeStore.MyselfSet(discord.User{ID: 1, Username: "me"}, false)
		ml.setMessages([]discord.Message{{ID: 702, ChannelID: channel.ID, GuildID: channel.GuildID, Author: discord.User{ID: 1, Username: "me"}}})
		ml.SetCursor(0)

		deleteCalls := 0
		deleteMessageFunc = func(*state.State, discord.ChannelID, discord.MessageID, api.AuditLogReason) error {
			deleteCalls++
			return errors.New("delete fail")
		}
		ml.delete()
		if deleteCalls != 1 {
			t.Fatalf("expected deleteMessageFunc to be called once, got %d", deleteCalls)
		}

		deleteMessageFunc = func(*state.State, discord.ChannelID, discord.MessageID, api.AuditLogReason) error { return nil }
		removeCalls := 0
		messageRemoveFunc = func(*state.State, discord.ChannelID, discord.MessageID) error {
			removeCalls++
			return errors.New("remove fail")
		}
		ml.delete()
		if removeCalls != 1 {
			t.Fatalf("expected messageRemoveFunc to be called once, got %d", removeCalls)
		}

		member := &discord.Member{User: discord.User{ID: 9, Username: "cached"}}
		m.state.Cabinet.MemberStore.MemberSet(channel.GuildID, member, false)
		ml.requestGuildMembers(channel.GuildID, []discord.Message{{Author: member.User}})

		sendGatewayCalls := 0
		sendGatewayFunc = func(*state.State, context.Context, ws.Event) error {
			sendGatewayCalls++
			return errors.New("gateway fail")
		}
		ml.requestGuildMembers(channel.GuildID, []discord.Message{{Author: discord.User{ID: 10, Username: "missing"}}})
		if sendGatewayCalls != 1 {
			t.Fatalf("expected sendGatewayFunc to be called once, got %d", sendGatewayCalls)
		}
	})
}

func TestMessagesListAdditionalCoverageBranches(t *testing.T) {
	t.Run("after draw and row helpers early returns", func(t *testing.T) {
		m := newTestModel()
		ml := m.messagesList

		ml.cfg.InlineImages.Enabled = false
		ml.useKitty = true
		ml.AfterDraw(&completeMockScreen{})

		ml.cfg.InlineImages.Enabled = true
		ml.useKitty = false
		ml.AfterDraw(&completeMockScreen{})

		msg := discord.Message{
			ID: 1,
			Attachments: []discord.Attachment{{
				Filename:    "img.png",
				URL:         "https://example.com/img.png",
				ContentType: "image/png",
			}},
		}
		ml.setMessages([]discord.Message{msg})
		ml.useKitty = true
		ml.cellW = 8
		ml.cellH = 16
		item1 := ml.buildImageItem(messagesListRow{kind: messagesListRowImage, messageIndex: 0, attachmentIndex: 0})
		item2 := ml.buildImageItem(messagesListRow{kind: messagesListRowImage, messageIndex: 0, attachmentIndex: 0})
		if item1 != item2 {
			t.Fatal("expected buildImageItem cache hit to reuse the same image item")
		}
		if item1.cellW != 8 || item1.cellH != 16 {
			t.Fatalf("expected kitty image item to inherit cell size, got %dx%d", item1.cellW, item1.cellH)
		}

		ml.imageItemByKey = map[string]*imageItem{"img": item1}
		ml.currentUseKitty()
		ml.setKittySuspended(&completeMockScreen{}, true)
		ml.setKittySuspended(&completeMockScreen{}, false)
	})

	t.Run("navigation mouse and selection guard branches", func(t *testing.T) {
		m := newTestModel()
		ml := m.messagesList

		ml.selectTop()
		ml.selectBottom()
		ml.selectReply()

		ml.SetRect(0, 0, 5, 2)
		ml.lastScreen = &mockEmoteScreen{cells: map[string]string{}}
		event := &tview.MouseMsg{
			EventMouse: *tcell.NewEventMouse(9, 9, tcell.ButtonPrimary, tcell.ModNone),
			Action:     tview.MouseLeftClick,
		}
		if cmd := ml.Update(event); cmd != nil {
			t.Fatalf("expected mouse click outside list to fall through, got %T", cmd)
		}

		ml.setMessages([]discord.Message{{ID: 10, ChannelID: 20, Author: discord.User{ID: 2, Username: "other"}}})
		ml.Update(tcell.NewEventKey(tcell.KeyRune, "K", tcell.ModNone))
		ml.Update(tcell.NewEventKey(tcell.KeyRune, "J", tcell.ModNone))
	})

	t.Run("reply selection and help gating branches", func(t *testing.T) {
		m := newTestModel()
		ml := m.messagesList
		channel := &discord.Channel{ID: 30, GuildID: 31, Type: discord.GuildText}
		m.SetSelectedChannel(channel)
		m.state.Cabinet.ChannelStore.ChannelSet(channel, false)
		setPermissionsForUser(m, channel.GuildID, channel, discord.User{ID: 1, Username: "me"}, discord.PermissionViewChannel|discord.PermissionManageMessages)

		referenced := &discord.Message{ID: 40}
		ml.setMessages([]discord.Message{{
			ID:                41,
			ChannelID:         channel.ID,
			GuildID:           channel.GuildID,
			Content:           "https://one.example",
			ReferencedMessage: referenced,
			Attachments:       []discord.Attachment{{Filename: "a.txt", URL: "https://two.example"}},
			Author:            discord.User{ID: 2, Username: "other"},
		}})
		ml.SetCursor(0)
		full := ml.FullHelp()
		if !containsKeybindGroup(full, ml.cfg.Keybinds.MessagesList.SelectReply.Keybind) {
			t.Fatal("expected full help to include select-reply when a reply target exists")
		}
		if !containsKeybindGroup(full, ml.cfg.Keybinds.MessagesList.Open.Keybind) {
			t.Fatal("expected full help to include open when URLs or attachments exist")
		}
		if !containsKeybindGroup(full, ml.cfg.Keybinds.MessagesList.Delete.Keybind) {
			t.Fatal("expected full help to include delete when manage-messages is allowed")
		}
	})

	t.Run("requestGuildMembers success wait path and yankContent failure", func(t *testing.T) {
		oldSendGatewayFunc := sendGatewayFunc
		oldClipboardWrite := clipboardWrite
		t.Cleanup(func() {
			sendGatewayFunc = oldSendGatewayFunc
			clipboardWrite = oldClipboardWrite
		})

		m := newTestModel()
		ml := m.messagesList
		guildID := discord.GuildID(50)
		called := make(chan struct{}, 1)
		sendGatewayFunc = func(*state.State, context.Context, ws.Event) error {
			go func() {
				time.Sleep(10 * time.Millisecond)
				ml.setFetchingChunk(false, 2)
			}()
			called <- struct{}{}
			return nil
		}

		ml.requestGuildMembers(guildID, []discord.Message{{Author: discord.User{ID: 60, Username: "missing"}}})
		select {
		case <-called:
		case <-time.After(300 * time.Millisecond):
			t.Fatal("expected requestGuildMembers to send a gateway request")
		}

		ml.setMessages([]discord.Message{{ID: 61, Content: "copy me", Author: discord.User{ID: 2, Username: "other"}}})
		ml.SetCursor(0)
		clipboardWrite = func(clipkg.Format, []byte) error { return errors.New("clipboard fail") }
		ml.yankContent()
		time.Sleep(10 * time.Millisecond)
	})
}

func TestMessagesListAdditionalHelperCoverage(t *testing.T) {
	t.Run("writeMessage blocked branch and drawAuthor color branch", func(t *testing.T) {
		m := newTestModel()
		ml := m.messagesList

		m.cfg.HideBlockedUsers = true
		m.state.RelationshipState = &relationship.State{}
		relationshipsField := reflect.ValueOf(m.state.RelationshipState).Elem().FieldByName("relationships")
		reflect.NewAt(relationshipsField.Type(), unsafe.Pointer(relationshipsField.UnsafeAddr())).Elem().Set(
			reflect.ValueOf(map[discord.UserID]discord.Relationship{
				55: {UserID: 55, Type: discord.BlockedRelationship},
			}),
		)

		blockedBuilder := tview.NewLineBuilder()
		ml.writeMessage(blockedBuilder, discord.Message{Author: discord.User{ID: 55, Username: "blocked"}}, tcell.StyleDefault, false)
		if got := joinedLinesText(blockedBuilder.Finish()); !strings.Contains(got, "Blocked message") {
			t.Fatalf("expected blocked-user text, got %q", got)
		}

		guildID := discord.GuildID(90)
		roleID := discord.RoleID(91)
		m.state.Cabinet.RoleStore.RoleSet(guildID, &discord.Role{ID: roleID, Color: 0x123456}, false)
		m.state.Cabinet.MemberStore.MemberSet(guildID, &discord.Member{
			User:    discord.User{ID: 56, Username: "colored"},
			Nick:    "nick",
			RoleIDs: []discord.RoleID{roleID},
		}, false)

		authorBuilder := tview.NewLineBuilder()
		ml.drawAuthor(authorBuilder, discord.Message{
			Author:  discord.User{ID: 56, Username: "colored"},
			GuildID: guildID,
		}, tcell.StyleDefault)
		line := authorBuilder.Finish()[0]
		if len(line) == 0 || line[0].Style.GetForeground() != tcell.NewHexColor(0x123456) {
			t.Fatalf("expected author style to use member color, got %#v", line)
		}
	})

	t.Run("animation and after-draw helper paths", func(t *testing.T) {
		ml := &messagesList{}
		ml.scheduleAnimatedRedraw(0)
		ml.stopAnimatedRedraw()

		m := newTestModel()
		ml = m.messagesList
		drawn := make(chan struct{}, 1)
		ml.queueDraw = func() { drawn <- struct{}{} }
		ml.scheduleAnimatedRedraw(-time.Millisecond)
		select {
		case <-drawn:
		case <-time.After(300 * time.Millisecond):
			t.Fatal("expected scheduled animated redraw to fire through queueDraw")
		}
		ml.stopAnimatedRedraw()

		tty := &mockTty{}
		screen := &screenWithTty{tty: tty}
		ml.cfg.InlineImages.Enabled = true
		ml.useKitty = true
		ml.emoteItemByKey = map[string]*imageItem{
			"emote": {
				kittyID:          12,
				pendingPlace:     true,
				kittyPayload:     "AAAA",
				kittyCols:        1,
				kittyRows:        1,
				kittyVisibleRows: 1,
				kittyCropH:       1,
				cellW:            1,
				cellH:            1,
			},
		}
		ml.AfterDraw(screen)
		if !strings.Contains(tty.String(), "a=p,i=12") {
			t.Fatalf("expected emote kitty placement command, got %q", tty.String())
		}
	})

	t.Run("scanAndDrawEmotes initializes kitty cell dimensions", func(t *testing.T) {
		cfg, _ := config.Load("")
		ml := newMessagesList(cfg, nil)
		ml.cfg.InlineImages.Enabled = true
		ml.useKitty = true
		ml.cellW = 7
		ml.cellH = 9
		ml.SetRect(0, 0, 100, 100)
		screen := &mockEmoteScreen{cells: map[string]string{
			"10,10": "https://cdn.discordapp.com/emojis/123.png",
			"11,10": "https://cdn.discordapp.com/emojis/123.png",
		}}

		ml.scanAndDrawEmotes(screen)

		if len(ml.emoteItemByKey) != 1 {
			t.Fatalf("expected one emote item, got %d", len(ml.emoteItemByKey))
		}
		for _, item := range ml.emoteItemByKey {
			if item.cellW != 7 || item.cellH != 9 {
				t.Fatalf("expected emote item to inherit kitty cell dimensions, got %dx%d", item.cellW, item.cellH)
			}
		}
	})
}

func TestMessagesListFinalEdgeCoverage(t *testing.T) {
	t.Run("after draw no tty and suspension early returns", func(t *testing.T) {
		m := newTestModel()
		ml := m.messagesList
		ml.cfg.InlineImages.Enabled = true
		ml.useKitty = true
		ml.AfterDraw(&completeMockScreen{})
		ml.useKitty = false
		ml.setKittySuspended(&completeMockScreen{}, true)
	})

	t.Run("queueAnimatedDraw app path and wrapStyledLine degenerate cases", func(t *testing.T) {
		m := newTestModel()
		ml := m.messagesList
		ml.queueDraw = nil
		ml.queueAnimatedDraw()

		if got := wrapStyledLine(tview.Line{{Text: "", Style: tcell.StyleDefault}}, 1); len(got) != 1 || len(got[0]) != 0 {
			t.Fatalf("expected empty segment wrap to return one empty line, got %#v", got)
		}
		if got := messageURLs(discord.Message{Content: "   ", Embeds: []discord.Embed{{URL: " "}}}); len(got) != 0 {
			t.Fatalf("expected blank URLs to be filtered, got %#v", got)
		}
	})

	t.Run("selection empty guards guild prepend and jump member fetches", func(t *testing.T) {
		oldSendGatewayFunc := sendGatewayFunc
		t.Cleanup(func() { sendGatewayFunc = oldSendGatewayFunc })

		m := newTestModelWithTransport(&mockTransport{
			messages: []discord.Message{{ID: 71, ChannelID: 70, GuildID: 80, Author: discord.User{ID: 2, Username: "other"}}},
		})
		ml := m.messagesList
		ml.selectUp()
		ml.selectDown()

		guildChannel := &discord.Channel{ID: 70, GuildID: 80, Type: discord.GuildText}
		m.SetSelectedChannel(guildChannel)
		ml.setMessages([]discord.Message{{ID: 72, ChannelID: guildChannel.ID, GuildID: guildChannel.GuildID, Author: discord.User{ID: 2, Username: "other"}}})
		ml.SetCursor(0)

		calls := make(chan struct{}, 4)
		sendGatewayFunc = func(*state.State, context.Context, ws.Event) error {
			go func() {
				time.Sleep(10 * time.Millisecond)
				ml.setFetchingChunk(false, 1)
			}()
			calls <- struct{}{}
			return nil
		}

		ml.selectUp()
		if ml.Cursor() == -1 {
			t.Fatal("expected selectUp on first guild message to keep a valid cursor after prepend")
		}

		if err := ml.jumpToMessage(*guildChannel, 71); err != nil {
			t.Fatalf("expected guild jumpToMessage to succeed, got %v", err)
		}

		timeout := time.After(300 * time.Millisecond)
		gotCalls := 0
		for gotCalls < 1 {
			select {
			case <-calls:
				gotCalls++
			case <-timeout:
				t.Fatalf("expected at least 1 gateway member-fetch call, got %d", gotCalls)
			}
		}
	})

	t.Run("full help includes edit for own message", func(t *testing.T) {
		m := newTestModel()
		ml := m.messagesList
		m.SetSelectedChannel(&discord.Channel{ID: 90, Type: discord.DirectMessage})
		ml.setMessages([]discord.Message{{ID: 91, ChannelID: 90, Content: "mine", Author: discord.User{ID: 1, Username: "me"}}})
		ml.SetCursor(0)
		full := ml.FullHelp()
		if !containsKeybindGroup(full, ml.cfg.Keybinds.MessagesList.Edit.Keybind) {
			t.Fatal("expected full help to include edit for own messages")
		}
	})
}

func TestMessagesListRemainingRenderBranches(t *testing.T) {
	t.Run("writeMessage forwarded default message branch", func(t *testing.T) {
		m := newTestModel()
		ml := m.messagesList
		builder := tview.NewLineBuilder()
		msg := discord.Message{
			Type:      discord.DefaultMessage,
			Timestamp: discord.NewTimestamp(time.Unix(0, 0)),
			Author:    discord.User{Username: "author"},
			Reference: &discord.MessageReference{Type: discord.MessageReferenceTypeForward},
			MessageSnapshots: []discord.MessageSnapshot{{
				Message: discord.MessageSnapshotMessage{
					Timestamp: discord.NewTimestamp(time.Unix(0, 0)),
					Content:   "forwarded body",
				},
			}},
		}
		ml.writeMessage(builder, msg, tcell.StyleDefault, false)
		if got := joinedLinesText(builder.Finish()); !strings.Contains(got, "forwarded body") {
			t.Fatalf("expected forwarded message content, got %q", got)
		}
	})

	t.Run("drawContent trims leading blank markdown lines for non-code content", func(t *testing.T) {
		m := newTestModel()
		ml := m.messagesList
		m.cfg.Markdown.Enabled = true
		builder := tview.NewLineBuilder()
		builder.Write("prefix ", tcell.StyleDefault)
		ml.drawContent(builder, discord.Message{Content: "\n\nhello"}, tcell.StyleDefault, false)
		lines := builder.Finish()
		if got := joinedLinesText(lines); !strings.Contains(got, "hello") {
			t.Fatalf("expected markdown content to render body, got %q", got)
		}
	})

	t.Run("trimLeadingContentLines keeps one blank line for non-code and removes all for code blocks", func(t *testing.T) {
		lines := []tview.Line{
			{},
			{},
			{{Text: "body", Style: tcell.StyleDefault}},
		}

		trimmedNonCode := trimLeadingContentLines(append([]tview.Line(nil), lines...), false)
		if len(trimmedNonCode) != 1 || joinedLineText(trimmedNonCode[0]) != "body" {
			t.Fatalf("unexpected non-code trim result: %#v", trimmedNonCode)
		}

		trimmedCode := trimLeadingContentLines(append([]tview.Line(nil), lines...), true)
		if len(trimmedCode) != 1 || joinedLineText(trimmedCode[0]) != "body" {
			t.Fatalf("unexpected code-block trim result: %#v", trimmedCode)
		}
	})

	t.Run("drawEmbeds skips blank embed lines", func(t *testing.T) {
		m := newTestModel()
		ml := m.messagesList
		ml.SetRect(0, 0, 80, 10)
		builder := tview.NewLineBuilder()
		ml.cfg.InlineImages.Enabled = true
		ml.drawEmbeds(builder, discord.Message{
			Embeds: []discord.Embed{{
				Image: &discord.EmbedImage{URL: "https://cdn.discordapp.com/emojis/123456.png"},
			}},
		}, tcell.StyleDefault)
		if got := joinedLinesText(builder.Finish()); got != "" {
			t.Fatalf("expected whitespace-only embed lines to be skipped, got %q", got)
		}
	})
}

func TestMessagesListEmoteCacheCallbackPath(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png.Encode: %v", err)
	}
	pngData := buf.Bytes()
	transport := &mockTransport{}
	m := newTestModelWithTransport(transport)
	ml := m.messagesList
	ml.cfg.InlineImages.Enabled = true
	ml.useKitty = true
	ml.SetRect(0, 0, 100, 100)
	url := "https://cdn.discordapp.com/emojis/4242.png"
	rt := &mockTransport{
		roundTrip: func(req *http.Request) (*http.Response, error) {
			if req.URL.String() != url {
				return nil, errors.New("unexpected url")
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader(pngData)),
				Header:     make(http.Header),
			}, nil
		},
	}
	ml.imageCache = imgpkg.NewCache(&http.Client{
		Transport: rt,
	})

	screen := &mockEmoteScreen{cells: map[string]string{
		"10,10": url,
		"11,10": url,
	}}
	ml.scanAndDrawEmotes(screen)

	deadline := time.Now().Add(300 * time.Millisecond)
	for !ml.imageCache.Requested(url) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !ml.imageCache.Requested(url) {
		t.Fatal("expected emoji cache request to be issued")
	}
}

func TestMessagesListDrawAndRedrawHelpers(t *testing.T) {
	t.Run("draw clears kitty cache on channel switch", func(t *testing.T) {
		m := newTestModel()
		ml := m.messagesList
		ml.cfg.InlineImages.Enabled = true
		ml.useKitty = true
		ml.kittyNeedsFullClear = true
		ml.SetRect(0, 0, 20, 5)
		ml.Model.SetCursor(-1)
		ml.nextKittyID = 9
		ml.imageItemByKey = map[string]*imageItem{
			"img": {kittyID: 1, kittyPlaced: true, kittyUploaded: true, pendingPlace: true},
		}
		ml.emoteItemByKey = map[string]*imageItem{
			"emo": {kittyID: 2, kittyPlaced: true, kittyUploaded: true, pendingPlace: true},
		}

		screen := &ttyScreen{tty: cellSizeTty{}}
		ml.Draw(screen)

		if ml.lastScreen != screen {
			t.Fatal("expected Draw to retain last screen")
		}
		if ml.kittyNeedsFullClear {
			t.Fatal("expected Draw to handle full kitty clear")
		}
		if len(ml.pendingDeletes) != 2 {
			t.Fatalf("expected 2 pending deletes from cleared maps, got %d", len(ml.pendingDeletes))
		}
		if len(ml.imageItemByKey) != 0 || len(ml.emoteItemByKey) != 0 {
			t.Fatal("expected Draw to clear kitty item caches after channel switch")
		}
		if ml.nextKittyID != 1 {
			t.Fatalf("expected kitty ids to reset, got %d", ml.nextKittyID)
		}
	})

	t.Run("draw suspends kitty when popup visible", func(t *testing.T) {
		m := newTestModel()
		ml := m.messagesList
		ml.cfg.InlineImages.Enabled = true
		ml.useKitty = true
		ml.imageItemByKey = map[string]*imageItem{
			"img": {useKitty: true, kittyPlaced: true, pendingPlace: true},
		}
		m.AddLayer(tview.NewBox(), layers.WithName(confirmModalLayerName), layers.WithVisible(true), layers.WithOverlay())

		lockScreen := &lockingTTYScreen{}
		ml.Draw(lockScreen)

		if !ml.kittySuspended {
			t.Fatal("expected popup overlay to suspend kitty drawing")
		}
		item := ml.imageItemByKey["img"]
		if item.pendingPlace || item.kittyPlaced || item.useKitty {
			t.Fatal("expected suspended kitty item to be invalidated")
		}
	})

	t.Run("draw collects offscreen kitty deletes", func(t *testing.T) {
		m := newTestModel()
		ml := m.messagesList
		ml.cfg.InlineImages.Enabled = true
		ml.useKitty = true
		ml.SetRect(0, 0, 20, 5)
		ml.imageItemByKey = map[string]*imageItem{
			"img": {kittyID: 7, kittyPlaced: true, useKitty: true},
		}
		ml.emoteItemByKey = map[string]*imageItem{
			"emo": {kittyID: 8, kittyPlaced: true, useKitty: true},
		}

		ml.Draw(&ttyScreen{tty: cellSizeTty{}})

		if len(ml.pendingDeletes) != 2 {
			t.Fatalf("expected two pending kitty deletes, got %d", len(ml.pendingDeletes))
		}
	})

	t.Run("animated redraw timer branches", func(t *testing.T) {
		ml := &messagesList{}
		done := make(chan struct{}, 1)
		ml.queueDraw = func() { done <- struct{}{} }

		ml.scheduleAnimatedRedraw(0)
		select {
		case <-done:
		case <-time.After(300 * time.Millisecond):
			t.Fatal("expected zero-delay redraw to be normalized and executed")
		}

		ml.scheduleAnimatedRedraw(250 * time.Millisecond)
		firstTimer := ml.animationTimer
		firstDue := ml.animationDue
		ml.scheduleAnimatedRedraw(500 * time.Millisecond)
		if ml.animationTimer != firstTimer || !ml.animationDue.Equal(firstDue) {
			t.Fatal("expected later redraw request not to replace earlier timer")
		}

		ml.scheduleAnimatedRedraw(20 * time.Millisecond)
		if ml.animationTimer == firstTimer {
			t.Fatal("expected earlier redraw request to replace existing timer")
		}
		ml.stopAnimatedRedraw()
		if ml.animationTimer != nil || !ml.animationDue.IsZero() {
			t.Fatal("expected stopAnimatedRedraw to clear timer state")
		}
	})
}

func TestMessagesListReactionPickerAndRenderingBranches(t *testing.T) {
	t.Run("show reaction picker requires message and channel", func(t *testing.T) {
		m := newTestModel()
		ml := m.messagesList

		ml.showReactionPicker()
		if m.HasLayer(reactionPickerLayerName) {
			t.Fatal("expected no picker without a selected message")
		}

		ml.setMessages([]discord.Message{{ID: 70, ChannelID: 700, Author: discord.User{ID: 1}}})
		ml.SetCursor(0)
		ml.showReactionPicker()
		if m.HasLayer(reactionPickerLayerName) {
			t.Fatal("expected no picker without a selected channel")
		}
	})

	t.Run("show reaction picker refreshes visible overlay", func(t *testing.T) {
		m := newTestModel()
		ml := m.messagesList
		channel := &discord.Channel{ID: 701, Type: discord.DirectMessage}
		m.SetSelectedChannel(channel)
		ml.setMessages([]discord.Message{{ID: 71, ChannelID: channel.ID, Author: discord.User{ID: 1}}})
		ml.SetCursor(0)

		ml.showReactionPicker()
		if !m.HasLayer(reactionPickerLayerName) {
			t.Fatal("expected reaction picker layer to open")
		}
		ml.showReactionPicker()
		if m.app.Focused() == m.messagesList {
			t.Fatalf("expected focus to move off the messages list, got %T", m.app.Focused())
		}
	})

	t.Run("rendering helpers cover non-markdown and attachment branches", func(t *testing.T) {
		m := newTestModel()
		ml := m.messagesList
		baseStyle := tcell.StyleDefault

		m.cfg.Markdown.Enabled = false
		if lines, root := ml.renderContentLinesWithMarkdown(discord.Message{Content: "plain"}, baseStyle, false, false); root != nil || len(lines) == 0 {
			t.Fatalf("expected plain rendering without markdown root, got root=%v lines=%d", root, len(lines))
		}
		if lines, root := ml.renderContentLinesWithMarkdown(discord.Message{Content: "**forced**"}, baseStyle, true, false); root == nil || len(lines) == 0 {
			t.Fatalf("expected forced markdown rendering, got root=%v lines=%d", root, len(lines))
		}

		builder := tview.NewLineBuilder()
		builder.Write("prefix ", baseStyle)
		ml.drawContent(builder, discord.Message{Content: "plain text"}, baseStyle, false)
		if len(builder.Finish()) == 0 {
			t.Fatal("expected drawContent to append plain text")
		}

		builder = tview.NewLineBuilder()
		ml.cfg.ShowAttachmentLinks = false
		ml.cfg.InlineImages.Enabled = true
		ml.drawDefaultMessage(builder, discord.Message{
			EditedTimestamp: discord.Timestamp(time.Now()),
			Attachments: []discord.Attachment{
				{Filename: "skip.png", URL: "https://example.com/skip.png", ContentType: "image/png"},
				{Filename: "keep.txt", URL: "https://example.com/keep.txt", ContentType: "text/plain"},
			},
		}, baseStyle, false)
		rendered := builder.Finish()
		flat := ""
		for _, line := range rendered {
			for _, segment := range line {
				flat += segment.Text
			}
			flat += "\n"
		}
		if !strings.Contains(flat, "(edited)") || !strings.Contains(flat, "keep.txt") || strings.Contains(flat, "skip.png") {
			t.Fatalf("unexpected rendered attachment text: %q", flat)
		}
	})
}

func TestMessagesListDrawKittyLifecycleBranches(t *testing.T) {
	t.Run("draw full clear resets kitty caches", func(t *testing.T) {
		m := newTestModel()
		ml := m.messagesList
		ml.cfg.InlineImages.Enabled = true
		ml.useKitty = true
		ml.kittyNeedsFullClear = true
		ml.nextKittyID = 42
		ml.imageItemByKey = map[string]*imageItem{
			"img": {kittyID: 7, kittyPlaced: true, kittyUploaded: true, lockKittyRegion: true, kittyCols: 2, kittyVisibleRows: 1},
		}
		ml.emoteItemByKey = map[string]*imageItem{
			"emo": {kittyID: 8, kittyPlaced: true, kittyUploaded: true, lockKittyRegion: true, kittyCols: 2, kittyVisibleRows: 1},
		}

		screen := &lockingTTYScreen{tty: cellSizeTty{}}
		ml.Draw(screen)

		if ml.kittyNeedsFullClear {
			t.Fatal("expected kitty full clear flag to be consumed")
		}
		if len(ml.pendingDeletes) != 2 {
			t.Fatalf("expected 2 pending deletes, got %d", len(ml.pendingDeletes))
		}
		if len(ml.imageItemByKey) != 0 || len(ml.emoteItemByKey) != 0 {
			t.Fatal("expected kitty caches to be cleared")
		}
		if ml.nextKittyID != 1 {
			t.Fatalf("expected kitty id sequence reset, got %d", ml.nextKittyID)
		}
		if screen.lockCalls == 0 {
			t.Fatal("expected prior kitty regions to be unlocked")
		}
	})

	t.Run("draw with overlay suspends kitty items", func(t *testing.T) {
		m := newTestModel()
		ml := m.messagesList
		ml.cfg.InlineImages.Enabled = true
		ml.useKitty = true
		ml.imageItemByKey = map[string]*imageItem{
			"img": {useKitty: true, kittyPlaced: true, kittyUploaded: true, pendingPlace: true, kittyCols: 2, kittyVisibleRows: 1},
		}
		ml.emoteItemByKey = map[string]*imageItem{
			"emo": {useKitty: true, kittyPlaced: true, kittyUploaded: true, pendingPlace: true, kittyCols: 2, kittyVisibleRows: 1},
		}

		m.openPicker()
		screen := &lockingTTYScreen{tty: cellSizeTty{}}
		ml.Draw(screen)

		if !ml.kittySuspended {
			t.Fatal("expected overlay to suspend kitty mode")
		}
		for _, item := range ml.imageItemByKey {
			if item.useKitty || item.pendingPlace || item.kittyPlaced || item.kittyUploaded {
				t.Fatal("expected suspended image item to be invalidated")
			}
		}
		for _, item := range ml.emoteItemByKey {
			if item.useKitty || item.pendingPlace || item.kittyPlaced || item.kittyUploaded {
				t.Fatal("expected suspended emote item to be invalidated")
			}
		}
	})

	t.Run("draw queues offscreen deletions", func(t *testing.T) {
		m := newTestModel()
		ml := m.messagesList
		ml.cfg.InlineImages.Enabled = true
		ml.useKitty = true
		ml.imageItemByKey = map[string]*imageItem{
			"img": {kittyID: 9, kittyPlaced: true, kittyUploaded: true, kittyCols: 2, kittyVisibleRows: 1},
		}
		ml.emoteItemByKey = map[string]*imageItem{
			"emo": {kittyID: 10, kittyPlaced: true, kittyUploaded: true, kittyCols: 2, kittyVisibleRows: 1},
		}

		screen := &lockingTTYScreen{tty: cellSizeTty{}}
		ml.Draw(screen)

		if len(ml.pendingDeletes) != 2 {
			t.Fatalf("expected two pending kitty deletions, got %v", ml.pendingDeletes)
		}
		if ml.imageItemByKey["img"].kittyPlaced || ml.emoteItemByKey["emo"].kittyPlaced {
			t.Fatal("expected offscreen kitty items to be invalidated")
		}
	})
}

func TestMessagesListRenderAndTitleBranches(t *testing.T) {
	m := newTestModel()
	ml := m.messagesList

	t.Run("setTitle includes topic when present", func(t *testing.T) {
		ml.setTitle(discord.Channel{ID: 50, Name: "general", Topic: "testing", Type: discord.GuildText})
		if got := ml.Title(); !strings.Contains(got, "testing") {
			t.Fatalf("expected topic in title, got %q", got)
		}
	})

	t.Run("renderContentLinesWithMarkdown handles forced and plain rendering", func(t *testing.T) {
		msg := discord.Message{Content: "**bold**"}
		m.cfg.Markdown.Enabled = false

		plain, root := ml.renderContentLinesWithMarkdown(msg, tcell.StyleDefault, false, false)
		if root != nil {
			t.Fatal("expected plain rendering to skip markdown AST")
		}
		if got := joinedLineText(plain[0]); got != "**bold**" {
			t.Fatalf("expected plain text output, got %q", got)
		}

		rendered, root := ml.renderContentLinesWithMarkdown(msg, tcell.StyleDefault, true, false)
		if root == nil {
			t.Fatal("expected forced markdown rendering to build an AST")
		}
		if got := joinedLineText(rendered[0]); !strings.Contains(got, "bold") {
			t.Fatalf("expected rendered markdown to include text, got %q", got)
		}
	})

	t.Run("drawContent handles leading blank code block separation", func(t *testing.T) {
		m.cfg.Markdown.Enabled = true
		builder := tview.NewLineBuilder()
		ml.drawTimestamps(builder, discord.NewTimestamp(time.Now()), tcell.StyleDefault)
		msg := discord.Message{Content: "\n```go\nfmt.Println(\"x\")\n```"}
		ml.drawContent(builder, msg, tcell.StyleDefault, false)
		lines := builder.Finish()
		if len(lines) < 2 {
			t.Fatalf("expected code block to start on a new line, got %d lines", len(lines))
		}
	})

	t.Run("drawDefaultMessage skips inline image filenames", func(t *testing.T) {
		m.cfg.Markdown.Enabled = false
		m.cfg.InlineImages.Enabled = true
		m.cfg.ShowAttachmentLinks = false
		builder := tview.NewLineBuilder()
		ml.drawDefaultMessage(builder, discord.Message{
			Author: discord.User{Username: "user"},
			Attachments: []discord.Attachment{
				{Filename: "visible.txt", URL: "https://example.com/visible.txt", ContentType: "text/plain"},
				{Filename: "hidden.png", URL: "https://example.com/hidden.png", ContentType: "image/png"},
			},
		}, tcell.StyleDefault, false)
		text := joinedLinesText(builder.Finish())
		if !strings.Contains(text, "visible.txt") {
			t.Fatalf("expected non-image attachment to be rendered, got %q", text)
		}
		if strings.Contains(text, "hidden.png") {
			t.Fatalf("expected inline image attachment text to be omitted, got %q", text)
		}
	})

	t.Run("memberForMessage returns nil for webhook and missing member", func(t *testing.T) {
		if got := ml.memberForMessage(discord.Message{
			GuildID:   10,
			WebhookID: 99,
			Author:    discord.User{ID: 1},
		}); got != nil {
			t.Fatal("expected webhook message member lookup to be skipped")
		}

		if got := ml.memberForMessage(discord.Message{
			GuildID: 10,
			Author:  discord.User{ID: 999},
		}); got != nil {
			t.Fatal("expected missing guild member lookup to return nil")
		}
	})
}

func TestMessagesListFetchAndPickerBranches(t *testing.T) {
	t.Run("showReactionPicker handles missing selection and channel", func(t *testing.T) {
		m := newTestModel()
		ml := m.messagesList
		ml.showReactionPicker()
		if m.HasLayer(reactionPickerLayerName) {
			t.Fatal("expected no reaction picker without a selected message")
		}

		ml.setMessages([]discord.Message{{ID: 1, Author: discord.User{ID: 2}}})
		ml.SetCursor(0)
		ml.showReactionPicker()
		if m.HasLayer(reactionPickerLayerName) {
			t.Fatal("expected no reaction picker without a selected channel")
		}
	})

	t.Run("showReactionPicker replaces existing layer", func(t *testing.T) {
		m := newTestModel()
		ml := m.messagesList
		channel := &discord.Channel{ID: 20, Type: discord.DirectMessage}
		m.SetSelectedChannel(channel)
		ml.setMessages([]discord.Message{{ID: 2, ChannelID: channel.ID, Author: discord.User{ID: 2}}})
		ml.SetCursor(0)
		m.AddLayer(tview.NewBox(), layers.WithName(reactionPickerLayerName), layers.WithVisible(true))

		ml.showReactionPicker()

		if !m.HasLayer(reactionPickerLayerName) {
			t.Fatal("expected reaction picker layer to remain visible")
		}
		if m.app.Focused() == m.messagesList {
			t.Fatalf("expected focus to move off the messages list, got %T", m.app.Focused())
		}
	})

	t.Run("prependOlderMessages covers nil, error, empty and success paths", func(t *testing.T) {
		mNil := newTestModelWithTransport(&mockTransport{})
		mlNil := mNil.messagesList
		mlNil.messages = []discord.Message{{ID: 50}}
		if got := mlNil.prependOlderMessages(); got != 0 {
			t.Fatalf("expected no messages without selected channel, got %d", got)
		}

		mErr := newTestModelWithTransport(&mockTransport{
			roundTrip: func(*http.Request) (*http.Response, error) {
				return nil, errors.New("fetch older")
			},
		})
		mlErr := mErr.messagesList
		mErr.SetSelectedChannel(&discord.Channel{ID: 99, Type: discord.DirectMessage})
		mlErr.messages = []discord.Message{{ID: 50}}
		if got := mlErr.prependOlderMessages(); got != 0 {
			t.Fatalf("expected error path to return 0, got %d", got)
		}

		mEmpty := newTestModelWithTransport(&mockTransport{messages: nil})
		mlEmpty := mEmpty.messagesList
		mEmpty.SetSelectedChannel(&discord.Channel{ID: 99, Type: discord.DirectMessage})
		mlEmpty.messages = []discord.Message{{ID: 50}}
		if got := mlEmpty.prependOlderMessages(); got != 0 {
			t.Fatalf("expected empty fetch to return 0, got %d", got)
		}
	})
}

func TestMessagesList_JumpToMessage_More(t *testing.T) {
	t.Run("jumpToMessage covers invalid, error, empty, absent and success paths", func(t *testing.T) {
		m := newTestModelWithTransport(&mockTransport{})
		ml := m.messagesList
		if err := ml.jumpToMessage(discord.Channel{}, 0); err == nil {
			t.Fatal("expected invalid jump request to fail")
		}

		mErr := newTestModelWithTransport(&mockTransport{
			roundTrip: func(*http.Request) (*http.Response, error) {
				return nil, errors.New("around fail")
			},
		})
		if err := mErr.messagesList.jumpToMessage(discord.Channel{ID: 1}, 2); err == nil || !strings.Contains(err.Error(), "around fail") {
			t.Fatalf("expected transport error, got %v", err)
		}

		mEmpty := newTestModelWithTransport(&mockTransport{messages: nil})
		if err := mEmpty.messagesList.jumpToMessage(discord.Channel{ID: 1}, 2); err == nil || err.Error() != "message not found" {
			t.Fatalf("expected message-not-found error, got %v", err)
		}

		mAbsent := newTestModelWithTransport(&mockTransport{messages: []discord.Message{{ID: 3}, {ID: 4}}})
		if err := mAbsent.messagesList.jumpToMessage(discord.Channel{ID: 1}, 2); err == nil || err.Error() != "message not present in loaded window" {
			t.Fatalf("expected absent target error, got %v", err)
		}

		mOK := newTestModelWithTransport(&mockTransport{
			messages: []discord.Message{
				{ID: 10, ChannelID: 1, Content: "before"},
				{ID: 11, ChannelID: 1, Content: "target"},
				{ID: 12, ChannelID: 1, Content: "after"},
			},
		})
		mOK.SetSelectedChannel(&discord.Channel{ID: 777, Type: discord.DirectMessage})
		mOK.typers[2] = time.AfterFunc(time.Hour, func() {})

		channel := discord.Channel{ID: 1, Name: "jump-target", Type: discord.DirectMessage}
		if err := mOK.messagesList.jumpToMessage(channel, 11); err != nil {
			t.Fatalf("expected successful jump, got %v", err)
		}
		if selected := mOK.SelectedChannel(); selected == nil || selected.ID != 1 {
			t.Fatalf("expected selected channel to switch to target, got %#v", selected)
		}
		if mOK.messagesList.Cursor() != 1 {
			t.Fatalf("expected cursor to land on target message, got %d", mOK.messagesList.Cursor())
		}
		if len(mOK.typers) != 0 {
			t.Fatal("expected jump to clear typing state")
		}
		if got := mOK.messagesList.Title(); !strings.Contains(got, "jump-target") {
			t.Fatalf("expected title to update for jumped channel, got %q", got)
		}
	})
}

func TestMessagesListAttachmentAndNavigationUtilityBranches(t *testing.T) {
	t.Run("showAttachmentsList item actions open resources and close", func(t *testing.T) {
		m := newTestModel()
		ml := m.messagesList

		oldOpenStart := openStart
		oldHTTPGetAttachment := httpGetAttachment
		oldMkdirAllAttachment := mkdirAllAttachment
		oldCreateAttachmentFile := createAttachmentFile
		oldCopyAttachmentData := copyAttachmentData
		t.Cleanup(func() {
			openStart = oldOpenStart
			httpGetAttachment = oldHTTPGetAttachment
			mkdirAllAttachment = oldMkdirAllAttachment
			createAttachmentFile = oldCreateAttachmentFile
			copyAttachmentData = oldCopyAttachmentData
		})

		opened := make(chan string, 4)
		openStart = func(target string) error {
			opened <- target
			return nil
		}
		httpGetAttachment = func(string) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("image"))}, nil
		}
		mkdirAllAttachment = func(string, os.FileMode) error { return nil }
		createAttachmentFile = func(string) (*os.File, error) {
			return os.CreateTemp(t.TempDir(), "attachment-*")
		}
		copyAttachmentData = func(dst io.Writer, src io.Reader) (int64, error) {
			return io.Copy(dst, src)
		}

		ml.showAttachmentsList(
			[]string{"https://example.com"},
			[]discord.Attachment{{Filename: "a.png", URL: "https://cdn.example/a.png", ContentType: "image/png"}},
		)
		if !m.HasLayer(attachmentsPickerLayerName) {
			t.Fatal("expected attachments picker layer to open")
		}

		ml.attachmentsPicker.Update(&picker.SelectedMsg{Item: picker.Item{Reference: 0}})
		select {
		case got := <-opened:
			if !strings.Contains(got, "attachments") {
				t.Fatalf("expected image attachment to open cached file, got %q", got)
			}
		case <-time.After(300 * time.Millisecond):
			t.Fatal("timed out waiting for attachment open")
		}
		if m.HasLayer(attachmentsPickerLayerName) {
			t.Fatal("expected picker to close after selection")
		}

		ml.showAttachmentsList(
			[]string{"https://example.com"},
			[]discord.Attachment{{Filename: "a.png", URL: "https://cdn.example/a.png", ContentType: "image/png"}},
		)
		ml.attachmentsPicker.Update(&picker.SelectedMsg{Item: picker.Item{Reference: 1}})
		select {
		case got := <-opened:
			if got != "https://example.com" {
				t.Fatalf("expected URL item to open its URL, got %q", got)
			}
		case <-time.After(300 * time.Millisecond):
			t.Fatal("timed out waiting for URL open")
		}

		ml.showAttachmentsList(nil, nil)
		ml.attachmentsPicker.Update(&picker.SelectedMsg{Item: picker.Item{Reference: "bad"}})
		ml.attachmentsPicker.Update(&picker.SelectedMsg{Item: picker.Item{Reference: 99}})
	})

	t.Run("selectUp and selectDown cover intermediate branches", func(t *testing.T) {
		m := newTestModel()
		ml := m.messagesList
		ml.setMessages([]discord.Message{{ID: 1}, {ID: 2}, {ID: 3}})

		ml.clearSelection()
		ml.selectUp()
		if got := ml.Cursor(); got != 2 {
			t.Fatalf("expected selectUp from no selection to choose last message, got %d", got)
		}

		ml.SetCursor(2)
		ml.selectUp()
		if got := ml.Cursor(); got != 1 {
			t.Fatalf("expected selectUp to move to previous message, got %d", got)
		}

		ml.clearSelection()
		ml.selectDown()
		if got := ml.Cursor(); got != 2 {
			t.Fatalf("expected selectDown from no selection to choose last message, got %d", got)
		}

		ml.SetCursor(1)
		ml.selectDown()
		if got := ml.Cursor(); got != 2 {
			t.Fatalf("expected selectDown to move to next message, got %d", got)
		}
	})

	t.Run("writeMessage covers blocked, reply, pinned and fallback paths", func(t *testing.T) {
		m := newTestModel()
		ml := m.messagesList
		baseStyle := tcell.StyleDefault

		replyBuilder := tview.NewLineBuilder()
		ml.writeMessage(replyBuilder, discord.Message{
			Type:              discord.InlinedReplyMessage,
			ReferencedMessage: &discord.Message{ID: 1, Author: discord.User{Username: "root"}},
		}, baseStyle, false)
		if got := joinedLinesText(replyBuilder.Finish()); !strings.Contains(got, "root") {
			t.Fatalf("expected inline reply text, got %q", got)
		}

		pinnedBuilder := tview.NewLineBuilder()
		ml.writeMessage(pinnedBuilder, discord.Message{
			Type:   discord.ChannelPinnedMessage,
			Author: discord.User{Username: "pinner"},
		}, baseStyle, false)
		if got := joinedLinesText(pinnedBuilder.Finish()); !strings.Contains(got, "pinned") {
			t.Fatalf("expected pinned message text, got %q", got)
		}

		otherBuilder := tview.NewLineBuilder()
		ml.writeMessage(otherBuilder, discord.Message{
			Type:   discord.CallMessage,
			Author: discord.User{Username: "caller"},
		}, baseStyle, false)
		if got := joinedLinesText(otherBuilder.Finish()); !strings.Contains(got, "caller") {
			t.Fatalf("expected fallback message to include author, got %q", got)
		}
	})

	t.Run("updateCellDimensions handles error and zero-sized cells", func(t *testing.T) {
		m := newTestModel()
		ml := m.messagesList

		ml.updateCellDimensions(&ttyScreen{tty: windowSizeErrTty{}})
		if ml.cellW != 0 || ml.cellH != 0 {
			t.Fatalf("expected window size error to leave cell dimensions unchanged, got %dx%d", ml.cellW, ml.cellH)
		}

		ml.updateCellDimensions(&ttyScreen{tty: zeroCellTty{}})
		if ml.cellW != 0 || ml.cellH != 0 {
			t.Fatalf("expected zero-sized cells to be ignored, got %dx%d", ml.cellW, ml.cellH)
		}
	})

	t.Run("cursor and yank helpers tolerate missing selection", func(t *testing.T) {
		m := newTestModel()
		ml := m.messagesList
		if got := ml.Cursor(); got != -1 {
			t.Fatalf("expected empty cursor to be -1, got %d", got)
		}

		ml.setMessages([]discord.Message{{ID: 1}})
		ml.Model.SetCursor(99)
		if got := ml.Cursor(); got != -1 {
			t.Fatalf("expected out-of-range row cursor to return -1, got %d", got)
		}

		ml.messages = []discord.Message{{ID: 1}}
		ml.rows = []messagesListRow{{kind: messagesListRowSeparator}, {kind: messagesListRowMessage, messageIndex: 0}}
		ml.rowsDirty = false
		ml.Model.SetChangedFunc(nil)
		ml.Model.SetCursor(0)
		if got := ml.Cursor(); got != -1 {
			t.Fatalf("expected separator row cursor to return -1, got %d", got)
		}

		ml.clearSelection()
		if cmd := ml.yankMessageID(); cmd != nil {
			t.Fatalf("expected missing selection to return no yank-message-id command, got %T", cmd)
		}
		if cmd := ml.yankURL(); cmd != nil {
			t.Fatalf("expected missing selection to return no yank-url command, got %T", cmd)
		}
	})

	t.Run("yank helpers cover clipboard failure path", func(t *testing.T) {
		m := newTestModel()
		ml := m.messagesList
		ml.setMessages([]discord.Message{{ID: 77, ChannelID: 88, Content: "body"}})
		ml.SetCursor(0)

		oldClipboardWrite := clipboardWrite
		calls := make(chan string, 2)
		clipboardWrite = func(_ clipkg.Format, data []byte) error {
			calls <- string(data)
			return errors.New("clipboard fail")
		}
		t.Cleanup(func() { clipboardWrite = oldClipboardWrite })

		executeCommand(requireCommand(t, ml.yankMessageID()))
		executeCommand(requireCommand(t, ml.yankURL()))

		for i := 0; i < 2; i++ {
			select {
			case <-calls:
			case <-time.After(300 * time.Millisecond):
				t.Fatal("timed out waiting for clipboard failure branch")
			}
		}
	})

	t.Run("delete covers missing selection and API failure", func(t *testing.T) {
		m := newTestModelWithTransport(&mockTransport{})
		ml := m.messagesList
		channel := &discord.Channel{ID: 444, Type: discord.DirectMessage}
		m.SetSelectedChannel(channel)

		ml.delete()

		ml.setMessages([]discord.Message{{ID: 999, ChannelID: channel.ID, Author: discord.User{ID: 1, Username: "me"}}})
		ml.SetCursor(0)
		ml.delete()
	})

	t.Run("reset clears popup layer and state", func(t *testing.T) {
		m := newTestModel()
		ml := m.messagesList
		ml.messages = []discord.Message{{ID: 1}}
		ml.rows = []messagesListRow{{kind: messagesListRowMessage, messageIndex: 0}}
		ml.rowsDirty = true
		ml.itemByID[1] = tview.NewTextView()
		m.AddLayer(tview.NewBox(), layers.WithName(reactionPickerLayerName), layers.WithVisible(true), layers.WithOverlay())

		ml.reset()

		if ml.messages != nil || ml.rows != nil || ml.rowsDirty {
			t.Fatal("expected reset to clear message state")
		}
		if len(ml.itemByID) != 0 {
			t.Fatal("expected reset to clear cached items")
		}
		if !ml.kittyNeedsFullClear {
			t.Fatal("expected reset to request a full kitty clear")
		}
		if m.HasLayer(reactionPickerLayerName) {
			t.Fatal("expected reset to remove reaction picker layer")
		}
		if ml.Title() != "" {
			t.Fatalf("expected reset to clear title, got %q", ml.Title())
		}
	})
}

func joinedLinesText(lines []tview.Line) string {
	var b strings.Builder
	for i, line := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(joinedLineText(line))
	}
	return b.String()
}

func TestMessagesListLoadingAndCursorBranches(t *testing.T) {
	t.Run("prepend older messages branches", func(t *testing.T) {
		t.Run("without selected channel", func(t *testing.T) {
			m := newTestModelWithTransport(&mockTransport{})
			ml := m.messagesList
			ml.messages = []discord.Message{{ID: 10}}
			if got := ml.prependOlderMessages(); got != 0 {
				t.Fatalf("expected no messages without selected channel, got %d", got)
			}
		})

		t.Run("fetch error", func(t *testing.T) {
			transport := &mockTransport{
				roundTrip: func(req *http.Request) (*http.Response, error) {
					if strings.Contains(req.URL.Path, "/messages") {
						return nil, errors.New("boom")
					}
					return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("[]")), Header: make(http.Header)}, nil
				},
			}
			m := newTestModelWithTransport(transport)
			ml := m.messagesList
			m.SetSelectedChannel(&discord.Channel{ID: 123, Type: discord.DirectMessage})
			ml.messages = []discord.Message{{ID: 10}}
			if got := ml.prependOlderMessages(); got != 0 {
				t.Fatalf("expected zero messages on fetch error, got %d", got)
			}
		})

		t.Run("empty window", func(t *testing.T) {
			m := newTestModelWithTransport(&mockTransport{})
			ml := m.messagesList
			m.SetSelectedChannel(&discord.Channel{ID: 123, Type: discord.DirectMessage})
			ml.messages = []discord.Message{{ID: 10}}
			if got := ml.prependOlderMessages(); got != 0 {
				t.Fatalf("expected zero messages for empty window, got %d", got)
			}
		})

		t.Run("successful prepend invalidates overlaps", func(t *testing.T) {
			transport := &mockTransport{
				messages: []discord.Message{
					{ID: 8, ChannelID: 123, Author: discord.User{ID: 2}},
					{ID: 7, ChannelID: 123, Author: discord.User{ID: 3}},
				},
			}
			m := newTestModelWithTransport(transport)
			ml := m.messagesList
			m.SetSelectedChannel(&discord.Channel{ID: 123, Type: discord.DirectMessage})
			ml.messages = []discord.Message{{ID: 10, ChannelID: 123, Author: discord.User{ID: 4}}}
			ml.itemByID[7] = tview.NewTextView()
			ml.itemByID[8] = tview.NewTextView()

			if got := ml.prependOlderMessages(); got != 2 {
				t.Fatalf("expected 2 prepended messages, got %d", got)
			}
			if len(ml.messages) != 3 || ml.messages[0].ID != 7 || ml.messages[1].ID != 8 || ml.messages[2].ID != 10 {
				t.Fatalf("unexpected prepended message order: %#v", ml.messages)
			}
			if _, ok := ml.itemByID[7]; ok {
				t.Fatal("expected overlapping cached item for message 7 to be invalidated")
			}
			if _, ok := ml.itemByID[8]; ok {
				t.Fatal("expected overlapping cached item for message 8 to be invalidated")
			}
		})
	})

	t.Run("jump to message branches", func(t *testing.T) {
		t.Run("invalid ids", func(t *testing.T) {
			m := newTestModelWithTransport(&mockTransport{})
			if err := m.messagesList.jumpToMessage(discord.Channel{}, 0); err == nil {
				t.Fatal("expected invalid id error")
			}
		})

		t.Run("transport error", func(t *testing.T) {
			transport := &mockTransport{
				roundTrip: func(req *http.Request) (*http.Response, error) {
					if strings.Contains(req.URL.Path, "/messages") {
						return nil, errors.New("boom")
					}
					return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("[]")), Header: make(http.Header)}, nil
				},
			}
			m := newTestModelWithTransport(transport)
			if err := m.messagesList.jumpToMessage(discord.Channel{ID: 123, Type: discord.DirectMessage}, 10); err == nil {
				t.Fatal("expected jump to message transport error")
			}
		})

		t.Run("not found in empty response", func(t *testing.T) {
			m := newTestModelWithTransport(&mockTransport{})
			if err := m.messagesList.jumpToMessage(discord.Channel{ID: 123, Type: discord.DirectMessage}, 10); err == nil {
				t.Fatal("expected not found error for empty response")
			}
		})

		t.Run("message missing from loaded window", func(t *testing.T) {
			m := newTestModelWithTransport(&mockTransport{
				messages: []discord.Message{
					{ID: 11, ChannelID: 123, Author: discord.User{ID: 2}},
				},
			})
			if err := m.messagesList.jumpToMessage(discord.Channel{ID: 123, Type: discord.DirectMessage}, 10); err == nil {
				t.Fatal("expected error when target message is not in loaded window")
			}
		})

		t.Run("success selects loaded target", func(t *testing.T) {
			channel := discord.Channel{ID: 123, Type: discord.DirectMessage, DMRecipients: []discord.User{{ID: 9, Username: "recipient"}}}
			m := newTestModelWithTransport(&mockTransport{
				messages: []discord.Message{
					{ID: 11, ChannelID: 123, Author: discord.User{ID: 2}},
					{ID: 10, ChannelID: 123, Author: discord.User{ID: 3}},
					{ID: 9, ChannelID: 123, Author: discord.User{ID: 4}},
				},
			})
			m.typers[discord.UserID(77)] = time.AfterFunc(time.Minute, func() {})
			if err := m.messagesList.jumpToMessage(channel, 10); err != nil {
				t.Fatalf("unexpected jump error: %v", err)
			}
			if m.SelectedChannel() == nil || m.SelectedChannel().ID != channel.ID {
				t.Fatal("expected jump to update selected channel")
			}
			if got := m.messagesList.Cursor(); got != 1 {
				t.Fatalf("expected target cursor 1 after reverse load, got %d", got)
			}
			if len(m.typers) != 0 {
				t.Fatal("expected jump to clear typers")
			}
		})
	})

	t.Run("cursor mapping and selection helpers", func(t *testing.T) {
		m := newTestModelWithTransport(&mockTransport{})
		ml := m.messagesList
		ml.messages = []discord.Message{{ID: 1}, {ID: 2}}
		ml.rows = []messagesListRow{
			{kind: messagesListRowSeparator},
			{kind: messagesListRowMessage, messageIndex: 0},
			{kind: messagesListRowMessage, messageIndex: 1},
		}
		ml.rowsDirty = false

		if got := ml.messageToRowIndex(-1); got != -1 {
			t.Fatalf("expected invalid message index to map to -1, got %d", got)
		}
		ml.Model.SetCursor(-1)
		if got := ml.Cursor(); got != -1 {
			t.Fatalf("expected cleared cursor to be ignored, got %d", got)
		}

		target := ml.messageToRowIndex(1)
		ml.onRowCursorChanged(0)
		if got := ml.Model.Cursor(); got != target && got != ml.messageToRowIndex(0) {
			t.Fatalf("expected separator cursor change to snap to a message row, got %d", got)
		}

		if got := ml.nearestMessageRowIndex(len(ml.rows) - 1); got == -1 {
			t.Fatal("expected nearest message row for in-range row")
		}
	})
}

func TestMessagesListPickerAndRenderBranches(t *testing.T) {
	t.Run("show reaction picker branches", func(t *testing.T) {
		m := newTestModelWithTransport(&mockTransport{})
		ml := m.messagesList

		ml.showReactionPicker()

		m.SetSelectedChannel(&discord.Channel{ID: 123, Type: discord.DirectMessage})
		ml.showReactionPicker()
		if m.HasLayer(reactionPickerLayerName) {
			t.Fatal("expected no reaction picker without a selected message")
		}

		ml.setMessages([]discord.Message{{ID: 1, ChannelID: 123, Author: discord.User{ID: 2}}})
		ml.SetCursor(0)
		ml.showReactionPicker()
		if !m.HasLayer(reactionPickerLayerName) {
			t.Fatal("expected reaction picker layer to open")
		}
		ml.showReactionPicker()
		if !m.HasLayer(reactionPickerLayerName) {
			t.Fatal("expected reaction picker layer to remain visible after replacement")
		}
	})

	t.Run("title and member lookup branches", func(t *testing.T) {
		m := newTestModelWithTransport(&mockTransport{})
		ml := m.messagesList

		ml.setTitle(discord.Channel{ID: 321, Type: discord.GuildText, Name: "general", Topic: "topic"})
		if title := ml.Title(); !strings.Contains(title, "topic") {
			t.Fatalf("expected channel topic in title, got %q", title)
		}

		if member := ml.memberForMessage(discord.Message{Author: discord.User{ID: 1}}); member != nil {
			t.Fatal("expected nil member outside guild")
		}
		if member := ml.memberForMessage(discord.Message{GuildID: 50, WebhookID: 99, Author: discord.User{ID: 1}}); member != nil {
			t.Fatal("expected nil member for webhook message")
		}
		if member := ml.memberForMessage(discord.Message{GuildID: 50, Author: discord.User{ID: 404}}); member != nil {
			t.Fatal("expected nil member when cabinet lookup fails")
		}
	})

	t.Run("render content fallback and attachment branches", func(t *testing.T) {
		m := newTestModelWithTransport(&mockTransport{})
		ml := m.messagesList
		m.cfg.Markdown.Enabled = false

		lines, root := ml.renderContentLinesWithMarkdown(discord.Message{Content: "plain text"}, tcell.StyleDefault, false, false)
		if root != nil {
			t.Fatal("expected markdown root to be nil when markdown is disabled")
		}
		if len(lines) != 1 || len(lines[0]) == 0 || lines[0][0].Text != "plain text" {
			t.Fatalf("unexpected plain render output: %#v", lines)
		}

		builder := tview.NewLineBuilder()
		m.cfg.InlineImages.Enabled = true
		m.cfg.ShowAttachmentLinks = false
		ml.drawDefaultMessage(builder, discord.Message{
			Author: discord.User{ID: 2, Username: "user"},
			Attachments: []discord.Attachment{
				{Filename: "image.png", URL: "https://example.com/image.png", ContentType: "image/png"},
				{Filename: "note.txt", URL: "https://example.com/note.txt", ContentType: "text/plain"},
			},
		}, tcell.StyleDefault, false)
		lines = builder.Finish()
		foundTextAttachment := false
		for _, line := range lines {
			for _, segment := range line {
				if strings.Contains(segment.Text, "note.txt") {
					foundTextAttachment = true
				}
				if strings.Contains(segment.Text, "image.png") {
					t.Fatal("expected inline image attachment text to be skipped")
				}
			}
		}
		if !foundTextAttachment {
			t.Fatal("expected non-image attachment text to be rendered")
		}
	})
}

func TestMessagesListHelperBranches(t *testing.T) {
	t.Run("updateCellDimensions handles missing tty and unchanged values", func(t *testing.T) {
		m := newTestModel()
		ml := m.messagesList
		ml.imageItemByKey = map[string]*imageItem{
			"img": {kittyPayload: "keep"},
		}

		ml.updateCellDimensions(&completeMockScreen{})
		if ml.cellW != 0 || ml.cellH != 0 {
			t.Fatalf("expected no cell dimensions without tty, got %dx%d", ml.cellW, ml.cellH)
		}

		ml.updateCellDimensions(&ttyScreen{tty: windowSizeErrTty{}})
		ml.updateCellDimensions(&ttyScreen{tty: zeroCellTty{}})
		if payload := ml.imageItemByKey["img"].kittyPayload; payload != "keep" {
			t.Fatalf("expected kitty payload to remain unchanged on invalid sizes, got %q", payload)
		}

		ml.cellW, ml.cellH = 10, 20
		ml.updateCellDimensions(&ttyScreen{tty: cellSizeTty{}})
		if payload := ml.imageItemByKey["img"].kittyPayload; payload != "keep" {
			t.Fatalf("expected unchanged dimensions to preserve kitty payload, got %q", payload)
		}
	})

	t.Run("nearestMessageRowIndex returns -1 when no messages exist", func(t *testing.T) {
		ml := newTestModel().messagesList
		ml.rows = []messagesListRow{{kind: messagesListRowSeparator}}
		ml.rowsDirty = false
		if got := ml.nearestMessageRowIndex(0); got != -1 {
			t.Fatalf("expected no nearest message row, got %d", got)
		}
	})

	t.Run("showAttachmentsList wires attachment actions", func(t *testing.T) {
		m := newTestModel()
		ml := m.messagesList

		oldOpenStart := openStart
		oldHTTPGetAttachment := httpGetAttachment
		oldMkdirAllAttachment := mkdirAllAttachment
		oldCreateAttachmentFile := createAttachmentFile
		oldCopyAttachmentData := copyAttachmentData
		t.Cleanup(func() {
			openStart = oldOpenStart
			httpGetAttachment = oldHTTPGetAttachment
			mkdirAllAttachment = oldMkdirAllAttachment
			createAttachmentFile = oldCreateAttachmentFile
			copyAttachmentData = oldCopyAttachmentData
		})

		opened := make(chan string, 4)
		openStart = func(target string) error {
			opened <- target
			return nil
		}
		httpGetAttachment = func(string) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("img")), Header: make(http.Header)}, nil
		}
		mkdirAllAttachment = func(string, os.FileMode) error { return nil }
		createAttachmentFile = func(string) (*os.File, error) { return os.CreateTemp(t.TempDir(), "attachment-*") }
		copyAttachmentData = func(dst io.Writer, src io.Reader) (int64, error) { return io.Copy(dst, src) }

		ml.showAttachmentsList(
			[]string{"https://example.com/link"},
			[]discord.Attachment{
				{Filename: "pic.png", URL: "https://example.com/pic.png", ContentType: "image/png"},
				{Filename: "note.txt", URL: "https://example.com/note.txt", ContentType: "text/plain"},
			},
		)

		if len(ml.attachmentsPicker.items) != 3 {
			t.Fatalf("expected 3 picker items, got %d", len(ml.attachmentsPicker.items))
		}

		ml.attachmentsPicker.items[0].open()
		if got := <-opened; !strings.Contains(got, "attachments/pic.png") && !strings.Contains(got, "attachments\\pic.png") {
			t.Fatalf("expected cached image attachment path, got %q", got)
		}

		ml.attachmentsPicker.items[1].open()
		if got := <-opened; got != "https://example.com/note.txt" {
			t.Fatalf("expected non-image attachment URL, got %q", got)
		}

		ml.attachmentsPicker.items[2].open()
		if got := <-opened; got != "https://example.com/link" {
			t.Fatalf("expected picked URL, got %q", got)
		}
	})
}

func TestMessagesList_MoreBranches(t *testing.T) {
	t.Run("yankContent_Error", func(t *testing.T) {
		m := newTestModel()
		ml := m.messagesList
		ml.SetCursor(-1) // Will cause selectedMessage to error
		if cmd := ml.yankContent(); cmd != nil {
			t.Errorf("expected nil command for no selection, got %T", cmd)
		}

		// clipboard error
		ml.setMessages([]discord.Message{{ID: 1, Content: "kek"}})
		ml.SetCursor(0)
		oldClipboardWrite := clipboardWrite
		defer func() { clipboardWrite = oldClipboardWrite }()
		clipboardWrite = func(fmt clipkg.Format, data []byte) error { return errors.New("fail") }
		if cmd := ml.yankContent(); cmd != nil {
			cmd()
		}
	})

	t.Run("setMessagePinned_Branches", func(t *testing.T) {
		m := newTestModel()
		m.state.Call(&gateway.ReadyEvent{User: discord.User{ID: 0}})
		m.state.Cabinet.MeStore.MyselfSet(discord.User{ID: 0}, true)
		ml := m.messagesList

		// Case 1: In ml.messages
		msg := discord.Message{ID: 1, ChannelID: 2, Content: "kek"}
		ml.setMessages([]discord.Message{msg})
		m.state.Cabinet.MessageStore.MessageSet(&msg, false)
		ml.setMessagePinned(2, 1, true)

		// Case 2: Not in ml.messages but in cabinet
		msg2 := discord.Message{ID: 2, ChannelID: 2, Content: "kek2"}
		m.state.Cabinet.MessageStore.MessageSet(&msg2, false)
		ml.setMessagePinned(2, 2, true)

		// Case 3: Not in cabinet
		ml.setMessagePinned(2, 99, true)
	})

	t.Run("confirmPin_Branches", func(t *testing.T) {
		m := newTestModel()
		ml := m.messagesList

		// no selection
		ml.SetCursor(-1)
		ml.confirmPin()

		// with selection but no permissions (non-DM)
		ch := &discord.Channel{ID: 2, GuildID: 10, Type: discord.GuildText}
		m.SetSelectedChannel(ch)
		ml.setMessages([]discord.Message{{ID: 1, ChannelID: 2, Author: discord.User{ID: 1}}})
		ml.SetCursor(0)
		ml.confirmPin()

		// with selection and permissions (DM)
		chDM := &discord.Channel{ID: 3, Type: discord.DirectMessage}
		m.SetSelectedChannel(chDM)
		ml.setMessages([]discord.Message{{ID: 1, ChannelID: 3, Author: discord.User{ID: 1}}})
		ml.SetCursor(0)
		ml.confirmPin()
	})

	t.Run("pin_Branches", func(t *testing.T) {
		m := newTestModel()
		ml := m.messagesList

		// no selection
		ml.SetCursor(-1)
		ml.pin()

		// with selection
		msg := discord.Message{ID: 1, ChannelID: 3, Author: discord.User{ID: 1}}
		ml.setMessages([]discord.Message{msg})
		ml.SetCursor(0)

		// no channel
		m.SetSelectedChannel(nil)
		ml.pin()

		// with channel but no permissions (non-DM)
		ch := &discord.Channel{ID: 2, GuildID: 10, Type: discord.GuildText}
		m.SetSelectedChannel(ch)
		// Missing permissions
		ml.pin()

		// with channel and permissions (DM)
		chDM := &discord.Channel{ID: 3, Type: discord.DirectMessage}
		m.SetSelectedChannel(chDM)

		// stub pinMessageFunc
		oldPinMessageFunc := pinMessageFunc
		defer func() { pinMessageFunc = oldPinMessageFunc }()

		pinMessageFunc = func(s *state.State, cID discord.ChannelID, mID discord.MessageID, r api.AuditLogReason) error {
			return nil
		}
		ml.pin()

		// error case
		pinMessageFunc = func(s *state.State, cID discord.ChannelID, mID discord.MessageID, r api.AuditLogReason) error {
			return errors.New("pin fail")
		}
		ml.pin()
	})
}
