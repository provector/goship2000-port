package menus

import (
	"encoding/json"
	"gunship2000-assets/game/diagnostics"
	"time"
)

// Opt-in, host-only history. Two transition classes each emit at most once per
// process to the existing bounded background log. No input is synthesized.
type inputTraceRow struct {
	Stage, Mode, Source, Profile                                                                string
	Micros                                                                                      int64
	FrameMS, Raw                                                                                float64
	Focus, Connected, Reset, AxisAvailable, IR, KeyIR, Automatic, Ready, Fallback, Keyboard, E0 bool
	Epoch                                                                                       uint64
	Step, Compression, Slot, Target                                                             int
	Lever, Charge, EffectiveIR, Phase, Flags                                                    uint16
	Axis, Before, After, Vertical, Hold, Hover, Rotor                                           int16
	Critical, Equipment                                                                         byte
}
type flightInputTrace struct {
	start       time.Time
	rows        [96]inputTraceRow
	next, count int
	emitted     map[string]bool
	pending     string
}

func (t *flightInputTrace) add(row inputTraceRow) {
	row.Micros = time.Since(t.start).Microseconds()
	t.rows[t.next] = row
	t.next = (t.next + 1) % len(t.rows)
	if t.count < len(t.rows) {
		t.count++
	}
}
func (t *flightInputTrace) flush() {
	reason := t.pending
	t.pending = ""
	if reason == "" || t.emitted[reason] {
		return
	}
	if t.emitted == nil {
		t.emitted = make(map[string]bool)
	}
	t.emitted[reason] = true
	// Keep each emitted record below the background logger's per-record cap,
	// including JSON escaping. Only the two named transitions can flush a batch.
	for i := 0; i < t.count; i++ {
		row := t.rows[(t.next-t.count+len(t.rows)+i)%len(t.rows)]
		data, err := json.Marshal(row)
		if err == nil {
			diagnostics.Info("input.trace", "Flight input history", "reason", reason, "sample", i+1, "of", t.count, "data", string(data))
		}
	}
}
