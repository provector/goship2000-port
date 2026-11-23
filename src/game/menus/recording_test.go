package menus

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gunship2000-assets/game/configuration"
	"gunship2000-assets/game/flight"
	"gunship2000-assets/game/replay"
)

func TestRecordingFirstCauseStopsRecorderOnlyWork(t *testing.T) {
	c := developmentScenarioController(t, "o-short-campaign")
	p := c.Rendering
	r := p.filmRecorder
	r.failRecording("append", os.ErrPermission)
	first := r.Error
	n := r.operationCount()
	ordinal := r.Ordinal
	// Prior pending flush must not create a new pending operation after failure.
	c.filmBeginStep(flight.Timing{}, false, 0, false, nil)
	if r.Pending != nil || r.Ordinal != ordinal || r.operationCount() != n || r.Error != first {
		t.Fatal("recorder continued or lost first cause")
	}
	proposal := campaignNaturalBaseEnd(t, c)
	if _, e := c.SealRecordedFilm(); e == nil {
		t.Fatal("failed recorder returned playable film")
	}
	if c.Debrief == nil || c.Debrief.Pending == nil {
		t.Fatal("recording failure blocked valid result", c.Screen, c.Message)
	}
	ackToLibrary(t, c)
	if c.Store.State().SelectedPilot().Statistics.LastMissionScore != proposal.Score.Score {
		t.Fatal("recording failure lost career")
	}
}

// Optional heavy integration: every byte is emitted by reached production GPS
// keyboard consumers and actual ending. No padding, forged checkpoints or End.
// This stress input is not human continuous-flight acceptance.
func TestLargeP4ProductionRecording(t *testing.T) {
	out := os.Getenv("GS2000_P4_LARGE_OUTPUT")
	if out == "" {
		t.Skip("explicit fresh large-recording output required")
	}
	if e := os.Mkdir(out, 0700); e != nil {
		t.Fatal(e)
	}
	c := renderingController(t)
	startRendering(t, c)
	jFinishFade(t, c)
	p := c.Rendering
	if e := c.openGPS(); e != nil {
		t.Fatal(e)
	}
	start := time.Now()
	count := 0
	for p.filmRecorder.Spool.Bytes() < 70<<20 {
		// Actual independent70Hz write-capable modal updates; keep cursor bounded
		// using real reversal/release and source ramp/filter, not state assignment.
		if count%40 == 0 {
			held := [4]bool{}
			held[(count/40)%2] = true
			if e := c.handleGPS(Actions{SceneDirections: held}); e != nil {
				t.Fatal(e)
			}
		}
		if e := c.tickGPS(); e != nil {
			t.Fatal(e)
		}
		count++
		if p.filmRecorder.Error != "" {
			t.Fatal(p.filmRecorder.Error)
		}
		if count > 150000 {
			t.Fatal("size target not reached", p.filmRecorder.Spool.Bytes())
		}
	}
	c.closeGPS()
	endFilm(t, c)
	f, e := c.SealRecordedFilm()
	if e != nil {
		t.Fatal(e)
	}
	b, e := replay.Marshal(f)
	if e != nil {
		t.Fatal(e)
	}
	if len(b) <= 64<<20 || len(b) > 128<<20 {
		t.Fatal("actual framed size", len(b))
	}
	if e = os.WriteFile(filepath.Join(out, "large.g2r"), b, 0600); e != nil {
		t.Fatal(e)
	}
	f, e = replay.Unmarshal(b)
	if e != nil {
		t.Fatal(e)
	}
	recordTime := time.Since(start)
	verifyStart := time.Now()
	// Playback has its own ceiling. A lower new-recording setting cannot strand it.
	c.RecordingLimit = 64 << 20
	q, e := NewFilmPlayback(c.Office.Assets, f, configuration.Defaults())
	if e != nil {
		t.Fatal(e)
	}
	for !q.Done {
		if e = q.Step(); e != nil {
			t.Fatal(e)
		}
	}
	report, _ := json.MarshalIndent(map[string]any{"bytes": len(b), "operations": len(f.Operations), "steps": f.End.Steps, "gps_ticks": count, "film_sha256": replay.BytesDigest(b), "record_marshal_seconds": recordTime.Seconds(), "playback_seconds": time.Since(verifyStart).Seconds(), "new_recording_limit": int64(c.RecordingLimit), "read_ceiling": replay.MaxBytes, "label": "Reached production keyboard GPS stress and ordinary generated ending; no padding/no invented suffix; not human continuous flight"}, "", "  ")
	if e = os.WriteFile(filepath.Join(out, "result.json"), report, 0600); e != nil {
		t.Fatal(e)
	}
}
