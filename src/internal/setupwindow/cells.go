package setupwindow

import (
	"github.com/gdamore/tcell/v2"
	"gunship2000-assets/internal/setupfont"
	"image/color"
	"sync"
)

// Only the tview owner draws cells. Publish a completed immutable front buffer.
// The SDL owner can resize, expose or repaint without observing half a widget.
type frame struct {
	cells            []tcell.SimCell
	cols, rows, x, y int
	cursor           bool
}
type screen struct {
	tcell.SimulationScreen
	mu      sync.Mutex
	current *frame
	closed  bool
}

func (s *screen) publish() {
	c, w, h := s.GetContents()
	x, y, v := s.GetCursor()
	// SimulationScreen replaces each changed cell's Runes slice on Show.
	// It reuses the outer front array; the renderer never accesses Bytes.
	s.current = &frame{append([]tcell.SimCell(nil), c...), w, h, x, y, v}
}
func (s *screen) Show() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		s.SimulationScreen.Show()
		s.publish()
	}
}
func (s *screen) Sync() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		s.SimulationScreen.Sync()
		s.publish()
	}
}
func (s *screen) Fini() { s.mu.Lock(); defer s.mu.Unlock(); s.closed = true; s.SimulationScreen.Fini() }
func (s *screen) resize(w, h int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		s.SetSize(w, h)
	}
}
func (s *screen) front() *frame { s.mu.Lock(); defer s.mu.Unlock(); return s.current }
func ink(c tcell.Color, fallback color.RGBA) color.RGBA {
	if !c.Valid() {
		return fallback
	}
	r, g, b := c.RGB()
	return color.RGBA{uint8(r), uint8(g), uint8(b), 255}
}
func mix(bg, fg, a uint8) uint8 { return uint8((int(bg)*(255-int(a)) + int(fg)*int(a)) / 255) }
func paint(f *frame, font setupfont.Font, pixels []byte) {
	const w, h = setupfont.Width, setupfont.Height
	for i, cell := range f.cells {
		fg, bg, attr := cell.Style.Decompose()
		fore, back := ink(fg, color.RGBA{255, 255, 255, 255}), ink(bg, color.RGBA{0, 0, 0, 255})
		if attr&tcell.AttrReverse != 0 {
			fore, back = back, fore
		}
		x, y := i%f.cols, i/f.cols
		cur := f.cursor && x == f.x && y == f.y
		for dy := 0; dy < h; dy++ {
			base := ((y*h+dy)*f.cols + x) * w * 4
			for dx := 0; dx < w; dx++ {
				p := pixels[base+4*dx : base+4*dx+4]
				p[0], p[1], p[2], p[3] = back.R, back.G, back.B, 255
				if cur && dy >= h-2 || attr&tcell.AttrUnderline != 0 && dy == h-3 {
					p[0], p[1], p[2] = fore.R, fore.G, fore.B
				}
			}
		}
		for _, r := range cell.Runes {
			if r == ' ' || r == 0 {
				continue
			}
			glyph := font[r]
			if glyph == nil {
				glyph = font['�']
			}
			for j, a := range glyph {
				if attr&tcell.AttrBold != 0 && j%w != 0 {
					a = max(a, glyph[j-1])
				}
				if a == 0 {
					continue
				}
				k := ((y*h+j/w)*f.cols*w + x*w + j%w) * 4
				p := pixels[k : k+3]
				p[0], p[1], p[2] = mix(p[0], fore.R, a), mix(p[1], fore.G, a), mix(p[2], fore.B, a)
			}
		}
	}
}
