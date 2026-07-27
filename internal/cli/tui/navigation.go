package tui

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
