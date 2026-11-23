package combat

// NotificationState is DATA272d:0478's current row and three history rows.
// Each row is kind, actor, presentation counter, argument. This is distinct
// from the four6-byte transient Result records and direct module sound IDs.
type NotificationState struct {
	Rows        [4][4]byte
	Display     [4][4]byte // Snapshot before the source consumer's E0 decrement.
	HistoryMode uint16     // Raw2d16; the separate source0688 mode-toggle binding remains unassigned.
	// LegacyO retains the recorded pre-O.1 queue domain for strictly identified
	// historical films; it is interpretation policy, not serialized game state.
	LegacyO bool `json:"-"`
}

// Recall is source0975:06BA, the documented Alt+M command. It changes only
// counters in the retained rows, not their kind/speaker/argument or queue order.
// A restored row starts at23 (not a newly queued24), including its source sound
// behavior at the later consumer. The unknown0688 input is a separate operation.
func (n *NotificationState) Recall() {
	n.Rows[0][2] = 0
	for i := 1; i < len(n.Rows); i++ {
		if n.Rows[i][2] != 0 {
			n.Rows[i][2] = 0
		} else if n.Rows[i][0] != 0 {
			n.Rows[i][2] = 23
		}
	}
}

// Queue is section1:6d94. Its duplicate scan deliberately includes rows1/2
// only, and only while the first history row's counter is nonzero.
func (n *NotificationState) Queue(kind, actor, argument int16) bool {
	if kind < 1 || kind > 22 || n.LegacyO && kind == 21 {
		return false
	} // Unmapped source pointer domains stay unsupported.
	if n.Rows[1][2] != 0 {
		for i := 1; i < 3; i++ {
			r := n.Rows[i]
			if int16(int8(r[0])) == kind && int16(int8(r[1])) == actor && int16(int8(r[3])) == argument {
				return false
			}
		}
	}
	row := [4]byte{byte(kind), byte(actor), 0, byte(argument)}
	if n.HistoryMode == 0 {
		row[2] = 24
	}
	n.Rows[3], n.Rows[2], n.Rows[1] = n.Rows[2], n.Rows[1], row
	row[1], row[2], row[3] = 0, 0, 0
	if n.HistoryMode != 0 {
		row[0], row[2] = 20, 12
	}
	n.Rows[0] = row
	return true
}

// Advance represents one reached source6e94 consumer, never an extra draw.
// Pre-counter23 may sound again when E0 isfalse; no invented one-shot latch.
func (n *NotificationState) Advance(e0, audioEnabled bool) []uint16 {
	n.Display = [4][4]byte{}
	var sound []uint16
	for i := range n.Rows {
		r := n.Rows[i]
		if r[0] == 0 || r[2] == 0 {
			continue
		}
		n.Display[i] = r
		if audioEnabled && r[2] == 23 {
			sound = append(sound, 0x14)
		}
		if e0 {
			n.Rows[i][2]--
		}
		if i == 0 {
			break
		} // Active headline hides all three history rows.
	}
	return sound
}
