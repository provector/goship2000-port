package cockpit

import "fmt"

// These are prepared drawing recipes, not simulated subsystem state. The
// application owns initialization pacing, readiness and confirmation input.
type PresentationLine struct {
	Text   string
	X, Y   int
	Colour byte
	Style  uint16
}

type ScenePrompt struct {
	Text                                   string
	Foreground, Background                 byte
	CenterX, TextY                         int
	LeftPadding, RightPadding, Top, Bottom int
	Style, MeasureStyle                    uint16
}

type ScenePresentation struct {
	Initialization    []PresentationLine
	Quit, End         ScenePrompt
	ConfirmationRemap []byte
}

func (p ScenePresentation) Validate() error {
	if len(p.Initialization) != 7 {
		return fmt.Errorf("scene presentation requires seven initialization lines")
	}
	if len(p.ConfirmationRemap) != 256 {
		return fmt.Errorf("scene presentation requires selected-VGA confirmation remap")
	}
	for _, line := range p.Initialization {
		if line.Text == "" || line.X < 0 || line.X >= 320 || line.Y < 0 || line.Y+6 > 200 {
			return fmt.Errorf("invalid scene initialization line")
		}
	}
	for _, prompt := range []ScenePrompt{p.Quit, p.End} {
		if prompt.Text == "" || prompt.Top < 0 || prompt.Bottom >= 200 || prompt.Top > prompt.TextY || prompt.TextY+6 > prompt.Bottom+1 || prompt.CenterX < 0 || prompt.CenterX >= 320 || prompt.LeftPadding < 0 || prompt.RightPadding < 0 {
			return fmt.Errorf("invalid prepared scene confirmation")
		}
	}
	return nil
}

func DrawInitialization(dst []byte, a *Assets, visibleLines int) error {
	if a == nil || !LogicalTarget(dst) {
		return fmt.Errorf("initialization needs a prepared indexed canvas")
	}
	if err := a.ScenePresentation.Validate(); err != nil {
		return err
	}
	if visibleLines < 0 || visibleLines > len(a.ScenePresentation.Initialization) {
		return fmt.Errorf("initialization visible-line count outside prepared sequence")
	}
	for _, line := range a.ScenePresentation.Initialization[:visibleLines] {
		if err := a.Text(dst, line.Text, line.X, line.Y, line.Colour, line.Style, Full); err != nil {
			return err
		}
	}
	return nil
}

func DrawSceneConfirmation(dst []byte, a *Assets, endMission bool) error {
	if a == nil || !LogicalTarget(dst) {
		return fmt.Errorf("scene confirmation needs a prepared indexed canvas")
	}
	if err := a.ScenePresentation.Validate(); err != nil {
		return err
	}
	p := a.ScenePresentation.Quit
	if endMission {
		p = a.ScenePresentation.End
	}
	bounds, err := SceneConfirmationBounds(a, endMission)
	if err != nil {
		return err
	}
	half := p.CenterX - bounds.Left - p.LeftPadding
	// Selected VGA19a1->0e34 preserves destination detail through the source
	// brightness lookup, then adds E8h. It is not a flat gray rectangle.
	for y := bounds.Top; y <= bounds.Bottom; y++ {
		for x := bounds.Left; x <= bounds.Right; x++ {
			at := y*320 + x
			PutLogical(dst, at, a.ScenePresentation.ConfirmationRemap[dst[at]]+p.Background)
		}
	}
	return a.Text(dst, p.Text, p.CenterX-half, p.TextY, p.Foreground, p.Style, Full)
}

func SceneConfirmationBounds(a *Assets, endMission bool) (Rect, error) {
	if a == nil {
		return Rect{}, fmt.Errorf("scene confirmation assets missing")
	}
	if err := a.ScenePresentation.Validate(); err != nil {
		return Rect{}, err
	}
	p := a.ScenePresentation.Quit
	if endMission {
		p = a.ScenePresentation.End
	}
	width, err := a.textWidth(p.Text, p.MeasureStyle)
	if err != nil {
		return Rect{}, err
	}
	// The source measures style02, shifts right1, then pads inclusive bounds.
	half := width >> 1
	b := Rect{p.CenterX - half - p.LeftPadding, p.Top, p.CenterX + half + p.RightPadding, p.Bottom}
	if b.Left < 0 || b.Right >= 320 {
		return Rect{}, fmt.Errorf("scene confirmation text exceeds prepared canvas")
	}
	return b, nil
}
