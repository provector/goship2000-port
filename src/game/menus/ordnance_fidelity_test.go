package menus

import (
	"bytes"
	assetdata "gunship2000-assets/game/assets"
	gamestate "gunship2000-assets/game/state"
	"os"
	"testing"
)

func TestOrdnanceHoverKeepsCommittedEquipmentAndHighlightsStation(t *testing.T) {
	for aircraft := 0; aircraft < 8; aircraft++ {
		v := ordnanceTestView(t, gamestate.RankCaptain)
		v.Working[0] = byte(aircraft)
		v.Assets.Preflight.StationMarkers = []assetdata.StationMarker{{Template: 16, Source: [4]int{256, 0, 16, 16}}, {Template: 17, Source: [4]int{272, 0, 16, 16}}}
		v.Assets.Images["POINTER.PIC"] = make([]byte, 64000)
		v.Assets.Images["POINTER.PIC"][256] = 32
		v.Assets.Images["POINTER.PIC"][272] = 33
		for i := range v.Assets.Images["TARMC_00.PIC"] {
			v.Assets.Images["TARMC_00.PIC"][i] = 7
		}
		s := v.ship()
		s.Components = []assetdata.ShipComponent{{Group: 1, Source: [4]int{0, 0, 5, 5}, Destination: [2]int{20, 20}}, {Group: 2, Source: [4]int{5, 0, 5, 5}, Destination: [2]int{30, 20}}}
		v.Assets.Preflight.Ships[aircraft] = s
		for i := range v.Assets.Images["SHIP_0"+string(rune('0'+aircraft))+".PIC"] {
			v.Assets.Images["SHIP_0"+string(rune('0'+aircraft))+".PIC"][i] = 48
		}
		for station := 0; station < 3; station++ {
			for corner := 0; corner < 2; corner++ {
				at := 14 + 8*station + 4*corner
				gamestate.RowPut(v.definition(), at, 40+station*50+corner*20)
				gamestate.RowPut(v.definition(), at+2, 50)
			}
		}
		// Empty committed stores: moving through the mount choices draws only
		// POINTER16/17 at SHHD anchors-4; it never installs the candidate COMP.
		for station := 0; station < 3; station++ {
			gamestate.RowPut(v.Working[:], 12+2*station, 0)
		}
		v.rebuild()
		ordnanceActivate(t, v, "fits", 7)
		before, live := v.Working, v.Rows[0]
		for i, c := range v.Choices {
			if c.Action != "mount" {
				continue
			}
			frame := v.FrameForHit(i)
			if frame[23*320+23] != 7 || frame[23*320+33] != 7 {
				t.Fatalf("aircraft%d hover installed candidate art", aircraft)
			}
			for corner := 0; corner < 2; corner++ {
				x := 40 + c.Station*50 + corner*20 - 4
				if frame[46*320+x] != byte(32+corner) {
					t.Fatalf("aircraft%d station%d marker%d", aircraft, c.Station, corner)
				}
			}
		}
		if v.Working != before || v.Rows[0] != live {
			t.Fatal("hover changed equipment/mass/accepted row")
		}
		ordnanceActivate(t, v, "mount", 7)
		if v.FrameForHit(-1)[23*320+23] != 48 {
			t.Fatalf("aircraft%d activation did not install committed art", aircraft)
		}
		committed := v.Working
		plain := v.FrameForHit(-1)
		for i, c := range v.Choices {
			if c.Action != "remove" {
				continue
			}
			frame := v.FrameForHit(i)
			if frame[23*320+23] != 48 {
				t.Fatal("remove hover erased committed equipment")
			}
			if bytes.Equal(frame, plain) {
				t.Fatal("remove hover omitted station markers")
			}
		}
		if v.Working != committed {
			t.Fatal("remove hover changed working state")
		}
	}
}

func TestOrdnanceAllSourceStationMarkersPreserveCommittedArt(t *testing.T) {
	dir := os.Getenv("GS2000_TEST_PREFLIGHT")
	if dir == "" {
		t.Skip("source-backed prepared data required")
	}
	a, err := assetdata.LoadMissionAssets(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Preflight.StationMarkers) != 2 {
		t.Fatal("prepared markers missing")
	}
	m := &gamestate.MissionData{SourceWorld: &a.Generation.Worlds[0], Conditions: gamestate.MissionConditions{Temperature: 68}}
	cases := 0
	for id := 0; id < 8; id++ {
		row, err := gamestate.InitializeReadinessRow(a, m, id)
		if err != nil {
			t.Fatal(err)
		}
		v, err := NewOrdnanceView(a, m, []gamestate.FlightPilot{{Rank: gamestate.RankCaptain}}, [][36]byte{row}, 0, gamestate.RankCaptain)
		if err != nil {
			t.Fatal(err)
		}
		for _, fit := range v.ship().Fits {
			if fit.Station > 2 || a.WeaponDefinitions[fit.System][1] == 0 {
				continue
			}
			v.stack = []ordnanceMenu{{Kind: "main"}, {Kind: "fits", System: fit.System}}
			v.rebuild()
			baseline := v.FrameForHit(-1)
			for index, choice := range v.Choices {
				if choice.Action != "mount" || choice.Station != fit.Station || choice.Value < 0 {
					continue
				}
				expected := append([]byte(nil), baseline...)
				for corner, marker := range a.Preflight.StationMarkers {
					at := 14 + 8*choice.Station + 4*corner
					x, y := int(int16(gamestate.RowWord(v.definition(), at))), int(int16(gamestate.RowWord(v.definition(), at+2)))
					if x == 0 {
						continue
					}
					// Independent hand copy of POINTER's nonzero pixels. Source
					// anchors already name screen positions, unlike SHIP COMP's+3.
					for dy := 0; dy < 16; dy++ {
						for dx := 0; dx < 16; dx++ {
							pixel := a.Images["POINTER.PIC"][(marker.Source[1]+dy)*320+marker.Source[0]+dx]
							if pixel != 0 {
								expected[(y-4+dy)*320+x-4+dx] = pixel
							}
						}
					}
				}
				got := v.FrameForHit(index)
				for y := 3; y < 117; y++ {
					if !bytes.Equal(got[y*320+3:y*320+222], expected[y*320+3:y*320+222]) {
						t.Fatalf("aircraft%d row%d station%d: hover differs from committed art plus source markers atY%d", id, index, choice.Station, y)
					}
				}
				cases++
			}
		}
		if v.Working != row || v.Rows[0] != row {
			t.Fatal("source hover changed accepted configuration")
		}
	}
	if cases < 178 {
		t.Fatalf("insufficient supported station hover coverage:%d", cases)
	}
}
