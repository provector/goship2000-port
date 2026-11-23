package menus

import (
	"math/rand"
	"strconv"
	"testing"
	"time"
)

// Model elapsed time independently in integer milliseconds. Only intervals with
// both endpoints in flight belong to the live input time line.
func TestModalSequenceUsesOnlyAdmittedTime(t *testing.T) {
	for _, seed := range []int64{521, 41, 99, 2304} {
		t.Run(strconv.FormatInt(seed, 10), func(t *testing.T) {
			rng := rand.New(rand.NewSource(seed))
			var clock flightWindowClock
			var expected time.Duration
			previouslyLive := true
			for n := 0; n < 400; n++ {
				modal := rng.Intn(4) == 0
				dt := time.Duration(1+rng.Intn(59)) * time.Millisecond
				if modal {
					dt += time.Duration(rng.Intn(40)) * time.Second
				}
				before := expected
				if !modal && previouslyLive {
					expected += dt
				}
				stamp, advance := clock.frame(dt.Seconds(), modal)
				if stamp != expected || advance != (expected-before).Seconds() {
					t.Fatalf("seed %d action %d: got %v %g, want %v %v", seed, n, stamp, advance, expected, expected-before)
				}
				previouslyLive = !modal
			}
		})
	}
}
