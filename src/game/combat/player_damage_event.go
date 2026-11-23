package combat

import "fmt"

// RecordAcceptedPlayerHit is source95ea→resident0bc2 event2. The packed
// return-code nibbles are not a weapon, target identity, or destruction credit.
func (s *State) RecordAcceptedPlayerHit(victim int, packed, phase uint16) error {
	if s == nil || s.init == nil || victim != s.init.PlayerRow {
		return fmt.Errorf("damage event victim outside admitted player domain")
	}
	return s.recordAcceptedDamage(victim, packed, phase)
}

// recordAcceptedDamage is shared95EA publication after the actual synchronous
// victim consumer. Domain admission stays with the player/NPC caller above it.
func (s *State) recordAcceptedDamage(victim int, packed, phase uint16) error {
	if packed == 0 {
		return nil
	}
	s.appendResult(Result{Kind: 2, Actor: byte(victim), Target: byte(packed), Timestamp: phase})
	return nil
}
