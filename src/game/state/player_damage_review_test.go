package state

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"gunship2000-assets/internal/testfixture"
	"os"
	"testing"
)

func TestPlayerDamageOriginalWholeRoutineAndThresholdBytes(t *testing.T) {
	raw, e := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if os.IsNotExist(e) {
		t.Skip("bound supported DOS executable required")
	}
	if e != nil {
		t.Fatal(e)
	}
	if fmt.Sprintf("%x", sha256.Sum256(raw)) != "1a1db96070adb8c23e1eb499b449cc147f9fb0fd7b47a65e88c4c5603a40e209" {
		t.Fatal("wrong DOS identity")
	}
	retained, e := os.ReadFile(testfixture.Path("damage/damage.bin"))
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(retained, raw[0xd8b0+0x7468:0xd8b0+0x7468+len(retained)]) {
		t.Fatal("retained Ghidra routine differs from original instructions")
	}
	for ip, want := range map[int][]byte{0x761a: {0x26, 0xfe, 0x87, 0xfe, 5}, 0x761f: {0x26, 0x80, 0xbf, 0xfe, 5, 2}, 0x767d: {0x24, 3}, 0x7683: {0x26, 0x80, 0x8f, 0xfe, 5, 0xf0}, 0x7746: {0x26, 0x80, 0x0f, 0xc0}, 0x78c4: {0x26, 0xd1, 0x3e, 0xe3, 0xa}, 0x79bf: {0x26, 0x80, 0xa7, 0x17, 0, 0xfe}} {
		if !bytes.Equal(raw[0xd8b0+ip:0xd8b0+ip+len(want)], want) {
			t.Fatalf("operand%04x", ip)
		}
	}
}
func TestPlayerEveryComponentActualHitAndRepeat(t *testing.T) {
	picks := []uint16{0, 26, 44, 51, 77, 95, 102, 128, 146, 172, 190}
	for bucket, pick := range picks {
		t.Run(fmt.Sprint(bucket), func(t *testing.T) {
			words := []uint16{pick}
			if bucket < 2 {
				words = append(words, 0)
			}
			if bucket == 10 {
				words = append(words, 0, 0)
			}
			m, c := damageFixture(t, 2, words...)
			before := PlayerDamage12(c.Systems)
			got, e := m.ApplyAcceptedPlayerHit(c)
			if e != nil || !got.Applied || got.Limit != "" || got.Bucket != int16(bucket) || got.RNGCalls != uint32(len(words)) {
				t.Fatal(got, e)
			}
			want := before
			want[bucket] = 1
			if bucket < 2 {
				want[bucket] = 0x81
			}
			if bucket == 3 || bucket == 8 {
				want[bucket] = 2
			}
			if after := PlayerDamage12(c.Systems); after != want {
				t.Fatal("complete12-byte mutation", after, want)
			}
			if m.Stores.Damage[0] != c.Systems.Damage {
				t.Fatal("physical store mirror")
			}
			if bucket == 4 && c.Systems.EngineTargets[0] != 7 || bucket == 9 && c.Systems.EngineTargets[1] != 7 {
				t.Fatal("partial engine target")
			}
			if bucket == 5 && (m.Stores.Ships[0].Fuel != 98 || c.Systems.Fuel[0] != 98) {
				t.Fatal("fuel lowbyte ANDFE")
			}
			if bucket == 10 && *c.Overlay != ([2][2]int16{{32, 40}, {}}) {
				t.Fatal("first artwork coordinate", *c.Overlay)
			}
			next := pick
			if bucket == 3 || bucket == 8 {
				next = 102
			}
			at := 0
			stream := []uint16{next}
			if bucket < 2 { /* lowbits reach2; no extra partial draw */
			}
			if bucket == 10 {
				stream = append(stream, 255, 31)
			}
			if bucket == 3 || bucket == 8 {
				stream = []uint16{pick, next}
			}
			c.Random = func() uint16 {
				if at >= len(stream) {
					t.Fatal("extra repeat draw")
				}
				v := stream[at]
				at++
				return v
			}
			again, e := m.ApplyAcceptedPlayerHit(c)
			if e != nil || again.Limit != "" || int(again.RNGCalls) != len(stream) {
				t.Fatal(again, e, at)
			}
			after := PlayerDamage12(c.Systems)
			if bucket == 3 || bucket == 8 {
				if after[bucket] != 2 || again.Bucket != 6 {
					t.Fatal("failed-wing reroll", after, again)
				}
			} else {
				v := byte(2)
				if bucket == 0 {
					v = 0xf2
				}
				if bucket == 1 {
					v = 0xc2
				}
				if after[bucket] != v {
					t.Fatal("repeat severity/fullbyte", after, bucket)
				}
			}
			if bucket == 4 && (c.Systems.EngineTargets[0] != 0 || after[11] != 0) || bucket == 9 && (c.Systems.EngineTargets[1] != 0 || after[11] != 0) {
				t.Fatal("one twinengine failure falsely terminal", after)
			}
			if bucket == 2 && after[11] != 1 {
				t.Fatal("mainrotor critical609 missing")
			}
			if bucket == 5 && (again.SourceStatus != 0x10 || again.Countdown != 4) {
				t.Fatal("source fuel terminal")
			}
			if bucket == 10 && *c.Overlay != ([2][2]int16{{32, 40}, {287, 71}}) {
				t.Fatal("second artwork coordinate", *c.Overlay)
			}
		})
	}
}
func TestGeneralFailureWholeByteNotMaskedSeverity(t *testing.T) {
	for _, masked := range []bool{false, true} {
		m, c := damageFixture(t, 2, 102)
		c.Systems.DamageMasks = [2]byte{0x80, 0x40}
		c.Systems.Damage[0] = 1
		c.Systems.Damage[2] = 1
		c.Systems.Damage[4] = 1
		if masked {
			c.Systems.Damage[4] = 0x81
		}
		got, e := m.ApplyAcceptedPlayerHit(c)
		if e != nil || got.Bucket != 6 {
			t.Fatal(got, e)
		}
		expected := byte(1)
		if masked {
			expected = 0
		}
		if c.Systems.Damage[9] != expected {
			t.Fatal("INC fullbyte==2 predicate", masked, c.Systems.Damage)
		}
	}
}
