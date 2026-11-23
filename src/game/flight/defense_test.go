package flight

import (
	"gunship2000-assets/game/world"
	"testing"
)

func TestDefenseSourceArithmeticAndOrder(t *testing.T) {
	q := &world.Bundle{Sine: make([]int16, 1025)}
	q.Sine[1024] = 16384
	d := NewDefenses(1, 2)
	p := Pose{H0: 1000, H1: 2000, Height: 100}
	if !d.Activate(LaunchChaff, 0, p, 6, q, nil) || d.Chaff != -2 || d.ChaffTimer != 10 || d.DriftH0 != 0 || d.DriftH1 != -72 || d.DriftHeight != -6 {
		t.Fatalf("chaff %+v", d)
	}
	if d.Activate(LaunchChaff, 0, p, 6, q, nil) {
		t.Fatal("negative count unavailable")
	}
	d.AdvanceDecoy()
	if d.Decoy.H1 != 1928 || d.Decoy.Height != 94 {
		t.Fatalf("decoy %+v", d)
	}
	if !d.Activate(LaunchFlare, 0, Pose{H0: 9, H1: 8, Height: 7}, 5, q, nil) || d.Flares != -1 || d.Decoy.H0 != 9 || d.DriftHeight != -10 {
		t.Fatal(d)
	}
	before := d
	d.AdvanceCounters(false)
	if d != before {
		t.Fatal("nonE0 changed counters")
	}
	d.AdvanceCounters(true)
	if d.FlareTimer != 9 || d.ChaffTimer != 9 {
		t.Fatal(d)
	}
	before = d
	if d.Activate(LaunchFlare, 0x40, p, 6, q, nil) || d != before {
		t.Fatal("damage guard mutates")
	}
}
func TestDefenseIRChargeAndRadarSharedDraw(t *testing.T) {
	d := NewDefenses(12, 12)
	draws := 0
	draw := func() uint16 { draws++; return 15 }
	d.Activate(ToggleRadarJammer, 0, Pose{}, 6, nil, draw)
	if d.RadarSignature != 8 || draws != 1 {
		t.Fatal(d, draws)
	}
	d.Activate(ToggleRadarJammer, 0, Pose{}, 6, nil, draw)
	if d.RadarSignature != 0 || draws != 1 {
		t.Fatal(d, draws)
	}
	d.Activate(ToggleIRJammer, 0, Pose{}, 6, nil, nil)
	d.AdvanceCounters(true)
	if d.IRActive != 63 || d.IRCharge != 63 {
		t.Fatal(d)
	}
	d.Activate(ToggleIRJammer, 0, Pose{}, 6, nil, nil)
	d.AdvanceCounters(true)
	if d.IRActive != 0 || d.IRCharge != 64 {
		t.Fatal(d)
	}
	d.Activate(ToggleIRJammer, 0, Pose{}, 6, nil, nil)
	if flags, disabled := d.GuidanceResponse(0x83, 0); !disabled || flags != 0x23 {
		t.Fatal(flags, disabled)
	}
	if flags, disabled := d.GuidanceResponse(0x8083, 0); disabled || flags != 0x8083 {
		t.Fatal(flags, disabled)
	}
	d.IRActive = 1
	d.AdvanceCounters(true)
	if d.IRActive != 0 || d.IRCharge != 0 {
		t.Fatal(d)
	}
	d.AdvanceCounters(true)
	if d.IRCharge != 1 {
		t.Fatal(d)
	}
}
