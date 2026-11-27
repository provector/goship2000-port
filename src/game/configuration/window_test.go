package configuration

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestWindowModeCompatibilityAndShortcutIndependence(t *testing.T) {
	if !slices.Equal(WindowChoices(), []string{"640x480", "800x600", "1024x768", "1280x1024", "Borderless F", "FullScreen"}) {
		t.Fatal(WindowChoices())
	}
	for _, oldFull := range []bool{false, true} {
		t.Run(fmt.Sprint(oldFull), func(t *testing.T) {
			for _, shortcut := range []bool{true, false} {
				path := filepath.Join(t.TempDir(), "config.cfg")
				if err := os.WriteFile(path, []byte(fmt.Sprintf(`{"fullscreen":%t,"window_width":811,"window_height":577,"music_volume":37,"alt_enter_borderless":%t}`, oldFull, shortcut)), 0600); err != nil {
					t.Fatal(err)
				}
				d, err := Load(path)
				if err != nil {
					t.Fatal(err)
				}
				want := Windowed
				if oldFull {
					want = FullScreen
				}
				if d.Config.WindowMode() != want {
					t.Fatal(d.Config.WindowMode())
				}

				for _, choice := range []string{BorderlessLabel, FullScreenLabel, "800x600"} {
					w, h := d.Config.WindowWidth, d.Config.WindowHeight
					if choice == "800x600" {
						w, h = 800, 600
					}
					if err = d.Config.SetWindowChoice(choice); err != nil {
						t.Fatal(err)
					}
					if err = d.Save(); err != nil {
						t.Fatal(err)
					}
					d, err = Load(path)
					if err != nil {
						t.Fatal(err)
					}
					raw, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					var retained map[string]json.RawMessage
					if err = json.Unmarshal(raw, &retained); err != nil || string(retained["alt_enter_borderless"]) != fmt.Sprint(shortcut) {
						t.Fatal("old key lost", err)
					}
					c := d.Config
					if c.WindowMode().Label(c.WindowWidth, c.WindowHeight) != choice || c.WindowWidth != w || c.WindowHeight != h || c.MusicVolume != 37 {
						t.Fatalf("window=%v size=%dx%d shortcut=%v music=%d", c.WindowMode(), c.WindowWidth, c.WindowHeight, shortcut, c.MusicVolume)
					}
				}
			}
		})
	}
}
