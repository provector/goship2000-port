package configuration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestExistingOffAndFreshModern(t *testing.T) {
	c := DefaultConfig()
	if !c.Runtime.FloatingGeometry || c.Runtime.SimulationResolution != "640x480" || c.Runtime.PresentationFPS != 60 || c.DrawFPSCPU {
		t.Fatal(c.Runtime)
	}
	for _, extra := range []string{"", `,"floating_point_rendering":false`, `,"floating_point_rendering":true`} {
		dir := t.TempDir()
		raw := []byte(`{"runtime":{"simulation_resolution":"1024x768","presentation_fps":37,"presentation_mode":"stepped","custom_u21":"keep"` + extra + `}}`)
		if err := os.WriteFile(filepath.Join(dir, "config.cfg"), raw, 0600); err != nil {
			t.Fatal(err)
		}
		d, _, err := Initialize(filepath.Join(dir, Filename))
		if err != nil {
			t.Fatal(err)
		}
		before := d.Config.Runtime
		if before.FloatingGeometry != (extra == `,"floating_point_rendering":true`) || before.PresentationFPS != 37 || before.SimulationResolution != "1024x768" || before.PresentationMode != "stepped" {
			t.Fatal(before)
		}
		for _, fp := range []bool{true, false} {
			d.Config.Runtime.FloatingGeometry = fp
			if err = d.Save(); err != nil {
				t.Fatal(err)
			}
			d, err = Load(d.Path)
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]json.RawMessage
			if err = json.Unmarshal(d.raw["runtime"], &fields); err != nil {
				t.Fatal(err)
			}
			if d.Config.Runtime.FloatingGeometry != fp || fields["floating_point_rendering"] == nil || string(fields["custom_u21"]) != `"keep"` {
				t.Fatal("lost saved value", fields)
			}
		}
	}
}
