package cockpit

import (
	"fmt"
	"gunship2000-assets/internal/rasterlayout"
)

const logicalPixels = 320 * 200

// A covered logical pass stores the current indexed image followed by its
// equally sized ownership plane. Ordinary callers supply just the image.
func LogicalTarget(dst []byte) bool { return len(dst) == logicalPixels || len(dst) == 2*logicalPixels }

func PutLogical(dst []byte, at int, ink byte) {
	dst[at] = ink
	if len(dst) == 2*logicalPixels {
		dst[logicalPixels+at] = 0
	}
}

// OverlayScratch belongs to one view. Each interleaved physical sample is
// drawn once, including destination-dependent remaps and transparent holes.
type OverlayScratch struct{ plane []byte }

func (s *OverlayScratch) Draw(dst, owners []byte, scale int, draw func([]byte) error) error {
	return s.DrawRect(dst, owners, scale, Full, draw)
}

// DrawRect transfers just the logical rectangle the supplied clipped writer
// reads or writes. The callback still sees the original logical coordinates.
func (s *OverlayScratch) DrawRect(dst, owners []byte, scale int, bounds Rect, draw func([]byte) error) error {
	if len(dst) > 256000 {
		layout := rasterlayout.FromPixels(len(dst))
		if layout == nil || len(owners) != 0 && len(owners) != len(dst) {
			return fmt.Errorf("invalid overlay dimensions")
		}
		return s.drawMapped(dst, owners, layout, bounds, draw)
	}
	if len(owners) == 0 {
		return LogicalPass(dst, scale, draw)
	}
	if (scale != 1 && scale != 2) || len(dst) != logicalPixels*scale*scale || len(owners) != len(dst) {
		return fmt.Errorf("invalid overlay dimensions")
	}
	if s.plane == nil {
		s.plane = make([]byte, 2*logicalPixels)
	}
	pixels, mask := s.plane[:logicalPixels], s.plane[logicalPixels:]
	if scale == 1 {
		copy(pixels, dst)
		copy(mask, owners)
		if err := draw(s.plane); err != nil {
			return err
		}
		copy(dst, pixels)
		copy(owners, mask)
		return nil
	}
	left, right := max(0, bounds.Left), min(319, bounds.Right)
	top, bottom := max(0, bounds.Top), min(199, bounds.Bottom)
	if left > right || top > bottom {
		return nil
	}
	for sy := 0; sy < 2; sy++ {
		for sx := 0; sx < 2; sx++ {
			for y := top; y <= bottom; y++ {
				at := (2*y+sy)*640 + 2*left + sx
				for x := left; x <= right; x++ {
					i := 320*y + x
					pixels[i] = dst[at]
					mask[i] = owners[at]
					at += 2
				}
			}
			if err := draw(s.plane); err != nil {
				return err
			}
			for y := top; y <= bottom; y++ {
				at := (2*y+sy)*640 + 2*left + sx
				for x := left; x <= right; x++ {
					i := 320*y + x
					dst[at] = pixels[i]
					owners[at] = mask[i]
					at += 2
				}
			}
		}
	}
	return nil
}

// Keep each real destination pixel for palette matches, holes and remaps. Logical
// cells can have different physical widths and heights at fractional scales.
func (s *OverlayScratch) drawMapped(dst, owners []byte, l *rasterlayout.Layout, b Rect, draw func([]byte) error) error {
	if s.plane == nil {
		s.plane = make([]byte, 2*logicalPixels)
	}
	plane := s.plane
	if len(owners) == 0 {
		plane = plane[:logicalPixels]
	}
	left := min(max(b.Left, 0), 320)
	right := max(left, min(b.Right, 319)+1)
	top := min(max(b.Top, 0), 200)
	bottom := max(top, min(b.Bottom, 199)+1)
	stride := 0
	if l.Width%320 == 0 {
		stride = l.Width / 320
	}
	for _, ys := range l.OverlayY {
		rows := ys.Write[ys.WriteStart[top]:ys.WriteStart[bottom]]
		for dx, xs := range l.OverlayX {
			cols := xs.Read[left:right]
			first := l.X[left] + dx
			for y, py := range ys.Read[top:bottom] {
				start := (top+y)*320 + left
				gatherOverlayRow(dst, owners, s.plane, start, int(py)*l.Width, cols, first, stride)
			}
			if err := draw(plane); err != nil {
				return err
			}
			writes := xs.Write[xs.WriteStart[left]:xs.WriteStart[right]]
			for _, y := range rows {
				scatterOverlayRow(dst, owners, s.plane, int(y.Logical)*320, int(y.Physical)*l.Width, left, cols, writes, first, stride)
			}
		}
	}
	return nil
}

// Uniform columns advance by a constant stride. Fractional columns use the
// phase's clamped read positions, including the last sample of short cells.
func gatherOverlayRow(dst, owners, plane []byte, start, row int, cols []uint16, first, stride int) {
	pixels := plane[start : start+len(cols)]
	if len(owners) == 0 {
		for x, px := range cols {
			pixels[x] = dst[row+int(px)]
		}
		return
	}
	mask := plane[logicalPixels+start : logicalPixels+start+len(cols)]
	if stride != 0 {
		at := row + first
		for x := range pixels {
			pixels[x] = dst[at]
			mask[x] = owners[at]
			at += stride
		}
		return
	}
	for x, px := range cols {
		at := row + int(px)
		pixels[x] = dst[at]
		mask[x] = owners[at]
	}
}

func scatterOverlayRow(dst, owners, plane []byte, start, row, left int, cols []uint16, writes []rasterlayout.Sample, first, stride int) {
	if len(owners) == 0 {
		for _, x := range writes {
			dst[row+int(x.Physical)] = plane[start+int(x.Logical)]
		}
		return
	}
	// A fully writable interval has consecutive logical indices. Only sparse
	// phases need both indices from the write table.
	if len(writes) == len(cols) {
		i := start + left
		pixels := plane[i : i+len(cols)]
		mask := plane[logicalPixels+i : logicalPixels+i+len(cols)]
		if stride != 0 {
			at := row + first
			for x, v := range pixels {
				dst[at] = v
				owners[at] = mask[x]
				at += stride
			}
		} else {
			for x, px := range cols {
				at := row + int(px)
				dst[at] = pixels[x]
				owners[at] = mask[x]
			}
		}
		return
	}
	for _, x := range writes {
		at := row + int(x.Physical)
		i := start + int(x.Logical)
		dst[at] = plane[i]
		owners[at] = plane[logicalPixels+i]
	}
}
