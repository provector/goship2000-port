package assets

import (
	"bytes"
	"gunship2000-assets/internal/testfixture"
	"os"
	"testing"
)

func TestGPSMissionAssetsCaptionSourceSlices(t *testing.T) {
	b, err := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if err != nil {
		t.Fatal(err)
	}
	if Hash(b) != "1a1db96070adb8c23e1eb499b449cc147f9fb0fd7b47a65e88c4c5603a40e209" {
		t.Fatal("wrong original GS2")
	}
	for _, tc := range []struct {
		name       string
		ip, length int
		literal    string
	}{
		{"gps-support-aircraft", 0x2222, 11, "- Aircraft"},
		{"gps-support-artillery", 0x222d, 16, "- Artillery : 3"},
	} {
		if !bytes.Equal(b[0x24290+tc.ip:][:tc.length], append([]byte(tc.literal), 0)) {
			t.Fatal("source caption", tc.name)
		}
		found := false
		for _, s := range IslandIce469085.CockpitLayout.Texts {
			if s.Name == tc.name {
				found = true
				if s.Section != 4 || s.CS != 0x975 || s.IP != tc.ip || s.Length != tc.length {
					t.Fatal("typed locator", s)
				}
			}
		}
		if !found {
			t.Fatal("new source caption absent from preparer", tc.name)
		}
	}
	// Source entry C9..DA patches only the final artillery digit with AA4>>4.
	if !bytes.Equal(b[0x24290+0xd1:][:11], []byte{0x26, 0xa0, 0xa4, 0x0a, 0xc0, 0xe8, 0x04, 0x04, 0x30, 0x2e, 0xa2}) || b[0x24290+0x223b] != '3' {
		t.Fatal("source count writer/digit")
	}
}
