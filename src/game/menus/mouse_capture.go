package menus

import "math"

// mouseCapture owns native lock epochs and inverse viewport scaling. Controllers
// receive one sample; constrained cursors consume DX/DY without a free-pointer
// clamp in between. No backend cursor position is ever written by the app.
type mouseCapture struct {
	captured, blockButtons, blockF10, discard bool
	left, right, focused                      bool
	viewport                                  Viewport
	epoch                                     string
	x, y                                      int
	xRemainder, yRemainder                    float64
	positioned                                bool
	everCaptured                              bool
}

type mouseCaptureSample struct {
	Enabled, Focused, Minimized, Client bool
	Left, Right, CtrlF10, F10           bool
	AbsoluteX, AbsoluteY                int
	AbsoluteInside                      bool
	DeltaX, DeltaY                      float64
	Viewport                            Viewport
	LogicalHeight                       int    // zero preserves the ordinary191-pixel domain
	Epoch                               string // screen/modal changes invalidate only movement/held anchors
}

type mouseCaptureResult struct {
	Captured, Acquire, Release, Reset bool
	SuppressButtons, SuppressF10      bool
	Relative, Inside                  bool
	X, Y, DX, DY                      int
}

func (p *mouseCapture) Read(s mouseCaptureSample) mouseCaptureResult {
	r := mouseCaptureResult{}
	focused := s.Focused && !s.Minimized
	changed := s.Viewport != p.viewport || s.Epoch != p.epoch || focused != p.focused
	p.viewport, p.epoch, p.focused = s.Viewport, s.Epoch, focused
	if !p.positioned {
		p.x, p.y, p.positioned = 160, 95, true
	}
	if !p.captured && !(s.Enabled && p.everCaptured) && s.AbsoluteInside && focused {
		p.x, p.y = s.AbsoluteX, s.AbsoluteY
	}
	if !s.F10 {
		p.blockF10 = false
	}
	if s.CtrlF10 {
		p.blockF10 = true
	}
	if changed {
		p.discard = true
		p.xRemainder, p.yRemainder = 0, 0
		p.blockButtons = (s.Left || s.Right) && (p.left || p.right)
		r.Reset = true
	}
	if p.captured && (!focused || !s.Enabled || p.blockF10) {
		p.captured, r.Release, p.discard, r.Reset = false, true, true, true
		p.xRemainder, p.yRemainder = 0, 0
		p.blockButtons = s.Left || s.Right
	}
	pressed := (s.Left && !p.left) || (s.Right && !p.right)
	if s.Enabled && focused && s.Client && !p.captured && !p.blockF10 && pressed && !p.blockButtons {
		p.captured, r.Acquire, r.Reset, p.discard, p.blockButtons = true, true, true, true, true
		p.everCaptured = true
		p.xRemainder, p.yRemainder = 0, 0
	}
	if !s.Left && !s.Right {
		p.blockButtons = false
	}
	r.Captured, r.SuppressButtons, r.SuppressF10 = p.captured, p.blockButtons, p.blockF10
	r.X, r.Y = p.x, p.y
	if p.captured {
		r.Relative, r.Inside = true, true
		// DisableCursor/EnableCursor may recenter internally. Discard the first
		// post-transition backend sample as well as the transition's old sample.
		if !r.Acquire && !changed && !p.discard && s.Viewport.Width > 0 && s.Viewport.Height > 0 {
			dx := s.DeltaX*320/s.Viewport.Width + p.xRemainder
			dy := s.DeltaY*200/s.Viewport.Height + p.yRemainder
			r.DX, r.DY = int(math.Trunc(dx)), int(math.Trunc(dy))
			p.xRemainder, p.yRemainder = dx-float64(r.DX), dy-float64(r.DY)
			p.x = max(0, min(319, p.x+r.DX))
			height := s.LogicalHeight
			if height == 0 {
				height = 191
			}
			p.y = max(0, min(height-1, p.y+r.DY))
			r.X, r.Y = p.x, p.y
		} else if !r.Acquire {
			p.discard = false
		}
	} else {
		r.Inside = focused && s.AbsoluteInside
		if s.Enabled && p.everCaptured {
			// EnableCursor recenters internally. Released click-policy input must
			// not feed that position into a constrained cursor or a future capture.
			// The visible host cursor moves freely; logical menus wait for capture.
			r.Relative, r.Inside = true, focused
		}
		if !focused {
			r.SuppressButtons = true
		}
	}
	p.left, p.right = s.Left, s.Right
	return r
}
