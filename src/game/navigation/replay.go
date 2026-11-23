package navigation

// ReplayState contains modal owners that can later write route/control state.
// Panel, marker and artwork snapshots are read-only display data. Sweep is kept
// because closing GPS hands its reached phase back to the cockpit map owner.
type ReplayState struct {
	Support                            *SupportMode `json:",omitempty"`
	Low, Editing                       bool
	Selected, Moving, CursorX, CursorY int
	Center                             Point
	Backup                             Route
	Remainder                          [2]int
	Poses                              [5]Point
	Active                             [5]bool
	Base, Home, FARP                   Point
	FARPPresent                        bool
	HomeError                          string
	BlinkPhase, SweepPhase             uint16
	HeadingPerturbation                int16
	DraftOwner                         uint16 `json:",omitempty"`
	Draft                              *Route `json:",omitempty"`
	PrimaryDrain                       bool   `json:",omitempty"`
}

func (g *GPS) ReplayState() *ReplayState {
	if g == nil {
		return nil
	}
	return &ReplayState{g.Support, g.Low, g.Editing, g.Selected, g.Moving, g.CursorX, g.CursorY, g.Center, g.backup, g.overviewKeyRemainder, g.Poses, g.Active, g.Base, g.Home, g.FARP, g.FARPPresent, g.HomeError, g.BlinkPhase, g.SweepPhase, g.HeadingPerturbation, g.draftOwner, g.draft, g.primaryDrain}
}
func (g *GPS) ReplayRemainder() [2]int { return g.overviewKeyRemainder }
