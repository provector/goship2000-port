package cockpit

import "fmt"

// LogicalPass applies an original320x200 recipe independently to each physical
// subpixel plane. Artwork/fonts are nearest-neighbour2x, while destination reads
// (mask, match/remap, attitude) inspect the real640x400 world index underneath.
// The world is never downsampled and upscaled as a completed image.
func LogicalPass(dst []byte, scale int, draw func([]byte) error) error {
	if len(dst) > 256000 {
		var scratch OverlayScratch
		return scratch.Draw(dst, nil, scale, draw)
	}
	if scale == 1 || scale == 0 {
		if !LogicalTarget(dst) {
			return fmt.Errorf("logical recipe needs320x200 target")
		}
		return draw(dst)
	}
	if scale != 2 || len(dst) != 256000 {
		return fmt.Errorf("logical recipe scale must be1 or2 with matching target")
	}
	var logical [64000]byte
	for sy := 0; sy < 2; sy++ {
		for sx := 0; sx < 2; sx++ {
			for y := 0; y < 200; y++ {
				for x := 0; x < 320; x++ {
					logical[y*320+x] = dst[(y*2+sy)*640+x*2+sx]
				}
			}
			if err := draw(logical[:]); err != nil {
				return err
			}
			for y := 0; y < 200; y++ {
				for x := 0; x < 320; x++ {
					dst[(y*2+sy)*640+x*2+sx] = logical[y*320+x]
				}
			}
		}
	}
	return nil
}

func ComposeScaled(dst []byte, a *Assets, in Inputs, scale int) error {
	return LogicalPass(dst, scale, func(p []byte) error { return Compose(p, a, in) })
}
func DrawSceneConfirmationScaled(dst []byte, a *Assets, end bool, scale int) error {
	return LogicalPass(dst, scale, func(p []byte) error { return DrawSceneConfirmation(p, a, end) })
}
func (a *Assets) TextScaled(dst []byte, text string, x, y int, colour byte, style uint16, clip Rect, scale int) error {
	return LogicalPass(dst, scale, func(p []byte) error { return a.Text(p, text, x, y, colour, style, clip) })
}
