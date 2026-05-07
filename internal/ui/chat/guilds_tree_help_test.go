package chat

import (
	"testing"

	"github.com/ayn2op/tview"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/gdamore/tcell/v3"
)

func TestGuildsTreeToggleExpandHelpAndKeybind(t *testing.T) {
	m := newTestModel()
	gt := m.guildsTree

	root := tview.NewTreeNode("")
	channel := tview.NewTreeNode("general").
		SetReference(discord.ChannelID(10)).
		SetExpanded(false)
	channel.AddChild(tview.NewTreeNode("thread").SetReference(discord.ChannelID(11)))
	root.AddChild(channel)
	gt.SetRoot(root)
	gt.SetCurrentNode(channel)

	shortHelp := gt.ShortHelp()
	if !hasKeybind(shortHelp, gt.cfg.Keybinds.GuildsTree.ToggleExpand.Keybind) {
		t.Fatal("expected short help to include toggle expand for a node with children")
	}
	selectHelp := gt.cfg.Keybinds.GuildsTree.SelectCurrent.Keybind.Help()
	for _, item := range shortHelp {
		if item.Help().Key == selectHelp.Key && item.Help().Desc != "sel" {
			t.Fatalf("expected select help to remain channel selection, got %q", item.Help().Desc)
		}
	}

	if channel.IsExpanded() {
		t.Fatal("expected channel node to start collapsed")
	}
	gt.Update(tcell.NewEventKey(tcell.KeyRune, " ", tcell.ModNone))
	if !channel.IsExpanded() {
		t.Fatal("expected toggle expand keybind to expand the current node")
	}
	gt.Update(tcell.NewEventKey(tcell.KeyRune, " ", tcell.ModNone))
	if channel.IsExpanded() {
		t.Fatal("expected toggle expand keybind to collapse the current node")
	}
}
