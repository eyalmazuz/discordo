package chat

import (
	"testing"

	"github.com/ayn2op/tview"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/gdamore/tcell/v3"
)

func TestGuildsTreeThreadCapableChannelsAreExpandable(t *testing.T) {
	m := newTestModel()
	parent := tview.NewTreeNode("guild")

	for _, channel := range []discord.Channel{
		{ID: 1, GuildID: 10, Type: discord.GuildText, Name: "text"},
		{ID: 2, GuildID: 10, Type: discord.GuildAnnouncement, Name: "announcements"},
		{ID: 3, GuildID: 10, Type: discord.GuildForum, Name: "forum"},
	} {
		setViewPermissionForTest(m, channel)
		m.state.Cabinet.ChannelStore.ChannelSet(&channel, false)
		m.guildsTree.createChannelNode(parent, channel)
		node := m.guildsTree.findNodeByReference(channel.ID)
		if node == nil {
			t.Fatalf("expected node for %v", channel.Type)
		}
		if !node.IsExpandable() {
			t.Fatalf("expected %v channel to be expandable for threads", channel.Type)
		}
	}
}

func TestGuildsTreeSelectTextChannelHydratesThreadChildrenAndLoadsChannel(t *testing.T) {
	m := newTestModel()
	guildID := discord.GuildID(10)
	parentChannel := discord.Channel{ID: 100, GuildID: guildID, Type: discord.GuildText, Name: "text"}
	thread := discord.Channel{ID: 101, GuildID: guildID, ParentID: parentChannel.ID, Type: discord.GuildPublicThread, Name: "thread"}
	m.state.Cabinet.GuildStore.GuildSet(&discord.Guild{ID: guildID}, false)
	setViewPermissionForTest(m, parentChannel)
	m.state.Cabinet.ChannelStore.ChannelSet(&parentChannel, false)
	m.state.Cabinet.ChannelStore.ChannelSet(&thread, false)

	node := tview.NewTreeNode("text").SetReference(parentChannel.ID)
	m.guildsTree.channelNodeByID[parentChannel.ID] = node

	cmd := m.guildsTree.onSelected(node)
	if cmd == nil {
		t.Fatal("expected selecting a text channel to return load command")
	}
	if child := m.guildsTree.findNodeByReference(thread.ID); child == nil {
		t.Fatal("expected selecting text channel to hydrate thread children")
	}
}

func TestGuildsTreeEnterSelectsChannelThroughModel(t *testing.T) {
	transport := &mockTransport{
		messages: []discord.Message{{ID: 300, ChannelID: 100, Content: "hello"}},
	}
	m := newTestModelWithTransport(transport)
	channel := discord.Channel{ID: 100, GuildID: 10, Type: discord.GuildText, Name: "text", LastMessageID: 300}
	setViewPermissionForTest(m, channel)

	node := tview.NewTreeNode("text").SetReference(channel.ID)
	m.guildsTree.SetRoot(tview.NewTreeNode("").AddChild(node))
	m.guildsTree.SetCurrentNode(node)

	selectMsg := m.guildsTree.Update(tcell.NewEventKey(tcell.KeyEnter, "", tcell.ModNone))()
	loadCmd := m.Update(selectMsg)
	if loadCmd == nil {
		t.Fatal("expected model to turn tree selection into channel load command")
	}
	loadedMsg := loadCmd()
	focusCmd := m.Update(loadedMsg)
	execCmdForTest(m.app, focusCmd)

	selected := m.SelectedChannel()
	if selected == nil || selected.ID != channel.ID {
		t.Fatalf("expected selected channel %d, got %#v", channel.ID, selected)
	}
	if got := len(m.messagesList.messages); got != 1 {
		t.Fatalf("expected loaded messages to be applied, got %d", got)
	}
}

func TestGuildsTreeSelectForumHydratesThreadsWithoutDuplicates(t *testing.T) {
	m := newTestModel()
	guildID := discord.GuildID(10)
	forum := discord.Channel{ID: 200, GuildID: guildID, Type: discord.GuildForum, Name: "forum"}
	thread := discord.Channel{ID: 201, GuildID: guildID, ParentID: forum.ID, Type: discord.GuildPublicThread, Name: "thread"}
	m.state.Cabinet.GuildStore.GuildSet(&discord.Guild{ID: guildID}, false)
	setViewPermissionForTest(m, forum)
	m.state.Cabinet.ChannelStore.ChannelSet(&forum, false)
	m.state.Cabinet.ChannelStore.ChannelSet(&thread, false)

	node := tview.NewTreeNode("forum").SetReference(forum.ID)
	m.guildsTree.channelNodeByID[forum.ID] = node

	if cmd := m.guildsTree.onSelected(node); cmd != nil {
		t.Fatal("expected selecting a forum to only expand thread children")
	}
	if got := len(node.GetChildren()); got != 1 {
		t.Fatalf("expected one forum thread child, got %d", got)
	}
	if cmd := m.guildsTree.onSelected(node); cmd != nil {
		t.Fatal("expected repeated forum select to remain expansion-only")
	}
	if got := len(node.GetChildren()); got != 1 {
		t.Fatalf("expected no duplicate forum thread children, got %d", got)
	}
}

func setViewPermissionForTest(m *Model, channel discord.Channel) {
	user := discord.User{ID: 1, Username: "me"}
	roleID := discord.RoleID(user.ID)
	m.state.Cabinet.GuildStore.GuildSet(&discord.Guild{ID: channel.GuildID}, false)
	m.state.Cabinet.ChannelStore.ChannelSet(&channel, false)
	m.state.Cabinet.MemberStore.MemberSet(channel.GuildID, &discord.Member{
		User:    user,
		RoleIDs: []discord.RoleID{roleID},
	}, false)
	m.state.Cabinet.RoleStore.RoleSet(channel.GuildID, &discord.Role{
		ID:          roleID,
		Permissions: discord.PermissionViewChannel,
	}, false)
}
