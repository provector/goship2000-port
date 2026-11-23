package assets

import (
	"strings"
	"testing"
)

func TestPreflightMetadataBoundsAndDefinitionOrder(t *testing.T) {
	p := &PreflightAssets{Schema: preflightMetadataSchema, Ships: map[int]ShipPresentation{}, Difficulty: make([]int16, 33), SystemOrder: make([]int, 28), SquadronNames: make([]string, 74), OutfitRanks: make([]string, 12), AwardNames: make([]string, 8), Text: map[int]string{1: "ACCEPT", 0x3d34: "NO BAY", 0x3d3b: "EMPTY"}}
	p.StationMarkers = []StationMarker{{16, [4]int{256, 0, 16, 16}}, {17, [4]int{272, 0, 16, 16}}}
	p.ObjectiveSummary = &OutfitObjectiveText{Headers: [2]string{"%cPRIMARY:%c  %s", "%cSECONDARY:%c  %s"}, RecordFormat: "%02d/%02d %s %Fs", Verbs: make([]string, 8), Abbreviations: make([][2]string, 4)}
	for key, count := range []int{3, 4, 2, 2, 2, 2, 2, 0, 3} {
		r := SettingsHelp{Key: key, Heading: "heading", Body: "body"}
		for value := 0; value < count; value++ {
			r.Choices = append(r.Choices, "choice")
			if key < 7 {
				r.Details = append(r.Details, "detail")
			}
		}
		p.SettingsHelp = append(p.SettingsHelp, r)
	}
	for i := range p.SystemOrder {
		p.SystemOrder[i] = i
	}
	for i := 0; i < 8; i++ {
		p.Ships[i] = ShipPresentation{Aircraft: i, Fits: []ShipFit{{System: 0, Quantity: 1, Station: 0, Preview: 0}}, Components: []ShipComponent{{Group: 0, Source: [4]int{0, 0, 1, 1}}}}
	}
	if err := validatePreflightAssets(p); err != nil {
		t.Fatal(err)
	}
	// The source reserved compact names are required by the flight adapter.
	delete(p.Text, 0x3d3b)
	if err := validatePreflightAssets(p); err == nil {
		t.Fatal("missing runtime sentinel accepted")
	}
	p.Text[0x3d3b] = "EMPTY"
	p.SystemOrder[3] = 0
	if err := validatePreflightAssets(p); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatal("duplicate definition IDs accepted")
	}
	p.SystemOrder[3] = 3
	s := p.Ships[0]
	s.Components[0].Source = [4]int{319, 0, 2, 1}
	p.Ships[0] = s
	if err := validatePreflightAssets(p); err == nil {
		t.Fatal("out-of-bounds source rectangle accepted")
	}
	s.Components[0].Source = [4]int{0, 0, 1, 1}
	s.Fits[0].Preview = 9
	p.Ships[0] = s
	if err := validatePreflightAssets(p); err == nil {
		t.Fatal("missing preview group accepted")
	}
}
