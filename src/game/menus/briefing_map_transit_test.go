package menus

import (
	gamestate "gunship2000-assets/game/state"
	"reflect"
	"testing"
)

func TestBriefingMapBaseEdgeUsesUnroundedXCell(t *testing.T) {
	for _, tc := range []struct {
		x, y int32
		want [4]int
	}{
		{32768, 8 * 32768, [4]int{29, 65, 29, 97}},
		{2 * 32768, 32768, [4]int{19, 137, 59, 137}},
		{14 * 32768, 32768, [4]int{139, 137, 179, 137}},
		{15 * 32768, 8 * 32768, [4]int{169, 65, 169, 97}},
		// 65535 truncates to cell1; using rounded Orders grids would give2.
		{65535, 8 * 32768, [4]int{39, 65, 39, 97}},
	} {
		p := gamestate.MissionPoint{tc.x, tc.y}
		got, ok := briefingMapEdgeSegment(p, 0x800)
		if !ok || got != tc.want {
			t.Fatalf("point%+v: got%v/%t want%v", p, got, ok, tc.want)
		}
		if _, ok := briefingMapEdgeSegment(p, 0x200); ok {
			t.Fatal("ordinary weather flag drew a corridor line")
		}
	}
}

func TestBriefingMapArrowUsesSourceMetricAndHeadConversion(t *testing.T) {
	for _, tc := range []struct {
		points [4]int
		want   [][4]int
	}{
		// D=40: the shaft starts4 and ends16 pixels from the marker.
		{[4]int{40, 50, 80, 50}, [][4]int{{44, 50, 56, 50}, {50, 52, 56, 50}, {50, 47, 56, 50}}},
		// Upward arms include a negative fractional offset before truncation.
		{[4]int{20, 30, 20, 50}, [][4]int{{20, 34, 20, 46}, {17, 40, 20, 46}, {22, 40, 20, 46}}},
		// D=max(5,3)+trunc(3/2)=6, rather than floor(sqrt(34))=5.
		{[4]int{40, 50, 45, 53}, [][4]int{{43, 52, 53, 58}, {47, 57, 53, 58}, {49, 52, 53, 58}}},
	} {
		p := tc.points
		if got := briefingMapArrowSegments(p[0], p[1], p[2], p[3]); !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("arrow%v: got%v want%v", p, got, tc.want)
		}
	}
	if got := briefingMapArrowSegments(4, 4, 4, 4); len(got) != 0 {
		t.Fatal("defensive zero-vector policy did not suppress degenerate arrow")
	}
	if got := briefingMapArrowSegments(0, 0, -32768, 0); len(got) != 0 {
		t.Fatal("defensive signed-abs overflow policy missing")
	}
}
