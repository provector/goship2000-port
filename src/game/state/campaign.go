package state

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	"strings"
)

const campaignSchemaVersion = 2

// CampaignGenerationPolicyPairedV1 repairs the DOS intermediate reader's
// shifted metadata destination. Empty retains historical generation semantics.
const CampaignGenerationPolicyPairedV1 = "paired-v1"

func validCampaignGenerationPolicy(policy string) bool {
	return policy == "" || policy == CampaignGenerationPolicyPairedV1
}

type CampaignPhase string

const (
	CampaignActive    CampaignPhase = "active"
	CampaignSuspended CampaignPhase = "suspended"
)

// These counters retain their demonstrated threshold/comparison operations;
// they are not ammunition stock, supply or a fabricated history of victories.
type CampaignState struct {
	Version            int                      `json:"version"`
	SourceProfile      string                   `json:"source_profile"`
	Phase              CampaignPhase            `json:"phase"`
	Theatre            Theatre                  `json:"theatre"`
	TargetCounter      int                      `json:"target_counter"`
	ResultCounters     [2]int                   `json:"result_counters"`
	LossesRelatedCount int                      `json:"losses_related_count"`
	SelectedScenario   int                      `json:"selected_scenario"`
	PriorScenario      int                      `json:"prior_scenario"`
	ScenarioOrdinal    int                      `json:"scenario_ordinal"`
	Caches             []CampaignFormationCache `json:"scenario_caches,omitempty"`
	Retained           CampaignRetained         `json:"retained_preparation"`
	Suspended          *CampaignSuspension      `json:"suspended_preparation,omitempty"`
	ImportSHA256       string                   `json:"import_sha256,omitempty"`
	CacheInventory     []CampaignCacheKnowledge `json:"cache_inventory,omitempty"`
	PlacementPolicy    string                   `json:"placement_policy,omitempty"`
	GenerationPolicy   string                   `json:"generation_policy,omitempty"`
}

type CampaignRetained struct {
	Reserved   [][32]byte   `json:"reserved_objects"`
	Provenance []byte       `json:"formation_provenance_base64"`
	Endpoint   MissionPoint `json:"endpoint"`
}

type CampaignFormationCache struct {
	Groups      []GenerationGroup `json:"groups,omitempty"`
	Scenario    string            `json:"scenario"`
	World       string            `json:"world"`
	Ordinal     int               `json:"scenario_ordinal"`
	BeforeCount int               `json:"writer_before_count"`
	Objects     []MissionObject   `json:"objects"`
	Provenance  []byte            `json:"writer_provenance_base64"`
}

// A suspended mission stores source-linked data, never copied paper prose,
// executable addresses, render buffers or another copy of the pilot profile.
type CampaignMissionSnapshot struct {
	GenerationGroups                        []GenerationGroup  `json:"generation_groups,omitempty"`
	PlacementPlan                           *RequiredFARPPlan  `json:"placement_plan,omitempty"`
	SourcePreparation                       *CampaignSourceRaw `json:"source_preparation,omitempty"`
	Scenario, World                         string
	Theatre                                 Theatre
	Suffix, Scene                           int
	Objects                                 []MissionObject
	Paths                                   []missionPath
	Objectives                              map[string]MissionObjective
	Conditions                              MissionConditions
	FriendlyTitle                           string
	RawE278, FlightTemplate                 int
	UnavailableAircraft, UnavailableSystems []int
}

// Exact imported blocks preserve fields not yet used by the native front end.
// Typed mission/settings values remain authoritative for understood operations.
type CampaignSourceRaw struct {
	RuntimeHeader  []byte `json:"runtime_header_base64"`
	SwitchBlock    []byte `json:"switch_block_base64"`
	TargetBlock    []byte `json:"target_block_base64"`
	ObjectiveKinds []byte `json:"objective_kinds_base64"`
	FriendlyLabel  []byte `json:"friendly_label_base64"`
}

type CampaignSuspension struct {
	Mission            CampaignMissionSnapshot `json:"mission"`
	Aircraft           [][36]byte              `json:"aircraft"`
	InitialAircraftIDs []byte                  `json:"initial_aircraft_ids_base64"`
	Flight             []FlightAssignment      `json:"flight"`
	Settings           PreflightSettings       `json:"settings"`
	SecondaryControl   byte                    `json:"secondary_control"`
}

// Identity and presentation belong to the authoritative pilot/squadron files.
// A suspended flight only retains the source-slot permutation and grouping.
type FlightAssignment struct {
	SourceIndex int  `json:"source_index"`
	Group       int  `json:"group"`
	Leader      bool `json:"leader"`
}

func (r CampaignRetained) Validate() error {
	if len(r.Reserved) != 5 || len(r.Provenance) != 80 {
		return fmt.Errorf("Campaign retained preparation requires five reserved rows and exactly80 formation bytes")
	}
	return nil
}

func (c CampaignFormationCache) Validate() error {
	if c.Scenario == "" || c.World == "" || len(c.Scenario) > 32 || len(c.World) > 32 || c.Ordinal < 0 || c.Ordinal > 255 || c.BeforeCount < 0 || c.BeforeCount > 255 || len(c.Objects) > 80 || len(c.Objects) != len(c.Provenance) {
		return fmt.Errorf("Campaign scenario cache has invalid source identity/counts")
	}
	return nil
}

func (s *CampaignState) Validate() error {
	if s == nil {
		return nil
	}
	if (s.Version != 1 && s.Version != 2 && s.Version != 3) || s.SourceProfile != assetdata.IslandIce469085.Name {
		return fmt.Errorf("Campaign version/source profile is unsupported")
	}
	if !validCampaignGenerationPolicy(s.GenerationPolicy) || s.GenerationPolicy != "" && s.Version < 2 {
		return fmt.Errorf("Campaign generation policy is unsupported")
	}
	if s.PlacementPolicy != "" && (s.PlacementPolicy != RequiredFARPPlacementV1 || s.Version < 3 || s.GenerationPolicy != CampaignGenerationPolicyPairedV1) {
		return fmt.Errorf("Campaign placement policy is unsupported")
	}
	if err := s.validateCacheInventory(); err != nil {
		return err
	}
	if s.Phase != CampaignActive && s.Phase != CampaignSuspended {
		return fmt.Errorf("Campaign phase %q is unsupported", s.Phase)
	}
	if s.Theatre.Raw() < 0 || s.TargetCounter < 0 || s.TargetCounter > 255 || s.ResultCounters[0] < 0 || s.ResultCounters[0] > 255 || s.ResultCounters[1] < 0 || s.ResultCounters[1] > 255 || s.LossesRelatedCount < 0 || s.LossesRelatedCount > 65535 || s.SelectedScenario < -128 || s.SelectedScenario > 127 || s.PriorScenario < -128 || s.PriorScenario > 127 || s.ScenarioOrdinal < 0 || s.ScenarioOrdinal > 255 || len(s.Caches) > 16 {
		return fmt.Errorf("Campaign counters/theatre/source selectors are invalid")
	}
	if err := s.Retained.Validate(); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, cache := range s.Caches {
		if err := cache.Validate(); err != nil {
			return err
		}
		if seen[cache.Scenario] {
			return fmt.Errorf("Campaign repeats scenario cache %q", cache.Scenario)
		}
		seen[cache.Scenario] = true
	}
	if s.Phase == CampaignSuspended && s.Suspended == nil {
		return fmt.Errorf("suspended Campaign has no complete preparation snapshot")
	}
	if s.Phase == CampaignActive && s.Suspended != nil {
		return fmt.Errorf("active Campaign must not contain a suspended preparation snapshot")
	}
	if s.Suspended != nil {
		m := s.Suspended.Mission
		if raw := m.SourcePreparation; raw != nil {
			if len(raw.RuntimeHeader) != 18 || len(raw.SwitchBlock) != 20 || len(raw.TargetBlock) != 310 || len(raw.ObjectiveKinds) != 5 || len(raw.FriendlyLabel) != 25 {
				return fmt.Errorf("Campaign imported source blocks are incomplete")
			}
		}
		if m.Theatre != s.Theatre || m.Suffix != s.SelectedScenario || m.Scenario == "" || m.World == "" || m.Scene < 0 || m.Scene > 5 || len(m.Objects) < 80 || len(m.Objects) > 81 || len(m.Paths) > 7281 || len(m.Objectives) > 5 || len(s.Suspended.Aircraft) != 5 || len(s.Suspended.InitialAircraftIDs) != 5 || len(s.Suspended.Flight) != 5 {
			return fmt.Errorf("suspended Campaign source/counts/flight are inconsistent")
		}
		if err := s.Suspended.Settings.Validate(); err != nil {
			return err
		}
		if m.Conditions.Callsign < 0 || m.Conditions.Callsign >= 26 || m.Conditions.SecondCallsign < 0 || m.Conditions.SecondCallsign >= 26 || len(m.FriendlyTitle) > 80 || m.RawE278 < 0 || m.RawE278 > 2 {
			return fmt.Errorf("suspended Campaign conditions/default selector are unsupported")
		}
		if s.Suspended.Flight[0].SourceIndex != 0 {
			return fmt.Errorf("suspended Campaign must retain the primary pilot in slot0")
		}
		seenSources := [5]bool{}
		for _, p := range s.Suspended.Flight {
			if p.SourceIndex < 0 || p.SourceIndex > 4 || p.Group < 0 || p.Group > 1 || seenSources[p.SourceIndex] {
				return fmt.Errorf("suspended Campaign flight assignment is invalid")
			}
			seenSources[p.SourceIndex] = true
		}
		if len(m.UnavailableAircraft) != 4 || len(m.UnavailableSystems) != 4 {
			return fmt.Errorf("Campaign restriction arrays require four signed IDs each")
		}
		for _, role := range []string{"base", "primary", "secondary"} {
			if _, ok := m.Objectives[role]; !ok {
				return fmt.Errorf("Campaign snapshot lacks the %s objective", role)
			}
		}
		for _, id := range s.Suspended.InitialAircraftIDs {
			if id > 7 {
				return fmt.Errorf("Campaign initial aircraft ID is unsupported")
			}
		}
		for _, id := range m.UnavailableAircraft {
			if id < -1 || id > 7 {
				return fmt.Errorf("Campaign aircraft restriction ID is unsupported")
			}
		}
		for _, id := range m.UnavailableSystems {
			if id < -1 || id > 27 {
				return fmt.Errorf("Campaign system restriction ID is unsupported")
			}
		}
	}
	return nil
}

func CloneCampaign(s *CampaignState) (*CampaignState, error) {
	if s == nil {
		return nil, nil
	}
	raw, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	var out CampaignState
	if err = json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return &out, out.Validate()
}

func CampaignStage(p *Pilot) (int, error) {
	if p == nil {
		return 0, fmt.Errorf("Campaign needs a pilot")
	}
	if p.Campaign != nil {
		if err := p.Campaign.Validate(); err != nil {
			return 0, err
		}
		if p.Campaign.Phase == CampaignSuspended {
			return 2, nil
		}
		return 1, nil
	}
	stage, ok := p.LegacyByte(0x4f)
	if !ok {
		return 0, fmt.Errorf("legacy Campaign phase is unavailable")
	}
	return int(stage), nil
}

func campaignInputs(s *CampaignState) *CampaignGenerationInputs {
	return &CampaignGenerationInputs{Stage: 1, Raw50: s.TargetCounter, Raw51: s.ResultCounters[0], Raw52: s.ResultCounters[1], LossWord: s.LossesRelatedCount, ScenarioCacheAbsent: s.ImportSHA256 == "", Caches: s.Caches, Retained: &s.Retained, CacheKnowledgeVersion: s.Version, CacheInventory: s.CacheInventory, GenerationPolicy: s.GenerationPolicy, PlacementPolicy: s.PlacementPolicy}
}

func CampaignStart(a *assetdata.MissionAssets, p *Pilot, seed int64, stream []int, retained []byte, safeGameplay ...bool) (*MissionData, *CampaignState, error) {
	if a == nil || a.Generation == nil || p == nil || p.MissionType != MissionCampaign || p.Status != StatusActive {
		return nil, nil, fmt.Errorf("Campaign needs an active pilot and prepared generation data")
	}
	s, err := CloneCampaign(p.Campaign)
	if err != nil {
		return nil, nil, err
	}
	in := MissionInputs{MissionType: MissionCampaign, Theatre: p.Theatre, Rank: p.Rank, PilotName: p.Name, Seed: seed, DrawStream: stream}
	rng := newMissionRNG(in)
	if s == nil {
		stage, err := CampaignStage(p)
		if err != nil {
			return nil, nil, err
		}
		if stage != 0 {
			return nil, nil, fmt.Errorf("legacy Campaign continuation needs an explicit matching Campaign cache import; no history was invented")
		}
		previous, _ := p.LegacyByte(0x5b)
		s = &CampaignState{Version: campaignSchemaVersion, SourceProfile: assetdata.IslandIce469085.Name, Phase: CampaignActive, Theatre: p.Theatre, TargetCounter: rng.inclusive(5, 8, "A1b342"), SelectedScenario: int(int8(previous)), Retained: CampaignRetained{Reserved: make([][32]byte, 5), Provenance: make([]byte, 80)}, CacheInventory: freshCampaignCacheInventory()}
		// The port owns initialized reserved storage. Preserve its known row0;
		// never invent the contents of released DOS slots from another module.
		if len(retained) == 32 {
			copy(s.Retained.Reserved[0][:], retained)
		} else if len(retained) != 0 {
			return nil, nil, fmt.Errorf("retained mission row must contain32 bytes")
		}
	}
	if s.Theatre != p.Theatre {
		return nil, nil, fmt.Errorf("Campaign theatre differs from the selected pilot; cancel the Campaign before changing theatre")
	}
	// Upgrade only the new preparation draft. Historical proposals/journals keep
	// their original immutable Campaign representation and calculator inputs.
	upgradeCampaignKnowledge(s)
	if len(safeGameplay) > 0 && safeGameplay[0] {
		s.GenerationPolicy = CampaignGenerationPolicyPairedV1
	}
	if int8(s.TargetCounter) == 0 {
		return nil, nil, fmt.Errorf("Campaign illustration has a zero source divisor; progress state is unsupported")
	}
	if len(safeGameplay) > 1 && safeGameplay[1] {
		s.Version, s.PlacementPolicy = 3, RequiredFARPPlacementV1
	}
	in.Campaign = campaignInputs(s)
	in.GenerationPolicy = s.GenerationPolicy
	in.PlacementPolicy = s.PlacementPolicy
	in.PreviousScenario = s.SelectedScenario
	in.rng = rng
	if s.Phase == CampaignSuspended {
		m, err := restoreCampaignMission(a, p, s, in, rng)
		if err != nil {
			return nil, nil, err
		}
		return m, s, nil
	}
	m, err := GenerateMission(a.Generation, in)
	if err != nil {
		return nil, nil, err
	}
	s.PriorScenario = s.SelectedScenario
	s.SelectedScenario = m.SourceWorld.Suffix
	s.ScenarioOrdinal = int(m.SourceWorld.SNHD[0])
	if m.campaignRetained == nil {
		return nil, nil, fmt.Errorf("Campaign generator did not retain its physical metadata")
	}
	s.Retained = *m.campaignRetained
	if m.campaignInitialCache != nil {
		found := false
		for i := range s.Caches {
			if s.Caches[i].Scenario == m.campaignInitialCache.Scenario {
				s.Caches[i] = *m.campaignInitialCache
				found = true
			}
		}
		if !found {
			s.Caches = append(s.Caches, *m.campaignInitialCache)
		}
		s.setCacheKnowledge(m.campaignInitialCache.Ordinal, CampaignCachePresent, "native preparation")
	}
	return m, s, s.Validate()
}

func campaignRetainedFromPopulation(g *genericMissionPopulation) *CampaignRetained {
	r := &CampaignRetained{Reserved: make([][32]byte, 5), Provenance: make([]byte, 80), Endpoint: g.endpoint}
	for i, o := range g.objects {
		if i < 5 {
			r.Reserved[i] = o.Raw
		}
		if i < 80 {
			r.Provenance[i] = byte(o.Formation)
		}
	}
	return r
}

func applyCampaignIntermediate(g *genericMissionPopulation, c CampaignFormationCache, r *CampaignRetained) error {
	return applyCampaignIntermediateKnown(g, c, r, nil)
}

func applyCampaignIntermediateKnown(g *genericMissionPopulation, c CampaignFormationCache, r *CampaignRetained, known *[80]bool, safeGameplay ...bool) error {
	if err := c.Validate(); err != nil {
		return err
	}
	if r == nil {
		return fmt.Errorf("Campaign cache hit needs exact retained formation metadata")
	}
	if err := r.Validate(); err != nil {
		return err
	}
	if c.World != g.source.WorldName || c.Ordinal != int(g.source.SNHD[0]) {
		return fmt.Errorf("Campaign cache source world/ordinal disagrees with the selected scenario")
	}
	paired := len(safeGameplay) > 0 && safeGameplay[0]
	if paired && c.Scenario != g.source.Scenario {
		return fmt.Errorf("Campaign paired cache scenario disagrees with the selected scenario")
	}
	k, n := len(g.objects), len(c.Objects)
	if k+n > 80 || !paired && k+2*n > 80 {
		return fmt.Errorf("Campaign cache post-count provenance write reaches adjacent original state; this continuation is unsupported")
	}
	// A1876a reads the old physical active prefix before source appenders
	// overwrite the inactive tail. Unknown is not a fabricated zero byte.
	if known != nil {
		end := k + n
		if paired {
			end = k // Cached rows carry their own writer provenance.
		}
		for i := 0; i < end; i++ {
			if !known[i] {
				return fmt.Errorf("Campaign cache needs known active provenance row%d", i)
			}
		}
	}
	// Provenance is a physical byte, not an unconditional table reference.
	// Step38's PHIL3 row23 retains154 against154 formations, but reachable
	// readers only compare that byte. Actual base/title lookups remain guarded
	// by objectFormation; never dereference an old-world byte speculatively.
	for i, o := range c.Objects {
		// The reader ignores its first header value and writes metadata after
		// increasing count. Newly appended objects retain their old slot byte.
		if paired {
			o.Formation = int(c.Provenance[i])
		} else {
			o.Formation = int(r.Provenance[k+i])
		}
		g.objects = append(g.objects, o)
	}
	if paired {
		// The initial cache writer visits each qualifying fixed formation once,
		// in FMHD order. Its paired provenance separates adjacent placements.
		if len(c.Groups) > 0 {
			for _, span := range c.Groups {
				span.First += k - c.BeforeCount
				g.groupWrites = append(g.groupWrites, span)
			}
		} else {
			for start := k; start < len(g.objects); {
				fi := g.objects[start].Formation
				end := start + 1
				for end < len(g.objects) && g.objects[end].Formation == fi {
					end++
				}
				g.groupWrites = append(g.groupWrites, GenerationGroup{First: start, Count: end - start, Formation: fi, PathFirst: len(g.paths)})
				start = end
			}
		}
		g.log("A1876a", "intermediate_cache_paired", -1, g.current, fmt.Sprintf("objects and writer metadata[%d,%d); policy%s", k, k+n, CampaignGenerationPolicyPairedV1))
		return nil
	}
	g.log("A1876a", "intermediate_cache_hit", -1, g.current, fmt.Sprintf("objects[%d,%d), metadata[%d,%d)", k, k+n, k+n, k+2*n))
	return nil
}

func CampaignSnapshot(m *MissionData) CampaignMissionSnapshot {
	return CampaignMissionSnapshot{GenerationGroups: m.generationGroups, PlacementPlan: m.PlacementPlan, SourcePreparation: m.SourcePreparation, Scenario: m.Scenario, World: m.World, Theatre: m.Inputs.Theatre, Suffix: m.SourceWorld.Suffix, Scene: m.Scene, Objects: append([]MissionObject(nil), m.Objects...), Paths: append([]missionPath(nil), m.Paths...), Objectives: m.Objectives, Conditions: m.Conditions, FriendlyTitle: m.Intelligence.FriendlyTitle, RawE278: m.RawE278, FlightTemplate: m.FlightTemplate, UnavailableAircraft: append([]int(nil), m.CampaignUnavailableAircraft...), UnavailableSystems: append([]int(nil), m.CampaignUnavailableSystems...)}
}

func restoreCampaignMission(a *assetdata.MissionAssets, p *Pilot, s *CampaignState, in MissionInputs, rng *missionRNG) (*MissionData, error) {
	if s.Suspended == nil {
		return nil, fmt.Errorf("Campaign suspended snapshot is absent")
	}
	x := s.Suspended.Mission
	var w *assetdata.MissionWorld
	for i := range a.Generation.Worlds {
		candidate := &a.Generation.Worlds[i]
		if candidate.Theatre == x.Theatre.Raw() && candidate.Suffix == x.Suffix && candidate.Scenario == x.Scenario && candidate.WorldName == x.World {
			w = candidate
			break
		}
	}
	if w == nil {
		return nil, fmt.Errorf("Campaign snapshot source world/scenario is unavailable or changed")
	}
	pop, err := newMissionPopulation(w, rng, nil)
	if err != nil {
		return nil, err
	}
	for _, object := range x.Objects {
		if object.Formation < 0 || object.Formation > 255 {
			return nil, fmt.Errorf("Campaign snapshot provenance is not an original byte")
		}
	}
	// Restoring uses frozen artwork/title and raw OBJECT rows, not all FMHD
	// entries. Validate the actual objective consumers (also used by TARGET
	// preparation/outfit text), retaining unrelated old-world provenance.
	for _, objective := range x.Objectives {
		if objective.ObjectIndex < 0 || objective.ObjectIndex >= 80 || objective.Formation < 0 || objective.Formation >= len(pop.forms) {
			return nil, fmt.Errorf("Campaign snapshot objective reference is invalid")
		}
	}
	condition := x.Conditions
	condition.Operation = ""
	for _, site := range []string{"P17e2", "P17f9", "P180c"} {
		condition.Operation += string(a.Generation.Tables.Consonants[rng.r0(20, site)])
	}
	condition.Operation += fmt.Sprintf("%03d", rng.r0(1000, "P1820"))
	if condition.Flags&0x200 != 0 {
		condition.Hour = rng.inclusive(21, 27, "P1842") % 24
	} else {
		condition.Hour = rng.inclusive(9, 15, "P185c")
	}
	if rng.err != nil {
		return nil, rng.err
	}
	stable := "FIELD"
	if x.Scene == 4 {
		stable = "FIELD3"
	} else if x.Scene == 2 || x.Scene == 5 {
		stable = "FIELD2"
	}
	// The full original cache stores81 rows but its reader restores count80.
	objects := append([]MissionObject(nil), x.Objects[:80]...)
	m := &MissionData{generationGroups: x.GenerationGroups, PlacementPlan: x.PlacementPlan, Inputs: in, Scenario: w.Scenario, World: w.WorldName, Map: w.TerrainName + "MAP.PIC", StableScene: stable, Palette: "FIELD.PAL", Scene: x.Scene, Objects: objects, Paths: append([]missionPath(nil), x.Paths...), Objectives: x.Objectives, Conditions: condition, RawE278: x.RawE278, FlightTemplate: x.FlightTemplate, SourceWorld: w, CampaignUnavailableAircraft: append([]int(nil), x.UnavailableAircraft...), CampaignUnavailableSystems: append([]int(nil), x.UnavailableSystems...), Draws: rng.trace}
	m.SourcePreparation = x.SourcePreparation
	m.Intelligence = missionIntel(m.Objects, w.OBJL)
	m.Intelligence.FriendlyTitle = x.FriendlyTitle
	m.ObjectHash = campaignObjectHash(objects)
	return m, nil
}

func campaignObjectHash(objects []MissionObject) string {
	h := sha256.New()
	for _, o := range objects {
		h.Write(o.Raw[:])
	}
	return hex.EncodeToString(h.Sum(nil))
}

func CampaignPenalty(p *Pilot, amount uint16) error {
	if len(p.LegacyRaw) != LegacyRecordSize {
		return fmt.Errorf("Campaign result needs the preserved opaque progression word; nothing was saved")
	}
	x := binary.LittleEndian.Uint16(p.LegacyRaw[0x59:])
	if int16(x) < 3000 {
		binary.LittleEndian.PutUint16(p.LegacyRaw[0x59:], x+amount)
	}
	return nil
}

// ImportCampaignCache is an explicit read-only conversion. Callers preserve
// exact input/provenance separately; it never opens a path or overwrites a save.
func ImportCampaignCache(a *assetdata.MissionAssets, p *Pilot, full []byte, intermediates map[string][]byte) (*CampaignState, error) {
	if p == nil || len(p.LegacyRaw) != LegacyRecordSize || p.MissionType != MissionCampaign || p.LegacyRaw[0x4f] != 2 {
		return nil, fmt.Errorf("Campaign import requires the matching suspended legacy pilot record")
	}
	if a == nil || a.Generation == nil || len(full) > 1<<20 {
		return nil, fmt.Errorf("Campaign import source/assets are invalid")
	}
	pos := 0
	take := func(n int) ([]byte, error) {
		if n < 0 || pos+n > len(full) {
			return nil, fmt.Errorf("Campaign cache truncated at offset%d, need%d bytes", pos, n)
		}
		b := full[pos : pos+n]
		pos += n
		return b, nil
	}
	state, err := take(18)
	if err != nil {
		return nil, err
	}
	refs, err := take(10)
	if err != nil {
		return nil, err
	}
	objectRaw, err := take(2592)
	if err != nil {
		return nil, err
	}
	theatre, err := take(1)
	if err != nil {
		return nil, err
	}
	switches, err := take(20)
	if err != nil {
		return nil, err
	}
	ordinal, err := take(1)
	if err != nil {
		return nil, err
	}
	targets, err := take(310)
	if err != nil {
		return nil, err
	}
	provenance, err := take(80)
	if err != nil {
		return nil, err
	}
	countRaw, err := take(2)
	if err != nil {
		return nil, err
	}
	count := int(binary.LittleEndian.Uint16(countRaw))
	if count > 7281 {
		return nil, fmt.Errorf("Campaign PATH count exceeds original16-bit allocation")
	}
	pathRaw, err := take(count * 9)
	if err != nil {
		return nil, err
	}
	kinds, err := take(5)
	if err != nil {
		return nil, err
	}
	friendly, err := take(25)
	if err != nil {
		return nil, err
	}
	aircraft, err := take(180)
	if err != nil {
		return nil, err
	}
	flight, err := take(205)
	if err != nil {
		return nil, err
	}
	initialIDs, err := take(5)
	if err != nil {
		return nil, err
	}
	airRestrictions, err := take(4)
	if err != nil {
		return nil, err
	}
	systemRestrictions, err := take(4)
	if err != nil {
		return nil, err
	}
	if pos != len(full) {
		return nil, fmt.Errorf("Campaign cache has%d unexplained trailing bytes", len(full)-pos)
	}
	if int(theatre[0]) != p.Theatre.Raw() || ordinal[0] != p.LegacyRaw[0x58] {
		return nil, fmt.Errorf("Campaign cache theatre/ordinal does not match the supplied pilot")
	}
	var w *assetdata.MissionWorld
	for i := range a.Generation.Worlds {
		candidate := &a.Generation.Worlds[i]
		if candidate.Theatre == p.Theatre.Raw() && candidate.Suffix == int(int8(p.LegacyRaw[0x5b])) && candidate.SNHD[0] == ordinal[0] {
			w = candidate
			break
		}
	}
	if w == nil {
		return nil, fmt.Errorf("Campaign cache's scenario/world is not prepared")
	}
	x := CampaignMissionSnapshot{Scenario: w.Scenario, World: w.WorldName, Theatre: p.Theatre, Suffix: w.Suffix, Scene: int(int8(state[13])), Objects: make([]MissionObject, 81), FlightTemplate: -1, Objectives: map[string]MissionObjective{}, Conditions: MissionConditions{Flags: binary.LittleEndian.Uint16(switches), WindDirection: binary.LittleEndian.Uint16(switches[8:]), WindSpeed: int(int16(binary.LittleEndian.Uint16(switches[10:]))), Temperature: int8(state[10]), Callsign: int(state[11]), SecondCallsign: int(state[12])}}
	x.SourcePreparation = &CampaignSourceRaw{RuntimeHeader: append([]byte(nil), state...), SwitchBlock: append([]byte(nil), switches...), TargetBlock: append([]byte(nil), targets...), ObjectiveKinds: append([]byte(nil), kinds...), FriendlyLabel: append([]byte(nil), friendly...)}
	for i := range x.Objects {
		copy(x.Objects[i].Raw[:], objectRaw[i*32:(i+1)*32])
		if i < 80 {
			x.Objects[i].Formation = int(provenance[i])
		}
	}
	for i := 0; i < count; i++ {
		raw := pathRaw[i*9:]
		x.Paths = append(x.Paths, missionPath{MissionPoint{int32(binary.LittleEndian.Uint32(raw)), int32(binary.LittleEndian.Uint32(raw[4:]))}, raw[8]})
	}
	if state[16] != 0 {
		x.FriendlyTitle = strings.SplitN(string(friendly), "\x00", 2)[0]
	}
	for i := 0; i < 5; i++ {
		raw := targets[i*62 : (i+1)*62]
		index := int(int16(binary.LittleEndian.Uint16(refs[i*2:])))
		if index < 0 || index >= 80 {
			return nil, fmt.Errorf("Campaign target%d references invalid object%d", i, index)
		}
		rawKind := int(kinds[i])
		if rawKind >= len(a.Generation.Tables.DisplayKinds) {
			return nil, fmt.Errorf("Campaign target kind%d is unsupported", rawKind)
		}
		if i != 0 && rawKind == 0 {
			continue
		}
		role := missionRole(i)
		objective := MissionObjective{Role: role, ObjectIndex: index, Formation: int(provenance[index]), RawKind: rawKind, DisplayKind: a.Generation.Tables.DisplayKinds[rawKind], Count: int(binary.LittleEndian.Uint16(raw[50:])), Name: strings.SplitN(string(raw[:32]), "\x00", 2)[0], Point: MissionPoint{int32(binary.LittleEndian.Uint32(raw[42:])), int32(binary.LittleEndian.Uint32(raw[46:]))}, OriginalPoint: MissionPoint{int32(binary.LittleEndian.Uint32(raw[34:])), int32(binary.LittleEndian.Uint32(raw[38:]))}}
		if binary.LittleEndian.Uint32(raw[54:]) != 0 {
			objective.Endpoint = &MissionPoint{int32(binary.LittleEndian.Uint32(raw[54:])), int32(binary.LittleEndian.Uint32(raw[58:]))}
		}
		x.Objectives[role] = objective
	}
	s := &CampaignState{Version: 1, SourceProfile: assetdata.IslandIce469085.Name, Phase: CampaignSuspended, Theatre: p.Theatre, TargetCounter: int(p.LegacyRaw[0x50]), ResultCounters: [2]int{int(p.LegacyRaw[0x51]), int(p.LegacyRaw[0x52])}, LossesRelatedCount: int(binary.LittleEndian.Uint16(p.LegacyRaw[0xa5:])), SelectedScenario: w.Suffix, PriorScenario: int(int8(p.LegacyRaw[0x5c])), ScenarioOrdinal: int(ordinal[0]), Retained: CampaignRetained{Reserved: make([][32]byte, 5), Provenance: append([]byte(nil), provenance...)}}
	for i := range s.Retained.Reserved {
		s.Retained.Reserved[i] = x.Objects[i].Raw
	}
	settings, err := PreflightFromPilot(p)
	if err != nil {
		return nil, err
	}
	susp := &CampaignSuspension{Mission: x, Aircraft: make([][36]byte, 5), InitialAircraftIDs: append([]byte(nil), initialIDs...), Flight: make([]FlightAssignment, 5), Settings: settings}
	for i := 0; i < 5; i++ {
		copy(susp.Aircraft[i][:], aircraft[i*36:(i+1)*36])
		raw := flight[i*41 : (i+1)*41]
		source := int(raw[0])
		if source > 4 {
			return nil, fmt.Errorf("Campaign flight source%d is invalid", source)
		}
		susp.Flight[i] = FlightAssignment{source, int(raw[1]), raw[2] != 0}
	}
	for _, v := range airRestrictions {
		susp.Mission.UnavailableAircraft = append(susp.Mission.UnavailableAircraft, int(int8(v)))
	}
	for _, v := range systemRestrictions {
		susp.Mission.UnavailableSystems = append(susp.Mission.UnavailableSystems, int(int8(v)))
	}
	s.Suspended = susp
	sum := sha256.Sum256(full)
	s.ImportSHA256 = hex.EncodeToString(sum[:])
	// Intermediate files do not contain sufficient standalone metadata history.
	// Retain only imports matching this full snapshot's scenario; other keys need
	// their own explicit source association rather than guessed basename parsing.
	for name, raw := range intermediates {
		if name == "" || len(raw) < 2 {
			return nil, fmt.Errorf("Campaign intermediate input is truncated")
		}
		expectedName := fmt.Sprintf("CAMPGN%d%d.DAT", int(int8(p.LegacyRaw[0x4e])), int(int8(ordinal[0])))
		if !strings.EqualFold(name, expectedName) {
			return nil, fmt.Errorf("Campaign intermediate %q has no established association with snapshot ordinal%d (expected%s)", name, ordinal[0], expectedName)
		}
		if len(intermediates) > 1 {
			return nil, fmt.Errorf("only the snapshot-associated intermediate cache can be imported explicitly")
		}
		n := int(int8(raw[1]))
		if n < 0 || len(raw) != 2+33*n {
			return nil, fmt.Errorf("Campaign intermediate framing is incomplete")
		}
		cache := CampaignFormationCache{Scenario: w.Scenario, World: w.WorldName, Ordinal: int(ordinal[0]), BeforeCount: int(raw[0]), Objects: make([]MissionObject, n), Provenance: append([]byte(nil), raw[2+32*n:]...)}
		for i := range cache.Objects {
			copy(cache.Objects[i].Raw[:], raw[2+32*i:2+32*(i+1)])
			cache.Objects[i].Formation = int(cache.Provenance[i])
		}
		s.Caches = append(s.Caches, cache)
	}
	upgradeCampaignKnowledge(s) // Import omissions remain unknown, including future ordinals.
	if err = s.Validate(); err != nil {
		return nil, err
	}
	return s, nil
}
