package chat

import (
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	clipkg "github.com/ayn2op/discordo/internal/clipboard"
	"github.com/ayn2op/discordo/internal/config"
	"github.com/ayn2op/tview"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/session"
	arikawastate "github.com/diamondburned/arikawa/v3/state"
	"github.com/diamondburned/arikawa/v3/state/store"
	"github.com/diamondburned/ningen/v3"
	"github.com/gdamore/tcell/v3"
	"github.com/ncruces/zenity"
)

type trackingReadCloser struct {
	io.Reader
	closed bool
}

func (r *trackingReadCloser) Close() error {
	r.closed = true
	return nil
}

func TestNewMessageInputInitialState(t *testing.T) {
	m := newTestModel()
	mi := newMessageInput(m.cfg, m)

	if !mi.GetDisabled() {
		t.Fatal("expected newMessageInput to start disabled")
	}
	if mi.sendMessageData == nil {
		t.Fatal("expected sendMessageData to be initialized")
	}
	if mi.cache == nil {
		t.Fatal("expected autocomplete cache to be initialized")
	}
	if mi.mentionsList == nil {
		t.Fatal("expected mentions list to be initialized")
	}
}

func TestMessageInputSendAndEditBranches(t *testing.T) {
	transport := &mockTransport{}
	m := newTestModelWithTransport(transport)
	mi := m.messageInput
	mi.SetDisabled(false)

	channel := &discord.Channel{ID: 123, Type: discord.GuildText}
	m.SetSelectedChannel(channel)
	reader := &trackingReadCloser{Reader: strings.NewReader("hello file")}
	mi.attach("test.txt", reader)
	mi.SetText("hello world", true)
	mi.typingTimer = time.NewTimer(time.Hour)

	mi.send()

	if transport.method != http.MethodPost || !strings.HasSuffix(transport.path, "/channels/123/messages") {
		t.Fatalf("expected send request to POST messages endpoint, got %s %s", transport.method, transport.path)
	}
	if !reader.closed {
		t.Fatal("expected attached file reader to be closed after send")
	}
	if mi.typingTimer != nil {
		t.Fatal("expected typing timer to be cleared after send")
	}
	if mi.GetText() != "" || len(mi.sendMessageData.Files) != 0 {
		t.Fatal("expected input and attachments to reset after send")
	}

	editTransport := &mockTransport{}
	mEdit := newTestModelWithTransport(editTransport)
	miEdit := mEdit.messageInput
	miEdit.SetDisabled(false)
	mEdit.SetSelectedChannel(channel)
	mEdit.messagesList.setMessages([]discord.Message{{ID: 300, ChannelID: channel.ID, Author: discord.User{ID: 1}}})
	mEdit.messagesList.SetCursor(0)
	miEdit.edit = true
	miEdit.SetText("edited", true)

	miEdit.send()

	if editTransport.method != http.MethodPatch || !strings.Contains(editTransport.path, "/messages/300") {
		t.Fatalf("expected edit request to PATCH selected message, got %s %s", editTransport.method, editTransport.path)
	}
	if miEdit.edit {
		t.Fatal("expected edit mode to reset after a successful edit")
	}

	errTransport := &mockTransport{}
	mErr := newTestModelWithTransport(errTransport)
	miErr := mErr.messageInput
	miErr.SetDisabled(false)
	mErr.SetSelectedChannel(channel)
	miErr.edit = true
	miErr.SetText("keep me", true)

	miErr.send()

	if !miErr.edit {
		t.Fatal("expected edit mode to remain enabled when no message is selected")
	}
	if miErr.GetText() != "keep me" {
		t.Fatalf("expected text to remain unchanged on edit failure, got %q", miErr.GetText())
	}
}

func TestMessageInputExpandMentionsAndProcessText(t *testing.T) {
	m := newTestModel()
	mi := m.messageInput
	m.state.Cabinet.MeStore.MyselfSet(discord.User{ID: 1, Username: "me"}, true)

	dm := &discord.Channel{
		ID:           10,
		Type:         discord.DirectMessage,
		DMRecipients: []discord.User{{ID: 2, Username: "buddy"}},
	}

	got := string(mi.expandMentions(dm, []byte("@buddy @me @missing")))
	want := discord.UserID(2).Mention() + " " + discord.UserID(1).Mention() + " @missing"
	if got != want {
		t.Fatalf("expected DM mentions %q, got %q", want, got)
	}

	got = mi.processText(dm, []byte("`@buddy` @buddy"))
	want = "`@buddy` " + discord.UserID(2).Mention()
	if got != want {
		t.Fatalf("expected code span mention expansion %q, got %q", want, got)
	}

	guildID := discord.GuildID(50)
	guildChannel := &discord.Channel{ID: 60, GuildID: guildID, Type: discord.GuildText}
	setPermissionsForUser(m, guildID, guildChannel, discord.User{ID: 3, Username: "visible"}, discord.PermissionViewChannel)
	setPermissionsForUser(m, guildID, guildChannel, discord.User{ID: 4, Username: "hidden"}, 0)

	got = string(mi.expandMentions(guildChannel, []byte("@visible @hidden")))
	want = discord.UserID(3).Mention() + " @hidden"
	if got != want {
		t.Fatalf("expected guild mentions %q, got %q", want, got)
	}
}

func TestMessageInputTabCompleteAndSearchMember(t *testing.T) {
	m := newTestModel()
	mi := m.messageInput
	mi.SetDisabled(false)

	dm := &discord.Channel{
		ID:           10,
		Type:         discord.DirectMessage,
		DMRecipients: []discord.User{{ID: 2, Username: "bob"}},
	}
	m.SetSelectedChannel(dm)

	mi.cfg.AutocompleteLimit = 0
	mi.SetText("@bo", true)
	mi.tabComplete()
	if mi.GetText() != "@bob " {
		t.Fatalf("expected DM tab completion to replace text, got %q", mi.GetText())
	}

	mi.cfg.AutocompleteLimit = 5
	mi.SetText("@al", true)
	mi.mentionsList.append(mentionsListItem{insertText: "alice", displayText: "Alice", style: tcell.StyleDefault})
	mi.mentionsList.rebuild()
	mi.chat.ShowLayer(mentionsListLayerName)
	mi.tabComplete()
	if mi.GetText() != "@alice " {
		t.Fatalf("expected mentions list completion to replace text, got %q", mi.GetText())
	}
	if mi.mentionsList.itemCount() != 0 {
		t.Fatal("expected mentions list to clear after tab completion")
	}

	mi.SetText("plain", true)
	mi.mentionsList.append(mentionsListItem{insertText: "stale", displayText: "Stale", style: tcell.StyleDefault})
	mi.mentionsList.rebuild()
	mi.tabComplete()
	if mi.mentionsList.itemCount() != 0 {
		t.Fatal("expected non-mention tab completion to stop suggestions")
	}

	guildID := discord.GuildID(70)
	cacheKey := guildID.String() + " ab"
	mi.cache.Create(cacheKey, 1)
	zeroTime := mi.lastSearch
	mi.searchMember(guildID, "ab")
	if mi.lastSearch != zeroTime {
		t.Fatal("expected exact cache hit to skip searching")
	}

	prefixKey := guildID.String() + " a"
	mi.cache.Create(prefixKey, 0)
	mi.searchMember(guildID, "ab")
	if !mi.cache.Exists(cacheKey) {
		t.Fatal("expected prefix cache reuse to create the more specific cache key")
	}

	mi.lastSearch = time.Now()
	missKey := guildID.String() + " zz"
	mi.searchMember(guildID, "zz")
	if mi.cache.Exists(missKey) {
		t.Fatal("expected rate-limited search to avoid creating a cache entry")
	}

	guildChannel := &discord.Channel{ID: 71, GuildID: guildID, Type: discord.GuildText}
	m.SetSelectedChannel(guildChannel)
	setPermissionsForUser(m, guildID, guildChannel, discord.User{ID: 5, Username: "alice"}, discord.PermissionViewChannel)
	mi.cfg.AutocompleteLimit = 0
	mi.cache.Create(guildID.String()+" al", 1)
	mi.SetText("@al", true)
	mi.tabComplete()
	if mi.GetText() != "@alice " {
		t.Fatalf("expected guild tab completion to replace text, got %q", mi.GetText())
	}
}

func TestMessageInputHelpersAndHelp(t *testing.T) {
	m := newTestModel()
	mi := m.messageInput

	mi.addMentionUser(nil)
	if mi.mentionsList.itemCount() != 0 {
		t.Fatal("expected nil user to be ignored")
	}

	mi.addMentionUser(&discord.User{ID: 2, Username: "buddy"})
	if mi.mentionsList.itemCount() != 1 {
		t.Fatalf("expected one mention user, got %d", mi.mentionsList.itemCount())
	}

	mi.attach("a.txt", strings.NewReader("a"))
	mi.attach("b.txt", strings.NewReader("b"))
	if got := len(mi.sendMessageData.Files); got != 2 {
		t.Fatalf("expected 2 attachments, got %d", got)
	}
	if footer := mi.GetFooter(); !strings.Contains(footer, "Attached a.txt and b.txt") {
		t.Fatalf("expected attachment footer, got %q", footer)
	}

	channel := &discord.Channel{ID: 80, GuildID: 81, Type: discord.GuildText}
	m.SetSelectedChannel(channel)
	setPermissionsForUser(m, channel.GuildID, channel, discord.User{ID: 1, Username: "me"}, discord.PermissionViewChannel|discord.PermissionAttachFiles)

	short := mi.ShortHelp()
	if !containsKeybind(short, mi.cfg.Keybinds.MessageInput.OpenFilePicker.Keybind) {
		t.Fatal("expected file picker keybind when attachments are allowed")
	}
	full := mi.FullHelp()
	if !containsKeybindGroup(full, mi.cfg.Keybinds.MessageInput.OpenFilePicker.Keybind) {
		t.Fatal("expected full help to include file picker keybind")
	}

	mi.mentionsList.append(mentionsListItem{insertText: "buddy", displayText: "Buddy", style: tcell.StyleDefault})
	mi.chat.ShowLayer(mentionsListLayerName)
	short = mi.ShortHelp()
	if !containsKeybind(short, mi.cfg.Keybinds.MessageInput.Cancel.Keybind) {
		t.Fatal("expected mentions-list help to include cancel keybind")
	}
	full = mi.FullHelp()
	if !containsKeybindGroup(full, mi.cfg.Keybinds.MentionsList.Top.Keybind) {
		t.Fatal("expected mentions-list full help to include navigation keybinds")
	}

	emitted := 0
	mi.stopTabCompletion(func(cmd tview.Command) {
		if cmd != nil {
			emitted++
		}
	})
	if emitted == 0 || mi.mentionsList.itemCount() != 0 {
		t.Fatal("expected stopTabCompletion to emit commands and clear suggestions")
	}

	members := memberList{{User: discord.User{Username: "user", Discriminator: "0001"}, Nick: "nick"}}
	if members.Len() != 1 || members.String(0) == "" {
		t.Fatal("expected memberList helpers to return data")
	}

	users := userList{{Username: "user", Discriminator: "0001"}}
	if users.Len() != 1 || users.String(0) == "" {
		t.Fatal("expected userList helpers to return data")
	}
}

func TestMessageInputAddMentionMemberStyling(t *testing.T) {
	m := newTestModel()
	mi := m.messageInput
	guildID := discord.GuildID(123)
	user := discord.User{ID: 55, Username: "user"}
	roleID := discord.RoleID(77)

	m.state.Cabinet.RoleStore.RoleSet(guildID, &discord.Role{ID: roleID, Color: 0x123456}, false)
	if err := m.state.Cabinet.PresenceSet(guildID, &discord.Presence{User: user, Status: discord.OfflineStatus}, false); err != nil {
		t.Fatalf("presence set: %v", err)
	}

	member := &discord.Member{
		User:    user,
		Nick:    "nickname",
		RoleIDs: []discord.RoleID{roleID},
	}
	if mi.addMentionMember(guildID, member) {
		t.Fatal("expected a single mention to stay below the autocomplete limit")
	}
	if got := mi.mentionsList.itemCount(); got != 1 {
		t.Fatalf("expected addMentionMember to append one member, got %d", got)
	}
	if mi.addMentionMember(guildID, nil) {
		t.Fatal("expected nil member to be ignored")
	}

	item := mi.mentionsList.items[0]
	if item.insertText != user.Username {
		t.Fatalf("expected insert text %q, got %q", user.Username, item.insertText)
	}
	if item.displayText != member.Nick {
		t.Fatalf("expected display text %q, got %q", member.Nick, item.displayText)
	}
	if fg := item.style.GetForeground(); fg != tcell.NewHexColor(0x123456) {
		t.Fatalf("expected mention color %v, got %v", tcell.NewHexColor(0x123456), fg)
	}
	if attrs := item.style.GetAttributes(); attrs&tcell.AttrDim == 0 {
		t.Fatalf("expected offline mention style to be dimmed, got attrs %v", attrs)
	}
}

func TestMessageInputEmojiAndUserMentionHelpers(t *testing.T) {
	m := newTestModel()
	mi := m.messageInput
	guildID := discord.GuildID(20)
	channel := &discord.Channel{ID: 10, GuildID: guildID, Type: discord.GuildText}

	// Ensure the guild exists in cabinet
	m.state.Cabinet.GuildStore.GuildSet(&discord.Guild{ID: guildID}, false)
	m.state.Cabinet.MeStore.MyselfSet(discord.User{ID: 1, Username: "me", Nitro: discord.NitroFull}, true)

	if got := string(mi.expandEmojis(channel, []byte(":missing:"))); got != ":missing:" {
		t.Fatalf("expected unknown emoji to stay unchanged, got %q", got)
	}

	emojis := []discord.Emoji{{ID: 1, Name: "kekw"}}
	if got := string(replaceEmojis(emojis, []byte(":kekw:"))); got != "<:kekw:1>" {
		t.Fatalf("expected known emoji to expand, got %q", got)
	}

	user := &discord.User{ID: 9, Username: "buddy"}
	if err := m.state.Cabinet.PresenceSet(discord.NullGuildID, &discord.Presence{User: *user, Status: discord.OfflineStatus}, false); err != nil {
		t.Fatalf("presence set: %v", err)
	}
	mi.addMentionUser(user)
	item := mi.mentionsList.items[len(mi.mentionsList.items)-1]
	if item.insertText != user.Username || item.displayText != user.DisplayOrUsername() {
		t.Fatalf("unexpected mention user item %#v", item)
	}
	if attrs := item.style.GetAttributes(); attrs&tcell.AttrDim == 0 {
		t.Fatalf("expected offline DM mention style to be dimmed, got attrs %v", attrs)
	}
}

func TestMessageInputClipboardEditorPickerAndHandleEvent(t *testing.T) {
	oldClipboardRead := clipboardRead
	oldClipboardWrite := clipboardWrite
	oldCreateTempFile := createTempFile
	oldCreateEditorCmd := createEditorCmd
	oldRunEditorCmd := runEditorCmd
	oldSelectFileMultiple := selectFileMultiple
	oldOpenFile := openFile
	t.Cleanup(func() {
		clipboardRead = oldClipboardRead
		clipboardWrite = oldClipboardWrite
		createTempFile = oldCreateTempFile
		createEditorCmd = oldCreateEditorCmd
		runEditorCmd = oldRunEditorCmd
		selectFileMultiple = oldSelectFileMultiple
		openFile = oldOpenFile
	})

	copiedText := ""
	clipboardWrite = func(format clipkg.Format, data []byte) error {
		if format == clipkg.FmtText {
			copiedText = string(data)
		}
		return nil
	}
	clipboardRead = func(format clipkg.Format) ([]byte, error) {
		switch format {
		case clipkg.FmtText:
			return []byte("clip text"), nil
		case clipkg.FmtImage:
			return []byte("png-bytes"), nil
		default:
			return nil, nil
		}
	}

	transport := &mockTransport{}
	m := newTestModelWithTransport(transport)
	mi := m.messageInput
	mi.SetDisabled(false)
	mi.cfg.TypingIndicator.Send = false
	channel := &discord.Channel{ID: 123, Type: discord.GuildText}
	m.SetSelectedChannel(channel)

	if got := mi.GetClipboardText(); got != "clip text" {
		t.Fatalf("expected clipboard text from seam, got %q", got)
	}

	mi.SetText("copy me", true)
	mi.HandleEvent(tcell.NewEventKey(tcell.KeyCtrlL, "", tcell.ModNone))
	mi.HandleEvent(tcell.NewEventKey(tcell.KeyCtrlQ, "", tcell.ModNone))
	if copiedText != "copy me" {
		t.Fatalf("expected Ctrl+Q copy flow to write %q, got %q", "copy me", copiedText)
	}

	mi.HandleEvent(tcell.NewEventKey(tcell.KeyCtrlV, "", tcell.ModNone))
	if got := len(mi.sendMessageData.Files); got != 1 {
		t.Fatalf("expected paste to attach one file, got %d", got)
	}

	mi.SetText("hello", true)
	mi.HandleEvent(tcell.NewEventKey(tcell.KeyEnter, "", tcell.ModNone))
	if transport.method != http.MethodPost || !strings.HasSuffix(transport.path, "/channels/123/messages") {
		t.Fatalf("expected Enter to send message, got %s %s", transport.method, transport.path)
	}

	mi.SetText("@al", true)
	mi.mentionsList.append(mentionsListItem{insertText: "alice", displayText: "Alice", style: tcell.StyleDefault})
	mi.mentionsList.rebuild()
	mi.chat.ShowLayer(mentionsListLayerName)
	mi.HandleEvent(tcell.NewEventKey(tcell.KeyEnter, "", tcell.ModNone))
	if mi.GetText() != "@alice " {
		t.Fatalf("expected Enter with mentions list to tab-complete, got %q", mi.GetText())
	}

	mi.mentionsList.append(mentionsListItem{insertText: "alice", displayText: "Alice", style: tcell.StyleDefault})
	mi.mentionsList.rebuild()
	mi.chat.ShowLayer(mentionsListLayerName)
	mi.HandleEvent(tcell.NewEventKey(tcell.KeyCtrlN, "", tcell.ModNone))
	mi.HandleEvent(tcell.NewEventKey(tcell.KeyCtrlP, "", tcell.ModNone))
	mi.HandleEvent(tcell.NewEventKey(tcell.KeyHome, "", tcell.ModNone))
	mi.HandleEvent(tcell.NewEventKey(tcell.KeyEnd, "", tcell.ModNone))
	mi.HandleEvent(tcell.NewEventKey(tcell.KeyEscape, "", tcell.ModNone))
	if mi.mentionsList.itemCount() != 0 {
		t.Fatal("expected cancel to clear mentions list")
	}

	mi.SetText("reset me", true)
	mi.chat.HideLayer(mentionsListLayerName)
	mi.HandleEvent(tcell.NewEventKey(tcell.KeyEscape, "", tcell.ModNone))
	if mi.GetText() != "" {
		t.Fatalf("expected cancel to reset text, got %q", mi.GetText())
	}

	mi.SetText("undo", true)
	if cmd := mi.HandleEvent(tcell.NewEventKey(tcell.KeyCtrlU, "", tcell.ModNone)); cmd == nil {
		t.Fatal("expected undo key to delegate to text area")
	}

	tempDir := t.TempDir()
	editorPath := filepath.Join(tempDir, "editor.md")
	createTempFile = func(string, string) (*os.File, error) {
		return os.Create(editorPath)
	}
	var editedPath string
	createEditorCmd = func(_ *config.Config, path string) *exec.Cmd {
		editedPath = path
		return exec.Command("true")
	}
	runEditorCmd = func(cmd *exec.Cmd) error {
		return os.WriteFile(editedPath, []byte("edited text"), 0o644)
	}

	mi.cfg.Editor = "fake-editor"
	mi.SetText("before", true)
	mi.editor()
	if mi.GetText() != "edited text" {
		t.Fatalf("expected editor flow to reload edited file, got %q", mi.GetText())
	}

	firstFile := filepath.Join(tempDir, "first.txt")
	if err := os.WriteFile(firstFile, []byte("first"), 0o644); err != nil {
		t.Fatalf("write first file: %v", err)
	}
	selectFileMultiple = func(...zenity.Option) ([]string, error) {
		return []string{firstFile, filepath.Join(tempDir, "missing.txt")}, nil
	}
	openFile = os.Open
	beforeFiles := len(mi.sendMessageData.Files)
	mi.openFilePicker()
	if len(mi.sendMessageData.Files) != beforeFiles+1 {
		t.Fatalf("expected file picker to attach existing files only, got %d attachments", len(mi.sendMessageData.Files))
	}
}

func TestNewMessageInput_InitialStateAndClipboardCallbacks(t *testing.T) {
	oldClipboardRead := clipboardRead
	oldClipboardWrite := clipboardWrite
	t.Cleanup(func() {
		clipboardRead = oldClipboardRead
		clipboardWrite = oldClipboardWrite
	})

	var copied []byte
	clipboardWrite = func(format clipkg.Format, data []byte) error {
		if format != clipkg.FmtText {
			t.Fatalf("expected text clipboard format, got %v", format)
		}
		copied = append([]byte(nil), data...)
		return nil
	}
	clipboardRead = func(format clipkg.Format) ([]byte, error) {
		if format != clipkg.FmtText {
			t.Fatalf("expected text clipboard format, got %v", format)
		}
		return []byte("from clipboard"), nil
	}

	m := newMockChatModel()
	mi := newMessageInput(m.cfg, m)
	if !mi.GetDisabled() {
		t.Fatal("expected newMessageInput to start disabled")
	}
	if mi.sendMessageData == nil || mi.cache == nil || mi.mentionsList == nil {
		t.Fatal("expected newMessageInput to initialize send data, cache, and mentions list")
	}

	mi.SetDisabled(false)
	mi.SetText("copy me", true)
	mi.TextArea.HandleEvent(tcell.NewEventKey(tcell.KeyCtrlL, "", tcell.ModNone))
	if !mi.HasSelection() {
		t.Fatal("expected Ctrl+L to select the text before copying")
	}
	mi.TextArea.HandleEvent(tcell.NewEventKey(tcell.KeyCtrlQ, "", tcell.ModNone))
	if string(copied) != "copy me" {
		t.Fatalf("expected copy callback to receive %q, got %q", "copy me", string(copied))
	}

	if got := mi.GetClipboardText(); got != "from clipboard" {
		t.Fatalf("expected clipboard read callback %q, got %q", "from clipboard", got)
	}

	clipboardRead = func(clipkg.Format) ([]byte, error) { return nil, errors.New("read fail") }
	if got := mi.GetClipboardText(); got != "" {
		t.Fatalf("expected clipboard read error to return empty text, got %q", got)
	}
}

func TestMessageInputHandleEvent_OpenEditorAndFilePickerShortcuts(t *testing.T) {
	oldCreateTempFile := createTempFile
	oldCreateEditorCmd := createEditorCmd
	oldSelectFileMultiple := selectFileMultiple
	oldOpenFile := openFile
	t.Cleanup(func() {
		createTempFile = oldCreateTempFile
		createEditorCmd = oldCreateEditorCmd
		selectFileMultiple = oldSelectFileMultiple
		openFile = oldOpenFile
	})

	transport := &mockTransport{}
	m := newTestModelWithTransport(transport)
	mi := m.messageInput
	mi.SetDisabled(false)
	channel := &discord.Channel{ID: 123, GuildID: 456, Type: discord.GuildText}
	m.SetSelectedChannel(channel)

	tempDir := t.TempDir()
	editorCalls := 0
	editorPath := filepath.Join(tempDir, "editor.md")
	createTempFile = func(string, string) (*os.File, error) {
		editorCalls++
		return os.Create(editorPath)
	}
	createEditorCmd = func(*config.Config, string) *exec.Cmd { return nil }
	mi.cfg.Editor = "fake-editor"

	if cmd := mi.HandleEvent(tcell.NewEventKey(tcell.KeyCtrlE, "", tcell.ModNone)); cmd == nil {
		t.Fatal("expected editor shortcut to return a redraw command")
	}
	if editorCalls != 1 {
		t.Fatalf("expected editor shortcut to invoke editor flow once, got %d", editorCalls)
	}

	filePath := filepath.Join(tempDir, "attach.txt")
	if err := os.WriteFile(filePath, []byte("attachment"), 0o644); err != nil {
		t.Fatalf("write attachment: %v", err)
	}
	pickerCalls := 0
	selectFileMultiple = func(...zenity.Option) ([]string, error) {
		pickerCalls++
		return []string{filePath}, nil
	}
	openFile = os.Open

	beforeFiles := len(mi.sendMessageData.Files)
	if cmd := mi.HandleEvent(tcell.NewEventKey(tcell.KeyRune, "\\", tcell.ModCtrl)); cmd == nil {
		t.Fatal("expected file picker shortcut to return a redraw command")
	}
	if pickerCalls != 1 {
		t.Fatalf("expected file picker shortcut to invoke picker once, got %d", pickerCalls)
	}
	if len(mi.sendMessageData.Files) != beforeFiles+1 {
		t.Fatalf("expected file picker shortcut to attach a file, got %d attachments", len(mi.sendMessageData.Files))
	}
}

func TestMessageInputTabSuggestionAndTyping(t *testing.T) {
	m := newTestModel()
	mi := m.messageInput
	mi.SetDisabled(false)
	m.state.Cabinet.MeStore.MyselfSet(discord.User{ID: 1, Username: "me"}, true)

	dm := &discord.Channel{
		ID:           10,
		Type:         discord.DirectMessage,
		DMRecipients: []discord.User{{ID: 2, Username: "bob"}, {ID: 3, Username: "alice"}},
	}
	m.SetSelectedChannel(dm)
	m.state.Cabinet.MessageStore.MessageSet(&discord.Message{ID: 1, ChannelID: dm.ID, Author: discord.User{ID: 2, Username: "bob"}}, false)
	m.state.Cabinet.MessageStore.MessageSet(&discord.Message{ID: 2, ChannelID: dm.ID, Author: discord.User{ID: 3, Username: "alice"}}, false)

	mi.SetText("@", true)
	mi.tabSuggestion()
	if mi.mentionsList.itemCount() == 0 {
		t.Fatal("expected DM recent-author suggestions")
	}

	mi.SetText("plain", true)
	mi.tabSuggestion()
	if mi.mentionsList.itemCount() != 0 {
		t.Fatal("expected non-mention text to clear suggestions")
	}

	mi.SetText("@bo", true)
	mi.tabSuggestion()
	if mi.mentionsList.itemCount() == 0 {
		t.Fatal("expected DM fuzzy suggestions")
	}

	guildID := discord.GuildID(50)
	guildChannel := &discord.Channel{ID: 60, GuildID: guildID, Type: discord.GuildText}
	m.SetSelectedChannel(guildChannel)
	setPermissionsForUser(m, guildID, guildChannel, discord.User{ID: 2, Username: "bob"}, discord.PermissionViewChannel)
	setPermissionsForUser(m, guildID, guildChannel, discord.User{ID: 3, Username: "alice"}, discord.PermissionViewChannel)
	m.state.Cabinet.MessageStore.MessageSet(&discord.Message{ID: 10, ChannelID: guildChannel.ID, GuildID: guildID, Author: discord.User{ID: 2, Username: "bob"}}, false)
	m.state.Cabinet.MessageStore.MessageSet(&discord.Message{ID: 11, ChannelID: guildChannel.ID, GuildID: guildID, Author: discord.User{ID: 3, Username: "alice"}}, false)
	mi.cfg.AutocompleteLimit = 1
	mi.cache.Create(guildID.String()+" bo", 1)

	mi.SetText("@", true)
	mi.tabSuggestion()
	if mi.mentionsList.itemCount() == 0 {
		t.Fatal("expected guild recent-author suggestions")
	}

	mi.SetText("@bo", true)
	mi.tabSuggestion()
	if got := mi.mentionsList.itemCount(); got == 0 || got > 1 {
		t.Fatalf("expected limited guild fuzzy suggestions, got %d", got)
	}

	mi.stopTypingTimer()
	if mi.typingTimer != nil {
		t.Fatal("expected stopTypingTimer to leave nil timer unchanged")
	}

	mi.cfg.TypingIndicator.Send = true
	m.SetSelectedChannel(dm)
	mi.HandleEvent(tcell.NewEventKey(tcell.KeyRune, "a", tcell.ModNone))
	if mi.typingTimer == nil {
		t.Fatal("expected regular typing to arm the typing timer")
	}
	mi.stopTypingTimer()
	if mi.typingTimer != nil {
		t.Fatal("expected stopTypingTimer to clear active timer")
	}
}

func TestMessageInputErrorBranchesAndPermissions(t *testing.T) {
	oldClipboardRead := clipboardRead
	oldCreateTempFile := createTempFile
	oldCreateEditorCmd := createEditorCmd
	oldRunEditorCmd := runEditorCmd
	oldReadFile := readFile
	oldSelectFileMultiple := selectFileMultiple
	t.Cleanup(func() {
		clipboardRead = oldClipboardRead
		createTempFile = oldCreateTempFile
		createEditorCmd = oldCreateEditorCmd
		runEditorCmd = oldRunEditorCmd
		readFile = oldReadFile
		selectFileMultiple = oldSelectFileMultiple
	})

	m := newTestModel()
	mi := m.messageInput
	mi.SetDisabled(false)

	clipboardRead = func(clipkg.Format) ([]byte, error) { return nil, errors.New("clipboard fail") }
	mi.paste()
	if len(mi.sendMessageData.Files) != 0 {
		t.Fatal("expected clipboard error to avoid adding attachments")
	}

	createTempFile = func(string, string) (*os.File, error) { return nil, errors.New("temp fail") }
	mi.editor()

	tempDir := t.TempDir()
	tempPath := filepath.Join(tempDir, "editor.md")
	createTempFile = func(string, string) (*os.File, error) { return os.Create(tempPath) }
	mi.cfg.Editor = ""
	mi.SetText("original", true)
	mi.editor()
	if mi.GetText() != "original" {
		t.Fatalf("expected empty editor setting to leave text unchanged, got %q", mi.GetText())
	}

	mi.cfg.Editor = "configured"
	createEditorCmd = func(*config.Config, string) *exec.Cmd { return nil }
	mi.editor()

	createEditorCmd = func(*config.Config, string) *exec.Cmd { return exec.Command("true") }
	runEditorCmd = func(*exec.Cmd) error { return errors.New("run fail") }
	mi.editor()

	runEditorCmd = func(*exec.Cmd) error { return nil }
	readFile = func(string) ([]byte, error) { return nil, errors.New("read fail") }
	mi.editor()

	mi.openFilePicker()

	channel := &discord.Channel{ID: 1, GuildID: 2, Type: discord.GuildText}
	m.SetSelectedChannel(channel)
	selectFileMultiple = func(...zenity.Option) ([]string, error) { return nil, errors.New("picker fail") }
	mi.openFilePicker()

	setPermissionsForUser(m, channel.GuildID, channel, discord.User{ID: 1, Username: "me"}, discord.PermissionViewChannel)
	full := mi.FullHelp()
	if containsKeybindGroup(full, mi.cfg.Keybinds.MessageInput.OpenFilePicker.Keybind) {
		t.Fatal("expected file picker help to be omitted without attach permission")
	}

	noopState := ningen.FromState(arikawastate.NewFromSession(session.New(""), store.NoopCabinet))
	if channelHasUser(noopState, 123, 456) {
		t.Fatal("expected missing permissions lookup to return false")
	}
}

func TestMessageInputSearchMemberWaitPath(t *testing.T) {
	m := newTestModel()
	mi := m.messageInput
	guildID := discord.GuildID(90)
	ml := m.messagesList

	ml.fetchingMembers.mu.Lock()
	ml.fetchingMembers.value = true
	ml.fetchingMembers.done = make(chan struct{})
	ml.fetchingMembers.mu.Unlock()

	go func() {
		time.Sleep(10 * time.Millisecond)
		ml.setFetchingChunk(false, 0)
		for {
			ml.fetchingMembers.mu.Lock()
			waiting := ml.fetchingMembers.value
			ml.fetchingMembers.mu.Unlock()
			if waiting {
				ml.setFetchingChunk(false, 2)
				return
			}
			time.Sleep(time.Millisecond)
		}
	}()

	mi.searchMember(guildID, "live")
	key := guildID.String() + " live"
	if !mi.cache.Exists(key) || mi.cache.Get(key) != 2 {
		t.Fatalf("expected live search to populate cache count, got exists=%v count=%d", mi.cache.Exists(key), mi.cache.Get(key))
	}
}

func TestMessageInputClipboardCopyAndSuggestionStopBranches(t *testing.T) {
	oldClipboardRead := clipboardRead
	t.Cleanup(func() {
		clipboardRead = oldClipboardRead
	})

	m := newTestModel()
	mi := m.messageInput
	mi.SetDisabled(false)

	clipboardRead = func(clipkg.Format) ([]byte, error) { return nil, errors.New("clipboard read fail") }
	if got := mi.GetClipboardText(); got != "" {
		t.Fatalf("expected clipboard read failure to return empty text, got %q", got)
	}

	copied := stubClipboardWrite(t)
	mi.SetText("copy me", true)
	mi.Select(0, len("copy me"))
	mi.HandleEvent(tcell.NewEventKey(tcell.KeyCtrlQ, "", tcell.ModNone))
	if got := waitForCopiedText(t, copied); got != "copy me" {
		t.Fatalf("expected Ctrl+Q to copy selected text, got %q", got)
	}

	mi.mentionsList.append(mentionsListItem{insertText: "stale", displayText: "Stale", style: tcell.StyleDefault})
	m.ShowLayer(mentionsListLayerName)
	mi.SetText("plain", true)
	mi.tabSuggestion()
	if mi.mentionsList.itemCount() != 0 {
		t.Fatalf("expected non-mention input to clear suggestions, got %d items", mi.mentionsList.itemCount())
	}
	if m.GetVisible(mentionsListLayerName) {
		t.Fatal("expected non-mention input to hide the mentions list")
	}

	m.SetSelectedChannel(nil)
	mi.SetText("@user", true)
	mi.tabSuggestion()
	if mi.mentionsList.itemCount() != 0 {
		t.Fatalf("expected missing selected channel to keep suggestions empty, got %d items", mi.mentionsList.itemCount())
	}
}

func TestMessageInputSearchMemberPrefixLimitFallsThroughToLiveSearch(t *testing.T) {
	m := newTestModel()
	mi := m.messageInput
	guildID := discord.GuildID(91)
	ml := m.messagesList

	mi.cache.Create(guildID.String()+" a", mi.chat.state.MemberState.SearchLimit)

	ml.fetchingMembers.mu.Lock()
	ml.fetchingMembers.value = true
	ml.fetchingMembers.done = make(chan struct{})
	ml.fetchingMembers.mu.Unlock()

	go func() {
		time.Sleep(10 * time.Millisecond)
		ml.setFetchingChunk(false, 0)
		for {
			ml.fetchingMembers.mu.Lock()
			waiting := ml.fetchingMembers.value
			ml.fetchingMembers.mu.Unlock()
			if waiting {
				ml.setFetchingChunk(false, 3)
				return
			}
			time.Sleep(time.Millisecond)
		}
	}()

	mi.searchMember(guildID, "ab")
	key := guildID.String() + " ab"
	if !mi.cache.Exists(key) {
		t.Fatal("expected prefix cache at the search limit to fall through to a live search")
	}
	if got := mi.cache.Get(key); got != 3 {
		t.Fatalf("expected live-search cache count %d, got %d", 3, got)
	}
}
