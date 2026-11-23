package navigation_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/game/navigation"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// This is the actual source-profiled GPS compositor on a supplied viewing state,
// not a new mission/actor or inferred screenshot seed. The screenshot's static
// coast/tiles determine a pixel-equivalent Center interval, not an exact pose.
func TestPHIL1GPSReportedLocationBeforeAfter(t *testing.T) {
	root := os.Getenv("GS2000_TEST_R221_ASSETS")
	shot := os.Getenv("GS2000_R221_MAP_SCREENSHOT")
	if root == "" || shot == "" {
		t.Skip("Preparation33 and declared supplied map screenshot required")
	}
	a, err := assetdata.LoadFrontEndAssets(root)
	if err != nil {
		t.Fatal(err)
	}
	w := a.World.Worlds["PHIL1"]
	if w == nil || assetdata.Hash(w.TileAtlas) != assetdata.Phil1CleanAtlasSHA256 {
		t.Fatal("verified clean PHIL1 conversion missing")
	}
	badBytes, err := os.ReadFile(filepath.Join(root, "world/raw/PHIL1BMP.GS2"))
	if err != nil || assetdata.Hash(badBytes) != assetdata.Phil1BadAtlasSHA256 {
		t.Fatal("immutable raw bad atlas missing", err)
	}
	bad := *w
	bad.TileAtlas = badBytes // Diagnostic viewing copy only, no prepared file changes.
	rawScreenshot, err := os.ReadFile(shot)
	if err != nil || assetdata.Hash(rawScreenshot) != "6b54027c859dd0521f2ae7f862d5ec591e5e292a044884a5d3f23c1d3fe4e1a5" {
		t.Fatal("declared supplied screenshot identity", err)
	}
	reference, err := png.Decode(bytes.NewReader(rawScreenshot))
	if err != nil || reference.Bounds().Dx() != 1055 || reference.Bounds().Dy() != 852 {
		t.Fatal("declared screenshot size", err)
	}
	palette := a.World.Palettes["PHILDARK"].Colours()
	out := os.Getenv("GS2000_R221_OUTPUT")
	receipts := []map[string]any{}
	for _, tc := range []struct {
		name   string
		center navigation.Point
		report bool
	}{
		{"reported-coast", navigation.Point{H0: 292176, H1: 267604}, true},
		{"unaffected-northwest-water", navigation.Point{H0: 40960, H1: 475136}, false},
	} {
		var routes [5]navigation.Route
		g := navigation.New(&routes, [5]navigation.Point{}, [5]bool{})
		g.Low, g.Center = true, tc.center
		before, after := make([]byte, 64000), make([]byte, 64000)
		if err := g.Draw(before, &bad, a.Cockpit); err != nil {
			t.Fatal(err)
		}
		if err := g.Draw(after, w, a.Cockpit); err != nil {
			t.Fatal(err)
		}
		changed, outside, matches := 0, 0, 0
		for y := 0; y < 200; y++ {
			for x := 0; x < 320; x++ {
				at := y*320 + x
				inside := x >= 2 && x <= 226 && y >= 3 && y <= 163
				if before[at] != after[at] {
					changed++
					if !inside {
						outside++
					}
				}
				if tc.report && inside {
					// Empirical title/client viewport: x0..1055, y≈46..837.
					// Sampling pixel centers avoids host nearest-scaling edges.
					rx := int((float64(x) + .5) * 1055 / 320)
					ry := 46 + int((float64(y)+.5)*791.25/200)
					got := color.RGBAModel.Convert(reference.At(rx, ry))
					want := color.RGBAModel.Convert(palette[int(before[at])])
					if got == want {
						matches++
					}
				}
			}
		}
		if outside != 0 || tc.report && (matches < 35500 || changed < 5000) || !tc.report && !bytes.Equal(before, after) {
			t.Fatal("reported-site correction/unchanged control", tc.name, matches, changed, outside)
		}
		receipt := map[string]any{"name": tc.name, "center": tc.center, "different_pixels": changed, "different_pixels_outside_map_clip": outside}
		if tc.report {
			receipt["screenshot_static_map_matching_pixels"] = matches
			receipt["screenshot_static_map_total_pixels"] = 225 * 161
			receipt["source_center_pixel_equivalence_interval"] = map[string][2]int{"H0": {292176, 292516}, "H1": {267604, 268058}}
			receipt["mosaic_origin"] = [2]int{743, 480}
		}
		receipts = append(receipts, receipt)
		if out != "" {
			if err := os.MkdirAll(out, 0700); err != nil {
				t.Fatal(err)
			}
			for _, frame := range []struct {
				name   string
				pixels []byte
			}{{"before", before}, {"after", after}} {
				img := image.NewPaletted(image.Rect(0, 0, 320, 200), palette)
				copy(img.Pix, frame.pixels)
				p := filepath.Join(out, "phil1-"+tc.name+"-"+frame.name+".png")
				file, err := os.Create(p)
				if err != nil {
					t.Fatal(err)
				}
				err = png.Encode(file, img)
				_ = file.Close()
				if err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	if out != "" {
		raw, err := os.ReadFile(shot)
		if err != nil {
			t.Fatal(err)
		}
		h := sha256.Sum256(raw)
		b, _ := json.MarshalIndent(map[string]any{"screenshot_sha256": hex.EncodeToString(h[:]), "palette": "PHILDARK", "bad_atlas_sha256": assetdata.Phil1BadAtlasSHA256, "clean_atlas_sha256": assetdata.Phil1CleanAtlasSHA256, "cases": receipts, "class": "Actual native navigation.GPS.Draw; supplied viewing state localized from static screenshot coast/tiles, not recovered mission seed/film or OS input. No prepared input or original modified."}, "", "  ")
		if err = os.WriteFile(filepath.Join(out, "phil1-map-location.json"), append(b, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("actual native GPS reported location/control: %+v", receipts)
}
