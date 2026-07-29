package tui

type actionDefinition struct {
	keys    []string
	display string
	label   string
}

func (ui *UI) actionHints(actions ...actionDefinition) string {
	items := make([]string, 0, len(actions))
	for _, action := range actions {
		items = append(items, action.display+" "+ui.t(action.label))
	}
	return joinLines(items...)
}

func actionMatches(value string, action actionDefinition) bool {
	for _, key := range action.keys {
		if value == key {
			return true
		}
	}
	return false
}

func (ui *UI) mouseHint() string {
	if ui.width <= 0 || ui.In != nil && accessibleMode(ui.In) {
		return ""
	}
	return ui.t("controls.mouse")
}

func moveIndex(index, count, delta int) int {
	return cycleIndex(index, count, delta)
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
