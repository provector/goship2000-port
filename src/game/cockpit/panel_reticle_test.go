package cockpit

import (
	"bytes"
	"testing"
)

func TestPanelReticleDestinationAndOwnership(t *testing.T) {
	a := Assets{Sprites: map[string]Image{"target-symbol-1": {Width: 3, Height: 1, Pixels: []byte{0, 15, 0}}}}
	in := Inputs{HUD: true, TargetAim: &TargetAimInputs{TargetActive: true, CenterX: 160, CenterY: 70, ClipBottom: 199}, TargetSymbol: 1}
	panel := Rect{Left: 159, Top: 59, Right: 181, Bottom: 96}
	p := make([]byte, 2*logicalPixels)
	for i := 0; i < logicalPixels; i++ {
		p[i] = byte(0x60 + i%20)
		p[i+logicalPixels] = 7
	}
	want := bytes.Clone(p)
	old := bytes.Clone(p)
	if err := a.drawTargeting(want, in); err != nil {
		t.Fatal(err)
	}
	if err := a.DrawPanelReticle(p, in, panel); err != nil {
		t.Fatal(err)
	}
	var writes, transparent int
	for i := 0; i < logicalPixels; i++ {
		x, y := i%320, i/320
		ref := old
		if x >= panel.Left && x <= panel.Right && y >= panel.Top && y <= panel.Bottom {
			ref = want
			if want[i+logicalPixels] == 0 {
				writes++
			} else {
				transparent++
			}
		}
		if p[i] != ref[i] || p[i+logicalPixels] != ref[i+logicalPixels] {
			t.Fatalf("unexpected ink or owner at %d,%d", x, y)
		}
	}
	if writes == 0 || transparent == 0 {
		t.Fatal("fixture requires both written and untouched destinations")
	}
}
