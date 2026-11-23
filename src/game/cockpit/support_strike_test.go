package cockpit

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"gunship2000-assets/internal/testfixture"
	"os"
	"testing"
)

func TestSupportStrikeSourceOperandsAndGlyph(t *testing.T) {
	b, err := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if err != nil {
		t.Fatal(err)
	}
	if got := sha256.Sum256(b); hex.EncodeToString(got[:]) != "1a1db96070adb8c23e1eb499b449cc147f9fb0fd7b47a65e88c4c5603a40e209" {
		t.Fatal("unrecognized source profile")
	}
	for ip, h := range map[int]string{0xca7: "26833ea20a007459", 0xcb2: "6805088b461080c402", 0xcc4: "26a1a90a268b16ab0a", 0xce8: "26a1a50a268b16a70a", 0xd00: "9a76337f0383c40e", 0x805: "0c04fe000002020000fefe000c0100fe00020c01fe00020000"} {
		want, _ := hex.DecodeString(h)
		if !bytes.Equal(b[0x1bb0+ip:][:len(want)], want) {
			t.Fatalf("source Support operand/glyph %x", ip)
		}
	}
}

func strikeAssets() *Assets {
	a := testAssets()
	a.Sine[1024] = 16384
	// Independent original byte transcription: three literal12 polylines.
	a.MapVectors["map-0805"] = []Vector{{Colour: 12, Points: [][2]int16{{-2, 0}, {0, 2}, {2, 0}, {0, -2}, {-2, 0}}}, {Colour: 12, Points: [][2]int16{{0, -2}, {0, 2}}}, {Colour: 12, Points: [][2]int16{{-2, 0}, {2, 0}}}}
	return a
}

func TestSupportStrikeProjectionShadowClipAndPurity(t *testing.T) {
	a := strikeAssets()
	before := bytes.Clone([]byte{a.MapVectors["map-0805"][0].Colour})
	point := MapPoint{H0: 200, H1: 300}
	project := func(x, y uint32) (int, int) { return int(x) / 2, int(y) / 3 }
	dst := bytes.Repeat([]byte{9}, 64000)
	if err := a.DrawSupportStrike(dst, &point, project, 8, Full); err != nil {
		t.Fatal(err)
	}
	for _, xy := range [][2]int{{100, 100}, {98, 100}, {102, 100}, {100, 98}, {100, 102}} {
		if dst[xy[1]*320+xy[0]] != 12 {
			t.Fatal("source literal red marker", xy)
		}
	}
	if dst[103*320+101] != 0 || dst[97*320+100] != 9 {
		t.Fatal("source0200 one-pixel shadow/extent")
	}
	if point != (MapPoint{H0: 200, H1: 300}) || a.MapVectors["map-0805"][0].Colour != before[0] {
		t.Fatal("draw mutated source point/glyph")
	}
	blank := bytes.Repeat([]byte{9}, 64000)
	if err := a.DrawSupportStrike(blank, nil, project, 8, Full); err != nil || !bytes.Equal(blank, bytes.Repeat([]byte{9}, 64000)) {
		t.Fatal("no confirmed active strike must draw nothing", err)
	}
	// Source reduced overview shift10 shrinks the original artwork; there is
	// no fixed screen-size substitute, invented red circle or global clip.
	small := bytes.Repeat([]byte{9}, 64000)
	if err := a.DrawSupportStrike(small, &point, project, 10, Full); err != nil {
		t.Fatal(err)
	}
	if small[100*320+100] != 12 || small[101*320+101] != 0 || small[98*320+100] != 9 {
		t.Fatal("reduced overview scale/shadow")
	}
	clipped := bytes.Repeat([]byte{9}, 64000)
	if err := a.DrawSupportStrike(clipped, &point, func(uint32, uint32) (int, int) { return 2, 3 }, 8, Rect{2, 3, 226, 163}); err != nil {
		t.Fatal(err)
	}
	if clipped[3*320+2] != 12 || clipped[2*320+2] != 9 || clipped[3*320+1] != 9 {
		t.Fatal("detail local clip")
	}
}
