package menus

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
)

func TestPresentationPoseCadence(t *testing.T) {
	counts := map[string]int{}
	for _, mode := range []string{"stepped", "smooth"} {
		c := mastFixture(t, 0, "pass")
		jFinishFade(t, c)
		p := c.Rendering
		p.presentation.mode = mode
		p.resetPresentation()
		seen := map[string]bool{}
		for i := 0; i < 60; i++ {
			if err := c.AdvanceRendering(1.0 / 60); err != nil {
				t.Fatal(err)
			}
			scene, _ := p.presentationInputs()
			seen[fmt.Sprint(scene.Camera)] = true
		}
		counts[mode] = len(seen)
	}
	// Native presentation requirement, independent of the flight equations:
	// reference poses change at logical updates; interpolation fills their gaps.
	if counts["stepped"] > 12 || counts["smooth"] < 2*counts["stepped"] {
		t.Fatal(counts)
	}
	t.Logf("one-second60FPS distinct camera poses: %v", counts)
	if path := os.Getenv("GS2000_K_POSE_MEASUREMENT"); path != "" {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err = json.NewEncoder(f).Encode(counts); err != nil {
			t.Fatal(err)
		}
	}
}
