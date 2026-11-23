package state

import (
	"encoding/binary"
	"fmt"
)

// CampaignResultInput is the result-reader/scoring boundary. LaunchDraft is the
// accepted preparation's draft, never the previous pilot's Campaign snapshot.
// SourceStage, LaunchHigh and CurrentMission have different source owners.
type CampaignResultInput struct {
	LaunchDraft     *CampaignState `json:"launch_draft"`
	Scenario        string         `json:"scenario"`
	World           string         `json:"world"`
	Ordinal         int            `json:"ordinal"`
	SourceStage     byte           `json:"source_stage"`
	LaunchHigh      byte           `json:"launch_high"`
	CurrentMission  MissionType    `json:"current_mission"`
	FinalObjects    [80][32]byte   `json:"final_objects"`
	Credits         [40]byte       `json:"credits"`
	NormalizedFlags byte           `json:"normalized_flags"`
	ScoredSuccess   bool           `json:"scored_success"`
	FollowerFlags   [4]byte        `json:"follower_flags"`
}

// CampaignResultProposal is immutable after construction. ScoreBonus is added
// to the source signed16 subtotal before sign extension/scaling. Concluded is a
// planned post-debrief change, not permission to publish before final ack.
type CampaignResultProposal struct {
	Campaign       *CampaignState `json:"campaign"`
	Quality        int16          `json:"quality"`
	CounterUpdated bool           `json:"counter_updated"`
	ScoreBonus     int16          `json:"score_bonus"`
	Verdict        int8           `json:"verdict"`
	Concluded      bool           `json:"concluded"`
	SortieSuccess  bool           `json:"sortie_success"`
	Losses         uint16         `json:"losses"`
}

func campaignCredit(credits [40]byte, row int) byte {
	return credits[row/2] >> uint((row&1)*4) & 15
}

// CampaignQuality follows GS A1b40e: eligibility reads OBJECT class/byte11 and
// flag70, and every nonzero credit owner counts. It is not player kill credit.
func CampaignQuality(objects [80][32]byte, credits [40]byte, flags byte) int16 {
	if flags&3 == 3 {
		return 200
	}
	numerator, denominator := 0, 0
	for i, row := range objects {
		if !(row[10] < 16 && row[11] != 0) && row[0]&0x70 == 0 {
			continue
		}
		denominator++
		if campaignCredit(credits, i) != 0 {
			numerator++
		}
	}
	if denominator == 0 {
		return 100
	}
	return int16(100 * numerator / denominator)
}

// CampaignTerminal is GS A1b3bc's signed-byte comparison, including tie defeat
// and the current-mode gate (which is not the launch-header counter gate).
func CampaignTerminal(s *CampaignState, mission MissionType) int8 {
	if s == nil || mission != MissionCampaign {
		return 0
	}
	threshold := int8(s.TargetCounter)
	a, b := int8(s.ResultCounters[0]), int8(s.ResultCounters[1])
	if a < threshold && b < threshold {
		return 0
	}
	if a > b {
		return 1
	}
	return -1
}

// CampaignFollowerLossCount is A1b4ae's fixed four exported status rows. It is
// not restricted to visible actors or used as a substitute for their metrics.
func CampaignFollowerLossCount(flags [4]byte) uint16 {
	var count uint16
	for _, value := range flags {
		if value&0x38 != 0 {
			count++
		}
	}
	return count
}

// PrepareCampaignResult runs before score scaling. It never mutates LaunchDraft
// or caller objects and rejects unknown credit-address domains before writing.
func PrepareCampaignResult(in CampaignResultInput) (*CampaignResultProposal, error) {
	if in.CurrentMission == MissionTraining && (in.SourceStage != 0 || in.LaunchDraft != nil) {
		return nil, fmt.Errorf("Training with active Campaign cache ownership is unsupported; Campaign was preserved")
	}
	if in.LaunchDraft == nil {
		return nil, fmt.Errorf("Campaign result requires its actual accepted launch draft")
	}
	draft, err := CloneCampaign(in.LaunchDraft)
	if err != nil {
		return nil, err
	}
	if in.CurrentMission != MissionCampaign || in.SourceStage != 1 || draft.Phase != CampaignActive || draft.Suspended != nil {
		return nil, fmt.Errorf("Campaign result requires an active accepted Campaign preparation")
	}
	if in.Scenario == "" || in.World == "" || in.Ordinal != draft.ScenarioOrdinal {
		return nil, fmt.Errorf("Campaign result scenario/ordinal does not match its launch draft")
	}
	cacheIndex := -1
	for i, cache := range draft.Caches {
		if cache.Scenario == in.Scenario {
			if cache.World != in.World || cache.Ordinal != in.Ordinal {
				return nil, fmt.Errorf("Campaign result cache source identity disagrees with launch")
			}
			cacheIndex = i
		}
	}
	if cacheIndex < 0 {
		return nil, fmt.Errorf("Campaign result has no exact launch scenario cache")
	}
	cache := &draft.Caches[cacheIndex]
	// The source reader sign-extends both header bytes. Native caches with an
	// address outside result's80 nibbles remain explicit unsupported states.
	k, n := int(int8(byte(cache.BeforeCount))), len(cache.Objects)
	if k < 0 || n > 127 || k+n > 80 {
		return nil, fmt.Errorf("Campaign result cache credit interval[%d,%d) outside80 source rows", k, k+n)
	}
	for i := range cache.Objects {
		if campaignCredit(in.Credits, k+i) == 0 {
			continue
		}
		r := &cache.Objects[i].Raw
		r[0] &= 0xc7
		r[10] = 0x1d
		r[12], r[13] = r[14], r[14]
		copy(r[15:27], "CMPGN WRECK\x00")
	}
	out := &CampaignResultProposal{Campaign: draft, Quality: CampaignQuality(in.FinalObjects, in.Credits, in.NormalizedFlags), SortieSuccess: in.ScoredSuccess, Losses: CampaignFollowerLossCount(in.FollowerFlags)}
	// A1e834's launch-high gate precedes the protection0400 scoring gate.
	if in.LaunchHigh&0xe0 == 0x80 {
		out.CounterUpdated = true
		counter := 1
		if in.ScoredSuccess && out.Quality >= 30 {
			counter = 0
		}
		draft.ResultCounters[counter] = int(byte(draft.ResultCounters[counter] + 1))
		if counter == 0 && CampaignTerminal(draft, in.CurrentMission) > 0 {
			// This API represents the ordinary source scoring call argument0.
			out.ScoreBonus = 100
		}
	}
	draft.LossesRelatedCount = int(out.Losses)
	out.Verdict = CampaignTerminal(draft, in.CurrentMission)
	// Loss18 returns before A1b1da even though the preceding score/counter and
	// cache updates already ran. Do not conclude at counter increment time.
	out.Concluded = in.NormalizedFlags&0x18 == 0 && out.Verdict != 0
	return out, draft.Validate()
}

// FinishCampaignResult applies the proposal to an independently owned branch
// after commander acceptance and before main awards/qualification/retirement,
// as at GS A1e48a. Later crew copyback retains the overlapping restriction tail.
// Persistence still belongs to final ack.
func FinishCampaignResult(p *Pilot, proposal *CampaignResultProposal, normalizedFlags byte) error {
	if p == nil || len(p.LegacyRaw) != LegacyRecordSize || p.Statistics == nil || proposal == nil || proposal.Campaign == nil {
		return fmt.Errorf("Campaign result finalization needs a complete owned pilot/proposal")
	}
	draft, err := CloneCampaign(proposal.Campaign)
	if err != nil {
		return err
	}
	p.Campaign = draft
	p.LegacyRaw[0x4f] = 1
	p.LegacyRaw[0x50] = byte(draft.TargetCounter)
	p.LegacyRaw[0x51], p.LegacyRaw[0x52] = byte(draft.ResultCounters[0]), byte(draft.ResultCounters[1])
	p.LegacyRaw[0x58], p.LegacyRaw[0x5b], p.LegacyRaw[0x5c] = byte(draft.ScenarioOrdinal), byte(draft.SelectedScenario), byte(draft.PriorScenario)
	binary.LittleEndian.PutUint16(p.LegacyRaw[0xa5:], proposal.Losses)
	// raw+a5 aliases the first stored squadmate's opaque39..40 tail. Keep a
	// single lossless owner; later crew SourceIndex copyback retains this tail.
	if len(p.SquadronPilots) != 0 && len(p.SquadronPilots[0].LegacyRaw) == 41 {
		binary.LittleEndian.PutUint16(p.SquadronPilots[0].LegacyRaw[39:], proposal.Losses)
	}
	if normalizedFlags&0x18 == 0 && proposal.Concluded {
		if proposal.Verdict > 0 {
			old := p.LegacyRaw[0x55]
			if old&3 < 3 {
				p.LegacyRaw[0x55] = 5*(old&3) + 1 + (old & 0x10)
			}
		}
		p.Campaign = nil
		p.LegacyRaw[0x4f] = 0
		p.MissionType = MissionFlight
		p.Statistics.MissionCampaignCounter += 100
	}
	syncSortieStatistics(p)
	return nil
}
