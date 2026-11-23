//go:build !headless

package menus

import (
	"fmt"
	"gunship2000-assets/game/render3d"
	"os"
	"path/filepath"
	"testing"
)

// An explicitly requested connected check of the normal mast controller and
// compositor. No original-game input, free-camera/art override or normal save.
func TestConnectedMastWindows(t *testing.T) {
	out := os.Getenv("GS2000_I1_NATIVE_MAST")
	if out == "" {
		t.Skip("explicit disposable connected capture output required")
	}
	if err := os.Mkdir(out, 0755); err != nil {
		t.Fatal(err)
	}
	for _, q := range []struct {
		id     int
		family string
	}{{1, "carrier"}, {3, "land"}, {4, "land"}} {
		t.Run(fmt.Sprintf("aircraft%d_%s", q.id, q.family), func(t *testing.T) {
			c := mastFixture(t, q.id, q.family)
			if err := c.Handle(Actions{ViewKey: 2}); err != nil {
				t.Fatal(err)
			}
			if err := RunOfficeWindow(c.Office, WindowOptions{FrontEnd: c, Development: true, Width: 960, Height: 720, TickRate: 70, SmokeFrames: 100, CaptureDirectory: filepath.Join(out, fmt.Sprintf("aircraft%d-%s", q.id, q.family))}); err != nil {
				t.Fatal(err)
			}
		})
	}
	t.Run("europe-source-tree-placement", func(t *testing.T) {
		c := mastFixture(t, 1, "land")
		// Declared inspection camera around source EURO7 placement00cb, not a
		// reconstructed user mission or naturally observed original frame.
		camera := render3d.Camera{Pose: render3d.Pose{H0: 224393, H1: 104675, Height: 34}}
		if err := c.ConfigureRenderingInspection(RenderingInspection{World: "EURO7", Camera: &camera, Exterior: true, NoHUD: true, Paused: true, Rate: DefaultPreviewVisualRate}); err != nil {
			t.Fatal(err)
		}
		if err := RunOfficeWindow(c.Office, WindowOptions{FrontEnd: c, Development: true, Width: 960, Height: 720, TickRate: 70, SmokeFrames: 100, CaptureDirectory: filepath.Join(out, "europe-tree")}); err != nil {
			t.Fatal(err)
		}
	})

}
