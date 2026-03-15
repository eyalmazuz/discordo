package picker

import (
	"github.com/eyalmazuz/tview"
	"github.com/eyalmazuz/tview/keybind"
	"github.com/gdamore/tcell/v3"
	"github.com/sahilm/fuzzy"
)

type (
	SelectedFunc func(item Item)
	CancelFunc   func()
)

type Picker struct {
	*tview.Flex
	input *tview.InputField
	list  *tview.List

	onSelected SelectedFunc
	onCancel   CancelFunc
	setFocus   func(p tview.Primitive)
	keyMap     *KeyMap

	items    Items
	filtered Items
}

func New() *Picker {
	p := &Picker{
		Flex:  tview.NewFlex(),
		input: tview.NewInputField(),
		list:  tview.NewList(),
	}

	// Show a horizontal bottom border to visually separate input from list.
	var borderSet tview.BorderSet
	borderSet.Bottom = tview.BoxDrawingsLightHorizontal
	borderSet.BottomLeft = borderSet.Bottom
	borderSet.BottomRight = borderSet.Bottom

	p.input.
		SetChangedFunc(p.onInputChanged).
		SetLabel("> ").
		SetBorders(tview.BordersBottom).
		SetBorderSet(borderSet).
		SetBorderStyle(tcell.StyleDefault.Dim(true))

	p.
		SetDirection(tview.FlexRow).
		// bottom border + value
		AddItem(p.input, 2, 0, true).
		AddItem(p.list, 0, 1, false)

	p.Update()
	return p
}

func (p *Picker) setFilteredItems(filtered Items) {
	p.filtered = filtered

	p.list.SetBuilder(func(index int, cursor int) tview.ListItem {
		if index < 0 || index >= len(p.filtered) {
			return nil
		}
		if p.filtered[index].Builder != nil {
			return p.filtered[index].Builder(index == cursor)
		}
		style := tcell.StyleDefault
		if index == cursor {
			style = style.Reverse(true)
		}
		line := p.filtered[index].Line
		if len(line) == 0 {
			line = tview.Line{{Text: p.filtered[index].Text, Style: style}}
		} else if index == cursor {
			line = reverseLine(line)
		}
		return tview.NewTextView().
			SetScrollable(false).
			SetWrap(false).
			SetWordWrap(false).
			SetTextStyle(style).
			SetLines([]tview.Line{line})
	})

	if len(filtered) == 0 {
		p.list.SetCursor(-1)
	} else {
		p.list.SetCursor(0)
	}
}

func reverseLine(line tview.Line) tview.Line {
	cloned := make(tview.Line, len(line))
	for i, segment := range line {
		cloned[i] = segment
		id, url := cloned[i].Style.GetUrl()
		cloned[i].Style = cloned[i].Style.Reverse(true)
		if url != "" {
			cloned[i].Style = cloned[i].Style.Url(url).UrlId(id)
		}
	}
	return cloned
}

func (p *Picker) SetKeyMap(keyMap *KeyMap) {
	p.keyMap = keyMap
}

func (p *Picker) SetFocusFunc(f func(p tview.Primitive)) {
	p.setFocus = f
}

// SetScrollBarVisibility sets when the picker's list scrollBar is rendered.
func (p *Picker) SetScrollBarVisibility(visibility tview.ScrollBarVisibility) {
	p.list.SetScrollBarVisibility(visibility)
}

// SetScrollBar sets the scrollBar primitive used by the picker's list.
func (p *Picker) SetScrollBar(scrollBar *tview.ScrollBar) {
	p.list.SetScrollBar(scrollBar)
}

func (p *Picker) SetSelectedFunc(onSelected SelectedFunc) {
	p.onSelected = onSelected
}

func (p *Picker) SetCancelFunc(onCancel CancelFunc) {
	p.onCancel = onCancel
}

func (p *Picker) ClearInput() {
	p.input.SetText("")
}

func (p *Picker) ClearList() {
	p.filtered = nil
	p.list.Clear()
}

func (p *Picker) ClearItems() {
	p.items = nil
	p.filtered = nil
}

func (p *Picker) AddItem(item Item) {
	p.items = append(p.items, item)
}

func (p *Picker) Update() {
	p.ClearInput()
	p.onInputChanged("")
}

func (p *Picker) onListSelected(index int) {
	if p.onSelected != nil {
		if index >= 0 && index < len(p.filtered) {
			item := p.filtered[index]
			p.onSelected(item)
		}
	}
}

func (p *Picker) onInputChanged(text string) {
	var fuzzied Items
	if text == "" {
		fuzzied = append(fuzzied, p.items...)
	} else {
		matches := fuzzy.FindFrom(text, p.items)
		for _, match := range matches {
			fuzzied = append(fuzzied, p.items[match.Index])
		}
	}
	p.setFilteredItems(fuzzied)
}

func (p *Picker) HandleEvent(event tcell.Event) tview.Command {
	switch event := event.(type) {
	case *tview.KeyEvent:
		if p.keyMap != nil {
			switch {
			case keybind.Matches(event, p.keyMap.ToggleFocus):
				if p.setFocus != nil {
					if p.input.HasFocus() {
						p.setFocus(p.list)
					} else {
						p.setFocus(p.input)
					}
				}
				return nil
			case keybind.Matches(event, p.keyMap.Up):
				p.list.HandleEvent(tcell.NewEventKey(tcell.KeyUp, "", tcell.ModNone))
				return nil
			case keybind.Matches(event, p.keyMap.Down):
				p.list.HandleEvent(tcell.NewEventKey(tcell.KeyDown, "", tcell.ModNone))
				return nil
			case keybind.Matches(event, p.keyMap.Top):
				p.list.SetCursor(0)
				return nil
			case keybind.Matches(event, p.keyMap.Bottom):
				p.list.SetCursor(len(p.filtered) - 1)
				return nil
			case keybind.Matches(event, p.keyMap.Select):
				p.onListSelected(p.list.Cursor())
				return nil
			case keybind.Matches(event, p.keyMap.Cancel):
				if p.onCancel != nil {
					p.onCancel()
				}
				return nil
			}
		}

		if p.list.HasFocus() && event.Key() == tcell.KeyRune {
			switch event.Str() {
			case "j":
				p.list.HandleEvent(tcell.NewEventKey(tcell.KeyDown, "", tcell.ModNone))
				return nil
			case "k":
				p.list.HandleEvent(tcell.NewEventKey(tcell.KeyUp, "", tcell.ModNone))
				return nil
			case "g":
				p.list.SetCursor(0)
				return nil
			case "G":
				p.list.SetCursor(len(p.filtered) - 1)
				return nil
			}
		}

		return p.Flex.HandleEvent(event)
	}
	return p.Flex.HandleEvent(event)
}
