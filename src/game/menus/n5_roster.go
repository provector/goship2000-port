package menus

import (
	"bytes"
	"fmt"
	gamestate "gunship2000-assets/game/state"
	"strings"
)

// Original GS DS0bf6/0bd6 display tables; these are not progression thresholds.
var rosterCrewRanks = [...]string{"WOC", "WO1", "CW2", "CW3", "CW4", "2LT", "1LT", "CPT", "MAJ", "LTC", "COL", "BG"}
var rosterCrewMedals = [...]string{"CMOH", "DSC", "SS", "BSV", "AMV", "ACV", "PH", "NDS"}

type rosterCrewRow struct {
	Name   string
	Rank   int
	Medals [8]int8
}

func rosterStoredCrew(p *gamestate.Pilot) [4]rosterCrewRow {
	var rows [4]rosterCrewRow
	if p == nil {
		return rows
	}
	for i := range rows {
		var raw []byte
		if len(p.SquadronPilots) == 4 {
			member := p.SquadronPilots[i]
			rows[i].Name, rows[i].Rank = member.Name, member.Rank.Raw()
			raw = member.LegacyRaw
		} else if len(p.LegacyRaw) == gamestate.LegacyRecordSize {
			start := 0x55 + 41*(i+1)
			raw = p.LegacyRaw[start : start+41]
			rows[i].Name = string(bytes.SplitN(raw[:26], []byte{0}, 2)[0])
			rows[i].Rank = int(int8(raw[34]))
		}
		if len(raw) == 41 {
			for j, v := range raw[26:34] {
				rows[i].Medals[j] = int8(v)
			}
		}
	}
	return rows
}

func rosterMedalsText(awards [8]int8) string {
	var b strings.Builder
	for i, n := range awards {
		if n <= 0 {
			continue
		}
		b.WriteString(rosterCrewMedals[i])
		if n > 1 {
			fmt.Fprint(&b, n)
		}
		b.WriteByte(' ')
	}
	return b.String()
}

func (c *FrontEnd) drawRosterCrew(frame []byte, p *gamestate.Pilot) {
	rows := rosterStoredCrew(p)
	font := c.Menus.Fonts[3]
	if font == nil {
		return
	}
	maxRank, maxName := 0, 0
	for _, row := range rows {
		// Source measures all four rows before suppressing rank0 at draw time.
		if row.Rank >= 0 && row.Rank < len(rosterCrewRanks) {
			maxRank = max(maxRank, font.Measure([]byte(rosterCrewRanks[row.Rank])))
		}
		maxName = max(maxName, font.Measure([]byte(row.Name)))
	}
	for i, row := range rows {
		if row.Rank <= 0 || row.Rank >= len(rosterCrewRanks) {
			continue
		}
		y := 49 + 6*i
		c.clippedText(frame, 3, rosterCrewRanks[row.Rank], 78, y, 0, [4]int{78, 10, 200, 66})
		c.clippedText(frame, 3, row.Name, 78+maxRank+4, y, 0, [4]int{78, 10, 200, 66})
		// Source column ownership uses measured rank/name maxima over all four rows.
		width := max(0, 180-maxName-maxRank)
		c.clippedText(frame, 3, rosterMedalsText(row.Medals), 78+maxRank+maxName+12, y, 0, [4]int{78 + maxRank + maxName + 12, y, width, 6})
	}
}

func (c *FrontEnd) viewedOfficeState() gamestate.SaveState {
	s := c.Store.State()
	if c.ViewedPilotID != "" {
		for _, p := range s.Slots {
			if p != nil && p.ID == c.ViewedPilotID {
				s.SelectedPilotID = p.ID
				break
			}
		}
	}
	return s
}

func (c *FrontEnd) viewingPostedPilot() bool {
	return c.ViewedPilotID == "" || c.ViewedPilotID == c.Store.State().SelectedPilotID
}

func (c *FrontEnd) crewAircraftName(accepted byte) string {
	// GS3:090b->1581:039c searches SHHD byte0, then reads name+26h.
	// Accepted is the front-end SHHD ID, not the compact simulation selector.
	return preflightCString(c.Office.Assets.Missions.AircraftDefinitions[int(accepted)], 38)
}
