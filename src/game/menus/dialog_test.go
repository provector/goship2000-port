package menus

import (
	"bytes"
	"encoding/json"
	"gunship2000-assets/game/cockpit"
	"image/color"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestQuitOriginalStyleOperandAndWrapperLifetime(t *testing.T) {
	b := originalGS(t)
	// Resident file=A-e000. The frame argument is sign-extended E276, not0.
	sourceOperand(t, b, 0xf27f, "a076e298500ee8ea0783c40c")
	// Section3 file369c0+IP. JUMP owns4 only until its controller returns.
	sourceOperand(t, b, 0x369c0+0x1d2c, "c60676e204")
	sourceOperand(t, b, 0x369c0+0x1d45, "c60676e200")
	// Main reset; the other nonzero writer5 is a separate controller, not
	// ordinary17d6 HAPPY/SAD feedback. Do not map every debrief scene to4/5.
	sourceOperand(t, b, 0x100ca, "c60676e200")
	sourceOperand(t, b, 0x369c0+0x13da, "c60676e205")
}

func TestQuitDialogSceneStylesAndPaletteOwnership(t *testing.T) {
	a := preparedUI(t)
	for _, q := range []struct {
		name, scene, palette string
		screen               Screen
		style                int
		body                 byte
		rgb                  [3]byte
	}{
		{"office", "", "", ScreenOffice, 0, 234, [3]byte{19, 19, 19}},
		{"settings", "", "FIELD.PAL", ScreenSettings, 0, 234, [3]byte{19, 19, 19}},
		{"commander", "COMMISSN.SCN", "COMM.PAL", ScreenDebrief, 0, 234, [3]byte{19, 19, 19}},
		{"jump-promotion", "JUMP.SCN", "JUMP.PAL", ScreenDebrief, 4, 26, [3]byte{20, 20, 20}},
		{"crew-happy", "HAPPY.SCN", "HAPPY.PAL", ScreenDebrief, 0, 234, [3]byte{19, 19, 19}},
		{"crew-sad", "SAD.SCN", "SAD_2.PAL", ScreenDebrief, 0, 234, [3]byte{19, 19, 20}},
	} {
		t.Run(q.name, func(t *testing.T) {
			c := &FrontEnd{Office: &Office{Assets: a}, Menus: a.Menus, Screen: ScreenQuit, previousScreen: q.screen, quitBackground: bytes.Repeat([]byte{31}, 64000)}
			if q.screen == ScreenDebrief {
				c.Debrief = &DebriefView{Scene: q.scene}
			}
			if q.screen == ScreenSettings {
				c.Mission = &MissionSession{}
			}
			palette := append([]byte(nil), func() []byte {
				var raw []byte
				for _, v := range c.CurrentPalette() {
					r, g, b, a := v.RGBA()
					raw = append(raw, byte(r>>8), byte(g>>8), byte(b>>8), byte(a>>8))
				}
				return raw
			}()...)
			if got := c.quitFrameStyle(); got != q.style {
				t.Fatalf("source context%d want%d", got, q.style)
			}
			indexed := c.Frame(false)
			if indexed[90*320+95] != q.body {
				t.Fatalf("body index%d want%d", indexed[90*320+95], q.body)
			}
			art := a.Images["POINTER.PIC"]
			sx := q.style * 5
			for _, p := range []struct{ x, y, tx, ty int }{{92, 85, sx + 2, 100}, {92, 86, sx + 2, 101}, {90, 87, sx, 102}, {91, 87, sx + 1, 102}, {229, 87, sx + 4, 102}, {92, 104, sx + 2, 104}} {
				if indexed[p.y*320+p.x] != art[p.ty*320+p.tx] {
					t.Fatal("source bevel sample mismatch", p)
				}
			}
			for at, v := range indexed {
				x, y := at%320, at/320
				if (x < 90 || x >= 230 || y < 85 || y >= 105) && v != 31 {
					t.Fatalf("dialog changed surrounding scene%d,%d", x, y)
				}
				if x >= 92 && x < 228 && y >= 87 && y < 103 && v != q.body && v != 10 {
					t.Fatalf("glyph holes/interior lost sampled body at%d,%d=%d", x, y, v)
				}
			}
			rgba, err := c.RenderRGBA(false)
			if err != nil {
				t.Fatal(err)
			}
			pixel := rgba.RGBAAt(95, 90)
			if pixel.R != q.rgb[0]<<2|q.rgb[0]>>4 || pixel.G != q.rgb[1]<<2|q.rgb[1]>>4 || pixel.B != q.rgb[2]<<2|q.rgb[2]>>4 {
				t.Fatal("source scene RGB6 not retained", pixel, q.rgb)
			}
			var after []byte
			for _, v := range c.CurrentPalette() {
				r, g, b, a := v.RGBA()
				after = append(after, byte(r>>8), byte(g>>8), byte(b>>8), byte(a>>8))
			}
			if !bytes.Equal(palette, after) {
				t.Fatal("dialog mutated scene palette")
			}
		})
	}
}

func TestQuitDebriefCancellationKeepsPendingResultAndScene(t *testing.T) {
	c := nResultController(t)
	c.Rendering.Initialized.EndSortieForQoL()
	if err := c.finishSortieResult(); err != nil {
		t.Fatal(err)
	}
	for _, page := range []string{"commander", "jump", "feedback"} {
		t.Run(page, func(t *testing.T) {
			// Page override is supplied rendering coverage; it does not claim this
			// no-objective fixture naturally earned a commission or promotion.
			c.enterDebriefPage(page)
			c.Debrief.Tick = 130
			before := append([]byte(nil), c.Frame(false)...)
			view := *c.Debrief
			state := c.Store.State()
			pending, err := c.Store.PendingSortie()
			if err != nil {
				t.Fatal(err)
			}
			frozen, _ := json.Marshal(pending)
			beforeRGBA, err := c.RenderRGBA(false)
			if err != nil {
				t.Fatal(err)
			}
			latch := windowInput{}
			viewport := officeViewport(960, 720, false)
			if err := c.Handle(latch.ReadViewport(Buttons{AltQ: true, Inside: true}, viewport)); err != nil {
				t.Fatal(err)
			}
			if c.Screen != ScreenQuit {
				t.Fatal("quit edge not owned")
			}
			for n := 0; n < 20; n++ {
				c.Frame(false)
				if _, err := c.RenderRGBA(false); err != nil {
					t.Fatal(err)
				}
				if err := c.Tick(); err != nil {
					t.Fatal(err)
				}
			}
			if !reflect.DeepEqual(view, *c.Debrief) || !reflect.DeepEqual(state, c.Store.State()) {
				t.Fatal("modal advanced debrief or applied career")
			}
			got, err := c.Store.PendingSortie()
			if err != nil {
				t.Fatal(err)
			}
			raw, _ := json.Marshal(got)
			if !bytes.Equal(frozen, raw) {
				t.Fatal("dialog changed pending result/choice")
			}
			if output := os.Getenv("GS2000_TEST_N2_DIALOG_OUTPUT"); output != "" {
				frame := c.Frame(false)
				if err := writeIndexedFrame(filepath.Join(output, page+"-quit-after.png"), frame, c.CurrentPalette(), c.Office.Assets.Root, map[string]any{"scope": "Supplied port page/modal, source-backed style, not original runtime", "page": page, "style": c.quitFrameStyle()}); err != nil {
					t.Fatal(err)
				}
				if page == "jump" {
					// Reconstruct N.1's specific style0 mistake without altering assets,
					// palette, persistent state or production mode selection.
					wrong := append([]byte(nil), c.quitBackground...)
					real := c.Debrief
					c.Debrief = &DebriefView{}
					c.drawQuit(wrong)
					c.Debrief = real
					if err := writeIndexedFrame(filepath.Join(output, "jump-quit-before-reproduction.png"), wrong, c.CurrentPalette(), c.Office.Assets.Root, map[string]any{"scope": "Offline reconstruction of N.1 hard-coded style0; not original screenshot", "page": page, "style": 0}); err != nil {
						t.Fatal(err)
					}
				}
			}
			latch.ReadViewport(Buttons{}, viewport)
			if err := c.Handle(latch.ReadViewport(Buttons{Text: []rune{'N'}, Inside: true}, viewport)); err != nil {
				t.Fatal(err)
			}
			if c.Screen != ScreenDebrief || !reflect.DeepEqual(view, *c.Debrief) || c.ExitRequested {
				t.Fatal("cancel advanced page or quit")
			}
			afterRGBA, err := c.RenderRGBA(false)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, c.Frame(false)) || !bytes.Equal(beforeRGBA.Pix, afterRGBA.Pix) {
				t.Fatal("cancel did not restore identical indices/palette")
			}
		})
	}
}

func TestQuitSimulationRetainsSeparateGS2RecipeAtBothScales(t *testing.T) {
	for _, scale := range []int{1, 2} {
		t.Run(string(rune('0'+scale)), func(t *testing.T) {
			c := mastFixture(t, 0, "land")
			jFinishFade(t, c)
			p := c.Rendering
			p.Scene.RenderScale = scale
			p.resetPresentation()
			original := append([]byte(nil), c.Frame(false)...)
			palette := append([]color.Color(nil), c.CurrentPalette()...)
			camera, phase := p.Scene.Camera, p.Visual.Phase
			if err := c.Handle(Actions{Quit: true}); err != nil {
				t.Fatal(err)
			}
			box, err := cockpit.SceneConfirmationBounds(c.Office.Assets.Cockpit, false)
			if err != nil {
				t.Fatal(err)
			}
			frame := c.Frame(false)
			a := c.Office.Assets.Cockpit.ScenePresentation
			at := (box.Top*scale)*(320*scale) + box.Left*scale
			if len(frame) != 64000*scale*scale || frame[at] != a.ConfirmationRemap[original[at]]+a.Quit.Background {
				t.Fatal("GS2 destination remap replaced by menu style")
			}
			for at, v := range frame {
				x, y := at%(320*scale), at/(320*scale)
				if (x < box.Left*scale || x >= (box.Right+1)*scale || y < box.Top*scale || y >= (box.Bottom+1)*scale) && v != original[at] {
					t.Fatal("GS2 confirmation changed outside its source rectangle")
				}
			}
			if !reflect.DeepEqual(palette, []color.Color(c.CurrentPalette())) {
				t.Fatal("simulation modal changed world palette")
			}
			if err := c.Handle(Actions{Cancel: true}); err != nil {
				t.Fatal(err)
			}
			if c.Screen != ScreenScene || p.Scene.Camera != camera || p.Visual.Phase != phase {
				t.Fatal("simulation cancel moved camera/phase")
			}
		})
	}
}
