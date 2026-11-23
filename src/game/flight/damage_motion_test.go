package flight

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"gunship2000-assets/internal/testfixture"
	"os"
	"testing"
)

func TestPlayerDamageMotionOriginalOperands(t *testing.T) {
	b, err := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprintf("%x", sha256.Sum256(b)) != "1a1db96070adb8c23e1eb499b449cc147f9fb0fd7b47a65e88c4c5603a40e209" {
		t.Fatal("GS2 identity")
	}
	for ip, literal := range map[int]string{
		0x2df1: "26803e000600", 0x2e0f: "26803e000601", 0x2e83: "80e40f2d00f8f7d8", 0x2eb0: "8006d10008",
		0x2ecb: "26803e050600", 0x2ee3: "26803e050601", 0x2efc: "9a38207f03f7d8250f00", 0x2f08: "8a26db002500023d01001bc02500f080c408",
		0x2f82: "26803e090600", 0x2fc2: "26803e020602", 0x2fca: "26803e070602", 0x2ff5: "26803e020602", 0x2ffd: "26803e070602",
		0x3005: "8b46ecc1f8022946ec", 0x300e: "26803e020601", 0x3016: "26803e070601", 0x3021: "8b46ecc1f8042946ec",
		0x3041: "26803e000600", 0x30e3: "80e3fbc1fb02", 0x3111: "8b46ec8e06de32262b062a2dd1f82601062a2d",
		0x31e4: "268b0e3a06c1f9032bc1", 0x3263: "268b0edc00d1e199f7f9", 0x3314: "a1cc00d1f8f7d8", 0x3353: "262b06c600",
	} {
		want, e := hex.DecodeString(literal)
		if e != nil {
			t.Fatal(e)
		}
		if !bytes.Equal(b[0xd8b0+ip:][:len(want)], want) {
			t.Fatalf("source0975:%04x", ip)
		}
	}
}

func TestSourceEngineCombinationThrustAndFullByteDomains(t *testing.T) {
	p, q := sourceFixture(t)
	for _, tc := range []struct {
		name                      string
		port, starboard, terminal byte
		want                      int16
	}{
		{"healthy", 0, 0, 0, 1765}, {"portpartial", 1, 0, 0, 1710}, {"starboardpartial", 0, 1, 0, 1710}, {"bothpartial-reduction-once", 1, 1, 0, 1710},
		{"portfailed", 2, 0, 0, 1544}, {"starboardfailed", 0, 2, 0, 1544}, {"failed-and-partial", 2, 1, 0, 1503}, {"partial-and-failed", 1, 2, 0, 1503}, {"bothfailed", 2, 2, 0, 882},
		{"fullbyte5-is-not-partial1", 5, 0, 0, 1765}, {"fullbyte6-is-not-failed2", 6, 0, 0, 1765}, {"609-overrides-healthy", 0, 0, 1, 882},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Literal first-E0 outputs independently derived from source1765 curve,
			// SAR2/SAR4 reductions followed by negative arithmetic half-step easing.
			f := State{Pose: Pose{H0: 200000, H1: 200000, Height: 600}, RadarAltitude: 598, Rotor: 1, Collective: 31, Thrust: 1765}
			in := Input{DF: 6, E0: true, Phase: 1, Switches: 4, MassTerm: 7200}
			in.Damage[2], in.Damage[7], in.Damage[9] = tc.port, tc.starboard, tc.terminal
			if err := Step(&f, p, in, q); err != nil {
				t.Fatal(err)
			}
			if f.Thrust != tc.want {
				t.Fatal(f.Thrust, tc.want)
			}
		})
	}
	// The exact source easing can stop one below a positive target from below.
	f := State{Pose: Pose{H0: 200000, H1: 200000, Height: 600}, RadarAltitude: 598, Rotor: 1, Collective: 31}
	in := Input{DF: 6, E0: true, Switches: 4, MassTerm: 7200}
	in.Damage[7] = 2
	for n := 0; n < 32; n++ {
		in.Phase = uint16(n + 1)
		if err := Step(&f, p, in, q); err != nil {
			t.Fatal(err)
		}
	}
	if f.Thrust != 1323 {
		t.Fatal("one-low floor is original, not target1324 rounding repair", f.Thrust)
	}
}

func TestMainAndTailRotorExactMotionAndRNG(t *testing.T) {
	p, q := sourceFixture(t)
	for _, tc := range []struct {
		name                 string
		main, tail           byte
		phase                uint16
		alt                  int16
		draws                []uint16
		pitch, heading, roll int16
		clear                bool
	}{
		{"partial-main-phase2", 1, 0, 2, 3, []uint16{255, 511, 511}, -255, -511, -511, false},
		{"partial-main-phase0", 1, 0, 0, 3, []uint16{511, 511, 255}, 511, 511, 255, false},
		{"critical-main", 2, 0, 2, 3, []uint16{65535, 65535}, -6143, -1023, 2048, true},
		{"main-at-ground-no-motion", 1, 0, 2, 2, nil, 0, 0, 0, false},
		{"partial-tail-phase2", 0, 1, 2, 3, []uint16{0x1234}, 0, 52, 0, false},
		{"partial-tail-asymmetric-phase0", 0, 1, 0, 3, []uint16{0x1234}, 0, 12, 0, false},
		{"critical-tail-negative", 0, 2, 0, 3, nil, 0, -2048, 0, false},
		{"critical-tail-positive", 0, 2, 0x200, 3, nil, 0, 2048, 0, false},
		{"both-partial-source-order", 1, 1, 2, 3, []uint16{255, 511, 511, 0x1234}, -255, -459, -511, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := State{RadarAltitude: tc.alt, Hover: 1, HeightControl: 1}
			in := Input{Phase: tc.phase}
			in.Damage[0], in.Damage[5] = tc.main, tc.tail
			n := 0
			in.Random = func() uint16 {
				if n >= len(tc.draws) {
					t.Fatal("extra damage-motion draw")
				}
				v := tc.draws[n]
				n++
				return v
			}
			k := kernel{p: p, q: q}
			pitch, heading, roll := int16(0), uint16(0), int16(0)
			k.damageMotion(&f, in, &pitch, &heading, &roll)
			if pitch != tc.pitch || int16(heading) != tc.heading || roll != tc.roll || n != len(tc.draws) {
				t.Fatal(pitch, heading, roll, n)
			}
			if (f.Hover == 0) != tc.clear || (f.HeightControl == 0) != tc.clear {
				t.Fatal("critical-main mode reset", f)
			}
		})
	}
}

func TestSourceAutorotationMainRotorAnd609Owners(t *testing.T) {
	p, q := sourceFixture(t)
	for _, tc := range []struct {
		name                  string
		main, terminal        byte
		wantPower, wantThrust int16
	}{
		{"disengaged-healthy-rotor", 0, 0, 1937, 615}, {"disengaged609-still-autorotates", 0, 1, 1937, 615},
		{"partial-rotor-cannot-autorotate", 1, 0, 1984, 0}, {"critical-rotor-cannot-autorotate", 2, 1, 1984, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := State{Pose: Pose{H0: 200000, H1: 200000, Height: 600}, RadarAltitude: 598, IndicatedSpeed: 64, Collective: 31, Power: 1984}
			in := Input{DF: 6, E0: true, Phase: 1, Switches: 4, MassTerm: 7200, Random: func() uint16 { return 0 }}
			in.Damage[0], in.Damage[9] = tc.main, tc.terminal
			if err := Step(&f, p, in, q); err != nil {
				t.Fatal(err)
			}
			if f.Power != tc.wantPower || f.Thrust != tc.wantThrust || f.Rotor != 0 {
				t.Fatal(f.Power, f.Thrust, f.Rotor)
			}
		})
	}
}
