package assets

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestStartupPANOperations(t *testing.T) {
	image := []byte{9, 9, 9, 9, 9, 9, 9, 9}
	cmd := []byte{2, 1, 2, 0x81, 0, 2, 7, 0x80, 1, 0x80, 4, 0x80, 1, 0xc0, 5, 0x80, 0, 0}
	// 8001 is a one-byte extended literal, C001 a one-byte extended fill.
	used, err := applyPANFrame(image, cmd)
	if err != nil {
		t.Fatal(err)
	}
	if used != len(cmd) || !bytes.Equal(image, []byte{1, 2, 9, 7, 7, 4, 5, 9}) {
		t.Fatalf("used%d pixels%v", used, image)
	}
	for _, bad := range [][]byte{{1}, {0x80, 0}, {0x80, 0xff, 0x7f}, {0, 9, 1}, {0x80, 2, 0xc0}} {
		if _, e := applyPANFrame(make([]byte, 8), bad); e == nil {
			t.Fatalf("accepted truncated/out-of-bounds %x", bad)
		}
	}
}

func TestStartupSSOperations(t *testing.T) {
	image := make([]byte, StartupPixels)
	image[0] = 77
	// A transparent literal, a literal, a transparent run, row break, a count/value row.
	cmd := []byte{0xfa, 0xfc, 2, 0xfd, 2, 0xfc, 0xfe, 0xf9, 4, 1, 0xfe, 0xff}
	used, e := applySSFrame(image, cmd, 0, 0, 4, 2, 3)
	if e != nil {
		t.Fatal(e)
	}
	if used != len(cmd) || image[0] != 77 || image[1] != 32 || image[320] != 31 || image[323] != 31 {
		t.Fatalf("pixels%v/%v", image[:4], image[320:324])
	}
	for _, bad := range [][]byte{{0xfa}, {0xfa, 0xfd, 4}, {0xf9, 5, 1, 0xfe, 0xff}, {0xfa, 5, 0xfe, 0xff}} {
		if _, e := applySSFrame(image, bad, 0, 0, 4, 1, 3); e == nil {
			t.Fatalf("accepted invalid SS %x", bad)
		}
	}
}

func TestStartupMalformedHeaders(t *testing.T) {
	for _, b := range [][]byte{nil, []byte("MADSS1"), append([]byte("MADSS1\x1a"), make([]byte, 24)...)} {
		if _, e := DecodeStartupSS(b); e == nil {
			t.Fatalf("accepted invalid SS header%x", b)
		}
	}
	for _, b := range [][]byte{nil, {0}, {0xff, 0xff, 1}} {
		if _, e := DecodeStartupPAN(b); e == nil {
			t.Fatalf("accepted invalid PAN%x", b)
		}
	}
}

// These tests read original input only. Python prototype pixels and Step18 DAC
// captures are independent witnesses, never inputs to either production decoder.
func TestStartupOriginalResources(t *testing.T) {
	root := os.Getenv("GS2000_FIXTURE_ROOT")
	if root == "" {
		t.Skip("GS2000_FIXTURE_ROOT optional original-resource witnesses")
	}
	for _, row := range []struct {
		name        string
		decode      func([]byte) (*StartupAnimation, error)
		frames      int
		reference   string
		palette     string
		first, last int
	}{
		{"LABSLOGO.SS", DecodeStartupSS, 82, "ss-frames.indices", "logo-installed.rgb6", 31, 69},
		{"GST.PAN", DecodeStartupPAN, 813, "pan-frames.indices", "intro-sample03.rgb6", 0, 256},
	} {
		t.Run(row.name, func(t *testing.T) {
			b, e := os.ReadFile(filepath.Join(root, "installation", row.name))
			if e != nil {
				t.Fatal(e)
			}
			a, e := row.decode(b)
			if e != nil {
				t.Fatal(e)
			}
			if len(a.Frames) != row.frames {
				t.Fatalf("frames%d", len(a.Frames))
			}
			goldenBytes, e := os.ReadFile(filepath.Join(root, "intro-hashes/frame-reference-hashes.json"))
			if e != nil {
				t.Fatal(e)
			}
			var golden map[string]struct {
				SHA256 []string `json:"sha256"`
			}
			if e = json.Unmarshal(goldenBytes, &golden); e != nil {
				t.Fatal(e)
			}
			witness := golden[row.name].SHA256
			palette, e := os.ReadFile(filepath.Join(root, "intro-palette", row.palette))
			if e != nil {
				t.Fatal(e)
			}
			if len(witness) != row.frames {
				t.Fatal("reference size")
			}
			for i, f := range a.Frames {
				if fmt.Sprintf("%x", sha256.Sum256(f.Pixels)) != witness[i] {
					t.Fatalf("frame%d differs independent Python", i)
				}
				if !bytes.Equal(f.PaletteRGB6[row.first*3:row.last*3], palette[row.first*3:row.last*3]) {
					t.Fatalf("frame%d source palette differs Step18", i)
				}
			}
			t.Logf("%s: %d frames source%s consumed%d logical%d", row.name, len(a.Frames), a.SourceSHA256, a.CompressedConsumed, a.LogicalBytes)
		})
	}
}

func TestStartupCompressedCopiesAndSpanMarkers(t *testing.T) {
	for _, row := range []struct {
		input []byte
		want  string
	}{
		{[]byte{0xa3, 0, 'A', 'B', 0xfe, 0, 0, 0}, "ABABA"},
		{[]byte{0x15, 0, 'A', 0xff, 0xf9, 0, 0, 0}, "AAAA"},
		{[]byte{0x2d, 0, 'A', 0, 0, 1, 'B', 0, 0, 0}, "AB"},
	} {
		got, used, e := expandStartupPAN(row.input)
		if e != nil {
			t.Fatal(e)
		}
		if string(got) != row.want || used != len(row.input) {
			t.Fatalf("got%q used%d wanted%q", got, used, row.want)
		}
	}
	if _, _, e := expandStartupPAN([]byte{0, 0, 0}); e == nil {
		t.Fatal("accepted back-reference before output")
	}
}

func TestStartupFadeWitness(t *testing.T) {
	root := os.Getenv("GS2000_FIXTURE_ROOT")
	if root == "" {
		t.Skip("optional original source / prior DAC witness")
	}
	b, e := os.ReadFile(filepath.Join(root, "installation/GST.PAN"))
	if e != nil {
		t.Fatal(e)
	}
	a, e := DecodeStartupPAN(b)
	if e != nil {
		t.Fatal(e)
	}
	fade, e := StartupPANFade(a, 150)
	if e != nil {
		t.Fatal(e)
	}
	if len(fade) != 301 {
		t.Fatalf("fadeframes%d", len(fade))
	}
	prior, e := os.ReadFile(filepath.Join(root, "intro-palette/intro-sample01.rgb6"))
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(fade[99].PaletteRGB6, prior) {
		t.Fatal("fadecallback98 differs independent original-game DAC witness")
	}
	if !bytes.Equal(fade[300].PaletteRGB6, a.Frames[0].PaletteRGB6) {
		t.Fatal("fade final palette not source")
	}
	for i, f := range fade {
		if !bytes.Equal(f.Pixels, a.Frames[0].Pixels) {
			t.Fatalf("fade%d changes decoded first frame", i)
		}
	}
	for _, batch := range []int{0, 1, 257} {
		if _, e := StartupPANFade(a, batch); e == nil {
			t.Fatalf("accepted unsupportedbatch%d", batch)
		}
	}
}

func TestStartupControlWordEagerRefill(t *testing.T) {
	input := append([]byte{0xff, 0xff}, []byte("ABCDEFGHIJKLMNO")...)
	input = append(input, 2, 0, 'P', 0, 0, 0)
	got, used, e := expandStartupPAN(input)
	if e != nil {
		t.Fatal(e)
	}
	if string(got) != "ABCDEFGHIJKLMNOP" || used != len(input) {
		t.Fatalf("eagerrefill got%q consumed%d", got, used)
	}
}

func TestStartupSSByteCountWrap(t *testing.T) {
	scene := make([]byte, StartupPixels)
	used, e := applySSFrame(scene, []byte{0xf9, 0, 1, 0xfe, 0xff}, 0, 0, 256, 1, 1)
	if e != nil || used != 5 {
		t.Fatalf("256-pixel byte count wrap: %d,%v", used, e)
	}
	if scene[255] != 31 || scene[256] != 0 {
		t.Fatal("byte-count wrap extent")
	}
}
