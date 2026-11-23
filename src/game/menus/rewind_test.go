package menus

import (
	"os"
	"reflect"
	"testing"

	"gunship2000-assets/game/replay"
)

// Opt-in uses an immutable copy of the real production-generated full sortie,
// rather than assigning event records or restoring a diagnostic pose snapshot.
func TestRewindStartsInitialStateNotPreviousEvent(t *testing.T) {
	path := os.Getenv("GS2000_O2_REWIND_FILM")
	if path == "" {
		t.Skip("explicit complete production film copy opt-in")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	f, err := replay.Unmarshal(data)
	if err != nil {
		t.Fatal(err)
	}
	eventOps := []int{}
	for i, op := range f.Operations {
		if len(op.Events) != 0 {
			eventOps = append(eventOps, i)
		}
	}
	if len(eventOps) < 3 || eventOps[0] < 2 || eventOps[2]-eventOps[1] < 3 {
		t.Fatal("requires genuine separated events", eventOps)
	}
	t.Logf("real event-bearing operation indices: %v", eventOps)
	c := developmentScenarioController(t, "o-clean-flight")
	career := c.Store.State()
	positions := []int{0, eventOps[0] + 1, eventOps[1] + 1 + (eventOps[2]-eventOps[1])/2}
	for _, pointer := range []bool{false, true} {
		for _, position := range positions {
			openViewer(t, c, f)
			initial := c.Viewer.Playback.Preview().FilmCheckpoint()
			for c.Viewer.Playback.Cursor < position {
				if err = c.Viewer.Playback.Step(); err != nil {
					t.Fatal(err)
				}
			}
			old := c.Viewer.Playback
			// Rewind is restart-and-Play even when requested from Stop.
			c.Viewer.Stopped = true
			if pointer {
				err = c.Handle(Actions{Inside: true, X: 42, Y: 176, PointerPressed: true})
			} else {
				err = c.Handle(Actions{Text: []rune("r")})
			}
			if err != nil {
				t.Fatal(err)
			}
			v := c.Viewer
			if v.Playback == old || v.Playback.Cursor != 0 || v.Playback.EventCount != 0 || v.Playback.reachedSteps != 0 || v.eventCursor != 0 || len(v.events) != 0 {
				t.Fatal("did not rebuild initial stream/event owners", pointer, position)
			}
			if v.Stopped || v.Next || v.Fast || !v.AudioReset || v.Selected != 2 || v.Playback.Engine.Store != nil {
				t.Fatal("fresh playing mode/audio/capability", pointer, position)
			}
			if !reflect.DeepEqual(initial, v.Playback.Preview().FilmCheckpoint()) {
				t.Fatal("initial checkpoint differs", pointer, position)
			}
			if err = c.AdvanceReplay(.12); err != nil {
				t.Fatal(err)
			}
			if v.Playback.Cursor == 0 || v.Playback.EventCount != 0 || v.Playback.Divergence != "" || v.Playback.Cursor > eventOps[0] {
				t.Fatal("restart jumped to first event or failed ordinary Play", pointer, position, v.Playback.Cursor, v.Playback.Divergence)
			}
			t.Logf("pointer=%v prior operation=%d -> cursor0/event0/Play; ordinary advance=%d, first event=%d", pointer, position, v.Playback.Cursor, eventOps[0])
		}
	}
	if !reflect.DeepEqual(career, c.Store.State()) {
		t.Fatal("rewind changed career")
	}
	after, err := os.ReadFile(path)
	if err != nil || replay.BytesDigest(after) != replay.BytesDigest(data) {
		t.Fatal("rewind changed source film", err)
	}
}
