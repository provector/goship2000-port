package menus

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// The Office-colored prebrief popup is a requested native presentation policy.
// These are supplied scene/fade tests through actual input, frame and palette
// owners, not observations of original DOS execution or physical OS input.
func TestPrebriefQuitLocalUIPalette(t *testing.T) {
	c := missionController(t)
	readyMission(t, c)
	for scene := 0; scene < 6; scene++ {
		for _, scale := range []int{1, 2} {
			for _, phase := range []string{"none", "immediate", "outgoing", "black", "incoming", "settled"} {
				t.Run(fmt.Sprintf("scene%d/%dx/%s", scene, scale, phase), func(t *testing.T) {
					m := *c.Mission.Data
					m.Scene, m.StableScene = scene, "FIELD"
					if scene == 4 {
						m.StableScene = "FIELD3"
					}
					if scene == 2 || scene == 5 {
						m.StableScene = "FIELD2"
					}
					pilot := c.Store.State().SelectedPilot()
					v, err := NewBriefingView(c.Office.Assets.Missions, &m, pilot, 101)
					if err != nil {
						t.Fatal(err)
					}
					control, err := NewBriefingView(c.Office.Assets.Missions, &m, pilot, 101)
					if err != nil {
						t.Fatal(err)
					}
					c.Mission.Data, c.Mission.View = &m, v
					c.Screen, c.fade = ScreenOffice, nil
					from, err := c.RenderRGBA(false)
					if err != nil {
						t.Fatal(err)
					}
					c.Screen, c.FadeMilliseconds = ScreenInterstitial, 150
					if phase != "none" {
						c.BeginSceneFade(from)
						switch phase {
						case "immediate":
							c.fade.Tick = 0
						case "outgoing":
							c.fade.Tick = c.fade.LegTicks / 2
						case "black":
							c.fade.Tick = c.fade.LegTicks
						case "incoming":
							c.fade.Tick = c.fade.LegTicks + c.fade.LegTicks/2
						case "settled":
							c.fade.Tick = c.fade.duration()
						}
					}
					before, err := c.RenderRGBA(false)
					if err != nil {
						t.Fatal(err)
					}
					oldIndices := append([]byte(nil), c.Frame(false)...)
					palette, save, mission := filmJSON(c.CurrentPalette()), filmJSON(c.Store.State()), filmJSON(c.Mission.Data)
					officeRNG, commander, pacing := c.Office.rng, v.commander, v.commanderPacingPhase
					interstitialTicks := c.Mission.interstitialTicks
					var fadeCopy *sceneFade
					if c.fade != nil {
						x := *c.fade
						fadeCopy = &x
					}
					latch := windowInput{}
					viewport := officeViewport(320*scale, 200*scale, true)
					if err := c.Handle(latch.ReadViewport(Buttons{AltQ: true}, viewport)); err != nil {
						t.Fatal(err)
					}
					if c.Screen != ScreenQuit || !c.usesLocalQuitUIPalette() {
						t.Fatal("prebrief quit not reached")
					}
					indices := append([]byte(nil), c.Frame(false)...)
					if indices[90*320+95] != 234 {
						t.Fatal("canonical source body index changed")
					}
					rgba, err := c.RenderRGBA(false)
					if err != nil {
						t.Fatal(err)
					}
					if rgba.RGBAAt(95, 90) != (color.RGBA{77, 77, 77, 255}) {
						t.Fatal("Office gray UI body absent", rgba.RGBAAt(95, 90))
					}
					for y := 0; y < 200; y++ {
						for x := 0; x < 320; x++ {
							if x >= 90 && x < 230 && y >= 85 && y < 105 {
								r, g, b, a := c.Office.Assets.Palette[indices[y*320+x]].RGBA()
								want := color.RGBA{byte(r >> 8), byte(g >> 8), byte(b >> 8), byte(a >> 8)}
								if rgba.RGBAAt(x, y) != want {
									t.Fatalf("local UI palette mismatch %d,%d", x, y)
								}
							} else if rgba.RGBAAt(x, y) != before.RGBAAt(x, y) {
								t.Fatalf("scene changed outside popup %d,%d", x, y)
							}
						}
					}
					// UI remains source320x200 at either configured presentation scale.
					// Compare the actual nearest expansion kernel separately from 3D2x.
					presented := nearestRGBA(rgba, image.Rect(0, 0, 320*scale, 200*scale))
					prior := nearestRGBA(before, presented.Bounds())
					for y := 0; y < 200*scale; y++ {
						for x := 0; x < 320*scale; x++ {
							if x >= 90*scale && x < 230*scale && y >= 85*scale && y < 105*scale {
								continue
							}
							if presented.RGBAAt(x, y) != prior.RGBAAt(x, y) {
								t.Fatalf("scaled scene changed %d,%d", x, y)
							}
						}
					}
					if scene == 4 && phase == "none" {
						canonical, e := indexedFrameRGBA(indices, c.CurrentPalette())
						if e != nil {
							t.Fatal(e)
						}
						if canonical.RGBAAt(95, 90) != (color.RGBA{162, 142, 60, 255}) {
							t.Fatal("canonical ICE recipe changed")
						}
						if dir := os.Getenv("GS2000_TEST_R11_PREFLIGHT_QUIT_OUTPUT"); dir != "" {
							if e = os.MkdirAll(dir, 0755); e != nil {
								t.Fatal(e)
							}
							for name, frame := range map[string]*image.RGBA{"gray": presented, "canonical-gold": nearestRGBA(canonical, presented.Bounds())} {
								f, e := os.Create(filepath.Join(dir, fmt.Sprintf("ice-quit-%dx-%s.png", scale, name)))
								if e != nil {
									t.Fatal(e)
								}
								e = png.Encode(f, frame)
								ce := f.Close()
								if e != nil {
									t.Fatal(e)
								}
								if ce != nil {
									t.Fatal(ce)
								}
							}
						}
					}
					path := filepath.Join(t.TempDir(), "misleading.png")
					if e := WriteFrontEndFrame(path, c, false); e == nil || !strings.Contains(e.Error(), "separate UI palette") {
						t.Fatal("misleading indexed capture admitted", e)
					}
					if _, e := os.Stat(path); !os.IsNotExist(e) {
						t.Fatal("rejected capture created output", e)
					}
					for i := 0; i < 5; i++ {
						if e := c.Tick(); e != nil {
							t.Fatal(e)
						}
					}
					waited, e := c.RenderRGBA(false)
					if e != nil {
						t.Fatal(e)
					}
					if !bytes.Equal(rgba.Pix, waited.Pix) || !bytes.Equal(indices, c.Frame(false)) {
						t.Fatal("modal redraw or wait changed frame")
					}
					if !bytes.Equal(palette, filmJSON(c.CurrentPalette())) || !bytes.Equal(save, filmJSON(c.Store.State())) || !bytes.Equal(mission, filmJSON(c.Mission.Data)) {
						t.Fatal("modal changed palette, save or mission")
					}
					if officeRNG != c.Office.rng || !reflect.DeepEqual(commander, v.commander) || pacing != v.commanderPacingPhase || interstitialTicks != c.Mission.interstitialTicks {
						t.Fatal("modal advanced RNG or view state")
					}
					if !reflect.DeepEqual(fadeCopy, c.fade) {
						t.Fatal("modal advanced fade")
					}
					if v.effects.Int63() != control.effects.Int63() {
						t.Fatal("modal consumed presentation RNG")
					}
					latch.ReadViewport(Buttons{}, viewport)
					cancel := Buttons{Text: []rune{'N'}}
					if scale == 2 {
						cancel = Buttons{Escape: true}
					}
					if e := c.Handle(latch.ReadViewport(cancel, viewport)); e != nil {
						t.Fatal(e)
					}
					restored, e := c.RenderRGBA(false)
					if e != nil {
						t.Fatal(e)
					}
					if c.Screen != ScreenInterstitial || c.ExitRequested || !bytes.Equal(before.Pix, restored.Pix) || !bytes.Equal(oldIndices, c.Frame(false)) || !bytes.Equal(save, filmJSON(c.Store.State())) {
						t.Fatal("cancel did not restore exact reached state")
					}
				})
			}
		}
	}
}

func TestQuitLocalUIPaletteScopeAndConfirmation(t *testing.T) {
	for _, previous := range []Screen{ScreenOffice, ScreenBriefing, ScreenDebrief, ScreenSettings, ScreenScene, ScreenSceneInitialization, ScreenReplay, ScreenFilms} {
		c := &FrontEnd{Screen: ScreenQuit, previousScreen: previous}
		if c.usesLocalQuitUIPalette() {
			t.Fatalf("local UI palette leaked to %s", previous)
		}
	}
	for _, previous := range []Screen{ScreenStartup, ScreenInterstitial} {
		c := &FrontEnd{Screen: ScreenQuit, previousScreen: previous}
		if !c.usesLocalQuitUIPalette() {
			t.Fatalf("local UI palette absent for %s", previous)
		}
	}
	for _, response := range []string{"Y", "N", "Esc"} {
		t.Run(response, func(t *testing.T) {
			c := missionController(t)
			readyMission(t, c)
			c.Screen = ScreenInterstitial
			c.fade = nil
			var latch windowInput
			v := officeViewport(640, 480, false)
			if e := c.Handle(latch.ReadViewport(Buttons{AltQ: true}, v)); e != nil {
				t.Fatal(e)
			}
			latch.ReadViewport(Buttons{}, v)
			b := Buttons{Text: []rune(response)}
			if response == "Esc" {
				b = Buttons{Escape: true}
			}
			if e := c.Handle(latch.ReadViewport(b, v)); e != nil {
				t.Fatal(e)
			}
			if response == "Y" {
				if !c.ExitRequested || c.Screen != ScreenQuit {
					t.Fatal("Y no longer exits")
				}
			} else if c.ExitRequested || c.Screen != ScreenInterstitial {
				t.Fatal("N/Esc no longer cancels")
			}
		})
	}
}

func TestQuitLocalUIPaletteKeepsSpecialDebriefRecipes(t *testing.T) {
	a := preparedUI(t)
	for _, scene := range []string{"JUMP.SCN", "KIA.SCN", "HAPPY.SCN", "SAD.SCN", "COMMISSN.SCN"} {
		t.Run(scene, func(t *testing.T) {
			c := &FrontEnd{Office: &Office{Assets: a}, Menus: a.Menus, Screen: ScreenQuit, previousScreen: ScreenDebrief, Debrief: &DebriefView{Scene: scene}, quitBackground: bytes.Repeat([]byte{31}, 64000)}
			indexed := c.Frame(false)
			expected, e := indexedFrameRGBA(indexed, c.CurrentPalette())
			if e != nil {
				t.Fatal(e)
			}
			actual, e := c.RenderRGBA(false)
			if e != nil {
				t.Fatal(e)
			}
			if c.usesLocalQuitUIPalette() || !bytes.Equal(expected.Pix, actual.Pix) {
				t.Fatal("special debrief recipe changed")
			}
		})
	}
}
