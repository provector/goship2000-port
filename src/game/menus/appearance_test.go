package menus

import (
	"encoding/binary"
	"gunship2000-assets/game/avionics"
	"gunship2000-assets/game/cockpit"
	"gunship2000-assets/game/flight"
	"gunship2000-assets/game/navigation"
	"gunship2000-assets/game/render3d"
	gamestate "gunship2000-assets/game/state"
	"gunship2000-assets/game/world"
	"testing"
	"time"
)

func TestDeathHistorySourceFillDecrementAndNoDrawMutation(t *testing.T) {
	p := &RenderingPreview{Flight: &FlightRuntime{}, Renderer: &render3d.Renderer{Assets: &world.Bundle{Combat: &world.CombatPresentation{DeathHistory: []string{"0", "1", "2", "3", "4", "5", "6", "7"}}}}, Initialized: &gamestate.InitializedMission{Entities: make([]gamestate.InitializedEntity, 1)}}
	p.Initialized.Entities[0].Kind = 3
	p.Initialized.Entities[0].Participation = 2
	p.Initialized.Entities[0].Pose = gamestate.InitializedPose{H0: 1000, H1: 2000, Height: 20}
	p.advanceDeathHistory(1)
	if p.Flight.deathHistoryCount != 7 || len(p.Flight.deathHistoryWorld) != 0 || p.Flight.deathHistory[7].Height != 20 {
		t.Fatal("initial source seed")
	}
	p.advanceDeathHistory(2)
	if p.Flight.deathHistoryCount != 6 || len(p.Flight.deathHistoryWorld) != 1 {
		t.Fatal("source7 to6")
	}
	// Entry7 is overwritten from the current P2 pose, then moves +8 and rises
	// (8-7)*128+((DA-7)&3)*8 =152. No RNG or camera is involved.
	q := p.Flight.deathHistory[7]
	if q.H0 != 1008 || q.H1 != 2008 || q.Height != 172 {
		t.Fatal(q)
	}
	p.Initialized.Entities[0].Participation = 1
	for n := 0; n < 6; n++ {
		p.advanceDeathHistory(uint16(n + 3))
	}
	if p.Flight.deathHistoryCount != 0 {
		t.Fatal("source history did not expire")
	}
	retained := p.Flight.deathHistory
	p.advanceDeathHistory(10)
	if p.Flight.deathHistory != retained || len(p.Flight.deathHistoryWorld) != 0 {
		t.Fatal("zero history mutated")
	}
}
func TestFeedbackUsesFrozenSceneAndHistoricalFallback(t *testing.T) {
	for kind := byte(0); kind < 6; kind++ {
		c := &FrontEnd{Debrief: &DebriefView{Scene: "SAD.SCN", Pending: &gamestate.PendingSortie{Proposal: &gamestate.SortieProposal{Facts: gamestate.SortieFacts{ResultContext: &gamestate.SortieResultContext{Version: 3, Scene: &kind}}}}}}
		picture, palette, ok := c.frozenFeedbackMedia()
		suffix := ""
		if kind == 2 {
			suffix = "_2"
		}
		if kind >= 4 {
			suffix = "_3"
		}
		if !ok || picture != "SAD"+suffix+".PIC" || palette != "SAD"+suffix+".PAL" {
			t.Fatal(kind, picture, palette, ok)
		}
		c.Debrief.Pending.Proposal.Facts.ResultContext.Version = 2
		if _, _, ok = c.frozenFeedbackMedia(); ok {
			t.Fatal("historical missing scene guessed")
		}
	}
}
func TestGPSActualControlColoursAndReservedRegistration(t *testing.T) {
	p := &RenderingPreview{p4Mechanics: true, Flight: &FlightRuntime{Avionics: avionics.New()}, Initialized: &gamestate.InitializedMission{Entities: make([]gamestate.InitializedEntity, 9)}}
	for a := 0; a < 5; a++ {
		e := &p.Initialized.Entities[a]
		e.RuntimeRow = a
		e.FlightSlot = a
		e.Kind = 0x21
		e.Participation = 1
		e.ControlRaw[1] = byte(a)
		e.Pose = gamestate.InitializedPose{H0: uint32(1000 + a), H1: 2000}
		p.Flight.Avionics.Contacts[a] = avionics.Contact{Flags: 1, Kind: 0x21, H0: e.Pose.H0, H1: e.Pose.H1}
	}
	r := &p.Initialized.Entities[5]
	r.RuntimeRow = 5
	r.FlightSlot = -1
	r.Kind = 0x1b
	r.Participation = 1
	r.Pose = gamestate.InitializedPose{H0: 5555, H1: 6666}
	got := p.ContactMapMarkers()
	if len(got) != 6 {
		t.Fatal(got)
	}
	for a := 0; a < 5; a++ {
		ink := byte(9)
		if a == 0 {
			ink = 5
		}
		if got[a].Colour != ink || got[a].Symbol != "map-06d4" {
			t.Fatal(a, got[a])
		}
	}
	if got[5].H0 != 5555 || got[5].Symbol != "map-070d" || got[5].Colour != 7 {
		t.Fatal(got[5])
	}
	r.Kind = 0x41
	r.Participation = 0
	if len(p.ContactMapMarkers()) != 5 {
		t.Fatal("collected reserve retained a marker")
	}
	p.p4Mechanics = false
	for _, m := range p.ContactMapMarkers() {
		if m.Colour != 5 {
			t.Fatal("old feedback changed", m)
		}
	}
}
func TestGPSReadsReachedFollowerControlWithoutASecondRouteOwner(t *testing.T) {
	p := &RenderingPreview{p4Mechanics: true, Flight: &FlightRuntime{State: &flight.State{}}, Initialized: &gamestate.InitializedMission{PlayerRow: 0, Entities: make([]gamestate.InitializedEntity, 5)}}
	p.Cockpit.Map = &cockpit.MapInputs{}
	for actor := 0; actor < 5; actor++ {
		e := &p.Initialized.Entities[actor]
		e.RuntimeRow = actor
		e.FlightSlot = actor
		e.Participation = 1
	}
	e := &p.Initialized.Entities[1]
	e.ControlRaw[0] = 1
	e.ControlRaw[1] = 7
	e.ControlRaw[3] = 12
	e.ControlRaw[5] = 4
	e.ControlRaw[30] = 99
	// A real actor/contact publication has changed CONTROL since initialization.
	// The map/GPS inspection carrier must consume the complete reached row.
	p.publishMapRoutes()
	if p.Flight.Navigation[1].Raw != e.ControlRaw {
		t.Fatal("follower navigation retained stale initialized CONTROL")
	}
	p.GPS = navigation.New(&p.Flight.Navigation, [5]navigation.Point{}, [5]bool{true, true})
	p.GPS.Selected = 1
	p.publishGPSPanel()
	first := *p.GPS.Panel
	if first.Control != e.ControlRaw {
		t.Fatal("GPS panel did not read reached actual CONTROL")
	}
	e.ControlRaw[1] = 1
	e.ControlRaw[3] = 0xff
	p.publishMapRoutes()
	p.publishGPSPanel()
	if first.Control[1] != 7 || p.GPS.Panel.Control[1] != 1 || p.GPS.Panel.Control[3] != 0xff {
		t.Fatal("published snapshots alias or remain stale")
	}
	p.p4Mechanics = false
	e.ControlRaw[1] = 8
	p.publishMapRoutes()
	if p.Flight.Navigation[1].Raw[1] != 1 {
		t.Fatal("historical navigation interpretation changed")
	}
}

func TestFollowerReservePickupKeepsActualCarrierIdentity(t *testing.T) {
	c := developmentScenarioController(t, "n6-rescue")
	p := c.Rendering
	m, r := p.Initialized, p.Flight
	// Controlled query0 approach, identical to the formerly competing player
	// fixture. Revive actor1; let actor4's real fuel-empty path activate reserve8.
	original := m.Entities[1]
	e := &m.Entities[4]
	e.Kind, e.Participation, e.Pose, e.RawMotion = 0x20, 1, original.Pose, 0
	e.ControlRaw[1], e.ControlRaw[5] = 1, 0
	binary.LittleEndian.PutUint16(e.ControlRaw[6:], 0)
	e.DynamicRaw[0], e.DynamicRaw[1] = 0x20, 1
	binary.LittleEndian.PutUint16(e.DynamicRaw[6:], 0)
	binary.LittleEndian.PutUint32(e.DynamicRaw[14:], e.Pose.H0)
	binary.LittleEndian.PutUint32(e.DynamicRaw[18:], e.Pose.H1)
	binary.LittleEndian.PutUint16(e.DynamicRaw[22:], e.Pose.Height)
	r.Systems.Fuel[1], r.Systems.Fuel[4] = 100, 0
	m.Stores.Ships[1].Fuel, m.Stores.Ships[4].Fuel = 100, 0
	r.State.Pose.Height, r.State.HeightTarget = 64, 64
	cargo, mass := m.Stores.Ships[0].Flags, r.MassTerm
	p.publishFlight()
	p.resetPresentation()
	if err := c.Handle(Actions{Cancel: true}); err != nil {
		t.Fatal(err)
	}
	for n := 0; n < 12 && m.Entities[8].Kind != 0x41; n++ {
		if err := c.AdvanceRendering(float64(flight.StepDuration()+time.Nanosecond) / float64(time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	if m.Entities[8].Kind != 0x41 || m.Entities[8].DynamicRaw[3] != 1 {
		t.Fatal("actual follower pickup changed", m.Entities[8].Kind, m.Entities[8].DynamicRaw[3])
	}
	found := false
	for _, event := range r.Combat.Results {
		if event.Kind == 6 && event.Actor == 1 && event.Target == 4 {
			found = true
		}
	}
	if !found || len(instanceModels(p, m.Entities[8].ID)) != 0 {
		t.Fatal("actual rescue result/collected registration", r.Combat.Results)
	}
	if m.Stores.Ships[0].Flags != cargo || r.MassTerm != mass {
		t.Fatal("follower rescue changed ordinary player cargo/mass")
	}
	t.Logf("supplied query0 approach: reserve8 collected by actor%d at step%d; player height%d", m.Entities[8].DynamicRaw[3], r.Steps, r.State.Pose.Height)
}
