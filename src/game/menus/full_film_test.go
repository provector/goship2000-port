package menus

import (
	"os"
	"reflect"
	"testing"

	"gunship2000-assets/game/assets"
	"gunship2000-assets/game/configuration"
	"gunship2000-assets/game/replay"
)

// Explicit opt-in private accepted film; no profile/data is embedded in release.
func TestAcceptedO1FilmCompleteCurrentViewer(t *testing.T) {
	path := os.Getenv("GS2000_O2_FULL_FILM")
	if path == "" {
		t.Skip("explicit preserved accepted O.1 film required")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	film, err := replay.Unmarshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	a, err := assets.LoadFrontEndAssets(os.Getenv("GS2000_TEST_MISSIONS"))
	if err != nil {
		t.Fatal(err)
	}
	var ending any
	for _, scale := range []int{1, 2} {
		cfg := configuration.Defaults()
		cfg.SimulationRenderScale = scale
		cfg.PresentationFPS = 30
		cfg.PresentationMode = "stepped"
		if scale == 2 {
			cfg.PresentationFPS = 60
			cfg.PresentationMode = "smooth"
		}
		q, err := NewFilmPlayback(a, film, cfg)
		if err != nil {
			t.Fatal(err)
		}
		c := &FrontEnd{Office: &Office{Assets: a}, Menus: a.Menus, Runtime: cfg, Viewer: NewReplayViewer(q), Screen: ScreenReplay}
		for !q.Done {
			if err := q.Step(); err != nil {
				t.Fatal(err)
			}
			if q.Cursor%1000 == 0 && !q.Done {
				before := q.Preview().FilmCheckpoint()
				c.Viewer.Demo = !c.Viewer.Demo
				c.Viewer.Map = !c.Viewer.Map
				c.Viewer.yaw += 0x100
				c.Frame(false)
				if !reflect.DeepEqual(before, q.Preview().FilmCheckpoint()) {
					t.Fatal("viewer mutated accepted-film state")
				}
			}
		}
		reached := q.Preview().FilmCheckpoint()
		if ending == nil {
			ending = reached
		} else if !reflect.DeepEqual(ending, reached) {
			t.Fatal("presentation changed complete ending")
		}
		t.Logf("scale%d: every operation/checkpoint/event/end matched (%d operations)", scale, q.Cursor)
	}
	after, err := os.ReadFile(path)
	if err != nil || string(raw) != string(after) {
		t.Fatal("accepted film changed", err)
	}
}
