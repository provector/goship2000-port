package menus

import (
	assetdata "gunship2000-assets/game/assets"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestOfficeViewportAndMouseMapping(t *testing.T) {
	cases := []struct {
		w, h   int
		square bool
		want   Viewport
	}{
		{960, 720, false, Viewport{0, 0, 960, 720}}, {1280, 720, false, Viewport{160, 0, 960, 720}},
		{960, 720, true, Viewport{0, 60, 960, 600}}, {800, 800, false, Viewport{0, 100, 800, 600}},
	}
	for _, c := range cases {
		v := officeViewport(c.w, c.h, c.square)
		if v != c.want {
			t.Fatalf("viewport %+v want%+v", v, c.want)
		}
		x, y, ok := v.LogicalPoint(v.X+v.Width/2, v.Y+v.Height/2)
		if !ok || x != 160 || y != 100 {
			t.Fatal(x, y, ok)
		}
		if _, _, ok = v.LogicalPoint(v.X+v.Width, v.Y); ok {
			t.Fatal("right edge must be exclusive")
		}
		if _, _, ok = v.LogicalPoint(v.X, v.Y-0.1); ok {
			t.Fatal("letterbox mapped into scene")
		}
		x, y, ok = v.LogicalPoint(v.X+v.Width-0.01, v.Y+v.Height-0.01)
		if !ok || x != 319 || y != 199 {
			t.Fatal("last pixel mapping", x, y, ok)
		}
	}
}

func TestOfficeTickCatchUpAndRenderIndependence(t *testing.T) {
	a := tickAccumulator{Rate: 70}
	b := tickAccumulator{Rate: 70}
	na, nb := 0, 0
	for i := 0; i < 60; i++ {
		na += a.Advance(1.0/60, false)
	}
	for i := 0; i < 144; i++ {
		nb += b.Advance(1.0/144, false)
	}
	if na != 70 || nb != 70 {
		t.Fatal(na, nb)
	}
	if a.Advance(30, false) != 8 {
		t.Fatal("long pause wasn't bounded")
	}
	if a.Advance(math.NaN(), false) != 0 {
		t.Fatal("bad elapsed accepted")
	}
	a.Advance(.01, false)
	a.Advance(.1, true)
	if a.Advance(.005, false) != 0 {
		t.Fatal("paused elapsed leaked")
	}
}

func TestGeneratedFramesCannotReplaceInputs(t *testing.T) {
	root := t.TempDir()
	assets := filepath.Join(root, "assets")
	if err := os.Mkdir(assets, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assets, "source"), []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := assetdata.GeneratedPath(filepath.Join(assets, "new.png"), assets); err == nil {
		t.Fatal("output entered prepared inputs")
	}
	if _, err := assetdata.GeneratedPath(filepath.Join(assets, "source"), assets); err == nil {
		t.Fatal("output overwrote source")
	}
	target := filepath.Join(root, "new", "frame.png")
	if got, err := assetdata.GeneratedPath(target, assets); err != nil || got != target {
		t.Fatal(got, err)
	}
}

func TestGeneratedFramesProtectProjectWithExternalAssets(t *testing.T) {
	project := t.TempDir()
	external := t.TempDir()
	if err := os.MkdirAll(filepath.Join(project, "src", "game"), 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"src/go.mod", "src/game/main.go"} {
		if err := os.WriteFile(filepath.Join(project, name), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	output := filepath.Join(project, "test-data", "capture")
	if _, err := assetdata.GeneratedPath(output, external); err == nil {
		t.Fatal("public source fixture admitted for capture")
	}
	if _, _, err := assetdata.ValidatePaths(external, output); err == nil {
		t.Fatal("public source fixture admitted for preparation")
	}
	if err := os.WriteFile(filepath.Join(project, "AGENTS.md"), []byte("project marker"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"original", "fixtures", "evidence", "working/step21"} {
		if _, err := assetdata.GeneratedPath(filepath.Join(project, filepath.FromSlash(name), "new.png"), external); err == nil {
			t.Fatalf("external assets bypassed %s protection", name)
		}
	}
}
