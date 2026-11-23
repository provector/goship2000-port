package configuration

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOldConfigLoggingDefaultsAndValidation(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "config.cfg")
	os.WriteFile(p, []byte(`{"runtime":{"presentation_mode":"stepped","presentation_fps":30}}`), 0600)
	d, e := Load(p)
	if e != nil {
		t.Fatal(e)
	}
	if d.Config.Logging != DefaultLogging() {
		t.Fatal(d.Config.Logging)
	}
	d.Config.Logging.ShowLog = true
	d.Config.Logging.WriteLog = true
	d.Config.Logging.Level = "DEBUG"
	if e = d.Save(); e != nil {
		t.Fatal(e)
	}
	d, e = Load(p)
	if e != nil || !d.Config.Logging.ShowLog || !d.Config.Logging.WriteLog {
		t.Fatal(e)
	}
	for _, n := range []string{"../x", "a/b", "a\\b", ".hidden", "CON.txt", "a.previous", "", "x y"} {
		c := DefaultLogging()
		c.FileName = n
		if c.Validate() == nil {
			t.Fatal(n)
		}
	}
	for _, v := range []int{0, -1, 101} {
		c := DefaultLogging()
		c.FileLimitMB = v
		if c.Validate() == nil {
			t.Fatal(v)
		}
	}
	if d.Config.Runtime.PresentationFPS != 30 {
		t.Fatal("changed presentation")
	}
}
