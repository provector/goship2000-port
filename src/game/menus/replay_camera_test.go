package menus

import (
	"bytes"
	"encoding/json"
	"gunship2000-assets/game/render3d"
	"gunship2000-assets/game/replay"
	gamestate "gunship2000-assets/game/state"
	"gunship2000-assets/game/world"
	"testing"
)

func replayFloorFixture(support int16, modelSupport bool) *FilmPlayback {
	polygon := [][2]int16{{-4096, -4096}, {-4096, 4096}, {4096, 4096}, {4096, -4096}, {-4096, -4096}}
	region := func(height int16) []world.HeightRegion {
		return []world.HeightRegion{{Points: polygon, Plane: world.Plane{0, 1, 0, -int32(height)}}}
	}
	air := &world.Geometry{Extent: 64}
	deck := &world.Geometry{}
	terrain := &world.Geometry{Heights: region(support)}
	if modelSupport {
		deck.Heights, terrain.Heights = region(support), region(-1)
	}
	b := &world.Bundle{Sine: make([]int16, 1025), Models: []string{"air", "deck"}, Geometries: map[string]*world.Geometry{"air": air, "deck": deck, "terrain": terrain}, Worlds: map[string]*world.World{}}
	b.Sine[1024] = 16384
	cells := make([]world.Cell, 64)
	for i := range cells {
		cells[i].Geometry = "terrain"
	}
	b.Worlds["TEST"] = &world.World{Grid: make([]byte, 4096), Cells: cells}
	body := gamestate.InitializedEntity{RuntimeRow: 0, ModelID: 0, Kind: 0xff, Pose: gamestate.InitializedPose{H0: 135168, H1: 135168}}
	ship := gamestate.InitializedEntity{RuntimeRow: 1, ModelID: 1, Kind: 0x17, Participation: 1, Pose: body.Pose}
	ship.Object.Raw[12] = 1
	ship.Pose.Height = 0xffff // Source query regions do not add placement height.
	m := &gamestate.InitializedMission{World: "TEST", PlayerRow: 0, Entities: []gamestate.InitializedEntity{body, ship}}
	p := &RenderingPreview{Initialized: m, Renderer: &render3d.Renderer{Assets: b}, Scene: render3d.Scene{World: "TEST"}}
	return &FilmPlayback{Engine: &FrontEnd{Rendering: p}}
}

func TestReplayCameraContactFloorSignedBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		support, height, want int16
		model                 bool
	}{
		{"carrier below", 360, 391, 424, true},
		{"carrier equal", 360, 392, 392, true},
		{"carrier above", 360, 393, 393, true},
		{"actual carrier start", 360, 362, 424, true},
		{"land below", 0, 31, 64, false},
		{"land equal", 0, 32, 32, false},
		{"land above", 0, 33, 33, false},
		{"land airborne unchanged", 0, 362, 362, false},
		{"signed negative below", -1, 30, 63, true},
		{"signed add wraps below", 32760, -32745, -32712, true},
		{"signed add wraps equal", 32760, -32744, -32744, true},
		{"signed add wraps above", 32760, 0, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := replayFloorFixture(tc.support, tc.model)
			q.Preview().Initialized.Entities[0].Pose.Height = uint16(tc.height)
			v := NewReplayViewer(q)
			if got := int16(v.camera.Height); got != tc.want {
				t.Fatalf("height%d want%d (source signed camera < support+32)", got, tc.want)
			}
		})
	}
}

func TestReplayCameraQueriesBeforeHorizontalClamp(t *testing.T) {
	q := replayFloorFixture(10, false)
	p := q.Preview()
	p.Initialized.Entities = p.Initialized.Entities[:1]
	p.Initialized.Entities[0].Pose = gamestate.InitializedPose{H0: 0x4200, H1: 0x4000 + 100, Height: 100}
	b := p.Renderer.Assets
	b.Geometries["edge"] = &world.Geometry{Heights: []world.HeightRegion{{Points: b.Geometries["terrain"].Heights[0].Points, Plane: world.Plane{0, 1, 0, -300}}}}
	b.Worlds["TEST"].Cells[1].Geometry = "edge"
	// Unclamped camera H1=16228 is in z-cell1; clamp16384 enters z-cell2.
	b.Worlds["TEST"].Grid[2+64*(63-1)] = 1
	v := NewReplayViewer(q)
	if v.camera.H1 != 0x4000 || v.camera.Height != 364 {
		t.Fatal("query used post-clamp cell instead of source pre-clamp300", v.camera)
	}
}

func TestContactCameraUsesHeightAndActualFootprint(t *testing.T) {
	q := replayFloorFixture(360, true)
	p := q.Preview()
	deck := p.Renderer.Assets.Geometries["deck"]
	roof := deck.Heights[0]
	roof.Threshold = 400
	roof.Plane = world.Plane{0, 1, 0, -720}
	deck.Heights = append([]world.HeightRegion{roof}, deck.Heights...)
	for _, tc := range []struct{ height, want uint16 }{{399, 399}, {400, 784}, {401, 784}} {
		p.Initialized.Entities[0].Pose.Height = tc.height
		if got := NewReplayViewer(q).camera.Height; got != tc.want {
			t.Fatal("model region did not consume actual camera height", tc, got)
		}
	}
	// The camera, unlike the aircraft, lies outside this small model footprint.
	deck.Heights = []world.HeightRegion{{Points: [][2]int16{{-50, -50}, {-50, 50}, {50, 50}, {50, -50}, {-50, -50}}, Plane: world.Plane{0, 1, 0, -360}}}
	p.Initialized.Entities[0].Pose.Height = 362
	if got := NewReplayViewer(q).camera.Height; got != 362 {
		t.Fatal("support was invented outside its actual footprint", got)
	}
}

func TestOrdinaryCameraContactAtValidWorldEdges(t *testing.T) {
	real := mastFixture(t, 6, "carrier")
	for _, xy := range [][2]uint32{{0x4000, 0x4000}, {0x7c000, 0x4000}, {0x4000, 0x7c000}, {0x7c000, 0x7c000}} {
		for _, mode := range []uint8{3, 4, 5, 6, 9, 10} {
			q := replayFloorFixture(360, true)
			p, a := q.Preview(), q.Preview().Renderer.Assets
			a.Sine, a.Atan, a.AngleBases = real.Rendering.Renderer.Assets.Sine, real.Rendering.Renderer.Assets.Atan, real.Rendering.Renderer.Assets.AngleBases
			pose := render3d.Pose{H0: xy[0], H1: xy[1], Height: 100}
			for i := range p.Initialized.Entities {
				p.Initialized.Entities[i].Pose.H0, p.Initialized.Entities[i].Pose.H1 = xy[0], xy[1]
			}
			anchor := render3d.CameraAnchor{ID: "body", Extent: 64, Camera: render3d.Camera{Pose: pose}}
			calls := 0
			in := render3d.CameraInputs{Player: anchor, Target: &anchor, SourceFloorRules: true, ProjectileView: &render3d.ProjectileCameraInputs{LastActive: true, Lifetime: 1, Current: anchor}, GroundHeight: func(v render3d.Pose) (int16, error) {
				calls++
				return a.ContactHeight("TEST", world.QueryPose{H0: v.H0, H1: v.H1, Height: int16(v.Height)}, p.Initialized.ContactObjects())
			}}
			c := render3d.NewCameraController(in)
			c.Mode, c.TrackedID = mode, anchor.ID
			// Mode5's initial distance is too short until its first established
			// remote offset. Select F7 supplies the source +0x180 initial offset.
			if mode == 5 {
				if _, err := c.Select(7, false, in, a); err != nil {
					t.Fatal(xy, mode, err)
				}
				// Reach1942's established-offset fallback, not the normal
				// remote branch that intentionally performs no floor query.
				in.Player.Camera.H0 += 0x1800
			}
			if err := c.Tick(in, a); err != nil {
				t.Fatal("valid edge Contact314C query", xy, mode, err)
			}
			if calls != 1 {
				t.Fatal("missing camera floor query", xy, mode, calls)
			}
		}
	}
}

func TestCameraProfileSelectedBeforeInitialization(t *testing.T) {
	c := mastFixture(t, 6, "carrier")
	jFinishFade(t, c)
	if !c.Rendering.cameraContactMechanics || c.Rendering.filmRecorder == nil || c.Rendering.filmRecorder.Film.Header.Simulation != replay.R12Compatibility {
		t.Fatal("new launch did not record current rules before first checkpoint")
	}
	for _, profile := range []string{replay.R11NavigationCompatibility, replay.R11NavigationAuthoredCompatibility, replay.R11WaypointReleaseCompatibility, replay.R11WaypointReleaseAuthoredCompatibility, replay.R11AvoidanceCompatibility, replay.R11AvoidanceAuthoredCompatibility, replay.R11CameraCompatibility, replay.R11CameraAuthoredCompatibility, replay.R11Compatibility, replay.R11AuthoredCompatibility, replay.R1Compatibility} {
		final, err := cloneFilmFinal(c.Mission.Final)
		if err != nil {
			t.Fatal(err)
		}
		engine := &FrontEnd{simulationProfile: profile, Office: c.Office, Runtime: c.Runtime, Screen: ScreenScene, filmIsolation: &filmSessionIsolation{Mode: "profile-construction-test"}, Mission: &MissionSession{Data: final.Mission, Final: final, Flight: final.Flight, ReadinessRows: final.Aircraft, PilotID: final.PilotID, Settings: final.Settings}}
		if err = engine.constructSimulation(final); err != nil {
			t.Fatal(profile, err)
		}
		p := engine.Rendering
		want := profile == replay.R11NavigationCompatibility || profile == replay.R11NavigationAuthoredCompatibility || profile == replay.R11WaypointReleaseCompatibility || profile == replay.R11WaypointReleaseAuthoredCompatibility || profile == replay.R11AvoidanceCompatibility || profile == replay.R11AvoidanceAuthoredCompatibility || profile == replay.R11CameraCompatibility || profile == replay.R11CameraAuthoredCompatibility
		wantAvoidance := profile == replay.R11NavigationCompatibility || profile == replay.R11NavigationAuthoredCompatibility || profile == replay.R11WaypointReleaseCompatibility || profile == replay.R11WaypointReleaseAuthoredCompatibility || profile == replay.R11AvoidanceCompatibility || profile == replay.R11AvoidanceAuthoredCompatibility
		if p.waypointReleaseMechanics != (profile == replay.R11NavigationCompatibility || profile == replay.R11NavigationAuthoredCompatibility || profile == replay.R11WaypointReleaseCompatibility || profile == replay.R11WaypointReleaseAuthoredCompatibility) || p.routeAvoidanceMechanics != wantAvoidance || (p.Flight.Combat.FilmState().Avoidance != nil) != wantAvoidance {
			t.Fatal("avoidance profile not frozen before first state", profile)
		}
		in := p.cameraInputs()
		v := p.Initialized.Entities[0].Pose
		height, err := in.GroundHeight(render3d.Pose{H0: v.H0, H1: v.H1, Height: v.Height})
		if err != nil || p.cameraContactMechanics != want || in.SourceFloorRules != want || want && height != 360 || !want && height != -1 {
			t.Fatal("profile callback selected after initialization or not frozen", profile, want, height, err)
		}
	}
}

func TestReplayCameraActualCarrierIsObservational(t *testing.T) {
	c := mastFixture(t, 6, "carrier")
	jFinishFade(t, c)
	p := c.Rendering
	before, _ := json.Marshal(p.FilmCheckpoint())
	career, _ := json.Marshal(c.Store.State())
	v := NewReplayViewer(&FilmPlayback{Engine: c})
	query := world.QueryPose{H0: v.camera.H0, H1: v.camera.H1, Height: 362}
	h, err := p.Renderer.Assets.ContactHeight(p.Scene.World, query, p.Initialized.ContactObjects())
	if err != nil || h != 360 || v.camera.Height != 424 {
		t.Fatal("actual retained carrier model support unavailable", h, v.camera, err)
	}
	for i := 0; i < 20; i++ {
		v.updateCamera()
	}
	after, _ := json.Marshal(p.FilmCheckpoint())
	careerAfter, _ := json.Marshal(c.Store.State())
	if !bytes.Equal(before, after) || !bytes.Equal(career, careerAfter) {
		t.Fatal("observational viewer camera changed recorded simulation or career")
	}
}
