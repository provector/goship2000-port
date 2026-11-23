package menus

import (
	assetdata "gunship2000-assets/game/assets"
)

// Pacing is a port policy, not the slowed reference recording's duration.
// The shared elapsed-time accumulator supplies logical ticks, never host frames.
const (
	paperScrollPixelsPerStep = 12
	paperScrollHoldTicks     = 1
	paperViewHeight          = 191
	paperPanelWidth          = 227
	paperHeaderLead          = 20
)

type paperMotion uint8

const (
	paperOpening paperMotion = iota
	paperNext
	paperClosing
)

type paperTransition struct {
	Motion       paperMotion
	From, To     []byte
	Offset, Hold int
}

func (t *paperTransition) extent() int {
	if t.Motion == paperNext {
		return paperPanelWidth
	}
	return paperViewHeight
}

func (c *FrontEnd) beginPaperTransition(kind paperMotion) {
	s := c.Mission
	v := s.View
	t := &paperTransition{Motion: kind}
	switch kind {
	case paperOpening:
		t.From = v.FrameForHit(2)
		v.EnterPapers(s.Page)
		t.To = v.paperFrame(s.Page, false)
	case paperNext:
		// Exclude the hand from both cached sheets, then restore it after the
		// clipped scroll. The right artwork, footer and cursor do not scroll.
		t.From = v.paperFrame(s.Page, false)
		s.Page = (s.Page + 1) % 4
		v.SetPaper(s.Page)
		t.To = v.paperFrame(s.Page, false)
	case paperClosing:
		t.From = v.paperFrame(s.Page, false)
		v.ResumeScene()
		t.To = v.FrameForHit(-1)
	}
	s.PaperTransition = t
	c.Screen = ScreenPaperTransition
	c.Message = ""
}

func (c *FrontEnd) tickPaperTransition() {
	t := c.Mission.PaperTransition
	if t.Hold > 0 {
		t.Hold--
		return
	}
	t.Offset = min(t.Offset+paperScrollPixelsPerStep, t.extent())
	t.Hold = paperScrollHoldTicks - 1
	if t.Offset == t.extent() {
		c.Screen = ScreenPapers
		if t.Motion == paperClosing {
			c.Screen = ScreenBriefing
		}
		c.Mission.PaperTransition = nil
	}
}

func (t *paperTransition) frame() []byte {
	f := append([]byte(nil), t.From...)
	p := t.Offset
	if p == 0 {
		return f
	}
	scene, paper, down := t.From, t.To, p
	if t.Motion == paperClosing {
		scene, paper, down = t.To, t.From, paperViewHeight-p
	}
	for y := 0; y < paperViewHeight; y++ {
		for x := 0; x < 320; x++ {
			at := y*320 + x
			switch t.Motion {
			case paperOpening, paperClosing:
				if down == 0 {
					f[at] = scene[at]
					continue
				}
				origin, scroll := paperViewHeight-down, down
				if x < paperPanelWidth {
					// The briefing caption is already the paper's leading strip.
					// Join it to the new sheet instead of scrolling two headings.
					// P1783 also skips20 source rows; the video shows this overlap.
					scroll = min(down, paperViewHeight-paperHeaderLead)
					origin = paperViewHeight - paperHeaderLead - scroll
				}
				if y < origin {
					f[at] = scene[(y+scroll)*320+x]
				} else {
					f[at] = paper[(y-origin)*320+x]
				}
			case paperNext:
				if x >= paperPanelWidth {
					continue
				}
				if x+p < paperPanelWidth {
					f[at] = t.From[y*320+x+p]
				} else {
					f[at] = t.To[y*320+x+p-paperPanelWidth]
				}
			}
		}
	}
	return f
}

func (c *FrontEnd) paperTransitionFrame() []byte {
	t := c.Mission.PaperTransition
	f := t.frame()
	handOffset := 0
	if t.Motion == paperOpening {
		handOffset = paperViewHeight - t.Offset
	}
	if t.Motion == paperClosing {
		handOffset = t.Offset
	}
	c.Mission.View.drawPaperHandOffset(f, handOffset)
	assetdata.OfficeFill(f, 0, paperViewHeight, 320, 200-paperViewHeight, 0)
	caption, colour := "PLEASE STAND BY", 10
	if t.Motion == paperNext {
		caption, colour = "VIEW NEXT PAGE", 7
	}
	assetdata.DrawBottomTooltip(f, c.Mission.View.captionFont(), caption, colour)
	return f
}
