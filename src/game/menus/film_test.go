package menus

import (
	"encoding/json"
	"gunship2000-assets/game/configuration"
	"gunship2000-assets/game/flight"
	"gunship2000-assets/game/replay"
	gamestate "gunship2000-assets/game/state"
	"os"
	"path/filepath"
	"testing"
)

func TestGPSConsumedTicksAndModalCheckpoint(t *testing.T) {
	c := developmentScenarioController(t, "o-combat")
	p := c.Rendering
	c.Handle(Actions{Cancel: true})
	c.Handle(Actions{})
	if e := c.AdvanceRendering(.12); e != nil {
		t.Fatal(e)
	}
	if p.Flight.Steps != 1 {
		t.Fatal("minimal source fixture needs one reached step")
	}
	for _, a := range []Actions{{GPSOpen: true}, {GPSKey: "f"}, {SceneDirections: [4]bool{false, true, false, false}}} {
		if e := c.Handle(a); e != nil {
			t.Fatal(e)
		}
	}
	if p.GPS == nil {
		t.Fatal("GPS missing")
	}
	before := p.GPS.CursorX
	steps, rng := p.Flight.Steps, p.Initialized.RandomState
	n := p.filmRecorder.operationCount()
	for i := 0; i < 30; i++ {
		if e := c.tickGPS(); e != nil {
			t.Fatal(e)
		}
	}
	if p.GPS.CursorX != before+47 || p.Flight.Steps != steps || p.Initialized.RandomState != rng {
		t.Fatal("source ramp/filter displacement or modal time", before, p.GPS.CursorX)
	}
	if p.filmRecorder.operationCount() != n+30 {
		t.Fatal("actual consumed GPS ticks omitted")
	}
	for _, a := range []Actions{{Selector1: true}, {ViewKey: 1}} {
		if e := c.Handle(a); e != nil {
			t.Fatal(e)
		}
	}
	endFilm(t, c)
	f := verifyFilm(t, c)
	if f.Header.InputSchema != replay.R1InputCompatibility || f.Operations[n].Checkpoint["modal"] == "" {
		t.Fatal("new typed modal profile missing")
	}
}

// A real player's normalized control schedule is an authored input fixture here,
// not a finished proposal. Current preparation/physics/weapons/objectives/service
// create every new fact. Extra keyboard UI visits are actually consumed/recorded.
func TestFullMissionNewRecording(t *testing.T) {
	input, out := os.Getenv("GS2000_O1_FULL_INPUT"), os.Getenv("GS2000_O1_FULL_OUTPUT")
	if input == "" || out == "" {
		t.Skip("opt-in preserved full-sortie input and fresh output")
	}
	if e := os.Mkdir(out, 0700); e != nil {
		t.Fatal(e)
	}
	data, e := os.ReadFile(input)
	if e != nil {
		t.Fatal(e)
	}
	old, e := replay.Unmarshal(data)
	if e != nil {
		t.Fatal(e)
	}
	c := developmentScenarioController(t, "o-combat")
	p, r := c.Rendering, c.Rendering.Flight
	c.Handle(Actions{Cancel: true})
	c.Handle(Actions{})
	ticks := 0
	for i, op := range old.Operations {
		if i == 2 && os.Getenv("GS2000_P4_FULL_LARGE") == "1" {
			if e := c.openGPS(); e != nil {
				t.Fatal(e)
			}
			for n := 0; p.filmRecorder.Spool.Bytes() < 70<<20; n++ {
				if n%40 == 0 {
					held := [4]bool{}
					held[(n/40)%2] = true
					if e := c.handleGPS(Actions{SceneDirections: held}); e != nil {
						t.Fatal(e)
					}
				}
				if e := c.tickGPS(); e != nil {
					t.Fatal(e)
				}
				if p.filmRecorder.Error != "" {
					t.Fatal(p.filmRecorder.Error)
				}
				if n > 100000 {
					t.Fatal("large fullrecording target not reached")
				}
			}
			if e := c.handleGPS(Actions{ViewKey: 1}); e != nil {
				t.Fatal(e)
			}
		}
		switch op.Kind {
		case "step", "terminal":
			var v filmStep
			if e := json.Unmarshal(op.Payload, &v); e != nil {
				t.Fatal(e)
			}
			a := Actions{SceneDirections: v.Input.Directions, WeaponTriggerValid: true, WeaponTriggerHeld: v.Input.Trigger, WeaponTriggerPressed: v.Input.Pressed, DefenseHeldValid: true, FlightHeldValid: true}
			if v.Input.HasCommand {
				a.FlightCommands = []int{v.Input.Command}
			}
			for _, x := range v.Input.Aux {
				switch x {
				case 0, 1, 2, 3:
					a.DefenseCommands = append(a.DefenseCommands, x)
				case 4:
					a.Backspace = true
				case 5:
					a.WaypointNext = true
				case 6:
					a.WaypointPrevious = true
				case 7:
					a.OrdnanceNext = true
				case 8:
					a.RocketSalvo = 1
				case 9:
					a.RocketSalvo = 2
				case 10:
					a.RocketSalvo = 4
				}
			}
			r.commandInput.SampleAt(r.commandInput.elapsed, a)
			timing := r.Clock.ReplayTiming()
			if timing != v.Timing {
				t.Fatal("authored logical schedule mismatch", i)
			}
			r.filmTiming = &timing
			r.Paused = false
			p.Paused = false
			if e := c.advanceFlight(0); e != nil {
				t.Fatal(i, e)
			}
		case "boundary":
			var b filmBoundary
			if e := json.Unmarshal(op.Payload, &b); e != nil {
				t.Fatal(e)
			}
			if i == 995 {
				// Reach the known planned endpoint using ordinary held keyboard inputs,
				// never assigning cursor/route/pose or completed objective bits.
				for n := 0; n < 1000 && (p.GPS.CursorX != 31 || p.GPS.CursorY != 130); n++ {
					held := [4]bool{p.GPS.CursorX > 31, p.GPS.CursorX < 31, p.GPS.CursorY > 130, p.GPS.CursorY < 130}
					if e := c.handleGPS(Actions{SceneDirections: held}); e != nil {
						t.Fatal(e)
					}
					if e := c.tickGPS(); e != nil {
						t.Fatal(e)
					}
					ticks++
				}
				if p.GPS.CursorX != 31 || p.GPS.CursorY != 130 {
					t.Fatal("keyboard route did not reach supplied endpoint", p.GPS.CursorX, p.GPS.CursorY)
				}
			}
			// Semantic boundary consumer records its actual result through live hooks.
			var dispatchErr error
			if b.Kind == "control" {
				dispatchErr = c.handleFlight(b.Action)
			} else {
				dispatchErr = c.applyFilmBoundary(b)
			}
			if e := dispatchErr; e != nil {
				t.Fatal(i, e)
			}
			if b.Kind == "reset" { /* clearFlightInput records the source reset itself. */
			}
		case "end":
			if p.Initialized.Sortie.Pending == "" {
				t.Fatal("no real base/exit end prompt", p.Initialized.Player)
			}
			if c.Screen != ScreenQuit && !c.presentNaturalSortieEnd() {
				t.Fatal("end prompt not delivered")
			}
			if e := c.Handle(Actions{ConfirmQuit: true}); e != nil {
				t.Fatal(e)
			}
		}
		if p.filmRecorder.Error != "" {
			t.Fatal("recording at source input operation", i, p.filmRecorder.Error)
		}
	}
	f, e := c.SealRecordedFilm()
	if e != nil {
		t.Fatal(e)
	}
	if f == nil || f.End.Reason != "base" || f.End.Summary == nil || !f.End.Summary.Primary || !f.End.Summary.Secondary {
		t.Fatalf("actual complete mission absent: %+v", f.End)
	}
	if r.ElapsedDA != 5985 {
		t.Fatal("independent DF/DC/DA recurrence", r.ElapsedDA)
	}
	// Preserve independently inspectable facts before any final acknowledgement.
	var reachedProposal *gamestate.SortieProposal
	if c.Debrief != nil && c.Debrief.Pending != nil {
		reachedProposal = c.Debrief.Pending.Proposal
		raw, _ := json.MarshalIndent(c.Debrief.Pending.Proposal, "", "  ")
		os.WriteFile(filepath.Join(out, "proposal.json"), raw, 0600)
	}
	// Acknowledge actual proposal through existing page/controller transactions.
	for n := 0; n < 80 && c.Screen == ScreenDebrief; n++ {
		c.Debrief.Tick = 400
		if c.Debrief.Page == "crew" && c.Debrief.Pending.Proposal.Result != nil {
			c.Debrief.CrewSelection = len(c.Debrief.Pending.Proposal.Result.Recommendations)
		}
		if e := c.Handle(Actions{Enter: true}); e != nil {
			t.Fatal(e)
		}
	}
	if c.Screen != ScreenFilms {
		t.Fatal("finalack did not reach library", c.Screen)
	}
	if !f.P4Mechanics() && f.End.Score != 433 {
		t.Fatal("historical source facts score changed", f.End.Score)
	}
	if f.P4Mechanics() {
		// New retained-target/reference mechanics can change which additional
		// objects this input schedule destroys. Keep the433 historical oracle above;
		// independently check the reached source finalizer/score arithmetic here.
		if reachedProposal == nil {
			t.Fatal("missing actual pending facts")
		}
		facts := reachedProposal.Facts
		var raw int16
		for i, k := range facts.ResultContext.DynamicKinds {
			if k&0x40 == 0 && k == 0x1c {
				raw += facts.ObjectValues[i]
			}
		}
		if facts.RawScore != raw || reachedProposal.Score.AfterObjectCredit != raw || reachedProposal.Score.AfterObjectiveBonus != raw+150 || reachedProposal.Score.PreScale != int32(raw)+200 || int32(f.End.Score) != (int32(raw)+200)*9/10 {
			t.Fatal("independent source finalizer/goal/aircraft/coefficient arithmetic", raw, reachedProposal.Score)
		}
	}

	encoded, e := replay.Marshal(f)
	if e != nil {
		t.Fatal(e)
	}
	if os.Getenv("GS2000_P4_FULL_LARGE") == "1" && (len(encoded) <= 64<<20 || len(encoded) > 128<<20) {
		t.Fatal("large fullmission size", len(encoded))
	}
	if e = os.WriteFile(filepath.Join(out, "full-new.g2r"), encoded, 0600); e != nil {
		t.Fatal(e)
	}
	for _, opt := range []configuration.RuntimeOptions{configuration.Defaults(), func() configuration.RuntimeOptions {
		o := configuration.Defaults()
		o.SimulationRenderScale = 2
		o.PresentationFPS = 30
		o.PresentationMode = "stepped"
		return o
	}()} {
		q, e := NewFilmPlayback(c.Office.Assets, f, opt)
		if e != nil {
			t.Fatal(e)
		}
		for !q.Done {
			if e = q.Step(); e != nil {
				t.Fatal(e)
			}
		}
	}
	report, _ := json.MarshalIndent(map[string]any{"source_input_film_sha256": replay.BytesDigest(data), "new_film_sha256": replay.BytesDigest(encoded), "steps": f.End.Steps, "bytes": len(encoded), "consumed_extra_keyboard_gps_ticks": ticks, "summary": f.End.Summary, "score": f.End.Score, "screen": c.Screen, "label": "Authored normalized control schedule from actual human sortie, genuine current production preparation/mechanics, no teleports or pre-completed facts; not a new human flight"}, "", "  ")
	os.WriteFile(filepath.Join(out, "result.json"), report, 0600)
	_ = flight.NativeClockPolicy
}

func TestGPSRecordingTransparentAndTamperRejected(t *testing.T) {
	c := developmentScenarioController(t, "o-combat")
	var launch filmLaunch
	if e := json.Unmarshal(c.Rendering.filmRecorder.Film.Launch, &launch); e != nil {
		t.Fatal(e)
	}
	final, e := cloneFilmFinal(&launch.Final)
	if e != nil {
		t.Fatal(e)
	}
	d := &FrontEnd{Office: &Office{Assets: c.Office.Assets}, Runtime: launch.Runtime, filmIsolation: &filmSessionIsolation{Mode: "practice"}, Screen: ScreenScene, Mission: &MissionSession{Data: final.Mission, Final: final, Flight: final.Flight, ReadinessRows: final.Aircraft, PilotID: final.PilotID, Settings: final.Settings}}
	if e := d.constructSimulation(final); e != nil {
		t.Fatal(e)
	}
	if launch.Adjustment != nil {
		if e := d.Rendering.applyFilmInitial(*launch.Adjustment, d.Office.Assets); e != nil {
			t.Fatal(e)
		}
	}
	// The fixture's only initial pause is presentation; use the same live source
	// GPS handlers on recording-enabled and capability-isolated engines.
	for _, x := range []*FrontEnd{c, d} {
		if e := x.openGPS(); e != nil {
			t.Fatal(e)
		}
	}
	actions := []Actions{{GPSKey: "f"}, {SceneDirections: [4]bool{false, true, true, false}}, {SceneDirections: [4]bool{true, false, false, true}}, {}, {Cancel: true}, {GPSKey: "f"}, {GPSKey: " "}, {SceneDirections: [4]bool{true, false, true, false}}, {Selector1: true}, {GPSKey: " "}, {Selector2: true}, {FlightFocusLost: true}, {}, {ViewKey: 1}}
	for n, a := range actions {
		for _, x := range []*FrontEnd{c, d} {
			if e := x.handleFlight(a); e != nil {
				t.Fatal(e)
			}
			for i := 0; i < 9; i++ {
				if e := x.tickGPS(); e != nil {
					t.Fatal(e)
				}
			}
		}
		if group := compareFilmCheckpoint(c.Rendering.FilmCheckpoint(), d.Rendering.FilmCheckpoint()); group != "" {
			t.Fatal("recording changed GPS owner", n, group)
		}
	}
	endFilm(t, c)
	f := verifyFilm(t, c)
	data, _ := replay.Marshal(f)
	g, _ := replay.Unmarshal(data)
	found := false
	for i := range g.Operations {
		op := &g.Operations[i]
		if op.Kind != "boundary" {
			continue
		}
		var b filmBoundary
		json.Unmarshal(op.Payload, &b)
		if b.GPSTick != nil {
			b.GPSTick.After.X++
			op.Payload = filmJSON(b)
			found = true
			break
		}
	}
	if !found {
		t.Fatal("no real modal event")
	}
	q, e := NewFilmPlayback(c.Office.Assets, g, configuration.Defaults())
	if e != nil {
		t.Fatal(e)
	}
	for !q.Done {
		if e = q.Step(); e != nil {
			break
		}
	}
	if q.Divergence == "" || q.FailureDiagnostic == nil || q.FailureDiagnostic.Operation != q.Cursor {
		t.Fatal("forged resolved axis witness accepted or diagnostic missing")
	}
	if e := q.Resume(); e == nil {
		t.Fatal("divergent GPS enabled Resume")
	}
}

func TestReachedElapsedPublicationIntoRealFacts(t *testing.T) {
	// Supplied clock history only: the shared source DA/DC leaf advances to each
	// boundary, then one real ordinary flight update owns the exported value.
	// No past flight/score is claimed for these deliberately shortened cases.
	for _, tc := range []struct {
		phase uint16
		score int32
	}{{10799, 45}, {10800, 45}, {11088, 44}, {25200, 22}, {35639, 22}, {35640, 22}} {
		c := developmentScenarioController(t, "o-combat")
		p, r := c.Rendering, c.Rendering.Flight
		c.AbandonFilmRecording()
		p.filmRecorder = nil
		var timing flight.Timing
		for n := 0; n < 110000; n++ {
			timing = r.Clock.ReplayTiming()
			if timing.Phase == tc.phase {
				break
			}
		}
		if timing.Phase != tc.phase {
			t.Fatal("clock fixture exhausted")
		}
		r.filmTiming = &timing
		r.Paused = false
		p.Paused = false
		if e := c.advanceFlight(0); e != nil {
			t.Fatal(e)
		}
		if r.ElapsedDA != tc.phase {
			t.Fatal("ordinary reached phase not published", tc.phase, r.ElapsedDA)
		}
		if tc.phase == 35640 {
			if c.Screen != ScreenDebrief || c.Debrief == nil {
				t.Fatal("99:00 did not enter debrief")
			}
			proposal := c.Debrief.Pending.Proposal
			if proposal.Facts.ElapsedDA != tc.phase || proposal.Score.Score != tc.score {
				t.Fatal("clock or score at live cutoff", proposal.Score)
			}
			continue
		}
		if c.Screen != ScreenScene {
			t.Fatal("premature ending", tc.phase, c.Screen)
		}
		p.Initialized.EndSortieForQoL()
		f, e := c.freezeSortieFacts()
		if e != nil {
			t.Fatal(e)
		}
		if f.ElapsedDA != tc.phase {
			t.Fatal("result read different owner")
		}
		proposal, e := gamestate.ProposeSortie(c.Mission.ResultLaunch.Start.PilotBefore, f, c.Mission.ResultLaunch.Tables)
		if e != nil {
			t.Fatal(e)
		}
		if proposal.Score.Score != tc.score {
			t.Fatal("source recommendation50 coefficient9 and time handcase", tc.phase, proposal.Score)
		}
	}
}
