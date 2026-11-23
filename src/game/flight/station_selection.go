package flight

import "fmt"

// NextSelectedStation compiles section1:0fa4..0fcd, the ordinary3920h Space
// handler. A station is skipped only for compact selector0 (NO BAY). EMPTY1
// and a real weapon with zero ammunition remain selectable; this does not fire.
func NextSelectedStation(current int, compact [4]byte) (int, error) {
	if current < 0 || current >= len(compact) {
		return current, fmt.Errorf("selected runtime station%d outside0..3", current)
	}
	for range compact {
		current = (current + 1) & 3
		if compact[current] != 0 {
			return current, nil
		}
	}
	// The DOS loop has no exit for four zero selectors. Valid accepted ships
	// provide a visible station; reject malformed supplied state rather than hang.
	return current, fmt.Errorf("runtime aircraft has no selectable station")
}
