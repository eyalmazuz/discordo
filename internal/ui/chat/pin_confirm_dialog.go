package chat

import (
	"github.com/ayn2op/discordo/internal/config"
	"github.com/ayn2op/discordo/internal/ui"
	"github.com/eyalmazuz/tview"
	"github.com/eyalmazuz/tview/layers"
	"github.com/gdamore/tcell/v3"
)

const (
	pinConfirmPrompt = "Do you want to pin this message"
	pinConfirmHelper = "please verify again that this is the message you want to pin"
)

type messageConfirmDialog struct {
	*tview.Flex
	form *tview.Form
}

func newMessageConfirmDialog(cfg *config.Config, prompt string, helper string, previewLines []tview.Line) *messageConfirmDialog {
	headerText := prompt
	if helper != "" {
		headerText += "\n" + helper
	}

	header := tview.NewTextView().
		SetText(headerText).
		SetTextAlign(tview.AlignmentCenter).
		SetWrap(true).
		SetWordWrap(true).
		SetScrollable(false)

	preview := tview.NewTextView().
		SetWrap(true).
		SetWordWrap(true).
		SetScrollable(false).
		SetLines(previewLines)

	form := tview.NewForm().
		SetButtonsAlignment(tview.AlignmentCenter)
	form.
		AddButton("yes").
		AddButton("no").
		SetFocus(0)

	dialog := &messageConfirmDialog{
		Flex: tview.NewFlex().
			SetDirection(tview.FlexRow).
			AddItem(header, 4, 0, false).
			AddItem(preview, 0, 1, false).
			AddItem(form, 3, 0, true),
		form: form,
	}

	dialog.Box = ui.ConfigureBox(dialog.Box, &cfg.Theme)
	dialog.
		SetBlurFunc(nil).
		SetFocusFunc(nil).
		SetBorderSet(cfg.Theme.Border.ActiveSet.BorderSet).
		SetBorderStyle(cfg.Theme.Border.ActiveStyle.Style).
		SetTitleStyle(cfg.Theme.Title.ActiveStyle.Style).
		SetFooterStyle(cfg.Theme.Footer.ActiveStyle.Style)

	bg := cfg.Theme.Dialog.Style.GetBackground()
	if bg != tcell.ColorDefault {
		dialog.SetBackgroundColor(bg)
		header.SetBackgroundColor(bg)
		preview.SetBackgroundColor(bg)
		form.SetBackgroundColor(bg)
	}

	buttonStyle := cfg.Theme.Dialog.Style.Style
	fg := cfg.Theme.Dialog.Style.GetForeground()
	if fg != tcell.ColorDefault {
		header.SetTextStyle(tcell.StyleDefault.Foreground(fg))
		buttonStyle = buttonStyle.Foreground(fg)
	}
	if bg != tcell.ColorDefault {
		buttonStyle = buttonStyle.Background(bg)
	}
	form.SetButtonStyle(buttonStyle)
	form.SetButtonActivatedStyle(buttonStyle.Reverse(true))

	return dialog
}

func (d *messageConfirmDialog) Focus(delegate func(p tview.Primitive)) {
	if delegate == nil {
		return
	}
	delegate(d.form)
}

func (d *messageConfirmDialog) HasFocus() bool {
	return d.form.HasFocus()
}

func (d *messageConfirmDialog) HandleEvent(event tcell.Event) tview.Command {
	switch event := event.(type) {
	case *tview.FormSubmitEvent:
		return func() tcell.Event {
			return &tview.ModalDoneEvent{
				ButtonIndex: event.ButtonIndex,
				ButtonLabel: event.ButtonLabel,
			}
		}
	case *tview.FormCancelEvent:
		return func() tcell.Event {
			return &tview.ModalDoneEvent{ButtonIndex: -1, ButtonLabel: ""}
		}
	}
	return d.form.HandleEvent(event)
}

func (v *Model) showMessageConfirmDialog(prompt string, helper string, previewLines []tview.Line, onDone func(label string)) {
	v.confirmModalPreviousFocus = v.app.GetFocus()
	v.confirmModalDone = onDone

	dialog := newMessageConfirmDialog(v.cfg, prompt, helper, previewLines)
	v.
		AddLayer(
			ui.Centered(dialog, max(v.cfg.Picker.Width, 72), max(v.cfg.Picker.Height, 16)),
			layers.WithName(confirmModalLayerName),
			layers.WithResize(true),
			layers.WithVisible(true),
			layers.WithOverlay(),
		).
		SendToFront(confirmModalLayerName)
	v.app.SetFocus(dialog)
}

func (v *Model) showPinConfirmDialog(previewLines []tview.Line, onDone func(label string)) {
	v.showMessageConfirmDialog(pinConfirmPrompt, pinConfirmHelper, previewLines, onDone)
}
