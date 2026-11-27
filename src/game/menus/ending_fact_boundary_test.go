package menus

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"gunship2000-assets/game/diagnostics"
	"gunship2000-assets/game/replay"
	gamestate "gunship2000-assets/game/state"
	"io"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestEndingMissingFuelRecoversEquivalentCampaignFacts(t *testing.T) {
	c := developmentScenarioController(t, "o-short-campaign")
	jFinishFade(t, c)
	c.Rendering.Initialized.EndSortieForQoL()
	session, err := diagnostics.StartSession(c.Store.Directory, "W3 focused", "ordinary logging off")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(session.Close)
	want, err := c.freezeSortieFacts()
	if err != nil {
		t.Fatal(err)
	}
	r := c.Rendering.Flight
	r.Systems.Fuel = r.Systems.Fuel[:len(r.Systems.Fuel)-1]
	got, err := c.freezeSortieFacts()
	if err != nil || !reflect.DeepEqual(want, got) || !strings.Contains(c.resultWarning, "Recovered") {
		t.Fatalf("equivalence=%t warning=%s err=%v", reflect.DeepEqual(want, got), c.resultWarning, err)
	}
	if err = c.finishSortieResult(); err != nil || c.Debrief == nil {
		t.Fatal("debrief", err, c.Message)
	}
	id := c.Debrief.Pending.Start.ID
	ackToLibrary(t, c)
	after := filmJSON(c.Store.State().SelectedPilot())
	if err = c.Store.CommitSortie(id); err != nil || !bytes.Equal(after, filmJSON(c.Store.State().SelectedPilot())) {
		t.Fatal("repeat", err)
	}
	reopened, err := gamestate.OpenSaveStore(c.Store.Directory, "")
	if err != nil || !bytes.Equal(after, filmJSON(reopened.State().SelectedPilot())) {
		t.Fatal("restart", err)
	}
	session.Close()
	names, err := filepath.Glob(filepath.Join(filepath.Dir(c.Store.Directory), "reports", "session-*", "incident-*", "error_report.zip"))
	if err != nil || len(names) != 1 {
		t.Fatal("report", names, err)
	}
	z, err := zip.OpenReader(names[0])
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	body, err := z.Open("report.txt")
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(body)
	body.Close()
	if err != nil || !bytes.Contains(data, []byte("Recovered")) || !bytes.Contains(data, []byte("result was saved")) {
		t.Fatal(string(data), err)
	}
	film, err := c.Library.Store.Load(c.Library.Entries[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if film.Header.Simulation != replay.RouteAcquisitionCompatibility {
		t.Fatal(film.Header.Simulation)
	}
	q, err := NewFilmPlayback(c.Office.Assets, film, c.Runtime)
	if err != nil {
		t.Fatal(err)
	}
	for !q.Done {
		if err = q.Step(); err != nil {
			t.Fatal(err)
		}
	}

}

func TestUnverifiableRecommendationOmitsOnlyMatchingBonus(t *testing.T) {
	c := developmentScenarioController(t, "o-short-campaign")
	jFinishFade(t, c)
	c.Rendering.Initialized.EndSortieForQoL()
	s := c.Mission
	complete, err := c.freezeSortieFacts()
	if err != nil {
		t.Fatal(err)
	}
	completeScore, err := gamestate.ProposeSortie(s.ResultLaunch.Start.PilotBefore, complete, s.ResultLaunch.Tables)
	if err != nil {
		t.Fatal(err)
	}
	s.InitialAircraftIDs = nil
	s.ResultLaunch.Recommended = nil
	f, err := c.freezeSortieFacts()
	if err != nil || f.SelectorMultisetMatch || !f.ResultContext.RecommendationUnknown {
		t.Fatal(err, c.resultWarning)
	}
	partial, err := gamestate.ProposeSortie(s.ResultLaunch.Start.PilotBefore, f, s.ResultLaunch.Tables)
	if err != nil {
		t.Fatal(err)
	}
	bonus := int32(0)
	if complete.SelectorMultisetMatch {
		bonus = 50
	}
	if partial.Score.PreScale != completeScore.Score.PreScale-bonus || partial.Score.AfterObjectCredit != completeScore.Score.AfterObjectCredit || partial.Score.AfterObjectiveBonus != completeScore.Score.AfterObjectiveBonus {
		t.Fatal("unrelated scoring change", partial.Score, completeScore.Score)
	}
	if err = c.finishSortieResult(); err != nil || c.Debrief == nil {
		t.Fatal("partial debrief", err, c.Message)
	}
}

// Rejected-result playback and sealed publication are serviced on subsequent
// event-loop turns. Observe the final screen only after that work finishes.
func waitForResultVerification(t *testing.T, c *FrontEnd) {
	t.Helper()
	until := time.Now().Add(30 * time.Second)
	for c.rejectedCheck != nil {
		if time.Now().After(until) {
			t.Fatal("result verification exceeded 30 seconds")
		}
		if err := c.AdvanceReplay(.016); err != nil {
			t.Fatal(err)
		}
		if c.rejectedCheck != nil && c.rejectedCheck.published != nil {
			time.Sleep(time.Millisecond)
		}
	}
}

func TestMissingFuelCannotUseAnotherActorOrStaleServiceReading(t *testing.T) {
	c := developmentScenarioController(t, "o-short-campaign")
	jFinishFade(t, c)
	p := c.Rendering
	p.Initialized.EndSortieForQoL()
	r := p.Flight
	n := len(r.Systems.Fuel) - 1
	r.Systems.Fuel = r.Systems.Fuel[:n]
	l := c.Mission.ResultLaunch
	saved := l.fuel[n]
	for _, bad := range []fuelReceipt{
		{actor: l.fuel[0].actor, value: saved.value, phase: saved.phase, step: saved.step},
		{actor: saved.actor, value: saved.value, phase: saved.phase + 1, step: saved.step},
		{actor: saved.actor, value: saved.value - 1, phase: saved.phase, step: saved.step},
	} {
		l.fuel[n] = bad
		f, err := c.freezeSortieFacts()
		if err != nil || int16(binary.LittleEndian.Uint16(f.ResultContext.Actors[n].Ship[23:25])) != -1 {
			t.Fatal("stale fuel admitted", bad, err)
		}
	}
	p.Initialized.Entities[p.Initialized.FlightRows[n]].ID = ""
	if _, err := c.freezeSortieFacts(); err == nil {
		t.Fatal("unidentified aircraft was scored")
	}
	p.Initialized.Sortie.Result = nil
	if _, err := c.freezeSortieFacts(); err == nil {
		t.Fatal("no terminal facts accepted")
	}
}

func TestRecordedForcedLandingReplaysAndResumes(t *testing.T) {
	c := developmentScenarioController(t, "o-short-campaign")
	jFinishFade(t, c)
	p := c.Rendering
	// Queue two accepted fuel-system hits through the recorded damage boundary.
	if err := p.queueDevelopmentDamageScenario("crash"); err != nil {
		t.Fatal(err)
	}
	playerDamageResume(t, c)
	if p.Flight.Systems.Damage[3] != 2 || p.Flight.State.Terminal != nil {
		t.Fatal("fuel failure became countdown")
	}
	for n := 0; n < 800 && c.Debrief == nil; n++ {
		if err := c.AdvanceRendering(.11); err != nil {
			t.Fatal(err)
		}
	}
	if c.Debrief == nil || p.Initialized.Sortie.Result == nil || p.Initialized.Sortie.Result.Reason != "disabled-contact" {
		t.Fatal("flight did not reach forced landing", c.Screen, p.Initialized.Sortie.Result)
	}
	ackToLibrary(t, c)
	f, err := c.Library.Store.Load(c.Library.Entries[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, practice := range []bool{false, true} {
		q, err := NewFilmPlayback(c.Office.Assets, f, c.Runtime)
		if err != nil {
			t.Fatal(err)
		}
		for !q.Done {
			if err = q.Step(); err != nil {
				t.Fatal(err)
			}
			if practice && q.Preview().Flight.Systems.Damage[3] == 2 {
				if err = q.Resume(); err != nil || q.Engine.Store != nil || !q.Preview().correctiveFlight {
					t.Fatal("isolated fuel-loss practice", err)
				}
				return
			}
		}
	}
	t.Fatal("no reached fuel-system transition for Resume")
}
