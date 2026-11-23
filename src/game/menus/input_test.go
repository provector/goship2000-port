package menus

import (
	"encoding/binary"
	"encoding/json"
	"gunship2000-assets/game/replay"
	gamestate "gunship2000-assets/game/state"
	"reflect"
	"testing"
	"time"
)

func TestJettisonSourceArithmeticAndStationIsolation(t *testing.T) {
	for _, tc := range []struct {
		name                             string
		station                          int
		flags                            uint16
		ammo, unit, hardware, mass, want int16
	}{
		{"cannon-signed-division", 0, 0, 199, 3, 7, 1000, 966},
		{"direct-mount", 1, 0, 5, 3, 7, 1000, 978},
		{"flag1", 2, 1, 199, 3, 7, 1000, 966},
		{"flag1000-direct", 2, 0x1001, 5, 3, 7, 1000, 978},
		{"negative-nonzero", 1, 1, -199, 3, 7, 1000, 1020},
		{"lowword-wrap", 0, 0, -32768, 20, 7, 32760, -23},
		{"empty-no-hardware-loss", 1, 0, 0, 3, 7, 1000, 1000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := gamestate.RuntimeWeapon{CompactID: 2}
			binary.LittleEndian.PutUint16(w.Raw[:], tc.flags)
			binary.LittleEndian.PutUint16(w.Raw[11:], uint16(tc.unit))
			binary.LittleEndian.PutUint16(w.Raw[13:], uint16(tc.hardware))
			m := &gamestate.InitializedMission{SelectedStation: tc.station, Stores: &gamestate.RuntimeStores{Ships: []gamestate.RuntimeAircraft{{}}, Definitions: []gamestate.RuntimeWeapon{w}}}
			for i := range m.Stores.Ships[0].Stations {
				m.Stores.Ships[0].Stations[i] = gamestate.RuntimeStation{CompactID: 2, Ammunition: 77, Present: true, Flags: 19}
			}
			m.Stores.Ships[0].Stations[tc.station].Ammunition = tc.ammo
			before := m.Stores.Ships[0]
			p := &RenderingPreview{Initialized: m, Flight: &FlightRuntime{MassTerm: tc.mass}}
			if err := p.jettisonSelectedStation(); err != nil {
				t.Fatal(err)
			}
			if p.Flight.MassTerm != tc.want || m.SelectedStation != tc.station {
				t.Fatal(p.Flight.MassTerm, tc.want, m.SelectedStation)
			}
			for i, st := range m.Stores.Ships[0].Stations {
				want := before.Stations[i]
				if i == tc.station {
					want.Ammunition = 0
				}
				if st != want {
					t.Fatal("unrelated fields/stores changed", i, st, want)
				}
			}
		})
	}
}

func TestJettisonUnknownDefinitionDoesNotMutate(t *testing.T) {
	m := &gamestate.InitializedMission{SelectedStation: 1, Stores: &gamestate.RuntimeStores{Ships: []gamestate.RuntimeAircraft{{}}}}
	m.Stores.Ships[0].Stations[1] = gamestate.RuntimeStation{CompactID: 16, Ammunition: 3}
	p := &RenderingPreview{Initialized: m, Flight: &FlightRuntime{MassTerm: 300}}
	before := m.Stores.Ships[0]
	if p.jettisonSelectedStation() == nil || m.Stores.Ships[0] != before || p.Flight.MassTerm != 300 {
		t.Fatal("guard changed stores/mass")
	}
	m.Stores.Ships[0].Stations[1].Ammunition = 0
	if err := p.jettisonSelectedStation(); err != nil {
		t.Fatal("source empty exits before definition read", err)
	}
}

func TestLogicalEdgesCoalesceAndDoNotCycleWeapon(t *testing.T) {
	var l inputLatch
	var q flightCommandInput
	for i, b := range []Buttons{{LastMessage: true, Jettison: true, OrdnanceNext: true}, {LastMessage: true, Jettison: true}, {}, {LastMessage: true}} {
		q.SampleAt(time.Duration(i)*time.Millisecond, l.Read(b))
	}
	q.NextAt(time.Second)
	if got := q.TakeAux(); !reflect.DeepEqual(got, []int{11, 12, 11}) {
		t.Fatal("held/release/order/cycle", got)
	}
	q.NextAt(2 * time.Second)
	if got := q.TakeAux(); len(got) != 0 {
		t.Fatal("repeated discrete input", got)
	}
	for _, token := range []string{"last-message", "jettison"} {
		var b Buttons
		applyControllerCommand(&b, "flight", token, true, nil)
		a := (&inputLatch{}).Read(b)
		if token == "last-message" && !a.LastMessage || token == "jettison" && !a.Jettison {
			t.Fatal(token, a)
		}
	}
}

func TestLaunchProfileAndInputDomainStayVersioned(t *testing.T) {
	for _, sim := range []string{replay.R4Compatibility, replay.R4AuthoredCompatibility, replay.R3Compatibility, replay.R3AuthoredCompatibility} {
		f := &replay.Film{Header: replay.Header{Simulation: sim, InputSchema: replay.R1InputCompatibility}}
		if !f.PlayerCopilotMechanics() || !f.R1Mechanics() || !f.SupportInput() {
			t.Fatal("lost inherited mechanics", sim)
		}
		if f.ReadyRepairsMechanics() != (sim == replay.R4Compatibility || sim == replay.R4AuthoredCompatibility) {
			t.Fatal(sim)
		}
		if f.Authored() != (sim == replay.R4AuthoredCompatibility || sim == replay.R3AuthoredCompatibility) {
			t.Fatal("authored wrong", sim)
		}
	}
	// New device events cannot grow historical boundary JSON payloads.
	b, _ := json.Marshal(Actions{LastMessage: true, Jettison: true})
	var decoded map[string]any
	_ = json.Unmarshal(b, &decoded)
	if _, ok := decoded["LastMessage"]; ok {
		t.Fatal(string(b))
	}
	if _, ok := decoded["Jettison"]; ok {
		t.Fatal(string(b))
	}
}

func TestPhysicalUtilityChordReleaseOrderFocusAndContext(t *testing.T) {
	for _, name := range []string{"recall", "jettison"} {
		t.Run(name, func(t *testing.T) {
			var k utilityKeyChords
			alt, shift := name == "recall", name == "jettison"
			m, space := alt, shift
			r, j := k.sample(true, true, alt, false, shift, m, space)
			if r != alt || j != shift {
				t.Fatal("fresh chord", r, j)
			}
			if r, j = k.sample(true, true, alt, false, shift, m, space); r || j {
				t.Fatal("held repeats")
			}
			if r, j = k.sample(true, true, false, false, false, m, space); r || j {
				t.Fatal("modifier release repeats")
			}
			if m && !k.mConsumed || space && !k.spaceConsumed {
				t.Fatal("base-key action leaked on modifier release")
			}
			k.sample(false, false, false, false, false, false, false)
			if m && !k.mConsumed || space && !k.spaceConsumed {
				t.Fatal("focus cleanup released physical ownership")
			}
			if r, j = k.sample(true, false, alt, false, shift, m, space); r || j {
				t.Fatal("context transition retriggered")
			}
			if r, j = k.sample(true, true, alt, false, shift, m, space); r || j {
				t.Fatal("return from context retriggered")
			}
			k.sample(true, true, false, false, false, false, false)
			if k.mConsumed || k.spaceConsumed {
				t.Fatal("physical release did not rearm")
			}
			if r, j = k.sample(true, true, alt, false, shift, m, space); r != alt || j != shift {
				t.Fatal("second physical press missing")
			}
		})
	}
	var k utilityKeyChords
	r, j := k.sample(true, false, true, false, false, true, false) // Ordinary replay map, not live recall.
	if r || j || !k.mConsumed {
		t.Fatal("replay context", r, j, k)
	}
	r, j = k.sample(true, true, true, false, false, true, false) // Resume while original M is held.
	if r || j {
		t.Fatal("held replay-map press leaked into practice")
	}
	c := &FrontEnd{Screen: ScreenReplay, Viewer: &ReplayViewer{Practice: true}}
	context, _ := controllerContext(c)
	k.sample(true, true, false, false, false, false, false)
	r, _ = k.sample(true, context == "flight", true, false, false, true, false)
	if !r {
		t.Fatal("fresh keyboard recall missing in isolated practice")
	}
}
