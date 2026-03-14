package chat

import (
	"testing"

	"github.com/ayn2op/discordo/pkg/picker"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
)

func TestAttachmentsPicker_Extra(t *testing.T) {
	m := newMockChatModel()
	ap := newAttachmentsPicker(m.cfg, m)

	t.Run("onSelected_Image", func(t *testing.T) {
		opened := false
		ap.SetItems([]attachmentItem{
			{
				label: "image.png",
				open: func() {
					opened = true
				},
			},
		})
		ap.onSelected(picker.Item{Reference: 0})
		if !opened {
			t.Errorf("expected open function to be called")
		}
	})
}

func TestReactionPicker_Extra(t *testing.T) {
	m := newMockChatModel()
	ml := m.messagesList
	rp := newReactionPicker(m.cfg, m, ml, ml.imageCache)

	t.Run("onSelected_InvalidReference", func(t *testing.T) {
		rp.onSelected(picker.Item{Reference: "invalid"})
		rp.onSelected(picker.Item{Reference: -1})
		rp.onSelected(picker.Item{Reference: 100})
	})

	t.Run("onSelected_NoSelectedMessage", func(t *testing.T) {
		rp.items = []discord.Emoji{{Name: "test"}}
		rp.onSelected(picker.Item{Reference: 0})
	})
}

func TestMessageSearchPopup_Extra(t *testing.T) {
	m := newMockChatModel()
	sp := newMessageSearchPopup(m.cfg, m, m.messagesList)

	t.Run("Prepare_Branches", func(t *testing.T) {
		ch := discord.Channel{ID: 123, Name: "chan"}
		sp.Prepare(ch, nil)
	})

	t.Run("search_InvalidChannel", func(t *testing.T) {
		sp.channel = discord.Channel{}
		sp.search()
	})

	t.Run("onInputChanged_SameQuery", func(t *testing.T) {
		sp.lastSubmitted = "query"
		sp.onInputChanged("query")
	})

	t.Run("setResults_Empty", func(t *testing.T) {
		sp.setResults(nil)
	})

	t.Run("buildItem_EmptyResults", func(t *testing.T) {
		sp.results = nil
		sp.status = ""
		if sp.buildItem(0, 0) != nil {
			t.Errorf("Expected nil for empty results and no status")
		}
		
		sp.status = "Searching..."
		if sp.buildItem(0, 0) == nil {
			t.Errorf("Expected status item")
		}
		if sp.buildItem(1, 0) != nil {
			t.Errorf("Expected nil for index > 0")
		}
	})
}

func TestModel_onGuildMembersChunk(t *testing.T) {
	m := newMockChatModel()
	m.onGuildMembersChunk(&gateway.GuildMembersChunkEvent{
		Members: []discord.Member{{User: discord.User{ID: 1}}},
	})
}
