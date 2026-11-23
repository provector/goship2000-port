package menus

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"gunship2000-assets/game/flight"
	"gunship2000-assets/game/replay"
)

func defenseCommands(p *RenderingPreview) []uint16 {
	var out []uint16
	for _, q := range p.takeSceneAudio() {
		if q.event == 0x0a || q.event == 0x2a {
			out = append(out, q.event)
		}
	}
	return out
}

func TestDefenseActualLogicalDispatchSuccessEmptyPauseAndOldProfile(t *testing.T) {
	for _, modern := range []bool{false, true} {
		c := mastFixture(t, 0, "pass")
		jFinishFade(t, c)
		p := c.Rendering
		p.p7Mechanics = modern
		r := p.Flight
		r.Defenses = flight.NewDefenses(3, 3)
		c.Handle(Actions{})
		p.takeSceneAudio()
		if err := c.Handle(Actions{DefenseCommands: []int{0, 1}}); err != nil {
			t.Fatal(err)
		}
		if len(defenseCommands(p)) != 0 {
			t.Fatal("input receipt played before logical consumer")
		}
		jStep(t, c, 1)
		var want []uint16
		if modern {
			want = []uint16{0x0a, 0x2a}
		}
		if got := defenseCommands(p); !reflect.DeepEqual(got, want) {
			t.Fatal(modern, got, want)
		}
		if r.Defenses.Chaff != 0 || r.Defenses.Flares != 0 {
			t.Fatal(r.Defenses)
		}
		c.Handle(Actions{DefenseCommands: []int{0, 1}})
		jStep(t, c, 1)
		if len(defenseCommands(p)) != 0 {
			t.Fatal("empty attempt sounded")
		}
		r.Defenses = flight.NewDefenses(6, 6)
		r.Systems.DamageMasks[0] |= 0x40
		c.Handle(Actions{DefenseCommands: []int{0, 1}})
		jStep(t, c, 1)
		if len(defenseCommands(p)) != 0 || r.Defenses.Chaff != 6 {
			t.Fatal("damaged rejection sounded")
		}
		r.Systems.DamageMasks[0] &^= 0x40
		c.Handle(Actions{FlightPause: true})
		c.Handle(Actions{DefenseCommands: []int{0, 1}})
		if len(defenseCommands(p)) != 0 {
			t.Fatal("pause/resume key fabricated release")
		}
		c.Handle(Actions{AnyKey: true}) // Real focus/keyboard receipt releases pause, not a bare model command.
		c.Handle(Actions{})
		c.Handle(Actions{GPSOpen: true})
		if p.GPS == nil {
			t.Fatal("GPS did not open")
		}
		c.Handle(Actions{DefenseCommands: []int{0, 1}})
		if len(defenseCommands(p)) != 0 || r.Defenses.Chaff != 6 {
			t.Fatal("GPS dispatched flight release")
		}
	}
}

// This is a controller-driven, real authored launch/record/result/commit, not
// physical OS input, continuous flight, an original runtime or listening claim.
func TestDefenseNewFilmStrictReplayAndVerifiedPracticeIsolation(t *testing.T) {
	c := authoredUIController(t)
	authorThroughUI(t, c)
	editorKey(t, c, "b")
	for i := 0; i < 100 && c.Screen != ScreenBriefing; i++ {
		if err := c.Tick(); err != nil {
			t.Fatal(err)
		}
	}
	if c.Screen != ScreenBriefing {
		t.Fatal(c.Screen, c.Message)
	}
	startRendering(t, c)
	jFinishFade(t, c)
	p := c.Rendering
	if !p.p7Mechanics {
		t.Fatal("new launch not P7")
	}
	editorAction(t, c, Actions{})
	for _, keys := range [][]int{{0, 1}, {0}, {1}} {
		editorAction(t, c, Actions{DefenseCommands: keys})
		jStep(t, c, 1)
	}
	editorAction(t, c, Actions{EndMission: true})
	editorAction(t, c, Actions{ConfirmQuit: true})
	if c.Screen != ScreenDebrief {
		t.Fatal(c.Screen, c.Message)
	}
	for i := 0; i < 64 && c.Screen != ScreenFilms; i++ {
		editorAction(t, c, Actions{Enter: true})
		editorAction(t, c, Actions{MenuEnd: true})
		editorAction(t, c, Actions{Enter: true})
	}
	if c.Screen != ScreenFilms || len(c.Library.Entries) != 1 {
		t.Fatal(c.Screen, c.Message, c.filmError)
	}
	f, err := c.Library.Store.Load(c.Library.Entries[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if f.Header.Simulation != replay.R12AuthoredCompatibility {
		t.Fatal(f.Header.Simulation)
	}
	before, _ := json.Marshal(c.Store.State())
	raw, _ := os.ReadFile(publishedFilmPath(t, c, f.Header.ID))
	for _, scale := range []int{1, 2} {
		options := c.Runtime
		options.SimulationRenderScale = scale
		q, er := NewFilmPlayback(c.Office.Assets, f, options)
		if er != nil {
			t.Fatal(er)
		}
		var audio []uint16
		for !q.Done {
			if er = q.Step(); er != nil {
				t.Fatal(er)
			}
			audio = append(audio, defenseCommands(q.Preview())...)
		}
		if q.Divergence != "" || !reflect.DeepEqual(audio, []uint16{0x0a, 0x2a, 0x0a, 0x2a}) {
			t.Fatal(scale, q.Divergence, audio)
		}
	}
	q, err := NewFilmPlayback(c.Office.Assets, f, c.Runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err = q.Resume(); err != nil {
		t.Fatal(err)
	}
	if q.Engine.Store != nil {
		t.Fatal("practice acquired career owner")
	}
	q.HandlePractice(Actions{})
	q.HandlePractice(Actions{DefenseCommands: []int{0, 1}})
	if err = q.AdvancePractice(.1); err != nil {
		t.Fatal(err)
	}
	if got := defenseCommands(q.Preview()); !reflect.DeepEqual(got, []uint16{0x0a, 0x2a}) {
		t.Fatal("verified practice", got)
	}
	after, _ := json.Marshal(c.Store.State())
	again, _ := os.ReadFile(publishedFilmPath(t, c, f.Header.ID))
	if string(before) != string(after) || string(raw) != string(again) {
		t.Fatal("playback/practice changed source film/career")
	}
	if out := os.Getenv("GS2000_P7_FEEDBACK_ARTIFACTS"); out != "" {
		if err = os.MkdirAll(out, 0700); err != nil {
			t.Fatal(err)
		}
		if err = writeNewFilmArtifact(filepath.Join(out, "defense-feedback-authored.g2r"), raw); err != nil {
			t.Fatal(err)
		}
	}
}
