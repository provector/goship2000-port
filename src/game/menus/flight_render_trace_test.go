package menus

import (
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Opt-in reproduction uses the accepted mission/controls path. It deliberately
// does not reconstruct the screenshot's unknown mission or supply a camera pose.
func TestAcceptedFlightBankTrace(t *testing.T) {
	root := os.Getenv("GS2000_L3_FLIGHT_TRACE")
	if root == "" {
		t.Skip("set GS2000_L3_FLIGHT_TRACE to a fresh trace directory")
	}
	for _, mode := range []string{"stepped", "smooth"} {
		for _, scale := range []int{1, 2} {
			c := mastFixture(t, 0, "pass")
			jFinishFade(t, c)
			p := c.Rendering
			p.Scene.RenderScale = scale
			p.Frame, p.worldFrame = make([]byte, p.Scene.TargetPixels()), make([]byte, p.Scene.TargetPixels())
			p.presentation.mode = mode
			p.resetPresentation()
			dir := filepath.Join(root, fmt.Sprintf("%s-scale%d", mode, scale))
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatal(err)
			}
			write := func(name string, b []byte) {
				f, err := os.OpenFile(filepath.Join(dir, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
				if err != nil {
					t.Fatal(err)
				}
				if _, err = f.Write(b); err != nil {
					t.Fatal(err)
				}
				if err = f.Close(); err != nil {
					t.Fatal(err)
				}
			}
			var rows []any
			for frame := 0; frame <= 42*60; frame++ {
				now := time.Duration(frame) * time.Second / 60
				held := [4]bool{}
				if now >= 10*time.Second && now < 18*time.Second {
					held[0] = true
				} else if now >= 26*time.Second && now < 34*time.Second {
					held[1] = true
				}
				collective := [11]bool{now < 3*time.Second}
				var commands []int
				if frame == 0 {
					commands = []int{0x100}
				}
				if err := c.Handle(Actions{InputTime: now, FlightHeldValid: true, FlightHeldKeys: collective, ViewShift: now < 3*time.Second, FlightCommands: commands, SceneDirections: held}); err != nil {
					t.Fatal(err)
				}
				if frame > 0 {
					if err := c.AdvanceRendering(1.0 / 60); err != nil {
						t.Fatal(err)
					}
				}
				if c.Screen != ScreenScene {
					t.Fatalf("trace left scene at%.3fs: %s", now.Seconds(), c.Screen)
				}
				c.Frame(false)
				if p.Error != nil {
					t.Fatal(p.Error)
				}
				if frame%15 == 0 {
					rows = append(rows, struct {
						Seconds             float64
						Camera, Pose, Stats any
						Steps               uint64
					}{now.Seconds(), p.Scene.Camera, p.Flight.State.Pose, p.Stats, p.Flight.Steps})
				}
				if frame%120 == 0 {
					name := fmt.Sprintf("frame%04d", frame)
					b, err := json.MarshalIndent(p.Scene, "", "  ")
					if err != nil {
						t.Fatal(err)
					}
					write(name+".scene.json", b)
					write(name+".world.indices", p.worldFrame)
					write(name+".display.indices", p.Frame)
					f, err := os.OpenFile(filepath.Join(dir, name+".png"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
					if err != nil {
						t.Fatal(err)
					}
					img := image.NewPaletted(image.Rect(0, 0, 320*scale, 200*scale), p.Palette)
					copy(img.Pix, p.Frame)
					if err = png.Encode(f, img); err != nil {
						t.Fatal(err)
					}
					if err = f.Close(); err != nil {
						t.Fatal(err)
					}
				}
			}
			b, err := json.MarshalIndent(rows, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			write("trace.json", b)
		}
	}
}
