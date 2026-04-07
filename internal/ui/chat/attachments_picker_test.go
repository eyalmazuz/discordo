package chat

import (
	"testing"

	"github.com/eyalmazuz/tview/layers"
	"github.com/eyalmazuz/tview/picker"
	"github.com/gdamore/tcell/v3"
)

func TestAttachmentsPickerSelectionAndHelp(t *testing.T) {
	m := newMockChatModel()
	ap := newAttachmentsPicker(m.cfg, m)
	m.messagesList.attachmentsPicker = ap

	if len(ap.ShortHelp()) == 0 {
		t.Fatal("expected short help to be populated")
	}
	if len(ap.FullHelp()) == 0 {
		t.Fatal("expected full help to be populated")
	}

	opened := 0
	ap.SetItems([]attachmentItem{
		{
			label: "one.txt",
			open: func() {
				opened++
			},
		},
	})
	m.AddLayer(ap, layers.WithName(attachmentsPickerLayerName), layers.WithVisible(true))

	ap.Update(&picker.SelectedMsg{Item: picker.Item{}})
	ap.Update(&picker.SelectedMsg{Item: picker.Item{Reference: "bad"}})
	ap.Update(&picker.SelectedMsg{Item: picker.Item{Reference: 99}})
	if opened != 0 {
		t.Fatalf("expected invalid selections to be ignored, got %d opens", opened)
	}

	executeModelCommand(m, ap.Update(&picker.SelectedMsg{Item: picker.Item{Reference: 0}}))
	if opened != 1 {
		t.Fatalf("expected valid selection to open once, got %d", opened)
	}
	if m.HasLayer(attachmentsPickerLayerName) {
		t.Fatal("expected picker layer to close after selection")
	}
	if m.app.Focused() != m.messagesList {
		t.Fatalf("expected focus to return to messages list, got %T", m.app.Focused())
	}

	m.AddLayer(ap, layers.WithName(attachmentsPickerLayerName), layers.WithVisible(true))
	ap.close()
	if m.HasLayer(attachmentsPickerLayerName) {
		t.Fatal("expected close to remove the picker layer")
	}

	ap = newAttachmentsPicker(m.cfg, m)
	setFocusForTest(m.app, ap)

	// Simulate ToggleFocus key event to trigger SetFocusFunc closure
	event := tcell.NewEventKey(tcell.KeyTab, "", tcell.ModNone)
	ap.Update(event)
}
