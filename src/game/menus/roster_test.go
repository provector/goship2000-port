package menus

import (
	gamestate "gunship2000-assets/game/state"
	"testing"
)

func TestTypedLossRosterUsesOriginalStatusNames(t *testing.T) {
	for _, c := range []struct {
		s    gamestate.PilotStatus
		want string
	}{{gamestate.StatusActive, "Active"}, {gamestate.StatusRetired, "Retired"}, {gamestate.StatusMIA, "MIA"}, {gamestate.StatusKIA, "KIA"}} {
		if got := rosterStatusLabel(&gamestate.Pilot{Status: c.s}); got != c.want {
			t.Fatal(c, got)
		}
	}
}
