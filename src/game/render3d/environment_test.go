package render3d

import (
	"bytes"
	"encoding/hex"
	"gunship2000-assets/internal/testfixture"
	"os"
	"testing"
)

func TestEnvironmentVisibilityPackingAndPaletteOrder(t *testing.T) {
	for _, q := range []struct {
		flags     uint16
		realistic bool
		want      uint16
		bank      string
	}{{0x300, false, 0x200, "DARK"}, {0x100, false, 0, "DAY"}, {0x100, true, 0x100, "GREY"}, {0x300, true, 0x300, "DARK"}, {0x20, true, 0x20, "DAY"}} {
		got := EffectiveEnvironmentFlags(q.flags, q.realistic)
		if got != q.want || EnvironmentPalette(got) != q.bank {
			t.Fatalf("%+v ->%04x/%s", q, got, EnvironmentPalette(got))
		}
	}
}

func TestEnvironmentPhilippineLFSRRemapAndDrawPurity(t *testing.T) {
	// Hand trace: source RNG4→2, first pixel2→1→b400→5a00→2d00.
	e := &EnvironmentState{Prefix: "PHIL", Switches: 0x100, Phase: 255, RNG: 4}
	e.Step(0x10)
	if len(e.noise) != 258 || !bytes.Equal([]byte{byte(e.noise[0]), byte(e.noise[1] >> 8), byte(e.noise[2] >> 8), byte(e.noise[3] >> 8)}, []byte{1, 0xb4, 0x5a, 0x2d}) {
		t.Fatal("LFSR/count source order", len(e.noise), e.noise[:4])
	}
	e.noise = []uint16{0, 1, 319, 320, 63999, 64000}
	before := e.PixelSeed
	pixels := make([]byte, 64000)
	remap := make([]byte, 256)
	remap[0] = 17
	if err := e.applyPixels(pixels, remap, 1); err != nil {
		t.Fatal(err)
	}
	if pixels[0] != 17 || pixels[319] != 17 || pixels[320] != 0 || pixels[63999] != 0 || e.PixelSeed != before {
		t.Fatal("exclusive bottom/state mutated")
	}
	clear := &EnvironmentState{Prefix: "PHIL", Phase: 255, RNG: 4}
	clear.Step(0xff)
	if len(clear.noise) != 0 || clear.RNG != 4 {
		t.Fatal("clear visibility manufactured weather")
	}
}

func TestEnvironmentAntarcticPulseBoundsAndTiming(t *testing.T) {
	e := &EnvironmentState{Prefix: "ANTC", Phase: 511, Switches: 0x100, RNG: 0x34}
	e.Step(0x10)
	// Independent source-LFSR draws:34→1a compass(cleared low3),→d trigger,
	// trigger bit1 is0: no pulse and no extra draws.
	if e.CompassDraw != 0 || e.RNG != 0xd || e.PulseRemaining != 0 {
		t.Fatal("source conditional draw order", *e)
	}
	e = &EnvironmentState{Prefix: "ANTC", Phase: 511, Switches: 0x100, RNG: 0x1c}
	e.Step(0x10)
	//1c→e compass cleared,→7 starts;7→b403→ee01. Low bytes3+1+255=259.
	if e.PulseRemaining != 259 || e.RNG != 0xee01 {
		t.Fatalf("pulse lifetime/draws:%+v", e)
	}
	e.Step(0x10) //513 odd: no DAC update
	if e.PulseVisible {
		t.Fatal("odd phase updated DAC")
	}
	e.Step(0x10) //514: q257→~257&15=14
	if !e.PulseVisible || e.PulseAmplitude != 14 {
		t.Fatal("even amplitude", e.PulseAmplitude)
	}
	rgb := make([]byte, 768)
	for i := range rgb {
		rgb[i] = 20
	}
	rgb[429] = 25
	rgb[430] = 26
	rgb[431] = 27
	e.applyPalette(rgb)
	if rgb[16*3] != 25 || rgb[16*3+1] != 26 || rgb[16*3+2] != 27 || rgb[15*3] != 20 || rgb[176*3] != 20 {
		t.Fatal("source ranges/cap")
	}
	e.PulseRemaining = 1
	e.RNG = 4
	e.Step(0x10)
	if e.PulseVisible || e.RNG != 2 || e.PulseDraw != 2 {
		t.Fatal("expiry must restore base and consume the source final draw", *e)
	}
}

func TestEnvironmentOriginalInstructionWitnesses(t *testing.T) {
	check := func(path string, base int, cases map[int]string) {
		t.Helper()
		raw, e := os.ReadFile(path)
		if os.IsNotExist(e) {
			t.Skip("optional original source missing")
		}
		if e != nil {
			t.Fatal(e)
		}
		for at, s := range cases {
			want, e := hex.DecodeString(s)
			if e != nil {
				t.Fatal(e)
			}
			if !bytes.Equal(raw[base+at:base+at+len(want)], want) {
				t.Fatalf("source operand%06x", base+at)
			}
		}
	}
	check(testfixture.Path("installation/GS.GS2"), 0x32e70, map[int]string{0x955: "a019ad988bc8", 0x95b: "8a2649a22ac0f7e1", 0x963: "322649a2250001310648a2"})
	check(testfixture.Path("installation/GS2.GS2"), 0x3df0, map[int]string{0x2038: "a15e19d1e873033500b4a35e19cb"})
	check(testfixture.Path("installation/GS2.GS2"), 0xd8b0, map[int]string{0x2606: "f606e53b01", 0x2645: "f606da00ff", 0x26b3: "f706da00ff01", 0x7434: "81e1ff1f81c90001", 0x743c: "d1ea730481f200b4"})
}
