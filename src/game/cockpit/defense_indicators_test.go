package cockpit

import "testing"

func TestDefenseIndicatorExactIndexAndExpiry(t *testing.T) {
	d := Definition{Colours: [6]byte{34, 10, 71, 10, 65, 68}}
	d.Parameters[0x78/2] = 10
	d.Parameters[0x7a/2] = 20
	d.Parameters[0x7c/2] = 30
	d.Parameters[0x7e/2] = 20
	p := make([]byte, 64000)
	fill(p, 10, 20, 11, 7, 34, Full)
	fill(p, 30, 20, 11, 7, 34, Full)
	p[20*320+11] = 0x1a
	drawDefenseIndicators(p, d, DefenseInputs{IRActive: 9, RadarSignature: 8}, 0)
	if p[20*320+10] != 10 || p[20*320+11] != 0x1a || p[25*320+39] != 10 || p[20*320+20] != 34 || p[26*320+10] != 34 {
		t.Fatal("source x,y,10,6,CPIT96,CPIT97 ABI")
	}
	clear(p)
	p[20*320+10] = 34
	drawDefenseIndicators(p, d, DefenseInputs{IRActive: 8}, 0)
	if p[20*320+10] != 34 {
		t.Fatal("last8 charge blinks")
	}
	drawDefenseIndicators(p, d, DefenseInputs{IRActive: 8}, 1)
	if p[20*320+10] != 10 {
		t.Fatal("blink phase1")
	}
}
