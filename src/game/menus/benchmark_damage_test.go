package menus

import (
	"gunship2000-assets/game/replay"
	"reflect"
	"strings"
	"testing"
)

// Supplied initial approach, real hostile release/contact and the same ordinary
// input stream in both interpretations. No damage, projectile or result injection.
func TestNarrowCommissionFirstStoreDifferenceIsVictimLocal(t *testing.T) {
	old := developmentScenarioControllerProfile(t, "p3-first-commission", replay.R5Compatibility)
	now := developmentScenarioControllerProfile(t, "p3-first-commission", replay.R6Compatibility)
	defer old.AbandonFilmRecording()
	defer now.AbandonFilmRecording()
	tick := func(c *FrontEnd, a Actions) {
		t.Helper()
		if e := c.Handle(a); e != nil {
			t.Fatal(e)
		}
		if e := c.AdvanceRendering(.12); e != nil {
			t.Fatal(e)
		}
		c.Rendering.takeSceneAudio()
	}
	for _, c := range []*FrontEnd{old, now} {
		if c.Rendering.Paused {
			tick(c, Actions{Cancel: true, WeaponTriggerValid: true})
		}
		for i := 0; i < 6; i++ {
			p := c.Rendering
			if strings.Contains(strings.ToUpper(p.Initialized.Stores.Ships[0].Stations[p.Initialized.SelectedStation].ShortName), "HELL") {
				break
			}
			tick(c, Actions{OrdnanceNext: true, WeaponTriggerValid: true})
		}
	}
	for i := 0; i < 1500; i++ {
		a := Actions{WeaponTriggerValid: true, WeaponTriggerHeld: true}
		if old.Rendering.Flight.Avionics.Target == 0 || i%20 == 0 {
			a = Actions{Backspace: true, WeaponTriggerValid: true}
		}
		tick(old, a)
		tick(now, a)
		x, y := old.Rendering, now.Rendering
		if reflect.DeepEqual(x.Initialized.Stores.Ships[0].Stations, y.Initialized.Stores.Ships[0].Stations) {
			continue
		}
		if x.Initialized.RandomState != y.Initialized.RandomState || x.Flight.Systems.Damage != y.Flight.Systems.Damage {
			t.Fatal("diverged before owner-only write", i)
		}
		changed := 0
		for n, s := range x.Initialized.Stores.Ships[0].Stations {
			ns := y.Initialized.Stores.Ships[0].Stations[n]
			if s == ns {
				continue
			}
			if s.Mode != 3 || ns.Ammunition != s.Ammunition>>1 {
				t.Fatal("unexpected first difference", i, n, s, ns)
			}
			changed++
		}
		if changed == 0 {
			t.Fatal("no paired-wing difference")
		}
		t.Logf("first victim-local station difference after scripted visit%d; equal damage and RNG", i)
		return
	}
	t.Fatal("real narrow encounter did not reach paired-wing contact")
}
