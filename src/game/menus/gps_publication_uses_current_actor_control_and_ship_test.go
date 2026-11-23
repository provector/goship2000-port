package menus

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"gunship2000-assets/game/combat"
	"gunship2000-assets/game/flight"
	"gunship2000-assets/game/navigation"
	gamestate "gunship2000-assets/game/state"
	"reflect"
	"testing"
)

func TestGPSPublicationUsesCurrentActorControlAndSHIP(t *testing.T) {
	// Supplied initialized/control/SHIP state exercises source ownership rather
	// than injecting a normalized panel. Production launch still admits player0.
	i := &gamestate.InitializedMission{PlayerRow: 1, Entities: []gamestate.InitializedEntity{
		{FlightSlot: 0, Pose: gamestate.InitializedPose{Height: 1}, SerializedAircraftSelector: 6},
		{FlightSlot: 1, Pose: gamestate.InitializedPose{Height: 90}, SerializedAircraftSelector: 0},
	}, Stores: &gamestate.RuntimeStores{Ships: []gamestate.RuntimeAircraft{{Flags: 6 | 2<<3, Fuel: 37, Chaff: -1, Flares: -2,
		Stations: [4]gamestate.RuntimeStation{{CompactID: 0, Present: true}, {CompactID: 1, ShortName: "EMPTY", Ammunition: 17}, {CompactID: 3, ShortName: "GUN", Ammunition: -1}}}, {Flags: 0, Fuel: 91}}, Damage: [][10]byte{{2}, {2}}}}
	copy(i.Entities[0].Object.Raw[15:27], "ACTOR ZERO")
	r := &FlightRuntime{State: &flight.State{Pose: flight.Pose{Height: 7}}, Systems: flight.Systems{DamageMasks: [2]byte{6, 0}}}
	r.Navigation[0].Raw[1], r.Navigation[0].Raw[4] = 2, 1
	binary.LittleEndian.PutUint16(r.Navigation[0].Raw[6:], 3)
	binary.LittleEndian.PutUint16(r.Navigation[0].Raw[8:], 4)
	r.Navigation[3].Raw[5], r.Navigation[4].Raw[5] = 1, 0xff
	g := navigation.New(&r.Navigation, [5]navigation.Point{}, [5]bool{true, true})
	g.Panel = &navigation.PanelSnapshot{}
	g.Panel.Objectives[0].Available = true
	g.Panel.Objectives[0].Raw[61] = 0x9b
	p := &RenderingPreview{Initialized: i, Flight: r, GPS: g}
	p.publishGPSPanel()
	first := *g.Panel
	if first.Selected != 0 || first.Player != 1 || first.Callsign != "ACTOR ZERO" || first.LeaderA != -1 || first.LeaderB != 1 || first.HeaderInk() != 9 {
		t.Fatalf("actor/player/leader source domains: %+v", first)
	}
	if first.Height != 1 || first.Control != r.Navigation[0].Raw || first.ShipFlags != 6|2<<3 || first.Chaff != 65535 || first.Flares != 65534 || first.Stations[2].Quantity != 65535 {
		t.Fatal("dynamic/control/SHIP snapshot ownership")
	}
	if first.FuelSeverityKnown {
		t.Fatal("nonplayer physical600 damage misread as source5fe severity")
	}
	// Republish uses the changed source leader/control rows, never camera or
	// rank; previously handed-out snapshots remain independent values.
	r.Navigation[3].Raw[5], r.Navigation[4].Raw[5] = 0xff, 1
	r.Navigation[0].Raw[1] = 3
	i.Stores.Ships[0].Chaff = 4
	p.publishGPSPanel()
	if first.Control[1] != 2 || first.Chaff != 65535 || first.Objectives[0].Raw[61] != 0x9b {
		t.Fatal("old publication aliases mutable source state")
	}
	if g.Panel == &first || g.Panel.Control[1] != 3 || g.Panel.Chaff != 4 || g.Panel.LeaderA != 1 || g.Panel.LeaderB != -1 {
		t.Fatal("logical publication did not replace source carriers")
	}
	g.Selected = 1
	p.publishGPSPanel()
	if g.Panel.Player != 1 || g.Panel.Selected != 1 || g.Panel.Height != 7 || !g.Panel.FuelSeverityKnown || g.Panel.FuelSeverity != 6 {
		t.Fatal("actual player state/mask not selected independently")
	}
}

func TestGPSObjectivesRetainRawTARGETAndIndependentStatus(t *testing.T) {
	targets := &gamestate.MissionTargets{}
	copy(targets.Records[2][:], "04/06 Destroy authentic TARGET")
	copy(targets.Records[3][:], "06/07 Identify authentic TARGET")
	binary.LittleEndian.PutUint16(targets.Records[2][32:], 3)
	targets.Records[2][61], targets.Records[3][61] = 0xfe, 0x37
	c := &FrontEnd{Rendering: &RenderingPreview{ResultTargets: targets}}
	got, err := c.gpsPanelObjectives()
	if err != nil {
		t.Fatal(err)
	}
	if !got[0].Available || !got[1].Available || got[0].Raw != targets.Records[2] || got[1].Raw != targets.Records[3] {
		t.Fatal("full62 TARGET lost during GPS publication")
	}
	targets.Records[2][61] = 0
	if got[0].Raw[61] != 0xfe {
		t.Fatal("GPS TARGET aliases result state")
	}
}

func TestGPSSingleLeaderAliasesUseInitializedNonflightRows(t *testing.T) {
	i := &gamestate.InitializedMission{PlayerRow: 0, Entities: []gamestate.InitializedEntity{
		{RuntimeRow: 0, FlightSlot: 0}, {RuntimeRow: 3, FlightSlot: -1}, {RuntimeRow: 4, FlightSlot: -1},
	}}
	i.Entities[1].ControlRaw[5], i.Entities[2].ControlRaw[5] = 0xff, 0xff
	r := &FlightRuntime{State: &flight.State{}}
	g := navigation.New(&r.Navigation, [5]navigation.Point{}, [5]bool{true})
	p := &RenderingPreview{Initialized: i, Flight: r, GPS: g}
	p.publishGPSPanel()
	if g.Panel.LeaderA != -1 || g.Panel.LeaderB != -1 || g.Panel.HeaderInk() != 7 {
		t.Fatal("Single used zero-filled absent navigation slots as actual leader selectors")
	}
	// If those exact rows are real flight members, their editable/current
	// navigation carriers take precedence over the retained launch copy.
	i.Entities[1].FlightSlot, i.Entities[2].FlightSlot = 3, 4
	r.Navigation[4].Raw[5] = 0
	p.publishGPSPanel()
	if g.Panel.LeaderA != 0 || g.Panel.HeaderInk() != 9 {
		t.Fatal("Flight ignored current source control leader alias")
	}
}

func TestGPSActualPreparedPanelAndLogicalScale(t *testing.T) {
	for _, scale := range []int{1, 2} {
		c := mastFixture(t, 0, "pass")
		jFinishFade(t, c)
		p := c.Rendering
		p.Scene.RenderScale = scale
		// Retained source rows supply nonplayer policy, altitude and speed;
		// display coverage does not turn these stationary actors into AI.
		p.Flight.Navigation[1].Raw[4] = 1
		binary.LittleEndian.PutUint16(p.Flight.Navigation[1].Raw[6:], 2)
		binary.LittleEndian.PutUint16(p.Flight.Navigation[1].Raw[8:], 4)
		before, _ := json.Marshal(p.Initialized)
		if err := c.openGPS(); err != nil {
			t.Fatal(err)
		}
		if p.GPS.Panel == nil || p.GPS.Panel.Selected != 0 || p.GPS.Panel.Player != byte(p.Initialized.PlayerRow) || p.GPS.Panel.Control != p.Flight.Navigation[0].Raw {
			t.Fatal("real scene GPS publication missing")
		}
		if c.Mission.Data.Targets == nil || c.Mission.Data.Targets == p.Initialized.Targets {
			t.Fatal("launch TARGET not retained separately from live progress")
		}
		if p.GPS.Panel.Objectives[0].Raw[32] == 0 || !p.GPS.Panel.Objectives[0].Available {
			t.Fatal("GPS player launch TARGET unavailable")
		}
		initialPanel := *p.GPS.Panel
		frame := c.gpsFrame()
		if p.Error != nil || len(frame) != 64000 {
			t.Fatal("GPS must retain source logical320 at both simulation scales", scale, p.Error)
		}
		if frame[72*320+232] != initialPanel.HeaderInk() || frame[79*320+316] != 8 {
			t.Fatal("actual prepared panel palette domain")
		}
		after, _ := json.Marshal(p.Initialized)
		if !bytes.Equal(before, after) || *p.GPS.Panel != initialPanel {
			t.Fatal("opening/drawing GPS mutated initialized state or published snapshot")
		}
		p.GPS.Selected = 1
		p.publishGPSPanel()
		if p.GPS.Panel.Control != p.Flight.Navigation[1].Raw || p.GPS.Panel.Selected == int(p.GPS.Panel.Player) || p.GPS.Panel.FuelSeverityKnown {
			t.Fatal("actual nonplayer source policy/severity ownership")
		}
		frozen := *p.Flight.State
		stores := append([]gamestate.RuntimeAircraft(nil), p.Initialized.Stores.Ships...)
		rng := p.Initialized.RandomState
		frame = c.gpsFrame()
		if p.Error != nil || len(frame) != 64000 {
			t.Fatal(p.Error)
		}
		if *p.Flight.State != frozen || !reflect.DeepEqual(stores, p.Initialized.Stores.Ships) || rng != p.Initialized.RandomState {
			t.Fatal("GPS draw advanced flight/stores/RNG")
		}
		c.closeGPS()
	}
}

func TestGPSCurrentCombatStatusWithoutResultAdmission(t *testing.T) {
	// Full retained TARGET records exist independently of career admission.
	targets := &gamestate.MissionTargets{}
	copy(targets.Records[2][:], "04/06 retained primary")
	targets.Records[2][61] = 0x76
	binary.LittleEndian.PutUint16(targets.Records[2][32:], 3)
	c := &FrontEnd{Mission: &MissionSession{Data: &gamestate.MissionData{Targets: targets}}, Rendering: &RenderingPreview{Flight: &FlightRuntime{Combat: &combat.State{}}}}
	got, err := c.gpsPanelObjectives()
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Raw[32] != 0 || got[0].Raw[61] != 0x76 || targets.Records[2][32] != 3 {
		t.Fatal("current status must not overwrite retained launch TARGET or lose tails")
	}
}

func TestGPSActualAircraftPanelKeepsSerializedSelectorAdapter(t *testing.T) {
	// Original GS DS3d0c preflight-to-SHIP map, independently retained by
	// cockpit/identity_test.go. Frontend4/5 are Kiowa/Blackhawk;6/7 Comanche.
	selectors := [8]byte{0, 1, 2, 3, 5, 6, 4, 4}
	for aircraft, selector := range selectors {
		c := mastFixture(t, aircraft, "land")
		jFinishFade(t, c)
		if err := c.openGPS(); err != nil {
			t.Fatal(aircraft, err)
		}
		if got := c.Rendering.GPS.Panel.ShipFlags & 7; got != selector {
			t.Fatalf("preflight%d selected serialized%d want%d", aircraft, got, selector)
		}
		if len(c.gpsFrame()) != 64000 || c.Rendering.Error != nil {
			t.Fatal(aircraft, c.Rendering.Error)
		}
		c.closeGPS()
	}
}

func TestGPSRetainedPreparationDoesNotRequireFreshNameAssets(t *testing.T) {
	raw := make([]byte, 310)
	for i := range raw {
		raw[i] = byte(i*17 + 3)
	}
	m := &gamestate.MissionData{SourcePreparation: &gamestate.CampaignSourceRaw{TargetBlock: raw}}
	c := &FrontEnd{Mission: &MissionSession{Data: m}, Rendering: &RenderingPreview{}}
	got, err := c.gpsPanelObjectives()
	if err != nil {
		t.Fatal(err)
	}
	if m.Targets != nil || !bytes.Equal(got[0].Raw[:], raw[2*62:3*62]) || !bytes.Equal(got[1].Raw[:], raw[3*62:4*62]) {
		t.Fatal("retained preparation TARGET was regenerated or changed its source")
	}
}
