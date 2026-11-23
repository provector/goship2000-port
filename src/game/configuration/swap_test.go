package configuration

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestSwapDefaultsExplicitFalseAndNoControlsMigration(t *testing.T) {
	for _, raw := range []string{`{}`, `{"runtime":{"swap_yaw_roll_axis":false}}`, `{"runtime":{"swap_yaw_roll_axis":true}}`} {
		p := filepath.Join(t.TempDir(), "config.cfg")
		os.WriteFile(p, []byte(raw), 0600)
		d, e := Load(p)
		if e != nil {
			t.Fatal(e)
		}
		want := bytes.Contains([]byte(raw), []byte("true"))
		if d.Config.Runtime.SwapYawRollAxis != want {
			t.Fatal(raw, d.Config.Runtime)
		}
		d.Config.MusicVolume = 37
		if e = d.Save(); e != nil {
			t.Fatal(e)
		}
		again, e := Load(p)
		if e != nil || again.Config.Runtime.SwapYawRollAxis != want {
			t.Fatal("unrelated save", e)
		}
	}
}
