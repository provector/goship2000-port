package combat

import (
	"slices"

	gamestate "gunship2000-assets/game/state"
)

// CopyForMission binds the combat controllers to the copied actors/stores.
// The city height lookup and prepared world geometry are read-only after launch.
func (s *State) CopyForMission(m *gamestate.InitializedMission) *State {
	v := *s
	v.init = m
	v.Events = slices.Clone(s.Events)
	v.ResultObserver = nil
	if s.route != nil {
		r := *s.route
		v.route = &r
	}
	if s.stationaryEnemy != nil {
		e := *s.stationaryEnemy
		v.stationaryEnemy = &e
	}
	return &v
}
