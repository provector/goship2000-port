package menus

import (
	"bytes"
	"encoding/json"
	"gunship2000-assets/game/avionics"
	"gunship2000-assets/game/cockpit"
	"gunship2000-assets/game/controllers"
	gamestate "gunship2000-assets/game/state"
	"reflect"
	"testing"
)

func mappedController(c *FrontEnd, id string) (*gameController, *controllerBackend) {
	d := controllers.Device{Instance: 4, Backend: "test/raw", GUID: "r4-mapping", Name: "R4 mapping fixture", Buttons: 1}
	p := controllers.Profile{ID: "r4-mapping", Device: d, CollectiveMode: "absolute", Bindings: map[string]controllers.Binding{id: {Kind: "control", Control: &controllers.Control{Kind: "button", Index: 0}}}}
	b := &controllerBackend{s: controllers.Snapshot{Devices: []controllers.Device{d}, States: map[int32]controllers.State{4: {Buttons: []bool{false}}}}}
	g := &gameController{live: controllers.NewLive(controllers.Config{Version: 2, Selected: p.ID, Profiles: map[string]controllers.Profile{p.ID: p}}, b), previous: map[string]bool{}}
	g.Merge(c, Buttons{}, 0, 0, 0, false, nil) // Fresh selected-device baseline.
	return g, b
}

func mappedPress(c *FrontEnd, id string) (Buttons, Buttons) {
	g, b := mappedController(c, id)
	s := b.s.States[4]
	s.Buttons[0] = true
	b.s.States[4] = s
	press := g.Merge(c, Buttons{}, .01, 0, 0, false, nil)
	held := g.Merge(c, Buttons{}, .01, 0, 0, false, nil)
	g.Close()
	return press, held
}

func TestControllerMFDReusesKeyboardEquipmentDamageAndOverride(t *testing.T) {
	seenInstalled, seenAbsent := false, false
	for aircraft := 0; aircraft < 8; aircraft++ {
		c := mastFixture(t, aircraft, "land")
		jFinishFade(t, c)
		p, r := c.Rendering, c.Rendering.Flight
		family, err := cockpit.Family(aircraft)
		if err != nil {
			t.Fatal(err)
		}
		d := c.Office.Assets.Cockpit.Definitions[family]
		installed := d.Parameters[0x52/2] == d.Parameters[0x5e/2]
		seenInstalled, seenAbsent = seenInstalled || installed, seenAbsent || !installed
		for _, mask := range []byte{0, 0x20} {
			for _, tc := range []struct {
				id    string
				index int
			}{{"view.primary_mfd", 6}, {"view.secondary_mfd", 7}} {
				press, held := mappedPress(c, tc.id)
				var latch inputLatch
				controller := latch.Read(press)
				if len(latch.Read(held).FlightCommands) != 0 {
					t.Fatal("held mapping repeated discrete MFD change", tc.id)
				}
				keyboardButtons := Buttons{}
				keyboardButtons.FlightKeys[tc.index] = true
				keyboard := (&inputLatch{}).Read(keyboardButtons)
				if !reflect.DeepEqual(controller.FlightCommands, keyboard.FlightCommands) || !reflect.DeepEqual(controller.FlightCommands, []int{tc.index}) {
					t.Fatal("controller bypassed keyboard semantic owner", tc.id, controller, keyboard)
				}
				var frames [2][]byte
				for source, a := range []Actions{keyboard, controller} {
					r.PrimaryMFD, r.SecondaryMFD = 5, 5
					r.Systems.DamageMasks[0] = mask
					for _, code := range a.FlightCommands {
						p.applyFlightCommand(code, c.Office.Assets.Cockpit)
					}
					wantPrimary, wantSecondary := int16(5), int16(5)
					if tc.index == 6 {
						wantPrimary = 0
					} else if installed {
						wantSecondary = -1
						if r.SystemFlags&1 == 0 {
							wantSecondary = 0
						}
					}
					if r.PrimaryMFD != wantPrimary || r.SecondaryMFD != wantSecondary || r.Systems.DamageMasks[0] != mask {
						t.Fatal("equipment/source writer differs", aircraft, tc.id, source, mask, r.PrimaryMFD, r.SecondaryMFD, installed)
					}
					p.publishFlight()
					p.publishAvionics()
					p.Dirty = true
					frames[source] = append([]byte(nil), c.Frame(false)...)
					if p.Error != nil {
						t.Fatal("actual cockpit/MFD composition", aircraft, tc.id, source, p.Error)
					}
				}
				if !bytes.Equal(frames[0], frames[1]) {
					t.Fatal("same command has different keyboard/controller pixels", aircraft, tc.id, mask)
				}
			}
		}
		// Existing acquisition override owns both modes; input dispatch adds no
		// competing mode owner, even on a single-MFD family.
		r.PrimaryMFD, r.SecondaryMFD = 1, 2
		r.Avionics = avionics.New()
		r.Avionics.MFDTargetRequested = true
		p.publishAvionics()
		if r.PrimaryMFD != 5 || r.SecondaryMFD != 5 || r.Avionics.MFDTargetRequested {
			t.Fatal("existing MFD override changed", aircraft, r.PrimaryMFD, r.SecondaryMFD)
		}
	}
	if !seenInstalled || !seenAbsent {
		t.Fatal("fixture did not span actual one/two-MFD equipment", seenInstalled, seenAbsent)
	}
}

func TestControllerEditorInformationMatchesKeyboardAndModalRules(t *testing.T) {
	var frames [2][]byte
	for source := 0; source < 2; source++ {
		c := authoredUIController(t)
		if err := c.openMissionFiles(); err != nil {
			t.Fatal(err)
		}
		editorKey(t, c, "n")
		preview := c.editorAssets().Previews[editorMapKey(c.Editor.Worlds[0])]
		editorClick(t, c, preview.X+2, preview.Y+2)
		// Source information toggles over a selected component, including its
		// still-open option picker; it is not a generic Escape-dismissed modal.
		if err := c.editorArm("defender", 0, 1); err != nil || c.Editor.Stage != "option" {
			t.Fatal("source component option fixture", err, c.Editor.Stage)
		}
		w, err := gamestate.AuthoredWorld(c.editorData(), c.Editor.Document.World)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for option := range c.Editor.Options {
			c.Editor.Option = option
			rows, err := c.editorPreviewRows()
			if err == nil && len(rows) > 0 && c.editorAssets().ModelInformation[int(w.FRMD[rows[0]*8+1])] != "" {
				found = true
				break
			}
		}
		if !found {
			t.Fatal("no actual source information for the category fixture")
		}
		before, _ := json.Marshal(c.Editor.Document)
		plain := append([]byte(nil), c.Frame(false)...)
		var a Actions
		if source == 0 {
			a = Actions{Text: []rune{'?'}}
		} else {
			press, _ := mappedPress(c, "editor.help")
			a = (&inputLatch{}).Read(press)
			if !reflect.DeepEqual(a.Text, []rune{'?'}) {
				t.Fatal("editor controller did not use ordinary information key", a.Text)
			}
		}
		if err := c.Handle(a); err != nil {
			t.Fatal(err)
		}
		if c.Editor.ControlReturn != "information" {
			t.Fatal("information consumer not reached", source, c.Editor.ControlReturn)
		}
		frames[source] = append([]byte(nil), c.Frame(false)...)
		if bytes.Equal(plain, frames[source]) {
			t.Fatal("actual source information did not reach the frame", source)
		}
		// A second fresh logical activation toggles it off while preserving the
		// selected placement option, just like the working keyboard consumer.
		if err := c.Handle(a); err != nil {
			t.Fatal(err)
		}
		if c.Editor.Stage != "option" || c.Editor.ControlReturn != "" {
			t.Fatal("information changed the option picker owner", source)
		}
		editorAction(t, c, Actions{Cancel: true})
		editorKey(t, c, "t")
		if c.Editor.Stage != "numeric" {
			t.Fatal("numeric owner fixture")
		}
		if err := c.Handle(a); err != nil || c.Editor.Stage != "numeric" || c.Editor.ControlReturn != "" {
			t.Fatal("information escaped a text/numeric owner", source, err)
		}
		after, _ := json.Marshal(c.Editor.Document)
		if !bytes.Equal(before, after) {
			t.Fatal("information mutated authored mission", source)
		}
	}
	if !bytes.Equal(frames[0], frames[1]) {
		t.Fatal("information rendering differs between keyboard/controller")
	}
}
