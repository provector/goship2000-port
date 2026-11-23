package menus

import (
	"bytes"
	"errors"
	assetdata "gunship2000-assets/game/assets"
	gamestate "gunship2000-assets/game/state"
	"strings"
	"testing"
)

// Build a genuinely over-capacity source loadout through the ordinary fitting
// and quantity dispatcher. No definition, capacity or weight is fabricated.
func overloadedRow(t *testing.T, c *FrontEnd) [36]byte {
	t.Helper()
	s, a := c.Mission, c.Office.Assets.Missions
	for id := 0; id < 8; id++ {
		row, err := gamestate.InitializeReadinessRow(a, s.Data, id)
		if err != nil {
			t.Fatal(err)
		}
		v, err := NewOrdnanceView(a, s.Data, s.Flight, [][36]byte{row}, 0, gamestate.RankBrigadierGeneral)
		if err != nil {
			t.Fatal(err)
		}
		for station := 0; station < 3; station++ {
			best := v.Working
			weight := gamestate.RowWord(best[:], 25)
			for index, fit := range a.Preflight.Ships[id].Fits {
				if fit.Station != station {
					continue
				}
				candidate := *v
				candidate.Working = v.Working
				candidate.stack = []ordnanceMenu{{Kind: "main"}, {Kind: "fits", System: fit.System}}
				candidate.rebuild()
				for i, ch := range candidate.Choices {
					if ch.Action == "mount" && ch.Value == index {
						if _, err = candidate.Activate(i); err != nil {
							t.Fatal(err)
						}
						break
					}
				}
				if n := gamestate.RowWord(candidate.Working[:], 25); n > weight {
					best, weight = candidate.Working, n
				}
			}
			v.Working = best
		}
		for _, system := range []int{18, 16} {
			if !v.hasSystem(system) {
				continue
			}
			v.stack = []ordnanceMenu{{Kind: "main"}, {Kind: "quantity", System: system}}
			v.rebuild()
			for i, ch := range v.Choices {
				if ch.Action == "quantity" {
					if _, err = v.Activate(i); err != nil {
						t.Fatal(err)
					}
					break
				}
			}
		}
		if gamestate.PreflightRatio(gamestate.RowWord(v.Working[:], 25), gamestate.RowWord(v.Working[:], 23)) > 100 {
			return v.Working
		}
	}
	t.Fatal("source loadout fixture did not exceed capacity")
	return [36]byte{}
}

func warningStrip(t *testing.T, c *FrontEnd, frame []byte, text string) {
	t.Helper()
	want := make([]byte, 64000)
	assetdata.DrawBottomTooltip(want, c.Office.Assets.Missions.Fonts[3], text, 12)
	if !bytes.Equal(frame[191*320:], want[191*320:]) {
		t.Fatal("source font3/ink0c warning missing or wrong")
	}
}

func TestTwoSourceOverloadsRemainVisibleUntilBothCorrected(t *testing.T) {
	c := missionController(t)
	readyMission(t, c)
	s := c.Mission
	bad := overloadedRow(t, c)
	for len(s.Flight) < 5 {
		s.Flight = append(s.Flight, s.Flight[0])
	}
	s.ReadinessRows = make([][36]byte, 5)
	for i := range s.ReadinessRows {
		row, err := gamestate.InitializeReadinessRow(c.Office.Assets.Missions, s.Data, int(bad[0]))
		if err != nil {
			t.Fatal(err)
		}
		s.ReadinessRows[i] = row
	}
	good := s.ReadinessRows[1]
	s.ReadinessRows[1], s.ReadinessRows[3] = bad, bad
	c.takeAudioCues()
	x, y, ok := missionActionPoint(c, 3)
	if !ok {
		t.Fatal("settings hotspot missing")
	}
	if err := c.Handle(Actions{X: x, Y: y, Inside: true}); err != nil {
		t.Fatal(err)
	}
	warningStrip(t, c, c.Frame(false), briefingOverloadWarning)
	requireAudioCues(t, c)
	if err := c.Handle(Actions{X: x, Y: y, Inside: true, Selector1: true}); err != nil {
		t.Fatal(err)
	}
	if c.Screen != ScreenBriefing {
		t.Fatal("overload accepted")
	}
	requireAudioCues(t, c, gs4Denied)
	// Leaving the hotspot restores its original caption; returning restores the
	// current warning without a timer, stale text, or a new denial sound.
	if err := c.Handle(Actions{Inside: false}); err != nil {
		t.Fatal(err)
	}
	warningFrame := make([]byte, 64000)
	c.drawPreflightWarning(warningFrame, briefingOverloadWarning)
	if bytes.Equal(c.Frame(false)[191*320:], warningFrame[191*320:]) {
		t.Fatal("leaving briefing hotspot retained stale warning")
	}
	if err := c.openOutfit(); err != nil {
		t.Fatal(err)
	}
	for _, stage := range []struct {
		slot int
		fix  bool
	}{{1, true}, {3, true}} {
		x, y, ok = missionActionPoint(c, 6)
		if !ok {
			t.Fatal("return hotspot missing")
		}
		c.takeAudioCues()
		if err := c.Handle(Actions{X: x, Y: y, Inside: true, Selector1: true}); err != nil {
			t.Fatal(err)
		}
		if c.Screen != ScreenOutfit || !strings.Contains(s.Outfit.Message, "BEYOND CAPACITY") {
			t.Fatal("outer warning invisible", s.Outfit.Message)
		}
		warningStrip(t, c, c.Frame(false), s.Outfit.Message)
		requireAudioCues(t, c, gs4Denied)
		// Repeated redraw/idle keeps the warning; a source hover change clears it.
		for i := 0; i < 8; i++ {
			if err := c.Handle(Actions{X: x, Y: y, Inside: true}); err != nil {
				t.Fatal(err)
			}
			warningStrip(t, c, c.Frame(false), s.Outfit.Message)
		}
		// The individual screen has its own original wording and persistent guard.
		v, err := NewOrdnanceView(c.Office.Assets.Missions, s.Data, s.Flight, s.ReadinessRows, stage.slot, gamestate.RankBrigadierGeneral)
		if err != nil {
			t.Fatal(err)
		}
		warningStrip(t, c, v.Frame(), "HELICOPTER IS LOADED BEYOND CAPACITY.  Decrease load.")
		s.ReadinessRows[stage.slot] = good
		s.Outfit.Rows = s.ReadinessRows
	}
	if err := c.Handle(Actions{X: x, Y: y, Inside: true, Selector1: true}); err != nil {
		t.Fatal(err)
	}
	if c.Screen != ScreenBriefing {
		t.Fatal("corrected flight still blocked")
	}
	if err := validatePreparedFlight(s.ReadinessRows); err != nil {
		t.Fatal("both corrected", err)
	}
}

func TestPreflightWarningsDoNotReenableInternalStripDiagnostics(t *testing.T) {
	c := missionController(t)
	readyMission(t, c)
	clean := c.Frame(false)
	c.reportDiagnostic("result", "controlled post-debrief publication diagnostic", errors.New("first cause"))
	if !bytes.Equal(clean, c.Frame(false)) {
		t.Fatal("diagnostic covered source caption")
	}
	if c.showPreflightRejection(errors.New("internal resource failure")) {
		t.Fatal("untyped error became gameplay warning")
	}
	rows := [][36]byte{validFlightRow(true), validFlightRow(false)}
	for _, tc := range []struct {
		offset, value int
		text          string
	}{{25, 1010, "BEYOND CAPACITY"}, {29, 0, "NO FUEL"}, {21, 0, "UNARMED"}} {
		r := append([][36]byte(nil), rows...)
		gamestate.RowPut(r[0][:], tc.offset, tc.value)
		var e *preflightRejection
		if !errors.As(validatePreparedFlight(r), &e) || !strings.Contains(e.Text, tc.text) {
			t.Fatal(tc, e)
		}
	}
}
