package cockpit

import (
	"bytes"
	"encoding/hex"
	"gunship2000-assets/internal/testfixture"
	"os"
	"testing"
)

func TestCaptionAndPauseOriginalArgumentOrder(t *testing.T) {
	b, err := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if os.IsNotExist(err) {
		t.Skip("optional original bytes")
	}
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []struct {
		at  int
		hex string
	}{
		{0x1dc60 + 0x28c1, "6a586a089a1a197f03"},         // y88,x8,191a
		{0x8400 + 0x2a3, "8b7e068b4608"},                 // actual07e0callee xBP6,yBP8
		{0xd8b0 + 0x5d3, "6a40b8a0002b46de509a1a197f03"}, // y64,x160-half
	} {
		w, e := hex.DecodeString(q.hex)
		if e != nil || !bytes.Equal(b[q.at:q.at+len(w)], w) {
			t.Fatalf("source ABI%06x", q.at)
		}
	}
	if !bytes.Equal(b[0x4ba4c:0x4ba4c+15], []byte("... PAUSED ...\x00")) {
		t.Fatal("actual source pause literal")
	}
}

func TestPauseBoundsRemapAndScaledDestinationOwnership(t *testing.T) {
	a := hudTestAssets()
	a.Literals["scene-pause"] = "... PAUSED ..."
	a.ScenePresentation.ConfirmationRemap = make([]byte, 256)
	for i := range a.ScenePresentation.ConfirmationRemap {
		a.ScenePresentation.ConfirmationRemap[i] = byte(i & 7)
	}
	// Testfont has14 glyphs*6=84 advance. Source SHR1 giveshalf42;
	// remap inclusive x114..205,y62..70; main textstarts118,64.
	for _, scale := range []int{1, 2} {
		p := bytes.Repeat([]byte{0x63}, 64000*scale*scale)
		if scale == 2 {
			p[62*2*640+114*2+1] = 0x65
		}
		if e := a.DrawPause(p, 0, scale); e != nil {
			t.Fatal(e)
		}
		at := func(x, y int) byte { return p[y*scale*320*scale+x*scale] }
		if at(113, 62) != 0x63 || at(114, 62) != 0xeb || at(205, 70) != 0xeb || at(206, 70) != 0x63 || at(118, 64) != 0x1a {
			t.Fatal("source padding/text/bounds", scale)
		}
		if scale == 2 && p[62*2*640+114*2+1] != 0xed {
			t.Fatal("scaled pass used wrong subpixel destination")
		}
	}
}
