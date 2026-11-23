package menus

import (
	"encoding/binary"
	"fmt"
	"testing"
)

// Original section1:3df9 -> 559e and 58f2 -> 559e do not own AF3.
// This exercises real accepted loadouts and the logical fire consumer, rather
// than treating a radar dot, designation or successful discharge as an attack-
// constructor limiter. Independent byte/caller checks live under stepO2.
func TestPlayerDischargeDoesNotInventCompressionLimiter(t *testing.T) {
	for station, label := range []string{"cannon", "rocket", "guided"} {
		for _, request := range []int16{0, 2, 4, 6, 16} {
			t.Run(fmt.Sprintf("%s/%d", label, request), func(t *testing.T) {
				c := armedMission(t)
				p, r := c.Rendering, c.Rendering.Flight
				p.Initialized.SelectedStation = station
				st := &p.Initialized.Stores.Ships[0].Stations[station]
				beforeAmmo := st.Ammunition
				definitionFound := false
				for _, w := range p.Initialized.Stores.Definitions {
					if w.CompactID == st.CompactID {
						family := binary.LittleEndian.Uint16(w.Raw[:]) & 3
						t.Logf("raw weapon=%d compact=%d source family=%d", st.RawWSHDID, st.CompactID, family)
						definitionFound = true
					}
				}
				if !definitionFound {
					t.Fatal("accepted source definition missing")
				}
				// Neutral trigger sample followed by held trigger, as at the normal
				// live input boundary. Supplied request isolates the tested caller.
				c.Handle(Actions{WeaponTriggerValid: true})
				c.Handle(Actions{WeaponTriggerValid: true, WeaponTriggerHeld: true})
				r.Compression.Request = request
				timing := r.Clock.ReplayTiming()
				r.filmTiming = &timing
				if err := c.advanceFlight(0); err != nil {
					t.Fatal(err)
				}
				if st.Ammunition >= beforeAmmo {
					t.Fatal("real discharge was not reached")
				}
				if r.Compression.Request != request {
					t.Fatal("player discharge falsely classified as source limiter", request, r.Compression.Request)
				}
				// Source family-specific cooldowns differ. The guided second
				// attempt rejects; these cannon/rocket definitions can discharge
				// again after the preceding movement visit. Neither owns AF3.
				beforeAmmo = st.Ammunition
				timing = r.Clock.ReplayTiming()
				r.filmTiming = &timing
				if err := c.advanceFlight(0); err != nil {
					t.Fatal(err)
				}
				if station == 2 && st.Ammunition != beforeAmmo {
					t.Fatal("guided cooldown attempt unexpectedly discharged", beforeAmmo, st.Ammunition)
				}
				if station != 2 && st.Ammunition >= beforeAmmo {
					t.Fatal("held cannon/rocket repeat did not discharge", beforeAmmo, st.Ammunition)
				}
				if r.Compression.Request != request {
					t.Fatal("repeat/cooldown attempt invented compression cancellation", request, r.Compression.Request)
				}
			})
		}
	}
}

func TestDesignationAndEmptyAttemptDoNotInventCompressionLimiter(t *testing.T) {
	for _, request := range []int16{0, 2, 4, 6, 16} {
		c := armedMission(t)
		p, r := c.Rendering, c.Rendering.Flight
		c.Handle(Actions{WeaponTriggerValid: true})
		r.Compression.Request = request
		c.Handle(Actions{Backspace: true, WeaponTriggerValid: true})
		timing := r.Clock.ReplayTiming()
		r.filmTiming = &timing
		if err := c.advanceFlight(0); err != nil {
			t.Fatal(err)
		}
		if r.Compression.Request != request {
			t.Fatal("designation was treated as cancellation", request, r.Compression.Request)
		}
		// Supplied empty station tests the source early rejection, not a
		// fabricated successful shot or an aggregate hit-rate claim.
		p.Initialized.Stores.Ships[0].Stations[p.Initialized.SelectedStation].Ammunition = 0
		c.Handle(Actions{WeaponTriggerValid: true, WeaponTriggerHeld: true})
		timing = r.Clock.ReplayTiming()
		r.filmTiming = &timing
		if err := c.advanceFlight(0); err != nil {
			t.Fatal(err)
		}
		if r.Compression.Request != request {
			t.Fatal("empty attempt was treated as cancellation", request, r.Compression.Request)
		}
	}
}

func TestReachedIncomingLimiterDiscardsAcceleratedBatchDebt(t *testing.T) {
	for _, request := range []int16{6, 16} {
		// Establish the reached source boundary one ordinary step at a time.
		// The fixture's preceding target/pose publication is part of the test;
		// merely pointing a supplied slot does not license assuming step1.
		ref := developmentScenarioController(t, "n6-incoming")
		if err := ref.Handle(Actions{Cancel: true}); err != nil {
			t.Fatal(err)
		}
		rr := ref.Rendering.Flight
		rr.Compression.Request = request
		for rr.Compression.Request != 0 && rr.Steps < 8 {
			timing := rr.Clock.ReplayTiming()
			rr.filmTiming = &timing
			if err := ref.advanceFlight(0); err != nil {
				t.Fatal(err)
			}
		}
		if rr.Compression.Request != 0 {
			t.Fatal("reference never reached guidance limiter")
		}
		c := developmentScenarioController(t, "n6-incoming")
		p, r := c.Rendering, c.Rendering.Flight
		if err := c.Handle(Actions{Cancel: true}); err != nil {
			t.Fatal(err)
		}
		r.Compression.Request = request
		// The admitted supplied incoming slot faces player0 within the source
		// guidance chain. A quarter-second logical scheduler batch queues several
		// compressed steps; the reached limiter must leave no accelerated debt.
		// AdvanceRendering may partition a host frame around visual visits and
		// legitimately schedule its remaining unaccelerated host time separately.
		if err := c.advanceFlight(.25); err != nil {
			t.Fatal(err)
		}
		if r.Steps != rr.Steps || r.Compression.Request != 0 {
			t.Fatal("reached guidance failed to cancel at same logical step", request, rr.Steps, r.Steps, r.Compression.Request)
		}
		rng, updates := p.Initialized.RandomState, r.Combat.Updates
		if err := c.advanceFlight(0); err != nil {
			t.Fatal(err)
		}
		if r.Steps != rr.Steps || r.Combat.Updates != updates || p.Initialized.RandomState != rng {
			t.Fatal("discarded accelerated debt executed future mechanics")
		}
		if p.Initialized.RandomState != ref.Rendering.Initialized.RandomState || r.Combat.Slots != rr.Combat.Slots {
			t.Fatal("batched guidance altered real slot state or shared RNG")
		}
		t.Logf("request%d reaches source limiter after%d complete steps", request, r.Steps)
	}
}
