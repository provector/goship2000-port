package menus

import (
	"os"
	"testing"

	"gunship2000-assets/game/assets"
	"gunship2000-assets/game/configuration"
	"gunship2000-assets/game/navigation"
	"gunship2000-assets/game/replay"
)

// The historical operation and the counterfactual start at the same verified
// boundary. Only the latter switches policy, explicitly after isolated Resume.
func TestFlyToBridgeContactContinuation(t *testing.T) {
	path := os.Getenv("GS2000_TEST_BRIDGE_FILM")
	if path == "" {
		t.Skip("requires the retained W50 Ai check recording")
	}
	a, err := assets.LoadFrontEndAssets(os.Getenv("GS2000_TEST_MENUS"))
	if err != nil {
		t.Fatal(err)
	}
	for _, corrected := range []bool{false, true} {
		f, err := replay.OpenFile(path)
		if err != nil {
			t.Fatal(err)
		}
		q, err := NewFilmPlayback(a, f, configuration.Defaults())
		if err != nil {
			t.Fatal(err)
		}
		for q.Cursor < 15489 {
			if err = q.Step(); err != nil {
				t.Fatal(err)
			}
		}
		p := q.Engine.Rendering
		if p.routeAcquisitionRetry {
			t.Fatal("historical playback opted into new policy")
		}
		if err = q.Resume(); err != nil {
			t.Fatal(err)
		}
		if p.routeAcquisitionRetry {
			t.Fatal("historical Resume changed policy")
		}
		p.routeAcquisitionRetry = corrected
		wing := &p.Initialized.Entities[3]
		route := &p.Flight.Navigation[3]
		goal := route.Point(1)
		step := p.Flight.Steps
		for i := 0; i < 20 && p.Flight.Steps == step; i++ {
			if err = q.AdvancePractice(.02); err != nil {
				t.Fatal(err)
			}
		}
		want := byte(0)
		if corrected {
			want = 31
		}
		t.Logf("retry=%v step=%d bridge=%#x assignment=%d targetSide=%q order=%d waypoint=%v slotLifetime=%d", corrected, p.Flight.Steps, p.Flight.Avionics.Contacts[12].Flags, wing.ControlRaw[3], p.Initialized.Entities[30].Side, wing.ControlRaw[1], goal, p.Flight.Combat.Slots[18].Lifetime)
		if p.Flight.Steps != 4067 || p.Flight.Avionics.Contacts[12].Flags != 30 || wing.ControlRaw[3] != want || wing.ControlRaw[1] != 3 || route.Point(1) != goal || !goal.Present() || p.Flight.Combat.Slots[18].Lifetime != 0 {
			t.Fatal("bridge report, assignment, launch or route differed")
		}
		if !corrected {
			continue
		}
		rangeToGoal := func() uint16 {
			_, d := navigation.Bearing(navigation.Point{H0: wing.Pose.H0, H1: wing.Pose.H1}, goal, p.Renderer.Assets)
			return d
		}
		start := rangeToGoal()
		nearest := start
		cleared := false
		ammo := func() int {
			total := 0
			for _, station := range p.Initialized.Stores.Ships[3].Stations {
				total += int(station.Ammunition)
			}
			return total
		}
		initialAmmo := ammo()
		last := byte(31)
		for i := 0; i < 650; i++ {
			if err = q.AdvancePractice(.1); err != nil {
				t.Fatal(err)
			}
			if wing.ControlRaw[3] != last {
				if !cleared {
					t.Logf("step %d assignment %d -> %d, order %d", p.Flight.Steps, last, wing.ControlRaw[3], wing.ControlRaw[1])
				}
				last = wing.ControlRaw[3]
			}
			nearest = min(nearest, rangeToGoal())
			if last != 31 {
				cleared = true
			}
		}
		t.Logf("after step %d: order %d, active waypoint %d, first waypoint range %d (closest %d) -> %d, ammunition expended %d", p.Flight.Steps, wing.ControlRaw[1], route.Index(), start, nearest, rangeToGoal(), initialAmmo-ammo())
		if !cleared || route.Point(1) != goal || nearest >= start || route.Index() != 2 {
			t.Fatal("target held indefinitely or accepted route did not progress")
		}
	}
}
