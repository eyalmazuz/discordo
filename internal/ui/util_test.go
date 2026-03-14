package ui

import (
	"reflect"
	"testing"
	"unsafe"

	"github.com/ayn2op/discordo/internal/config"
	"github.com/ayn2op/tview"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/ningen/v3"
	"github.com/diamondburned/ningen/v3/states/relationship"
	"github.com/diamondburned/arikawa/v3/utils/json/option"
	"github.com/gdamore/tcell/v3"
)

func TestConfigureBoxAndCentered(t *testing.T) {
	theme := &config.Theme{}
	theme.Border.Enabled = true
	theme.Border.Padding = [4]int{1, 2, 3, 4}
	theme.Border.NormalSet.BorderSet = tview.BorderSetPlain()
	theme.Border.ActiveSet.BorderSet = tview.BorderSetDouble()
	theme.Border.NormalStyle.Style = tcell.StyleDefault.Foreground(tcell.ColorBlue)
	theme.Border.ActiveStyle.Style = tcell.StyleDefault.Foreground(tcell.ColorGreen)

	box := ConfigureBox(tview.NewBox(), theme)
	if box == nil {
		t.Fatal("expected ConfigureBox to return the box")
	}
	if got := box.GetBorders(); got != tview.BordersAll {
		t.Fatalf("GetBorders() = %v, want BordersAll", got)
	}
	if got := box.GetBorderSet(); got != tview.BorderSetPlain() {
		t.Fatalf("GetBorderSet() = %v, want plain set", got)
	}

	box.Focus(nil)
	if got := box.GetBorderSet(); got != tview.BorderSetDouble() {
		t.Fatalf("GetBorderSet() after Focus = %v, want double set", got)
	}
	box.Blur()
	if got := box.GetBorderSet(); got != tview.BorderSetPlain() {
		t.Fatalf("GetBorderSet() after Blur = %v, want plain set", got)
	}

	centered := Centered(box, 10, 5)
	if _, ok := centered.(*tview.Grid); !ok {
		t.Fatalf("Centered() returned %T, want *tview.Grid", centered)
	}
}

func TestChannelToString(t *testing.T) {
	icons := config.Icons{
		GuildText:             "#",
		GuildCategory:         "cat:",
		GuildAnnouncement:     "ann:",
		GuildAnnouncementThread:"at:",
		GuildPublicThread:     "pt:",
		GuildPrivateThread:    "vt:",
		GuildVoice:            "voice:",
		GuildStageVoice:       "stage:",
		GuildForum:            "forum:",
		GuildStore:            "store:",
	}

	if got := ChannelToString(discord.Channel{Type: discord.DirectMessage, Name: "named"}, icons, nil); got != "named" {
		t.Fatalf("ChannelToString(named DM) = %q, want named", got)
	}

	dm := discord.Channel{
		Type:         discord.DirectMessage,
		DMRecipients: []discord.User{{Username: "alice"}, {Username: "bob"}},
	}
	if got := ChannelToString(dm, icons, nil); got != "alice, bob" {
		t.Fatalf("ChannelToString(DM recipients) = %q, want %q", got, "alice, bob")
	}

	group := discord.Channel{Type: discord.GroupDM, DMRecipients: []discord.User{{Username: "carol"}}}
	if got := ChannelToString(group, icons, nil); got != "carol" {
		t.Fatalf("ChannelToString(group DM) = %q, want carol", got)
	}

	stateWithFriendNick := &ningen.State{RelationshipState: &relationship.State{}}
	relationshipsField := reflect.ValueOf(stateWithFriendNick.RelationshipState).Elem().FieldByName("relationships")
	reflect.NewAt(relationshipsField.Type(), unsafe.Pointer(relationshipsField.UnsafeAddr())).Elem().Set(
		reflect.ValueOf(map[discord.UserID]discord.Relationship{
			7: {
				UserID:   7,
				Type:     discord.FriendRelationship,
				Nickname: option.NewString("bestie"),
			},
		}),
	)
	dmWithNick := discord.Channel{
		Type:         discord.DirectMessage,
		DMRecipients: []discord.User{{ID: 7, Username: "alice"}},
	}
	if got := ChannelToString(dmWithNick, icons, stateWithFriendNick); got != "bestie" {
		t.Fatalf("ChannelToString(friend nickname) = %q, want %q", got, "bestie")
	}

	if got := ChannelToString(discord.Channel{Type: discord.GuildText, Name: "general"}, icons, nil); got != "#general" {
		t.Fatalf("ChannelToString(guild text) = %q, want #general", got)
	}

	if got := ChannelToString(discord.Channel{Type: discord.GuildDirectory, Name: "mystery"}, icons, nil); got != "mystery" {
		t.Fatalf("ChannelToString(unknown type) = %q, want mystery", got)
	}

	cases := []struct {
		channel discord.Channel
		want    string
	}{
		{channel: discord.Channel{Type: discord.GuildCategory, Name: "category"}, want: "cat:category"},
		{channel: discord.Channel{Type: discord.GuildVoice, Name: "voice"}, want: "voice:voice"},
		{channel: discord.Channel{Type: discord.GuildStageVoice, Name: "stage"}, want: "stage:stage"},
		{channel: discord.Channel{Type: discord.GuildAnnouncementThread, Name: "thread"}, want: "at:thread"},
		{channel: discord.Channel{Type: discord.GuildPublicThread, Name: "thread"}, want: "pt:thread"},
		{channel: discord.Channel{Type: discord.GuildPrivateThread, Name: "thread"}, want: "vt:thread"},
		{channel: discord.Channel{Type: discord.GuildAnnouncement, Name: "news"}, want: "ann:news"},
		{channel: discord.Channel{Type: discord.GuildForum, Name: "forum"}, want: "forum:forum"},
		{channel: discord.Channel{Type: discord.GuildStore, Name: "store"}, want: "store:store"},
	}
	for _, tc := range cases {
		if got := ChannelToString(tc.channel, icons, nil); got != tc.want {
			t.Fatalf("ChannelToString(%v) = %q, want %q", tc.channel.Type, got, tc.want)
		}
	}
}

func TestSortHelpersAndMergeStyle(t *testing.T) {
	guildChannels := []discord.Channel{
		{ID: 1, Position: 2},
		{ID: 2, Position: 1},
	}
	SortGuildChannels(guildChannels)
	if guildChannels[0].ID != 2 || guildChannels[1].ID != 1 {
		t.Fatalf("SortGuildChannels() = %+v", guildChannels)
	}

	privateChannels := []discord.Channel{
		{ID: 1, LastMessageID: 10},
		{ID: 2},
		{ID: 3, LastMessageID: 30},
	}
	SortPrivateChannels(privateChannels)
	if privateChannels[0].ID != 3 || privateChannels[1].ID != 1 || privateChannels[2].ID != 2 {
		t.Fatalf("SortPrivateChannels() = %+v", privateChannels)
	}

	if got := getMessageIDFromChannel(discord.Channel{ID: 99}); got != 99 {
		t.Fatalf("getMessageIDFromChannel() = %v, want channel ID 99 when LastMessageID is invalid", got)
	}

	base := tcell.StyleDefault.Foreground(tcell.ColorRed).Background(tcell.ColorBlue).Bold(true).Underline(true)
	overlay := tcell.StyleDefault.Background(tcell.ColorGreen).Italic(true).Dim(true)
	merged := MergeStyle(base, overlay)
	if merged.GetForeground() != tcell.ColorRed {
		t.Fatalf("foreground = %v, want red", merged.GetForeground())
	}
	if merged.GetBackground() != tcell.ColorGreen {
		t.Fatalf("background = %v, want green", merged.GetBackground())
	}
	if !merged.HasBold() || !merged.HasItalic() || !merged.HasDim() || !merged.HasUnderline() {
		t.Fatalf("merged style lost attributes: %+v", merged)
	}

	fallback := MergeStyle(
		tcell.StyleDefault.Foreground(tcell.ColorYellow).Background(tcell.ColorBlack).Reverse(true).StrikeThrough(true),
		tcell.StyleDefault,
	)
	if fallback.GetForeground() != tcell.ColorYellow || fallback.GetBackground() != tcell.ColorBlack {
		t.Fatalf("expected MergeStyle to fall back to base colors, got fg=%v bg=%v", fallback.GetForeground(), fallback.GetBackground())
	}
	if !fallback.HasReverse() || !fallback.HasStrikeThrough() {
		t.Fatalf("expected MergeStyle to preserve reverse/strike-through, got %+v", fallback)
	}
}
