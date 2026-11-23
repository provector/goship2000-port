package state

import (
	"bytes"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	"strings"
)

// SquadronPilot owns one of the four stored wingmen, not a six-slot roster pilot.
// Unknown statistic bytes remain intact; no AI behavior is inferred here.
type SquadronPilot struct {
	Name      string `json:"name"`
	Rank      Rank   `json:"rank"`
	Portrait  int    `json:"portrait"`
	LegacyRaw []byte `json:"legacy_record_base64,omitempty"`
}

type FlightPilot struct {
	SourceIndex int      `json:"source_index"`
	Name        string   `json:"name"`
	Rank        Rank     `json:"rank"`
	Portrait    int      `json:"portrait"`
	Group       int      `json:"group"`
	Leader      bool     `json:"leader"`
	Callsign    string   `json:"callsign"`
	Incomplete  bool     `json:"incomplete,omitempty"`
	AwardCounts []int8   `json:"award_counters,omitempty"`
	Raw         [41]byte `json:"raw"`
	StoredRaw   [41]byte `json:"stored_raw"`
}

func squadronFromPilot(p *Pilot) ([]SquadronPilot, error) {
	if p == nil {
		return nil, fmt.Errorf("flight assignments require a pilot")
	}
	if len(p.SquadronPilots) != 0 {
		if len(p.SquadronPilots) != 4 {
			return nil, fmt.Errorf("squadron requires exactly four wingmen")
		}
		out := append([]SquadronPilot(nil), p.SquadronPilots...)
		for i := range out {
			out[i].LegacyRaw = append([]byte(nil), out[i].LegacyRaw...)
		}
		return out, nil
	}
	if len(p.LegacyRaw) != LegacyRecordSize {
		return nil, fmt.Errorf("stored squadron rows are unavailable")
	}
	result := make([]SquadronPilot, 4)
	for i := range result {
		// The notional row0 at current+55h overlaps other progression fields.
		// Only indices1..4 contain the stored wingman records used by OUTFIT.
		r := p.LegacyRaw[0x55+41*(i+1) : 0x55+41*(i+2)]
		rank, err := RankFromRaw(int(int8(r[34])))
		if err != nil {
			return nil, fmt.Errorf("wingman%d: %w", i+1, err)
		}
		name := r[:26]
		if at := bytes.IndexByte(name, 0); at >= 0 {
			name = name[:at]
		}
		result[i] = SquadronPilot{string(name), rank, int(r[36]), append([]byte(nil), r...)}
	}
	return result, nil
}

func validateSquadronMember(p SquadronPilot) error {
	if err := validSaveText("wingman name", p.Name, 25); err != nil {
		return err
	}
	portrait := [4]int{5 + (p.Portrait/6)*48, (p.Portrait % 6) * 32, 37, 32}
	if p.Rank.Raw() < 0 || p.Portrait < 0 || !assetdata.OfficeRectFits(portrait) {
		return fmt.Errorf("wingman rank or portrait is unsupported")
	}
	if len(p.LegacyRaw) != 0 && len(p.LegacyRaw) != 41 {
		return fmt.Errorf("wingman raw record must contain41 bytes")
	}
	return nil
}

// InitFlightPilots does not manufacture post-mission squadron history. A manually
// promoted record may contain blank wingmen; preserve them and mark that fact.
func InitFlightPilots(p *Pilot, m *MissionData) ([]FlightPilot, error) {
	if p == nil || m == nil {
		return nil, fmt.Errorf("flight assignments require pilot and mission")
	}
	count := 1
	if m.Inputs.MissionType.Raw() > 1 && !m.SoloBuilder {
		count = 5
	}
	result := make([]FlightPilot, count)
	result[0] = FlightPilot{SourceIndex: 0, Name: p.Name, Rank: p.Rank, Leader: true}
	if p.Statistics != nil {
		result[0].AwardCounts = append([]int8(nil), p.Statistics.AwardCounts...)
	}
	if count == 1 {
		return result, nil
	}
	stored, err := squadronFromPilot(p)
	if err != nil {
		return nil, err
	}
	for i, member := range stored {
		if err := validateSquadronMember(member); err != nil {
			return nil, fmt.Errorf("wingman%d: %w", i+1, err)
		}
		result[i+1] = FlightPilot{SourceIndex: i + 1, Name: member.Name, Rank: member.Rank,
			Portrait: member.Portrait, Group: BoolInt(i+1 >= 3), Leader: i+1 == 3,
			// Rank0 is valid source history; missing operands remain unsupported.
			Incomplete: strings.TrimSpace(member.Name) == ""}
		copy(result[i+1].StoredRaw[:], member.LegacyRaw)
		if len(member.LegacyRaw) == 41 {
			for _, count := range member.LegacyRaw[26:34] {
				result[i+1].AwardCounts = append(result[i+1].AwardCounts, int8(count))
			}
		}
	}
	return result, nil
}

func BoolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func SquadronFromFlight(flight []FlightPilot) ([]SquadronPilot, error) {
	if len(flight) != 5 || flight[0].SourceIndex != 0 {
		return nil, fmt.Errorf("stored squadron copyback requires five rows with primary source0")
	}
	result := make([]SquadronPilot, 4)
	seen := [5]bool{}
	for _, row := range flight[1:] {
		if row.SourceIndex < 1 || row.SourceIndex > 4 || seen[row.SourceIndex] {
			return nil, fmt.Errorf("wingman assignments must contain each stored source1..4 exactly once")
		}
		seen[row.SourceIndex] = true
		// Imported assignment order may differ from stored record order.
		i := row.SourceIndex - 1
		result[i] = SquadronPilot{Name: row.Name, Rank: row.Rank, Portrait: row.Portrait,
			LegacyRaw: append([]byte(nil), row.StoredRaw[:]...)}
		if err := validateSquadronMember(result[i]); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func RefreshFlightPresentation(flight []FlightPilot, a *assetdata.MissionAssets, m *MissionData) error {
	if a == nil || a.Generation == nil || a.Preflight == nil || len(a.Preflight.OutfitRanks) != 12 || m == nil || len(flight) == 0 {
		return fmt.Errorf("flight presentation source tables are missing")
	}
	callIDs := [2]int{m.Conditions.Callsign, m.Conditions.SecondCallsign}
	for i := range flight {
		p := &flight[i]
		if p.Rank.Raw() < 0 || p.Rank.Raw() >= len(a.Preflight.OutfitRanks) || p.Group < 0 || p.Group > 1 {
			return fmt.Errorf("flight%d rank/group is unsupported", i+1)
		}
		which := BoolInt(p.Group != flight[0].Group)
		if callIDs[which] < 0 || callIDs[which] >= len(a.Generation.Tables.Callsigns) {
			return fmt.Errorf("flight callsign index is invalid")
		}
		p.Callsign = fmt.Sprintf("%s %d", a.Generation.Tables.Callsigns[callIDs[which]], i+1)
		p.Raw[0], p.Raw[1], p.Raw[2], p.Raw[3] = byte(p.SourceIndex), byte(p.Group), byte(BoolInt(p.Leader)), byte(p.Portrait)
		clear(p.Raw[4:])
		label := assetdata.OfficePilotLabel(p.Name, a.Preflight.OutfitRanks[p.Rank.Raw()])
		// The source uses unbounded sprintf into these fixed fields. The port
		// keeps field boundaries explicit instead of letting long JSON text spill.
		copy(p.Raw[4:29], label)
		copy(p.Raw[30:40], p.Callsign)
	}
	return nil
}

// InitializeSquadron applies the recovered replacement initializer explicitly.
// It is reserved for comparison fixtures/future outcome handling, never invoked
// just because a player edits rank or opens pre-flight preparation.
func InitializeSquadron(p *Pilot, roster []*Pilot, names []string, draw func(int) int) ([]SquadronPilot, error) {
	if len(names) != 74 || draw == nil {
		return nil, fmt.Errorf("squadron initialization requires74 source names and bounded draws")
	}
	rows, err := squadronFromPilot(p)
	if err != nil {
		return nil, err
	}
	for i := range rows {
		if rows[i].Rank != RankWOC {
			continue // Outcome-related18h replacement flags are outside this fixture operation.
		}
		portrait := 0
		for attempts := 0; ; attempts++ {
			if attempts >= 1024 {
				return nil, fmt.Errorf("squadron portrait retries exhausted")
			}
			portrait = draw(33) + 1
			if portrait < 1 || portrait > 33 {
				return nil, fmt.Errorf("squadron portrait draw outside source range")
			}
			used := false
			for _, other := range rows {
				used = used || other.Portrait == portrait
			}
			if !used {
				break
			}
		}
		name := ""
		for attempts := 0; ; attempts++ {
			if attempts >= 4096 {
				return nil, fmt.Errorf("squadron name retries exhausted")
			}
			index := draw(74)
			if index < 0 || index >= len(names) {
				return nil, fmt.Errorf("squadron name draw outside source range")
			}
			name = names[index]
			used := false
			for _, owner := range roster {
				if owner == nil {
					continue
				}
				used = used || strings.Contains(owner.Name, name)
				members := rows
				if owner.ID != p.ID {
					var err error
					members, err = squadronFromPilot(owner)
					if err != nil {
						return nil, err
					}
				}
				for _, member := range members {
					used = used || strings.Contains(member.Name, name)
				}
			}
			for _, member := range rows {
				used = used || strings.Contains(member.Name, name)
			}
			if !used {
				break
			}
		}
		raw := make([]byte, 41)
		copy(raw, rows[i].LegacyRaw)
		clear(raw[26:34]) // Source clears eight raw counters, then sets byte33 below.
		raw[33], raw[34], raw[35], raw[36] = 1, 1, 1, byte(portrait)
		copy(raw, name)
		raw[len(name)] = 0 // strcpy leaves unrelated bytes after its terminator intact.
		rows[i] = SquadronPilot{Name: name, Rank: RankWO1, Portrait: portrait, LegacyRaw: raw}
	}
	return rows, nil
}
