package menus

import (
	"encoding/json"

	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBottomStripOfficeCampaignOtherScreenAndLongStatus(t *testing.T) {
	c := renderingController(t)
	found := false
	for i, m := range c.Office.Assets.Metadata.Missions {
		if strings.EqualFold(m.Text, "Campaign") {
			c.Office.State.MissionType = i
			found = true
			break
		}
	}
	if !found {
		t.Fatal("source office Campaign label unavailable")
	}
	c.Office.State.Active = true
	c.Office.HoverKey = 3
	caption, font, _ := c.Office.Caption()
	if !strings.Contains(caption, "CAMPAIGN") {
		t.Fatal("office witness differs", caption)
	}
	frames := map[string][]byte{"office-campaign-1x": c.Office.Frame(false)}
	// Independent bitmap-font oracle: draw the existing glyphs at the required
	// logical center. This also covers the separately found Film Library caller.
	f := c.Menus.Fonts[font]
	width := f.Measure([]byte(caption))
	ref := make([]byte, 64000)
	f.DrawClipped(ref, []byte(caption), (320-width)/2, 193, 7, 0, 0, [4]int{2, 191, 316, 9})
	for i := 191 * 320; i < 64000; i++ {
		if frames["office-campaign-1x"][i] != ref[i] {
			t.Fatal("office strip not centered", i)
		}
	}
	c.Library = &FilmLibrary{Selected: -1, Status: "Film unavailable: copied diagnostic"}
	film := c.filmLibraryFrame()
	// Q3.2: FilmLibrary.Status is a native diagnostic, no longer a strip owner.
	for i := 191 * 320; i < 64000; i++ {
		if film[i] != 0 {
			t.Fatal("native library diagnostic entered source strip", i)
		}
	}
	frames["film-library-status-1x"] = film
	c.Screen = ScreenRoster
	frames["roster-1x"] = c.Frame(false)
	for _, text := range []string{"HELICOPTER HAS NO FUEL.  Add fuel.", strings.Repeat(originalRosterPrompt, 18)} {
		low := make([]byte, 64000)
		high := make([]byte, 256000)
		c.drawOriginalStrip(low, text, 7)
		c.drawOriginalStrip(high, text, 7)
		for y := 191; y < 200; y++ {
			for x := 0; x < 320; x++ {
				v := low[y*320+x]
				if v != 0 && (x < 2 || x >= 318) {
					t.Fatal("clip", x)
				}
				at := 2*y*640 + 2*x
				if high[at] != v || high[at+1] != v || high[at+640] != v || high[at+641] != v {
					t.Fatal("presentation changed alignment")
				}
			}
		}
		key := "status-1x"
		if len(text) > 100 {
			key = "long-error-1x"
		}
		frames[key] = low
		frames[strings.TrimSuffix(key, "1x")+"2x"] = high
	}
	// The office frame's UI strip uses the same logical pixels before2x scaling.
	office2 := make([]byte, 256000)
	for y := 0; y < 200; y++ {
		for x := 0; x < 320; x++ {
			v := frames["office-campaign-1x"][y*320+x]
			at := 2*y*640 + 2*x
			office2[at] = v
			office2[at+1] = v
			office2[at+640] = v
			office2[at+641] = v
		}
	}
	frames["office-campaign-2x"] = office2
	if out := os.Getenv("GS2000_Q21_CAPTURE_DIR"); out != "" {
		os.MkdirAll(out, 0700)
		for name, frame := range frames {
			img, e := indexedFrameRGBA(frame, c.Office.Assets.Palette)
			if e != nil {
				t.Fatal(e)
			}
			f, e := os.Create(filepath.Join(out, name+".png"))
			if e != nil {
				t.Fatal(e)
			}
			e = png.Encode(f, img)
			f.Close()
			if e != nil {
				t.Fatal(e)
			}
		}
		b, _ := json.MarshalIndent(map[string]any{"caption": caption, "font": font, "width": width, "origin_x": (320 - width) / 2, "vertical_y": 193, "scale2": "exact doubling of logical bottom strip; no new glyph rasterization"}, "", "  ")
		os.WriteFile(filepath.Join(out, "receipt.json"), b, 0600)
	}
}
