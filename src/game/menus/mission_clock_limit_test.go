package menus

import (
	"bytes"
	"testing"

	"gunship2000-assets/game/flight"
	gamestate "gunship2000-assets/game/state"
)

// Supply only elapsed clock history: the next real flight update will show
// 98:59. The live controller and the strict reader start from this same clock.
func TestMissionClockLimit(t *testing.T) {
	c := developmentScenarioController(t, "o-combat")
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(c.Handle(Actions{Cancel: true}))
	must(c.Handle(Actions{}))
	var seed flight.Clock
	for {
		next := seed
		if next.ReplayTiming().Phase == 35639 {
			break
		}
		seed = next
	}
	p := c.Rendering
	r := p.Flight
	r.Clock = seed
	must(c.advanceFlight(.1))
	if c.Screen != ScreenScene || r.Steps != 1 || r.ElapsedDA != 35639 {
		t.Fatal("before cutoff", c.Screen, r.Steps, r.ElapsedDA)
	}
	for i := 0; i < 2; i++ {
		must(c.Handle(Actions{CompressionIncrease: true}))
		must(c.Handle(Actions{}))
	}
	if r.Compression.Rate() != 4 {
		t.Fatal("fixture failed to engage 4x")
	}
	must(c.advanceFlight(.25))
	if c.Screen != ScreenDebrief || c.Debrief == nil || r.ElapsedDA != 35640 || r.Clock.ReplayState().Phase != 35640 {
		t.Fatal("compressed cutoff", c.Screen, r.ElapsedDA, r.Steps, c.Message)
	}
	proposal := c.Debrief.Pending.Proposal
	if proposal.Facts.ElapsedDA != 35640 || proposal.Facts.Reason != "voluntary" {
		t.Fatal("incorrect result", proposal.Facts.Reason, proposal.Facts.ElapsedDA)
	}
	stepCount := r.Steps
	for i := 0; i < 3; i++ {
		must(c.AdvanceRendering(.25))
	}
	if c.Debrief.Pending.Proposal != proposal || r.Steps != stepCount {
		t.Fatal("advanced after debrief")
	}
	f, e := c.SealRecordedFilm()
	must(e)
	if f == nil || len(f.Operations) < 2 {
		t.Fatal("missing ordinary film")
	}
	endings := 0
	for _, op := range f.Operations {
		if op.Kind == "end" {
			endings++
		}
	}
	if endings != 1 || f.Operations[len(f.Operations)-1].Kind != "end" || f.End.Reason != "Alt+E" {
		t.Fatal("recorded ending", endings, f.End.Reason)
	}
	// Strictly reconstruct every boundary through the final flight step. The
	// supplied clock is outside the format and is supplied equally to both runs.
	beforeEnd := func() *FilmPlayback {
		q, e := NewFilmPlayback(c.Office.Assets, f, c.Runtime)
		must(e)
		seeded := false
		for _, op := range f.Operations {
			if op.Kind == "end" {
				break
			}
			if !seeded && op.Kind == "step" {
				q.Preview().Flight.Clock = seed
				seeded = true
			}
			must(q.Step())
		}
		if !seeded || q.Engine.filmIsolation.Done || q.Preview().Flight.ElapsedDA != 35640 || q.Preview().Initialized.Sortie.Result != nil {
			t.Fatal("strict playback injected an ending")
		}
		return q
	}
	q := beforeEnd()
	must(q.Step())
	if !q.Done || q.Divergence != "" {
		t.Fatal("recorded ending failed", q.Divergence)
	}
	manual := beforeEnd()
	must(manual.Engine.Handle(Actions{EndMission: true}))
	if manual.Engine.Screen != ScreenQuit {
		t.Fatal("expected manual confirmation")
	}
	must(manual.Engine.Handle(Actions{ConfirmQuit: true}))
	// Supply the same immutable launch and local roster to the pure result
	// calculation. The reconstructed flight itself never owns a career store.
	factsOwner := FrontEnd{Mission: c.Mission, Office: c.Office, Store: c.Store, Rendering: manual.Preview()}
	facts, e := factsOwner.freezeSortieFacts()
	must(e)
	launch := c.Mission.ResultLaunch
	expected, e := gamestate.ProposeSortie(launch.Start.PilotBefore, facts, launch.Tables)
	must(e)
	if !bytes.Equal(filmJSON(expected), filmJSON(proposal)) {
		t.Fatal("confirmed Alt+E and automatic full result proposals differ")
	}
	practice := beforeEnd()
	must(practice.Resume())
	must(practice.AdvancePractice(.25))
	if !practice.Done || practice.Engine.Store != nil || practice.Preview().Flight.Steps != stepCount {
		t.Fatal("practice advanced or acquired career ownership")
	}
	t.Logf("4x batch stopped at phase %d after %d flight updates; one recorded end, score %d, complete proposal identical to manual Alt+E", r.ElapsedDA, stepCount, proposal.Score.Score)
}
