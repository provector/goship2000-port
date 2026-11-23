package menus

import (
	"bytes"
	"gunship2000-assets/game/cockpit"
	"gunship2000-assets/game/navigation"
	gamestate "gunship2000-assets/game/state"
	"testing"
)

func TestSupportStrikeSnapshotOldProfileAndGPSClearing(t *testing.T) {
	c := artilleryController(t, 0)
	p := c.Rendering
	s := p.Initialized.Support
	before := p.FilmCheckpoint()
	rng := p.Initialized.RandomState
	p.p7Mechanics = false
	s.Target = gamestate.InitializedPose{H0: 200000, H1: 280000}
	s.Request = 255
	p.publishSupportStrike()
	if p.Cockpit.Map.SupportStrike != nil {
		t.Fatal("historical MFD reference silently upgraded")
	}
	p.p7Mechanics = true
	p.publishSupportStrike()
	want := cockpit.MapPoint{H0: 200000, H1: 280000}
	if p.Cockpit.Map.SupportStrike == nil || *p.Cockpit.Map.SupportStrike != want {
		t.Fatal("active actual source point absent")
	}
	// The snapshot is detached and presentation-only; no request, step or RNG
	// is advanced by publication or drawing. The synthetic request here is an
	// isolated producer fixture; complete actual commands are tested separately.
	frozen := filmJSON(p.Initialized)
	s.Target.H0++
	if p.Cockpit.Map.SupportStrike.H0 != 200000 {
		t.Fatal("drawing retained mutable runtime alias")
	}
	s.Target.H0--
	editorAction(t, c, Actions{GPSOpen: true})
	if p.GPS == nil || p.GPS.SupportStrike == nil || *p.GPS.SupportStrike != want {
		t.Fatal("GPS entry lost active point")
	}
	modal := filmJSON(p.GPS.ReplayState())
	frame := c.gpsFrame()
	marker := bytes.Count(frame, []byte{12})
	for n := 0; n < 3; n++ {
		c.gpsFrame()
	}
	if modalAfter := filmJSON(p.GPS.ReplayState()); !bytes.Equal(modal, modalAfter) || rng != p.Initialized.RandomState || !bytes.Equal(frozen, filmJSON(p.Initialized)) {
		t.Fatal("draw/publication mutated simulation or recorded modal")
	}
	s.Request = 0
	p.publishSupportStrike()
	if p.GPS.SupportStrike != nil || p.Cockpit.Map.SupportStrike != nil {
		t.Fatal("completed request did not clear both snapshots")
	}
	cleared := c.gpsFrame()
	if bytes.Equal(frame, cleared) || bytes.Count(cleared, []byte{12}) >= marker {
		t.Fatal("red strike marker retained after request0")
	}
	// The presentation pointer itself is deliberately absent from the modal
	// record, even while detail/overview draw different transformed positions.
	if bytes.Contains(modal, []byte("SupportStrike")) {
		t.Fatal("marker became a mutable checkpoint owner")
	}
	_ = before
	c.AbandonFilmRecording()
}

func TestSupportStrikeDrawsAtActualProjectedPointAcrossGPSMaps(t *testing.T) {
	c := artilleryController(t, 0)
	p := c.Rendering
	editorAction(t, c, Actions{GPSOpen: true})
	point := navigation.HighPoint(120, 90)
	p.Initialized.Support.Request = 255
	p.Initialized.Support.Target.H0 = point.H0
	p.Initialized.Support.Target.H1 = point.H1
	p.publishSupportStrike()
	for _, detail := range []bool{false, true} {
		g := p.GPS
		g.Low = detail
		if detail {
			g.Center = point
		}
		frame := c.gpsFrame()
		x, y := g.Pixel(point)
		if frame[y*320+x] != 12 {
			t.Fatalf("actual active strike center detail%v at%d,%d got%d", detail, x, y, frame[y*320+x])
		}
		if detail {
			x, y = 230+int(int32(point.H0)/0x1702), 66+int(int32(point.H1)/-0x2000)
			if frame[y*320+x] != 12 {
				t.Fatal("reduced overview strike absent", x, y)
			}
		}
	}
	c.AbandonFilmRecording()
}
