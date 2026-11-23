package state

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	"reflect"
)

const MissionRecoveryPolicy = "native-launch-target-unique-mia-v1"
const OptionalMissionRecoveryPolicy = "historical-recovery-optional-v2"

// Source GS A1ee44 expects the launch GS category, not GS2's cleared live kind.
// Its original TARGET loader skips a successful read. This bounded native repair
// retains launch records and admits only a unique, fully identified MIA owner.
type MissionRecoveryCandidate struct {
	SourceRow    int    `json:"source_row"`
	Before       *Pilot `json:"pilot_before"`
	BeforeSHA256 string `json:"pilot_before_sha256"`
}

type MissionRecoveryContext struct {
	Policy               string                     `json:"policy"`
	LaunchTargets        [][62]byte                 `json:"launch_targets"`
	ObjectiveAssignments [2]uint16                  `json:"objective_assignments"`
	Candidates           []MissionRecoveryCandidate `json:"candidates"`
	ObjectiveRevisions   *[2]ObjectiveRevision      `json:"objective_revisions,omitempty"`
}

type MissionRecoveredPilot struct {
	Policy       string `json:"policy"`
	SourceRow    int    `json:"source_row"`
	BeforeSHA256 string `json:"pilot_before_sha256"`
	Before       *Pilot `json:"pilot_before"`
	After        *Pilot `json:"pilot_after"`
}

func (c *MissionRecoveryContext) UnmarshalJSON(raw []byte) error {
	type plain MissionRecoveryContext
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return err
	}
	if len(fields) != 4 && len(fields) != 5 {
		return fmt.Errorf("mission recovery needs all four launch owners")
	}
	for _, name := range []string{"policy", "launch_targets", "objective_assignments", "candidates"} {
		if _, ok := fields[name]; !ok {
			return fmt.Errorf("mission recovery field%s missing", name)
		}
	}
	var rows [][]json.RawMessage
	if err := json.Unmarshal(fields["launch_targets"], &rows); err != nil || len(rows) != 5 {
		return fmt.Errorf("mission recovery needs five complete launch TARGET62 records")
	}
	for _, row := range rows {
		if len(row) != 62 {
			return fmt.Errorf("mission recovery launch TARGET tail missing")
		}
		for _, v := range row {
			if string(v) == "null" {
				return fmt.Errorf("mission recovery unknown TARGET byte")
			}
		}
	}
	var assignments []json.RawMessage
	if err := json.Unmarshal(fields["objective_assignments"], &assignments); err != nil || len(assignments) != 2 {
		return fmt.Errorf("mission recovery assignment pair missing")
	}
	for _, v := range assignments {
		if string(v) == "null" {
			return fmt.Errorf("mission recovery assignment unknown")
		}
	}
	if raw, ok := fields["objective_revisions"]; ok {
		var revisions []map[string]json.RawMessage
		if err := json.Unmarshal(raw, &revisions); err != nil || len(revisions) != 2 {
			return fmt.Errorf("mission recovery needs both complete objective revisions")
		}
		for _, revision := range revisions {
			var source []json.RawMessage
			if len(revision) != 3 || len(revision["seq"]) == 0 || string(revision["seq"]) == "null" || len(revision["completed"]) == 0 || string(revision["completed"]) == "null" || json.Unmarshal(revision["source"], &source) != nil || len(source) != 62 {
				return fmt.Errorf("mission recovery objective revision source incomplete")
			}
			for _, b := range source {
				if string(b) == "null" {
					return fmt.Errorf("mission recovery objective revision byte unknown")
				}
			}
		}
	}
	var value plain
	if err := assetdata.DecodeObject(raw, &value); err != nil {
		return err
	}
	*c = MissionRecoveryContext(value)
	return nil
}

func validateRecoveryCandidates(rows []MissionRecoveryCandidate, playerID string) error {
	if len(rows) > 5 {
		return fmt.Errorf("mission recovery exceeds source roster rows1..5")
	}
	seen := map[string]bool{}
	last := 0
	for _, r := range rows {
		if r.SourceRow <= last || r.SourceRow > 5 || r.Before == nil || r.Before.Status != StatusMIA || r.Before.ID == playerID || seen[r.Before.ID] || !validSortieHash(r.BeforeSHA256) {
			return fmt.Errorf("mission recovery candidate identity/order invalid")
		}
		test := EmptySaveState()
		test.Slots[0] = r.Before
		if err := test.Validate(); err != nil {
			return fmt.Errorf("mission recovery candidate: %w", err)
		}
		raw, err := assetdata.JsonBytes(pilotDocument{pilotSchemaVersion, r.Before})
		if err != nil || assetdata.Hash(raw) != r.BeforeSHA256 {
			return fmt.Errorf("mission recovery candidate fingerprint mismatch")
		}
		seen[r.Before.ID], last = true, r.SourceRow
	}
	return nil
}

// Interpret a source POST view without changing native stable roster slots.
// A unique match makes historical relative row ordering immaterial; ambiguity
// is excluded instead of emulating DOS first-match/token collisions.
func sourceRecoveryRoster(s SaveState, files map[string][]byte) []MissionRecoveryCandidate {
	rows := s.Slots
	for i, p := range rows {
		if p != nil && p.ID == s.SelectedPilotID {
			rows[0], rows[i] = rows[i], rows[0]
			break
		}
	}
	var out []MissionRecoveryCandidate
	for i := 1; i < len(rows); i++ {
		if p := rows[i]; p != nil && p.Status == StatusMIA {
			out = append(out, MissionRecoveryCandidate{i, cloneSortiePilot(p), assetdata.Hash(files[pilotFilename(p.ID)])})
		}
	}
	return out
}

func prepareMissionRecovery(p *SortieProposal) (*MissionRecoveredPilot, error) {
	if p.Version == SafeSortieProposalVersion {
		recovered, decision, err := prepareOptionalMissionRecovery(p)
		p.RecoveryDecision = decision
		return recovered, err
	}
	f := p.Facts
	if f.ResultContext == nil || f.ResultContext.MissionRecovery == nil {
		return nil, nil
	}
	c := f.ResultContext.MissionRecovery
	if c.Policy != MissionRecoveryPolicy || len(c.LaunchTargets) != 5 {
		return nil, fmt.Errorf("unsupported mission recovery launch contract")
	}
	if err := validateRecoveryCandidates(c.Candidates, p.PilotBefore.ID); err != nil {
		return nil, err
	}
	// A1e472's actual write follows the player-loss exit. This native boundary
	// additionally requires the assigned player's real return and retained cargo,
	// never a rescue-normalized flag or a fabricated follower return.
	if p.Result == nil || p.Result.Loss || f.SourceFlags&0x80 == 0 {
		return nil, nil
	}
	qualified := false
	var roles [2]bool
	for role := 0; role < 2; role++ {
		if f.SourceFlags&(1<<role) == 0 || binary.LittleEndian.Uint16(c.LaunchTargets[role+2][32:]) != 2 {
			continue
		}
		if c.ObjectiveAssignments[role] != 1 || len(f.ResultContext.Actors) == 0 || f.ResultContext.Actors[0].Ship[0]&0xf8 != 0x18 {
			continue
		}
		qualified = true
		roles[role] = true
	}
	if !qualified {
		return nil, nil
	}
	var chosen *MissionRecoveryCandidate
	for i := range c.Candidates {
		r := &c.Candidates[i]
		before := r.Before
		if before.Theatre.Raw() < 0 {
			return nil, fmt.Errorf("MIA theatre provenance cannot establish a unique recovery owner")
		}
		if before.Theatre != p.PilotBefore.Theatre {
			continue
		}
		if len(before.LegacyRaw) != LegacyRecordSize || before.LegacyStatus != 3 || before.LegacyRaw[0x56] != 3 || before.Rank.Raw() < 0 || before.Rank.Raw() > 11 {
			return nil, fmt.Errorf("MIA record lacks supported scenario/status provenance")
		}
		if before.LegacyRaw[0x5b] != f.ResultContext.SelectedScenario {
			continue
		}
		if chosen != nil {
			return nil, fmt.Errorf("mission recovery has ambiguous MIA identities; no pilot revived")
		}
		chosen = r
	}
	if chosen == nil {
		return nil, nil
	}
	// A replaced TARGET is a restriction on this compatibility revival only.
	// It must not reject an ordinary retasked result with no matching roster MIA.
	for role, applies := range roles {
		if !applies {
			continue
		}
		expected := c.LaunchTargets[role+2]
		expected[32], expected[33] = 0, 0
		if !reflect.DeepEqual(expected, f.Targets[role+2]) {
			return nil, fmt.Errorf("mission recovery TARGET was replaced or lacks original opaque bytes")
		}
	}
	after := cloneSortiePilot(chosen.Before)
	// The original consumer writes only +56. Native typed status mirrors that
	// byte; score, awards, opaque bytes and this pilot's Phoenix stay untouched.
	after.Status, after.LegacyStatus, after.LegacyRaw[0x56] = StatusActive, 1, 1
	return &MissionRecoveredPilot{MissionRecoveryPolicy, chosen.SourceRow, chosen.BeforeSHA256, cloneSortiePilot(chosen.Before), after}, nil
}
