package menus

import (
	"gunship2000-assets/game/assets"
	"gunship2000-assets/game/configuration"
	"gunship2000-assets/game/replay"
	gamestate "gunship2000-assets/game/state"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestLivePointCampaignCommitAndReplay(t *testing.T) {
	c := developmentScenarioController(t, "o-short-campaign")
	verifyLivePointCampaign(t, c)
}

func TestMigratedCareerLivePointCampaignCommitAndReplay(t *testing.T) {
	c := missionController(t)
	root := c.Store.Directory
	before := c.Store.State()
	c.Store.Close()
	store, err := gamestate.OpenManagedCareer(root, "", filepath.Dir(root), true)
	if err != nil {
		t.Fatal(err)
	}
	if store.PackDirectory() != filepath.Join(root, "pilots") || !reflect.DeepEqual(before, store.State()) {
		t.Fatal("migration changed pilot before launch")
	}
	c.Store = store
	bank, err := assets.LoadFrontEndAssets(os.Getenv("GS2000_TEST_MISSIONS"))
	if err != nil {
		t.Fatal(err)
	}
	c.Office.Assets, c.Menus = bank, bank.Menus
	if _, err = c.ConfigureDevelopmentScenario("o-short-campaign"); err != nil {
		t.Fatal(err)
	}
	verifyLivePointCampaign(t, c)
}
func verifyLivePointCampaign(t *testing.T, c *FrontEnd) {
	t.Helper()
	before := c.Store.State().SelectedPilot().Statistics.MissionCampaignCounter
	settings, switches := c.Mission.Settings, c.Rendering.Initialized.Switches
	run := func() { useComparedMissionPoint(t, c) }
	run()
	mark := c.Rendering.sessionPoint
	if mark == nil {
		t.Fatal("missing point")
	}
	for i := 0; i < 2; i++ {
		if e := c.Handle(Actions{Cancel: true}); e != nil {
			t.Fatal(e)
		}
		for j := 0; j < 3+i; j++ {
			if e := c.AdvanceRendering(.12); e != nil {
				t.Fatal(e)
			}
		}
		if c.Rendering.Flight.Steps <= mark.recorder.Ordinal {
			t.Fatal("continuation did not advance")
		}
		opts := configuration.Defaults()
		opts.SetResolution("800x600")
		applyDisplay(c, opts)
		run()
		if c.Mission.Settings != settings || c.Rendering.Initialized.Switches != switches {
			t.Fatal("restoration changed the accepted mission settings or effective switches")
		}
		if c.Rendering.sessionPoint != mark || c.Rendering.Flight.Steps != mark.recorder.Ordinal || c.Rendering.Scene.Resolution != "800x600" {
			t.Fatal("restoration lost point, settings or logical position")
		}
	}
	proposal := campaignNaturalBaseEnd(t, c)
	if c.pointAvailable() {
		t.Fatal("point available in debrief")
	}
	ackToLibrary(t, c)
	if c.Store.State().SelectedPilot().Statistics.MissionCampaignCounter != before+1 {
		t.Fatal("campaign commitment")
	}
	committed := c.Store.State()
	if e := c.Store.CommitSortie(proposal.Facts.SortieID); e != nil {
		t.Fatal(e)
	}
	reopened, e := gamestate.OpenSaveStore(c.Store.Directory, "")
	if e != nil || !reflect.DeepEqual(reopened.State(), committed) {
		t.Fatal("restart or repeat acknowledgment differs", e)
	}
	filmAction(t, c, 15)
	for !c.Viewer.Playback.Done {
		if e := c.Viewer.Playback.Step(); e != nil {
			t.Fatal(e)
		}
	}
	if c.pointAvailable() {
		t.Fatal("point available in viewer")
	}
}

func TestSessionPointRestoresReachedCargo(t *testing.T) {
	c := developmentScenarioController(t, "o-cargo")
	c.Handle(Actions{Cancel: true})
	if e := c.AdvanceRendering(.12); e != nil {
		t.Fatal(e)
	}
	point := func() { useComparedMissionPoint(t, c) }
	point()
	mark := c.Rendering.sessionPoint
	if mark.recorder.Ordinal == 0 {
		t.Fatal("empty recording point")
	}
	for i := 0; i < 2; i++ {
		if e := c.Handle(Actions{}); e != nil {
			t.Fatal(e)
		}
		if e := c.Handle(Actions{GPSOpen: true}); e != nil {
			t.Fatal(e)
		}
		if c.Rendering.GPS == nil {
			t.Fatal("GPS did not open")
		}
		c.Rendering.GPS.Selected = 0
		if e := c.Handle(Actions{GPSKey: "c"}); e != nil {
			t.Fatal(e)
		}
		if c.Rendering.Initialized.Stores.Ships[0].Flags&0x18 != 0 {
			t.Fatal("drop did not occur")
		}
		if e := c.Handle(Actions{ViewKey: 1}); e != nil {
			t.Fatal(e)
		}
		point()
		if c.Rendering.Initialized.Stores.Ships[0].Flags&0x18 == 0 {
			t.Fatal("discarded cargo drop survived load")
		}
	}
	endFilm(t, c)
	verifyFilm(t, c)
}

func TestPointDiscardsCombatBranch(t *testing.T) {
	c := developmentScenarioController(t, "o-combat")
	if e := c.Handle(Actions{Cancel: true}); e != nil {
		t.Fatal(e)
	}
	tick := func(a Actions) {
		t.Helper()
		if e := c.Handle(a); e != nil {
			t.Fatal(e)
		}
		if e := c.AdvanceRendering(.12); e != nil {
			t.Fatal(e)
		}
	}
	tick(Actions{WeaponTriggerValid: true})
	run := func() { useComparedMissionPoint(t, c) }
	p := c.Rendering
	r := p.Flight
	for i := 0; i < 4; i++ {
		s := p.Initialized.Stores.Ships[0].Stations[p.Initialized.SelectedStation]
		if strings.Contains(strings.ToUpper(s.ShortName), "HELL") {
			break
		}
		tick(Actions{OrdnanceNext: true, WeaponTriggerValid: true})
	}
	fire := func(i int) {
		if r.Avionics.Target == 0 || i%20 == 0 {
			tick(Actions{WeaponTriggerValid: true, Backspace: true})
		} else {
			tick(Actions{WeaponTriggerValid: true, WeaponTriggerHeld: true})
		}
	}
	for i := 0; i < 900; i++ {
		fire(i)
		active := false
		for _, s := range r.Combat.Slots {
			if s.Lifetime > 0 {
				active = true
				break
			}
		}
		if active {
			break
		}
	}
	active := false
	for _, s := range r.Combat.Slots {
		if s.Lifetime > 0 {
			active = true
		}
	}
	if !active {
		t.Fatal("fixture did not launch a projectile")
	}
	slots := r.Combat.Slots
	damage := r.Systems.Damage
	fuel := slices.Clone(r.Systems.Fuel)
	run()
	for i := 0; i < 900 && !r.Combat.Primary.Achieved; i++ {
		fire(i)
	}
	if !r.Combat.Primary.Achieved {
		t.Fatal("fixture did not destroy objective")
	}
	if e := p.QueueAcceptedPlayerHit(AcceptedPlayerHit{VictimRow: p.Initialized.PlayerRow, Source: "point fixture"}); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 5; i++ {
		tick(Actions{})
	}
	if len(r.DamageEvents) == 0 {
		t.Fatal("damage branch not exercised")
	}
	run()
	r = c.Rendering.Flight
	if r.Combat.Primary.Achieved || r.Combat.Slots != slots || r.Systems.Damage != damage || !slices.Equal(r.Systems.Fuel, fuel) {
		t.Fatal("abandoned combat escaped the restored point")
	}
	endFilm(t, c)
	verifyFilm(t, c)
}

func TestPointDiscardsConfirmedWingmanRoute(t *testing.T) {
	c := mastFixture(t, 0, "pass")
	jFinishFade(t, c)
	act := func(a Actions) {
		t.Helper()
		if e := c.Handle(a); e != nil {
			t.Fatal(e)
		}
	}
	point := func() { useComparedMissionPoint(t, c) }
	act(Actions{})
	act(Actions{FlightCommands: []int{2}})
	if e := c.advanceFlight(.12); e != nil {
		t.Fatal(e)
	}
	act(Actions{})
	before := c.Rendering.FilmCheckpoint()
	point()
	act(Actions{GPSOpen: true})
	act(Actions{})
	for i := 0; i < 3; i++ {
		act(Actions{GPSKey: "n"})
	}
	if c.Rendering.GPS.Selected != 3 {
		t.Fatal("recipient")
	}
	act(Actions{GPSKey: "f"})
	act(Actions{Inside: true, X: 130, Y: 80, Selector1: true})
	act(Actions{GPSKey: "f"})
	act(Actions{ViewKey: 1})
	act(Actions{})
	for i := 0; i < 200; i++ {
		if e := c.advanceFlight(.1); e != nil {
			t.Fatal(e)
		}
		if c.Rendering.Flight.Combat.FilmState().Navigation[3].Count > 0 {
			break
		}
	}
	if c.Rendering.Flight.Combat.FilmState().Navigation[3].Count == 0 {
		t.Fatal("route did not reach navigation")
	}
	point()
	if c.Rendering.Flight.Combat.FilmState().Navigation[3].Count != 0 {
		t.Fatal("abandoned route survived")
	}
	if d := compareFilmCheckpoint(before, c.Rendering.FilmCheckpoint()); d != "" {
		t.Fatal("restored checkpoint differs", d)
	}
	endFilm(t, c)
	verifyFilm(t, c)
}

func useComparedMissionPoint(t *testing.T, c *FrontEnd) {
	t.Helper()
	if c.Rendering.sessionPoint == nil {
		// The reference starts a second engine and consumes only recorded
		// operations. Its save installs a bookmark but never changes the live engine.
		finishReference(t, c, nil)
		c.Rendering.sessionPoint = nil
	}
	if err := c.saveOrLoadPoint(); err != nil {
		t.Fatal(err)
	}
}

func finishReference(t *testing.T, c *FrontEnd, saved *pointReconstruction) *pointReconstruction {
	t.Helper()
	w, err := c.reconstructPoint(saved)
	if err != nil {
		t.Fatal(err)
	}
	for {
		done, err := w.advance(c, time.Millisecond)
		if err != nil {
			t.Fatal(err)
		}
		if done {
			return w
		}
	}
}

func TestMissionPointContinuationMatchesUninterruptedSortie(t *testing.T) {
	for _, scenario := range []string{"o-short-campaign", "o-terminal"} {
		t.Run(scenario, func(t *testing.T) {
			var want []map[string]string
			var endReason string
			var endScore int16
			var endEvents []replay.Event
			var summary *replay.Summary
			var campaignScore gamestate.SortieScore
			for _, mode := range []string{"uninterrupted", "save only", "snapshot loads", "reconstruction loads"} {
				t.Run(mode, func(t *testing.T) {
					c := developmentScenarioController(t, scenario)
					act := func(a Actions) {
						t.Helper()
						if err := c.Handle(a); err != nil {
							t.Fatal(err)
						}
					}
					tick := func() {
						t.Helper()
						if err := c.AdvanceRendering(.12); err != nil {
							t.Fatal(err)
						}
					}
					// Terminal fixture has an actual pending accepted hit at launch.
					var ref *pointReconstruction
					switch mode {
					case "save only", "snapshot loads":
						if err := c.saveOrLoadPoint(); err != nil {
							t.Fatal(err)
						}
					case "reconstruction loads":
						ref = finishReference(t, c, nil)
					}
					if mode == "snapshot loads" || ref != nil {
						for i := 0; i < 2; i++ {
							act(Actions{Cancel: true})
							act(Actions{ViewKey: 4})
							for j := 0; j <= i; j++ {
								tick()
							}
							if ref != nil {
								finishReference(t, c, ref)
							} else if err := c.saveOrLoadPoint(); err != nil {
								t.Fatal(err)
							}
						}
					}
					act(Actions{Cancel: true})
					var got []map[string]string
					for i := 0; i < 120 && c.Rendering != nil; i++ {
						tick()
						if c.Rendering != nil {
							got = append(got, c.Rendering.FilmCheckpoint())
						}
					}
					if mode == "uninterrupted" {
						want = got
					} else if !reflect.DeepEqual(want, got) {
						for i := range min(len(want), len(got)) {
							if g := compareFilmCheckpoint(want[i], got[i]); g != "" {
								t.Fatalf("continued tick %d differs in %s", i, g)
							}
						}
						t.Fatalf("continued ticks %d, want %d", len(got), len(want))
					}
					if scenario == "o-short-campaign" {
						proposal := campaignNaturalBaseEnd(t, c)
						if mode == "uninterrupted" {
							campaignScore = proposal.Score
						} else if !reflect.DeepEqual(campaignScore, proposal.Score) {
							t.Fatal("campaign awards differ")
						}
					} else if c.Rendering != nil {
						t.Fatal("expected terminal damage ending")
					}
					f := verifyFilm(t, c)
					q, err := NewFilmPlayback(c.Office.Assets, f, configuration.Defaults())
					if err != nil {
						t.Fatal(err)
					}
					for i := 0; i < 5; i++ {
						if err := q.Step(); err != nil {
							t.Fatal(err)
						}
					}
					if err := q.Resume(); err != nil {
						t.Fatal(err)
					}
					if q.Engine.pointAvailable() || q.Engine.Store != nil {
						t.Fatal("practice obtained live mission ownership")
					}
					if err := q.AdvancePractice(.12); err != nil {
						t.Fatal(err)
					}
					var events []replay.Event
					for i := 0; i < f.OperationCount(); i++ {
						op, err := f.OperationAt(i)
						if err != nil {
							t.Fatal(err)
						}
						events = append(events, op.Events...)
					}
					if mode == "uninterrupted" {
						endReason, endScore, endEvents, summary = f.End.Reason, f.End.Score, events, f.End.Summary
					} else if endReason != f.End.Reason || endScore != f.End.Score || !slices.Equal(endEvents, events) || !reflect.DeepEqual(summary, f.End.Summary) {
						t.Fatalf("ended with %s, %d, %d events", f.End.Reason, f.End.Score, len(events))
					}
				})
			}
		})
	}
}

func TestMissionPointFailuresPreserveOwners(t *testing.T) {
	c := developmentScenarioController(t, "o-short-campaign")
	live := c.Rendering
	rec := live.filmRecorder
	// Mismatched result target alias cannot be safely captured.
	live.ResultTargets = nil
	if err := c.saveOrLoadPoint(); err == nil || live.sessionPoint != nil || live.filmRecorder != rec {
		t.Fatal("inconsistent capture published")
	}
	live.ResultTargets = live.Initialized.Targets
	if err := c.saveOrLoadPoint(); err != nil {
		t.Fatal(err)
	}
	mark := live.sessionPoint
	if err := c.Handle(Actions{Cancel: true}); err != nil {
		t.Fatal(err)
	}
	if err := c.AdvanceRendering(.12); err != nil {
		t.Fatal(err)
	}
	before := live.FilmCheckpoint()
	old := mark.scene.Initialized.RandomState
	mark.scene.Initialized.RandomState++
	count := rec.Spool.Count()
	if err := c.saveOrLoadPoint(); err == nil || c.Rendering != live || rec.Spool.Count() != count {
		t.Fatal("invalid snapshot replaced continuation")
	}
	mark.scene.Initialized.RandomState = old
	if err := rec.Spool.Close(); err != nil {
		t.Fatal(err)
	}
	if err := c.saveOrLoadPoint(); err == nil || c.Rendering != live || live.sessionPoint != mark || rec.Spool.Count() != count || compareFilmCheckpoint(before, live.FilmCheckpoint()) != "" {
		t.Fatal("unavailable writer replaced continuation")
	}
	if group := compareFilmCheckpoint(mark.checkpoint, mark.scene.FilmCheckpoint()); group != "" {
		t.Fatal("failed load changed snapshot", group)
	}
	c.AbandonFilmRecording()
	if live.sessionPoint != nil {
		t.Fatal("abandon retained mission point")
	}
}
