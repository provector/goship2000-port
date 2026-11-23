package combat

import (
	"encoding/binary"
	"encoding/json"
	"gunship2000-assets/game/avionics"
	gamestate "gunship2000-assets/game/state"
	"gunship2000-assets/internal/testfixture"
	"os"
	"testing"
)

func groundFixture(t *testing.T) (*State, Input) {
	s, in := artilleryFixture(t)
	s.init.Support = nil
	s.SetMissionActorsEnabled(true)
	for i := range s.init.Entities {
		s.init.Entities[i].Kind = 0xff
		s.init.Entities[i].Participation = 0
	}
	if err := json.Unmarshal([]byte(`{"Paths":[{"Flag":64},{"Point":{"X":262144,"Y":262144}},{"Point":{"X":262144,"Y":264144},"Flag":64}]}`), s.init); err != nil {
		t.Fatal(err)
	}
	s.init.Offsets = map[int][][2]int16{1: {{100, 0}}}
	header := &s.init.Entities[9]
	header.Kind = 0x40
	header.Participation = 1
	header.DynamicRaw[2] = 1
	header.RawMotion = 20
	header.Pose = gamestate.InitializedPose{H0: 0x40000, H1: 0x40000}
	child := &s.init.Entities[10]
	child.Kind = 3
	child.Participation = 1
	child.DynamicRaw[2] = 0x10
	child.RawMotion = 20
	child.LeaderRow = 9
	child.Pose = gamestate.InitializedPose{H0: 0x40000 + 100, H1: 0x40000}
	return s, in
}
func TestRouteHeaderQueuedChildAndSourceOrderedTermination(t *testing.T) {
	s, in := groundFixture(t)
	draws := 0
	in.Random = func() uint16 { draws++; return 4 }
	if err := s.advanceRouteHeader(9, in); err != nil {
		t.Fatal(err)
	}
	header, child := &s.init.Entities[9], &s.init.Entities[10]
	if header.Pose.H1 != 0x40000+20 || s.route.Cursor[9] != 1 || s.route.Queued[10] != 0 || header.DynamicRaw[2] != 0x81 || draws != 0 {
		t.Fatal("source first header step", header, s.route, draws)
	}
	if err := s.advanceGroundRoute(10, in); err != nil {
		t.Fatal(err)
	}
	if child.Pose.H0 != 0x40000+96 || child.Pose.H1 != 0x40000 || header.DynamicRaw[2] != 1 || draws != 0 {
		t.Fatal("queued point uses actual DF/10 and releases header", child, draws)
	}
	moved := false
	for tick := 0; tick < 200; tick++ {
		if _, err := s.advanceLogisticsFollowers(in, nil); err != nil {
			t.Fatal(tick, err)
		}
		moved = moved || child.Pose.H1 > 0x40000+100
		if header.Participation == 4 {
			break
		}
	}
	if !moved || header.Participation != 4 || child.DynamicRaw[2] != 0 || header.Pose.H1 != 264144 || draws == 0 || draws&1 != 0 {
		t.Fatal("actual finite path and child cleanup", moved, header, child, draws)
	}
	old := child.Pose
	for i := 0; i < 20; i++ {
		if _, err := s.advanceLogisticsFollowers(in, nil); err != nil {
			t.Fatal(err)
		}
	}
	if child.Pose != old {
		t.Fatal("completed path kept moving")
	}
}
func TestGroundFormationJitterWidthAndInitializers(t *testing.T) {
	s, in := groundFixture(t)
	child := &s.init.Entities[10]
	child.Participation = 0
	if err := s.advanceGroundRoute(10, in); err != nil {
		t.Fatal(err)
	}
	if child.Pose.H0 != 0x40000+100 || child.Pose.H1 != 0x40000 || child.RawMotion != 20 || child.Participation != 1 || s.route.Queued[10] != -1 {
		t.Fatal("sourceP0 directoffset", child)
	}
	child.Pose.H0 = 0x40000
	draws := 0
	in.Random = func() uint16 {
		draws++
		if draws == 1 {
			return 7
		}
		return 0
	}
	if err := s.advanceGroundRoute(10, in); err != nil {
		t.Fatal(err)
	}
	if child.Pose.H0 != 0x40000+25 || child.Pose.H1 != 0x40000-1 || draws != 2 {
		t.Fatal("signedSAR2/two draws", child.Pose, draws)
	}
	child.Participation = 2
	child.RawMotion = 4
	if err := s.advanceGroundRoute(10, in); err != nil {
		t.Fatal(err)
	}
	if child.Participation != 3 || child.RawMotion != 4 {
		t.Fatal("ground death phase2 only", child)
	}
	for i := 0; i < 5; i++ {
		if err := s.advanceGroundRoute(10, in); err != nil {
			t.Fatal(err)
		}
	}
	if child.Kind != 0x1c || child.Participation != 0 {
		t.Fatal("shared old-motion-zero wreck rule", child)
	}
}
func TestGroundMissingReferenceGuardsAndHistoricalCheckpointOmission(t *testing.T) {
	s, in := groundFixture(t)
	s.init.Entities[10].LeaderRow = -1
	if err := s.advanceGroundRoute(10, in); err == nil {
		t.Fatal("unknown leader synthesized")
	}
	s, in = groundFixture(t)
	s.route.Queued[10] = 300
	if err := s.advanceGroundRoute(10, in); err == nil {
		t.Fatal("missing consumed PATH point synthesized")
	}
	s.SetMissionActorsEnabled(false)
	raw, _ := json.Marshal(s.FilmState())
	var fields map[string]any
	json.Unmarshal(raw, &fields)
	if _, ok := fields["Route"]; ok {
		t.Fatal("historical checkpoints acquired route state")
	}
}

func TestGroundSignedABS8000FormationHeading(t *testing.T) {
	s, in := groundFixture(t)
	e := &s.init.Entities[10]
	e.Pose.H0 = s.init.Entities[9].Pose.H0 + 100 + 131072
	e.Pose.Angles[1] = 0x8000
	in.Random = func() uint16 { return 4 }
	if err := s.advanceGroundRoute(10, in); err != nil {
		t.Fatal(err)
	}
	// BFC4 JLE/BFD7 JG compare the word ABS as signed: ABS8000 remains
	// negative and takes the average-heading arm, not SourceAngle.
	if e.Pose.Angles[1] != 0xc000 {
		t.Fatal("signed source ABS8000", e.Pose)
	}
}
func TestOriginalGroundAndRouteDispatchOperands(t *testing.T) {
	_ = sourceTables(t)
	raw, err := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if err != nil {
		t.Fatal(err)
	}
	for kind, want := range map[int]uint16{2: 0x465e, 3: 0x465e, 7: 0x465e, 0x40: 0x84d4, 0x41: 0x87f8} {
		if binary.LittleEndian.Uint16(raw[0xd8b0+0xfc6e+kind*2:]) != want {
			t.Fatal("sourceCSalias dispatch", kind)
		}
	}
}

func TestRouteTailUsesProvenActualRow80Aliases(t *testing.T) {
	for _, v := range []int16{0, 15, 16, 31, 32, 0x110, 0x11f, 0x120} {
		s, in := groundFixture(t)
		s.init.Entities[73] = s.init.Entities[9]
		s.init.Entities[73].RuntimeRow = 73
		s.init.Entities[9].Kind = 0xff
		s.init.Entities[74] = s.init.Entities[10]
		s.init.Entities[74].RuntimeRow = 74
		s.init.Entities[74].LeaderRow = 73
		s.init.Entities[10].Kind = 0xff
		header := &s.init.Entities[73]
		header.Pose.H1 = 264144
		s.route.Cursor[73] = 1
		// The original final Flag40 path clears actual DYN80+2 iff its nibble
		// matches group1. This is a word's lowbyte, not an extra entity.
		h := v
		in.HoldHeightTarget = &h
		if err := s.advanceRouteHeader(73, in); err != nil {
			t.Fatal(err)
		}
		want := v
		if byte(v)>>4 == 1 {
			want = int16(uint16(v) & 0xff00)
		}
		if h != want || len(s.init.Entities) != 80 {
			t.Fatal("actual2D04alias", v, h, want)
		}
	}
	s, in := groundFixture(t)
	s.init.Entities[73] = s.init.Entities[9]
	s.init.Entities[73].RuntimeRow = 73
	s.init.Entities[9].Kind = 0xff
	s.init.Entities[74] = s.init.Entities[10]
	s.init.Entities[74].RuntimeRow = 74
	s.init.Entities[74].LeaderRow = 73
	s.init.Entities[10].Kind = 0xff
	end := s.init.Paths[2]
	end.Point.Y += 2000
	s.init.Paths = append(s.init.Paths, end)
	s.init.Paths[2].Flag = 0
	s.init.Entities[73].Pose.H1 = 264144
	s.route.Cursor[73] = 1
	h := int16(16)
	in.HoldHeightTarget = &h
	if err := s.advanceRouteHeader(73, in); err != nil {
		t.Fatal(err)
	}
	if s.route.Altitude[0] != 1 || s.sourceFollowerAltitude(0) != 1 || s.sourceFollowerAltitude(1) != 26 || h != 16 {
		t.Fatal("source2F12[80]/2FB2actualaltitude0", s.route, h)
	}
	s.SetMissionActorsEnabled(false)
	if s.sourceFollowerAltitude(0) != 0 {
		t.Fatal("old profile acquired mutable table alias")
	}
}
func TestReachedRouteTailHistoryCandidatesAndActualRemap(t *testing.T) {
	s, _ := groundFixture(t)
	h := int16(0x1234)
	hist := [4]int16{1, 2, 0x2345, 4}
	candidates := [8]avionics.Candidate{}
	candidates[0].Angle = 0x3456
	candidates[5].Angle = 0x4567
	remap := make([]byte, 256)
	remap[2] = 2
	remap[26] = 26
	remap[50] = 50
	in := Input{HoldHeightTarget: &h, TranslationHistory0: &hist, ContactCandidates: &candidates, WeatherRemap: remap}
	for row, want := range map[int]byte{80: 0x34, 81: 0x45, 82: 0x34, 83: 0x67, 84: 2, 85: 26, 86: 50} {
		got, err := s.routeTailByte(row, in)
		if err != nil || got != want {
			t.Fatal("actual source tail owner", row, got, want, err)
		}
	}
	for _, row := range []int{80, 81, 82, 83} {
		if err := s.clearRouteTailByte(row, in); err != nil {
			t.Fatal(err)
		}
	}
	if h != 0x1200 || hist[2] != 0x2300 || candidates[0].Angle != 0x56 || candidates[5].Angle != 0x4500 {
		t.Fatal("source byte domains", h, hist, candidates)
	}
	before := append([]byte(nil), remap...)
	if err := s.clearRouteTailByte(84, in); err == nil || string(before) != string(remap) {
		t.Fatal("shared read-only media was mutated")
	}
	if _, err := s.routeTailByte(87, in); err == nil {
		t.Fatal("unknown tail owner guessed")
	}
}
func TestQueuedGroundExactEqualityBoundary(t *testing.T) {
	for _, tc := range []struct {
		offset uint32
		want   uint32
		queued int16
	}{{39, 0, -1}, {40, 36, 0}} {
		s, in := groundFixture(t)
		s.route.Queued[10] = 0
		s.init.Entities[10].Pose.H0 = 0x40000 + tc.offset
		if err := s.advanceGroundRoute(10, in); err != nil {
			t.Fatal(err)
		}
		if s.init.Entities[10].Pose.H0 != 0x40000+tc.want || s.route.Queued[10] != tc.queued {
			t.Fatal("strict unsigned motion>metric", tc, s.init.Entities[10].Pose, s.route.Queued[10])
		}
	}
}
