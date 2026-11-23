package menus

import (
	"encoding/binary"
	"gunship2000-assets/game/flight"
	"gunship2000-assets/game/navigation"
	gamestate "gunship2000-assets/game/state"
	"reflect"
	"testing"
)

func TestPauseChordAndFreshRelease(t *testing.T) {
	for _, tc := range []struct{ p, alt, ctrl, shift, want bool }{{true, false, false, false, true}, {true, true, false, false, true}, {true, false, true, false, false}, {true, false, false, true, false}, {false, true, false, false, false}} {
		if got := pauseChord(tc.p, tc.alt, tc.ctrl, tc.shift); got != tc.want {
			t.Fatal(tc, got)
		}
	}
	var reader inputLatch
	a := reader.Read(Buttons{FlightPause: true, AnyKeyPressed: true})
	if !a.FlightPause {
		t.Fatal("plain P missing")
	}
	a = reader.Read(Buttons{FlightPause: true})
	if a.FlightPause || a.AnyKey {
		t.Fatal("held P became new resume", a)
	}
	reader.Read(Buttons{})
	if !reader.Read(Buttons{FlightPause: true}).FlightPause {
		t.Fatal("fresh P missing")
	}
}
func TestMemberCaptionSourceBoundaries(t *testing.T) {
	var r [70]byte
	binary.LittleEndian.PutUint16(r[6:], 1)
	binary.LittleEndian.PutUint16(r[8:], 3)
	for _, tc := range []struct {
		mode          int
		height        int16
		title, detail string
	}{{1, 90, "[ FORMED UP ]", ""}, {3, 90, "[ FLY TO ]", "[ alt.: NOE - speed: HIGH ]"}, {2, 2, "[ LANDED ]", ""}, {2, 3, "[ HOLDING ]", "[ alt.: NOE ]"}, {6, 90, "[ LANDING ]", ""}, {0, 90, "[ OUT OF ACTION ]", ""}, {9, 90, "", ""}} {
		before := r
		title, detail := sourceMemberCaption(tc.mode, tc.height, r)
		if title != tc.title || detail != tc.detail || !reflect.DeepEqual(before, r) {
			t.Fatal(tc, title, detail)
		}
	}
}
func TestOutfitWarningSurvivesStationaryInput(t *testing.T) {
	v := outfitTestView(t)
	v.Mission.CampaignUnavailableAircraft = []int{0, -1, -1, -1}
	result, err := v.Handle(outfitPanelAction(0, "D", false, false))
	if err != nil || result.Message != "Additional helicopter or weapon systems are unavailable." {
		t.Fatal(result, err)
	}
	v.Hover = 1
	area := v.Assets.Metadata.Screens["OUTFIT.SCN"].Areas[0].Rectangle
	for i := 0; i < 80; i++ {
		_, e := v.Handle(Actions{Inside: true, X: area[0] + 2, Y: area[1] + 2})
		if e != nil {
			t.Fatal(e)
		}
		if v.Message == "" {
			t.Fatal("warning erased by idle input", i)
		}
		v.Tick()
	}
	_, e := v.Handle(Actions{Inside: false})
	if e != nil {
		t.Fatal(e)
	}
	if v.Message != "" {
		t.Fatal("pointer redraw retained stale warning")
	}
}

func TestGPSFollowerDamageUsesPhysical600WithoutInventingMasks(t *testing.T) {
	i := &gamestate.InitializedMission{PlayerRow: 0, Entities: []gamestate.InitializedEntity{{FlightSlot: 1, Pose: gamestate.InitializedPose{Height: 40}}}, Stores: &gamestate.RuntimeStores{Ships: make([]gamestate.RuntimeAircraft, 2), Damage: [][10]byte{{}, {1, 2, 3, 0, 1, 2, 3, 0, 2, 1}}}}
	r := &FlightRuntime{State: &flight.State{}}
	g := navigation.New(&r.Navigation, [5]navigation.Point{}, [5]bool{true, true})
	g.Selected = 1
	p := &RenderingPreview{Initialized: i, Flight: r, GPS: g, r11Mechanics: true}
	p.publishGPSPanel()
	if !g.Panel.ScalarDamageKnown || g.Panel.DamageKnown || g.Panel.FuelSeverityKnown || g.Panel.Damage[0] != 0 || g.Panel.Damage[1] != 0 {
		t.Fatal("unknown5fe/5ff masks invented", g.Panel)
	}
	for j := 0; j < 8; j++ {
		if g.Panel.Damage[j+2] != i.Stores.Damage[1][j] {
			t.Fatal("physical damage shifted", j)
		}
	}
	frozen := *g.Panel
	i.Stores.Damage[1][0] = 2
	if frozen.Damage[2] != 1 {
		t.Fatal("snapshot aliased physical stores")
	}
	p.r11Mechanics = false
	p.publishGPSPanel()
	if g.Panel.ScalarDamageKnown || g.Panel.Damage != [10]byte{} {
		t.Fatal("historical panel interpretation changed")
	}
}
