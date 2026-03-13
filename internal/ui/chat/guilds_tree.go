package chat

import (
	"fmt"
	"log/slog"

	"github.com/ayn2op/discordo/internal/clipboard"
	"github.com/ayn2op/discordo/internal/config"
	"github.com/ayn2op/discordo/internal/ui"
	"github.com/ayn2op/tview"
	"github.com/ayn2op/tview/help"
	"github.com/ayn2op/tview/keybind"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/diamondburned/ningen/v3"
	"github.com/gdamore/tcell/v3"
)

type dmNode struct{}

type guildsTree struct {
	*tview.TreeView
	chat *Model

	cfg *config.Config

	// Fast-path indexes for frequent event handlers (read updates, picker
	// navigation). They mirror the current rendered tree and are rebuilt on
	// READY before nodes are added.
	guildNodeByID   map[discord.GuildID]*tview.TreeNode
	channelNodeByID map[discord.ChannelID]*tview.TreeNode
	dmRootNode      *tview.TreeNode
}

var _ help.KeyMap = (*guildsTree)(nil)

func newGuildsTree(cfg *config.Config, chatView *Model) *guildsTree {
	gt := &guildsTree{
		TreeView: tview.NewTreeView(),
		cfg:      cfg,
		chat:     chatView,

		guildNodeByID:   make(map[discord.GuildID]*tview.TreeNode),
		channelNodeByID: make(map[discord.ChannelID]*tview.TreeNode),
	}

	gt.Box = ui.ConfigureBox(gt.Box, &cfg.Theme)
	gt.
		SetRoot(tview.NewTreeNode("")).
		SetTopLevel(1).
		SetMarkers(tview.TreeMarkers{
			Expanded:  cfg.Sidebar.Markers.Expanded,
			Collapsed: cfg.Sidebar.Markers.Collapsed,
			Leaf:      cfg.Sidebar.Markers.Leaf,
		}).
		SetGraphics(cfg.Theme.GuildsTree.Graphics).
		SetGraphicsColor(tcell.GetColor(cfg.Theme.GuildsTree.GraphicsColor)).
		SetSelectedFunc(gt.onSelected).
		SetTitle("Guilds")

	return gt
}

func (gt *guildsTree) ShortHelp() []keybind.Keybind {
	cfg := gt.cfg.Keybinds.GuildsTree
	selectCurrent := cfg.SelectCurrent.Keybind
	collapseParent := cfg.CollapseParentNode.Keybind
	selectHelp := selectCurrent.Help()
	selectDesc := selectHelp.Desc
	if node := gt.GetCurrentNode(); node != nil {
		if len(node.GetChildren()) > 0 {
			if node.IsExpanded() {
				selectDesc = "collapse"
			} else {
				selectDesc = "expand"
			}
		} else {
			switch node.GetReference().(type) {
			case discord.GuildID, dmNode:
				selectDesc = "expand"
			}
		}
	}
	selectCurrent.SetHelp(selectHelp.Key, selectDesc)
	collapseHelp := collapseParent.Help()
	collapseParent.SetHelp(collapseHelp.Key, "collapse parent")

	shortHelp := []keybind.Keybind{cfg.Up.Keybind, cfg.Down.Keybind, selectCurrent}
	if gt.canCollapseParent(gt.GetCurrentNode()) {
		shortHelp = append(shortHelp, collapseParent)
	}
	return shortHelp
}

func (gt *guildsTree) FullHelp() [][]keybind.Keybind {
	cfg := gt.cfg.Keybinds.GuildsTree
	selectCurrent := cfg.SelectCurrent.Keybind
	collapseParent := cfg.CollapseParentNode.Keybind
	selectHelp := selectCurrent.Help()
	selectDesc := selectHelp.Desc
	if node := gt.GetCurrentNode(); node != nil {
		if len(node.GetChildren()) > 0 {
			if node.IsExpanded() {
				selectDesc = "collapse"
			} else {
				selectDesc = "expand"
			}
		} else {
			switch node.GetReference().(type) {
			case discord.GuildID, dmNode:
				selectDesc = "expand"
			}
		}
	}
	selectCurrent.SetHelp(selectHelp.Key, selectDesc)
	collapseHelp := collapseParent.Help()
	collapseParent.SetHelp(collapseHelp.Key, "collapse parent")

	actions := []keybind.Keybind{selectCurrent, cfg.MoveToParentNode.Keybind}
	if gt.canCollapseParent(gt.GetCurrentNode()) {
		actions = append(actions, collapseParent)
	}

	return [][]keybind.Keybind{
		{cfg.Up.Keybind, cfg.Down.Keybind, cfg.Top.Keybind, cfg.Bottom.Keybind},
		actions,
		{cfg.YankID.Keybind},
	}
}

func (gt *guildsTree) canCollapseParent(node *tview.TreeNode) bool {
	if node == nil {
		return false
	}
	path := gt.GetPath(node)
	// Path layout is [root, ..., node]. A non-root parent means at least 3 nodes.
	if len(path) < 3 {
		return false
	}
	parent := path[len(path)-2]
	return parent != nil && parent.GetLevel() != 0
}

func (gt *guildsTree) resetNodeIndex() {
	// Keep allocated map capacity; READY can rebuild often during reconnects.
	clear(gt.guildNodeByID)
	clear(gt.channelNodeByID)
	gt.dmRootNode = nil
}

func (gt *guildsTree) createFolderNode(folder gateway.GuildFolder, guildsByID map[discord.GuildID]*gateway.GuildCreateEvent) {
	name := "Folder"
	if folder.Name != "" {
		name = folder.Name
	}

	folderNode := tview.NewTreeNode(name).SetExpanded(gt.cfg.Theme.GuildsTree.AutoExpandFolders)
	if folder.Color != 0 {
		folderStyle := tcell.StyleDefault.Foreground(tcell.NewHexColor(int32(folder.Color)))
		gt.setNodeLineStyle(folderNode, folderStyle)
	}
	gt.GetRoot().AddChild(folderNode)

	for _, guildID := range folder.GuildIDs {
		if guildEvent, ok := guildsByID[guildID]; ok {
			gt.createGuildNode(folderNode, guildEvent.Guild)
		}
	}
}

func (gt *guildsTree) unreadStyle(indication ningen.UnreadIndication) tcell.Style {
	var style tcell.Style
	switch indication {
	case ningen.ChannelRead:
		style = style.Dim(true)
	case ningen.ChannelMentioned:
		style = style.Underline(true)
		fallthrough
	case ningen.ChannelUnread:
		style = style.Bold(true)
	}

	return style
}

func (gt *guildsTree) getGuildNodeStyle(guildID discord.GuildID) tcell.Style {
	if gt.chat == nil || gt.chat.state == nil {
		return tcell.StyleDefault
	}
	indication := gt.chat.state.GuildIsUnread(guildID, ningen.GuildUnreadOpts{UnreadOpts: ningen.UnreadOpts{IncludeMutedCategories: true}})
	return gt.unreadStyle(indication)
}

func (gt *guildsTree) getChannelNodeStyle(channelID discord.ChannelID) tcell.Style {
	if gt.chat == nil || gt.chat.state == nil {
		return tcell.StyleDefault
	}
	indication := gt.chat.state.ChannelIsUnread(channelID, ningen.UnreadOpts{IncludeMutedCategories: true})
	return gt.unreadStyle(indication)
}

func (gt *guildsTree) createGuildNode(n *tview.TreeNode, guild discord.Guild) {
	guildNode := tview.NewTreeNode(guild.Name).
		SetReference(guild.ID).
		SetExpandable(true).
		SetExpanded(false).
		SetIndent(gt.cfg.Theme.GuildsTree.Indents.Guild)
	gt.setNodeLineStyle(guildNode, gt.getGuildNodeStyle(guild.ID))
	n.AddChild(guildNode)
	gt.guildNodeByID[guild.ID] = guildNode
}

func (gt *guildsTree) createChannelNode(node *tview.TreeNode, channel discord.Channel) {
	if gt.chat != nil && gt.chat.state != nil {
		if channel.Type != discord.DirectMessage && channel.Type != discord.GroupDM && channel.Type != discord.GuildCategory && !gt.chat.state.HasPermissions(channel.ID, discord.PermissionViewChannel) {
			return
		}
	}

	var state *ningen.State
	if gt.chat != nil {
		state = gt.chat.state
	}

	channelNode := tview.NewTreeNode(ui.ChannelToString(channel, gt.cfg.Icons, state)).SetReference(channel.ID)
	gt.setNodeLineStyle(channelNode, gt.getChannelNodeStyle(channel.ID))
	switch channel.Type {
	case discord.DirectMessage:
		channelNode.SetIndent(gt.cfg.Theme.GuildsTree.Indents.DM)
	case discord.GroupDM:
		channelNode.SetIndent(gt.cfg.Theme.GuildsTree.Indents.GroupDM)
	case discord.GuildCategory:
		channelNode.SetIndent(gt.cfg.Theme.GuildsTree.Indents.Category)
		channelNode.SetExpandable(true).SetExpanded(true)
	case discord.GuildForum:
		channelNode.SetIndent(gt.cfg.Theme.GuildsTree.Indents.Forum)
		channelNode.SetExpandable(true).SetExpanded(false)
	case discord.GuildText, discord.GuildAnnouncement:
		channelNode.SetIndent(gt.cfg.Theme.GuildsTree.Indents.Channel)
		channelNode.SetExpandable(true).SetExpanded(false)
	default:
		channelNode.SetIndent(gt.cfg.Theme.GuildsTree.Indents.Channel)
	}
	node.AddChild(channelNode)
	gt.channelNodeByID[channel.ID] = channelNode
	gt.setNodeLineStyle(channelNode, gt.getChannelNodeStyle(channel.ID))
}

func (gt *guildsTree) setNodeLineStyle(node *tview.TreeNode, style tcell.Style) {
	line := node.GetLine()
	for i := range line {
		line[i].Style = style
	}
	node.SetLine(line)
}

func (gt *guildsTree) createChannelNodes(node *tview.TreeNode, channels []discord.Channel) {
	// Preserve exact ordering semantics:
	// 1) top-level non-categories (in input order),
	// 2) categories that have at least one child in the source slice (in input order),
	// 3) parented channels under already-created categories (in input order).
	//
	// We precompute parent presence once to avoid the O(n^2) category-child scan.
	hasChildByParentID := make(map[discord.ChannelID]struct{}, len(channels))
	for _, channel := range channels {
		if channel.ParentID.IsValid() {
			hasChildByParentID[channel.ParentID] = struct{}{}
		}
	}

	for _, channel := range channels {
		if channel.Type != discord.GuildCategory && !channel.ParentID.IsValid() {
			gt.createChannelNode(node, channel)
		}
	}

	for _, channel := range channels {
		if channel.Type == discord.GuildCategory {
			if _, ok := hasChildByParentID[channel.ID]; ok {
				gt.createChannelNode(node, channel)
			}
		}
	}

	for _, channel := range channels {
		if channel.ParentID.IsValid() {
			// Parent categories are inserted earlier in this function, so this
			// lookup is O(1) and avoids per-channel subtree walks.
			parent := gt.channelNodeByID[channel.ParentID]
			if parent != nil {
				gt.createChannelNode(parent, channel)
			}
		}
	}
}

func (gt *guildsTree) onSelected(node *tview.TreeNode) {
	ref := node.GetReference()
	if ref == nil {
		return
	}

	// Case 1: Forum Channels (Expansion only)
	if cid, ok := ref.(discord.ChannelID); ok {
		channel, err := gt.chat.state.Cabinet.Channel(cid)
		if err == nil && channel.Type == discord.GuildForum {
			gt.loadChildren(node)
			node.SetExpanded(!node.IsExpanded())
			return
		}

		// Case 2: Openable Text Channels/Threads
		if err == nil && (channel.Type == discord.GuildText || channel.Type == discord.GuildAnnouncement ||
			channel.Type == discord.GuildPublicThread || channel.Type == discord.GuildPrivateThread ||
			channel.Type == discord.GuildAnnouncementThread || channel.Type == discord.DirectMessage ||
			channel.Type == discord.GroupDM) {
			gt.loadChannel(node, channel)
			return
		}
	}

	// Case 3: Container Nodes (Guilds, Categories, DMs Folder)
	gt.loadChildren(node)
	node.SetExpanded(!node.IsExpanded())
}

func (gt *guildsTree) loadChildren(node *tview.TreeNode) bool {
	if len(node.GetChildren()) != 0 {
		return true
	}

	switch ref := node.GetReference().(type) {
	case discord.GuildID:
		go gt.chat.state.MemberState.Subscribe(ref)

		channels, err := gt.chat.state.Cabinet.Channels(ref)
		if err != nil {
			slog.Error("failed to get channels", "err", err, "guild_id", ref)
			return false
		}

		ui.SortGuildChannels(channels)
		gt.createChannelNodes(node, channels)
		return true
	case discord.ChannelID:
		channel, err := gt.chat.state.Cabinet.Channel(ref)
		if err != nil {
			slog.Error("failed to get channel from state", "err", err, "channel_id", ref)
			return false
		}

		// Handle thread population for channels that can have them
		if channel.Type == discord.GuildForum || channel.Type == discord.GuildText || channel.Type == discord.GuildAnnouncement {
			allChannels, err := gt.chat.state.Cabinet.Channels(channel.GuildID)
			if err != nil {
				slog.Error("failed to get channels for threads", "err", err, "guild_id", channel.GuildID)
				return false
			}

			for _, ch := range allChannels {
				if ch.ParentID == channel.ID && (ch.Type == discord.GuildPublicThread ||
					ch.Type == discord.GuildPrivateThread ||
					ch.Type == discord.GuildAnnouncementThread) {
					gt.createChannelNode(node, ch)
				}
			}
			return true
		}
	case dmNode: // Direct messages folder
		channels, err := gt.chat.state.PrivateChannels()
		if err != nil {
			slog.Error("failed to get private channels", "err", err)
			return false
		}

		ui.SortPrivateChannels(channels)
		for _, c := range channels {
			gt.createChannelNode(node, c)
		}
		return true
	}

	return false
}

func (gt *guildsTree) loadChannel(node *tview.TreeNode, channel *discord.Channel) {
	limit := gt.cfg.MessagesLimit
	messages, err := gt.chat.state.Messages(channel.ID, uint(limit))
	if err != nil {
		slog.Error("failed to get messages", "err", err, "channel_id", channel.ID, "limit", limit)
		return
	}

	go gt.chat.state.ReadState.MarkRead(channel.ID, channel.LastMessageID)

	if guildID := channel.GuildID; guildID.IsValid() {
		gt.chat.messagesList.requestGuildMembers(guildID, messages)
	}

	gt.chat.SetSelectedChannel(channel)
	gt.chat.clearTypers()
	gt.chat.messageInput.stopTypingTimer()

	gt.chat.messagesList.reset()
	gt.chat.messagesList.setTitle(*channel)
	gt.chat.messagesList.setMessages(messages)
	gt.chat.messagesList.ScrollToEnd()

	hasNoPerm := channel.Type != discord.DirectMessage && channel.Type != discord.GroupDM && !gt.chat.state.HasPermissions(channel.ID, discord.PermissionSendMessages)
	gt.chat.messageInput.SetDisabled(hasNoPerm)
	var text string
	if hasNoPerm {
		text = "You do not have permission to send messages in this channel."
	} else {
		text = "Message..."
		if gt.cfg.AutoFocus {
			gt.chat.app.SetFocus(gt.chat.messageInput)
		}
	}
	gt.chat.messageInput.SetPlaceholder(tview.NewLine(tview.NewSegment(text, tcell.StyleDefault.Dim(true))))
}

func (gt *guildsTree) collapseParentNode(node *tview.TreeNode) {
	gt.
		GetRoot().
		Walk(func(n, parent *tview.TreeNode) bool {
			if n == node && parent.GetLevel() != 0 {
				parent.Collapse()
				gt.SetCurrentNode(parent)
				return false
			}

			return true
		})
}

func (gt *guildsTree) HandleEvent(event tcell.Event) tview.Command {
	switch event := event.(type) {
	case *tview.KeyEvent:
		redraw := tview.RedrawCommand{}
		handler := gt.TreeView.HandleEvent

		switch {
		case keybind.Matches(event, gt.cfg.Keybinds.GuildsTree.CollapseParentNode.Keybind):
			gt.collapseParentNode(gt.GetCurrentNode())
			return redraw
		case keybind.Matches(event, gt.cfg.Keybinds.GuildsTree.MoveToParentNode.Keybind):
			return handler(tcell.NewEventKey(tcell.KeyRune, "K", tcell.ModNone))
		case keybind.Matches(event, gt.cfg.Keybinds.GuildsTree.Up.Keybind):
			return handler(tcell.NewEventKey(tcell.KeyUp, "", tcell.ModNone))
		case keybind.Matches(event, gt.cfg.Keybinds.GuildsTree.Down.Keybind):
			return handler(tcell.NewEventKey(tcell.KeyDown, "", tcell.ModNone))
		case keybind.Matches(event, gt.cfg.Keybinds.GuildsTree.Top.Keybind):
			gt.Move(gt.GetRowCount() * -1)
			return redraw
		case keybind.Matches(event, gt.cfg.Keybinds.GuildsTree.Bottom.Keybind):
			gt.Move(gt.GetRowCount())
			return redraw
		case keybind.Matches(event, gt.cfg.Keybinds.GuildsTree.SelectCurrent.Keybind):
			if node := gt.GetCurrentNode(); node != nil {
				gt.onSelected(node)
			}
			return redraw
		case keybind.Matches(event, gt.cfg.Keybinds.GuildsTree.ToggleExpand.Keybind) || (event.Key() == tcell.KeyRune && event.Str() == " "):
			if node := gt.GetCurrentNode(); node != nil {
				gt.loadChildren(node)
				node.SetExpanded(!node.IsExpanded())
			}
			return redraw
		case keybind.Matches(event, gt.cfg.Keybinds.GuildsTree.YankID.Keybind):
			gt.yankID()
			return nil
		}
		// Do not fall through to TreeView defaults for unmatched keys.
		return nil
	}
	return gt.TreeView.HandleEvent(event)
}

func (gt *guildsTree) yankID() {
	node := gt.GetCurrentNode()
	if node == nil {
		return
	}

	// Reference of a tree node in the guilds tree is its ID.
	// discord.Snowflake (discord.GuildID and discord.ChannelID) have the String method.
	if id, ok := node.GetReference().(fmt.Stringer); ok {
		go func() {
			if err := clipboard.Write(clipboard.FmtText, []byte(id.String())); err != nil {
				slog.Error("failed to copy node id", "err", err)
			}
		}()
	}
}

func (gt *guildsTree) findNodeByReference(reference any) *tview.TreeNode {
	switch ref := reference.(type) {
	case discord.GuildID:
		return gt.guildNodeByID[ref]
	case discord.ChannelID:
		return gt.channelNodeByID[ref]
	case dmNode:
		return gt.dmRootNode
	default:
		// Fallback keeps this helper safe for non-indexed custom references.
		var found *tview.TreeNode
		gt.GetRoot().Walk(func(node, _ *tview.TreeNode) bool {
			if node.GetReference() == reference {
				found = node
				return false
			}
			return true
		})
		return found
	}
}

func (gt *guildsTree) findNodeByChannelID(channelID discord.ChannelID) *tview.TreeNode {
	channel, err := gt.chat.state.Cabinet.Channel(channelID)
	if err != nil {
		slog.Error("failed to get channel", "channel_id", channelID, "err", err)
		return nil
	}

	// If it's a thread, we must ensure the parent channel is expanded first
	if channel.Type == discord.GuildPublicThread || channel.Type == discord.GuildPrivateThread || channel.Type == discord.GuildAnnouncementThread {
		if gt.findNodeByChannelID(channel.ParentID) == nil {
			return nil
		}
	}

	var reference any
	if guildID := channel.GuildID; guildID.IsValid() {
		reference = guildID
	} else {
		reference = dmNode{}
	}

	if parentNode := gt.findNodeByReference(reference); parentNode != nil {
		if len(parentNode.GetChildren()) == 0 {
			gt.onSelected(parentNode)
		}
	}

	node := gt.findNodeByReference(channelID)
	return node
}

func (gt *guildsTree) expandPathToNode(node *tview.TreeNode) {
	if node == nil {
		return
	}
	for _, n := range gt.GetPath(node) {
		n.Expand()
	}
}
