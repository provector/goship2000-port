package configuration

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultsAndExplicitOldValues(t *testing.T) {
	c := DefaultConfig()
	if c.Runtime.SimulationRenderScale != 2 || c.Fullscreen || c.Runtime.PresentationMode != "smooth" || c.Runtime.PresentationFPS != 60 || c.Logging.TerminalTarget != "custom" {
		t.Fatal(c)
	}
	if Defaults().SimulationRenderScale != 1 {
		t.Fatal("legacy programmatic/film fallback changed")
	}
	for _, raw := range []string{`{}`, `{"fullscreen":false,"runtime":{"simulation_render_scale":1,"presentation_mode":"stepped","presentation_fps":30},"logging":{"terminal_target":"system"}}`} {
		p := filepath.Join(t.TempDir(), "config.cfg")
		os.WriteFile(p, []byte(raw), 0600)
		d, e := Load(p)
		if e != nil {
			t.Fatal(e)
		}
		expectedScale := 2
		expectedFPS := 60
		target := "custom"
		if raw != "{}" {
			expectedScale = 1
			expectedFPS = 30
			target = "system"
		}
		if d.Config.Runtime.SimulationRenderScale != expectedScale || d.Config.Runtime.PresentationFPS != expectedFPS || d.Config.Fullscreen || d.Config.Logging.TerminalTarget != target {
			t.Fatal(d.Config)
		}
		d.Config.MusicVolume = 37
		if e = d.Save(); e != nil {
			t.Fatal(e)
		}
		again, e := Load(p)
		if e != nil || again.Config.Runtime != d.Config.Runtime || again.Config.Logging != d.Config.Logging {
			t.Fatal(again, e)
		}
	}
	c.Logging.TerminalTarget = "other"
	if c.Validate() == nil {
		t.Fatal("invalid target admitted")
	}
}
