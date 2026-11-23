package assets

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"strings"
)

// Encoding-independent verification is an additional complete-profile witness,
// never a replacement for manifest/output integrity or historical resource IDs.
const decodedResourceProfileVersion = "logical-resource-decoded-media-v1"
const decodedPreparation31 = "fcf81694640c69b51f009340a149e0f18505978e8f43a46589ee23c1e0c3645c"
const decodedPreparation32Candidate = "4c5c7daf526b72b498d823bccf99d3ab7e1f3a2c14f574a793e64d4418467829"
const decodedPreparation34 = "9e00adda6089f198216818d75a916a10a50c49003a560ff90708b559257381d5"
const decodedPreparation33 = "d0f9b5647ae6378f70daa4840ddd11b936f66d4668a092bfc5c447ba27710aa1"
const decodedPreparation32 = "92e551b1666eab30b58655f098c7260dbb6856c6ef3d1e837d26bc3fb5e9fe44"

func preparedPNGOutput(o Output) bool {
	if !strings.HasSuffix(o.Path, ".png") {
		return false
	}
	switch o.Kind {
	case "indexed_picture_png", "mission_supported_colour_png", "labelled_palette_swatch", "world_prepared_data", "hud_gray8_index_png", "cockpit_gray8_index_png", "cockpit_context_colour_png", "bitmap_font_mask_atlas", "hud_binary_glyph_mask", "office_lettering_atlas_png", "replay_browser_context_colour_png":
		return true
	}
	return false
}

// Preserve indexed pixels AND the complete ordered palette (including alpha),
// dimensions and decoded model. RGB equality alone would lose index semantics.
// The Go PNG decoder is also the prepared-reader decoder; ancillary encoding
// details it does not consume are not runtime inputs. No PNG re-encoding is used.
func decodedPNGIdentity(raw []byte) ([]byte, error) {
	cfg, e := png.DecodeConfig(bytes.NewReader(raw))
	if e != nil {
		return nil, e
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > 4096 || cfg.Height > 4096 || int64(cfg.Width)*int64(cfg.Height) > maxPixels {
		return nil, fmt.Errorf("decoded profile PNG dimensions exceed bounds")
	}
	im, e := png.Decode(bytes.NewReader(raw))
	if e != nil {
		return nil, e
	}
	doc := struct {
		Version, Model string
		Width, Height  int
		Palette        [][4]uint16
		Pixels         []byte
	}{Version: "prepared-png-pixels-v1", Width: cfg.Width, Height: cfg.Height}
	var data []byte
	var stride, bpp int
	switch p := im.(type) {
	case *image.Paletted:
		doc.Model = "indexed8"
		data, stride, bpp = p.Pix, p.Stride, 1
		for _, c := range p.Palette {
			var n color.NRGBA64
			switch q := c.(type) {
			case color.NRGBA:
				n = color.NRGBA64{uint16(q.R) * 257, uint16(q.G) * 257, uint16(q.B) * 257, uint16(q.A) * 257}
			case color.NRGBA64:
				n = q
			default:
				n = color.NRGBA64Model.Convert(c).(color.NRGBA64)
			}
			doc.Palette = append(doc.Palette, [4]uint16{n.R, n.G, n.B, n.A})
		}
	case *image.Gray:
		doc.Model = "gray8"
		data, stride, bpp = p.Pix, p.Stride, 1
	case *image.Gray16:
		doc.Model = "gray16be"
		data, stride, bpp = p.Pix, p.Stride, 2
	case *image.RGBA:
		doc.Model = "rgba8"
		data, stride, bpp = p.Pix, p.Stride, 4
	case *image.NRGBA:
		doc.Model = "nrgba8"
		data, stride, bpp = p.Pix, p.Stride, 4
	case *image.RGBA64:
		doc.Model = "rgba16be"
		data, stride, bpp = p.Pix, p.Stride, 8
	case *image.NRGBA64:
		doc.Model = "nrgba16be"
		data, stride, bpp = p.Pix, p.Stride, 8
	default:
		return nil, fmt.Errorf("unsupported decoded profile PNG model %T", im)
	}
	width := cfg.Width * bpp
	doc.Pixels = make([]byte, 0, width*cfg.Height)
	for y := 0; y < cfg.Height; y++ {
		doc.Pixels = append(doc.Pixels, data[y*stride:y*stride+width]...)
	}
	return json.Marshal(doc)
}
