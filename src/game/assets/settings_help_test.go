package assets

import (
	"encoding/binary"
	"gunship2000-assets/internal/testfixture"
	"os"
	"strings"
	"testing"
)

func TestSettingsDetailedDescriptionsFromOriginalBytes(t *testing.T) {
	b, err := os.ReadFile(testfixture.Path("installation/GS.GS2"))
	if err != nil {
		t.Skip("optional original source unavailable")
	}
	if Hash(b) != "46ce1a52b67e50ebee7d5aacafa063021bfd7e39303b5a67da61a5cd5aed90e9" {
		t.Fatal("unrecognized source identity")
	}
	help, err := prepareSettingsHelp(b, &IslandIce469085)
	if err != nil {
		t.Fatal(err)
	}
	// Fixed DS/file relationships and the literal witnesses are independently
	// read here, without reusing production source-string/pointer helpers.
	counts := []int{3, 4, 2, 2, 2, 2, 2}
	total := 0
	for key, count := range counts {
		if len(help[key].Details) != count {
			t.Fatal("detailed source slots", key)
		}
		for value := 0; value < count; value++ {
			at := 0x3d2f0 + 0x4c3a + 8*key + 2*value
			ptr := int(binary.LittleEndian.Uint16(b[at:]))
			tail := b[0x3d2f0+ptr:]
			end := 0
			for end < len(tail) && tail[end] != 0 {
				end++
			}
			if help[key].Details[value] != string(tail[:end]) {
				t.Fatalf("detail key%d value%d changed source bytes", key, value)
			}
			total++
		}
	}
	if total != 17 || help[1].Details[0] != "POOR:  The bottom of the barrel. These troops are not much better than untrained civilians. Not much of a threat." || help[4].Details[1] != "OFF:  You're on your own; keep alert and stay healthy." {
		t.Fatal("independent literal witnesses")
	}
	for value, want := range []string{"SAFETY ON", "SAFETY OFF", "POWER ON"} {
		ptr := int(binary.LittleEndian.Uint16(b[0x3d2f0+0x4c32+2*value:]))
		tail := b[0x3d2f0+ptr:]
		end := 0
		for tail[end] != 0 {
			end++
		}
		if string(tail[:end]) != want || help[8].Choices[value] != want {
			t.Fatal("source APU short state", value)
		}
	}
	if len(help[8].Details) != 0 {
		t.Fatal("APU must not interpret the detailed table beyond supported rows")
	}
	// Instruction-derived lifetime and selected-table operands are bound to
	// section2 original1f13 (file35710+IP), never GS2's unrelated section2.
	for _, v := range []struct {
		ip  int
		hex []byte
	}{
		{0x603, []byte{0xc7, 0x06, 0xb2, 0x9b, 0x01, 0x00}},
		{0x63b, []byte{0x2b, 0xc0, 0xa3, 0xb2, 0x9b, 0xa3, 0xb4, 0x9b, 0xa3, 0xba, 0x9b}},
		{0x856, []byte{0xc7, 0x06, 0xba, 0x9b, 0x01, 0x00}},
		{0x91f, []byte{0xff, 0xb7, 0x3a, 0x4c}},
	} {
		for i, want := range v.hex {
			if b[0x35710+v.ip+i] != want {
				t.Fatalf("source operand at%04x", v.ip+i)
			}
		}
	}
	broken := append([]byte(nil), b...)
	binary.LittleEndian.PutUint16(broken[0x3d2f0+0x4c3a:], 0)
	if _, err = prepareSettingsHelp(broken, &IslandIce469085); err == nil || !strings.Contains(err.Error(), "detailed help missing") {
		t.Fatal("missing consumed detail pointer accepted")
	}
}
