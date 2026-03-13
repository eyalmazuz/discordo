package picker

import "github.com/ayn2op/tview"

type Item struct {
	Text       string
	Line       tview.Line
	Builder    func(selected bool) tview.ListItem
	FilterText string
	Reference  any
}

type Items []Item

func (is Items) String(index int) string {
	return is[index].FilterText
}

func (is Items) Len() int {
	return len(is)
}
