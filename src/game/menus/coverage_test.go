package menus

import (
	"bytes"
	"testing"
)

func TestFullSceneProbeHasExactWriteCoverage(t *testing.T) {
	c := naturalControllerProfile(t, true, "")
	defer c.AbandonFilmRecording()
	p := c.Rendering
	row := -1
	for _, e := range p.Initialized.Entities {
		if e.RuntimeRow > 5 && e.Participation != 0 && e.Kind&0x38 == 0 {
			for _, in := range p.instances {
				if in.ID == e.ID {
					row = e.RuntimeRow
					break
				}
			}
		}
		if row >= 0 {
			break
		}
	}
	if row < 0 {
		t.Fatal("normal mission target absent")
	}
	p.Flight.Avionics.Target = uint16(row + 1)
	p.Flight.Avionics.TargetPanelZoom = 512
	p.Scene.GraphicsOptions |= 8
	p.collectMaterialMask = false
	for _, scale := range []int{1, 2} {
		for _, mode := range []int{0, 7} {
			p.Scene.RenderScale = scale
			in := p.Cockpit
			in.PrimaryMFD = 5
			in.Arrangement = 0
			in.CameraMode = mode
			for _, flags := range []byte{0, 1} {
				in.SystemFlags = flags
				size := p.Scene.TargetPixels()
				oldA, oldB := make([]byte, size), bytes.Repeat([]byte{255}, size)
				p.collectMaterialMask = false
				for _, dst := range [][]byte{oldA, oldB} {
					if err := p.composeSceneOverlays(dst, in, c.Office.Assets.Cockpit); err != nil {
						t.Fatal(err)
					}
				}
				const untouched = 0xfe
				p.skyMask = bytes.Repeat([]byte{untouched}, size)
				p.collectMaterialMask = true
				actual := make([]byte, size)
				if err := p.composeSceneOverlays(actual, in, c.Office.Assets.Cockpit); err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(actual, oldA) {
					t.Fatal("coverage tracking changed indexed output")
				}
				for i := 0; i < size; i++ {
					old := oldA[i] != 0 || oldB[i] != 255
					fast := p.skyMask[i] != untouched
					if old != fast {
						t.Fatal("probe coverage differs", scale, mode, flags, i, old, fast)
					}
				}
			}
		}
	}
}

func TestOverlayScratchReuseAndTextExclusions(t *testing.T) {
	var scratch presentationScratch
	for _, size := range []int{64000, 256000, 64000} {
		mask := bytes.Repeat([]byte{0x5c}, size)
		draw := func(dst []byte) error { dst[5] = 0; dst[6] = 255; return nil }
		if e := scratch.erase(mask, draw); e != nil {
			t.Fatal(e)
		}
		if mask[5] != 0 || mask[6] != 0 || mask[7] != 0x5c {
			t.Fatal("same-ink or transparent owner lost")
		}
		if n := testing.AllocsPerRun(100, func() {
			if e := scratch.erase(mask, draw); e != nil {
				panic(e)
			}
		}); n != 0 {
			t.Fatal("overlay churn", n)
		}
	}
}
