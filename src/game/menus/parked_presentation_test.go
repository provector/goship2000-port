package menus

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"gunship2000-assets/game/flight"
	"gunship2000-assets/game/navigation"
	"gunship2000-assets/game/render3d"
	gamestate "gunship2000-assets/game/state"
	"gunship2000-assets/game/world"
)

func parkingFixture(carrier bool) *RenderingPreview {
	const middle = uint32(0x41000)
	square := func(r int16, h int32) world.HeightRegion {
		return world.HeightRegion{Points: [][2]int16{{-r, -r}, {-r, r}, {r, r}, {r, -r}, {-r, -r}}, Plane: world.Plane{0, 1, 0, -h}}
	}
	terrain := &world.Geometry{ID: "flat", Heights: []world.HeightRegion{square(4096, 0)}}
	if carrier {
		terrain.Heights[0] = square(4096, -1)
	}
	deck := &world.Geometry{ID: "deck", Heights: []world.HeightRegion{{Points: [][2]int16{{-120, -300}, {-120, 300}, {120, 300}, {120, -300}, {-120, -300}}, Plane: world.Plane{0, 1, 0, -360}}}}
	b := &world.Bundle{Worlds: map[string]*world.World{"TEST": {ID: "TEST", Grid: make([]byte, 4096), Cells: make([]world.Cell, 64)}}, Geometries: map[string]*world.Geometry{"flat": terrain, "heli": {ID: "heli", Extent: 40}, "deck": deck}, Models: []string{"heli", "deck"}}
	for i := range b.Worlds["TEST"].Cells {
		b.Worlds["TEST"].Cells[i].Geometry = "flat"
	}
	m := &gamestate.InitializedMission{World: "TEST", PlayerRow: 0, Entities: make([]gamestate.InitializedEntity, 80), Rows: make([]gamestate.MissionObject, 80), Targets: &gamestate.MissionTargets{}}
	binary.LittleEndian.PutUint32(m.Targets.Records[0][34:], middle)
	binary.LittleEndian.PutUint32(m.Targets.Records[0][38:], middle)
	floor := uint16(0)
	if carrier {
		floor = 360
		m.Entities[6].Pose.H0, m.Entities[6].Pose.H1 = middle, middle
		m.Entities[6].Object.Raw[12] = 1
	}
	m.Entities[0].ID = "player"
	m.Entities[0].Pose = gamestate.InitializedPose{H0: middle + 1000, H1: middle, Height: floor + 2}
	binary.LittleEndian.PutUint32(m.Rows[0].Raw[2:], middle+1000)
	binary.LittleEndian.PutUint32(m.Rows[0].Raw[6:], middle)
	for row := 1; row < 3; row++ {
		e := &m.Entities[row]
		e.ID = fmt.Sprintf("member%d", row)
		e.Kind, e.Participation, e.ControlRaw[1] = 0x20, 1, 2
		e.Pose = gamestate.InitializedPose{H0: middle, H1: middle, Height: floor + 2}
		binary.LittleEndian.PutUint32(m.Rows[row].Raw[2:], middle)
		binary.LittleEndian.PutUint32(m.Rows[row].Raw[6:], middle)
	}
	p := &RenderingPreview{Initialized: m, Renderer: &render3d.Renderer{Assets: b}, Flight: &FlightRuntime{State: &flight.State{Pose: flight.Pose{H0: middle + 1000, H1: middle, Height: int16(floor + 2)}}}}
	p.Scene = render3d.Scene{World: "TEST", Camera: render3d.Camera{Pose: render3d.Pose{H0: middle, H1: middle - 500, Height: floor + 100}}}
	for row := 1; row < 3; row++ {
		e := m.Entities[row]
		p.Scene.Instances = append(p.Scene.Instances, render3d.Instance{ID: e.ID, ModelID: 0, Pose: render3d.Pose{H0: e.Pose.H0, H1: e.Pose.H1, Height: e.Pose.Height}})
	}
	return p
}

func TestParkedSpacingIsPrivateAndDeckSafe(t *testing.T) {
	for _, carrier := range []bool{false, true} {
		t.Run(fmt.Sprint(carrier), func(t *testing.T) {
			p := parkingFixture(carrier)
			before, _ := json.Marshal(p.Initialized)
			sceneBefore := append([]render3d.Instance(nil), p.Scene.Instances...)
			shown, changed := p.parkedPresentation(p.Scene, "member2")
			if !changed || shown.Instances[0].Pose != p.Scene.Instances[0].Pose || shown.Instances[1].Pose == shown.Instances[0].Pose {
				t.Fatal("overlap unchanged or first slot moved", shown.Instances)
			}
			if carrier && shown.Instances[1].H0 != p.Scene.Instances[1].H0 {
				t.Fatal("unsafe carrier side offset rather than safe longitudinal bay", shown.Instances[1])
			}
			o := p.parkedDisplay.offsets["member2"]
			if !p.parkingFootprintSafe(shown.Instances[1].H0, shown.Instances[1].H1, 40, o.floor, p.Initialized.ContactObjects()) {
				t.Fatal("display footprint left support")
			}
			if shown.Camera.H0-p.Scene.Camera.H0 != shown.Instances[1].H0-p.Scene.Instances[1].H0 || shown.Camera.H1-p.Scene.Camera.H1 != shown.Instances[1].H1-p.Scene.Instances[1].H1 {
				t.Fatal("tracked camera did not follow private bay")
			}
			after, _ := json.Marshal(p.Initialized)
			if string(before) != string(after) || !reflect.DeepEqual(sceneBefore, p.Scene.Instances) {
				t.Fatal("display changed simulation or canonical scene")
			}
			again, _ := p.parkedPresentation(p.Scene, "member2")
			if !reflect.DeepEqual(again, shown) {
				t.Fatal("same logical step layout changed on redraw")
			}
		})
	}
}

func TestParkedSpacingReservationsTouchdownBlendAndNoSafeBay(t *testing.T) {
	p := parkingFixture(true)
	e := &p.Initialized.Entities[2]
	e.Kind, e.ControlRaw[1] = 0x22, 8
	binary.LittleEndian.PutUint32(e.ControlRaw[40:], e.Pose.H0)
	binary.LittleEndian.PutUint32(e.ControlRaw[44:], e.Pose.H1)
	p.Scene.Instances[1].Height = 500
	if _, changed := p.parkedPresentation(p.Scene, ""); changed {
		t.Fatal("far airborne model shifted")
	}
	reserved := p.parkedDisplay.offsets[e.ID]
	if reserved.x == 0 && reserved.z == 0 {
		t.Fatal("RTB did not reserve bay before touchdown")
	}
	// Ordering the lower-index aircraft later does not reassign an existing bay.
	p.Initialized.Entities[1].Kind, p.Initialized.Entities[1].ControlRaw[1] = 0x22, 8
	p.Flight.Steps++
	p.parkedPresentation(p.Scene, "")
	if p.parkedDisplay.offsets[e.ID] != reserved {
		t.Fatal("late lower-slot B order changed existing reservation")
	}
	e.Kind, e.ControlRaw[1] = 0x26, 6
	p.Flight.Steps++
	p.Scene.Instances[1].Height = 394 // deck360 +2 +32: half offset.
	half, _ := p.parkedPresentation(p.Scene, "")
	if int32(half.Instances[1].H1-p.Scene.Instances[1].H1) != reserved.z/2 {
		t.Fatal("final descent did not blend offset")
	}
	e.Kind, e.ControlRaw[1] = 0x20, 2
	p.Flight.Steps++
	p.Scene.Instances[1].Height = 362
	full, _ := p.parkedPresentation(p.Scene, "")
	if int32(full.Instances[1].H1-p.Scene.Instances[1].H1) != reserved.z {
		t.Fatal("parked offset differs from reserved bay")
	}
	// A new route order leaves the same private bay continuously; it cannot
	// move ordinary initial lateral formations or already-airborne aircraft.
	e.Kind, e.ControlRaw[1], e.Participation = 0x22, 1, 0
	for _, tc := range []struct {
		name     string
		distance uint32
		height   uint16
		fraction int32
	}{
		{"departure_start", 0, 362, 64}, {"departure_half", 32, 362, 32},
		{"initial_lateral_formation", 400, 362, 0}, {"already_airborne", 0, 462, 0},
	} {
		e.Pose.H0 = reserved.homeX + tc.distance
		p.Scene.Instances[1].H0, p.Scene.Instances[1].Height = e.Pose.H0, tc.height
		p.Flight.Steps++
		shown, _ := p.parkedPresentation(p.Scene, "")
		if int32(shown.Instances[1].H1-p.Scene.Instances[1].H1) != reserved.z*tc.fraction/64 {
			t.Fatal(tc.name, "incorrect departure/neighbor offset")
		}
	}
	e.Kind, e.ControlRaw[1], e.Participation, e.Pose.H0 = 0x20, 2, 1, reserved.homeX
	p.Scene.Instances[1].H0, p.Scene.Instances[1].Height = reserved.homeX, 362
	// Shrink the only support polygon below one full radius. No guessed deck
	// clipping, altitude change or route correction is permitted as a fallback.
	g := p.Renderer.Assets.Geometries["deck"]
	g.Heights[0].Points = [][2]int16{{-20, -20}, {-20, 20}, {20, 20}, {20, -20}, {-20, -20}}
	p.Flight.Steps++
	unchanged, changed := p.parkedPresentation(p.Scene, "")
	if changed || !reflect.DeepEqual(unchanged, p.Scene) {
		t.Fatal("no-safe-bay must retain source overlap")
	}
}

func TestPreparedParkedPairFramesAndCanonicalPurity(t *testing.T) {
	for _, family := range []string{"land", "carrier"} {
		for _, scale := range []int{1, 2} {
			t.Run(fmt.Sprintf("%s/%d", family, scale), func(t *testing.T) {
				var c *FrontEnd
				if family == "land" {
					c = developmentScenarioController(t, "flight-return")
				} else {
					c = mastFixture(t, 0, family)
					jFinishFade(t, c)
				}
				p := c.Rendering
				m := p.Initialized
				if len(m.FlightRows) != 5 {
					t.Fatal("five actual flight members required")
				}
				rows := make([][32]byte, len(m.Rows))
				for i := range rows {
					rows[i] = m.Rows[i].Raw
				}
				home, e := navigation.ResolveHome(rows, m.Entities[1].ControlRaw, 1)
				if e != nil {
					t.Fatal(e)
				}
				floor, ok := p.parkingFloor(home.H0, home.H1, m.ContactObjects())
				if !ok {
					t.Fatal("actual home lacks ground/deck", home)
				}
				for row := 1; row < 3; row++ {
					a := &m.Entities[row]
					a.Kind, a.Participation, a.ControlRaw[1] = 0x20, 1, 2
					a.Pose.H0, a.Pose.H1, a.Pose.Height = home.H0, home.H1, uint16(floor+2)
				}
				p.filterSceneInstances()
				p.Scene.RenderScale = scale
				p.Frame = make([]byte, p.Scene.TargetPixels())
				p.worldFrame = make([]byte, p.Scene.TargetPixels())
				p.Scene.Camera = render3d.Camera{Pose: render3d.Pose{H0: home.H0 + 600, H1: home.H1 + 600, Height: uint16(floor + 512)}, Angles: [3]uint16{0xea00, 0x6000, 0}}
				p.Cockpit.CameraMode, p.ViewControls.Mode, p.Cockpit.HUD = 1, 1, false
				p.presentation.mode = "stepped"
				p.resetPresentation()
				p.parkedDisplay = parkedDisplayState{valid: true, step: p.Flight.Steps}
				p.Dirty = true
				c.Frame(false)
				before := append([]byte(nil), p.Frame...)
				parkingCapture(t, p, fmt.Sprintf("%s-%dx-before", family, scale), before)
				checkpoint := p.FilmCheckpoint()
				state, _ := json.Marshal(m)
				canonical := append([]byte(nil), p.avionicsReference...)
				p.parkedDisplay.valid = false
				p.Dirty = true
				c.Frame(false)
				after := append([]byte(nil), p.Frame...)
				parkingCapture(t, p, fmt.Sprintf("%s-%dx-after", family, scale), after)
				if p.Error != nil || reflect.DeepEqual(before, after) || len(p.parkedDisplay.offsets) == 0 {
					t.Fatal("actual pair did not show safe private offset", family, scale, p.Error, p.parkedDisplay)
				}
				unchanged, _ := json.Marshal(m)
				if string(state) != string(unchanged) || !reflect.DeepEqual(checkpoint, p.FilmCheckpoint()) || !reflect.DeepEqual(canonical, p.avionicsReference) {
					t.Fatal("parked presentation changed authoritative/reference state")
				}
				t.Logf("actual%s world%s home%+v support%d layout%+v", family, m.World, home, floor, p.parkedDisplay.offsets)
				// Supplied source successors demonstrate the last64-height visual
				// blend on actual geometry. This is not a flown AI/landing witness.
				member := &m.Entities[2]
				for _, height := range []int16{66, 34, 2} {
					member.Kind, member.ControlRaw[1], member.Participation = 0x26, 6, 1
					member.Pose.Height = uint16(floor + height)
					p.Flight.Steps++
					p.filterSceneInstances()
					p.resetPresentation()
					p.Dirty = true
					c.Frame(false)
					if p.Error != nil {
						t.Fatal(p.Error)
					}
					parkingCapture(t, p, fmt.Sprintf("%s-%dx-descent-%02d", family, scale, height), p.Frame)
				}
			})
		}
	}
}

func parkingCapture(t *testing.T, p *RenderingPreview, name string, pixels []byte) {
	t.Helper()
	dir := os.Getenv("GS2000_R221_PARKING_CAPTURE")
	if dir == "" {
		return
	}
	if e := os.MkdirAll(dir, 0700); e != nil {
		t.Fatal(e)
	}
	scale := p.Scene.Scale()
	im := image.NewPaletted(image.Rect(0, 0, 320*scale, 200*scale), p.Palette)
	copy(im.Pix, pixels)
	f, e := os.OpenFile(filepath.Join(dir, name+".png"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		t.Fatal(e)
	}
	if e = png.Encode(f, im); e != nil {
		t.Fatal(e)
	}
	if e = f.Close(); e != nil {
		t.Fatal(e)
	}
}
