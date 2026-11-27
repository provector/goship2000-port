package menus

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"gunship2000-assets/game/navigation"
	"gunship2000-assets/game/render3d"
	gamestate "gunship2000-assets/game/state"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// The supplied editor mission is a private optional regression fixture.
func TestCarrierReturnsUseIndividualDeckBerths(t *testing.T) {
	path := os.Getenv("GS2000_TEST_CARRIER_MISSION")
	if path == "" {
		t.Skip("set GS2000_TEST_CARRIER_MISSION to the Arctic Fleet editor mission")
	}
	c := authoredUIController(t)
	c.simulationProfile = ""
	s := c.Store.State()
	s.SelectedPilot().Theatre = gamestate.TheatreAntarctica
	if e := c.Store.Commit(s); e != nil {
		t.Fatal(e)
	}
	c.RefreshOffice()
	if e := os.MkdirAll(c.MissionDirectory, 0700); e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(c.MissionDirectory, "Arctic Fleet.g2m"), b, 0600); e != nil {
		t.Fatal(e)
	}
	if e = c.openMissionFiles(); e != nil {
		t.Fatal(e)
	}
	c.Files.Selected = 0
	if e = c.activateMissionFile(15); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 40 && c.Screen != ScreenBriefing; i++ {
		c.Tick()
	}
	if c.Screen != ScreenBriefing {
		t.Fatal(c.Screen, c.Files.Status)
	}
	for slot := range c.Mission.ReadinessRows {
		if e := c.openOrdnance(slot); e != nil {
			t.Fatal(e)
		}
		choose := func(action string, id int) {
			t.Helper()
			for index, o := range c.Mission.Ordnance.Choices {
				if o.Enabled && o.Action == action && (id < 0 || id == o.Value) {
					if e := c.developmentScenarioClick(index); e != nil {
						t.Fatal(e)
					}
					return
				}
			}
			t.Fatal(action, id)
		}
		choose("aircraft", -1)
		choose("replace", 0)
		choose("back", -1)
		if slot == 0 && ordnanceAcceptance(c.Mission.Ordnance.Working[:], 0) != "" {
			choose("fits", -1)
			choose("mount", -1)
			choose("back", -1)
		}
		choose("back", -1)
	}
	if c.Screen == ScreenOutfit {
		if e := c.developmentScenarioClick(6); e != nil {
			t.Fatal(e)
		}
	}
	startRendering(t, c)
	jFinishFade(t, c)
	p := c.Rendering
	t.Log("deck", p.carrierBerths)
	for _, v := range p.Scene.Instances {
		if v.ModelID == 64 {
			t.Logf("carrier %+v", v)
		}
	}
	send := func(a Actions) {
		t.Helper()
		if e := c.Handle(a); e != nil {
			t.Fatal(e)
		}
	}
	wingmen := func(key string) {
		t.Helper()
		send(Actions{})
		send(Actions{GPSOpen: true})
		send(Actions{})
		if c.Rendering.GPS == nil {
			t.Fatal("GPS did not open", c.Screen, c.Rendering.Flight.requireRelease)
		}
		for i := 1; i <= 4; i++ {
			for j := 0; c.Rendering.GPS.Selected != i && j < 5; j++ {
				send(Actions{GPSKey: "n"})
			}
			if c.Rendering.GPS.Selected != i {
				t.Fatal("GPS selection", i)
			}
			send(Actions{GPSKey: key})
			if key == "f" {
				send(Actions{Inside: true, X: 18, Y: 26, Selector1: true})
				send(Actions{GPSKey: "f"})
			}
		}
		send(Actions{ViewKey: 1})
		send(Actions{})
	}
	// Depart the carrier on independently confirmed GPS routes before recalling
	// the formation. All inputs pass through the recorder's boundary owner.
	wingmen("f")
	for i := 0; i < 35; i++ {
		if e = c.advanceFlight(.12); e != nil {
			t.Fatal(e)
		}
	}
	saved := p.FilmCheckpoint()
	useComparedMissionPoint(t, c)
	wingmen("b")
	// Replace one return order with an empty confirmed draft. The other three
	// retain their actual return controllers. Load then discards this branch.
	send(Actions{GPSOpen: true})
	send(Actions{})
	for i := 0; c.Rendering.GPS.Selected != 3 && i < 5; i++ {
		send(Actions{GPSKey: "n"})
	}
	if c.Rendering.GPS.Selected != 3 {
		t.Fatal("missing wingman")
	}
	send(Actions{GPSKey: "f"})
	send(Actions{})
	send(Actions{GPSKey: "f"})
	send(Actions{ViewKey: 1})
	send(Actions{})
	for i := 1; i <= 4; i++ {
		e := p.Initialized.Entities[i]
		if i == 3 {
			if e.ControlRaw[0] != 0 || e.ControlRaw[1] != 2 || e.Kind != 0x20 || binary.LittleEndian.Uint32(e.ControlRaw[40:]) != 0 {
				t.Fatal("old return still active", e.Kind, e.ControlRaw)
			}
		} else if e.ControlRaw[1] != 8 {
			t.Fatal("other return changed", i, e.ControlRaw[1])
		}
	}
	for i := 0; i < 15; i++ {
		if e = c.advanceFlight(.12); e != nil {
			t.Fatal(e)
		}
	}
	useComparedMissionPoint(t, c)
	p = c.Rendering
	if mismatch := compareFilmCheckpoint(saved, p.FilmCheckpoint()); mismatch != "" {
		t.Fatal("first restoration", mismatch)
	}
	send(Actions{})
	for i := 0; i < 8; i++ {
		if e = c.AdvanceRendering(.12); e != nil {
			t.Fatal(e)
		}
	}
	useComparedMissionPoint(t, c)
	p = c.Rendering
	if mismatch := compareFilmCheckpoint(saved, p.FilmCheckpoint()); mismatch != "" {
		t.Fatal("second restoration", mismatch)
	}
	send(Actions{})
	if e = c.AdvanceRendering(.12); e != nil {
		t.Fatal(e)
	}
	wingmen("b")
	seen := map[navigation.Point]bool{}
	for i := 1; i < 5; i++ {
		berth := p.carrierBerths[i]
		if !berth.Present() || seen[berth] {
			t.Fatal("missing or shared berth", i, berth)
		}
		seen[berth] = true
		if q := p.Flight.Navigation[i].Point(1); q.H0 != berth.H0 || q.H1 != berth.H1 {
			t.Fatal("return differs from assigned berth", i, q, berth)
		}
	}
	for i := 0; i < 1500; i++ {
		if e := c.advanceFlight(.12); e != nil {
			t.Fatal(e)
		}
		landed := true
		for i := 1; i < 5; i++ {
			v := p.Initialized.Entities[i]
			berth := p.carrierBerths[i]
			landed = landed && v.Kind == 0x20 && v.Participation == 1 && v.Pose.Height == 362 && v.Pose.H0 == berth.H0 && v.Pose.H1 == berth.H1
		}
		if landed {
			t.Log("all four landed", p.Flight.Steps)
			break
		}
		if i == 1499 {
			t.Fatal("incomplete landings", p.Initialized.Entities[1].Pose, p.Initialized.Entities[1].Kind, p.Flight.Steps)
		}
	}
	// View across the island from both sides. Removing a hidden aircraft must
	// leave all pixels unchanged; the unobstructed view must lose aircraft pixels.
	for _, floating := range []bool{false, true} {
		for _, side := range []int32{-1, 1} {
			scene := p.Scene
			scene.FloatingGeometry = floating
			slot := 3
			h := p.carrierBerths[slot]
			scene.Camera = render3d.Camera{Pose: render3d.Pose{H0: h.H0 + uint32(side*1700), H1: h.H1, Height: 460}, Angles: [3]uint16{0, uint16(side * 0x4000), 0}}
			draw := func(scene render3d.Scene) []byte {
				frame := make([]byte, scene.TargetPixels())
				v := p.Visual.Clone()
				if _, err := p.Renderer.Draw(scene, &v, frame); err != nil {
					t.Fatal(err)
				}
				return frame
			}
			frame := draw(scene)
			parkingCapture(t, p, fmt.Sprintf("carrier-float-%t-side-%d", floating, side), frame)
			scene.Instances = slices.DeleteFunc(slices.Clone(scene.Instances), func(in render3d.Instance) bool { return in.ID == scene.DeckCrew[slot] })
			same := bytes.Equal(frame, draw(scene))
			if same != (side == 1) {
				t.Fatal("island occlusion", floating, side, same)
			}
		}
	}
	wingmen("f")
	for i := 0; i < 90; i++ {
		if e = c.advanceFlight(.12); e != nil {
			t.Fatal(e)
		}
	}
	for i := 1; i < 5; i++ {
		e := p.Initialized.Entities[i]
		if e.Kind == 0x20 && e.Pose.Height == 362 {
			t.Fatal("failed to leave berth", i)
		}
	}
	send(Actions{EndMission: true})
	send(Actions{ConfirmQuit: true})
	verifyFilm(t, c)
}
