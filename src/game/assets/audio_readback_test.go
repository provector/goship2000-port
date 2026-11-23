package assets

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCurrentAndFrozen30DefenseAudioReadback(t *testing.T) {
	for _, tc := range []struct {
		variable string
		current  bool
	}{{"GS2000_TEST_MISSIONS", true}, {"GS2000_P7_OLD30", false}} {
		root := os.Getenv(tc.variable)
		if root == "" {
			t.Fatalf("required declared audio fixture %s is unavailable", tc.variable)
		}
		before := map[string]string{}
		for _, name := range []string{"manifest.json", "audio/audio.json"} {
			b, err := os.ReadFile(filepath.Join(root, name))
			if err != nil {
				t.Fatal(err)
			}
			before[name] = Hash(b)
		}
		a, err := LoadAudioAssets(root)
		if err != nil {
			t.Fatal(tc.variable, err)
		}
		if a.SupportsDefenseReleaseAudio() != tc.current {
			t.Fatal("historical/current capability conflated", tc.variable)
		}
		for _, id := range []uint16{0x0a, 0x2a} {
			_, found := a.GS2Commands[id]
			if found != tc.current {
				t.Fatalf("%s exact family widened %02x", tc.variable, id)
			}
		}
		for name, want := range before {
			b, err := os.ReadFile(filepath.Join(root, name))
			if err != nil || Hash(b) != want {
				t.Fatal("readback rewrote frozen/current assets", name, err)
			}
		}
	}
}
