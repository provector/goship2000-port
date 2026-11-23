package flight

import "testing"

func TestLaterJammerWriterGatesAndRequests(t *testing.T) {
	for _, tc := range []struct {
		name      string
		switches  uint16
		damage    byte
		e0        bool
		phase     byte
		requests  uint16
		ir, radar uint16
	}{
		{"none retains manual toggle", 0, 0, true, 0, 0, 31, 6},
		{"weapons also includes defenses", 0x18, 0, true, 0, 0, 0, 0},
		{"countermeasures clears", 8, 0, true, 0, 0, 0, 0},
		{"damage exits before clear", 8, 0x40, true, 0, 0, 31, 6},
		{"E0 gate", 8, 0, false, 0, 0, 31, 6},
		{"odd phase gate", 8, 0, true, 1, 0, 31, 6},
		{"IR incoming request retains", 8, 0, true, 2, 0x80, 31, 0},
		{"radar incoming request retains", 8, 0, true, 2, 0x40, 0, 6},
		{"both incoming requests retain", 8, 0, true, 2, 0xc0, 31, 6},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := NewDefenses(30, 30)
			d.IRActive, d.IRCharge, d.RadarSignature = 31, 32, 6
			d.ReconcileJammerRequests(tc.switches, tc.damage, tc.e0, tc.phase, tc.requests)
			if d.IRActive != tc.ir || d.RadarSignature != tc.radar || d.IRCharge != 32 || d.Chaff != 30 || d.Flares != 30 {
				t.Fatalf("unexpected later writer: %+v", d)
			}
		})
	}
}

func TestJammerCommandThenLaterWriterThenRecharge(t *testing.T) {
	d := NewDefenses(3, 3)
	d.Activate(ToggleIRJammer, 0, Pose{}, 6, nil, nil)
	d.Activate(ToggleRadarJammer, 0, Pose{}, 6, nil, func() uint16 { return 5 })
	d.AdvanceCounters(true) // next early E0 phase is before its command.
	if d.IRActive != 63 || d.IRCharge != 63 || d.RadarSignature != 6 {
		t.Fatal(d)
	}
	d.ReconcileJammerRequests(8, 0, true, 2, 0)
	if d.IRActive != 0 || d.IRCharge != 63 || d.RadarSignature != 0 {
		t.Fatal(d)
	}
	d.AdvanceCounters(true)
	if d.IRCharge != 64 {
		t.Fatal("later clear retains charge and next inactive phase recharges", d)
	}
}
