package tui

type actionHint struct {
	keys  string
	label string
}

func (ui *UI) actionHints(hints ...actionHint) string {
	items := make([]string, 0, len(hints))
	for _, hint := range hints {
		items = append(items, hint.keys+" "+ui.t(hint.label))
	}
	return joinLines(items...)
}

func (ui *UI) mouseHint() string {
	if ui.width <= 0 {
		return ""
	}
	return ui.t("controls.mouse")
}

func moveIndex(index, count, delta int) int {
	if count <= 0 {
		return 0
	}
	index += delta
	if index < 0 {
		return 0
	}
	if index >= count {
		return count - 1
	}
	return index
}

func cycleIndex(index, count, delta int) int {
	if count <= 0 {
		return 0
	}
	index = (index + delta) % count
	if index < 0 {
		index += count
	}
	return index
}
