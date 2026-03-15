package chat

import (
	"os/exec"
	"testing"

	"github.com/ayn2op/discordo/internal/config"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/gdamore/tcell/v3"
)

func TestMessageInput_Extra(t *testing.T) {
	m := newMockChatModel()
	mi := newMessageInput(m.cfg, m)

	// Mock editor functions to avoid hangs/suspension
	oldCreateEditorCmd := createEditorCmd
	oldRunEditor := runEditorCmd
	createEditorCmd = func(cfg *config.Config, path string) *exec.Cmd {
		return exec.Command("true")
	}
	runEditorCmd = func(cmd *exec.Cmd) error {
		return nil
	}
	defer func() {
		createEditorCmd = oldCreateEditorCmd
		runEditorCmd = oldRunEditor
	}()

	t.Run("HandleEvent_KeyEvent_CtrlU", func(t *testing.T) {
		mi.SetText("some text", true)
		mi.HandleEvent(tcell.NewEventKey(tcell.KeyCtrlU, "", tcell.ModNone))
		// Ctrl+U is Undo, so text might remain if there's nothing to undo or change to previous state.
		// We just ensure no panic.
	})

	t.Run("HandleEvent_KeyEvent_Escape", func(t *testing.T) {
		mi.SetText("some text", true)
		mi.HandleEvent(tcell.NewEventKey(tcell.KeyEsc, "", tcell.ModNone))
		if mi.GetText() != "" {
			t.Errorf("Expected empty text after Escape")
		}
	})

	t.Run("HandleEvent_KeyEvent_Tab", func(t *testing.T) {
		mi.HandleEvent(tcell.NewEventKey(tcell.KeyTab, "", tcell.ModNone))
	})

	t.Run("HandleEvent_KeyEvent_Enter", func(t *testing.T) {
		mi.SetText("hello", true)
		mi.HandleEvent(tcell.NewEventKey(tcell.KeyEnter, "", tcell.ModNone))
	})

	t.Run("HandleEvent_KeyEvent_CtrlS", func(t *testing.T) {
		mi.HandleEvent(tcell.NewEventKey(tcell.KeyCtrlS, "", tcell.ModNone))
	})

	t.Run("HandleEvent_KeyEvent_CtrlE", func(t *testing.T) {
		mi.HandleEvent(tcell.NewEventKey(tcell.KeyCtrlE, "", tcell.ModNone))
	})

	t.Run("HandleEvent_KeyEvent_Up", func(t *testing.T) {
		mi.HandleEvent(tcell.NewEventKey(tcell.KeyUp, "", tcell.ModNone))
	})

	t.Run("HandleEvent_KeyEvent_Down", func(t *testing.T) {
		mi.HandleEvent(tcell.NewEventKey(tcell.KeyDown, "", tcell.ModNone))
	})
}

func TestMessageInput_Autocomplete_Branches(t *testing.T) {
	m := newMockChatModel()
	mi := newMessageInput(m.cfg, m)

	t.Run("tabSuggestion_NoTrigger", func(t *testing.T) {
		mi.SetText("hello", true)
		mi.tabSuggestion()
	})

	t.Run("tabSuggestionEmojis_Dedupe", func(t *testing.T) {
		mi.tabSuggestionEmojis(&discord.Channel{}, "kek")
	})

	t.Run("tabSuggestionEmojis_DM_Nitro", func(t *testing.T) {
		m.state.Cabinet.MeStore.MyselfSet(discord.User{Nitro: discord.NitroFull}, true)
		m.state.Cabinet.GuildStore.GuildSet(&discord.Guild{ID: 1, Name: "G1"}, false)
		m.state.Cabinet.EmojiSet(1, []discord.Emoji{
			{ID: 10, Name: "kek1"},
			{ID: 11, Name: "kek2"},
		}, false)
		m.cfg.AutocompleteLimit = 1
		mi.tabSuggestionEmojis(&discord.Channel{Type: discord.DirectMessage}, "") // name == "" -> should hit break
		mi.tabSuggestionEmojis(&discord.Channel{Type: discord.DirectMessage}, "kek") // name != "" -> should hit truncation
		mi.tabSuggestionEmojis(&discord.Channel{GuildID: 1}, "nomatch") // itemCount == 0 -> should hit early return
	})

	t.Run("tabSuggestionEmojis_Guild", func(t *testing.T) {
		m.state.Cabinet.EmojiSet(1, []discord.Emoji{
			{ID: 10, Name: "kek1"},
			{ID: 11, Name: "kek2"},
		}, false)
		m.cfg.AutocompleteLimit = 1
		mi.tabSuggestionEmojis(&discord.Channel{GuildID: 1}, "kek")
		mi.tabSuggestionEmojis(&discord.Channel{GuildID: 1}, "") // name == "" -> should hit break
	})

	t.Run("dedupeEmojisByName_Branch", func(t *testing.T) {
		emojis := []discord.Emoji{
			{Name: "kek"},
			{Name: "kek"}, // duplicate
		}
		deduped := dedupeEmojisByName(emojis)
		if len(deduped) != 1 {
			t.Errorf("expected 1 emoji, got %d", len(deduped))
		}
	})

	t.Run("tabCompleteEmoji_EarlyReturns", func(t *testing.T) {
		// itemCount == 0
		mi.mentionsList.clear()
		mi.tabCompleteEmoji(0, "kek")

		// itemCount > 0 but !ok (Cursor < 0)
		mi.mentionsList.append(mentionsListItem{insertText: "kek"})
		mi.mentionsList.SetCursor(-1)
		mi.tabCompleteEmoji(0, "kek")
	})

	t.Run("tabCompleteMention_EarlyReturns", func(t *testing.T) {
		// itemCount == 0
		mi.mentionsList.clear()
		mi.tabCompleteMention(0, "user")

		// itemCount > 0 but !ok (Cursor < 0)
		mi.mentionsList.append(mentionsListItem{insertText: "user"})
		mi.mentionsList.SetCursor(-1)
		mi.tabCompleteMention(0, "user")
	})
}
