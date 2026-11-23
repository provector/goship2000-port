package cockpit

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestTargetDossierSourceBoundsAndScale(t *testing.T) {
	a := hudTestAssets()
	a.ScenePresentation.ConfirmationRemap = make([]byte, 256)
	for i := range a.ScenePresentation.ConfirmationRemap {
		a.ScenePresentation.ConfirmationRemap[i] = byte(i & 7)
	}
	for _, scale := range []int{1, 2} {
		p := bytes.Repeat([]byte{99}, 64000*scale*scale)
		if err := DrawTargetDossierScaled(p, a, scale, 160, "Fuel Tank", "No threat.", "None", "None"); err != nil {
			t.Fatal(err)
		}
		for y := 0; y < 200*scale; y++ {
			for x := 0; x < 320*scale; x++ {
				if (x < 50*scale || x >= 271*scale || y < 16*scale || y >= 89*scale) && p[y*320*scale+x] != 99 {
					t.Fatal("dossier escaped source rectangle", scale, x, y)
				}
			}
		}
		if p[16*scale*320*scale+50*scale] != 203 {
			t.Fatal("source background", scale)
		}
		if bytes.Count(p, []byte{10}) == 0 || bytes.Count(p, []byte{2}) == 0 {
			t.Fatal("source labels/values absent", scale)
		}
	}
}

func TestDossierWrapSourceThresholdAndContinuation(t *testing.T) {
	a := hudTestAssets()
	for i := range a.Font.Advances {
		a.Font.Advances[i] = 2
	}
	a.Font.Advances[' '-32] = 1
	cases := []struct {
		text           string
		x, left, right int
		want           string
	}{
		{"AA B", 3, 3, 11, "AA B"},  // ending X10<11 does not break
		{"AA B", 3, 3, 10, "AA\nB"}, // equality X10 breaks, not >right+1
		{"AA BB", 3, 3, 10, "AA\nBB"},
		{"A BB CC", 5, 1, 11, "A\nBB CC"}, // continuation uses left1, not initial5
		{"AA\nBB", 3, 3, 20, "AA BB"},     // original oldLF is a space before rewrapping
		{"A  B", 3, 3, 20, "A  B"},        // preserve source spaces, not Fields normalization
	}
	for _, tc := range cases {
		got, e := dossierWrappedText(a, tc.text, tc.x, Rect{tc.left, 0, tc.right, 88})
		if e != nil || got != tc.want {
			t.Fatal(tc, got, e)
		}
	}
	if _, e := dossierWrappedText(a, "AAAA", 3, Rect{3, 0, 11, 88}); e == nil {
		t.Fatal("unchecked original no-space rewind was not bounded")
	}
}

func TestDossierFuelBaseHangarRemapAndInset(t *testing.T) {
	a := hudTestAssets()
	a.ScenePresentation.ConfirmationRemap = make([]byte, 256)
	for i := range a.ScenePresentation.ConfirmationRemap {
		a.ScenePresentation.ConfirmationRemap[i] = byte(i & 15)
	}
	// Exact section5 text; formatter, not a rewritten description, controls wrap.
	cases := [][4]string{
		{"Fuel Tank", "Fuel Tank found at friendly helicopter bases and FARPs. No threat.", "None", "None"},
		{"Base", "Friendly helicopter base. No threat.", "None", "None"},
		{"Hangar", "Light metal structure that may be armed with machine-guns or (1.5 km) IR guided missiles of varying quality (4.5-7.0 km). A potential threat.", "M261s, Cannon, TOW-2 and both Hellfires", "M255s, Minipod, Sidearm, Stinger, Sidewinder and Penguin"},
	}
	for _, scale := range []int{1, 2} {
		for _, v := range cases {
			before := make([]byte, 64000*scale*scale)
			for i := range before {
				before[i] = byte(i*13 + 7)
			}
			p := append([]byte(nil), before...)
			if e := DrawTargetDossierScaled(p, a, scale, 160, v[0], v[1], v[2], v[3]); e != nil {
				t.Fatal(v[0], scale, e)
			}
			width := 320 * scale
			for y := 0; y < 200*scale; y++ {
				for x := 0; x < width; x++ {
					at := y*width + x
					if x < 50*scale || x >= 271*scale || y < 16*scale || y >= 89*scale {
						if p[at] != before[at] {
							t.Fatal("unrelated frame changed", v[0], scale, x, y)
						}
					}
					// Border x50..57 is inside the destination remap but outside the source
					// font clip58. No continuation or glyph may paint it with2/10/0.
					if x >= 50*scale && x < 58*scale && y >= 16*scale && y < 89*scale {
						want := a.ScenePresentation.ConfirmationRemap[before[at]] + 0xc8
						if p[at] != want {
							t.Fatal("wrong continuation inset/remap", v[0], scale, x, y, p[at], want)
						}
					}
				}
			}
			// Independent real-subpixel lookup: different initial indices stay different
			// C8+brightness values at the untouched top row, at either target scale.
			for x := 50 * scale; x < 271*scale; x++ {
				at := 16*scale*width + x
				want := byte(0xc8 + (before[at] & 15))
				if p[at] != want {
					t.Fatal("opaque fill or wrong subpixel remap", v[0], scale, x)
				}
			}
		}
	}
}

func TestDossierPreparedFontFuelFARPAndHangarWrap(t *testing.T) {
	root := os.Getenv("GS2000_TEST_MISSIONS")
	if root == "" {
		t.Skip("set GS2000_TEST_MISSIONS to the retained prepared tree")
	}
	raw, e := os.ReadFile(filepath.Join(root, "world/cockpit/metadata.json"))
	if e != nil {
		t.Fatal(e)
	}
	var doc struct{ Data Assets }
	if e = json.Unmarshal(raw, &doc); e != nil {
		t.Fatal(e)
	}
	a := &doc.Data
	if a.Font.First != 32 || a.Font.Height != 6 || len(a.Font.Advances) != 95 {
		t.Fatal("actual prepared source font required")
	}
	cases := []struct {
		text string
		x    int
		want string
	}{
		{"Fuel Tank found at friendly helicopter bases and FARPs. No threat.", 58, "Fuel Tank found at friendly helicopter bases and FARPs.\nNo threat."},
		{"Friendly helicopter base. No threat.", 58, "Friendly helicopter base. No threat."},
		{"Light metal structure that may be armed with machine-guns or (1.5 km) IR guided missiles of varying quality (4.5-7.0 km). A potential threat.", 58, "Light metal structure that may be armed with\nmachine-guns or (1.5 km) IR guided missiles of varying\nquality (4.5-7.0 km). A potential threat."},
		{"M261s, Cannon, TOW-2 and both Hellfires", 122, "M261s, Cannon, TOW-2 and both\nHellfires"},
		{"M255s, Minipod, Sidearm, Stinger, Sidewinder and Penguin", 122, "M255s, Minipod, Sidearm, Stinger,\nSidewinder and Penguin"},
	}
	for _, tc := range cases {
		got, e := dossierWrappedText(a, tc.text, tc.x, Rect{58, 16, 270, 88})
		if e != nil || got != tc.want {
			t.Fatal(tc.text, got, tc.want, e)
		}
	}
}
