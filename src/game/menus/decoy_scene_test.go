package menus

import (
	"bytes"
	"encoding/json"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/game/configuration"
	"gunship2000-assets/game/flight"
	"gunship2000-assets/game/render3d"
	"gunship2000-assets/game/replay"
	"gunship2000-assets/game/world"
	"gunship2000-assets/internal/testfixture"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestDecoySharedPresentationOwnership(t *testing.T) {
	p, _, _ := weaponFixture(t, 0x0e10)
	raw, err := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if err != nil {
		t.Fatal(err)
	}
	geometries, err := world.PlayerDecoyGeometries(raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range geometries {
		p.Renderer.Assets.Geometries[g.ID] = g
	}
	d := &p.Flight.Defenses
	*d = flight.NewDefenses(30, 30)
	d.EnableR3()
	pose := flight.Pose{H0: 0x40000, H1: 0x40000, Height: 512, Angles: [3]uint16{0x1000, 0x2000, 0x3000}}
	check := func(id string, active bool) {
		t.Helper()
		before, _ := json.Marshal(d)
		for i := 0; i < 4; i++ {
			p.refreshPlayerDecoyScene()
		}
		after, _ := json.Marshal(d)
		if !bytes.Equal(before, after) {
			t.Fatal("painting changed defense state")
		}
		count := 0
		for _, in := range p.Scene.Instances {
			if in.ID != playerDecoyInstanceID {
				continue
			}
			count++
			if in.GeometryID != id || in.Angles != [3]uint16{} || in.Oriented || in.Callback != 0 || in.Pose != (render3d.Pose{H0: d.Decoy.H0, H1: d.Decoy.H1, Height: uint16(d.Decoy.Height)}) {
				t.Fatal("effect not shared position-only descriptor", in)
			}
		}
		if active && count != 1 || !active && count != 0 {
			t.Fatal("effect count", count, active)
		}
	}
	check("", false)
	if !d.Activate(flight.LaunchFlare, 0, pose, 2, p.Renderer.Assets, nil) {
		t.Fatal("manual flare refused")
	}
	check(world.PlayerFlareGeometry, true)
	d.AdvanceDecoy()
	check(world.PlayerFlareGeometry, true)
	// A failed deployment cannot replace the descriptor or an active effect.
	if d.Activate(flight.LaunchChaff, 0x40, pose, 2, p.Renderer.Assets, nil) {
		t.Fatal("damaged equipment deployed")
	}
	check(world.PlayerFlareGeometry, true)
	// Simultaneous automatic arms use IR first/radar second. Their existing
	// consumer leaves one chaff selector, not two particles or3 per cartridge.
	d.FlareTimer, d.ChaffTimer = 0, 0
	if _, err = d.ApplyR3Automatic(flight.R3DefenseRequest{Switches: 8, E0: true, Phase: 2, Threats: 0xc0, Pose: pose, DF: 2, Angles: p.Renderer.Assets}, nil); err != nil {
		t.Fatal(err)
	}
	if d.FlareTimer == 0 || d.ChaffTimer == 0 {
		t.Fatal("simultaneous fixture did not activate both timers")
	}
	check(world.PlayerChaffGeometry, true)
	if !d.Activate(flight.LaunchFlare, 0, pose, 2, p.Renderer.Assets, nil) {
		t.Fatal("later manual flare refused")
	}
	check(world.PlayerFlareGeometry, true)
	d.Chaff = 1
	if !d.Activate(flight.LaunchChaff, 0, pose, 2, p.Renderer.Assets, nil) || d.Chaff != -2 {
		t.Fatal("source signed depletion fixture failed", d.Chaff)
	}
	check(world.PlayerChaffGeometry, true)
	if d.Activate(flight.LaunchChaff, 0, pose, 2, p.Renderer.Assets, nil) {
		t.Fatal("exhausted chaff redeployed")
	}
	check(world.PlayerChaffGeometry, true) // Active effect outlives the cartridges.
	// Legacy films have no selector owner: do not infer a category from timers.
	owner := d.R3
	d.R3 = nil
	check("", false)
	d.R3 = owner
	for i := 0; i < 10; i++ {
		d.AdvanceCounters(true)
	}
	check("", false)
	// NPC stores/defense do not publish AF5/AF7/3134. No effect is inferred
	// merely because an arbitrary flight member has expended cartridges.
	p.Initialized.Stores.Ships = append(p.Initialized.Stores.Ships, p.Initialized.Stores.Ships[0])
	p.Initialized.Stores.Ships[1].Flares = 27
	check("", false)
}

func TestDecoyNaturalLatePublicationAndCanonicalPurity(t *testing.T) {
	c := naturalCPGController(t, 1)
	p, r := c.Rendering, c.Rendering.Flight
	c.Handle(Actions{FlightCommands: []int{2}})
	for n := 0; n < 40 && r.Defenses.FlareTimer == 0; n++ {
		if err := c.advanceFlight(float64(flight.StepDuration()+time.Nanosecond) / float64(time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	if r.Defenses.FlareTimer == 0 || r.Defenses.R3.StatusPointer != 0x3078 {
		t.Fatal("natural IR encounter did not reach positive late consumer")
	}
	count := 0
	for _, in := range p.Scene.Instances {
		if in.ID == playerDecoyInstanceID {
			count++
			if in.H0 != r.Defenses.Decoy.H0 || in.H1 != r.Defenses.Decoy.H1 || in.Height != uint16(r.Defenses.Decoy.Height) {
				t.Fatal("scene did not publish post-drift decoy", in, r.Defenses.Decoy)
			}
		}
	}
	if count != 1 {
		t.Fatal("late consumer effect absent/duplicated", count, r.Steps)
	}
	// Recompute the canonical display reference with/without the new scene
	// instance. It must exclude presentation-only geometry in both passes.
	seed := append([]byte(nil), p.avionicsReference...)
	bottom := p.avionicsReferenceBottom
	if err := p.advanceAvionicsReference(c.Office.Assets.Cockpit); err != nil {
		t.Fatal(err)
	}
	with := append([]byte(nil), p.avionicsReference...)
	p.avionicsReference, p.avionicsReferenceBottom = append([]byte(nil), seed...), bottom
	p.Scene.Instances = withoutPlayerDecoy(p.Scene.Instances)
	if err := p.advanceAvionicsReference(c.Office.Assets.Cockpit); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(with, p.avionicsReference) {
		t.Fatal("decoy painting changed checkpointed canonical destination")
	}
	p.refreshPlayerDecoyScene()
	var beforeFlags, afterFlags byte
	if err := p.sourceTargetClip(c.Office.Assets.Cockpit, &beforeFlags); err != nil {
		t.Fatal(err)
	}
	p.Scene.Instances = withoutPlayerDecoy(p.Scene.Instances)
	if err := p.sourceTargetClip(c.Office.Assets.Cockpit, &afterFlags); err != nil || beforeFlags != afterFlags {
		t.Fatal("decoy changed logical target clip feedback", beforeFlags, afterFlags, err)
	}
	p.refreshPlayerDecoyScene()
	for _, scale := range []int{1, 2} {
		for _, mode := range []string{"stepped", "smooth"} {
			p.Scene.RenderScale = scale
			p.Frame, p.worldFrame = make([]byte, p.Scene.TargetPixels()), make([]byte, p.Scene.TargetPixels())
			p.presentation.mode = mode
			p.resetPresentation()
			before := p.FilmCheckpoint()
			for n := 0; n < 100; n++ {
				if err := p.draw(c.Office.Assets.Cockpit); err != nil {
					t.Fatal(mode, scale, err)
				}
			}
			if got := p.FilmCheckpoint(); !reflect.DeepEqual(before, got) {
				t.Fatal("100 redraws changed simulation checkpoint", mode, scale, compareFilmCheckpoint(before, got))
			}
			if output := os.Getenv("GS2000_R4_OUTPUT"); output != "" {
				if err := os.MkdirAll(output, 0700); err != nil {
					t.Fatal(err)
				}
				palette, err := p.Renderer.Palette(p.Scene, &p.Visual)
				if err != nil {
					t.Fatal(err)
				}
				image := image.NewPaletted(image.Rect(0, 0, 320*scale, 200*scale), palette)
				copy(image.Pix, p.Frame)
				file, err := os.Create(filepath.Join(output, fmt.Sprintf("decoy-natural-%s-%dx.png", mode, scale)))
				if err != nil {
					t.Fatal(err)
				}
				err = png.Encode(file, image)
				closeErr := file.Close()
				if err != nil || closeErr != nil {
					t.Fatal(err, closeErr)
				}
				if mode == "stepped" {
					writeDecoyRenderViews(t, p, output, scale)
				}
			}
		}
	}
}

// These are explicitly supplied camera views about the naturally released
// effect's real shared position, not an ordinary F5/mast observation. They
// retain the normal world queue/material/clipping pipeline and make the small
// geometry visible for matched pixel inspection without moving the aircraft.
func writeDecoyRenderViews(t *testing.T, p *RenderingPreview, output string, scale int) {
	t.Helper()
	decoy, ok := p.playerDecoyInstance()
	if !ok {
		t.Fatal("active fixture decoy unavailable")
	}
	scene := p.Scene
	scene.RenderScale, scene.View, scene.Environment = scale, render3d.FullView(), nil
	scene.Camera = render3d.Camera{Pose: render3d.Pose{H0: decoy.H0, H1: decoy.H1 - 256, Height: decoy.Height}}
	scene.Instances = withoutPlayerDecoy(scene.Instances)
	palette, err := p.Renderer.Palette(scene, &p.Visual)
	if err != nil {
		t.Fatal(err)
	}
	write := func(name string) {
		t.Helper()
		pixels := make([]byte, scene.TargetPixels())
		if _, err := p.Renderer.Draw(scene, &p.Visual, pixels); err != nil {
			t.Fatal(err)
		}
		image := image.NewPaletted(image.Rect(0, 0, 320*scale, 200*scale), palette)
		copy(image.Pix, pixels)
		file, err := os.Create(filepath.Join(output, fmt.Sprintf("decoy-fixture-%s-%dx.png", name, scale)))
		if err != nil {
			t.Fatal(err)
		}
		err = png.Encode(file, image)
		closed := file.Close()
		if err != nil || closed != nil {
			t.Fatal(err, closed)
		}
	}
	write("before-front")
	for _, effect := range []struct{ name, geometry string }{{"flare", world.PlayerFlareGeometry}, {"chaff", world.PlayerChaffGeometry}} {
		decoy.GeometryID = effect.geometry
		scene.Instances = append(withoutPlayerDecoy(scene.Instances), decoy)
		for _, view := range []struct {
			name   string
			angles [3]uint16
		}{{"front", [3]uint16{}}, {"bank", [3]uint16{0, 0, 0x2000}}, {"turn", [3]uint16{0, 0x1000, 0}}} {
			scene.Camera.Angles = view.angles
			write(effect.name + "-" + view.name)
		}
	}
}

func TestExistingR3FilmStrictWithDecoyPaint(t *testing.T) {
	directory, input := os.Getenv("GS2000_TEST_MISSIONS"), os.Getenv("GS2000_R4_R3_FILM")
	if directory == "" || input == "" {
		t.Skip("prepared assets and retained complete R3 film required")
	}
	raw, err := os.ReadFile(input)
	if err != nil {
		t.Fatal(err)
	}
	f, err := replay.Unmarshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	a, err := assetdata.LoadFrontEndAssets(directory)
	if err != nil {
		t.Fatal(err)
	}
	q, err := NewFilmPlayback(a, f, configuration.RuntimeOptions{DisableEnemyAI: true, DisableWingmenAI: false})
	if err != nil {
		t.Fatal(err)
	}
	seen := false
	resumeOperation := 0
	for !q.Done {
		if err = q.Step(); err != nil {
			t.Fatal("historical R3 strict", q.Cursor, err)
		}
		p := q.Preview()
		if p == nil || p.Flight.Defenses.R3 == nil || p.Flight.Defenses.FlareTimer|p.Flight.Defenses.ChaffTimer == 0 {
			continue
		}
		in, active := p.playerDecoyInstance()
		if !active || in.ID != playerDecoyInstanceID {
			t.Fatal("recorded R3 selector has no display consumer")
		}
		if !seen {
			resumeOperation = q.Cursor
			before := p.FilmCheckpoint()
			p.refreshPlayerDecoyScene()
			if err := p.draw(a.Cockpit); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, p.FilmCheckpoint()) {
				t.Fatal("R3 redraw changed recorded checkpoint")
			}
		}
		seen = true
	}
	if !seen {
		t.Fatal("retained film did not contain an active recorded R3 decoy")
	}
	if q.Engine.Store != nil || q.Engine.Mission.ResultLaunch != nil {
		t.Fatal("strict playback gained career authority")
	}
	practice, err := NewFilmPlayback(a, f, configuration.RuntimeOptions{DisableEnemyAI: true, DisableWingmenAI: false})
	if err != nil {
		t.Fatal(err)
	}
	for practice.Cursor < resumeOperation && !practice.Done {
		if err := practice.Step(); err != nil {
			t.Fatal("R3 decoy Resume prefix", err)
		}
	}
	p := practice.Preview()
	before, _ := json.Marshal(p.Flight.Defenses)
	want, active := p.playerDecoyInstance()
	if !active || practice.Done {
		t.Fatal("active decoy Resume boundary missing")
	}
	if err := practice.Resume(); err != nil {
		t.Fatal(err)
	}
	p = practice.Preview()
	after, _ := json.Marshal(p.Flight.Defenses)
	got, active := p.playerDecoyInstance()
	if !bytes.Equal(before, after) || !active || got != want || practice.Engine.Store != nil || practice.Engine.Mission.ResultLaunch != nil {
		t.Fatal("Resume changed decoy ownership/pose or gained career authority")
	}
	t.Logf("retained R3 complete film strict:%d operations, decoy display observed", q.Cursor)
}
