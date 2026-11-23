package menus

import (
	"encoding/binary"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/internal/rasterlayout"
	"image"
	"image/color"
)

// Only simulation accepts the two explicit indexed targets. Menus/GPS retain
// their320x200 logical coordinates; the display viewport is always aspect4:3.
func indexedFrameSize(indices []byte) (int, int, error) {
	if l := rasterlayout.FromPixels(len(indices)); l != nil {
		return l.Width, l.Height, nil
	}
	return 0, 0, fmt.Errorf("unsupported indexed frame length%d", len(indices))
}
func indexedFrameRGBA(indices []byte, palette color.Palette) (*image.RGBA, error) {
	return indexedFrameRGBAInto(indices, palette, nil)
}
func indexedFrameRGBAInto(indices []byte, palette color.Palette, out *image.RGBA) (*image.RGBA, error) {
	w, h, err := indexedFrameSize(indices)
	if err != nil {
		return nil, err
	}
	if w == 320 && out == nil {
		return assetdata.FrameRGBA(indices, palette)
	}
	if len(palette) != 256 {
		return nil, fmt.Errorf("simulation requires256 palette entries")
	}
	if out == nil || out.Bounds() != image.Rect(0, 0, w, h) {
		out = image.NewRGBA(image.Rect(0, 0, w, h))
	}
	// RGBAModel supplies premultiplied channels; the display always uses opaque alpha.
	var colours [256]uint32
	for i, c := range palette {
		rgb := color.RGBAModel.Convert(c).(color.RGBA)
		colours[i] = uint32(rgb.R) | uint32(rgb.G)<<8 | uint32(rgb.B)<<16 | 0xff000000
	}
	l := rasterlayout.FromPixels(len(indices))
	bar := l.Top * w
	end := len(indices) - bar
	picture := out.Pix[4*bar : 4*end]
	for i, index := range indices[bar:end] {
		binary.LittleEndian.PutUint32(picture[4*i:], colours[index])
	}
	for i := 0; i < bar; i++ {
		binary.LittleEndian.PutUint32(out.Pix[4*i:], 0xff000000)
		binary.LittleEndian.PutUint32(out.Pix[4*(end+i):], 0xff000000)
	}
	return out, nil
}

// drawOriginalStrip draws a source-owned message supplied by its actual caller.
// Native operational diagnostics have no implicit path into this renderer.
func (c *FrontEnd) drawOriginalStrip(frame []byte, text string, colour int) {
	if len(frame) == 64000 {
		assetdata.OfficeFill(frame, 0, 191, 320, 9, 0)
		assetdata.DrawBottomTooltip(frame, c.Menus.Fonts[3], text, colour)
		return
	}
	layout := rasterlayout.FromPixels(len(frame))
	if layout == nil {
		return
	}
	var logical [64000]byte
	assetdata.DrawBottomTooltip(logical[:], c.Menus.Fonts[3], text, colour)
	for y := layout.Y[191]; y < layout.Y[200]; y++ {
		src := logical[layout.LogicalY[y]*320:]
		row := frame[y*layout.Width:]
		for x, lx := range layout.LogicalX {
			row[x] = src[lx]
		}
	}
}

// nearestRGBA rescales UI presentation only, never scene geometry or a canonical
// world/reference buffer. Used when a global modal freezes a different fade leg.
func nearestRGBA(src *image.RGBA, bounds image.Rectangle) *image.RGBA {
	dst := image.NewRGBA(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		sy := src.Rect.Min.Y + (y-bounds.Min.Y)*src.Rect.Dy()/bounds.Dy()
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			sx := src.Rect.Min.X + (x-bounds.Min.X)*src.Rect.Dx()/bounds.Dx()
			dst.SetRGBA(x, y, src.RGBAAt(sx, sy))
		}
	}
	return dst
}
