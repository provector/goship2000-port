package menus

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"gunship2000-assets/game/flight"
	"gunship2000-assets/internal/testfixture"
	"os"
	"reflect"
	"testing"
)

func TestTerminalOriginalOperandsAndInstalledVGAOwnership(t *testing.T) {
	raw, err := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if os.IsNotExist(err) {
		t.Skip("immutable recognized original needed for operand checks")
	}
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprintf("%x", sha256.Sum256(raw)) != "1a1db96070adb8c23e1eb499b449cc147f9fb0fd7b47a65e88c4c5603a40e209" {
		t.Fatal("original source profile differs")
	}
	// File mappings independently follow MZ resident600+16*CS+IP, section1
	// d8b0+IP and selected section7 driver3e550+IP. No runtime-CS claim.
	for _, c := range []struct {
		at   int
		want []byte
	}{
		{0xd8b0 + 0x23c6, []byte{0x26, 0xf6, 0x06, 0x66, 0x02, 0x10}},
		{0xd8b0 + 0x23ff, []byte{0xb8, 0xff, 0x7f}},
		{0xd8b0 + 0x24af, []byte{0xb8, 0x00, 0x20}},
		{0xd8b0 + 0x2436, []byte{0xb9, 0x48, 0x01}},
		{0xd8b0 + 0x24e6, []byte{0xb9, 0x48, 0x01}},
		{0xd8b0 + 0x2530, []byte{0x26, 0xff, 0x0e, 0x0c, 0x2d}},
		{0xd8b0 + 0x25c6, []byte{0xf6, 0x06, 0xde, 0x00, 0x80}},
		{0x3df0 + 0x1f20, []byte{0x55, 0x8b, 0xec, 0x06, 0x57, 0x56}},
		{0x3e550 + 0x5f, []byte{0xea, 0x14, 0x07, 0x47, 0x1d}}, //1979 ->0714 point.
		{0x3e550 + 0x78, []byte{0xea, 0xb3, 0x0c, 0x47, 0x1d}}, //1992 ->0cb3 CRTC.
		{0x3e550 + 0x725, []byte{0x8a, 0xc2, 0xaa, 0xcb}},
		{0x3e550 + 0xccd, []byte{0xba, 0xd4, 0x03, 0xef}}, //not a rectangle fill.
		{0xa120 + 0x450, []byte("setup.dat\x00")},
		{0xa120 + 0x477, []byte{0xa0, 0xbe, 0x3b, 0x98, 0x50}}, //SETUP byte0 -> mode init.
	} {
		if !bytes.Equal(raw[c.at:c.at+len(c.want)], c.want) {
			t.Fatalf("source operand differs at file%x", c.at)
		}
	}
}

func TestTerminalPerIterationCountsAudioAndCompletion(t *testing.T) {
	for _, c := range []struct {
		status byte
		count  int16
		want   int
		colour byte
		event  uint16
		shift  int
	}{
		{0x10, 8, 4095, 66, 0x28, 4}, {0x10, 4, 8191, 66, 0x28, 4},
		{4, 2, 4095, 67, 0x11, 2}, {4, 1, 8191, 67, 0x11, 0},
		{0x14, 3, 10922, 66, 0x28, 0}, {0x10, 32767, 1, 66, 0x28, 0},
		{4, 8193, 0, 67, 0x11, 0},
	} {
		t.Run(fmt.Sprintf("%02x/%d", c.status, c.count), func(t *testing.T) {
			s := &flight.State{Terminal: &flight.Terminal{Status: c.status, Countdown: c.count}}
			var p terminalPresentation
			draws := 0
			audio, err := p.advance(s, terminalPresentationInputs{AudioEnabled: true, Random: func() uint16 { draws++; return 0 }})
			if err != nil || draws != c.want || p.PixelCount != c.want || p.RNGCalls != uint64(c.want) || s.Terminal.Countdown != c.count-1 || p.SourceCountdown != c.count || p.DisplayStartRows != c.shift {
				t.Fatal("source loop/countdown ownership", p, draws, err)
			}
			if c.want != 0 && p.pixels[0].colour != c.colour {
				t.Fatal("source SI start/colour", p.pixels[0])
			}
			if !reflect.DeepEqual(audio, []sceneAudioCommand{{event: 0xb}, {event: c.event}}) {
				t.Fatal("source ordered audio", audio)
			}
			if p.Completed != (c.count == 1) {
				t.Fatal("completion boundary", p.Completed)
			}
			if p.Completed {
				before := p.Frames
				again, err := p.advance(s, terminalPresentationInputs{AudioEnabled: true, Random: func() uint16 { t.Fatal("completed sequence consumed RNG"); return 0 }})
				if err != nil || len(again) != 0 || p.Frames != before {
					t.Fatal("completed sequence repeated")
				}
			}
		})
	}
	// Repeated source calls are deliberate. No once-at-entry replacement policy.
	s := &flight.State{Terminal: &flight.Terminal{Status: 4, Countdown: 2}}
	var p terminalPresentation
	for n := 0; n < 2; n++ {
		audio, err := p.advance(s, terminalPresentationInputs{View: 3, AudioEnabled: true})
		if err != nil || len(audio) != 2 || !p.Exterior || p.RNGCalls != 0 {
			t.Fatal("exterior/repeated command ownership", p, err)
		}
	}
}

func TestTerminalPixelAddressesScaleAndDrawPurity(t *testing.T) {
	//327 -> linear327 (x7,y1),328 ->320,ffff ->63943. DIV328 is not
	// an ordinary clipped x<320 point. These expectations are hand calculated.
	values := []uint16{327, 328, 65535}
	s := &flight.State{Terminal: &flight.Terminal{Status: 0x10, Countdown: 10001}}
	var p terminalPresentation
	next := 0
	_, err := p.advance(s, terminalPresentationInputs{Random: func() uint16 { v := values[next]; next++; return v }})
	if err != nil || !reflect.DeepEqual(p.pixels, []terminalPixel{{327, 66}, {320, 67}, {63943, 66}}) {
		t.Fatal("raw point addressing", p.pixels, err)
	}
	before := append([]terminalPixel(nil), p.pixels...)
	for _, scale := range []int{1, 2} {
		var first []byte
		for draw := 0; draw < 3; draw++ {
			dst := bytes.Repeat([]byte{9}, 64000*scale*scale)
			if err := p.draw(dst, scale); err != nil {
				t.Fatal(err)
			}
			for i, want := range []struct {
				index  int
				colour byte
			}{{327, 66}, {320, 67}, {63943, 66}} {
				_ = i
				x, y := want.index%320*scale, want.index/320*scale
				for sy := 0; sy < scale; sy++ {
					for sx := 0; sx < scale; sx++ {
						if dst[(y+sy)*320*scale+x+sx] != want.colour {
							t.Fatal("source point expansion", scale, want)
						}
					}
				}
			}
			if first == nil {
				first = dst
			} else if !bytes.Equal(first, dst) {
				t.Fatal("identical source state redrew differently")
			}
		}
	}
	if next != 3 || p.Frames != 1 || p.RNGCalls != 3 || !reflect.DeepEqual(before, p.pixels) {
		t.Fatal("drawing mutated terminal state or consumed RNG")
	}
}

func TestTerminalCRTCTailPolicyAndFade(t *testing.T) {
	for _, scale := range []int{1, 2} {
		dst := make([]byte, 64000*scale*scale)
		for y := 0; y < 200*scale; y++ {
			for x := 0; x < 320*scale; x++ {
				dst[y*320*scale+x] = byte(y / scale)
			}
		}
		p := terminalPresentation{Active: true, DisplayStartRows: 2}
		if err := p.draw(dst, scale); err != nil {
			t.Fatal(err)
		}
		if dst[0] != 2 || dst[197*scale*320*scale] != 199 || dst[198*scale*320*scale] != 0 || dst[199*scale*320*scale] != 0 {
			t.Fatal("closed CRTC rows or Step38 fresh-profile zero-tail policy", scale)
		}
	}
	rgb := make([]byte, 768)
	copy(rgb, []byte{63, 48, 4, 3, 0, 255})
	if err := fadeTerminalRGB6(rgb); err != nil || !bytes.Equal(rgb[:6], []byte{48, 36, 3, 3, 0, 192}) {
		t.Fatal("source byte decay", rgb[:6], err)
	}
	for _, c := range []struct {
		view           int
		mode, graphics byte
		fade           bool
	}{{0, 0, 0x10, true}, {3, 0, 0x10, true}, {11, 0, 0x10, false}, {0, 1, 0x10, false}, {0, 0, 0, false}} {
		s := &flight.State{Terminal: &flight.Terminal{Status: 4, Countdown: 32767}}
		var p terminalPresentation
		_, err := p.advance(s, terminalPresentationInputs{View: c.view, GraphicsMode: c.mode, GraphicsOptions: c.graphics})
		if err != nil || p.Fade != c.fade {
			t.Fatal("source fade gates", c, p, err)
		}
	}
}

func TestTerminalInvalidBoundaryDoesNotPublishPartialEffects(t *testing.T) {
	for _, count := range []int16{-1, 2} {
		s := &flight.State{Terminal: &flight.Terminal{Status: 4, Countdown: count}}
		p := terminalPresentation{}
		if _, err := p.advance(s, terminalPresentationInputs{}); err == nil {
			t.Fatal("unsupported negative counter or missing shared RNG admitted")
		}
		if s.Terminal.Countdown != count || p.Active || p.Frames != 0 || p.RNGCalls != 0 {
			t.Fatal("invalid input published effects")
		}
	}
}
