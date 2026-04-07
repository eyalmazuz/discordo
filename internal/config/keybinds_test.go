package config

import "testing"

func TestKeybindUnmarshalTOML(t *testing.T) {
	var single Keybind
	if err := single.UnmarshalTOML(" ctrl+k "); err != nil {
		t.Fatalf("UnmarshalTOML(string) error = %v", err)
	}
	if got := single.Keys(); len(got) != 1 || got[0] != "ctrl+k" {
		t.Fatalf("single.Keys() = %v, want [ctrl+k]", got)
	}
	if help := single.Help(); help.Key != "ctrl+k" {
		t.Fatalf("single.Help().Key = %q, want ctrl+k", help.Key)
	}

	var multi Keybind
	multi.SetHelp("old", "desc")
	if err := multi.UnmarshalTOML([]any{" ctrl+p ", 7, "ctrl+n"}); err != nil {
		t.Fatalf("UnmarshalTOML([]any) error = %v", err)
	}
	if got := multi.Keys(); len(got) != 2 || got[0] != "ctrl+p" || got[1] != "ctrl+n" {
		t.Fatalf("multi.Keys() = %v, want [ctrl+p ctrl+n]", got)
	}
	if help := multi.Help(); help.Key != "ctrl+p" || help.Desc != "desc" {
		t.Fatalf("multi.Help() = %+v, want key ctrl+p and desc desc", help)
	}

	var unsupported Keybind
	if err := unsupported.UnmarshalTOML(123); err != nil {
		t.Fatalf("UnmarshalTOML(unsupported) error = %v", err)
	}
	if len(unsupported.Keys()) != 0 {
		t.Fatalf("unsupported.Keys() = %v, want empty", unsupported.Keys())
	}
}

func TestDefaultKeybindFactories(t *testing.T) {
	keybind := newKeybind("ctrl+x", "example")
	if got := keybind.Keys(); len(got) != 1 || got[0] != "ctrl+x" {
		t.Fatalf("newKeybind keys = %v, want [ctrl+x]", got)
	}
	if help := keybind.Help(); help.Key != "ctrl+x" || help.Desc != "example" {
		t.Fatalf("newKeybind help = %+v", help)
	}

	picker := defaultPickerKeybinds()
	if picker.Up.Help().Desc != "up" || picker.ToggleFocus.Help().Key != "tab" {
		t.Fatalf("unexpected picker defaults: %+v", picker)
	}

	nav := defaultNavigationKeybinds()
	if nav.Top.Help().Key != "g" || nav.Bottom.Help().Key != "G" {
		t.Fatalf("unexpected navigation defaults: %+v", nav)
	}

	guilds := defaultGuildsTreeKeybinds()
	if guilds.ToggleExpand.Help().Key != "space" || guilds.CollapseParentNode.Help().Key != "-" {
		t.Fatalf("unexpected guild defaults: %+v", guilds)
	}

	messages := defaultMessagesListKeybinds()
	if messages.React.Help().Key != "+" || messages.DeleteConfirm.Help().Key != "d" {
		t.Fatalf("unexpected messages defaults: %+v", messages)
	}

	input := defaultMessageInputKeybinds()
	if input.OpenEditor.Help().Key != "ctrl+e" || input.OpenFilePicker.Help().Key != "ctrl+\\" {
		t.Fatalf("unexpected input defaults: %+v", input)
	}

	mentions := defaultMentionsListKeybinds()
	if mentions.Up.Help().Key != "ctrl+p" || mentions.Down.Help().Key != "ctrl+n" {
		t.Fatalf("unexpected mentions defaults: %+v", mentions)
	}

	all := defaultKeybinds()
	if all.ToggleMessageSearch.Help().Key != "ctrl+f" {
		t.Fatalf("ToggleMessageSearch help key = %q, want ctrl+f", all.ToggleMessageSearch.Help().Key)
	}
	if all.MessagesList.React.Help().Key != "+" {
		t.Fatalf("MessagesList.React help key = %q, want +", all.MessagesList.React.Help().Key)
	}
	if all.MessageInput.OpenFilePicker.Help().Key != "ctrl+\\" {
		t.Fatalf("MessageInput.OpenFilePicker help key = %q, want ctrl+\\\\", all.MessageInput.OpenFilePicker.Help().Key)
	}
}
