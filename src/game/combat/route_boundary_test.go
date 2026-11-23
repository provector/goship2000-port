package combat

import (
	"encoding/json"
	"gunship2000-assets/game/avionics"
	"reflect"
	"strings"
	"testing"
)

func lastGroundGroup(t *testing.T, final bool) (*State, Input) {
	t.Helper()
	s, in := groundFixture(t)
	// A supported population can truncate a group to header78/child79.
	// Group numbering begins at2; retain the ordinary first group delimiter.
	paths := `{"Paths":[{"Flag":64},{"Point":{"X":262144,"Y":262144}},{"Point":{"X":262144,"Y":264144},"Flag":64},{"Point":{"X":262144,"Y":262144}},{"Point":{"X":262144,"Y":264144},"Flag":64}]}`
	if !final {
		paths = `{"Paths":[{"Flag":64},{"Point":{"X":262144,"Y":262144}},{"Point":{"X":262144,"Y":264144},"Flag":64},{"Point":{"X":262144,"Y":262144}},{"Point":{"X":262144,"Y":264144}},{"Point":{"X":262144,"Y":266144},"Flag":64}]}`
	}
	if err := json.Unmarshal([]byte(paths), s.init); err != nil {
		t.Fatal(err)
	}
	s.init.Entities[78], s.init.Entities[79] = s.init.Entities[9], s.init.Entities[10]
	s.init.Entities[9].Kind, s.init.Entities[10].Kind = 0xff, 0xff
	s.init.Entities[78].RuntimeRow, s.init.Entities[79].RuntimeRow = 78, 79
	s.init.Entities[78].DynamicRaw[2] = 2
	s.init.Entities[79].DynamicRaw[2], s.init.Entities[79].LeaderRow = 0x20, 78
	s.init.Entities[78].Pose.H1 = 264144
	s.route.Cursor[78] = 1
	return s, in
}

func TestRouteHeaderStopsAtActualActorPool(t *testing.T) {
	for _, final := range []bool{true, false} {
		for _, safe := range []bool{true, false} {
			s, in := lastGroundGroup(t, final)
			s.init.SafeGameplay = safe
			height := int16(0x120) // A valid group2 match in the old DATA272D row80 alias.
			history := [4]int16{1, 2, 0x120, 4}
			candidates := [8]avionics.Candidate{}
			candidates[0].Angle, candidates[5].Angle = 0x2034, 0x1220
			remap := make([]byte, 256)
			remap[2], remap[26] = 2, 26 // Actual source nibbles cannot match group2.
			in.HoldHeightTarget, in.TranslationHistory0, in.ContactCandidates, in.WeatherRemap = &height, &history, &candidates, remap
			draws := 0
			in.Random = func() uint16 { draws++; return 4 }
			altitude, speed := s.route.Altitude, s.route.Speed
			if err := s.advanceRouteHeader(78, in); err != nil {
				t.Fatal(err)
			}
			if draws != 0 || len(s.init.Entities) != 80 {
				t.Fatal("boundary changed RNG or invented actors")
			}
			if final && s.init.Entities[79].DynamicRaw[2] != 0 || !final && s.route.Queued[79] != 1 {
				t.Fatal("last actual child79 lost its original transition")
			}
			if safe {
				if height != 0x120 || history[2] != 0x120 || candidates[0].Angle != 0x2034 || candidates[5].Angle != 0x1220 || s.route.Altitude != altitude || s.route.Speed != speed {
					t.Fatal("out-of-pool aliases changed unrelated typed owners")
				}
			} else if final && (height != 0x100 || history[2] != 0x100 || candidates[0].Angle != 0x34 || candidates[5].Angle != 0x1200) {
				t.Fatal("historical final-point alias interpretation changed")
			} else if !final && s.route.Altitude[0] != 1 {
				t.Fatal("historical queued-height alias interpretation changed")
			}
		}
	}
}

func TestRouteHeaderAvoidsSuppliedWeatherAliasGuard(t *testing.T) {
	for _, safe := range []bool{false, true} {
		s, in := lastGroundGroup(t, true)
		s.init.SafeGameplay = safe
		height := int16(0)
		history := [4]int16{}
		candidates := [8]avionics.Candidate{}
		remap := make([]byte, 256)
		remap[2] = 0x20 // Supplied diagnostic state; canonical remap[2] is2.
		in.HoldHeightTarget, in.TranslationHistory0, in.ContactCandidates, in.WeatherRemap = &height, &history, &candidates, remap
		err := s.advanceRouteHeader(78, in)
		if safe && err != nil || !safe && (err == nil || !strings.Contains(err.Error(), "row84")) {
			t.Fatal("incorrect old/new weather-tail outcome", safe, err)
		}
		if remap[2] != 0x20 || s.init.Entities[79].DynamicRaw[2] != 0 {
			t.Fatal("actual child or shared weather media changed")
		}
	}
}

func TestRouteHeaderNormalMovementAndRNGUnchanged(t *testing.T) {
	legacy, oldInput := groundFixture(t)
	safe, newInput := groundFixture(t)
	safe.init.SafeGameplay = true
	oldDraws, newDraws := 0, 0
	oldInput.Random = func() uint16 { oldDraws++; return 4 }
	newInput.Random = func() uint16 { newDraws++; return 4 }
	for tick := 0; tick < 200; tick++ {
		for _, pair := range []struct {
			s  *State
			in Input
		}{{legacy, oldInput}, {safe, newInput}} {
			if err := pair.s.advanceRouteHeader(9, pair.in); err != nil {
				t.Fatal(tick, err)
			}
			if err := pair.s.advanceGroundRoute(10, pair.in); err != nil {
				t.Fatal(tick, err)
			}
		}
		if !reflect.DeepEqual(legacy.init.Entities, safe.init.Entities) || !reflect.DeepEqual(legacy.route, safe.route) || oldDraws != newDraws {
			t.Fatal("ordinary trajectory/queue/RNG changed", tick)
		}
	}
}
