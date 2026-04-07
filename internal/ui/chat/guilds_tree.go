package chat

import (
	"fmt"
	"log/slog"

	"github.com/ayn2op/discordo/internal/clipboard"
	"github.com/ayn2op/discordo/internal/config"
	"github.com/ayn2op/discordo/internal/ui"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/diamondburned/ningen/v3"
	"github.com/eyalmazuz/tview"
	"github.com/eyalmazuz/tview/help"
	"github.com/eyalmazuz/tview/keybind"
	"github.com/gdamore/tcell/v3"
)

type dmNode struct{}
type dmAlertRef struct {
	channelID discord.ChannelID
}

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
	dmAlertNodeByID map[discord.ChannelID]*tview.TreeNode
	dmAlertOrder    []discord.ChannelID
	dmAlertCounts   map[discord.ChannelID]int
	dmAlertSepNode  *tview.TreeNode
}

var _ help.KeyMap = (*guildsTree)(nil)

func newGuildsTree(cfg *config.Config, chatView *Model) *guildsTree {
	gt := &guildsTree{
		TreeView: tview.NewTreeView(),
		cfg:      cfg,
		chat:     chatView,

		guildNodeByID:   make(map[discord.GuildID]*tview.TreeNode),
		channelNodeByID: make(map[discord.ChannelID]*tview.TreeNode),
		dmAlertNodeByID: make(map[discord.ChannelID]*tview.TreeNode),
		dmAlertCounts:   make(map[discord.ChannelID]int),
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
			switch node.GetReference().(type) {
			case discord.ChannelID:
				selectDesc = "sel"
			default:
				if node.IsExpanded() {
					selectDesc = "collapse"
				} else {
					selectDesc = "expand"
				}
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
	if node := gt.GetCurrentNode(); node != nil && len(node.GetChildren()) > 0 {
		toggleExpand := cfg.ToggleExpand.Keybind
		toggleHelp := toggleExpand.Help()
		if node.IsExpanded() {
			toggleExpand.SetHelp(toggleHelp.Key, "collapse")
		} else {
			toggleExpand.SetHelp(toggleHelp.Key, "expand")
		}
		shortHelp = append(shortHelp, toggleExpand)
	}
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
			switch node.GetReference().(type) {
			case discord.ChannelID:
				selectDesc = "sel"
			default:
				if node.IsExpanded() {
					selectDesc = "collapse"
				} else {
					selectDesc = "expand"
				}
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

	actions := []keybind.Keybind{selectCurrent, cfg.ToggleExpand.Keybind, cfg.MoveToParentNode.Keybind}
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
	clear(gt.dmAlertNodeByID)
	gt.dmAlertSepNode = nil
}

func (gt *guildsTree) dmAlertLabel(channelID discord.ChannelID) string {
	count := gt.dmAlertCounts[channelID]
	label := "Direct Message"
	if gt != nil && gt.chat != nil && gt.chat.state != nil {
		if channel, err := gt.chat.state.Cabinet.Channel(channelID); err == nil {
			label = ui.ChannelToString(*channel, gt.cfg.Icons, gt.chat.state)
		}
	}
	if count > 0 {
		label = fmt.Sprintf("%s (%d)", label, count)
	}
	return label
}

func (gt *guildsTree) rebuildDMAlertSection() {
	root := gt.GetRoot()
	if root == nil || gt.dmRootNode == nil {
		return
	}

	current := gt.GetCurrentNode()
	var currentRef any
	if current != nil {
		currentRef = current.GetReference()
	}

	existing := root.GetChildren()
	nonAlert := make([]*tview.TreeNode, 0, len(existing))
	for _, child := range existing {
		switch child.GetReference().(type) {
		case dmAlertRef:
			continue
		default:
			if child == gt.dmAlertSepNode {
				continue
			}
			nonAlert = append(nonAlert, child)
		}
	}

	clear(gt.dmAlertNodeByID)
	alertNodes := make([]*tview.TreeNode, 0, len(gt.dmAlertOrder))
	for _, channelID := range gt.dmAlertOrder {
		count := gt.dmAlertCounts[channelID]
		if count <= 0 {
			continue
		}
		node := tview.NewTreeNode(gt.dmAlertLabel(channelID)).
			SetReference(dmAlertRef{channelID: channelID}).
			SetIndent(gt.cfg.Theme.GuildsTree.Indents.DM)
		gt.setNodeLineStyle(node, tcell.StyleDefault.Bold(true))
		alertNodes = append(alertNodes, node)
		gt.dmAlertNodeByID[channelID] = node
	}

	children := make([]*tview.TreeNode, 0, len(alertNodes)+len(nonAlert)+1)
	children = append(children, alertNodes...)
	if len(alertNodes) > 0 {
		gt.dmAlertSepNode = tview.NewTreeNode("--------------").SetSelectable(false)
		gt.setNodeLineStyle(gt.dmAlertSepNode, tcell.StyleDefault.Dim(true))
		children = append(children, gt.dmAlertSepNode)
	} else {
		gt.dmAlertSepNode = nil
	}
	children = append(children, nonAlert...)
	root.SetChildren(children)

	switch ref := currentRef.(type) {
	case dmAlertRef:
		if node := gt.dmAlertNodeByID[ref.channelID]; node != nil {
			gt.SetCurrentNode(node)
			return
		}
	case discord.GuildID, discord.ChannelID, dmNode:
		if node := gt.findNodeByReference(ref); node != nil {
			gt.SetCurrentNode(node)
			return
		}
	}
}

func (gt *guildsTree) addDMAlert(channelID discord.ChannelID) {
	if channelID == 0 {
		return
	}
	if _, ok := gt.dmAlertCounts[channelID]; !ok {
		gt.dmAlertOrder = append([]discord.ChannelID{channelID}, gt.dmAlertOrder...)
	}
	gt.dmAlertCounts[channelID]++
	for i := 1; i < len(gt.dmAlertOrder); i++ {
		if gt.dmAlertOrder[i] == channelID {
			gt.dmAlertOrder = append([]discord.ChannelID{channelID}, append(gt.dmAlertOrder[:i], gt.dmAlertOrder[i+1:]...)...)
			break
		}
	}
	gt.rebuildDMAlertSection()
}

func (gt *guildsTree) reorderDMChannel(channelID discord.ChannelID) {
	if gt.dmRootNode == nil || !gt.dmRootNode.IsExpanded() {
		return
	}
	children := gt.dmRootNode.GetChildren()
	idx := -1
	for i, child := range children {
		if ref, ok := child.GetReference().(discord.ChannelID); ok && ref == channelID {
			idx = i
			break
		}
	}
	if idx <= 0 { // already first or not found
		return
	}
	node := children[idx]
	reordered := make([]*tview.TreeNode, 0, len(children))
	reordered = append(reordered, node)
	reordered = append(reordered, children[:idx]...)
	reordered = append(reordered, children[idx+1:]...)
	gt.dmRootNode.SetChildren(reordered)
}

func (gt *guildsTree) clearDMAlert(channelID discord.ChannelID) {
	if _, ok := gt.dmAlertCounts[channelID]; !ok {
		return
	}
	delete(gt.dmAlertCounts, channelID)
	for i, id := range gt.dmAlertOrder {
		if id == channelID {
			gt.dmAlertOrder = append(gt.dmAlertOrder[:i], gt.dmAlertOrder[i+1:]...)
			break
		}
	}
	gt.rebuildDMAlertSection()
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

func (gt *guildsTree) guildNodeStyle(guildID discord.GuildID) tcell.Style {
	if gt == nil || gt.chat == nil || gt.chat.state == nil {
		return tcell.StyleDefault
	}
	indication := gt.chat.state.GuildIsUnread(guildID, ningen.GuildUnreadOpts{UnreadOpts: ningen.UnreadOpts{IncludeMutedCategories: true}})
	return gt.unreadStyle(indication)
}

func (gt *guildsTree) channelNodeStyle(channel discord.Channel) tcell.Style {
	if gt == nil || gt.chat == nil || gt.chat.state == nil {
		return tcell.StyleDefault
	}
	unread := gt.unreadStyle(gt.chat.state.ChannelIsUnread(channel.ID, ningen.UnreadOpts{IncludeMutedCategories: true}))
	if channel.Type != discord.DirectMessage || len(channel.DMRecipients) != 1 {
		return unread
	}

	recipient := channel.DMRecipients[0]
	presence, err := gt.chat.state.Cabinet.Presence(discord.NullGuildID, recipient.ID)
	if err != nil {
		return ui.MergeStyle(gt.dmStatusStyle(discord.OfflineStatus), unread)
	}

	return ui.MergeStyle(gt.dmStatusStyle(presence.Status), unread)
}

func (gt *guildsTree) dmStatusStyle(status discord.Status) tcell.Style {
	switch status {
	case discord.DoNotDisturbStatus:
		return gt.cfg.Theme.GuildsTree.DNDStyle.Style
	case discord.IdleStatus:
		return gt.cfg.Theme.GuildsTree.IdleStyle.Style
	case discord.OnlineStatus:
		return gt.cfg.Theme.GuildsTree.OnlineStyle.Style
	default:
		return gt.cfg.Theme.GuildsTree.OfflineStyle.Style
	}
}

func (gt *guildsTree) createGuildNode(n *tview.TreeNode, guild discord.Guild) {
	guildNode := tview.NewTreeNode(guild.Name).
		SetReference(guild.ID).
		SetExpandable(true).
		SetExpanded(false).
		SetIndent(gt.cfg.Theme.GuildsTree.Indents.Guild)
	gt.setNodeLineStyle(guildNode, gt.guildNodeStyle(guild.ID))
	n.AddChild(guildNode)
	gt.guildNodeByID[guild.ID] = guildNode
}

func (gt *guildsTree) createChannelNode(node *tview.TreeNode, channel discord.Channel) {
	if gt != nil && gt.chat != nil && gt.chat.state != nil &&
		channel.Type != discord.DirectMessage &&
		channel.Type != discord.GroupDM &&
		channel.Type != discord.GuildCategory &&
		channel.Type != discord.GuildPublicThread &&
		channel.Type != discord.GuildPrivateThread &&
		channel.Type != discord.GuildAnnouncementThread &&
		!gt.chat.state.HasPermissions(channel.ID, discord.PermissionViewChannel) {
		return
	}

	name := channel.Name
	if name == "" && channel.Type == discord.DirectMessage && len(channel.DMRecipients) == 1 {
		name = channel.DMRecipients[0].Username
	}
	if gt != nil && gt.chat != nil && gt.chat.state != nil {
		name = ui.ChannelToString(channel, gt.cfg.Icons, gt.chat.state)
	}

	channelNode := tview.NewTreeNode(name).SetReference(channel.ID)
	gt.setNodeLineStyle(channelNode, gt.channelNodeStyle(channel))
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

func (gt *guildsTree) onSelected(node *tview.TreeNode) tview.Cmd {
	if len(node.GetChildren()) != 0 {
		switch node.GetReference().(type) {
		case discord.ChannelID:
			// Fall through to channel-loading logic below.
		default:
			node.SetExpanded(!node.IsExpanded())
			return nil
		}
	}

	switch ref := node.GetReference().(type) {
	case discord.GuildID:
		go gt.chat.state.MemberState.Subscribe(ref)

		channels, err := gt.chat.state.Cabinet.Channels(ref)
		if err != nil {
			slog.Error("failed to get channels", "err", err, "guild_id", ref)
			return nil
		}

		ui.SortGuildChannels(channels)
		gt.createChannelNodes(node, channels)
		node.Expand()
		return nil
	case discord.ChannelID:
		channel, err := gt.chat.state.Cabinet.Channel(ref)
		if err != nil {
			slog.Error("failed to get channel from state", "err", err, "channel_id", ref)
			return nil
		}

		if channel.Type == discord.GuildCategory {
			node.SetExpanded(!node.IsExpanded())
			return nil
		}

		if channel.GuildID.IsValid() && (channel.Type == discord.GuildText || channel.Type == discord.GuildAnnouncement || channel.Type == discord.GuildForum) {
			channels, err := gt.chat.state.Cabinet.Channels(channel.GuildID)
			if err == nil {
				for _, child := range channels {
					if child.ParentID != channel.ID {
						continue
					}
					switch child.Type {
					case discord.GuildPublicThread, discord.GuildPrivateThread, discord.GuildAnnouncementThread:
						if gt.findNodeByReference(child.ID) == nil {
							gt.createChannelNode(node, child)
						}
					}
				}
				// Threads are loaded as children but not auto-expanded;
				// use space to expand/collapse the thread list.
			}
		}

		// Handle forum channels differently - they contain threads, not direct messages
		if channel.Type == discord.GuildForum {
			// Get all channels from the guild - this includes active threads from GuildCreateEvent
			allChannels, err := gt.chat.state.Cabinet.Channels(channel.GuildID)
			if err != nil {
				slog.Error("failed to get channels for forum threads", "err", err, "guild_id", channel.GuildID)
				return nil
			}

			// Filter for threads that belong to this forum channel
			var forumThreads []discord.Channel
			for _, ch := range allChannels {
				if ch.ParentID == channel.ID && (ch.Type == discord.GuildPublicThread ||
					ch.Type == discord.GuildPrivateThread ||
					ch.Type == discord.GuildAnnouncementThread) {
					forumThreads = append(forumThreads, ch)
				}
			}

			// Add threads as child nodes
			for _, thread := range forumThreads {
				gt.createChannelNode(node, thread)
			}
			node.Expand()
			return nil
		}

		return gt.loadChannel(*channel)
	case dmAlertRef:
		channel, err := gt.chat.state.Cabinet.Channel(ref.channelID)
		if err != nil {
			slog.Error("failed to get channel from state", "err", err, "channel_id", ref.channelID)
			return nil
		}
		if channelNode := gt.findNodeByChannelID(ref.channelID); channelNode != nil {
			gt.SetCurrentNode(channelNode)
		}
		return gt.loadChannel(*channel)
	case dmNode: // Direct messages folder
		channels, err := gt.chat.state.PrivateChannels()
		if err != nil {
			slog.Error("failed to get private channels", "err", err)
			return nil
		}

		ui.SortPrivateChannels(channels)
		for _, c := range channels {
			gt.createChannelNode(node, c)
		}
		node.Expand()
		return nil
	}
	return nil
}

func (gt *guildsTree) loadChannel(channel discord.Channel) tview.Cmd {
	limit := uint(gt.cfg.MessagesLimit)
	return func() tview.Msg {
		messages, err := gt.chat.state.Messages(channel.ID, limit)
		if err != nil {
			slog.Error("failed to get messages", "err", err, "channel_id", channel.ID, "limit", limit)
			return nil
		}

		go gt.chat.state.ReadState.MarkRead(channel.ID, channel.LastMessageID)

		if guildID := channel.GuildID; guildID.IsValid() {
			gt.chat.messagesList.requestGuildMembers(guildID, messages)
		}
		return &channelLoadedMsg{Channel: channel, Messages: messages}
	}
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

func (gt *guildsTree) Update(msg tview.Msg) tview.Cmd {
	switch msg := msg.(type) {
	case *tview.TreeViewSelectedMsg:
		return gt.onSelected(msg.Node)
	case *tview.KeyMsg:
		handler := gt.TreeView.Update
		switch {
		case keybind.Matches(msg, gt.cfg.Keybinds.GuildsTree.CollapseParentNode.Keybind):
			if node := gt.GetCurrentNode(); node != nil && node.IsExpanded() && len(node.GetChildren()) > 0 {
				node.Collapse()
			} else {
				gt.collapseParentNode(gt.GetCurrentNode())
			}
			return nil
		case keybind.Matches(msg, gt.cfg.Keybinds.GuildsTree.MoveToParentNode.Keybind):
			return handler(tcell.NewEventKey(tcell.KeyRune, "K", tcell.ModNone))
		case keybind.Matches(msg, gt.cfg.Keybinds.GuildsTree.Up.Keybind):
			return handler(tcell.NewEventKey(tcell.KeyUp, "", tcell.ModNone))
		case keybind.Matches(msg, gt.cfg.Keybinds.GuildsTree.Down.Keybind):
			return handler(tcell.NewEventKey(tcell.KeyDown, "", tcell.ModNone))
		case keybind.Matches(msg, gt.cfg.Keybinds.GuildsTree.Top.Keybind):
			gt.Move(gt.GetRowCount() * -1)
			return nil
		case keybind.Matches(msg, gt.cfg.Keybinds.GuildsTree.Bottom.Keybind):
			gt.Move(gt.GetRowCount())
			return nil
		case keybind.Matches(msg, gt.cfg.Keybinds.GuildsTree.ToggleExpand.Keybind):
			if node := gt.GetCurrentNode(); node != nil {
				if len(node.GetChildren()) == 0 {
					// No children loaded yet — trigger onSelected to load them.
					return gt.onSelected(node)
				}
				node.SetExpanded(!node.IsExpanded())
			}
			return nil
		case keybind.Matches(msg, gt.cfg.Keybinds.GuildsTree.SelectCurrent.Keybind):
			return handler(tcell.NewEventKey(tcell.KeyEnter, "", tcell.ModNone))
		case keybind.Matches(msg, gt.cfg.Keybinds.GuildsTree.YankID.Keybind):
			return gt.yankID()
		}
		// Do not fall through to TreeView defaults for unmatched keys.
		return nil
	}
	return gt.TreeView.Update(msg)
}

func (gt *guildsTree) yankID() tview.Cmd {
	node := gt.GetCurrentNode()
	if node == nil {
		return nil
	}

	// Reference of a tree node in the guilds tree is its ID.
	// discord.Snowflake (discord.GuildID and discord.ChannelID) have the String method.
	if id, ok := node.GetReference().(fmt.Stringer); ok {
		return func() tview.Msg {
			if err := clipboardWrite(clipboard.FmtText, []byte(id.String())); err != nil {
				slog.Error("failed to copy node id", "err", err)
			}
			return nil
		}
	}
	return nil
}

func (gt *guildsTree) findNodeByReference(reference any) *tview.TreeNode {
	switch ref := reference.(type) {
	case discord.GuildID:
		return gt.guildNodeByID[ref]
	case discord.ChannelID:
		return gt.channelNodeByID[ref]
	case dmNode:
		return gt.dmRootNode
	case dmAlertRef:
		return gt.dmAlertNodeByID[ref.channelID]
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
