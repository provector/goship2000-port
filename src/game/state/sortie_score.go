package state

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	"reflect"
	"strings"
)

// SortieFacts is an immutable termination snapshot, not a DOS SCORE.DAT file.
// SourceFlags preserve the pre-normalization facts. ResultContext retains real
// participant/crew provenance; Targets retain all five62-byte opaque records.
type SortieFacts struct {
	SoloBuilder            bool                 `json:"solo_builder,omitempty"`
	SortieID               string               `json:"sortie_id"`
	SourceProfile          string               `json:"source_profile"`
	MissionType            MissionType          `json:"mission_type"`
	ActorCount             int                  `json:"actor_count"`
	SourceMode             byte                 `json:"source_mode"`
	Reason                 string               `json:"reason"`
	SourceFlags            byte                 `json:"source_flags"`
	RawScore               int16                `json:"raw_score"`
	ElapsedDA              uint16               `json:"elapsed_da"`
	Coefficient            int16                `json:"launch_coefficient"`
	SelectorMultisetMatch  bool                 `json:"selector_multiset_match"`
	RecommendedSelectors   []byte               `json:"recommended_selectors"`
	AcceptedSelectors      []byte               `json:"accepted_selectors"`
	LaunchSwitches         uint16               `json:"launch_switches"`
	ObjectCount            int                  `json:"object_count"`
	ObjectValues           [80]int16            `json:"object_values"`
	ObjectCredits          [40]byte             `json:"object_credit_nibbles"`
	PlayerAircraftValue    int8                 `json:"player_aircraft_table_value"`
	InactiveAircraftValues [4]int8              `json:"inactive_aircraft_table_values"`
	InactiveHeaderPolicy   string               `json:"inactive_header_policy"`
	LaunchObjectiveKinds   [2]int16             `json:"launch_display_objective_kinds"`
	Targets                [][62]byte           `json:"targets"`
	PlayerTerminal         bool                 `json:"player_terminal"`
	FinalPose              InitializedPose      `json:"final_pose"`
	FinalFuel              int16                `json:"final_fuel_percent"`
	FinalObjects           [80][32]byte         `json:"final_objects"`
	ResultContext          *SortieResultContext `json:"result_context,omitempty"`
}

// Pending facts have no optional zero/default fields. encoding/json otherwise
// accepts short fixed arrays by filling missing elements with zero, which would
// conceal a truncated TARGET tail or a missing zero-valued result input.
func (f *SortieFacts) UnmarshalJSON(data []byte) error {
	type plain SortieFacts
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	typ := reflect.TypeOf(plain{})
	if len(fields) < typ.NumField()-2 || len(fields) > typ.NumField() {
		return fmt.Errorf("pending sortie facts require every typed field")
	}
	for i := 0; i < typ.NumField(); i++ {
		name := strings.Split(typ.Field(i).Tag.Get("json"), ",")[0]
		if name == "result_context" || name == "solo_builder" {
			if raw, ok := fields[name]; ok && bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
				return fmt.Errorf("pending result context cannot be null")
			}
			continue
		}
		raw, ok := fields[name]
		if !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return fmt.Errorf("pending sortie fact %s is missing/null", name)
		}
	}
	for name := range fields {
		found := false
		for i := 0; i < typ.NumField(); i++ {
			found = found || name == strings.Split(typ.Field(i).Tag.Get("json"), ",")[0]
		}
		if !found {
			return fmt.Errorf("unknown pending sortie fact %s", name)
		}
	}
	array := func(raw json.RawMessage, n int) ([]json.RawMessage, error) {
		var v []json.RawMessage
		if err := json.Unmarshal(raw, &v); err != nil || len(v) != n {
			return nil, fmt.Errorf("pending sortie array requires exactly%d elements", n)
		}
		for _, item := range v {
			if bytes.Equal(bytes.TrimSpace(item), []byte("null")) {
				return nil, fmt.Errorf("pending sortie array contains unknown/null element")
			}
		}
		return v, nil
	}
	if _, err := array(fields["object_values"], 80); err != nil {
		return err
	}
	if _, err := array(fields["object_credit_nibbles"], 40); err != nil {
		return err
	}
	if _, err := array(fields["inactive_aircraft_table_values"], 4); err != nil {
		return err
	}
	if _, err := array(fields["launch_display_objective_kinds"], 2); err != nil {
		return err
	}
	for _, shape := range []struct {
		name       string
		rows, cols int
	}{{"targets", 5, 62}, {"final_objects", 80, 32}} {
		rows, err := array(fields[shape.name], shape.rows)
		if err != nil {
			return err
		}
		for _, row := range rows {
			if _, err := array(row, shape.cols); err != nil {
				return err
			}
		}
	}
	var pose map[string]json.RawMessage
	if err := json.Unmarshal(fields["final_pose"], &pose); err != nil || len(pose) != 4 {
		return fmt.Errorf("pending final pose is incomplete")
	}
	for _, name := range []string{"H0", "H1", "Height", "Angles"} {
		raw, ok := pose[name]
		if !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return fmt.Errorf("pending final pose %s missing", name)
		}
	}
	if _, err := array(pose["Angles"], 3); err != nil {
		return err
	}
	var value plain
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*f = SortieFacts(value)
	return nil
}

// SortieTables is prepared from the recognized GS source profile. Keeping the
// original indices avoids guessing names from medal/rank threshold order.
type SortieTables struct {
	SourceProfile       string     `json:"source_profile"`
	RankThresholds      [12]int16  `json:"rank_thresholds"`
	MedalThresholds     [6]int16   `json:"medal_thresholds"`
	MedalPenaltyCredits [6]int16   `json:"medal_penalty_credits"`
	RankNames           [12]string `json:"rank_names"`
	MedalNames          [8]string  `json:"medal_names"`
}

const (
	// Version1 is retained strictly for validating N/N.4 committed proposals and
	// durable journals; version2 pins N.5/O.2 scoring. Version3 adds the
	// source-backed authored Simulation domain. Version4 separates actual
	// accepted/refused commission CRT paths and pending replacement names.
	// Historical decisions and career scores are never regenerated on load.
	SortieProposalVersion     = 4
	SafeSortieProposalVersion = 5
	// Native Single/Training preparation owns a fresh one-row readiness scope;
	// it does not inherit four hidden DOS rows from an earlier Flight mission.
	SortieFreshInactiveHeaders    = "native-fresh-inactive-shhd0-v1"
	SortieSuppliedInactiveHeaders = "supplied-source-inactive-header-values-v1"
	SortieAcceptedFlightHeaders   = "accepted-active-flight-and-fresh-padding-headers-v1"
)

type SortieScore struct {
	NormalizedFlags      byte   `json:"normalized_flags"`
	RawScore             int16  `json:"raw_score"`
	AfterObjectCredit    int16  `json:"after_object_credit"`
	AfterObjectiveBonus  int16  `json:"after_objective_bonus"`
	AircraftContribution int16  `json:"aircraft_contribution"`
	PreScale             int32  `json:"pre_scale"`
	Score                int32  `json:"score"`
	ExportedScoreWord    int16  `json:"exported_score_word"`
	ElapsedDA            uint16 `json:"elapsed_da"`
	TimeFactor           int16  `json:"time_factor"`
	CareerApplied        bool   `json:"career_applied"`
}

// A proposal is pure data. Choosing a commission does not mutate the pilot;
// ResolveSortieProposal returns an independent final pilot for journal commit.
type SortieProposal struct {
	Version                   int                      `json:"version"`
	Facts                     SortieFacts              `json:"facts"`
	FactsDigest               string                   `json:"facts_sha256"`
	Tables                    SortieTables             `json:"tables"`
	PilotBefore               *Pilot                   `json:"pilot_before"`
	ScoredPilot               *Pilot                   `json:"scored_pilot"`
	Score                     SortieScore              `json:"score"`
	Success                   bool                     `json:"success"`
	TrainingSuccess           bool                     `json:"training_success"`
	InitialRankRecommendation int                      `json:"initial_rank_recommendation"`
	SpecialOutcome            bool                     `json:"source_special_outcome"`
	CommanderRank             int                      `json:"commander_rank"`
	RecommendedRank           int                      `json:"recommended_rank"`
	Commission                bool                     `json:"commission"`
	Awards                    [8]int8                  `json:"awards"`
	MedalIndex                int                      `json:"medal_index"`
	MedalDeferred             bool                     `json:"medal_deferred"`
	PenaltyCredit             int16                    `json:"penalty_credit"`
	Retirement                bool                     `json:"retirement"`
	AcceptedPilot             *Pilot                   `json:"accepted_pilot"`
	RefusedPilot              *Pilot                   `json:"refused_pilot"`
	Result                    *SortieResultDetails     `json:"result,omitempty"`
	RefusedResult             *SortieResultDetails     `json:"refused_result,omitempty"`
	RecoveredPilot            *MissionRecoveredPilot   `json:"recovered_pilot,omitempty"`
	RecoveryDecision          *MissionRecoveryDecision `json:"recovery_decision,omitempty"`
}

func (t SortieTables) Validate() error {
	if t.SourceProfile != assetdata.IslandIce469085.Name {
		return fmt.Errorf("sortie result tables need the recognized source profile")
	}
	if t.RankThresholds != [12]int16{0, 100, 1000, 3000, 6000, 1000, 3000, 6000, 10000, 15000, 21000, 28000} ||
		t.MedalThresholds != [6]int16{1700, 1300, 1000, 750, 500, 250} ||
		t.MedalPenaltyCredits != [6]int16{1500, 1000, 500, 200, 0, 0} {
		return fmt.Errorf("sortie result table operands do not match the supported source profile")
	}
	for _, name := range t.RankNames {
		if name == "" {
			return fmt.Errorf("prepared source rank name missing")
		}
	}
	for _, name := range t.MedalNames {
		if name == "" {
			return fmt.Errorf("prepared source medal name missing")
		}
	}
	return nil
}

// SortieSelectorMultisetMatch is GS1163:1c56's consume-once selector comparison.
// Aircraft selectors here are accepted front-end SHHD identities, not cockpit IDs.
func SortieSelectorMultisetMatch(retained, accepted []byte) bool {
	if len(retained) != len(accepted) || len(retained) == 0 || len(retained) > 5 {
		return false
	}
	remaining := append([]byte(nil), retained...)
	for _, selector := range accepted {
		found := -1
		for i, candidate := range remaining {
			if candidate == selector {
				found = i
				break
			}
		}
		if found < 0 {
			return false
		}
		remaining[found] = 255
	}
	return true
}

func (f SortieFacts) Validate() error {
	if f.SortieID == "" || len(f.SortieID) > 128 || f.SourceProfile != assetdata.IslandIce469085.Name {
		return fmt.Errorf("sortie result needs stable identity and recognized source profile")
	}
	if f.MissionType != MissionSingle && f.MissionType != MissionTraining && f.MissionType != MissionFlight && f.MissionType != MissionCampaign && f.MissionType != MissionSimulated {
		return fmt.Errorf("result progression excludes unknown mission types")
	}
	if f.ActorCount < 1 || f.ActorCount > 5 || f.ActorCount != 1 && f.ResultContext == nil {
		return fmt.Errorf("result progression does not admit follower result producers")
	}
	if (f.MissionType == MissionSingle || f.MissionType == MissionTraining) && f.ActorCount != 1 {
		return fmt.Errorf("Single/Training result has one genuine actor")
	}
	if f.SoloBuilder && (f.ActorCount != 1 || f.MissionType != MissionSimulated || f.LaunchSwitches>>13 != uint16(MissionSingle.Raw())) {
		return fmt.Errorf("solo Simulation result conflicts with launch reservation")
	}
	if f.MissionType == MissionSimulated && (f.ActorCount != 5 && !(f.SoloBuilder && f.ActorCount == 1) || f.ResultContext == nil) {
		return fmt.Errorf("authored Simulation requires five genuine actors and full result ownership")
	}
	if f.SourceMode != 0 {
		return fmt.Errorf("replay/resume/record-capacity source modes are outside the admitted native mode0 result boundary")
	}
	if f.ResultContext == nil && (f.PlayerTerminal || f.SourceFlags&0x3c != 0 || f.MissionType == MissionFlight || f.MissionType == MissionCampaign) {
		return fmt.Errorf("loss, wounded, rescue or terminal results require the later damage/loss package")
	}
	switch f.Reason {
	case "base", "carrier":
		if f.SourceFlags&0x80 == 0 {
			return fmt.Errorf("natural base/carrier result lacks returned flag")
		}
	case "corridor":
	case "voluntary":
		if f.SourceFlags&0x80 != 0 {
			return fmt.Errorf("voluntary termination cannot grant safe-return credit")
		}
	case "terminal", "disabled-contact", "fuel-out", "fuel-out-contact", "rough-contact", "hard-landing", "damage-fuel", "crash", "damaged":
		if f.ResultContext == nil {
			return fmt.Errorf("damage ending requires complete frozen source context")
		}
	default:
		return fmt.Errorf("unsupported sortie ending reason %q", f.Reason)
	}
	if f.ObjectCount != 80 || len(f.Targets) != 5 {
		return fmt.Errorf("result requires all80 object credit inputs and five retained62-byte TARGET records")
	}
	switch f.InactiveHeaderPolicy {
	case SortieFreshInactiveHeaders:
		if f.InactiveAircraftValues != [4]int8{} {
			return fmt.Errorf("fresh inactive SHHD0 headers must have source table value0")
		}
	case SortieSuppliedInactiveHeaders:
		// Explicit source/supplied-state fixtures can exercise the four source
		// loop iterations without claiming naturally present flight members.
	case SortieAcceptedFlightHeaders:
		if f.ResultContext == nil {
			return fmt.Errorf("active flight header values require accepted actor provenance")
		}
	default:
		return fmt.Errorf("inactive readiness-header ownership is not established")
	}
	for _, kind := range f.LaunchObjectiveKinds {
		if kind != 0 && kind != 3 && f.ResultContext == nil {
			return fmt.Errorf("result progression admits destruction/count objectives and Training no-op only")
		}
	}
	expectedRecommendations := f.ActorCount
	if f.ResultContext != nil && f.ResultContext.RecommendationUnknown {
		expectedRecommendations = 0
	}
	if len(f.RecommendedSelectors) != expectedRecommendations || len(f.AcceptedSelectors) != f.ActorCount {
		return fmt.Errorf("result needs retained and accepted player aircraft-selector snapshots")
	}
	for _, selectors := range [][]byte{f.RecommendedSelectors, f.AcceptedSelectors} {
		for _, selector := range selectors {
			if selector > 7 {
				return fmt.Errorf("result aircraft selector outside source0..7")
			}
		}
	}
	if f.InactiveHeaderPolicy == SortieFreshInactiveHeaders {
		// Recognized original SHHD signedbyte6 values. This independently
		// catches a cockpit/serialized-selector alias at the normal adapter.
		values := [8]int8{0, -50, 25, 50, 50, 25, 25, 50}
		if f.PlayerAircraftValue != values[f.AcceptedSelectors[0]] {
			return fmt.Errorf("player aircraft score value disagrees with its accepted SHHD identity")
		}
	}
	if f.SelectorMultisetMatch != SortieSelectorMultisetMatch(f.RecommendedSelectors, f.AcceptedSelectors) {
		return fmt.Errorf("launch selector-multiset flag disagrees with retained selectors")
	}
	return validateResultContext(f)
}

func cloneSortiePilot(p *Pilot) *Pilot {
	if p == nil {
		return nil
	}
	raw, _ := json.Marshal(p)
	var out Pilot
	_ = json.Unmarshal(raw, &out)
	return &out
}

func sortieWord(p *Pilot, at int) int16 {
	return int16(binary.LittleEndian.Uint16(p.LegacyRaw[at : at+2]))
}
func sortieLong(p *Pilot, at int) int32 {
	return int32(binary.LittleEndian.Uint32(p.LegacyRaw[at : at+4]))
}
func sortiePutWord(p *Pilot, at int, value int16) {
	binary.LittleEndian.PutUint16(p.LegacyRaw[at:at+2], uint16(value))
}
func sortiePutLong(p *Pilot, at int, value int32) {
	binary.LittleEndian.PutUint32(p.LegacyRaw[at:at+4], uint32(value))
}

func syncSortieStatistics(p *Pilot) {
	for i, count := range p.Statistics.AwardCounts {
		p.LegacyRaw[0x24+i] = byte(count)
	}
	sortiePutLong(p, 0x66, p.Statistics.BestMissionScore)
	sortiePutLong(p, 0x6a, p.Statistics.LastMissionScore)
	sortiePutLong(p, 0x6e, p.Statistics.MissionCampaignCounter)
	sortiePutLong(p, 0x72, p.Statistics.CareerScore)
	p.LegacyRaw[0x54] = byte(p.Rank.Raw())
	p.LegacyRaw[0x65] = byte(p.MissionType.Raw())
}

// NormalizeSortieFlags is the legacy healthy-fact adapter. Complete N.4 facts
// use normalizeResult's ordered, recorded source normalizer instead.
func NormalizeSortieFlags(mission MissionType, flags byte) byte {
	if mission == MissionTraining {
		return flags & 0xcb
	}
	if flags&0x90 != 0 {
		return flags & 0x9f
	}
	return flags
}

func sortieDisqualifier(f SortieFacts) bool {
	for i, value := range f.ObjectValues {
		credit := (f.ObjectCredits[i/2] >> uint((i&1)*4)) & 15
		if credit == 1 && value < 0 {
			return true
		}
	}
	return false
}

// ServiceRetirement preserves the historical interpretation for pending results
// written before the original remainder helper was identified.
func (p *SortieProposal) ServiceRetirement(pilot *Pilot) bool {
	if p.Facts.ResultContext != nil && p.Facts.ResultContext.RetireByMissions {
		return pilot.Statistics.Missions() >= 50 && !(pilot.Campaign != nil && pilot.Campaign.Phase == CampaignActive && CampaignTerminal(pilot.Campaign, pilot.MissionType) == 0)
	}
	return pilot.Statistics.Campaigns() >= 50
}

// ProposeSortie follows GS scoring, commander selection, main award/rank commit
// ordering, but does not write a save or mutate the caller's final facts/pilot.
func ProposeSortie(p *Pilot, facts SortieFacts, tables SortieTables) (*SortieProposal, error) {
	version := SortieProposalVersion
	if facts.ResultContext != nil && facts.ResultContext.GameplayPolicy == SafeGameplayPolicy {
		version = SafeSortieProposalVersion
	}
	return proposeSortieVersion(p, facts, tables, version)
}

func proposeSortieVersion(p *Pilot, facts SortieFacts, tables SortieTables, version int) (*SortieProposal, error) {
	return proposeSortieVersionChoices(p, facts, tables, version, nil)
}

// A v4 CREWPRO decision precedes the replacement initializer. Reconstructing a
// frozen CRT branch from its accepted source operands consumes no live RNG.
type sortieCrewBranchChoices struct {
	Accepted bool
	Choices  *SortieCrewChoices
}

func proposeSortieVersionChoices(p *Pilot, facts SortieFacts, tables SortieTables, version int, chosen *sortieCrewBranchChoices) (*SortieProposal, error) {
	if version != 1 && version != 2 && version != 3 && version != SortieProposalVersion && version != SafeSortieProposalVersion {
		return nil, fmt.Errorf("unsupported sortie proposal interpretation %d", version)
	}
	safe := facts.ResultContext != nil && facts.ResultContext.GameplayPolicy == SafeGameplayPolicy
	if safe != (version == SafeSortieProposalVersion) {
		return nil, fmt.Errorf("sortie proposal version differs from frozen gameplay policy")
	}
	if !safe && facts.ResultContext != nil && facts.ResultContext.MissionRecovery != nil && (facts.ResultContext.MissionRecovery.Policy == OptionalMissionRecoveryPolicy || facts.ResultContext.MissionRecovery.ObjectiveRevisions != nil) {
		return nil, fmt.Errorf("historical result cannot contain R5 recovery provenance")
	}
	if version < 3 && facts.MissionType == MissionSimulated {
		return nil, fmt.Errorf("historical proposals do not admit authored Simulation")
	}
	if version == 1 && facts.ResultContext != nil && facts.ResultContext.MissionRecovery != nil {
		return nil, fmt.Errorf("historical result cannot contain a later mission-recovery policy")
	}
	if err := facts.Validate(); err != nil {
		return nil, err
	}
	if err := tables.Validate(); err != nil {
		return nil, err
	}
	if p == nil || p.Status != StatusActive || p.MissionType != facts.MissionType || p.Rank.Raw() < 0 || len(p.LegacyRaw) != LegacyRecordSize || p.Statistics == nil || len(p.Statistics.AwardCounts) != 8 {
		return nil, fmt.Errorf("result needs an active matching pilot with complete retained progression fields")
	}
	if ctx := facts.ResultContext; ctx != nil {
		if !validGameplayPolicy(ctx.GameplayPolicy) {
			return nil, fmt.Errorf("unsupported result gameplay policy")
		}
		if ctx.GameplayPolicy == SafeGameplayPolicy && facts.MissionType == MissionTraining {
			if ctx.Campaign != nil || facts.LaunchSwitches&0x400 == 0 {
				return nil, fmt.Errorf("Training needs its protected independent result owner")
			}
			want := ""
			if p.Campaign != nil {
				raw, e := json.Marshal(p.Campaign)
				if e != nil {
					return nil, e
				}
				want = assetdata.Hash(raw)
			}
			if ctx.RetainedCampaignSHA256 != want {
				return nil, fmt.Errorf("Training retained Campaign owner differs from accepted launch")
			}
		} else if ctx.RetainedCampaignSHA256 != "" {
			return nil, fmt.Errorf("retained Training Campaign owner on unrelated result")
		}
	}
	if facts.MissionType == MissionCampaign && p.Rank.Raw() < 5 {
		return nil, fmt.Errorf("Campaign results require the admitted commissioned-leader domain; warrant crossmode history is unsupported")
	}
	if p.Statistics.MissionCampaignCounter < 0 || facts.ResultContext == nil && p.Statistics.MissionCampaignCounter >= 5000 {
		return nil, fmt.Errorf("retained packed mission/Campaign counter is outside the admitted nonnegative active-career domain")
	}
	// Source loss-related AD59 aliases the first retained squadmate tail. Until
	// loss producers are implemented, a nonzero retained owner is not defaulted.
	if facts.ResultContext == nil && sortieWord(p, 0xa3) != 0 {
		return nil, fmt.Errorf("retained loss-related progression state is outside the admitted result boundary")
	}
	copyFactsRaw, err := json.Marshal(facts)
	if err != nil {
		return nil, fmt.Errorf("freeze sortie facts: %w", err)
	}
	var copyFacts SortieFacts
	if err = json.Unmarshal(copyFactsRaw, &copyFacts); err != nil {
		return nil, fmt.Errorf("freeze complete sortie facts: %w", err)
	}
	hash := sha256.Sum256(copyFactsRaw)
	out := &SortieProposal{Version: version, Facts: copyFacts, FactsDigest: hex.EncodeToString(hash[:]), Tables: tables, PilotBefore: cloneSortiePilot(p), ScoredPilot: cloneSortiePilot(p), MedalIndex: -1}
	flags := NormalizeSortieFlags(facts.MissionType, facts.SourceFlags)
	if facts.ResultContext != nil {
		out.Result, err = normalizeResult(copyFacts)
		if err != nil {
			return nil, err
		}
		flags = out.Result.NormalizedFlags[0]
		// AD59 is one retained 32-bit owner, overlapping the first crew tail.
		sortiePutLong(out.ScoredPilot, 0xa3, sortieLong(out.ScoredPilot, 0xa3)+int32(out.Result.LostFollowers))
		// SourceA19eb7..19ed0 publishes the accepted map suffix/prior suffix
		// separately from SNHD ordinal. Training skips only the suffix pair.
		if facts.MissionType != MissionTraining {
			out.ScoredPilot.LegacyRaw[0x5c] = p.LegacyRaw[0x5b]
			out.ScoredPilot.LegacyRaw[0x5b] = facts.ResultContext.SelectedScenario
		}
		out.ScoredPilot.LegacyRaw[0x58] = facts.ResultContext.ScenarioOrdinal
		out.ScoredPilot.LegacyRaw[0x56] = 1
		out.ScoredPilot.LegacyStatus = 1
	}
	value := facts.RawScore
	out.Score = SortieScore{NormalizedFlags: flags, RawScore: value, TimeFactor: 100, ElapsedDA: facts.ElapsedDA, CareerApplied: facts.LaunchSwitches&0x400 == 0}
	for i, objectValue := range facts.ObjectValues {
		if (facts.ObjectCredits[i/2]>>uint((i&1)*4))&15 == 7 {
			value -= objectValue / 2
		}
	}
	out.Score.AfterObjectCredit = value
	if flags&1 != 0 {
		value += 100
	}
	if flags&2 != 0 {
		value += 50
	}
	out.Score.AfterObjectiveBonus = value
	out.TrainingSuccess = facts.MissionType == MissionTraining && value >= 100 && flags&0x10 == 0
	out.Success = !sortieDisqualifier(facts) && ((value >= 50 && flags&3 != 0) || out.TrainingSuccess)
	if facts.ResultContext != nil && facts.ResultContext.Campaign != nil {
		input := *facts.ResultContext.Campaign
		input.FinalObjects = facts.FinalObjects
		input.Credits = facts.ObjectCredits
		input.NormalizedFlags = flags
		input.ScoredSuccess = out.Success
		copy(input.FollowerFlags[:], out.Result.NormalizedFlags[1:])
		out.Result.Campaign, err = PrepareCampaignResult(input)
		if err != nil {
			return nil, err
		}
		value += out.Result.Campaign.ScoreBonus
		out.ScoredPilot.Campaign, err = CloneCampaign(out.Result.Campaign.Campaign)
		if err != nil {
			return nil, err
		}
	}
	total := int32(value)
	if facts.LaunchSwitches&0x400 == 0 && flags&3 != 0 {
		shipValue := int16(facts.PlayerAircraftValue)
		if shipValue > 0 {
			shipValue *= 2
		}
		if flags&0x80 != 0 || shipValue < 0 {
			out.Score.AircraftContribution = shipValue
		}
		// A1e8c7..1e90c visits all four readiness headers even in Single.
		// Inactive result statuses0 omit positive values, but negative table
		// values still contribute. These are not fabricated follower metrics.
		for i, value := range facts.InactiveAircraftValues {
			v := int16(value)
			// GS A1e8f1..1e90a adds each follower once; only the earlier player
			// path doubles positive values. Preserve the old bug solely to verify
			// immutable version1 records, never to score a newly launched sortie.
			if version == 1 && out.Result != nil && i+1 < facts.ActorCount && v > 0 {
				v *= 2
			}
			if v < 0 || out.Result != nil && i+1 < facts.ActorCount && out.Result.NormalizedFlags[i+1]&0x80 != 0 {
				out.Score.AircraftContribution += v
			}
		}
		switch facts.LaunchSwitches & 3 {
		case 0:
			out.Score.AircraftContribution = 0
		case 1:
			out.Score.AircraftContribution /= 2
		}
		total += int32(out.Score.AircraftContribution)
	}
	if facts.SelectorMultisetMatch {
		total += 50
	}
	out.Score.PreScale = total
	// Native int32 arithmetic deliberately retains the original low32 multiply.
	score := total * int32(facts.Coefficient) / 10
	if out.Score.ElapsedDA > 25200 {
		out.Score.ElapsedDA = 25200
	}
	if score > 0 && out.Score.ElapsedDA >= 10800 {
		out.Score.TimeFactor = 100 + int16(-50*int32(out.Score.ElapsedDA-10800)/14400)
		score = score * int32(out.Score.TimeFactor) / 100
	}
	out.Score.Score = score
	q := out.ScoredPilot
	q.LegacyRaw[0x33] = 0 // ACE9 cleared by scoring; later commander re-runs Training helper.
	if out.Score.CareerApplied {
		q.Statistics.LastMissionScore = score
		q.Statistics.CareerScore += score
		if score > q.Statistics.BestMissionScore {
			q.Statistics.BestMissionScore = score
		}
		q.Statistics.MissionCampaignCounter++
		penalty := sortieWord(q, 0x59)
		if flags&0x80 == 0 && penalty < 3000 {
			penalty += 200
		}
		if flags&3 == 0 && penalty < 3000 {
			penalty += 200
		}
		sortiePutWord(q, 0x59, penalty)
		out.Score.ExportedScoreWord = int16(score)
	}
	syncSortieStatistics(q)
	// The controller exports BC4A for Training but AD20 for other missions.
	// In an explicit protected non-Training supplied case AD20 is unchanged.
	if facts.MissionType != MissionTraining {
		out.Score.ExportedScoreWord = int16(q.Statistics.LastMissionScore)
	}
	if out.Result != nil {
		if err = prepareCrewResults(out); err != nil {
			return nil, err
		}
	}
	out.InitialRankRecommendation = sortieOrdinaryRank(q, tables)
	if out.RecoveredPilot, err = prepareMissionRecovery(out); err != nil {
		return nil, err
	}
	// A1EA6C..A1EAA6 publishes861c before commander/award acceptance. The separate
	// roster-wide MIA mission-rescue writer is excluded by the freeze boundary.
	out.SpecialOutcome = out.Success && out.Score.CareerApplied &&
		(sortieInitialMedalQuery(q, out) == 0 || out.InitialRankRecommendation == 11 || out.RecoveredPilot != nil || out.Result != nil && out.Result.Campaign != nil && out.Result.Campaign.Verdict > 0)
	out.CommanderRank = sortieCommanderRank(q, out, tables)
	out.Commission = out.CommanderRank >= 5
	if out.Result != nil && out.Result.Loss {
		out.CommanderRank = 0
		out.Commission = false
	}
	out.AcceptedPilot, out.Awards, out.MedalIndex, out.MedalDeferred, out.PenaltyCredit, out.RecommendedRank, out.Retirement, err = resolveSortieBranch(out, true)
	if err != nil {
		return nil, err
	}
	out.RefusedPilot, _, _, _, _, _, _, err = resolveSortieBranch(out, false)
	if err != nil {
		return nil, err
	}
	if out.Result != nil {
		out.Result.CrewPro = !out.Result.Loss && facts.MissionType != MissionTraining && !out.Commission && facts.ActorCount > 1
		// v4 freezes two separate CRT continuations before the UI chooses. This
		// is pure branch construction, never two draws from one live RNG owner.
		var refused *SortieResultDetails
		if version >= 4 && out.Commission {
			b, _ := json.Marshal(out.Result)
			refused = new(SortieResultDetails)
			if err = json.Unmarshal(b, refused); err != nil {
				return nil, err
			}
		}
		var acceptedChoices, refusedChoices *SortieCrewChoices
		if chosen != nil {
			if chosen.Accepted || !out.Commission {
				acceptedChoices = chosen.Choices
			} else {
				refusedChoices = chosen.Choices
			}
		}
		if err = prepareResultPresentationAndReplacements(out, acceptedChoices); err != nil {
			return nil, err
		}
		if refused != nil {
			branch := *out
			branch.Result, branch.AcceptedPilot, branch.Commission = refused, out.RefusedPilot, false
			_, _, _, _, _, _, branch.Retirement, err = resolveSortieBranch(out, false)
			if err != nil {
				return nil, err
			}
			if err = prepareResultPresentationAndReplacements(&branch, refusedChoices); err != nil {
				return nil, err
			}
			out.RefusedResult = refused
		}
	}
	return out, nil
}

func sortieInitialMedalQuery(p *Pilot, out *SortieProposal) int16 {
	// This preserves the actual AX query convention, not a modern optional
	// medal enum: no objective returns0; an exhausted threshold loop returns
	// the last threshold250, not0. Source241C/242E is used by861c's caller.
	if sortieDisqualifier(out.Facts) {
		return 1
	}
	if out.Score.NormalizedFlags&3 == 0 {
		return 0
	}
	start := 2
	if out.Facts.LaunchSwitches&3 >= 2 {
		start = 1
	}
	if out.Score.NormalizedFlags&0x83 == 0x83 && out.Facts.LaunchSwitches&3 >= 2 && p.Statistics.AwardCounts[0] == 0 {
		start = 0
	}
	for i := start; i < 6; i++ {
		if p.Statistics.LastMissionScore < int32(out.Tables.MedalThresholds[i]) {
			continue
		}
		if int16(int8(p.LegacyRaw[0x2c+i]))+1 < int16(p.Statistics.AwardCounts[i]) {
			return -1
		}
		return int16(i)
	}
	return out.Tables.MedalThresholds[5]
}

func sortieOrdinaryRank(p *Pilot, t SortieTables) int {
	rank := p.Rank.Raw()
	if p.Statistics.CareerScore <= sortieLong(p, 0x7a) || rank >= 11 || rank == 4 || p.Statistics.LastMissionScore < 50 {
		return 0
	}
	if rank == 10 && p.Campaign != nil && p.Campaign.Phase == CampaignActive && CampaignTerminal(p.Campaign, p.MissionType) == 0 {
		return 0
	}
	threshold := t.RankThresholds[rank+1] + sortieWord(p, 0x59)
	if p.Statistics.CareerScore < int32(threshold) {
		return 0
	}
	return rank + 1
}

func sortieCommissionRank(p *Pilot, t SortieTables) int {
	rank := p.Rank.Raw()
	for candidate := rank + 4; candidate >= 5; candidate-- {
		if candidate >= len(t.RankThresholds) {
			continue
		}
		threshold := t.RankThresholds[candidate] + sortieWord(p, 0x59)
		if p.Statistics.CareerScore >= int32(threshold) && p.Statistics.CareerScore > sortieLong(p, 0x7a) && rank <= 4 && p.Statistics.LastMissionScore >= 100 {
			return candidate
		}
	}
	if rank == 0 {
		return sortieOrdinaryRank(p, t)
	}
	if p.Statistics.MissionCampaignCounter >= 6 && p.Statistics.LastMissionScore >= 100 {
		return 5
	}
	return 0
}

func sortieCommanderRank(p *Pilot, out *SortieProposal, t SortieTables) int {
	rank := p.Rank.Raw()
	candidate := 0
	if out.TrainingSuccess && rank == 0 {
		candidate = 1
	} else {
		candidate = sortieCommissionRank(p, t)
		if out.Score.NormalizedFlags&3 == 0 || out.Score.NormalizedFlags&0x10 != 0 {
			candidate = 0
		}
	}
	if candidate <= rank {
		candidate = 0
	}
	if rank > 1 && sortieOrdinaryRank(p, t) == 0 {
		candidate = 0
	}
	return candidate
}

func sortieSetRank(p *Pilot, rank int, commander bool) error {
	r, err := RankFromRaw(rank)
	if err != nil {
		return err
	}
	p.Rank = r
	sortiePutLong(p, 0x7a, p.Statistics.CareerScore)
	// Only the commander acceptance branch changes next mission type. The
	// later ordinary promotion callback writes rank/snapshot alone.
	if commander {
		if rank < 5 {
			p.MissionType = MissionSingle
		} else {
			p.MissionType = MissionFlight
		}
	}
	return nil
}

func resolveSortieBranch(out *SortieProposal, acceptCommission bool) (*Pilot, [8]int8, int, bool, int16, int, bool, error) {
	p := cloneSortiePilot(out.ScoredPilot)
	var awards [8]int8
	medal := -1
	deferred := false
	credit := int16(0)
	recommended := out.CommanderRank
	if out.TrainingSuccess {
		p.LegacyRaw[0x33] = 1
	}
	if out.CommanderRank > 5 {
		p.LegacyRaw[0x55] |= 0x10 // Source commander presentation callback, before accept/refuse.
	}
	if out.CommanderRank > 0 && (!out.Commission || acceptCommission) {
		if err := sortieSetRank(p, out.CommanderRank, true); err != nil {
			return nil, awards, medal, false, credit, 0, false, err
		}
	}
	// SourceA1e48a concludes Campaign BEFORE the main award/rank/JUMP path.
	// The qualification copy below and later retirement query therefore read
	// the transformed55 and packed counter including a completed Campaign+100.
	if out.Result != nil && out.Result.Campaign != nil {
		if err := FinishCampaignResult(p, out.Result.Campaign, out.Score.NormalizedFlags); err != nil {
			return nil, awards, medal, false, credit, 0, false, err
		}
	}
	// Section3:242e commits before the main ordinary rank query. Its medal
	// credit is subtracted only AFTER that query; it cannot qualify this rank.
	if p.LegacyRaw[0x33] != 0 {
		if p.Statistics.AwardCounts[7] == 0 {
			p.Statistics.AwardCounts[7]++
			awards[7]++
		}
	} else if !sortieDisqualifier(out.Facts) && out.Score.NormalizedFlags&3 != 0 {
		start := 2
		if out.Facts.LaunchSwitches&3 >= 2 {
			start = 1
		}
		if out.Score.NormalizedFlags&0x83 == 0x83 && out.Facts.LaunchSwitches&3 >= 2 && p.Statistics.AwardCounts[0] == 0 {
			start = 0
		}
		for i := start; i < 6; i++ {
			if p.Statistics.LastMissionScore < int32(out.Tables.MedalThresholds[i]) {
				continue
			}
			medal = i
			repeat := int16(int8(p.LegacyRaw[0x2c+i]))
			if repeat+1 < int16(p.Statistics.AwardCounts[i]) {
				p.LegacyRaw[0x2c+i]++
				deferred = true
			} else {
				p.Statistics.AwardCounts[i]++
				p.LegacyRaw[0x2c+i] = 0
				awards[i]++
				credit += out.Tables.MedalPenaltyCredits[i]
				sortiePutLong(p, 0x76, p.Statistics.CareerScore)
			}
			break
		}
	}
	if out.Result != nil && out.Score.NormalizedFlags&0x14 != 0 {
		p.Statistics.AwardCounts[6]++
		awards[6]++
	}
	if out.Result != nil && out.Result.Loss {
		p.LegacyStatus = 4
		p.Status = StatusKIA
		if out.Score.NormalizedFlags&8 != 0 {
			p.LegacyStatus = 3
			p.Status = StatusMIA
		}
		p.LegacyRaw[0x56] = p.LegacyStatus
		// The KIA controller commits the award helper, then returns before
		// ordinary promotion/crew/Campaign conclusion and qualification copy.
		syncSortieStatistics(p)
		return p, awards, medal, deferred, credit, 0, false, nil
	}
	if p.Rank.Raw() != 0 && p.LegacyRaw[0x55]&3 == 0 {
		p.LegacyRaw[0x55] |= 1
	}
	// Source JUMP( rescuedRosterRow ) skips its ordinary rank query/action.
	// Commander acceptance already occurred before A1e472 and remains separate.
	if out.Score.NormalizedFlags&3 != 0 && out.RecoveredPilot == nil {
		if r := sortieOrdinaryRank(p, out.Tables); r > 0 {
			if err := sortieSetRank(p, r, false); err != nil {
				return nil, awards, medal, deferred, credit, 0, false, err
			}
			if recommended == 0 || out.Commission && !acceptCommission {
				recommended = r
			}
		}
	}
	// Section3:1de4..1df8 updates the retained previous qualification bits
	// after the main debrief. Preserve all unrelated packed bits.
	p.LegacyRaw[0x55] = p.LegacyRaw[0x55]&0xf3 | (p.LegacyRaw[0x55]&3)<<2
	sortiePutWord(p, 0x59, sortieWord(p, 0x59)-credit)
	oldRank := out.PilotBefore.Rank.Raw()
	if oldRank <= 1 && p.Rank.Raw() > 1 {
		penalty := sortieWord(p, 0x59)
		half := penalty / 2
		half += 99
		sortiePutWord(p, 0x59, (half/100)*100)
	}
	activeCampaign := p.Campaign != nil && p.Campaign.Phase == CampaignActive && CampaignTerminal(p.Campaign, p.MissionType) == 0
	retired := p.Rank.Raw() == 11 || out.ServiceRetirement(p) && !activeCampaign || sortieWord(p, 0xa3) >= 16
	if retired {
		p.Status = StatusRetired
		p.LegacyStatus = 2
		p.LegacyRaw[0x56] = 2
	}
	syncSortieStatistics(p)
	return p, awards, medal, deferred, credit, recommended, retired, nil
}

// ValidateSortieProposal rederives immutable proposal data before it is consumed
// from disk. It does not trust serialized recommendations or an edited pilot.
func ValidateSortieProposal(p *SortieProposal) error {
	if p == nil || p.Version != 1 && p.Version != 2 && p.Version != 3 && p.Version != SortieProposalVersion && p.Version != SafeSortieProposalVersion {
		return fmt.Errorf("unsupported pending sortie proposal")
	}
	if p.Version < SafeSortieProposalVersion && p.RecoveryDecision != nil {
		return fmt.Errorf("historical proposal cannot contain an optional recovery decision")
	}
	want, err := proposeSortieVersion(p.PilotBefore, p.Facts, p.Tables, p.Version)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(want, p) {
		return fmt.Errorf("pending sortie proposal differs from its immutable source facts")
	}
	return nil
}

func ResolveSortieProposal(p *SortieProposal, acceptCommission bool) (*Pilot, error) {
	return ResolveSortieProposalWithChoices(p, acceptCommission, nil)
}
