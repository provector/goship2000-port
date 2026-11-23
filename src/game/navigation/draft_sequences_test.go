package navigation

import (
	"math/rand"
	"strconv"
	"testing"
)

// Operate on a committed route. A transaction either publishes one valid
// order or leaves the most recent authoritative order completely intact.
func TestGeneratedWingmanEditTransactions(t *testing.T) {
	for _, seed := range []int64{106, 219, 451, 603} {
		t.Run(strconv.FormatInt(seed, 10), func(t *testing.T) {
			rng := rand.New(rand.NewSource(seed))
			g := routeFixture()
			g.Selected = 1
			owner := uint16(0x2101)
			published := 0
			g.StageWingmanRoutes(func(row int) (uint16, bool) { return owner, true })
			g.SetFollowerOrders(true, func(row int) {
				if row != 1 {
					t.Fatal("published to another aircraft", row)
				}
				published++
			})
			g.PreserveWingmanRouteEdits()
			for trial := 0; trial < 80; trial++ {
				before := g.Routes[1]
				count := published
				if e := g.StartEdit(); e != nil {
					t.Fatal(trial, e)
				}
				points := rng.Intn(4)
				for n := 0; n < points; n++ {
					g.Append()
					if n < 2 && g.Routes[1] != before {
						t.Fatal("unconfirmed order escaped")
					}
				}
				// The third point is the documented automatic confirmation.
				if points == 3 {
					if g.Editing || published != count+1 || !g.Routes[1].Point(3).Present() {
						t.Fatal(trial, "third waypoint did not confirm the route")
					}
					continue
				}
				mode := rng.Intn(4)
				switch mode {
				case 0:
					g.FinishEdit()
				case 1:
					g.CancelUnfinishedWingmanRoute()
				case 2:
					owner ^= 0x0100
					g.FinishEdit()
				case 3:
					g.Routes[1].Raw[1] ^= 8
					before = g.Routes[1]
					g.FinishEdit()
				}
				if g.Editing {
					t.Fatal("draft still active", trial)
				}
				if mode == 0 && points > 0 {
					if published != count+1 || !g.Routes[1].Point(1).Present() {
						t.Fatal(trial, "valid draft did not publish exactly once")
					}
				} else if g.Routes[1] != before || published != count {
					t.Fatal(trial, "cancel, empty or superseded order changed live state")
				}
			}
		})
	}
}
