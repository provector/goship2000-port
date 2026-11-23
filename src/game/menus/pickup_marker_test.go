package menus

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"gunship2000-assets/game/avionics"
	"gunship2000-assets/game/navigation"
	"gunship2000-assets/game/replay"
	gamestate "gunship2000-assets/game/state"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestOrdinaryTroopMapSymbolNeedsActualSighting(t *testing.T) {
	p := &RenderingPreview{p4Mechanics: true, Flight: &FlightRuntime{Avionics: avionics.New()}, Initialized: &gamestate.InitializedMission{Entities: make([]gamestate.InitializedEntity, 11)}}
	e := &p.Initialized.Entities[10]
	e.RuntimeRow = 10
	e.FlightSlot = -1
	e.Kind = 0x15
	e.Participation = 1
	e.Pose = gamestate.InitializedPose{H0: 123456, H1: 234560}
	if got := p.ContactMapMarkers(); len(got) != 0 {
		t.Fatal("undiscovered pickup revealed", got)
	}
	p.Flight.Avionics.Contacts[10] = avionics.Contact{Flags: 6, Kind: 0x15, H0: e.Pose.H0, H1: e.Pose.H1}
	got := p.ContactMapMarkers()
	if len(got) != 1 || got[0].Symbol != "map-070d" || got[0].Colour != 7 || got[0].H0 != e.Pose.H0 || got[0].H1 != e.Pose.H1 {
		t.Fatal("actual gray infantry glyph", got)
	}
	p.Initialized.Objectives = map[string]gamestate.InitializedObjective{"primary": {SourceRow: 10}}
	p.ResultTargets = &gamestate.MissionTargets{}
	p.ResultTargets.Records[2][32] = 2
	if m := p.contactMapMarkers(true); len(m) != 1 || !m[0].ObjectiveTriangle || got[0].ObjectiveTriangle {
		t.Fatal("GPS-only objective decoration", m)
	}
	p.ResultTargets.Records[2][32] = 0
	if p.contactMapMarkers(true)[0].ObjectiveTriangle {
		t.Fatal("completed pickup still marked")
	}
	// The existing completion event changes contact classification synchronously.
	e.Kind = 0x41
	p.Flight.Avionics.Contacts[10].Kind = 0x41
	if got := p.ContactMapMarkers(); len(got) != 0 {
		t.Fatal("boarded troop mark retained", got)
	}
}

func TestPreparedHUDNavigationWaypointChangeAndHistoricalControl(t *testing.T) {
	for _, scale := range []int{1, 2} {
		c := mastProfileFixture(t, 0, "land", replay.R221Compatibility)
		jFinishFade(t, c)
		p := c.Rendering
		if !p.fieldCorrectionsMechanics {
			t.Fatal("new interpretation missing")
		}
		p.Scene.RenderScale = scale
		p.Frame, p.worldFrame = make([]byte, p.Scene.TargetPixels()), make([]byte, p.Scene.TargetPixels())
		p.presentation.mode = "stepped"
		r := &p.Flight.Navigation[0]
		r.SetIndex(1)
		from := p.Flight.State.Pose
		if err := r.SetPoint(1, navigation.Point{H0: from.H0 + 4096, H1: from.H1 + 4096}); err != nil {
			t.Fatal(err)
		}
		if err := r.SetPoint(2, navigation.Point{H0: from.H0 - 4096, H1: from.H1 + 4096}); err != nil {
			t.Fatal(err)
		}
		r.ToFlight(p.Flight.State)
		p.publishNavigation()
		if !p.Cockpit.Navigation.HUDMarker || !p.Cockpit.Navigation.Active {
			t.Fatal("normal route publication lost HUD marker")
		}
		capture := func(name string, b []byte) {
			root := os.Getenv("GS2000_R221_PICKUP_CAPTURE")
			if root == "" {
				return
			}
			if err := os.MkdirAll(root, 0700); err != nil {
				t.Fatal(err)
			}
			img := image.NewPaletted(image.Rect(0, 0, 320*scale, 200*scale), p.Palette)
			copy(img.Pix, b)
			f, err := os.OpenFile(filepath.Join(root, fmt.Sprintf("hud-scale%d-%s.png", scale, name)), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
			if err != nil {
				t.Fatal(err)
			}
			if err := png.Encode(f, img); err != nil {
				t.Fatal(err)
			}
			if err := f.Close(); err != nil {
				t.Fatal(err)
			}
		}
		state := func() []byte {
			b, err := json.Marshal(struct {
				I any
				S any
				N any
				A any
			}{p.Initialized, p.Flight.State, p.Flight.Navigation, p.Flight.Avionics})
			if err != nil {
				t.Fatal(err)
			}
			return b
		}
		beforeState := state()
		p.Cockpit.Navigation.HUDMarker = false
		p.resetPresentation()
		p.Dirty = true
		before := append([]byte(nil), c.Frame(false)...)
		capture("before-omission", before)
		p.Cockpit.Navigation.HUDMarker = true
		p.resetPresentation()
		p.Dirty = true
		after := append([]byte(nil), c.Frame(false)...)
		capture("after", after)
		if p.Error != nil {
			t.Fatal(p.Error)
		}
		count := 0
		for at, b := range before {
			if after[at] != b {
				x, y := at%(320*scale), at/(320*scale)
				if y < 4*scale || y >= 8*scale || x < 93*scale || x >= 228*scale {
					t.Fatalf("marker changed unrelated pixel%d,%d", x, y)
				}
				count++
			}
		}
		if count == 0 {
			t.Fatal("actual prepared HUD triangle absent")
		}
		if !bytes.Equal(beforeState, state()) {
			t.Fatal("drawing altered mission/navigation/flight/RNG")
		}
		oldBearing := p.Cockpit.Navigation.Bearing
		p.applyNavigationKey("6")
		if !p.Cockpit.Navigation.Active || p.Cockpit.Navigation.Bearing == oldBearing {
			t.Fatal("waypoint change retained old HUD bearing")
		}
		// Boarding does not make the active waypoint disappear; a route is a
		// separate owner. Only changing/clearing its selector removes the pip.
		// Supplied post-boarding facts, not a naturally flown pickup witness:
		// completion clears TARGET status and fills the player's own hold.
		binary.LittleEndian.PutUint16(p.Initialized.Targets.Records[2][32:], 0)
		p.Initialized.Stores.Ships[0].Flags = 6 | 0x18
		p.publishNavigation()
		if !p.Cockpit.Navigation.Active {
			t.Fatal("troop lifecycle wrongly cleared navigation")
		}
		r.SetIndex(0)
		p.publishNavigation()
		if p.Cockpit.Navigation.Active {
			t.Fatal("zero selector retained triangle")
		}
		p.fieldCorrectionsMechanics = false
		r.SetIndex(1)
		p.publishNavigation()
		if p.Cockpit.Navigation.HUDMarker {
			t.Fatal("historical visual interpretation changed")
		}
	}
}
