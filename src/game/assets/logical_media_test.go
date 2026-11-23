package assets

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDecodedPNGPreservesIndicesPaletteAndBounds(t *testing.T) {
	im := image.NewPaletted(image.Rect(0, 0, 20, 4), color.Palette{color.NRGBA{R: 9, A: 0}, color.NRGBA{G: 200, A: 255}})
	for i := range im.Pix {
		im.Pix[i] = byte(i % 2)
	}
	encode := func(level png.CompressionLevel) []byte {
		var b bytes.Buffer
		if e := (&png.Encoder{CompressionLevel: level}).Encode(&b, im); e != nil {
			t.Fatal(e)
		}
		return b.Bytes()
	}
	a, b := encode(png.NoCompression), encode(png.BestCompression)
	if bytes.Equal(a, b) {
		t.Fatal("compression fixture ineffective")
	}
	x, e := decodedPNGIdentity(a)
	if e != nil {
		t.Fatal(e)
	}
	y, e := decodedPNGIdentity(b)
	if e != nil || !bytes.Equal(x, y) {
		t.Fatal("lossless encoding changes pixels", e)
	}
	im.Pix[0] ^= 1
	y, _ = decodedPNGIdentity(encode(png.BestCompression))
	if bytes.Equal(x, y) {
		t.Fatal("index change ignored")
	}
	im.Pix[0] ^= 1
	im.Palette[0] = color.NRGBA{R: 10, A: 0}
	y, _ = decodedPNGIdentity(encode(png.BestCompression))
	if bytes.Equal(x, y) {
		t.Fatal("hidden palette color change ignored")
	}
	for _, rect := range []image.Rectangle{image.Rect(0, 0, 4097, 1), image.Rect(0, 0, 2048, 1024)} {
		var b bytes.Buffer
		png.Encode(&b, image.NewGray(rect))
		if _, e := decodedPNGIdentity(b.Bytes()); e == nil {
			t.Fatal("decode bounds ignored", rect)
		}
	}
	if _, e := decodedPNGIdentity([]byte("broken PNG")); e == nil {
		t.Fatal("malformed PNG admitted")
	}
}
func TestCompleteDecodedPreparation31(t *testing.T) {
	root := os.Getenv("GS2000_Q14_CACHE_COPY")
	accepted := os.Getenv("GS2000_Q14_REFERENCE_ASSETS")
	if accepted == "" {
		accepted = os.Getenv("GS2000_TEST_EDITOR")
	}
	if root == "" || accepted == "" {
		t.Skip("explicit owned cache copy/current reference required")
	}
	var baseline []byte
	for _, p := range []string{accepted, root} {
		r, e := newPreparedReader(p)
		if e != nil {
			t.Fatal(e)
		}
		raw, e := r.read("manifest.json", 64<<20)
		if e != nil {
			t.Fatal(e)
		}
		var m Manifest
		if e = DecodeObject(raw, &m); e != nil {
			t.Fatal(e)
		}
		b, e := canonicalLogicalResourceDataMode(r, &m, true)
		if e != nil || Hash(b) != decodedPreparation31 {
			t.Fatal("complete decoded profile differs", Hash(b), e)
		}
		if baseline == nil {
			baseline = b
		} else if !bytes.Equal(baseline, b) {
			t.Fatal("semantic inventory differs")
		}
		h, e := LogicalResourceProfile(p)
		if e != nil || h != LogicalPreparation31 {
			t.Fatal(h, e)
		}
	}
}
func TestDecodedFallbackRetainsIntegrityAndSemanticRejection(t *testing.T) {
	input := os.Getenv("GS2000_Q14_CACHE_COPY")
	if input == "" {
		t.Skip("explicit owned cache copy required")
	}
	root := copyPrepared(t, input)
	mp := filepath.Join(root, "manifest.json")
	original, _ := os.ReadFile(mp)
	var m Manifest
	json.Unmarshal(original, &m)
	var o *Output
	for _, r := range m.Resources {
		for i := range r.Outputs {
			q := &r.Outputs[i]
			if strings.HasSuffix(q.Path, "ADS.PIC.diagnostic-grayscale.png") {
				o = q
			}
		}
	}
	if o == nil {
		t.Fatal("diagnostic PNG prerequisite missing")
	}
	p := filepath.Join(root, filepath.FromSlash(o.Path))
	raw, _ := os.ReadFile(p)
	bad := append([]byte(nil), raw...)
	bad[len(bad)/2] ^= 1
	os.WriteFile(p, bad, 0600)
	if _, e := LogicalResourceProfile(root); e == nil || !strings.Contains(e.Error(), "SHA-256") {
		t.Fatal("integrity weakened", e)
	}
	img, e := png.Decode(bytes.NewReader(raw))
	if e != nil {
		t.Fatal(e)
	}
	indexed, ok := img.(*image.Paletted)
	if !ok {
		t.Fatal("expected indexed witness")
	}
	indexed.Pix[0] ^= 1
	var b bytes.Buffer
	png.Encode(&b, indexed)
	os.WriteFile(p, b.Bytes(), 0600)
	o.SHA256 = Hash(b.Bytes())
	o.Size = b.Len()
	v, _ := json.Marshal(m)
	os.WriteFile(mp, v, 0600)
	if _, e := LogicalResourceProfile(root); e == nil || !strings.Contains(e.Error(), "unsupported logical resource content") {
		t.Fatal("changed decoded pixels admitted with matching declared hash", e)
	}
	bad = []byte("malformed PNG")
	os.WriteFile(p, bad, 0600)
	o.SHA256 = Hash(bad)
	o.Size = len(bad)
	v, _ = json.Marshal(m)
	os.WriteFile(mp, v, 0600)
	if _, e := LogicalResourceProfile(root); e == nil || !strings.Contains(e.Error(), "logical PNG") {
		t.Fatal("bad PNG admitted", e)
	}
}
