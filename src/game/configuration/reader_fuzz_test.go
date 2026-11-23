package configuration

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"
)

func FuzzSettingsDocument(f *testing.F) {
	valid, e := json.Marshal(DefaultConfig())
	if e != nil {
		f.Fatal(e)
	}
	f.Add(valid)
	f.Add([]byte(`{"runtime":{"simulation_render_scale":1}}`))
	f.Add([]byte(`{"runtime":null}`))
	root := f.TempDir()
	path := filepath.Join(root, "config.cfg")
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 32<<10 {
			return
		}
		doc, e := decodeSettingsAt(path, root, b)
		if e != nil {
			return
		}
		if e = doc.Config.Validate(); e != nil {
			t.Fatal(e)
		}
		other, e := decodeSettingsAt(path, root, b)
		if e != nil || !reflect.DeepEqual(other.Config, doc.Config) {
			t.Fatal("unstable parse", e)
		}
	})
}
