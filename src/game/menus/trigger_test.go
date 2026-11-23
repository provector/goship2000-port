package menus

import (
	"strings"
	"testing"
	"time"

	"gunship2000-assets/game/combat"
)

func TestShortTriggerTapSurvivesOneLogicalBoundary(t *testing.T) {
	q := flightCommandInput{}
	q.SampleAt(10*time.Millisecond, Actions{WeaponTriggerValid: true, WeaponTriggerHeld: true})
	q.SampleAt(25*time.Millisecond, Actions{WeaponTriggerValid: true})
	q.NextAt(9 * time.Millisecond)
	if q.Trigger {
		t.Fatal("future tap became a shot")
	}
	q.NextAt(99 * time.Millisecond)
	if !q.Trigger {
		t.Fatal("recorded press/release between logical updates was lost")
	}
	q.NextAt(198 * time.Millisecond)
	if q.Trigger {
		t.Fatal("released tap retried on a later logical update")
	}
}

func TestBackendMakeAfterReleaseIsOneIntent(t *testing.T) {
	var latch inputLatch
	a := latch.Read(Buttons{WeaponTriggerPressed: true})
	if a.WeaponTriggerHeld || !a.WeaponTriggerPressed {
		t.Fatal("backend make event was replaced with current released state")
	}
	q := flightCommandInput{}
	q.SampleAt(10*time.Millisecond, a)
	q.NextAt(99 * time.Millisecond)
	if !q.Trigger {
		t.Fatal("released backend make was discarded")
	}
	q.NextAt(198 * time.Millisecond)
	if q.Trigger {
		t.Fatal("backend make repeated without another press")
	}
}

func TestRejectedRocketTapIsConsumedWithoutRetry(t *testing.T) {
	c := armedMission(t)
	p, r := c.Rendering, c.Rendering.Flight
	p.Initialized.SelectedStation = 1
	ammo := p.Initialized.Stores.Ships[0].Stations[1].Ammunition
	r.Combat.LastSlot = 15
	r.Combat.Slots[15] = combat.Projectile{Flags: 1, Lifetime: 131, Parameter: 3, Speed: 50, Pose: p.Initialized.Player}
	// Press and release precede the next ~98.56ms source-policy boundary.
	// Life131 must reject at559e before86e6 turns the old rocket into life49.
	c.Handle(Actions{WeaponTriggerValid: true})
	c.Handle(Actions{InputTime: 10 * time.Millisecond, WeaponTriggerValid: true, WeaponTriggerHeld: true})
	c.Handle(Actions{InputTime: 25 * time.Millisecond, WeaponTriggerValid: true})
	if err := c.AdvanceRendering(.1); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range r.TriggerTrace {
		found = found || strings.Contains(e.Reason, "previous projectile cooldown")
	}
	if !found || p.Initialized.Stores.Ships[0].Stations[1].Ammunition != ammo {
		t.Fatal("tap was lost or bypassed source cooldown", r.TriggerTrace)
	}
	if err := c.AdvanceRendering(.2); err != nil {
		t.Fatal(err)
	}
	if p.Initialized.Stores.Ships[0].Stations[1].Ammunition != ammo {
		t.Fatal("rejected tap became a delayed shot after cooldown")
	}
}

func TestTriggerTapsCoalesceWithoutBacklogAndResetDropsThem(t *testing.T) {
	q := flightCommandInput{}
	for _, at := range []time.Duration{10, 30, 50} {
		q.SampleAt(at*time.Millisecond, Actions{WeaponTriggerValid: true, WeaponTriggerHeld: true})
		q.SampleAt((at+5)*time.Millisecond, Actions{WeaponTriggerValid: true})
	}
	q.NextAt(99 * time.Millisecond)
	if !q.Trigger {
		t.Fatal("no intent at its first boundary")
	}
	q.NextAt(198 * time.Millisecond)
	if q.Trigger {
		t.Fatal("multiple sub-tick taps queued delayed shots")
	}
	q.SampleAt(210*time.Millisecond, Actions{WeaponTriggerValid: true, WeaponTriggerHeld: true})
	q.SampleAt(220*time.Millisecond, Actions{WeaponTriggerValid: true})
	q.Reset()
	q.NextAt(time.Second)
	if q.Trigger {
		t.Fatal("modal/focus reset retained a pending intent")
	}
}
