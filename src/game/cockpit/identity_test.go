package cockpit

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"gunship2000-assets/internal/testfixture"
	"os"
	"path/filepath"
	"testing"
)

func TestPreflightAircraftCockpitFamilies(t *testing.T) {
	// Aircraft identities are independently given by the original SHHD labels:
	// Kiowa4, Blackhawk5, Comanche Gunship6 and Comanche Scout7.
	want := [8]string{"APA", "APA", "COB", "DEF", "KIA", "BLK", "COM", "COM"}
	for aircraft, family := range want {
		got, err := Family(aircraft)
		if err != nil || got != family {
			t.Fatalf("preflight aircraft%d: %q, %v; want%s", aircraft, got, err, family)
		}
	}
	for _, aircraft := range []int{-1, 8, 0x24} {
		if _, err := Family(aircraft); err == nil {
			t.Fatalf("packed/invalid preflight ID%d accepted", aircraft)
		}
	}
}

func TestOriginalAircraftHandoffAndCockpitSelectorTables(t *testing.T) {
	read := func(name, expected string) []byte {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(testfixture.Path("installation"), name))
		if os.IsNotExist(err) {
			t.Skip("optional original source is unavailable")
		}
		if err != nil {
			t.Fatal(err)
		}
		if fmt.Sprintf("%x", sha256.Sum256(b)) != expected {
			t.Fatal("original source profile mismatch:", name)
		}
		return b
	}
	gs := read("GS.GS2", "46ce1a52b67e50ebee7d5aacafa063021bfd7e39303b5a67da61a5cd5aed90e9")
	gs2 := read("GS2.GS2", "1a1db96070adb8c23e1eb499b449cc147f9fb0fd7b47a65e88c4c5603a40e209")
	// GS section2 original1c89 has base32e70. At02fe the writer reads
	// DS3d0c[preflightID] and replaces only the destination SHIP low three bits.
	writer := []byte{0x8a, 0x87, 0x0c, 0x3d, 0x32, 0x46, 0xb8, 0x24, 0x07, 0x30, 0x46, 0xb8}
	if !bytes.Equal(gs[0x32e70+0x2fe:0x32e70+0x30a], writer) {
		t.Fatal("handoff selector producer operands changed")
	}
	remap := gs[0x3d2f0+0x3d0c : 0x3d2f0+0x3d14]
	if !bytes.Equal(remap, []byte{0, 1, 2, 3, 5, 6, 4, 4}) {
		t.Fatal("source preflight-to-SHIP selector table changed")
	}
	// GS2 section0 original0975 has basea120. Its loader consumes SHIP0&7
	// and uses the *different* CS07cd near-pointer table for PIC/DAT/STAT names.
	if !bytes.Equal(gs2[0xa120+0x702:0xa120+0x710], []byte{0x8e, 0x06, 0xb4, 0x32, 0x26, 0x8a, 0x1e, 0x00, 0x00, 0x83, 0xe3, 0x07, 0xd1, 0xe3}) {
		t.Fatal("source cockpit selector consumer operands changed")
	}
	for aircraft, selector := range remap {
		ptr := int(binary.LittleEndian.Uint16(gs2[0xa120+0x7cd+2*int(selector):]))
		name := gs2[0xa120+ptr:]
		end := bytes.IndexByte(name, 0)
		if end < 0 {
			t.Fatal("unterminated source cockpit prefix")
		}
		got, err := Family(aircraft)
		if err != nil || got != string(bytes.ToUpper(name[:end])) {
			t.Fatalf("source-composed preflight%d selector%d prefix%q !=%q (%v)", aircraft, selector, name[:end], got, err)
		}
	}
}
