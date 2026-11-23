// Package rasterlayout maps the original 320x200 picture into a presentation
// framebuffer. The display always presents that picture at a corrected 4:3.
package rasterlayout

type Layout struct {
	Width, Height, PictureHeight, Top int
	X                                 [321]int
	Y                                 [201]int
	LogicalX, LogicalY                []int
	OverlayX, OverlayY                []SamplePhase
}

// SamplePhase maps one subpixel pass of a logical overlay. Short cells repeat
// their final physical sample on read and contribute no write at this phase.
// WriteStart indexes the ordered writes for any half-open logical interval.
type SamplePhase struct {
	Read       []uint16
	Write      []Sample
	WriteStart []uint16
}

type Sample struct {
	Logical, Physical uint16
}

func overlayPhases(edges []int, extent int) []SamplePhase {
	cells := len(edges) - 1
	phases := make([]SamplePhase, (extent+cells-1)/cells)
	for offset := range phases {
		phase := &phases[offset]
		phase.Read = make([]uint16, cells)
		phase.Write = make([]Sample, 0, cells)
		phase.WriteStart = make([]uint16, cells+1)
		for i, start := range edges[:cells] {
			at := start + offset
			phase.Read[i] = uint16(min(at, edges[i+1]-1))
			phase.WriteStart[i] = uint16(len(phase.Write))
			if at < edges[i+1] {
				phase.Write = append(phase.Write, Sample{uint16(i), uint16(at)})
			}
		}
		phase.WriteStart[cells] = uint16(len(phase.Write))
	}
	return phases
}

func newLayout(w, h, picture int) *Layout {
	l := &Layout{Width: w, Height: h, PictureHeight: picture, Top: (h - picture) / 2, LogicalX: make([]int, w), LogicalY: make([]int, h)}
	for i := range l.X {
		l.X[i] = (i*w + 319) / 320
	}
	for i := range l.Y {
		l.Y[i] = l.Top + (i*picture+199)/200
	}
	for i := range l.LogicalX {
		l.LogicalX[i] = i * 320 / w
	}
	for i := range l.LogicalY {
		l.LogicalY[i] = max(0, min(199, (i-l.Top)*200/picture))
	}
	if w*h > 640*400 {
		l.OverlayX = overlayPhases(l.X[:], w)
		l.OverlayY = overlayPhases(l.Y[:], picture)
	}
	return l
}

var low = newLayout(320, 200, 200)
var double = newLayout(640, 400, 400)
var vga = newLayout(640, 480, 480)
var svga = newLayout(800, 600, 600)
var xga = newLayout(1024, 768, 768)
var sxga = newLayout(1280, 1024, 960)

func FromPixels(n int) *Layout {
	switch n {
	case 320 * 200:
		return low
	case 640 * 400:
		return double
	case 640 * 480:
		return vga
	case 800 * 600:
		return svga
	case 1024 * 768:
		return xga
	case 1280 * 1024:
		return sxga
	}
	return nil
}
func ByName(name string) *Layout {
	switch name {
	case "320x200":
		return low
	case "640x400":
		return double
	case "640x480":
		return vga
	case "800x600":
		return svga
	case "1024x768":
		return xga
	case "1280x1024":
		return sxga
	}
	return nil
}
func Resolve(name string, legacyScale int) *Layout {
	if l := ByName(name); l != nil {
		return l
	}
	if legacyScale == 2 {
		return double
	}
	return low
}

// Mapped reports whether logical rows require the aspect-correct mapping
// instead of the original uniform 1x or 2x span arithmetic.
func (l *Layout) Mapped() bool { return l != low && l != double }

func (l *Layout) ShiftRows(dst []byte, n int) {
	picture := dst[l.Top*l.Width : l.Y[200]*l.Width]
	shift := (l.Y[n] - l.Top) * l.Width
	copy(picture, picture[shift:])
	clear(picture[len(picture)-shift:])
}
