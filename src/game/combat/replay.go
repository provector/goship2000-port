package combat

// FilmState includes the source-owned private admission/scratch fields omitted
// by diagnostic JSON. Asset/init pointers and observer callbacks are rebound.
type FilmState struct {
	Slots                                [SlotCount]Projectile
	NextSlot, LastSlot                   uint16
	Credit                               [40]byte
	Results                              [4]Result
	Primary, Secondary                   Objective
	Score                                int16
	Updates                              uint64
	IncomingThreat, IncomingRadarHistory uint16
	IncomingAdmitted, IncomingWarning    [SlotCount]bool
	IncomingAimKnown                     bool
	IncomingAimPose                      any
	Route                                *RouteState          `json:",omitempty"`
	Avoidance                            *RouteAvoidanceState `json:",omitempty"`
	Navigation                           *NavigationState     `json:",omitempty"`
	Enemy                                *EnemyFilmState      `json:",omitempty"`
}

func (s *State) FilmState() FilmState {
	return FilmState{s.Slots, s.NextSlot, s.LastSlot, s.Credit, s.Results, s.Primary, s.Secondary, s.Score, s.Updates, s.IncomingThreat, s.IncomingRadarHistory, s.incomingAdmitted, s.incomingWarning, s.incomingAimKnown, s.incomingAimPose, s.filmRoute(), s.filmAvoidance(), s.filmNavigation(), s.filmEnemy()}
}

// appendResult keeps the recovered four-row ring and notifies the film before
// a later writer can shift an event away. The observer must not mutate gameplay.
func (s *State) appendResult(r Result) {
	copy(s.Results[:3], s.Results[1:])
	r.Sequence = s.Results[3].Sequence + 1
	s.Results[3] = r
	if s.ResultObserver != nil {
		s.ResultObserver(r)
	}
}

func (s *State) filmRoute() *RouteState {
	if !s.missionActorsEnabled {
		return nil
	}
	if s.route == nil {
		return nil
	}
	q := *s.route
	return &q
}
