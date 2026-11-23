package menus

import (
	"encoding/binary"
	"fmt"
	"gunship2000-assets/game/combat"
	"gunship2000-assets/game/replay"
	gamestate "gunship2000-assets/game/state"
	"testing"
)

// Supplied geometry/control/crew operands isolate the real actor dispatcher and
// accepted constructor. This is not a naturally acquired or flown mission.
func engagementController(t *testing.T, enabled bool) *FrontEnd {
	t.Helper()
	c := mastFixture(t, 0, "pass")
	jFinishFade(t, c)
	c.AbandonFilmRecording()
	p, r := c.Rendering, c.Rendering.Flight
	p.wingmanEngagementCompression = enabled
	r.State.Rotor, r.State.FlightStage, r.State.HeightControl, r.State.WaypointIndex = 1, 3, 0, 0
	for actor := 1; actor < 5; actor++ {
		e := &p.Initialized.Entities[actor]
		e.Kind, e.Participation, e.ControlRaw[1] = 0xff, 0, 0
	}
	e := &p.Initialized.Entities[3]
	e.Kind, e.Participation, e.ControlRaw[1], e.ControlRaw[3], e.ControlRaw[4] = 0x20, 1, 2, 11, 1
	e.DynamicRaw[3], e.RawMotion = 0, 0
	e.Pose = gamestate.InitializedPose{H0: 0x40000, H1: 0x40000, Height: 512}
	target := &p.Initialized.Entities[10]
	target.Kind, target.Participation, target.Object.Raw[31] = 3, 1, 0
	target.Pose = gamestate.InitializedPose{H0: 0x40000, H1: 0x41000, Height: 512}
	ship := &p.Initialized.Stores.Ships[3]
	proficiency := byte(25)
	ship.Proficiency = &proficiency
	found := false
	for _, weapon := range p.Initialized.Stores.Definitions {
		flags := binary.LittleEndian.Uint16(weapon.Raw[:])
		if weapon.CompactID > 1 && flags&0x800 != 0 && flags&3 != 3 && weapon.Raw[4] != 0 && binary.LittleEndian.Uint16(weapon.Raw[9:]) > 0 {
			ship.Stations = [4]gamestate.RuntimeStation{{}, {Present: true, RawWSHDID: weapon.RawWSHDID, CompactID: weapon.CompactID, Mode: 1, Ammunition: 10}}
			found = true
			break
		}
	}
	if !found {
		t.Fatal("accepted original ground weapon absent")
	}
	p.Initialized.Stores.Damage[3] = [10]byte{}
	p.Initialized.CrewPerformance[3].SkillKnown = true
	p.Initialized.CrewPerformance[3].RawB = 25
	return c
}

func hasFollowerLaunch(r *FlightRuntime) bool {
	for _, event := range r.CombatEvents {
		if event.Kind == "launch" && event.Actor == 3 && event.Slot == 18 && event.Source == "A7E6:AF21 healthy follower" {
			return true
		}
	}
	return false
}

func TestEngagementCompressionBoundariesAndHistoricalControl(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		for _, request := range []int16{4, 6, 16} {
			t.Run(fmt.Sprintf("policy%v/rate%d", enabled, request), func(t *testing.T) {
				c := engagementController(t, enabled)
				p, r := c.Rendering, c.Rendering.Flight
				r.Compression.Request, p.Cockpit.CompressionRequest = request, request
				timing := r.Clock.ReplayTiming()
				r.filmTiming = &timing
				if err := c.advanceFlight(0); err != nil {
					t.Fatal(err)
				}
				if !hasFollowerLaunch(r) || r.State.CompressionLimit || r.State.CompressionReset || r.Avionics.CompressionLimited {
					t.Fatal("fixture did not isolate actual healthy engagement", r.CombatEvents, r.State.CompressionLimit, r.State.CompressionReset, r.Avionics.CompressionLimited)
				}
				want := request
				if enabled && request > 4 {
					want = 0
				}
				if r.Compression.Request != want || p.Cockpit.CompressionRequest != want || r.Steps != 1 {
					t.Fatal("rate/HUD/logical boundary", request, want, r.Compression.Request, p.Cockpit.CompressionRequest, r.Steps)
				}
			})
		}
	}
}

func TestEngagementDropsRemainingAcceleratedBatch(t *testing.T) {
	for _, request := range []int16{6, 16} {
		ref := engagementController(t, true)
		rr := ref.Rendering.Flight
		rr.Compression.Request = request
		timing := rr.Clock.ReplayTiming()
		rr.filmTiming = &timing
		if err := ref.advanceFlight(0); err != nil {
			t.Fatal(err)
		}
		c := engagementController(t, true)
		p, r := c.Rendering, c.Rendering.Flight
		r.Compression.Request = request
		if err := c.advanceFlight(.25); err != nil {
			t.Fatal(err)
		}
		if r.Steps != 1 || r.Compression.Request != 0 || !hasFollowerLaunch(r) || p.Initialized.RandomState != ref.Rendering.Initialized.RandomState || r.Combat.Slots != rr.Combat.Slots {
			t.Fatal("accelerated tick ran after engagement", request, r.Steps, r.Compression.Request)
		}
		before, updates := p.Initialized.RandomState, r.Combat.Updates
		if err := c.advanceFlight(0); err != nil {
			t.Fatal(err)
		}
		if r.Steps != 1 || r.Combat.Updates != updates || p.Initialized.RandomState != before {
			t.Fatal("discarded compressed debt resumed later")
		}
	}
}

func TestHoldAndUnrelatedRadioDoNotLimitCompression(t *testing.T) {
	c := engagementController(t, true)
	p, r := c.Rendering, c.Rendering.Flight
	p.wingmenAutonomy = false
	r.Combat.SetWingmenAutonomyEnabled(false)
	p.Initialized.Entities[3].ControlRaw[3] = 0
	p.Initialized.Entities[3].ControlRaw[1] = 3
	r.OrderRecipient = 3
	if err := c.Handle(Actions{}); err != nil {
		t.Fatal(err)
	}
	if err := c.Handle(Actions{WingmanOrder: "h"}); err != nil {
		t.Fatal(err)
	}
	r.Compression.Request, p.Cockpit.CompressionRequest = 16, 16
	timing := r.Clock.ReplayTiming()
	r.filmTiming = &timing
	if err := c.advanceFlight(0); err != nil {
		t.Fatal(err)
	}
	if p.Initialized.Entities[3].ControlRaw[1] != 2 || p.Initialized.Entities[3].ControlRaw[2] != 3 || r.Compression.Request != 16 || p.Cockpit.CompressionRequest != 16 || hasFollowerLaunch(r) {
		t.Fatal("HOLD changed rate", p.Initialized.Entities[3].ControlRaw[1], r.Compression.Request)
	}
	// A notification is deliberately supplied without any accepted constructor.
	// It must not become a second, generic limiter path.
	r.Combat.Events = append(r.Combat.Events, combat.Event{Kind: "notification", Actor: 3, Target: 10, Notification: 2})
	p.collectCombatEvents()
	if r.Compression.Request != 16 || p.Cockpit.CompressionRequest != 16 {
		t.Fatal("radio dispatch invented engagement")
	}
}

func TestEngagementThroughPresentedFrameDropsAcceleratedDebt(t *testing.T) {
	c := engagementController(t, true)
	p, r := c.Rendering, c.Rendering.Flight
	r.Compression.Request, p.Cockpit.CompressionRequest = 16, 16
	if err := c.AdvanceRendering(.25); err != nil {
		t.Fatal(err)
	}
	// The first visual interval contains the accepted release. Subsequent
	// intervals may consume the remaining host time at ordinary 1x, but none
	// may drain the former 16x debt. .25 host seconds admits at most four
	// ordinary source updates after discarding the first accelerated batch.
	if r.Compression.Request != 0 || p.Cockpit.CompressionRequest != 0 || r.Steps < 1 || r.Steps > 4 {
		t.Fatal("presented frame retained accelerated debt", r.Steps, r.Compression.Request, p.Cockpit.CompressionRequest)
	}
}

func TestLimiterDisplayEpochAndProfileConstruction(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		p := &RenderingPreview{wingmanEngagementCompression: enabled, Flight: &FlightRuntime{}}
		p.Flight.Compression.Request, p.Cockpit.CompressionRequest = 16, 16
		p.limitFlightCompression()
		want := int16(16)
		if enabled {
			want = 0
		}
		if p.Flight.Compression.Request != 0 || p.Cockpit.CompressionRequest != want {
			t.Fatal("historical display epoch changed", enabled, p.Cockpit.CompressionRequest)
		}
	}
	for _, profile := range []string{replay.R12Compatibility, replay.R12AuthoredCompatibility} {
		formation, modern := launchSimulationPolicy(profile)
		if !formation || !modern {
			t.Fatal("new policy selected after initialization", profile)
		}
	}
}
