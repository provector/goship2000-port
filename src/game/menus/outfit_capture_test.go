package menus

import (
	"encoding/json"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func captureOutfitObjectiveRepair(t *testing.T, v *OutfitView, frame []byte) {
	t.Helper()
	root := os.Getenv("GS2000_L3_OUTFIT_CAPTURE")
	if root == "" {
		return
	}
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	write := func(name string, raw []byte) {
		t.Helper()
		f, err := os.OpenFile(filepath.Join(root, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.Write(raw); err != nil {
			t.Fatal(err)
		}
		if err = f.Close(); err != nil {
			t.Fatal(err)
		}
	}
	before := append([]byte(nil), frame...)
	for y := 99; y < 124; y++ {
		clear(before[y*320+217 : y*320+317])
	}
	y := 99
	for _, role := range []string{"primary", "secondary"} {
		if o, ok := v.Mission.Objectives[role]; ok {
			for _, line := range ordnanceWrap(v.Assets.Fonts[3], "\x8f"+strings.ToUpper(role)+": \x8a"+o.Name, 100) {
				preflightText(v.Assets, before, 3, line, 217, y, 10, [4]int{217, 99, 100, 25})
				y += 6
			}
		}
	}
	for name, raw := range map[string][]byte{"before-name-only-control": before, "after-source-producer": frame} {
		write(name+".indices", raw)
		f, err := os.OpenFile(filepath.Join(root, name+".png"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			t.Fatal(err)
		}
		img := image.NewPaletted(image.Rect(0, 0, 320, 200), v.Assets.Palettes["FIELD.PAL"])
		copy(img.Pix, raw)
		if err = png.Encode(f, img); err != nil {
			t.Fatal(err)
		}
		if err = f.Close(); err != nil {
			t.Fatal(err)
		}
	}
	b, err := json.MarshalIndent(struct {
		Meaning    string
		Objectives any
		SourceText any
		Lines      []string
	}{
		"Actual generated preflight fixture; before is the previous name-only producer reconstructed in the same panel, not original-game output", v.Mission.Objectives, v.Assets.Preflight.ObjectiveSummary, v.objectiveLines()}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	write("inputs.json", b)
}
