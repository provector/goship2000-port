package menus

import (
	"bytes"
	"encoding/json"
	"gunship2000-assets/game/configuration"
	"gunship2000-assets/game/replay"
	s "gunship2000-assets/game/state"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
)

// This is a controller recording with labelled supplied starting poses only.
// Targets, damage, weapons, orders, contact and constructor outcomes are never
// injected. It is not an OS-input or naturally approached battlefield witness.
func TestRecordedRealFollowerEngagement(t *testing.T) {
	c := renderingController(t)
	next := c.Store.State()
	pilot := next.SelectedPilot()
	pilot.Rank = s.RankBrigadierGeneral
	pilot.MissionType = s.MissionFlight
	pilot.Theatre = s.TheatrePersianGulf
	var err error
	pilot.SquadronPilots, err = s.InitializeSquadron(pilot, next.Slots[:], c.Office.Assets.Missions.Preflight.SquadronNames, rand.New(rand.NewSource(42)).Intn)
	if err != nil {
		t.Fatal(err)
	}
	if err = c.Store.Commit(next); err != nil {
		t.Fatal(err)
	}
	if err = c.RefreshOffice(); err != nil {
		t.Fatal(err)
	}
	seed := int64(1)
	c.MissionSeed = &seed
	readyMission(t, c)
	startRendering(t, c)
	jFinishFade(t, c)
	if e := c.returnRenderingBriefing(); e != nil {
		t.Fatal(e)
	}
	if e := c.openOrdnance(3); e != nil {
		t.Fatal(e)
	}
	choose := func(action string, system, station int) {
		t.Helper()
		for i, v := range c.Mission.Ordnance.Choices {
			if v.Enabled && v.Action == action && (system < 0 || v.System == system) && (station < 0 || v.Station == station) {
				missionClick(t, c, i)
				return
			}
		}
		t.Fatal("missing accepted ordnance", action, system, station)
	}
	choose("fits", 7, -1)
	choose("mount", 7, 0)
	choose("back", -1, -1)
	choose("back", -1, -1)
	if c.Screen == ScreenOutfit {
		missionClick(t, c, 6)
	}
	if c.Screen != ScreenBriefing {
		t.Fatal(c.Screen, c.Message)
	}
	startRendering(t, c)
	jFinishFade(t, c)
	p := c.Rendering
	var target s.InitializedEntity
	for _, e := range p.Initialized.Entities {
		if e.Kind >= 4 && e.Kind <= 6 && e.Participation == 1 && e.Object.Raw[31] < 2 && e.Pose.H0 > 0x8000 && e.Pose.H1 > 0x8000 && e.Pose.H0 < 0x78000 && e.Pose.H1 < 0x78000 {
			target = e
			break
		}
	}
	if target.ID == "" {
		for _, e := range p.Initialized.Entities {
			if e.ID != "" {
				t.Log(e.RuntimeRow, e.Kind, e.Participation, e.Object.Raw[31], e.Pose)
			}
		}
		t.Fatal("no actual generated passive ground target")
	}
	pose := s.InitializedPose{H0: target.Pose.H0, H1: target.Pose.H1 - 4096, Height: target.Pose.Height + 256}
	a := FilmInitialAdjustment{Label: "R12 real-controller engagement: supplied fresh member3 position only; ordinary weapons/contact/constructor; voluntary ending", Followers: []FilmFollowerPlacement{{Actor: 3, Pose: pose, AltitudeSelector: 2}}}
	if e := c.ApplyFilmInitialAdjustment(a); e != nil {
		t.Fatal(e)
	}
	act := func(a Actions) {
		t.Helper()
		if e := c.Handle(a); e != nil {
			t.Fatal(e)
		}
	}
	act(Actions{})
	act(Actions{FlightCommands: []int{2}})
	act(Actions{})
	act(Actions{GPSOpen: true})
	act(Actions{})
	for n := 0; n < 3; n++ {
		act(Actions{GPSKey: "n"})
	}
	if p.GPS.Selected != 3 {
		t.Fatal("GPS recipient", p.GPS.Selected)
	}
	if p.Initialized.Entities[3].ControlRaw[4] == 0 {
		act(Actions{GPSKey: "w"})
	}
	act(Actions{ViewKey: 1})
	act(Actions{})
	for n := 0; n < 8; n++ {
		act(Actions{CompressionIncrease: true})
		act(Actions{})
	}
	if p.Flight.Compression.Request != 16 {
		t.Fatal("rate", p.Flight.Compression.Request)
	}
	rows := []map[string]any{}
	launch := false
	for n := 0; n < 300; n++ {
		before := p.Flight.Compression.Request
		if e := c.AdvanceRendering(.02); e != nil {
			t.Fatal(e)
		}
		e := p.Initialized.Entities[3]
		for _, ev := range p.Flight.CombatEvents {
			if ev.Kind == "launch" && ev.Actor == 3 && ev.Source == "A7E6:AF21 healthy follower" {
				launch = true
				rows = append(rows, map[string]any{"event": ev, "step": p.Flight.Steps, "request_before": before, "request_after": p.Flight.Compression.Request, "hud": p.Cockpit.CompressionRequest, "target": e.ControlRaw[3]})
			}
		}
		if launch {
			break
		}
		if n%25 == 0 {
			t.Log("waiting", n, p.Flight.Steps, e.Kind, e.Participation, e.ControlRaw, e.Pose, p.Flight.Failure)
		}
	}
	if !launch {
		t.Fatal("no naturally selected follower release", target, p.Initialized.Stores.Ships[3])
	}
	if p.Flight.Compression.Request != 0 || p.Cockpit.CompressionRequest != 0 {
		t.Fatal("accepted release did not reset", rows)
	}
	for n := 0; n < 8; n++ {
		if e := c.AdvanceRendering(.1); e != nil {
			t.Fatal(e)
		}
	}
	endFilm(t, c)
	f, e := c.SealRecordedFilm()
	if e != nil {
		t.Fatal(e)
	}
	if f.Header.Simulation != replay.R12Compatibility {
		t.Fatal(f.Header.Simulation)
	}
	opt := configuration.Defaults()
	opt.DisableWingmenAI = true
	raw, e := replay.Marshal(f)
	if e != nil {
		t.Fatal(e)
	}
	f, e = replay.Unmarshal(raw)
	if e != nil {
		t.Fatal(e)
	}
	ownerBefore, _ := json.Marshal(c.Store.State())
	q, e := NewFilmPlayback(c.Office.Assets, f, opt)
	if e != nil {
		t.Fatal(e)
	}
	for !q.Done {
		if e = q.Step(); e != nil {
			t.Fatal(e)
		}
	}
	for _, step := range []uint64{3, 6} {
		practice, e := NewFilmPlayback(c.Office.Assets, f, opt)
		if e != nil {
			t.Fatal(e)
		}
		for practice.Preview().Flight.Steps < step {
			if e = practice.Step(); e != nil {
				t.Fatal(e)
			}
		}
		if e = practice.Resume(); e != nil {
			t.Fatal(e)
		}
		if practice.Engine.Store != nil {
			t.Fatal("practice gained career owner")
		}
		if e = practice.HandlePractice(Actions{}); e != nil {
			t.Fatal(e)
		}
		if e = practice.AdvancePractice(.1); e != nil {
			t.Fatal(e)
		}
	}
	ownerAfter, _ := json.Marshal(c.Store.State())
	after, _ := replay.Marshal(f)
	if !bytes.Equal(ownerBefore, ownerAfter) || !bytes.Equal(raw, after) {
		t.Fatal("strict playback/Resume changed source career or film")
	}
	out := os.Getenv("R12_OUT")
	if out != "" {
		if e = os.WriteFile(filepath.Join(out, "accepted-engagement.gsf"), raw, 0600); e != nil {
			t.Fatal(e)
		}
		raw, e = json.MarshalIndent(map[string]any{"scope": "Fresh generated Gulf seed1 mission, source-accepted ordnance, labelled supplied member3 starting pose, real GPS weapons order and logical compression input; natural contact/accepted release. Voluntary ending, no career acknowledgment. Native controller, not OS input.", "target": target, "initial_adjustment": a, "events": rows, "profile": f.Header.Simulation, "strict_replay": true, "isolated_resume_before_and_after": true, "operations": len(f.Operations), "steps": f.End.Steps}, "", "  ")
		if e != nil {
			t.Fatal(e)
		}
		os.WriteFile(filepath.Join(out, "accepted-engagement.json"), append(raw, '\n'), 0600)
	}
}
