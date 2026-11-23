package assets

import (
	"fmt"
	"image"
	"image/color"
)

func FrameRGBA(indices []byte, palette color.Palette) (*image.RGBA, error) {
	if len(indices) != 320*200 || len(palette) != 256 {
		return nil, fmt.Errorf("office frame requires 320x200 indices and256 palette entries")
	}
	img := image.NewRGBA(image.Rect(0, 0, 320, 200))
	var rgba [256]color.RGBA
	for i, c := range palette {
		rgba[i] = color.RGBAModel.Convert(c).(color.RGBA)
	}
	for i, index := range indices {
		c := rgba[index]
		j := 4 * i
		img.Pix[j], img.Pix[j+1], img.Pix[j+2], img.Pix[j+3] = c.R, c.G, c.B, 255
	}
	return img, nil
}
