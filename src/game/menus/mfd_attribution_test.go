package menus

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"gunship2000-assets/game/cockpit"
)

func TestComancheMapGutterIsSourceArtworkAcrossModes(t *testing.T) {
	for _, id := range []int{6, 7} {
		for _, scale := range []int{1, 2} {
			c := mastFixture(t, id, "land")
			jFinishFade(t, c)
			p := c.Rendering
			p.Scene.RenderScale = scale
			p.Frame, p.worldFrame = make([]byte, p.Scene.TargetPixels()), make([]byte, p.Scene.TargetPixels())
			p.presentation.mode = "stepped"
			a := c.Office.Assets.Cockpit
			art := a.Images["COM_CPIT.PIC"]
			before, _ := json.Marshal(p.Flight.State)
			for _, mode := range []int16{0, 1, 2, 3, 4, 0} {
				p.Cockpit.PrimaryMFD, p.Cockpit.SecondaryMFD = mode, mode
				p.Cockpit.PrimaryMap, p.Cockpit.SecondaryMap = mode == 0, mode == 0
				p.resetPresentation()
				p.Dirty = true
				c.Frame(false)
				if p.Error != nil {
					t.Fatal(p.Error)
				}
				// Independent raw CPIT60 is173; tile copier37b0 clips x<173.
				// Source copy-run restoration owns the adjacent x172 artwork,
				// including its index45h strip; COM E1.0 skips the remap pass.
				for y := 130; y <= 165; y++ {
					for dy := 0; dy < scale; dy++ {
						for dx := 0; dx < scale; dx++ {
							at := (y*scale+dy)*(320*scale) + 172*scale + dx
							if p.Frame[at] != art.Pixels[y*320+172] {
								t.Fatalf("COM%d scale%d mode%d source gutter%d altered: %x != %x", id, scale, mode, y, p.Frame[at], art.Pixels[y*320+172])
							}
						}
					}
				}
			}
			after, _ := json.Marshal(p.Flight.State)
			if !bytes.Equal(before, after) {
				t.Fatal("MFD composition mutated authoritative flight state")
			}
		}
	}
}

// Opt-in writer attribution uses a genuine prepared Training mission, with an
// explicitly supplied acquired target. It does not reconstruct the user's seed.
func TestMFDWriterAttribution(t *testing.T) {
	root := os.Getenv("GS2000_L3_MFD_REPORT")
	if root == "" {
		t.Skip("set GS2000_L3_MFD_REPORT for indexed-stage evidence")
	}
	for _, id := range []int{6, 7} {
		for _, scale := range []int{1, 2} {
			c := mastFixture(t, id, "land")
			jFinishFade(t, c)
			p := c.Rendering
			p.Scene.RenderScale = scale
			p.Frame, p.worldFrame = make([]byte, p.Scene.TargetPixels()), make([]byte, p.Scene.TargetPixels())
			p.presentation.mode = "stepped"
			p.Cockpit.PrimaryMFD, p.Cockpit.SecondaryMFD = 5, 0
			p.Cockpit.PrimaryMap, p.Cockpit.SecondaryMap = false, true
			for n, e := range p.Initialized.Entities {
				if n > 0 && e.ModelID > 0 {
					p.Flight.Avionics.Target = uint16(n + 1)
					break
				}
			}
			p.publishAvionics()
			p.Flight.Avionics.TargetPanelZoom = 512
			p.Scene.GraphicsOptions |= 8
			p.Cockpit.PrimaryMFD, p.Cockpit.SecondaryMFD = 5, 0
			p.resetPresentation()
			if err := p.commitReferenceFrame(c.Office.Assets.Cockpit); err != nil {
				t.Fatal(err)
			}
			_, in := p.presentationInputs()
			stages := map[string][]byte{"world": append([]byte(nil), p.worldFrame...)}
			frame := append([]byte(nil), p.worldFrame...)
			if err := cockpit.ComposeScaled(frame, c.Office.Assets.Cockpit, in, scale); err != nil {
				t.Fatal(err)
			}
			stages["cockpit"] = append([]byte(nil), frame...)
			if err := p.drawTargetMFD(frame, in, c.Office.Assets.Cockpit); err != nil {
				t.Fatal(err)
			}
			stages["target"] = frame
			dir := filepath.Join(root, fmt.Sprintf("comanche%d-scale%d", id, scale))
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatal(err)
			}
			for name, raw := range stages {
				if err := os.WriteFile(filepath.Join(dir, name+".indices"), raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			b, err := json.MarshalIndent(struct {
				Aircraft, Scale int
				Inputs          cockpit.Inputs
			}{id, scale, in}, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(filepath.Join(dir, "inputs.json"), b, 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
}
