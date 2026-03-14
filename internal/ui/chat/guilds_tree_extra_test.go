package chat

import (
	"testing"

	"github.com/ayn2op/tview"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/ningen/v3"
	"github.com/gdamore/tcell/v3"
)

func TestGuildsTreeUnreadStyleAndFindNodeFallback(t *testing.T) {
	gt := newGuildsTree(newMockChatModel().cfg, nil)

	tests := []struct {
		name       string
		indication ningen.UnreadIndication
		want       tcell.AttrMask
		underline  bool
	}{
		{name: "read", indication: ningen.ChannelRead, want: tcell.AttrDim},
		{name: "unread", indication: ningen.ChannelUnread, want: tcell.AttrBold},
		{name: "mentioned", indication: ningen.ChannelMentioned, want: tcell.AttrBold, underline: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			style := gt.unreadStyle(tt.indication)
			attrs := style.GetAttributes()
			if attrs&tt.want != tt.want {
				t.Fatalf("expected attrs %v to include %v", attrs, tt.want)
			}
			if style.HasUnderline() != tt.underline {
				t.Fatalf("expected underline=%v, got %v", tt.underline, style.HasUnderline())
			}
		})
	}

	type customReference struct{ value string }
	custom := customReference{value: "custom"}
	node := tview.NewTreeNode("custom").SetReference(custom)
	gt.GetRoot().AddChild(node)
	if got := gt.findNodeByReference(custom); got != node {
		t.Fatalf("expected fallback lookup to return custom node, got %v", got)
	}
}

func TestGuildsTreeCollapseParentAndLoadChildren(t *testing.T) {
	m := newMockChatModel()
	gt := newGuildsTree(m.cfg, m)

	guildID := discord.GuildID(10)
	textChannel := &discord.Channel{ID: 20, GuildID: guildID, Name: "general", Type: discord.GuildText}
	threadChannel := &discord.Channel{ID: 21, GuildID: guildID, ParentID: textChannel.ID, Name: "thread", Type: discord.GuildPublicThread}
	dmChannel := &discord.Channel{ID: 30, Type: discord.DirectMessage, DMRecipients: []discord.User{{ID: 2, Username: "friend"}}}

	setPermissionsForUser(m, guildID, textChannel, discord.User{ID: 1, Username: "me"}, discord.PermissionViewChannel)
	m.state.Cabinet.ChannelStore.ChannelSet(threadChannel, false)
	m.state.Cabinet.ChannelStore.ChannelSet(dmChannel, false)

	guildNode := tview.NewTreeNode("guild").SetReference(guildID).SetExpanded(true)
	channelNode := tview.NewTreeNode("general").SetReference(textChannel.ID)
	gt.GetRoot().AddChild(guildNode)
	guildNode.AddChild(channelNode)
	gt.guildNodeByID[guildID] = guildNode
	gt.channelNodeByID[textChannel.ID] = channelNode
	gt.SetCurrentNode(channelNode)
	gt.SetRect(0, 0, 80, 24)
	gt.Draw(&completeMockScreen{})

	gt.collapseParentNode(channelNode)
	if gt.GetCurrentNode() != guildNode {
		t.Fatalf("expected collapseParentNode to move focus to the parent, got %v", gt.GetCurrentNode())
	}
	if guildNode.IsExpanded() {
		t.Fatal("expected collapseParentNode to collapse the parent node")
	}

	emptyGuildNode := tview.NewTreeNode("empty guild").SetReference(guildID)
	if ok := gt.loadChildren(emptyGuildNode); !ok {
		t.Fatal("expected guild loadChildren to succeed")
	}
	if len(emptyGuildNode.GetChildren()) == 0 {
		t.Fatal("expected guild loadChildren to populate channels")
	}
	if ok := gt.loadChildren(emptyGuildNode); !ok {
		t.Fatal("expected guild loadChildren fast path to succeed when children already exist")
	}

	threadParentNode := tview.NewTreeNode("parent").SetReference(textChannel.ID)
	if ok := gt.loadChildren(threadParentNode); !ok {
		t.Fatal("expected channel loadChildren to load thread children")
	}
	if len(threadParentNode.GetChildren()) != 1 || threadParentNode.GetChildren()[0].GetReference() != threadChannel.ID {
		t.Fatalf("expected thread child to be created, got %#v", threadParentNode.GetChildren())
	}

	dmRoot := tview.NewTreeNode("Direct Messages").SetReference(dmNode{})
	if ok := gt.loadChildren(dmRoot); !ok {
		t.Fatal("expected DM loadChildren to succeed")
	}
	if len(dmRoot.GetChildren()) == 0 {
		t.Fatal("expected DM loadChildren to populate private channels")
	}

	unknown := tview.NewTreeNode("unknown").SetReference("unknown")
	if gt.loadChildren(unknown) {
		t.Fatal("expected loadChildren to return false for unknown references")
	}
}

func TestGuildsTreeExpandPathToNode(t *testing.T) {
	gt := newGuildsTree(newMockChatModel().cfg, nil)
	parent := tview.NewTreeNode("parent").SetExpanded(false)
	child := tview.NewTreeNode("child").SetExpanded(false)
	gt.GetRoot().AddChild(parent)
	parent.AddChild(child)

	gt.expandPathToNode(nil)
	gt.expandPathToNode(child)

	if !gt.GetRoot().IsExpanded() || !parent.IsExpanded() || !child.IsExpanded() {
		t.Fatal("expected expandPathToNode to expand the entire path to the target node")
	}
}

func TestGuildsTreeLoadChildren_ErrorBranches(t *testing.T) {
	m := newMockChatModel()
	gt := newGuildsTree(m.cfg, m)

	t.Run("GuildError", func(t *testing.T) {
		if gt.loadChildren(tview.NewTreeNode("missing guild").SetReference(discord.GuildID(404))) {
			t.Fatal("expected missing guild channels to fail loading children")
		}
	})

	t.Run("ChannelError", func(t *testing.T) {
		if gt.loadChildren(tview.NewTreeNode("missing channel").SetReference(discord.ChannelID(505))) {
			t.Fatal("expected missing channel to fail loading children")
		}
	})

	t.Run("DMError", func(t *testing.T) {
		// Create a model with a transport that returns 401 for private channels
		transport := &mockTransport{}
		mErr := newTestModelWithTokenAndTransport("error-token", transport)
		
		gtErr := newGuildsTree(mErr.cfg, mErr)
		if gtErr.loadChildren(tview.NewTreeNode("dm").SetReference(dmNode{})) {
			t.Fatal("expected missing private channels to fail loading children")
		}
	})
}

func TestGuildsTreeFindNodeByChannelID_LoadsThreadPath(t *testing.T) {
	m := newMockChatModel()
	gt := newGuildsTree(m.cfg, m)
	m.guildsTree = gt

	guildID := discord.GuildID(99)
	parentChannel := &discord.Channel{ID: 200, GuildID: guildID, Name: "general", Type: discord.GuildText}
	threadChannel := &discord.Channel{ID: 201, GuildID: guildID, ParentID: parentChannel.ID, Name: "thread", Type: discord.GuildPublicThread}

	setPermissionsForUser(m, guildID, parentChannel, discord.User{ID: 1, Username: "me"}, discord.PermissionViewChannel)
	m.state.Cabinet.ChannelStore.ChannelSet(threadChannel, false)

	guildNode := tview.NewTreeNode("guild").SetReference(guildID)
	gt.GetRoot().AddChild(guildNode)
	gt.guildNodeByID[guildID] = guildNode

	if node := gt.findNodeByChannelID(threadChannel.ID); node == nil || node.GetReference() != threadChannel.ID {
		t.Fatalf("expected thread lookup to load and return the thread node, got %v", node)
	}

	if node := gt.findNodeByChannelID(9999); node != nil {
		t.Fatalf("expected unknown channel lookup to return nil, got %v", node)
	}
}

func TestGuildsTreeYankIDBranches(t *testing.T) {
	gt := newGuildsTree(newMockChatModel().cfg, nil)
	copied := stubClipboardWrite(t)

	gt.yankID()

	custom := tview.NewTreeNode("custom").SetReference(struct{}{})
	gt.GetRoot().AddChild(custom)
	gt.SetCurrentNode(custom)
	gt.yankID()

	guild := tview.NewTreeNode("guild").SetReference(discord.GuildID(42))
	gt.GetRoot().AddChild(guild)
	gt.SetCurrentNode(guild)
	gt.yankID()
	if got := waitForCopiedText(t, copied); got != "42" {
		t.Fatalf("expected guild yank to copy %q, got %q", "42", got)
	}
}

func TestGuildsTreeExpandPathToNodeNil(t *testing.T) {
	gt := newGuildsTree(newMockChatModel().cfg, nil)
	gt.expandPathToNode(nil)
}
