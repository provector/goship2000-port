package assets

import (
	"bytes"
	"gunship2000-assets/internal/testfixture"
	"os"
	"testing"
)

func TestPreparedRotorDataUsesVerifiedInitializedCarrier(t *testing.T) {
	root := os.Getenv("GS2000_TEST_PREPARED")
	if root == "" {
		t.Skip("explicit disposable prepared tree")
	}
	a, err := LoadAudioAssets(root)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(testfixture.Path("installation/ASOUND.GS2"))
	if os.IsNotExist(err) {
		t.Skip("optional original audio source")
	}
	if err != nil {
		t.Fatal(err)
	}
	known := IslandIce469085.Source(RoleAudioADigital)
	if Hash(raw) != known.SHA256 {
		t.Fatal("test source does not match centralized profile")
	}
	offset := IslandIce469085.AudioLayout.Drivers["GS2"].DSFile
	if !bytes.Equal(a.ASoundGS2Data, raw[offset:]) {
		t.Fatal("runtime rotor data differs from separately read original initialized DS slice")
	}
	if len(a.ASoundGS2Data) == 0 || len(a.ASoundGS2Data) > 65536 {
		t.Fatal("bounded initialized-data carrier domain")
	}
}
