package menus

import (
	"bytes"
	"encoding/json"
	"gunship2000-assets/game/configuration"
	"gunship2000-assets/game/replay"
	"reflect"
	"testing"
)

// A fresh generated Flight launch and normal logical GPS operations. This
// checks active navigation transport, not natural objective/career acceptance.
func TestNavigationRecordedActiveStateStrictReplayAndPractice(t *testing.T) {
	c := mastFixture(t, 0, "pass")
	jFinishFade(t, c)
	p := c.Rendering
	if !p.navigationMechanics || p.Flight.Combat.FilmState().Navigation == nil {
		t.Fatal("profile not selected before launch")
	}
	act := func(a Actions) {
		t.Helper()
		if err := c.Handle(a); err != nil {
			t.Fatal(err)
		}
	}
	act(Actions{})
	act(Actions{FlightCommands: []int{2}})
	c.advanceFlight(.12)
	act(Actions{})
	act(Actions{GPSOpen: true})
	act(Actions{})
	for i := 0; i < 3; i++ {
		act(Actions{GPSKey: "n"})
	}
	if p.GPS == nil || p.GPS.Selected != 3 {
		t.Fatal("recipient")
	}
	act(Actions{GPSKey: "f"})
	act(Actions{Inside: true, X: 130, Y: 80, Selector1: true})
	act(Actions{GPSKey: "f"})
	act(Actions{ViewKey: 1})
	act(Actions{})
	active := false
	for tick := 0; tick < 200; tick++ {
		if err := c.advanceFlight(.1); err != nil {
			t.Fatal(err)
		}
		n := p.Flight.Combat.FilmState().Navigation
		if n != nil && n[3].Count > 0 {
			active = true
			break
		}
	}
	if !active {
		t.Fatal("assigned route did not calculate navigation", p.Initialized.FollowerLimits)
	}
	endFilm(t, c)
	f, err := c.SealRecordedFilm()
	if err != nil {
		t.Fatal(err)
	}
	if !f.NavigationMechanics() || f.Header.Simulation != replay.R12Compatibility || f.Header.InputSchema != replay.R1InputCompatibility {
		t.Fatal("new native policy not frozen")
	}
	ownerBefore, _ := json.Marshal(c.Store.State())
	filmBefore, _ := replay.Marshal(f)
	opt := configuration.Defaults()
	opt.DisableWingmenAI = !c.Runtime.DisableWingmenAI
	q, err := NewFilmPlayback(c.Office.Assets, f, opt)
	if err != nil {
		t.Fatal(err)
	}
	for !q.Done {
		if err = q.Step(); err != nil {
			t.Fatal(err)
		}
	}
	practice, err := NewFilmPlayback(c.Office.Assets, f, opt)
	if err != nil {
		t.Fatal(err)
	}
	for practice.Cursor < len(f.Operations) {
		if err = practice.Step(); err != nil {
			t.Fatal(err)
		}
		n := practice.Preview().Flight.Combat.FilmState().Navigation
		if n != nil && n[3].Count > 0 {
			break
		}
	}
	before := practice.Preview().Flight.Combat.FilmState().Navigation
	if before == nil || before[3].Count == 0 {
		t.Fatal("no reached active navigation")
	}
	if err = practice.Resume(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, practice.Preview().Flight.Combat.FilmState().Navigation) || practice.Engine.Store != nil {
		t.Fatal("Resume discarded navigation or acquired career capability")
	}
	practice.HandlePractice(Actions{})
	if err = practice.AdvancePractice(.12); err != nil {
		t.Fatal(err)
	}
	ownerAfter, _ := json.Marshal(c.Store.State())
	filmAfter, _ := replay.Marshal(f)
	if !bytes.Equal(ownerBefore, ownerAfter) || !bytes.Equal(filmBefore, filmAfter) {
		t.Fatal("playback/practice mutated source film/career")
	}
}
