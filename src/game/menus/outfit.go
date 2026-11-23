package menus

import (
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	gamestate "gunship2000-assets/game/state"
	"strings"
)

// A22e5e changes the outline every fourth callback. Mapping those callbacks to
// the shared logical tick is a port cadence approximation, not measured DOS time.
const outfitTransferBlinkHalfTicks = 4

type OutfitAction struct {
	Kind     string // none, ordnance, return
	Index    int
	Message  string
	AudioCue uint16 // Exactly one source OUTFIT action/rejection cue.
}

// OUTFIT edits the live preparation arrays. Escape cancels only a pending
// transfer; original RETURN and ORDNANCE ACCEPT are separate actions.
type OutfitView struct {
	Assets          *assetdata.MissionAssets
	Mission         *gamestate.MissionData
	Flight          []gamestate.FlightPilot
	Rows            [][36]byte
	Icons           []byte
	Selected, Hover int
	Source          int
	Mode            string
	Duplicate       bool
	Message         string
	blinkCounter    uint8
	blinkWhite      bool
}

func NewOutfitView(a *assetdata.MissionAssets, m *gamestate.MissionData, flight []gamestate.FlightPilot, rows [][36]byte) (*OutfitView, error) {
	if a == nil || a.Preflight == nil || m == nil || len(flight) != 5 || len(rows) != 5 {
		return nil, fmt.Errorf("OUTFIT requires five prepared aircraft and pilot rows")
	}
	if len(a.Metadata.Screens["OUTFIT.SCN"].Areas) == 0 || len(a.Images["POINTER.PIC"]) != 64000 || len(a.Images["PILOT.PIC"]) != 64000 || len(a.Images["MENU3037.PIC"]) != 64000 {
		return nil, fmt.Errorf("OUTFIT presentation assets are incomplete")
	}
	if err := gamestate.RefreshFlightPresentation(flight, a, m); err != nil {
		return nil, err
	}
	icons := a.Images["MENU3037.PIC"]
	if m.Scene == 4 || m.Scene == 5 {
		icons = append([]byte(nil), icons...)
		for i := range icons {
			icons[i] = briefingIceIndex(icons[i])
		}
	}
	return &OutfitView{Assets: a, Mission: m, Flight: flight, Rows: rows, Icons: icons, Hover: 1}, nil
}

// Tick changes presentation only. Rebuilding Frame restores ordinary group
// outlines, replacing the source callback's save-under/restore drawing globals.
func (v *OutfitView) Tick() {
	if v.Mode == "" {
		return
	}
	v.blinkCounter++
	if v.blinkCounter%outfitTransferBlinkHalfTicks == 0 {
		v.blinkWhite = (v.blinkCounter/outfitTransferBlinkHalfTicks)%2 == 0
	}
}

func (v *OutfitView) transferRectangles() [][4]int {
	if v.Mode == "" || v.Source < 0 || v.Source >= len(v.Flight) {
		return nil
	}
	minimum := 0
	if v.Mode == "pilot" {
		minimum = 1
	}
	panels := []int{v.Source}
	if v.Selected >= minimum && v.Selected < len(v.Flight) && v.Selected != v.Source {
		panels = append(panels, v.Selected)
	}
	var rectangles [][4]int
	for _, panel := range panels {
		for _, area := range v.Assets.Metadata.Screens["OUTFIT.SCN"].Areas {
			if area.Key != panel+1 {
				continue
			}
			dx, w := 41, 62
			if v.Mode == "pilot" {
				dx, w = 3, 37
			}
			rectangles = append(rectangles, [4]int{area.Rectangle[0] + dx, area.Rectangle[1] + 11, w, 32})
			break
		}
	}
	return rectangles
}

func (v *OutfitView) Hit(x, y int) int {
	areas := v.Assets.Metadata.Screens["OUTFIT.SCN"].Areas
	for i := len(areas) - 1; i >= 0; i-- {
		if areas[i].Key >= 1 && areas[i].Key <= 6 && insideRect(x, y, areas[i].Rectangle) {
			return areas[i].Key
		}
	}
	return -1
}

func (v *OutfitView) Handle(a Actions) (OutfitAction, error) {
	result := OutfitAction{Kind: "none", Index: v.Selected}
	previousHover := v.Hover
	v.Hover = -1
	if a.Inside {
		v.Hover = v.Hit(a.X, a.Y)
	}
	if v.Hover >= 1 && v.Hover <= 5 {
		v.Selected = v.Hover - 1
	}
	if previousHover != v.Hover || a.Cancel {
		v.Message = ""
	}
	if a.Cancel {
		if v.Mode == "" {
			result.Kind = "return"
		}
		v.Mode = ""
		result.AudioCue = gs4Accepted
		return result, nil
	}
	command := ""
	for _, r := range a.Text {
		if strings.ContainsRune("PpHhDdLl", r) {
			command = strings.ToUpper(string(r))
			break
		}
	}
	if command != "" || a.Selector1 || a.Selector2 {
		v.Message = ""
	}
	switch command {
	case "P":
		if v.Hover < 2 || v.Hover > 5 {
			result.AudioCue = gs4Denied
			return result, nil
		}
		result.AudioCue = gs4Accepted
		if v.Mode == "pilot" {
			return result, v.apply()
		}
		v.Source, v.Mode, v.Duplicate = v.Selected, "pilot", false
	case "H", "D":
		if v.Hover < 1 || v.Hover > 5 {
			result.AudioCue = gs4Denied
			return result, nil
		}
		if command == "D" && !(v.Mode == "helicopter" && v.Duplicate) && v.duplicateDenied(v.Selected) {
			v.Message = "Additional helicopter or weapon systems are unavailable."
			result.Message = v.Message
			result.AudioCue = gs4Denied
			return result, nil
		}
		result.AudioCue = gs4Accepted
		if v.Mode == "helicopter" {
			return result, v.apply()
		}
		v.Source, v.Mode, v.Duplicate = v.Selected, "helicopter", command == "D"
	case "L":
		if v.Selected != 0 && v.Selected != 2 && v.Selected != 3 {
			result.AudioCue = gs4Denied
			return result, nil
		}
		result.AudioCue = gs4Accepted
		for i := range v.Flight {
			v.Flight[i].Leader = i == 0 || i == v.Selected
			v.Flight[i].Group = gamestate.BoolInt(v.Selected > i) ^ gamestate.BoolInt(v.Selected == 3)
		}
		return result, gamestate.RefreshFlightPresentation(v.Flight, v.Assets, v.Mission)
	}
	if command != "" {
		return result, nil // One source-style event per update; do not also activate a panel.
	}
	if !a.Selector1 || !a.Inside {
		return result, nil
	}
	if v.Hover == 6 {
		v.Mode = "" // Leaving the local screen also ends its transfer feedback.
		return OutfitAction{Kind: "return", AudioCue: gs4Accepted}, nil
	}
	if v.Hover < 1 || v.Hover > 5 || v.Mode == "pilot" && v.Selected == 0 {
		result.AudioCue = gs4Denied
		return result, nil
	}
	if v.Mode != "" {
		result.AudioCue = gs4Accepted
		return result, v.apply()
	}
	return OutfitAction{Kind: "ordnance", Index: v.Selected, AudioCue: gs4Accepted}, nil
}

func (v *OutfitView) apply() error {
	source, destination := v.Source, v.Selected
	if source == destination {
		v.Mode = ""
		return nil
	}
	if v.Mode == "pilot" {
		if source < 1 || source > 4 || destination < 1 || destination > 4 {
			return fmt.Errorf("pilot transfer excludes the primary slot")
		}
		// Assignment/group/callsign belong to the slot. The broad stored-record
		// swap retains each slot's opaque final words, matching A21e38..21eac.
		left, right := v.Flight[source], v.Flight[destination]
		copyMember := func(to *gamestate.FlightPilot, from gamestate.FlightPilot) {
			last := append([]byte(nil), to.StoredRaw[37:41]...)
			to.Name, to.Rank, to.Portrait, to.Incomplete = from.Name, from.Rank, from.Portrait, from.Incomplete
			to.StoredRaw = from.StoredRaw
			to.AwardCounts = append([]int8(nil), from.AwardCounts...)
			copy(to.StoredRaw[37:41], last)
		}
		copyMember(&v.Flight[source], right)
		copyMember(&v.Flight[destination], left)
	} else {
		saved := v.Rows[source]
		if !v.Duplicate {
			v.Rows[source] = v.Rows[destination]
		}
		v.Rows[destination] = saved
	}
	v.Mode, v.Duplicate = "", false
	return gamestate.RefreshFlightPresentation(v.Flight, v.Assets, v.Mission)
}

func (v *OutfitView) duplicateDenied(slot int) bool {
	r := v.Rows[slot][:]
	// Source duplication checks three aircraft restrictions, whereas the
	// CHANGE HELICOPTER menu scans all four stored entries.
	for i, id := range v.Mission.CampaignUnavailableAircraft {
		if i < 3 && id == int(int8(r[0])) {
			return true
		}
	}
	for station := 0; station < 3; station++ {
		if gamestate.RowWord(r, 12+2*station) <= 0 {
			continue
		}
		id, index := int(int8(r[3+station])), -1
		for i, candidate := range v.Assets.Preflight.SystemOrder {
			if candidate == id {
				index = i
			}
		}
		if preflightDenied(v.Mission.CampaignUnavailableSystems, index) {
			return true
		}
	}
	return false
}

func (v *OutfitView) Frame() []byte {
	frame := make([]byte, 64000)
	for i := range v.Flight {
		var area assetdata.OfficeArea
		for _, candidate := range v.Assets.Metadata.Screens["OUTFIT.SCN"].Areas {
			if candidate.Key == i+1 {
				area = candidate
			}
		}
		x, y, w, h := area.Rectangle[0], area.Rectangle[1], area.Rectangle[2], area.Rectangle[3]
		// Adjacent AREA rectangles include the shared divider: the source frame
		// is one pixel narrower/shorter except at the right/bottom edges.
		fw, fh := w-gamestate.BoolInt(i%3 < 2), h-gamestate.BoolInt(i < 3)
		drawPreflightFrame(v.Assets, frame, [4]int{x, y, fw, fh}, 0, 0)
		colour := 7
		if v.Flight[i].Group == 0 {
			colour = 9
		}
		assetdata.OfficeFill(frame, x+3, y+3, w-6, 7, byte(colour))
		clip := [4]int{x + 3, y + 2, w - 6, h - 3}
		preflightText(v.Assets, frame, 3, "CALLSIGN: "+strings.ToUpper(v.Flight[i].Callsign), x+5, y+4, 15, clip)
		portrait := v.Flight[i].Portrait
		_ = assetdata.OfficeBlit(v.Assets.Images["PILOT.PIC"], frame, [4]int{5 + (portrait/6)*48, (portrait % 6) * 32, 37, 32}, x+3, y+11, false)
		id := int(int8(v.Rows[i][0]))
		_ = assetdata.OfficeBlit(v.Icons, frame, [4]int{(2*id/8)*80 + 9, (2 * id % 8) * 25, 62, 32}, x+41, y+11, false)
		drawPreflightOutline(frame, [4]int{x + 3, y + 11, 37, 32}, byte(colour))
		drawPreflightOutline(frame, [4]int{x + 41, y + 11, 62, 32}, byte(colour))
		label := assetdata.OfficeCString(v.Flight[i].Raw[4:30])
		// Original A224A4..A224ED formats short rank and name only. The
		// awards formatter belongs to the separate squadron/roster owner.
		preflightText(v.Assets, frame, 3, label, x+4, y+44, 10, clip)
		section := "HEAVY SECTION"
		if v.Flight[i].Group != 0 {
			section = "LIGHT SECTION"
		}
		if v.Flight[i].Leader {
			section += " LEADER"
		}
		preflightText(v.Assets, frame, 3, section, x+4, y+50, 10, clip)
		definition := v.Assets.AircraftDefinitions[id]
		preflightText(v.Assets, frame, 3, preflightCString(definition, 38), x+4, y+56, 10, clip)
		fuel := gamestate.PreflightRatio(gamestate.RowWord(v.Rows[i][:], 29), gamestate.RowWord(v.Rows[i][:], 27))
		load := gamestate.PreflightRatio(gamestate.RowWord(v.Rows[i][:], 25), gamestate.RowWord(v.Rows[i][:], 23))
		preflightText(v.Assets, frame, 3, fmt.Sprintf("FUEL: %d%%  LOAD: %d%%", fuel, load), x+4, y+62, 10, clip)
		line := y + 68
		for _, entry := range outfitStores(v.Assets, v.Rows[i]) {
			preflightText(v.Assets, frame, 3, entry, x+4, line, 10, clip)
			line += 6
		}
	}
	drawPreflightFrame(v.Assets, frame, [4]int{214, 96, 106, 31}, 0, 0)
	drawPreflightFrame(v.Assets, frame, [4]int{214, 128, 106, 63}, 0, 1)
	drawPreflightFrame(v.Assets, frame, [4]int{225, 177, 82, 10}, 1, 0)
	preflightText(v.Assets, frame, 3, "RETURN TO BRIEFING", 226, 179, 12, [4]int{225, 177, 82, 10})
	objectiveLine := 99
	for _, line := range v.objectiveLines() {
		preflightText(v.Assets, frame, 3, line, 217, objectiveLine, 10, [4]int{217, 99, 100, 25})
		objectiveLine += 6
	}
	var helps []string
	if v.Hover >= 1 && v.Hover <= 5 {
		helps = append(helps, "Selector #1 - ARMING")
		if v.Selected > 0 {
			helps = append(helps, "P - Swap PILOTS")
		}
		helps = append(helps, "H - Swap HELICOPTERS", "D - Duplicate Helicopter")
		if v.Selected == 0 || v.Selected == 2 || v.Selected == 3 {
			name, section := assetdata.OfficeCString(v.Flight[v.Selected].Raw[4:30]), "heavy"
			if v.Selected == 0 {
				name = "yourself"
			} else if v.Selected >= 3 {
				section = "light"
			}
			helps = append(helps, fmt.Sprintf("L - Assign %s as %s section leader", name, section))
		}
	}
	line := 131
	for _, text := range helps {
		for _, wrapped := range ordnanceWrap(v.Assets.Fonts[3], text, 98) {
			preflightText(v.Assets, frame, 3, wrapped, 218, line, 10, [4]int{214, 128, 106, 43})
			line += 6
		}
	}
	caption := "OUTFIT SUMMARY"
	if v.Mode == "pilot" {
		caption = "Select Pilot to swap.  ESC to Cancel"
	} else if v.Mode == "helicopter" {
		caption = "Select Helicopter to swap.  ESC to Cancel"
		if v.Duplicate {
			caption = "Select Helicopter for replacement.  ESC to Cancel"
		}
	}
	if v.Message != "" {
		caption = v.Message
	}
	captionFont := 3
	if v.Mode == "" && v.Message == "" {
		captionFont = 5
	} // A230bc uses large font5 for the idle title.
	if font := v.Assets.Fonts[captionFont]; font != nil {
		colour := 7
		if v.Message != "" {
			colour = 12 // GS1:118de: warning ink0c, font3; persists until caption redraw.
		}
		assetdata.DrawBottomTooltip(frame, font, caption, colour)
	}
	if v.blinkWhite {
		for _, rectangle := range v.transferRectangles() {
			drawPreflightOutline(frame, rectangle, 15)
		}
	}
	return frame
}

func outfitStores(a *assetdata.MissionAssets, row [36]byte) []string {
	r := row[:]
	var lines []string
	if definition := a.WeaponDefinitions[int(int8(r[2]))]; definition != nil && gamestate.RowWord(r, 21) > 0 {
		lines = append(lines, fmt.Sprintf("%s: %d rounds", preflightCString(definition, 17), gamestate.RowWord(r, 21)))
	}
	ids, counts := make([]int, 0, 3), make([]int16, 0, 3)
	for station := 0; station < 3; station++ {
		id, count := int(int8(r[3+station])), gamestate.RowWord(r, 12+2*station)
		if id < 0 || count == 0 {
			continue
		}
		found := -1
		for i, old := range ids {
			if old == id {
				found = i
			}
		}
		if found < 0 {
			ids, counts = append(ids, id), append(counts, count)
		} else {
			counts[found] += count // Original aggregate uses wrapping16-bit addition.
		}
	}
	for i, id := range ids {
		name := preflightCString(a.WeaponDefinitions[id], 43)
		lines = append(lines, fmt.Sprintf("%dx %s", counts[i], name))
	}
	return lines
}

// A241a6 orders eight signed counters, omits nonpositive counts and prints the
// count only above one. No medal name or count is inferred from pilot rank.
func outfitAwardText(counts []int8, names []string) string {
	if len(counts) != 8 || len(names) != 8 {
		return ""
	}
	var parts []string
	for i, count := range counts {
		if count > 1 {
			parts = append(parts, fmt.Sprintf("%s%d", names[i], count))
		} else if count == 1 {
			parts = append(parts, names[i])
		}
	}
	return strings.Join(parts, " ")
}
