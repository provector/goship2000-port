//go:build !headless

package menus

import (
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/game/configuration"
	"gunship2000-assets/game/flight"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestConnectedPlayerWeapons(t *testing.T) {
	out := os.Getenv("GS2000_M1_NATIVE")
	if out == "" {
		t.Skip("explicit fresh disposable connected output required")
	}
	if err := os.Mkdir(out, 0755); err != nil {
		t.Fatal(err)
	}
	scale, err := strconv.Atoi(os.Getenv("GS2000_M1_NATIVE_SCALE"))
	if err != nil || (scale != 1 && scale != 2) {
		t.Fatal("scale1/2 required")
	}
	c := armedLongbowMission(t)
	c.Office.Assets.Audio, err = assetdata.LoadAudioAssets(os.Getenv("GS2000_TEST_MISSIONS"))
	if err != nil {
		t.Fatal(err)
	}
	options := configuration.Defaults()
	options.SimulationRenderScale = scale
	c.Runtime = options
	p := c.Rendering
	if os.Getenv("GS2000_M1_NATIVE_SUPPLIED_POSE") == "1" {
		row := 24
		e := p.Initialized.Entities[row]
		if e.Kind != 6 || e.Participation != 1 {
			t.Fatal("declared natural target fixture changed")
		}
		s := p.Flight.State
		s.Pose = flight.Pose{H0: e.Pose.H0, H1: e.Pose.H1 - 3000, Height: int16(e.Pose.Height + 256)}
		s.Forward, s.Rotor, s.Collective, s.Hover, s.HeightControl, s.HeightTarget, s.CruiseSpeed = 0, 1, 16, 1, 1, int16(e.Pose.Height+256), 0
		p.Flight.Avionics.Target = uint16(row + 1)
		p.publishFlight()
		if err = p.ViewControls.Tick(p.cameraInputs(), p.Renderer.Assets); err != nil {
			t.Fatal(err)
		}
		p.publishView()
		p.DiagnosticPolicy += " Explicit supplied hovering firing pose near generated TOWED GUN24; population/accepted stores unchanged. Not a natural initialized first frame."
	}
	p.Scene.RenderScale = scale
	p.Frame, p.worldFrame = make([]byte, p.Scene.TargetPixels()), make([]byte, p.Scene.TargetPixels())
	p.resetPresentation()
	revision, mission := c.Store.State().Revision, c.Mission.Data.ObjectHash
	if err = writeDevelopmentCheckpoint(out, 0, c); err != nil {
		t.Fatal(err)
	}
	if err = RunOfficeWindow(c.Office, WindowOptions{Runtime: options, FrontEnd: c, Development: true, Width: 960, Height: 720, TickRate: 70, SmokeFrames: 3600, CaptureDirectory: filepath.Join(out, "checkpoints")}); err != nil {
		t.Fatal(err)
	}
	if err = writeDevelopmentCheckpoint(out, 3600, c); err != nil {
		t.Fatal(err)
	}
	if c.Store.State().Revision != revision || c.Mission.Data.ObjectHash != mission {
		t.Fatal("native combat changed save or regenerated mission")
	}
	launches := 0
	for _, e := range p.Flight.CombatEvents {
		if e.Kind == "launch" {
			launches++
		}
	}
	if launches == 0 {
		t.Fatal("native physical trigger never launched a projectile")
	}
	if os.Getenv("GS2000_M1_NATIVE_SUPPLIED_POSE") == "1" {
		destroyed := 0
		for _, e := range p.Flight.CombatEvents {
			if e.Kind == "destroyed" {
				destroyed++
			}
		}
		if destroyed == 0 {
			t.Fatal("supplied native firing case did not destroy any generated target")
		}
	}
	if p.GPS != nil || c.sceneConfirmationPresented() || c.Screen != ScreenScene {
		t.Fatal("native combat inspection left a modal")
	}
}
