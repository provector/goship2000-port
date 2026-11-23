package flight

import "testing"

func TestHeldJammersChargeRandomAndRelease(t *testing.T) {
	d := NewDefenses(30, 30)
	draws := 0
	draw := func() uint16 { draws++; return 5 }
	for i := 0; i < 20; i++ {
		d.AdvanceCounters(true)
		d.SetManualHeld([2]bool{true, true}, 0, draw)
	}
	if draws != 1 || d.RadarSignature != 6 || d.IRActive != 45 {
		t.Fatal(draws, d)
	}
	d.SetManualHeld([2]bool{true, false}, 0, draw)
	if d.IRActive != 0 || d.IRCharge != 45 || d.RadarSignature != 6 {
		t.Fatal(d)
	}
	d.SetManualHeld([2]bool{}, 0, draw)
	if d.RadarSignature != 0 {
		t.Fatal("release latched")
	}
	d.SetManualHeld([2]bool{true, true}, 0x40, draw)
	if d.RadarSignature != 0 || d.IRActive != 0 || draws != 1 {
		t.Fatal("damaged equipment accepted")
	}
}
