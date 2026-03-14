package chat

import (
	"testing"

	"github.com/ayn2op/discordo/pkg/picker"
	"github.com/ayn2op/tview/layers"
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
	m.AddLayer(ap, layers.WithName(attachmentsListLayerName), layers.WithVisible(true))

	ap.onSelected(picker.Item{})
	ap.onSelected(picker.Item{Reference: "bad"})
	ap.onSelected(picker.Item{Reference: 99})
	if opened != 0 {
		t.Fatalf("expected invalid selections to be ignored, got %d opens", opened)
	}

	ap.onSelected(picker.Item{Reference: 0})
	if opened != 1 {
		t.Fatalf("expected valid selection to open once, got %d", opened)
	}
	if m.HasLayer(attachmentsListLayerName) {
		t.Fatal("expected picker layer to close after selection")
	}
	if m.app.GetFocus() != m.messagesList {
		t.Fatalf("expected focus to return to messages list, got %T", m.app.GetFocus())
	}

	m.AddLayer(ap, layers.WithName(attachmentsListLayerName), layers.WithVisible(true))
	ap.close()
	if m.HasLayer(attachmentsListLayerName) {
		t.Fatal("expected close to remove the picker layer")
	}
}
