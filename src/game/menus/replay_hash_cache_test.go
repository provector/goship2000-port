package menus

import (
	gamestate "gunship2000-assets/game/state"
	"testing"
)

func TestReplayCacheCanonical(t *testing.T) {
	c := new(replayHashCache)
	m := &gamestate.InitializedMission{Entities: []gamestate.InitializedEntity{{ID: "quote & < >"}, {ID: "other"}}}
	for _, change := range []func(){func() {}, func() { m.Entities[0].ControlRaw[42]++ }, func() { m.Entities[0], m.Entities[1] = m.Entities[1], m.Entities[0] }, func() { m.Rows = []gamestate.MissionObject{{Formation: 2}} }, func() { m.Entities = []gamestate.InitializedEntity{} }, func() { m.Entities = nil }} {
		change()
		if c.missionHash(m) != filmHash(m) {
			t.Fatal("mission did not use canonical bytes")
		}
	}
	b := make([]byte, 64000)
	for _, v := range [][]byte{nil, {}, b, b, b[1:], b[:10]} {
		for bottom := 1; bottom <= 2; bottom++ {
			if len(v) > 0 {
				v[len(v)-1]++
			}
			if got := c.referenceHash(v, bottom); got != (*replayHashCache)(nil).referenceHash(v, bottom) {
				t.Fatal("reference differs")
			}
		}
	}
}
