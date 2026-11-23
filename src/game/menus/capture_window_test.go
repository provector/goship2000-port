//go:build !headless

package menus

import (
	"os"
	"path/filepath"
	"testing"
)

// This opt-in window consumes real backend events in the normal wrapper. Its
// controller/save fixture is disposable; an external X11 probe supplies input.
func TestConnectedCaptureWindow(t *testing.T) {
	out := os.Getenv("GS2000_K_NATIVE_CAPTURE")
	if out == "" {
		t.Skip("explicit disposable connected capture output required")
	}
	c := sourceFrontEnd(t, true)
	if err := c.openMission(); err != nil {
		t.Fatal(err)
	}
	before := c.Store.State().Revision
	if err := RunOfficeWindow(c.Office, WindowOptions{FrontEnd: c, Development: true, DrawPointer: true, Width: 960, Height: 720, TickRate: 70, CaptureDirectory: filepath.Join(out, "captures")}); err != nil {
		t.Fatal(err)
	}
	if c.Store.State().Revision != before {
		t.Fatal("capture-only backend probe changed pilot state")
	}
}
