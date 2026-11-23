package configuration

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestStockTextUpgradePreservesEditsAndStepped30(t *testing.T) {
	c := DefaultConfig()
	var old map[string]Text
	json.Unmarshal([]byte(q1StockTextJSON), &old)
	for k, v := range old {
		c.UI[k] = v
	}
	custom := c.UI["render_scale"]
	custom.Label = "Matt's resolution"
	q := custom.Choices["2"]
	q.Label = "My high resolution"
	custom.Choices["2"] = q
	c.UI["render_scale"] = custom
	c.Runtime.PresentationMode = "stepped"
	c.Runtime.PresentationFPS = 30
	c.WindowWidth = 1111
	c.WindowHeight = 777
	b, _ := json.MarshalIndent(c, "", "  ")
	p := filepath.Join(t.TempDir(), "config.cfg")
	os.WriteFile(p, b, 0600)
	d, e := Load(p)
	if e != nil {
		t.Fatal(e)
	}
	after, _ := os.ReadFile(p)
	if !bytes.Equal(b, after) {
		t.Fatal("opening rewrote cfg")
	}
	if d.Config.UI["presentation"].Label != "Framerate" || d.Config.UI["presentation"].Choices["stepped"].Label != "10fps (Legacy)" || d.Config.UI["presentation"].Choices["smooth"].Label != "60fps" {
		t.Fatal(d.Config.UI["presentation"])
	}
	if d.Config.UI["render_scale"].Label != "Matt's resolution" || d.Config.UI["render_scale"].Choices["2"].Label != "My high resolution" || d.Config.UI["render_scale"].Tooltip != DefaultText()["render_scale"].Tooltip {
		t.Fatal("field-wise wording upgrade", d.Config.UI["render_scale"])
	}
	d.Config.SetVolume(true, 37)
	if e = d.Save(); e != nil {
		t.Fatal(e)
	}
	paths, e := LoadApplication(filepath.Dir(p), "")
	if e != nil {
		t.Fatal(e)
	}
	if paths.Config.Runtime.PresentationFPS != 30 || paths.Config.Runtime.PresentationMode != "stepped" || paths.Config.WindowWidth != 1111 || paths.Config.WindowHeight != 777 {
		t.Fatal("unrelated Save changed startup/presentation", paths.Config)
	}
}

func TestInitialGameWindowDimensions(t *testing.T) {
	for _, tc := range []struct {
		settings      string
		width, height int
	}{
		{`{}`, 1024, 768},
		{`{"window_width":960,"window_height":720}`, 1024, 768},
		{`{"window_width":640,"window_height":480}`, 640, 480},
		{`{"window_width":800,"window_height":600}`, 800, 600},
		{`{"window_width":1280,"window_height":1024}`, 1280, 1024},
		{`{"window_width":1111,"window_height":777}`, 1111, 777},
	} {
		doc, err := decodeSettings(filepath.Join(t.TempDir(), Filename), []byte(tc.settings))
		if err != nil {
			t.Fatal(err)
		}
		if doc.Config.WindowWidth != tc.width || doc.Config.WindowHeight != tc.height {
			t.Errorf("%s gave %dx%d", tc.settings, doc.Config.WindowWidth, doc.Config.WindowHeight)
		}
	}
}
