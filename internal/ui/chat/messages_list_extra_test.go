package chat

import (
	"testing"

	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/ayn2op/tview"
	"github.com/gdamore/tcell/v3"
)

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
		ml.writeMessage(builder, msgJoin, baseStyle)
		
		// 3. ChannelPinned
		msgPinned := discord.Message{Type: discord.ChannelPinnedMessage, Author: discord.User{Username: "pinner"}}
		ml.writeMessage(builder, msgPinned, baseStyle)
	})

	t.Run("drawAuthor_Member", func(t *testing.T) {
		builder := tview.NewLineBuilder()
		msg := discord.Message{
			Author: discord.User{ID: 2, Username: "user2"},
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
		ml.drawContent(builder, msg, tcell.StyleDefault)
	})

	t.Run("drawDefaultMessage_Attachments", func(t *testing.T) {
		builder := tview.NewLineBuilder()
		msg := discord.Message{
			Attachments: []discord.Attachment{
				{Filename: "test.txt", URL: "https://example.com/test.txt"},
			},
		}
		ml.drawDefaultMessage(builder, msg, tcell.StyleDefault)
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
		Title: "Title",
		URL: "https://url.com",
		Provider: &discord.EmbedProvider{Name: "Provider"},
		Author: &discord.EmbedAuthor{Name: "Author"},
		Description: "Description with \\*escape\\*",
		Fields: []discord.EmbedField{
			{Name: "F1", Value: "V1"},
			{Name: "F2"},
			{Value: "V3"},
		},
		Footer: &discord.EmbedFooter{Text: "Footer"},
		Color: 0xFF0000,
	}
	
	msg := discord.Message{Embeds: []discord.Embed{embed}}
	ml.drawEmbeds(builder, msg, tcell.StyleDefault)
}

func TestMessagesList_Yank_And_Help(t *testing.T) {
	m := newTestModel()
	ml := m.messagesList
	ml.messages = []discord.Message{{ID: 1, Content: "content", Author: discord.User{Username: "user"}}}
	ml.SetCursor(0)

	t.Run("yankID", func(t *testing.T) {
		ml.yankID()
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
		ml.drawReplyMessage(builder, msg, tcell.StyleDefault)
	})
}
