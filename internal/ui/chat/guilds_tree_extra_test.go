package chat

import (
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/ayn2op/discordo/internal/clipboard"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/state/store"
	"github.com/diamondburned/arikawa/v3/state/store/defaultstore"
	"github.com/diamondburned/ningen/v3"
	"github.com/eyalmazuz/tview"
	"github.com/gdamore/tcell/v3"
)

type errGuildChannelStore struct {
	base *defaultstore.Channel
}

func (s *errGuildChannelStore) Reset() error {
	return s.base.Reset()
}

func (s *errGuildChannelStore) Channel(id discord.ChannelID) (*discord.Channel, error) {
	return s.base.Channel(id)
}

func (s *errGuildChannelStore) CreatePrivateChannel(recipient discord.UserID) (*discord.Channel, error) {
	return s.base.CreatePrivateChannel(recipient)
}

func (s *errGuildChannelStore) Channels(discord.GuildID) ([]discord.Channel, error) {
	return nil, store.ErrNotFound
}

func (s *errGuildChannelStore) PrivateChannels() ([]discord.Channel, error) {
	return s.base.PrivateChannels()
}

func (s *errGuildChannelStore) ChannelSet(c *discord.Channel, update bool) error {
	return s.base.ChannelSet(c, update)
}

func (s *errGuildChannelStore) ChannelRemove(c *discord.Channel) error {
	return s.base.ChannelRemove(c)
}

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
	gt.onSelected(emptyGuildNode)
	if len(emptyGuildNode.GetChildren()) == 0 {
		t.Fatal("expected guild loadChildren to populate channels")
	}
	gt.onSelected(emptyGuildNode)

	threadParentNode := tview.NewTreeNode("parent").SetReference(textChannel.ID)
	gt.onSelected(threadParentNode)

	dmRoot := tview.NewTreeNode("Direct Messages").SetReference(dmNode{})
	gt.onSelected(dmRoot)
	if len(dmRoot.GetChildren()) == 0 {
		t.Fatal("expected DM loadChildren to populate private channels")
	}

	unknown := tview.NewTreeNode("unknown").SetReference("unknown")
	gt.onSelected(unknown)
	if false {
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
		gt.onSelected(tview.NewTreeNode("missing guild").SetReference(discord.GuildID(404)))
	})

	t.Run("ChannelError", func(t *testing.T) {
		gt.onSelected(tview.NewTreeNode("missing channel").SetReference(discord.ChannelID(505)))
	})

	t.Run("DMError", func(t *testing.T) {
		// Create a model with a transport that returns 401 for private channels
		transport := &mockTransport{}
		mErr := newTestModelWithTokenAndTransport("error-token", transport)

		gtErr := newGuildsTree(mErr.cfg, mErr)
		gtErr.onSelected(tview.NewTreeNode("dm").SetReference(dmNode{}))
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

func TestGuildsTreeDMAlerts(t *testing.T) {
	m := newTestModel()
	root := m.guildsTree.GetRoot()
	dmRoot := tview.NewTreeNode("Direct Messages").SetReference(dmNode{}).SetExpandable(true)
	root.ClearChildren().AddChild(dmRoot)
	m.guildsTree.dmRootNode = dmRoot

	dm1 := &discord.Channel{ID: 91, Type: discord.DirectMessage, DMRecipients: []discord.User{{ID: 2, Username: "alice"}}}
	dm2 := &discord.Channel{ID: 92, Type: discord.DirectMessage, DMRecipients: []discord.User{{ID: 3, Username: "bob"}}}
	if err := m.state.Cabinet.ChannelStore.ChannelSet(dm1, false); err != nil {
		t.Fatalf("channel set dm1: %v", err)
	}
	if err := m.state.Cabinet.ChannelStore.ChannelSet(dm2, false); err != nil {
		t.Fatalf("channel set dm2: %v", err)
	}

	m.guildsTree.addDMAlert(dm1.ID)
	m.guildsTree.addDMAlert(dm1.ID)
	m.guildsTree.addDMAlert(dm2.ID)

	children := root.GetChildren()
	if len(children) < 4 {
		t.Fatalf("expected alert section + separator + dm root, got %d children", len(children))
	}
	if ref, ok := children[0].GetReference().(dmAlertRef); !ok || ref.channelID != dm2.ID {
		t.Fatalf("expected most recent DM alert first, got %v", children[0].GetReference())
	}
	if got := joinedLineText(children[0].GetLine()); !strings.Contains(got, "bob (1)") {
		t.Fatalf("expected bob count label, got %q", got)
	}
	if got := joinedLineText(children[1].GetLine()); !strings.Contains(got, "alice (2)") {
		t.Fatalf("expected alice count label, got %q", got)
	}

	m.guildsTree.clearDMAlert(dm2.ID)
	children = root.GetChildren()
	if len(children) < 3 {
		t.Fatalf("expected one alert + separator + dm root, got %d children", len(children))
	}
	if _, ok := children[0].GetReference().(dmAlertRef); !ok {
		t.Fatalf("expected remaining alert node first, got %v", children[0].GetReference())
	}

	m.guildsTree.clearDMAlert(dm1.ID)
	children = root.GetChildren()
	if len(children) != 1 || children[0] != dmRoot {
		t.Fatalf("expected only dm root after clearing alerts, got %v", children)
	}
}

func TestGuildsTreeResetNodeIndexClearsDMAlerts(t *testing.T) {
	m := newTestModel()
	gt := m.guildsTree

	gt.dmRootNode = tview.NewTreeNode("Direct Messages").SetReference(dmNode{})
	gt.guildNodeByID[1] = tview.NewTreeNode("guild")
	gt.channelNodeByID[2] = tview.NewTreeNode("channel")
	gt.addDMAlert(3)

	gt.resetNodeIndex()

	if gt.dmRootNode != nil {
		t.Fatal("expected resetNodeIndex to clear the DM root pointer")
	}
	if len(gt.guildNodeByID) != 0 || len(gt.channelNodeByID) != 0 {
		t.Fatal("expected resetNodeIndex to clear indexed tree nodes")
	}
	if len(gt.dmAlertCounts) != 0 || len(gt.dmAlertOrder) != 0 || len(gt.dmAlertNodeByID) != 0 {
		t.Fatal("expected resetNodeIndex to clear DM alert state")
	}
	if gt.dmAlertSepNode != nil {
		t.Fatal("expected resetNodeIndex to clear the DM alert separator")
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
	executeCommand(requireCommand(t, gt.yankID()))
	if got := waitForCopiedText(t, copied); got != "42" {
		t.Fatalf("expected guild yank to copy %q, got %q", "42", got)
	}
}

func TestGuildsTreeYankIDClipboardFailure(t *testing.T) {
	gt := newGuildsTree(newMockChatModel().cfg, nil)
	node := tview.NewTreeNode("guild").SetReference(discord.GuildID(99))
	gt.GetRoot().AddChild(node)
	gt.SetCurrentNode(node)

	oldClipboardWrite := clipboardWrite
	called := make(chan struct{}, 1)
	clipboardWrite = func(_ clipboard.Format, _ []byte) error {
		called <- struct{}{}
		return fmt.Errorf("clipboard fail")
	}
	t.Cleanup(func() { clipboardWrite = oldClipboardWrite })

	executeCommand(requireCommand(t, gt.yankID()))
	select {
	case <-called:
	case <-time.After(300 * time.Millisecond):
		t.Fatal("timed out waiting for clipboard failure branch")
	}
}

func TestGuildsTreeExpandPathToNodeNil(t *testing.T) {
	gt := newGuildsTree(newMockChatModel().cfg, nil)
	gt.expandPathToNode(nil)
}

func TestGuildsTreeAdditionalBranchCoverage(t *testing.T) {
	t.Run("group dm channel uses group dm indent", func(t *testing.T) {
		m := newMockChatModel()
		gt := newGuildsTree(m.cfg, m)
		parent := tview.NewTreeNode("parent")
		channel := discord.Channel{ID: 15, Type: discord.GroupDM, Name: "group"}
		gt.createChannelNode(parent, channel)
		if len(parent.GetChildren()) != 1 {
			t.Fatal("expected group dm node to be created")
		}
		child := parent.GetChildren()[0]
		field := reflect.ValueOf(child).Elem().FieldByName("indent")
		indent := *(*int)(unsafe.Pointer(field.UnsafeAddr()))
		if indent != gt.cfg.Theme.GuildsTree.Indents.GroupDM {
			t.Fatalf("expected group dm indent %d, got %d", gt.cfg.Theme.GuildsTree.Indents.GroupDM, indent)
		}
	})

	t.Run("short and full help expose collapse for nested nodes", func(t *testing.T) {
		m := newMockChatModel()
		gt := newGuildsTree(m.cfg, m)
		parent := tview.NewTreeNode("parent").SetReference(discord.GuildID(1)).SetExpanded(true)
		child := tview.NewTreeNode("child").SetReference(discord.ChannelID(2))
		gt.GetRoot().AddChild(parent)
		parent.AddChild(child)
		gt.SetCurrentNode(child)
		gt.SetRect(0, 0, 80, 24)
		gt.Draw(&completeMockScreen{})

		if !gt.canCollapseParent(child) {
			t.Fatal("expected nested node to allow collapsing its parent")
		}
		collapseKey := gt.cfg.Keybinds.GuildsTree.CollapseParentNode.Keybind.Help().Key
		foundShort := false
		for _, binding := range gt.ShortHelp() {
			if binding.Help().Key == collapseKey {
				foundShort = true
				break
			}
		}
		if !foundShort {
			t.Fatal("expected short help to include collapse-parent for nested nodes")
		}
		foundFull := false
		for _, group := range gt.FullHelp() {
			for _, binding := range group {
				if binding.Help().Key == collapseKey {
					foundFull = true
					break
				}
			}
		}
		if !foundFull {
			t.Fatal("expected full help to include collapse-parent for nested nodes")
		}
	})

	t.Run("help marks leaf guild and dm nodes as expandable", func(t *testing.T) {
		gt := newGuildsTree(newMockChatModel().cfg, nil)

		guildNode := tview.NewTreeNode("guild").SetReference(discord.GuildID(1))
		gt.SetCurrentNode(guildNode)
		if !containsKeybindGroup(gt.FullHelp(), gt.cfg.Keybinds.GuildsTree.MoveToParentNode.Keybind) {
			t.Fatal("expected full help to remain populated for guild leaf")
		}

		dmRoot := tview.NewTreeNode("dm").SetReference(dmNode{})
		gt.SetCurrentNode(dmRoot)
		if len(gt.ShortHelp()) == 0 {
			t.Fatal("expected short help for DM root leaf")
		}
	})

	t.Run("onSelected nil and forum branches", func(t *testing.T) {
		m := newMockChatModel()
		gt := newGuildsTree(m.cfg, m)

		gt.onSelected(tview.NewTreeNode("nil"))

		forum := &discord.Channel{ID: 22, GuildID: 33, Name: "forum", Type: discord.GuildForum}
		m.state.Cabinet.ChannelStore.ChannelSet(forum, false)
		node := tview.NewTreeNode("forum").SetReference(forum.ID).SetExpanded(false)
		gt.onSelected(node)
		if !node.IsExpanded() {
			t.Fatal("expected forum node selection to toggle expansion")
		}
	})

	t.Run("loadChannel message error", func(t *testing.T) {
		transport := &mockTransport{
			roundTrip: func(req *http.Request) (*http.Response, error) {
				if strings.Contains(req.URL.Path, "/messages") {
					return &http.Response{
						StatusCode: 500,
						Body:       io.NopCloser(strings.NewReader(`{"message":"boom"}`)),
						Header:     make(http.Header),
					}, nil
				}
				return (&mockTransport{}).RoundTrip(req)
			},
		}
		mErr := newTestModelWithTransport(transport)
		gtErr := newGuildsTree(mErr.cfg, mErr)
		channel := &discord.Channel{ID: 70, GuildID: 80, Name: "general", Type: discord.GuildText}
		mErr.state.Cabinet.ChannelStore.ChannelSet(channel, false)
		gtErr.loadChannel(*channel)
		if mErr.SelectedChannel() != nil {
			t.Fatal("expected loadChannel error to leave selected channel unchanged")
		}
	})

	t.Run("loadChildren thread channel lookup error", func(t *testing.T) {
		m := newMockChatModel()
		m.state.Cabinet.ChannelStore = &errGuildChannelStore{base: defaultstore.NewChannel()}
		gt := newGuildsTree(m.cfg, m)
		text := &discord.Channel{ID: 51, GuildID: 61, Name: "general", Type: discord.GuildText}
		if err := m.state.Cabinet.ChannelStore.ChannelSet(text, false); err != nil {
			t.Fatalf("failed to seed channel store: %v", err)
		}
		gt.onSelected(tview.NewTreeNode("general").SetReference(text.ID))
	})

	t.Run("handle event collapse parent and non-key fallthrough", func(t *testing.T) {
		m := newMockChatModel()
		gt := newGuildsTree(m.cfg, m)
		parent := tview.NewTreeNode("parent").SetReference(discord.GuildID(1)).SetExpanded(true)
		child := tview.NewTreeNode("child").SetReference(discord.ChannelID(2))
		gt.GetRoot().AddChild(parent)
		parent.AddChild(child)
		gt.SetCurrentNode(child)
		gt.SetRect(0, 0, 80, 24)
		gt.Draw(&completeMockScreen{})

		gt.Update(tcell.NewEventKey(tcell.KeyRune, "-", tcell.ModNone))
		if gt.GetCurrentNode() != parent || parent.IsExpanded() {
			t.Fatal("expected collapse-parent key to collapse and select parent")
		}

		gt.SetCurrentNode(child)
		if cmd := gt.Update(tcell.NewEventKey(tcell.KeyRune, "p", tcell.ModNone)); cmd != nil {
			t.Fatalf("expected move-to-parent key to update selection directly, got %T", cmd)
		}
		if gt.GetCurrentNode() != parent {
			t.Fatal("expected move-to-parent key to move the current node to the parent")
		}
		if cmd := gt.Update(tcell.NewEventMouse(0, 0, tcell.ButtonNone, 0)); cmd != nil {
			t.Fatalf("expected unmatched non-key event to fall through without command, got %T", cmd)
		}
	})

	t.Run("find node by reference and channel id handles dm and missing thread parent", func(t *testing.T) {
		m := newMockChatModel()
		gt := newGuildsTree(m.cfg, m)
		m.guildsTree = gt

		dmRoot := tview.NewTreeNode("Direct Messages").SetReference(dmNode{})
		gt.dmRootNode = dmRoot
		gt.GetRoot().AddChild(dmRoot)
		dm := &discord.Channel{ID: 91, Type: discord.DirectMessage, DMRecipients: []discord.User{{ID: 2, Username: "friend"}}}
		m.state.Cabinet.ChannelStore.ChannelSet(dm, false)

		if got := gt.findNodeByReference(dmNode{}); got != dmRoot {
			t.Fatalf("expected DM root lookup, got %v", got)
		}
		if got := gt.findNodeByChannelID(dm.ID); got == nil || got.GetReference() != dm.ID {
			t.Fatalf("expected DM lookup to load and return the DM node, got %v", got)
		}

		thread := &discord.Channel{ID: 92, GuildID: 100, ParentID: 93, Type: discord.GuildPublicThread}
		m.state.Cabinet.ChannelStore.ChannelSet(thread, false)
		if got := gt.findNodeByChannelID(thread.ID); got != nil {
			t.Fatalf("expected missing thread parent lookup to fail, got %v", got)
		}
	})
}
