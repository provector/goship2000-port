package cockpit

import (
	"bytes"
	"encoding/hex"
	"gunship2000-assets/internal/testfixture"
	"os"
	"testing"
)

func hudTestAssets() *Assets {
	a := testAssets()
	for _, name := range []string{"digits-zero-mask", "digits-opaque"} {
		i := a.Atlases[name]
		for digit := 0; digit < 10; digit++ {
			i.Pixels[digit*6*i.Width] = byte(digit + 1)
		}
		a.Atlases[name] = i
	}
	for name, colour := range map[string]byte{"hud-arrow-alternate": 31, "hud-arrow": 33} {
		i := a.Sprites[name]
		i.Pixels[0] = colour
		a.Sprites[name] = i
	}
	for _, name := range []string{"station-baseline", "station-plain", "station-highlight"} {
		a.Sprites[name] = Image{Width: 1, Height: 1, Pixels: []byte{0}}
	}
	return a
}

func TestClearanceNumbersSourceHandCases(t *testing.T) {
	a := hudTestAssets()
	p := bytes.Repeat([]byte{99}, 64000)
	if err := a.clearanceNumbers(p, 128); err != nil {
		t.Fatal(err)
	}
	// AE7=128: SAR6*50=100, phase0; upper tick150 at38, lower50
	// at70, upper200 at22; current lowword(128*25)>>5=100 at54.
	for _, q := range []struct {
		x, y int
		want byte
	}{
		{285, 53, 31}, {286, 54, 2}, {290, 54, 1}, {294, 54, 1},
		{286, 38, 2}, {290, 38, 6}, {294, 38, 1},
		{286, 70, 1}, {290, 70, 6}, {294, 70, 1}, {286, 22, 3},
	} {
		if p[q.y*320+q.x] != q.want {
			t.Fatalf("pixel%d,%d=%d want%d", q.x, q.y, p[q.y*320+q.x], q.want)
		}
	}
	clear(p)
	if err := a.clearanceNumbers(p, 1280); err != nil {
		t.Fatal(err)
	}
	//1280*25/32=1000, unsigned width4 selects X286-4.
	if p[54*320+282] != 2 || p[54*320+294] != 1 {
		t.Fatal("source fourdigit width decision")
	}
	clear(p)
	if err := a.clearanceNumber(p, 286, 54, -50); err != nil {
		t.Fatal(err)
	}
	if p[54*320+282] != 0 || p[54*320+286] != 1 || p[54*320+290] != 6 {
		t.Fatal("negative tick must remain absolute3digit")
	}
}

func TestSpeedArrowSignedBranchesAndBlink(t *testing.T) {
	a := hudTestAssets()
	for _, q := range []struct {
		speed      int16
		phase      uint16
		y          int
		colour     byte
		firstDigit byte
	}{
		{100, 0, 41, 31, 2}, {0, 0, 57, 31, 1}, {-125, 2, 77, 31, 2},
		{250, 0, 17, 31, 3}, {251, 1, 17, 33, 3}, {-126, 1, 77, 33, 2},
	} {
		p := make([]byte, 64000)
		if err := a.speedNumber(p, q.speed, q.phase); err != nil {
			t.Fatal(err)
		}
		if p[q.y*320+19] != q.colour || p[(q.y+1)*320+20] != q.firstDigit {
			t.Fatalf("source speed case%+v", q)
		}
	}
	for _, q := range []struct {
		speed int16
		phase uint16
	}{{-1, 0}, {-125, 1}, {251, 0}, {-126, 2}} {
		p := make([]byte, 64000)
		if err := a.speedNumber(p, q.speed, q.phase); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(p, make([]byte, 64000)) {
			t.Fatalf("blinkoff drew:%+v", q)
		}
	}
}

func TestCommonFlightHUDAllViewsAndArrangements(t *testing.T) {
	a := hudTestAssets()
	for _, camera := range []int{0, 1, 2, 3, 4, 5, 6, 7} {
		for arrangement := 0; arrangement < 4; arrangement++ {
			p := make([]byte, 64000)
			in := Inputs{AircraftID: 0, Arrangement: arrangement, CameraMode: camera, HUD: true, HUDScale: 128, HUDVertical: 256, Vertical: 100, Damage: [10]byte{0x10}}
			if err := Compose(p, a, in); err != nil {
				t.Fatal(camera, arrangement, err)
			}
			if p[53*320+285] != 31 || p[41*320+19] != 31 {
				t.Fatalf("common flight HUD missing camera%d arrangement%d", camera, arrangement)
			}
			in.Damage[0] |= 0x80
			clear(p)
			if err := Compose(p, a, in); err != nil {
				t.Fatal(err)
			}
			if p[53*320+285] != 0 || p[41*320+19] != 0 {
				t.Fatal("damage80 failed common HUD guard")
			}
		}
	}
}

func TestDefenseCountPlacementAndSourceTimers(t *testing.T) {
	a := hudTestAssets()
	var d Definition
	for off, v := range map[int]int16{0x8c: 50, 0x8e: 100, 0x90: 70, 0x92: 110} {
		d.Parameters[off/2] = v
	}
	p := make([]byte, 64000)
	in := DefenseInputs{Chaff: 12, Flares: 34, ChaffTimer: 1}
	if err := a.drawDefenseCounts(p, d, in, 0); err != nil {
		t.Fatal(err)
	}
	if p[100*320+50] != 0 || p[110*320+70] != 4 {
		t.Fatal("AF7 must blink chaff; AF5clear retains flares")
	}
	if err := a.drawDefenseCounts(p, d, in, 2); err != nil {
		t.Fatal(err)
	}
	if p[100*320+50] != 2 {
		t.Fatal("source count12 at CPIT8c/8e")
	}
}

func TestHUDBranchesOriginalOperands(t *testing.T) {
	raw, err := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if os.IsNotExist(err) {
		t.Skip("optional original unavailable")
	}
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []struct {
		at   int
		want string
	}{
		{0x1dc60 + 0x250b, "26a1e70a8bc8c1f8066bc032"},
		{0x1dc60 + 0x2591, "266b06e70a19c1e805"},
		{0x1dc60 + 0x2617, "26a17604c1e002b9190099f7f9"},
		{0x1dc60 + 0x3ce9, "81fee803721781fe00807711"},
		{0x1dc60 + 0x566, "26833ef70a007407f606da0002"},
		{0x1dc60 + 0x575, "6802028e06b83326ff361900"},
		{0x1dc60 + 0x5aa, "6802028e06b83326ff361b00"},
	} {
		want, _ := hex.DecodeString(q.want)
		if !bytes.Equal(raw[q.at:q.at+len(want)], want) {
			t.Fatalf("original operand file%x", q.at)
		}
	}
}
