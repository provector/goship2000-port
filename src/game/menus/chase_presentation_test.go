package menus

import (
	"gunship2000-assets/game/render3d"
	gamestate "gunship2000-assets/game/state"
	"gunship2000-assets/game/world"
	"testing"
)

func TestTrackedCameraDoesNotSnapForParkedPlayerContact(t *testing.T) {
	p := &RenderingPreview{Initialized: &gamestate.InitializedMission{PlayerID: "player"}, ViewControls: &render3d.CameraController{Mode: 4, TrackedID: "wingman"}}
	for _, mode := range []string{"smooth", "stepped"} {
		p.presentation.mode = mode
		for _, change := range [][2]bool{{false, false}, {true, false}, {false, true}, {true, true}} {
			if p.presentationDiscontinuity(false, change[0], change[1]) {
				t.Fatalf("%s independent tracked camera snapped for player contact/boundary%v", mode, change)
			}
		}
		if !p.presentationDiscontinuity(true, false, false) {
			t.Fatal("projectile return lost its discontinuity")
		}
	}
	for _, mode := range []uint8{0, 1, 2, 3, 5, 6, 7, 8, 9, 10} {
		p.ViewControls.Mode = mode
		// R1.2 qualifies R1.1's overly broad contact expectation: source
		// contact flutter is continuous presentation, not a camera teleport.
		if p.presentationDiscontinuity(false, true, false) || !p.presentationDiscontinuity(false, false, true) {
			t.Fatalf("mode%d contact/boundary ownership", mode)
		}
	}
	p.ViewControls.Mode = 4
	p.ViewControls.TrackedID = "player"
	if p.presentationDiscontinuity(false, true, false) || !p.presentationDiscontinuity(false, false, true) {
		t.Fatal("ownship tracking contact/boundary ownership")
	}
}

// A display-only receipt: identical ten authoritative tracked-camera endpoints,
// with player landed/airborne/landed contact transitions. This is deliberately a
// supplied-state native interpolation test, not a measured desktop frame rate.
func TestTrackedCameraCadenceIgnoresPlayerLanding(t *testing.T) {
	for _, mode := range []string{"stepped", "smooth"} {
		for _, contact := range []bool{false, true} {
			p := &RenderingPreview{Initialized: &gamestate.InitializedMission{PlayerID: "player"}, ViewControls: &render3d.CameraController{Mode: 4, TrackedID: "wingman"}, Renderer: &render3d.Renderer{Assets: &world.Bundle{Sine: make([]int16, 1025)}}}
			p.presentation.mode = mode
			p.resetPresentation()
			seen := map[uint32]bool{}
			for tick := 0; tick < 10; tick++ {
				p.beginPresentationStep()
				p.Scene.Camera.H0 += 60
				p.endPresentationStep()
				if p.presentationDiscontinuity(false, contact, false) {
					p.resetPresentation()
				}
				for frame := 0; frame < 6; frame++ {
					p.presentation.alpha = float64(frame) / 6
					s, _ := p.presentationInputs()
					seen[s.Camera.H0] = true
				}
			}
			want := 10
			if mode == "smooth" {
				want = 60
			}
			if len(seen) != want {
				t.Fatalf("%s landed%v %d camera poses, want%d", mode, contact, len(seen), want)
			}
			if p.Scene.Camera.H0 != 600 {
				t.Fatal("presentation altered authoritative endpoint")
			}
			t.Logf("supplied ten endpoint updates/60 draws: mode=%s playerContact=%v distinct=%d endpoint=%d", mode, contact, len(seen), p.Scene.Camera.H0)
		}
	}
}
