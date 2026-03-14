package picker

func (p *Picker) FilteredCount() int {
	return len(p.filtered)
}
