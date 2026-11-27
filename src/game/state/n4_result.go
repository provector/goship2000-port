package state

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

// SortieResultContext is the versioned, source-owned extension to N's frozen
// facts. Actor rows are actual participants, never reserved rescue objects.
type SortieResultContext struct {
	RecommendationUnknown  bool                    `json:"recommendation_unknown,omitempty"`
	Version                int                     `json:"version"`
	Scene                  *byte                   `json:"scene,omitempty"`
	LaunchPose             InitializedPose         `json:"launch_pose"`
	Actors                 []SortieActorFacts      `json:"actors"`
	DynamicKinds           [80]byte                `json:"dynamic_kinds"`
	WorldPoses             [80]InitializedPose     `json:"world_poses"`
	SetupFlags             byte                    `json:"setup_flags"`
	SelectedScenario       byte                    `json:"selected_scenario"`
	ScenarioOrdinal        byte                    `json:"scenario_ordinal"`
	Seed                   uint32                  `json:"seed"`
	SeedPolicy             string                  `json:"seed_policy"`
	MusicOverride          *int                    `json:"music_override,omitempty"`
	ReplacementNames       []string                `json:"replacement_names"`
	OccupiedNames          []string                `json:"occupied_names"`
	Campaign               *CampaignResultInput    `json:"campaign,omitempty"`
	MissionRecovery        *MissionRecoveryContext `json:"mission_recovery,omitempty"`
	ReservedRescues        []SortieReservedRescue  `json:"reserved_rescues,omitempty"`
	RetireByMissions       bool                    `json:"retire_by_missions,omitempty"`
	GameplayPolicy         string                  `json:"gameplay_policy,omitempty"`
	RetainedCampaignSHA256 string                  `json:"retained_campaign_sha256,omitempty"`
}

func (c *SortieResultContext) UnmarshalJSON(data []byte) error {
	type plain SortieResultContext
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	t := reflect.TypeOf(plain{})
	known := map[string]bool{}
	for i := 0; i < t.NumField(); i++ {
		tag := strings.Split(t.Field(i).Tag.Get("json"), ",")
		name := tag[0]
		known[name] = true
		raw, ok := fields[name]
		optional := len(tag) > 1 && tag[1] == "omitempty"
		if (!ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null"))) && !optional {
			return fmt.Errorf("frozen result context %s missing/null", name)
		}
	}
	for name := range fields {
		if !known[name] {
			return fmt.Errorf("unknown frozen result context %s", name)
		}
	}
	exact := func(raw json.RawMessage, n int) error {
		var values []json.RawMessage
		if err := json.Unmarshal(raw, &values); err != nil || len(values) != n {
			return fmt.Errorf("frozen result context array requires%d elements", n)
		}
		for _, v := range values {
			if bytes.Equal(bytes.TrimSpace(v), []byte("null")) {
				return fmt.Errorf("frozen result context array contains null")
			}
		}
		return nil
	}
	pose := func(raw json.RawMessage) error {
		var p map[string]json.RawMessage
		if err := json.Unmarshal(raw, &p); err != nil || len(p) != 4 {
			return fmt.Errorf("frozen context pose requires four fields")
		}
		for _, name := range []string{"H0", "H1", "Height", "Angles"} {
			v, ok := p[name]
			if !ok || bytes.Equal(bytes.TrimSpace(v), []byte("null")) {
				return fmt.Errorf("frozen context pose %s missing/null", name)
			}
		}
		return exact(p["Angles"], 3)
	}
	if err := pose(fields["launch_pose"]); err != nil {
		return err
	}
	if err := exact(fields["dynamic_kinds"], 80); err != nil {
		return err
	}
	if err := exact(fields["world_poses"], 80); err != nil {
		return err
	}
	var worldPoses []json.RawMessage
	if err := json.Unmarshal(fields["world_poses"], &worldPoses); err != nil {
		return err
	}
	for _, p := range worldPoses {
		if err := pose(p); err != nil {
			return err
		}
	}
	var actors []map[string]json.RawMessage
	if err := json.Unmarshal(fields["actors"], &actors); err != nil {
		return err
	}
	at := reflect.TypeOf(SortieActorFacts{})
	for _, a := range actors {
		if len(a) != at.NumField() {
			return fmt.Errorf("frozen actor needs every typed field")
		}
		for i := 0; i < at.NumField(); i++ {
			name := strings.Split(at.Field(i).Tag.Get("json"), ",")[0]
			raw, ok := a[name]
			if !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
				return fmt.Errorf("frozen actor %s missing/null", name)
			}
		}
		for _, shape := range []struct {
			name string
			n    int
		}{{"damage", 12}, {"ship", 30}, {"stored_raw", 41}} {
			if err := exact(a[shape.name], shape.n); err != nil {
				return err
			}
		}
		if err := pose(a["pose"]); err != nil {
			return err
		}
	}
	if raw, ok := fields["reserved_rescues"]; ok {
		var rescues []map[string]json.RawMessage
		if err := json.Unmarshal(raw, &rescues); err != nil {
			return err
		}
		for _, rescue := range rescues {
			if len(rescue) != 3 {
				return fmt.Errorf("frozen reserve requires actor/kind/carrier")
			}
			for _, name := range []string{"actor", "kind", "carrier"} {
				if b, ok := rescue[name]; !ok || bytes.Equal(bytes.TrimSpace(b), []byte("null")) {
					return fmt.Errorf("frozen reserve %s missing/null", name)
				}
			}
		}
	}
	var value plain
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	if value.Version < 3 && value.Scene != nil {
		return fmt.Errorf("historical result context cannot contain scene")
	}
	if value.Version == 3 && (value.Scene == nil || *value.Scene > 5) {
		return fmt.Errorf("result context3 needs explicit source scene0..5")
	}
	*c = SortieResultContext(value)
	return nil
}

// SortieReservedRescue preserves actor+4 carrier ownership, not extra crew.
// Version2 only: v1 proposals never reinterpret an omitted carrier as player0.
type SortieReservedRescue struct {
	Actor   int  `json:"actor"`
	Kind    byte `json:"kind"`
	Carrier byte `json:"carrier"`
}

type SortieActorFacts struct {
	Actor            int             `json:"actor"`
	SourceIndex      int             `json:"source_index"`
	Name             string          `json:"name"`
	Callsign         string          `json:"callsign"`
	Group            int             `json:"group"`
	FlightLeader     bool            `json:"flight_leader"`
	RankRaw          int             `json:"rank"`
	AcceptedSelector byte            `json:"accepted_selector"`
	Kind             byte            `json:"kind"`
	Control          byte            `json:"control"`
	Leader           int             `json:"leader"`
	Pose             InitializedPose `json:"pose"`
	Damage           [12]byte        `json:"damage"`
	Ship             [30]byte        `json:"ship"`
	RawA             byte            `json:"raw_a"`
	RawB             byte            `json:"raw_b"`
	Flags            byte            `json:"flags"`
	StoredRaw        [41]byte        `json:"stored_raw"`
}

// Raw A/B are source error quantities. No autonomous decisions are enabled by
// their initialization; absent route/fire events leave these values unchanged.
type SourceCrewPerformance struct {
	RawA, RawB byte
	SkillKnown bool
}

func InitializeCrewPerformance(s *RuntimeStores, crew []FlightPilot, count int) ([]SourceCrewPerformance, error) {
	if s == nil || count < 1 || count > len(s.Ships) || count > len(crew) {
		return nil, fmt.Errorf("crew performance needs actual accepted rows")
	}
	out := make([]SourceCrewPerformance, count)
	for i := 0; i < count; i++ {
		skill, err := ComputeSourceProficiency(crew[i])
		if err != nil {
			// Unknown imported history keeps preparation/flight available, but
			// cannot later masquerade as a complete result producer.
			continue
		}
		s.Ships[i].Proficiency = new(byte)
		*s.Ships[i].Proficiency = skill
		s.Blueprint[i].Proficiency = new(byte)
		*s.Blueprint[i].Proficiency = skill
		out[i] = SourceCrewPerformance{RawB: skill, SkillKnown: true}
	}
	return out, nil
}

// ApplyOrdinaryCrewRoutePoint is the conditional source GPS transaction seam;
// callers supply four already-owned gameplay draws only after these gates.
func (m *InitializedMission) ApplyOrdinaryCrewRoutePoint(actor int, h0 uint32, control byte, cargoSpecial bool, words [4]uint16) bool {
	if m == nil || actor < 1 || actor >= len(m.CrewPerformance) || h0 == 0 || control != 3 || cargoSpecial || !m.CrewPerformance[actor].SkillKnown || m.Stores == nil || m.Stores.Ships[actor].Proficiency == nil || *m.Stores.Ships[actor].Proficiency == 0 {
		return false
	}
	p := &m.CrewPerformance[actor]
	p.RawA = SourceRouteErrorPoint(p.RawA, *m.Stores.Ships[actor].Proficiency, words)
	return true
}

func (m *InitializedMission) ApplyAcceptedCrewFire(actor int, random7 byte, threshold uint16) (bool, error) {
	if m == nil || actor < 1 || actor >= len(m.CrewPerformance) || !m.CrewPerformance[actor].SkillKnown {
		return false, fmt.Errorf("accepted follower fire lacks actual initialized actor")
	}
	b, success := SourceAcceptedFireError(m.CrewPerformance[actor].RawB, random7, threshold)
	m.CrewPerformance[actor].RawB = b
	return success, nil
}

type ResultRandomDraw struct {
	Site  string `json:"site"`
	Bound int    `json:"bound"`
	Raw15 uint16 `json:"raw15"`
	Value int    `json:"value"`
	State uint32 `json:"state"`
}
type ResultRandomState struct {
	Seed   uint32             `json:"seed"`
	State  uint32             `json:"state"`
	Policy string             `json:"policy"`
	Draws  []ResultRandomDraw `json:"draws"`
}

const ResultSeedPolicy = "native-sortie-sha256-le32-v1"

// ResultSeed is a native replacement for GS's BIOS tick seed. It is bound to the
// immutable sortie identity once; it is unrelated to GS2's gameplay LFSR.
func ResultSeed(sortieID string) uint32 {
	h := sha256.Sum256([]byte("GS-result-v1\x00" + sortieID))
	return binary.LittleEndian.Uint32(h[:4])
}

func (r *ResultRandomState) Draw(site string, bound int) int {
	if bound <= 1 {
		return 0
	} // Original helper does not initialize/draw here.
	r.State = r.State*214013 + 2531011
	raw := uint16(r.State>>16) & 32767
	v := int(uint32(raw) * uint32(bound) >> 15)
	r.Draws = append(r.Draws, ResultRandomDraw{site, bound, raw, v, r.State})
	return v
}

type SortieCrewMetric struct {
	Actor, SourceIndex            int
	Name                          string
	Rank                          int
	Flags                         byte
	Flight, Attack, Defense, Mean int
}
type SortieCrewRecommendation struct {
	Type        int `json:"type"` // 0 promotion; 1..5 original medal indices.
	Actor       int `json:"actor"`
	SourceIndex int `json:"source_index"`
}
type SortieCrewChoices struct {
	Recipients []int                `json:"recipients"`
	Names      []string             `json:"names,omitempty"`
	Result     *SortieResultDetails `json:"source_result,omitempty"` // v4 frozen post-CREWPRO continuation
}

type SortieResultDetails struct {
	NormalizedFlags    [5]byte                    `json:"normalized_flags"`
	Crew               []SortieCrewMetric         `json:"crew"`
	Recommendations    []SortieCrewRecommendation `json:"recommendations"`
	CrewPro            bool                       `json:"crew_pro"`
	RescuerActor       int                        `json:"rescuer_actor"`
	RescueTextVariant  int                        `json:"rescue_text_variant"`
	Loss               bool                       `json:"loss"`
	LostFollowers      int                        `json:"lost_followers"`
	Distance           int16                      `json:"distance"`
	Threshold          int16                      `json:"threshold"`
	RNG                ResultRandomState          `json:"rng"`
	Campaign           *CampaignResultProposal    `json:"campaign,omitempty"`
	Replacements       []SquadronPilot            `json:"replacements,omitempty"`
	ReplacementCount   int                        `json:"replacement_count,omitempty"`
	ReplacementInitial bool                       `json:"replacement_initial,omitempty"`
	MusicCue           uint16                     `json:"music_cue"`
	FeedbackVariants   map[string]uint32          `json:"feedback_variants"`
}

// ComputeSourceProficiency is GS1c89:0452..04bd. The rank10/11 lookup overlaps
// the adjacent medal-weight bytes; low-word MUL and signed IDIV are intentional.
func ComputeSourceProficiency(row FlightPilot) (byte, error) {
	if row.SourceIndex == 0 {
		return 0, nil
	}
	if row.Incomplete || row.Rank.Raw() < 0 || row.Rank.Raw() > 11 || len(row.AwardCounts) != 8 {
		return 0, fmt.Errorf("accepted crew proficiency requires complete rank/medal provenance")
	}
	ranks := [12]int8{0, 50, 45, 40, 35, 30, 25, 20, 15, 10, 0, -10}
	weights := [8]int8{0, -10, -5, -3, -2, -1, 3, 0}
	total := int16(ranks[row.Rank.Raw()])
	for i, c := range row.AwardCounts {
		total += int16(c) * int16(weights[i])
	}
	if total < 10 {
		total = 10
	}
	return byte(int16(uint16(total)*255) / 100), nil
}

// SourceRouteErrorPoint is called only by an admitted ordinary follower GPS
// transaction. Its four words belong to the gameplay stream, not result RNG.
func SourceRouteErrorPoint(old, skill byte, words [4]uint16) byte {
	if skill == 0 {
		return old
	}
	for i := 0; i < 4; i += 2 {
		e := byte(words[i]^words[i+1]) % skill
		old = byte((uint16(old) + uint16(e)) / 2)
	}
	return old
}

func SourceAcceptedFireError(old byte, random7 byte, threshold uint16) (byte, bool) {
	if uint16(random7&127) >= threshold {
		return old - (old >> 2), true
	}
	return old + ((255 - old) >> 2), false
}

func sourceHealth(damage [12]byte) uint16 {
	h := int16(20)
	for _, b := range damage[:10] {
		h -= int16(b & 3)
	}
	return uint16(13*h - 5)
}

// FinalResultMetric reproduces SAR32/4, low-word subtraction, ABS16 and wrapped
// L1 sum. These are native coordinates; replacing this with metres is incorrect.
func FinalResultMetric(a, b InitializedPose) uint16 {
	abs := func(v int16) uint16 {
		if v < 0 {
			return uint16(-v)
		}
		return uint16(v)
	}
	return abs(int16(int32(a.H0)>>4)-int16(int32(b.H0)>>4)) + abs(int16(int32(a.H1)>>4)-int16(int32(b.H1)>>4))
}

// FinalizeSortieFacts constructs result0268 and pre-normalization flags without
// advancing a dying object/projectile. Its input is the frozen world, not AE9.
func FinalizeSortieFacts(f *SortieFacts) error {
	if f == nil || f.ResultContext == nil {
		return fmt.Errorf("source finalizer needs frozen result context")
	}
	c := f.ResultContext
	if !validGameplayPolicy(c.GameplayPolicy) {
		return fmt.Errorf("unsupported frozen gameplay result policy")
	}
	if len(c.Actors) != f.ActorCount || len(f.Targets) != 5 || f.ActorCount < 1 || f.ActorCount > 5 {
		return fmt.Errorf("source finalizer actor/TARGET ownership is incomplete")
	}
	if c.Version == 2 || c.Version == 3 {
		if err := validateReservedRescues(c); err != nil {
			return err
		}
	}
	f.RawScore = 0
	base := InitializedPose{H0: binary.LittleEndian.Uint32(f.Targets[0][42:46]), H1: binary.LittleEndian.Uint32(f.Targets[0][46:50])}
	farp := InitializedPose{H0: binary.LittleEndian.Uint32(f.Targets[1][42:46]), H1: binary.LittleEndian.Uint32(f.Targets[1][46:50])}
	farpActive := binary.LittleEndian.Uint16(f.Targets[1][32:34]) != 0
	near := FinalResultMetric(f.FinalPose, base)
	if farpActive {
		if d := FinalResultMetric(f.FinalPose, farp); d < near {
			near = d
		}
	}
	for i, kind := range c.DynamicKinds {
		if kind&0x40 != 0 {
			continue
		}
		if kind == 0x1d {
			f.ObjectCredits[i/2] |= 8 << uint((i&1)*4)
			continue
		}
		if kind == 0x1c {
			f.RawScore += f.ObjectValues[i]
			continue
		}
		if kind&0x30 == 0 && FinalResultMetric(f.FinalPose, c.WorldPoses[i]) < near {
			f.SourceFlags |= 0x20
		}
	}
	if f.SourceFlags&0x20 == 0 && near < 1024 {
		f.SourceFlags |= 0x40
	}
	if sourceHealth(c.Actors[0].Damage) < 128 {
		f.SourceFlags |= 4
	}
	c.Actors[0].Flags = f.SourceFlags
	for i := 1; i < len(c.Actors); i++ {
		a := &c.Actors[i]
		flags := a.Flags
		if a.Kind == 0x1c {
			flags |= 0x10
		} else if c.SetupFlags&8 != 0 && a.Kind == 0x41 {
			flags |= 0x80
		} else {
			inherited := false
			if a.Control == 1 {
				if a.Leader == 0 {
					inherited = f.SourceFlags&0x80 != 0
				} else if c.GameplayPolicy == SafeGameplayPolicy {
					if a.Leader < 0 || a.Leader >= len(c.Actors) || a.Leader == i {
						return fmt.Errorf("formed actor%d has invalid actual leader%d", i, a.Leader)
					}
					// R5 fixes the follower-only RESULT274 base off-by-one.
					// Keep ascending finalization and the defined inheritance rule.
					inherited = c.Actors[a.Leader].Flags&0x80 != 0
				} else if a.Leader >= 1 && a.Leader < 4 && a.Leader+1 < len(c.Actors) {
					// Literal274+leader lookup: preserve the bounded source quirk.
					inherited = c.Actors[a.Leader+1].Flags&0x80 != 0
				}
			}
			if inherited || FinalResultMetric(a.Pose, base) <= 768 {
				flags |= 0x80
			} else if a.Control == 0 && (!farpActive || FinalResultMetric(a.Pose, farp) > 768) {
				flags |= 0x20
			}
		}
		a.Flags = flags
	}
	if c.Version == 2 || c.Version == 3 {
		if err := validateReservedRescues(c); err != nil {
			return err
		}
		// Source110a..116d runs after ordinary follower flags. Only the actual
		// carrier80 inherits; proximity20/40 and pickup alone are insufficient.
		for _, rescue := range c.ReservedRescues {
			if rescue.Kind == 0x41 && c.Actors[int(rescue.Carrier)].Flags&0x80 != 0 {
				c.Actors[rescue.Actor].Flags |= 0x80
			}
		}
	}
	return nil
}

func validateReservedRescues(c *SortieResultContext) error {
	if c.Version == 1 {
		if len(c.ReservedRescues) != 0 {
			return fmt.Errorf("historical result context1 cannot contain new rescue facts")
		}
		return nil
	}
	if c.Version == 3 && len(c.Actors) == 1 && len(c.ReservedRescues) == 0 {
		return nil
	}
	if (c.Version != 2 && c.Version != 3) || len(c.Actors) != 5 || len(c.ReservedRescues) != 4 {
		return fmt.Errorf("result context2 needs all four source rescue owners")
	}
	for i, rescue := range c.ReservedRescues {
		if rescue.Actor != i+1 || rescue.Kind != c.DynamicKinds[rescue.Actor+4] || (rescue.Kind == 0x41 && rescue.Carrier >= 5) {
			return fmt.Errorf("invalid source rescue actor/carrier provenance")
		}
	}
	return nil
}

func validateResultContext(f SortieFacts) error {
	c := f.ResultContext
	if c == nil {
		return nil
	}
	if (c.Version != 1 && c.Version != 2 && c.Version != 3) || len(c.Actors) != f.ActorCount || c.SeedPolicy == "" {
		return fmt.Errorf("source result context version/actor/RNG provenance missing")
	}
	if c.Version == 3 {
		if c.Scene == nil || *c.Scene > 5 {
			return fmt.Errorf("result context3 needs explicit source scene0..5")
		}
	} else if c.Scene != nil {
		return fmt.Errorf("historical result context has new scene")
	}
	if err := validateReservedRescues(c); err != nil {
		return err
	}
	if c.SeedPolicy == ResultSeedPolicy && c.Seed != ResultSeed(f.SortieID) {
		return fmt.Errorf("native result seed differs from frozen sortie identity")
	}
	if f.MissionType == MissionCampaign && c.Campaign == nil {
		return fmt.Errorf("Campaign facts lack the actual accepted launch draft")
	}
	if c.Campaign != nil && (c.Campaign.CurrentMission != f.MissionType || c.Campaign.LaunchHigh != byte(f.LaunchSwitches>>8)) {
		return fmt.Errorf("Campaign mode/launch-header domains disagree with frozen facts")
	}
	seen := [5]bool{}
	for i, a := range c.Actors {
		if a.Actor != i || a.SourceIndex < 0 || a.SourceIndex > 4 || seen[a.SourceIndex] || a.RankRaw < 0 || a.RankRaw > 11 || a.AcceptedSelector > 7 {
			return fmt.Errorf("source actor identity/crew provenance incomplete")
		}
		seen[a.SourceIndex] = true
		if a.AcceptedSelector != f.AcceptedSelectors[i] {
			return fmt.Errorf("actor aircraft identity differs from accepted selector snapshot")
		}
		if i > 0 {
			if a.SourceIndex == 0 || a.Name == "" || int(int8(a.StoredRaw[34])) != a.RankRaw {
				return fmt.Errorf("stored crew identity/rank disagrees with accepted actor")
			}
			rank, _ := RankFromRaw(a.RankRaw)
			counts := make([]int8, 8)
			for j, b := range a.StoredRaw[26:34] {
				counts[j] = int8(b)
			}
			skill, err := ComputeSourceProficiency(FlightPilot{SourceIndex: a.SourceIndex, Rank: rank, AwardCounts: counts})
			if err != nil || a.Ship[29] != skill {
				return fmt.Errorf("SHIP1d is not the accepted crew rank/medal producer")
			}
		}
	}
	if f.InactiveHeaderPolicy == SortieAcceptedFlightHeaders {
		values := [8]int8{0, -50, 25, 50, 50, 25, 25, 50}
		if f.PlayerAircraftValue != values[f.AcceptedSelectors[0]] {
			return fmt.Errorf("player score header differs from accepted aircraft")
		}
		for i, value := range f.InactiveAircraftValues {
			want := int8(0)
			if i+1 < f.ActorCount {
				want = values[f.AcceptedSelectors[i+1]]
			}
			if value != want {
				return fmt.Errorf("active/padding score header%d differs from accepted source", i+1)
			}
		}
	}
	if c.Actors[0].SourceIndex != 0 {
		return fmt.Errorf("player must own source0")
	}
	var raw int16
	for i, kind := range c.DynamicKinds {
		if kind&0x40 != 0 {
			continue
		}
		if kind == 0x1c {
			raw += f.ObjectValues[i]
		}
		if kind == 0x1d && (f.ObjectCredits[i/2]>>uint((i&1)*4))&8 == 0 {
			return fmt.Errorf("kind1d final credit marker was not constructed")
		}
	}
	if raw != f.RawScore {
		return fmt.Errorf("final raw points differ from frozen dynamic kinds (AE9 is not0268)")
	}
	return nil
}

func normalizeResult(f SortieFacts) (*SortieResultDetails, error) {
	c := f.ResultContext
	r := &SortieResultDetails{RescuerActor: -1, RescueTextVariant: -1, RNG: ResultRandomState{Seed: c.Seed, State: c.Seed, Policy: c.SeedPolicy, Draws: []ResultRandomDraw{}}}
	r.NormalizedFlags[0] = f.SourceFlags
	for i := 1; i < len(c.Actors); i++ {
		r.NormalizedFlags[i] = c.Actors[i].Flags
	}
	if f.MissionType == MissionTraining {
		for i := range r.NormalizedFlags {
			r.NormalizedFlags[i] &= 0xcb
		}
		r.Loss = r.NormalizedFlags[0]&0x18 != 0
		return r, nil
	}
	for i := 1; i < f.ActorCount; i++ {
		flags := r.NormalizedFlags[i]
		if flags&0xa0 == 0xa0 {
			flags &= 0x7f
		} else if flags&0x20 != 0 && r.RNG.Draw(fmt.Sprintf("follower%d-survival", i), 100) >= 60 {
			flags = flags&0xdf | 8
		}
		r.NormalizedFlags[i] = flags
		if flags&0x18 != 0 {
			r.LostFollowers++
		}
	}
	n := 0
	for i := 1; i < f.ActorCount; i++ {
		if r.NormalizedFlags[i]&0x38 == 0 && c.Actors[i].AcceptedSelector == 5 {
			n++
		}
	}
	if n > 0 {
		choice := r.RNG.Draw("rescuer-selection", n)
		for i := 1; i < f.ActorCount; i++ {
			if r.NormalizedFlags[i]&0x18 == 0 && c.Actors[i].AcceptedSelector == 5 {
				if choice == 0 {
					r.RescuerActor = i
					break
				}
				choice--
			}
		}
	}
	flags := r.NormalizedFlags[0]
	if flags&0xd0 != 0 {
		flags &= 0x9f
	} else {
		x := int32(f.FinalPose.H0 - c.LaunchPose.H0)
		y := int32(f.FinalPose.H1 - c.LaunchPose.H1)
		if x == -2147483648 || y == -2147483648 {
			return nil, fmt.Errorf("result distance excludes ABS32(INT_MIN)")
		}
		if x < 0 {
			x = -x
		}
		if y < 0 {
			y = -y
		}
		if x < y {
			x, y = y, x
		}
		r.Distance = int16((x + y/2) / 32768)
		r.Threshold = 5 * (int16(2*(n+10)) - r.Distance)
		if r.Threshold < 10 {
			r.Threshold = 10
		}
		if r.RNG.Draw("player-survival", 100) > int(r.Threshold) {
			flags = flags&0x9f | 8
		}
	}
	r.NormalizedFlags[0] = flags
	r.Loss = flags&0x18 != 0
	return r, nil
}

func prepareCrewResults(out *SortieProposal) error {
	c, r := out.Facts.ResultContext, out.Result
	var metrics [4]SortieCrewMetric
	for i := 0; i < 4; i++ {
		metrics[i] = SortieCrewMetric{Actor: i + 1, Flags: r.NormalizedFlags[i+1]}
		if i+1 < len(c.Actors) {
			a := c.Actors[i+1]
			m := &metrics[i]
			m.SourceIndex = a.SourceIndex
			m.Name = a.Name
			m.Rank = a.RankRaw
			m.Flight = int(255-a.RawA) * 100 / 255
			m.Attack = int(255-a.RawB) * 100 / 255
			m.Defense = int(byte(sourceHealth(a.Damage))) * 100 / 255
		}
		if metrics[i].Flags&0x18 == 0 {
			threshold := (metrics[i].Defense - 50) * 100 / 40
			if threshold < 0 {
				threshold = 0
			}
			if threshold > 100 {
				threshold = 100
			}
			if r.RNG.Draw(fmt.Sprintf("crew%d-wound", i+1), 100) >= threshold {
				metrics[i].Flags |= 4
				r.NormalizedFlags[i+1] |= 4
			}
		}
		metrics[i].Mean = (metrics[i].Flight + metrics[i].Attack + metrics[i].Defense) / 3
	}
	for i := 1; i < out.Facts.ActorCount; i++ {
		r.Crew = append(r.Crew, metrics[i-1])
	}
	rows, err := squadronFromPilot(out.ScoredPilot)
	if err != nil {
		return err
	}
	meanRank := 0
	for _, row := range rows {
		meanRank += row.Rank.Raw()
	}
	meanRank /= 4
	leader := out.PilotBefore.Rank.Raw()
	add := func(kind, actor int) {
		r.Recommendations = append(r.Recommendations, SortieCrewRecommendation{Type: kind, Actor: actor, SourceIndex: c.Actors[actor].SourceIndex})
	}
	if meanRank < leader-2 && out.Score.NormalizedFlags&3 != 0 {
		best, limit := 0, leader-1
		for i, m := range r.Crew {
			if m.Flags&0x18 == 0 && m.Mean >= 50 && m.Rank < limit {
				best = i + 1
				limit = m.Rank
			}
		}
		if best != 0 {
			add(0, best)
		}
	}
	// The crew caller is1c4b:06ee -> canonical240e, which reads signed9bda.
	// It is NOT241c/242e's AX return used by the player's special predicate.
	query := sortieCrewMedalQuery(out)
	if query >= 0 && query <= 5 && (query != 5 || out.ScoredPilot.Statistics.AwardCounts[5] > 1) {
		best, second := 0, 0
		hi, lo := 49, 49
		for i, m := range r.Crew {
			if m.Flags&0x18 != 0 {
				continue
			}
			if m.Mean > hi {
				second, best = best, i+1
				lo, hi = hi, m.Mean
			} else if m.Mean > lo {
				second = i + 1
				lo = m.Mean
			}
		}
		for _, actor := range []int{best, second} {
			if actor != 0 {
				if query < 5 {
					query++
				}
				add(query, actor)
			}
		}
	}
	if len(r.Recommendations) == 3 && out.Score.NormalizedFlags&3 != 3 {
		r.Recommendations = r.Recommendations[:2]
	}
	return nil
}

func sortieCrewMedalQuery(out *SortieProposal) int {
	if sortieDisqualifier(out.Facts) || out.Score.NormalizedFlags&3 == 0 {
		return -1
	}
	p := out.ScoredPilot
	start := 2
	if out.Facts.LaunchSwitches&3 >= 2 {
		start = 1
	}
	if out.Score.NormalizedFlags&0x83 == 0x83 && out.Facts.LaunchSwitches&3 >= 2 && p.Statistics.AwardCounts[0] == 0 {
		start = 0
	}
	for i := start; i < 6; i++ {
		if p.Statistics.LastMissionScore >= int32(out.Tables.MedalThresholds[i]) {
			return i
		}
	}
	return -1
}

// DefaultSortieCrewChoices records actor identities, not visible row indices.
func DefaultSortieCrewChoices(p *SortieProposal) *SortieCrewChoices {
	if p == nil || p.Result == nil {
		return nil
	}
	c := &SortieCrewChoices{Recipients: make([]int, len(p.Result.Recommendations))}
	for i, r := range p.Result.Recommendations {
		c.Recipients[i] = r.Actor
	}
	return c
}

func validateCrewChoices(p *SortieProposal, choices *SortieCrewChoices, leaderRank int) error {
	if choices == nil {
		choices = DefaultSortieCrewChoices(p)
	}
	if p.Result == nil {
		if choices != nil && len(choices.Recipients) != 0 {
			return fmt.Errorf("legacy proposal has no crew recommendations")
		}
		return nil
	}
	if choices == nil || len(choices.Recipients) != len(p.Result.Recommendations) {
		return fmt.Errorf("crew choice count differs from source recommendations")
	}
	for i, actor := range choices.Recipients {
		if actor < 1 || actor >= p.Facts.ActorCount {
			return fmt.Errorf("crew recipient is not an admitted participating actor")
		}
		if p.Result.Recommendations[i].Type == 0 && p.Facts.ResultContext.Actors[actor].RankRaw >= leaderRank-1 {
			return fmt.Errorf("crew promotion would reach leader rank")
		}
		if p.Result.Recommendations[i].Type != 0 {
			for j, other := range choices.Recipients {
				if i != j && p.Result.Recommendations[j].Type != 0 && actor == other {
					return fmt.Errorf("duplicate crew medal recipient")
				}
			}
		}
	}
	return nil
}

// CycleSortieCrewRecipient keeps the original rank/duplicate predicates; notably
// loss flags18 are not retested here. Unpopulated native actor rows stay excluded.
func CycleSortieCrewRecipient(p *SortieProposal, choices *SortieCrewChoices, index int) (*SortieCrewChoices, error) {
	return CycleSortieCrewRecipientForChoice(p, choices, index, true)
}
func CycleSortieCrewRecipientForChoice(p *SortieProposal, choices *SortieCrewChoices, index int, accepted bool) (*SortieCrewChoices, error) {
	if p == nil || p.Result == nil || index < 0 || index >= len(p.Result.Recommendations) {
		return nil, fmt.Errorf("crew selector outside recommendation table")
	}
	if choices == nil {
		choices = DefaultSortieCrewChoices(p)
	}
	out := &SortieCrewChoices{Recipients: append([]int(nil), choices.Recipients...), Names: append([]string(nil), choices.Names...)}
	for n := 0; n < 4; n++ {
		out.Recipients[index] = out.Recipients[index]%4 + 1
		leader := p.AcceptedPilot
		if p.Commission && !accepted {
			leader = p.RefusedPilot
		}
		if validateCrewChoices(p, out, leader.Rank.Raw()) == nil {
			return out, nil
		}
	}
	return nil, fmt.Errorf("no bounded source recipient available")
}

func prepareResultPresentationAndReplacements(out *SortieProposal, choices *SortieCrewChoices) error {
	r, c := out.Result, out.Facts.ResultContext
	flags := out.Score.NormalizedFlags
	if out.Version >= 4 {
		r.CrewPro = !r.Loss && out.Facts.MissionType != MissionTraining && !out.Commission && out.Facts.ActorCount > 1
	}
	r.MusicCue = 0x1c
	if c.MusicOverride != nil && *c.MusicOverride >= 0 && *c.MusicOverride <= 6 {
		r.MusicCue = [7]uint16{0x1b, 0x1a, 0x19, 0x1e, 0x1c, 0x1f, 0x20}[*c.MusicOverride]
	} else if flags&0x10 != 0 {
		r.MusicCue = 0x20
	} else if flags&8 != 0 {
		r.MusicCue = 0x1c
	} else if r.Campaign != nil && r.Campaign.Verdict != 0 {
		if r.Campaign.Verdict > 0 {
			r.MusicCue = 0x1e
		} else {
			r.MusicCue = 0x1f
		}
	} else if out.Success && (out.Facts.MissionType == MissionTraining || out.SpecialOutcome || r.LostFollowers == 0) {
		r.MusicCue = 0x1b
		if out.InitialRankRecommendation != 0 {
			r.MusicCue = 0x19 + uint16(r.RNG.Draw("result-music-variant", 2))
		}
	}
	if flags&0x20 != 0 && r.RescuerActor >= 1 {
		r.RescueTextVariant = r.RNG.Draw("rescue-text", 2)
	}
	if !r.Loss && out.Facts.MissionType != MissionTraining && (out.Commission || out.Facts.ActorCount > 1) {
		rows, err := squadronFromPilot(out.AcceptedPilot)
		if err != nil {
			return err
		}
		if out.Version >= 4 && r.CrewPro {
			if err = applyCrewAwardsToRows(out, choices, rows, out.AcceptedPilot.Rank.Raw()); err != nil {
				return err
			}
		}
		need := false
		allInitial := true
		for i := range rows {
			allInitial = allInitial && rows[i].Rank.Raw() == 0
			if rows[i].Rank.Raw() == 0 {
				need = true
			}
			for _, m := range r.Crew {
				if m.SourceIndex == i+1 && m.Flags&0x18 != 0 {
					need = true
				}
			}
		}
		if need && len(c.ReplacementNames) != 74 {
			return fmt.Errorf("source replacement requires74 prepared names")
		}
		for i := range rows {
			lost := false
			for _, m := range r.Crew {
				if m.SourceIndex == i+1 {
					lost = m.Flags&0x18 != 0
				}
			}
			if rows[i].Rank.Raw() != 0 && !lost {
				continue
			}
			if out.Version >= 4 {
				r.ReplacementCount++
				r.ReplacementInitial = allInitial
			}
			portrait := 0
			for tries := 0; ; tries++ {
				if tries == 4096 {
					return fmt.Errorf("source portrait rejection did not terminate")
				}
				portrait = r.RNG.Draw(fmt.Sprintf("crew%d-replacement-portrait", i+1), 33) + 1
				used := false
				for _, other := range rows {
					used = used || other.Portrait == portrait
				}
				if !used {
					break
				}
			}
			name := ""
			for tries := 0; ; tries++ {
				if tries == 4096 {
					return fmt.Errorf("source name rejection did not terminate")
				}
				name = c.ReplacementNames[r.RNG.Draw(fmt.Sprintf("crew%d-replacement-name", i+1), 74)]
				used := false
				for _, other := range c.OccupiedNames {
					used = used || strings.Contains(other, name)
				}
				for _, other := range rows {
					used = used || strings.Contains(other.Name, name)
				}
				if name != "" && !used {
					break
				}
			}
			raw := append([]byte(nil), rows[i].LegacyRaw...)
			if len(raw) != 41 {
				return fmt.Errorf("replacement row has no opaque41-byte owner")
			}
			clear(raw[26:34])
			raw[33], raw[34], raw[35], raw[36] = 1, 1, 1, byte(portrait)
			copy(raw, name)
			raw[len(name)] = 0
			rows[i] = SquadronPilot{Name: name, Rank: RankWO1, Portrait: portrait, LegacyRaw: raw}
		}
		r.Replacements = rows
		if out.Version >= 4 && (out.Retirement || out.AcceptedPilot.Rank.Raw() == 11) {
			r.ReplacementCount = 0
		}
	}
	// Feedback is after all result-affecting decisions. Keep its source branch
	// draws in the same stream; animation/head-frame choices remain the earlier
	// documented presentation-only hash policy and cannot reroll outcomes.
	r.FeedbackVariants = map[string]uint32{}
	draw := func(key string, n int) { r.FeedbackVariants[key] = uint32(r.RNG.Draw("feedback-"+key, n)) }
	if !r.Loss && !out.Retirement {
		// A1e543..1e55d passes8610 only when861c is clear. A special
		// outcome suppresses the crew-loss feedback branch, not the facts.
		feedbackLost := r.LostFollowers > 0 && !out.SpecialOutcome
		friendly := false
		for i, v := range out.Facts.ObjectValues {
			if v < 0 && (out.Facts.ObjectCredits[i/2]>>uint((i&1)*4))&15 == 1 {
				friendly = true
			}
		}
		if friendly {
			draw("friendly", 2)
		} else if out.Success && !feedbackLost {
			if flags&3 != 3 {
				draw("partial", 8)
			} else {
				draw("success", 10)
			}
			if out.Facts.MissionType == MissionCampaign && r.Campaign != nil && r.Campaign.Quality < 30 {
				draw("campaign-warning", 3)
			}
			if flags&0x80 == 0 {
				draw("no-return", 4)
			}
		} else if out.Success {
			draw("loss", 3)
			if out.Facts.MissionType == MissionCampaign && r.Campaign != nil && r.Campaign.Quality < 30 {
				draw("campaign-warning", 3)
			}
		} else {
			draw("failure", 6)
		}
	}
	// The immutable replacement selection is applied only when resolving pending
	// choices. Neither proposal creation nor a redraw writes the persistent crew.
	return nil
}

func copySquadronRows(p *Pilot, rows []SquadronPilot) error {
	if len(rows) != 4 {
		return fmt.Errorf("source squadron copyback needs four stored rows")
	}
	p.SquadronPilots = make([]SquadronPilot, 4)
	for i, row := range rows {
		if err := validateSquadronMember(row); err != nil {
			return err
		}
		p.SquadronPilots[i] = row
		p.SquadronPilots[i].LegacyRaw = append([]byte(nil), row.LegacyRaw...)
		copy(p.LegacyRaw[0x55+41*(i+1):], row.LegacyRaw)
	}
	return nil
}

func applyCrewAwardsToRows(out *SortieProposal, choices *SortieCrewChoices, rows []SquadronPilot, leaderRank int) error {
	if choices == nil {
		choices = DefaultSortieCrewChoices(out)
	}
	if err := validateCrewChoices(out, choices, leaderRank); err != nil {
		return err
	}
	if len(rows) != 4 {
		return fmt.Errorf("crew awards require four source rows")
	}
	for i, rec := range out.Result.Recommendations {
		actor := choices.Recipients[i]
		source := out.Facts.ResultContext.Actors[actor].SourceIndex - 1
		row := &rows[source]
		if rec.Type == 0 {
			rank, err := RankFromRaw(row.Rank.Raw() + 1)
			if err != nil {
				return err
			}
			row.Rank, row.LegacyRaw[34] = rank, byte(rank.Raw())
		} else {
			row.LegacyRaw[26+rec.Type]++
		}
	}
	for _, m := range out.Result.Crew {
		if m.Flags&0x14 != 0 {
			rows[m.SourceIndex-1].LegacyRaw[32]++
		}
	}
	return nil
}

func resolveSortieResultForChoices(p *SortieProposal, accepted bool, choices *SortieCrewChoices) (*SortieResultDetails, error) {
	base := SortieResultForChoice(p, accepted)
	if p.Version < 4 {
		if choices != nil && choices.Result != nil {
			return nil, fmt.Errorf("historical result cannot contain a post-CREWPRO continuation")
		}
		return base, nil
	}
	if base == nil || !base.CrewPro || choices == nil {
		return base, nil
	}
	want := DefaultSortieCrewChoices(p)
	same := len(want.Recipients) == len(choices.Recipients)
	if same {
		for i, actor := range want.Recipients {
			same = same && choices.Recipients[i] == actor
		}
	}
	if same {
		return base, nil
	}
	// The proposal's default decisions remain immutable. A changed source
	// recommendation reconstructs its own post-CREWPRO CRT continuation, which
	// the pending choice document freezes before any REPLACE page is shown.
	branch, err := proposeSortieVersionChoices(p.PilotBefore, p.Facts, p.Tables, p.Version, &sortieCrewBranchChoices{accepted, choices})
	if err != nil {
		return nil, err
	}
	return SortieResultForChoice(branch, accepted), nil
}

func applyCrewChoices(p *Pilot, out *SortieProposal, choices *SortieCrewChoices) error {
	r := out.Result
	if r == nil || r.Loss || out.Facts.MissionType == MissionTraining {
		return nil
	}
	if out.Version >= 4 && !r.CrewPro && len(r.Replacements) == 0 {
		return nil
	}
	if choices == nil {
		choices = DefaultSortieCrewChoices(out)
	}
	if err := validateCrewChoices(out, choices, p.Rank.Raw()); err != nil {
		return err
	}
	rows, err := squadronFromPilot(p)
	if err != nil {
		return err
	}
	// AD59 loss count shares the first stored row's opaque tail37..40.
	copy(rows[0].LegacyRaw[37:41], p.LegacyRaw[0xa3:0xa7])
	if r.CrewPro {
		if err = applyCrewAwardsToRows(out, choices, rows, p.Rank.Raw()); err != nil {
			return err
		}
	}
	if len(r.Replacements) == 4 {
		for i, row := range r.Replacements {
			if row.Name != rows[i].Name || rows[i].Rank.Raw() == 0 {
				tail := append([]byte(nil), rows[i].LegacyRaw[37:41]...)
				rows[i] = row
				rows[i].LegacyRaw = append([]byte(nil), row.LegacyRaw...)
				copy(rows[i].LegacyRaw[37:41], tail)
			}
		}
	}
	if choices != nil && len(choices.Names) > 0 {
		if out.Version < 4 || r.ReplacementCount == 0 || len(choices.Names) != 4 {
			return fmt.Errorf("replacement names need a v4 replacement page")
		}
		for i, name := range choices.Names {
			if name == "" {
				continue
			} // Explicitly unchanged slot; never an empty surname.
			if err := ValidateReplacementName(name); err != nil {
				return err
			}
			// GS1627:0000 (resident file18270h) trims ASCII spaces before
			// 2930 copies the surname. The helper already rejects leading
			// space; preserve internal spaces and all opaque row tail bytes.
			name = strings.TrimRight(name, " ")
			rows[i].Name = name
			copy(rows[i].LegacyRaw, name)
			rows[i].LegacyRaw[len(name)] = 0 // source strcpy preserves opaque bytes afterNUL
		}
	}
	return copySquadronRows(p, rows)
}

func ResolveSortieProposalWithChoices(p *SortieProposal, acceptCommission bool, choices *SortieCrewChoices) (*Pilot, error) {
	if err := ValidateSortieProposal(p); err != nil {
		return nil, err
	}
	q := cloneSortiePilot(p.AcceptedPilot)
	if p.Commission && !acceptCommission {
		q = cloneSortiePilot(p.RefusedPilot)
	}
	view := *p
	var err error
	view.Result, err = resolveSortieResultForChoices(p, acceptCommission, choices)
	if err != nil {
		return nil, err
	}
	if choices != nil && choices.Result != nil && !reflect.DeepEqual(choices.Result, view.Result) {
		return nil, fmt.Errorf("pending crew continuation differs from source choices")
	}
	if choices != nil && len(choices.Names) > 0 {
		if p.Version < 4 || view.Result == nil || view.Result.ReplacementCount == 0 || len(choices.Names) != 4 {
			return nil, fmt.Errorf("replacement name choices have no admitted page")
		}
		for _, name := range choices.Names {
			if name != "" {
				if err := ValidateReplacementName(name); err != nil {
					return nil, err
				}
			}
		}
	}
	if err := applyCrewChoices(q, &view, choices); err != nil {
		return nil, err
	}
	return q, nil
}

// UnsupportedMissionRecoveryOwner bounds GS A1ee44's separate roster-wide
// rescue writer. This is not GS326c's survival/rescuer-name normalization. Source
// checks slots1..5 (literal source quirk), MIA3, matching theatre and raw5b.
// N.4 has no cross-pilot transaction for reviving that owner, so it must refuse
// this exact branch rather than silently committing an incomplete recovery.
func UnsupportedMissionRecoveryOwner(player *Pilot, facts SortieFacts, roster []*Pilot) (string, error) {
	if player == nil || len(player.LegacyRaw) != LegacyRecordSize || len(facts.Targets) != 5 {
		return "", fmt.Errorf("mission recovery owner requires complete player/TARGET provenance")
	}
	possible := facts.SourceFlags&1 != 0 && binary.LittleEndian.Uint16(facts.Targets[2][32:34]) == 2 || facts.SourceFlags&2 != 0 && binary.LittleEndian.Uint16(facts.Targets[3][32:34]) == 2
	if !possible {
		return "", nil
	}
	scenario := player.LegacyRaw[0x5b]
	if facts.MissionType != MissionTraining {
		if facts.ResultContext == nil {
			return "", fmt.Errorf("mission recovery query lacks selected-scenario launch producer")
		}
		scenario = facts.ResultContext.SelectedScenario
	}
	for i := 1; i < 6 && i < len(roster); i++ {
		candidate := roster[i]
		if candidate == nil {
			continue
		}
		if candidate.Status != StatusMIA {
			continue
		}
		if len(candidate.LegacyRaw) != LegacyRecordSize {
			return "", fmt.Errorf("MIA mission recovery record lacks source provenance")
		}
		if candidate.Theatre == player.Theatre && candidate.LegacyRaw[0x5b] == scenario {
			return candidate.ID, nil
		}
	}
	return "", nil
}

// NormalizeKnownPilotStatus upgrades only the formerly opaque, byte-witnessed
// source statuses2..4. Call on an owned in-memory load; no save write is implied.
func NormalizeKnownPilotStatus(p *Pilot) bool {
	if p == nil || p.Status != StatusLegacyUnknown || len(p.LegacyRaw) != LegacyRecordSize || p.LegacyRaw[0x56] != p.LegacyStatus {
		return false
	}
	switch p.LegacyStatus {
	case 2:
		p.Status = StatusRetired
	case 3:
		p.Status = StatusMIA
	case 4:
		p.Status = StatusKIA
	default:
		return false
	}
	return true
}

// SortieResultForChoice reads one immutable v4 CRT branch; historical proposals
// keep their original offer-based interpretation and exact recorded decisions.
func SortieResultForChoice(p *SortieProposal, accepted bool, choices ...*SortieCrewChoices) *SortieResultDetails {
	if p == nil {
		return nil
	}
	if p.Version >= 4 && len(choices) > 0 && choices[0] != nil && choices[0].Result != nil {
		return choices[0].Result
	}
	if p.Version >= 4 && p.Commission && !accepted {
		return p.RefusedResult
	}
	return p.Result
}
func ValidateReplacementName(name string) error {
	if len(name) == 0 || len(name) > 10 || name[0] == ' ' {
		return fmt.Errorf("replacement name must contain1..10 bytes with no leading space")
	}
	for _, c := range []byte(name) {
		if c < 0x20 || c > 0x7f {
			return fmt.Errorf("replacement name has unsupported source character")
		}
	}
	return nil
}
