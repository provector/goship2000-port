package state

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	"math/rand"
	"strings"
)

type CampaignGenerationInputs struct {
	Stage, Raw50, Raw51, Raw52, LossWord int
	ScenarioCacheAbsent                  bool
	Caches                               []CampaignFormationCache `json:"-"`
	Retained                             *CampaignRetained        `json:"-"`
	CacheKnowledgeVersion                int                      `json:"-"`
	CacheInventory                       []CampaignCacheKnowledge `json:"-"`
	PlacementPolicy                      string                   `json:"placement_policy,omitempty"`
	GenerationPolicy                     string                   `json:"generation_policy,omitempty"`
	// Nil preserves exact complete historical carriers. A supplied mask is a
	// transient partial-input proof; it is never serialized as original bytes.
	ProvenanceKnown *[80]bool `json:"-"`
}

type MissionInputs struct {
	rng              *missionRNG
	Campaign         *CampaignGenerationInputs
	MissionType      MissionType
	Theatre          Theatre
	Rank             Rank
	PilotName        string
	Seed             int64
	DrawStream       []int
	PreviousScenario int
	RetainedObject   []byte
	GenerationPolicy string `json:"generation_policy,omitempty"`
	PlacementPolicy  string `json:"placement_policy,omitempty"`
}

type MissionPoint struct{ X, Y int32 }

type MissionDraw struct {
	Site         string
	Bound, Value int
}

type MissionDecision struct {
	Site, Operation string
	Formation       int
	Point           MissionPoint
	Reason          string
}

type MissionObject struct {
	Raw       [32]byte
	Formation int
}

func (o MissionObject) Point() MissionPoint {
	return MissionPoint{int32(binary.LittleEndian.Uint32(o.Raw[2:])), int32(binary.LittleEndian.Uint32(o.Raw[6:]))}
}

type MissionObjective struct {
	Role                                                string
	Formation, ObjectIndex, RawKind, DisplayKind, Count int
	Name                                                string
	Point, OriginalPoint                                MissionPoint
	Endpoint                                            *MissionPoint
}

type MissionConditions struct {
	Flags                    uint16
	WindDirection            uint16
	WindSpeed                int
	Temperature              int8
	Callsign, SecondCallsign int
	Hour                     int
	Operation                string
}

type MissionIntelligence struct {
	GroundIDs           []int
	GroundNames         []string
	AirID, AirCount     int
	AirName, AirDensity string
	FriendlyTitle       string
}

type MissionData struct {
	generationGroups []GenerationGroup
	// SoloBuilder retains the single-aircraft reservation of a generated layout.
	SoloBuilder                                             bool                  `json:"solo_builder,omitempty"`
	PlacementPlan                                           *RequiredFARPPlan     `json:"placement_plan,omitempty"`
	Support                                                 *SupportConfiguration `json:"support,omitempty"`
	Targets                                                 *MissionTargets       `json:"targets,omitempty"`
	TargetCountdown                                         *uint16               `json:"target_countdown,omitempty"`
	SourcePreparation                                       *CampaignSourceRaw    `json:"source_preparation,omitempty"`
	campaignInitialCache                                    *CampaignFormationCache
	campaignRetained                                        *CampaignRetained
	RawE278                                                 int
	Paths                                                   []missionPath
	CampaignUnavailableAircraft, CampaignUnavailableSystems []int
	Inputs                                                  MissionInputs
	Scenario, World, Map, StableScene, Palette              string
	Scene                                                   int
	Objects                                                 []MissionObject
	Objectives                                              map[string]MissionObjective
	Conditions                                              MissionConditions
	Intelligence                                            MissionIntelligence
	Draws                                                   []MissionDraw
	Decisions                                               []MissionDecision
	ObjectHash                                              string
	FlightTemplate                                          int
	SourceWorld                                             *assetdata.MissionWorld
}

// The stream is a development-only reference input; bound<=1 consumes no draw.
type missionRNG struct {
	// Only authored compilation uses the named CRT stream; existing generators retain their clock/RNG policy.
	authoredCRT *uint32
	local       *rand.Rand
	stream      []int
	explicit    bool
	cursor      int
	trace       []MissionDraw
	err         error
}

func newMissionRNG(in MissionInputs) *missionRNG {
	return &missionRNG{local: rand.New(rand.NewSource(in.Seed)), stream: in.DrawStream, explicit: in.DrawStream != nil}
}

func (r *missionRNG) r0(n int, site string) int {
	if n <= 1 {
		return 0
	}
	if r.err != nil {
		return 0
	}
	v := 0
	if r.explicit {
		if r.cursor >= len(r.stream) {
			r.err = fmt.Errorf("mission draw stream exhausted at %s, bound %d", site, n)
			return 0
		}
		v = r.stream[r.cursor]
		r.cursor++
	} else if r.authoredCRT != nil {
		*r.authoredCRT = *r.authoredCRT*214013 + 2531011
		raw := (*r.authoredCRT >> 16) & 32767
		v = int(raw * uint32(n) >> 15)
	} else {
		v = r.local.Intn(n)
	}
	if v < 0 || v >= n {
		r.err = fmt.Errorf("mission draw %d outside [0,%d) at %s", v, n, site)
		return 0
	}
	r.trace = append(r.trace, MissionDraw{site, n, v})
	return v
}

func (r *missionRNG) inclusive(lo, hi int, site string) int {
	if hi <= lo {
		return lo
	}
	return lo + r.r0(hi-lo+1, site)
}

func MissionI16(v int) int { return int(int16(v)) }

func missionI8(v int) int { return int(int8(v)) }

func missionAbs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func missionMetric(a, b [2]int) int {
	x, y := missionAbs(MissionI16(a[0]-b[0])), missionAbs(MissionI16(a[1]-b[1]))
	if x < y {
		x, y = y, x
	}
	return MissionI16(x + y/2)
}

func missionCell(p MissionPoint, scale int) [2]int { return [2]int{int(p.X) / scale, int(p.Y) / scale} }

func missionPoint(x, y int) MissionPoint { return MissionPoint{int32(x), int32(y)} }

func MissionGrid(v int32) int { return int(int16((v + 0x4000) / 0x8000)) }

func MissionMapPoint(p MissionPoint, areaX, areaY int) (int, int) {
	return MissionI16(int(p.X/3277) + areaX + 19), MissionI16(-int(p.Y/4096) + areaY + 145)
}

func GenerateMission(data *assetdata.MissionGenerationData, in MissionInputs) (*MissionData, error) {
	if data == nil || data.Version != 1 {
		return nil, fmt.Errorf("mission-generation data version is unsupported")
	}
	if !validCampaignGenerationPolicy(in.GenerationPolicy) {
		return nil, fmt.Errorf("mission generation policy is unsupported")
	}
	if err := validateMissionTables(data.Tables); err != nil {
		return nil, err
	}
	if in.Rank.Raw() < 0 || in.Theatre.Raw() < 0 || in.MissionType.Raw() < 0 {
		return nil, fmt.Errorf("invalid mission inputs")
	}
	if in.MissionType != MissionTraining {
		return generateOtherMission(data, in)
	}
	if in.Theatre.Raw() > 1 {
		return nil, fmt.Errorf("Training is supported only in Central Europe and Persian Gulf")
	}
	var world *assetdata.MissionWorld
	for i := range data.Worlds {
		w := &data.Worlds[i]
		if w.Theatre == in.Theatre.Raw() && len(w.THHD) > 22 && w.Suffix == int(w.THHD[22]) {
			world = w
			break
		}
	}
	if world == nil {
		return nil, fmt.Errorf("prepared Training world missing for %s", in.Theatre)
	}
	rng := newMissionRNG(in)
	condition, err := missionConditions(world, data.Tables, rng, true)
	if err != nil {
		return nil, err
	}
	pop, err := newMissionPopulation(world, rng, in.RetainedObject)
	if err != nil {
		return nil, err
	}
	if err = pop.training(); err != nil {
		return nil, err
	}
	template, err := missionChooseTemplate(data.Tables, rng, 0, 0, 1, in.Rank.Raw())
	if err != nil {
		return nil, err
	}
	if err := pop.validate(); err != nil {
		return nil, err
	}
	result := missionResult(world, in, condition, pop, data.Tables)
	result.FlightTemplate = template
	return result, nil
}

func missionConditions(w *assetdata.MissionWorld, t assetdata.MissionTables, r *missionRNG, training bool) (MissionConditions, error) {
	if len(w.THHD) != 54 || len(t.Consonants) != 20 || len(t.Callsigns) != 26 {
		return MissionConditions{}, fmt.Errorf("invalid prepared mission conditions tables")
	}
	c := MissionConditions{}
	bit8 := r.r0(5, "A19eec") < 1
	bit9 := r.r0(100, "A19f0a") > 40
	if bit8 {
		c.Flags |= 0x100
	}
	if bit9 {
		c.Flags |= 0x200
	}
	c.WindDirection = uint16(r.r0(256, "A19f32") << 8)
	weighted := func(rows [][]int, draw int) (int, error) {
		for _, row := range rows {
			if len(row) < 2 {
				return 0, fmt.Errorf("short weighted table row")
			}
			draw -= missionI8(row[0])
			if draw <= 0 {
				return missionI8(row[1]), nil
			}
		}
		return 0, fmt.Errorf("weights do not cover bounded draw")
	}
	var err error
	c.WindSpeed, err = weighted(t.Wind, 1+r.r0(100, "A19f43/R1"))
	if err != nil {
		return c, err
	}
	temp, err := weighted(t.Temperature, 1+r.r0(100, "A19f84/R1"))
	if err != nil {
		return c, err
	}
	ix := 25
	if bit9 {
		ix++
	}
	c.Temperature = int8(int(w.THHD[ix]) + temp + r.r0(5, "A19fb2"))
	c.Callsign = r.r0(26, "A19fe2")
	c.SecondCallsign = r.r0(25, "A19fef")
	if c.SecondCallsign >= c.Callsign {
		c.SecondCallsign++
	}
	for _, site := range []string{"P17e2", "P17f9", "P180c"} {
		c.Operation += string(t.Consonants[r.r0(20, site)])
	}
	c.Operation += fmt.Sprintf("%03d", r.r0(1000, "P1820"))
	if bit9 {
		c.Hour = r.inclusive(21, 27, "P1842") % 24
	} else {
		c.Hour = r.inclusive(9, 15, "P185c")
	}
	if training {
		r.r0(100, "A1a017")
		r.r0(5, "A1a092")
	}
	return c, r.err
}

func missionChooseTemplate(t assetdata.MissionTables, r *missionRNG, first, second, count, rank int) (int, error) {
	for attempt := 0; attempt < 10000; attempt++ {
		roll := 1 + r.r0(100, "A22539/R1")
		chosen := -1
		for ix, row := range t.Templates {
			if len(row) != 47 {
				return 0, fmt.Errorf("invalid 47-byte flight template")
			}
			if int(int8(row[0])) == first {
				roll -= int(int8(row[1]))
				if roll <= 0 {
					chosen = ix
					break
				}
			}
		}
		if chosen < 0 {
			return 0, fmt.Errorf("flight template weights do not cover draw")
		}
		row := t.Templates[chosen]
		ok := true
		secondary := second == 0
		for i := 0; i < count; i++ {
			typ := int(int8(row[2+9*i]))
			threshold, exists := t.Thresholds[typ]
			if !exists || threshold > rank {
				ok = false
			}
			if second == 1 && typ == 5 || second == 2 && (typ == 6 || typ == 3 || typ == 4) {
				secondary = true
			}
		}
		if r.err != nil {
			return 0, r.err
		}
		if ok && secondary {
			return chosen, nil
		}
	}
	return 0, fmt.Errorf("flight-template diagnostic retry limit reached")
}

func missionResult(w *assetdata.MissionWorld, in MissionInputs, c MissionConditions, p *missionPopulation, t assetdata.MissionTables) *MissionData {
	scene := int(w.SNHD[10] & 15)
	base := p.roles[0]
	if p.forms[p.objects[base].Formation].Flags&0x40000000 != 0 {
		if scene == 4 {
			scene = 5
		} else {
			scene = 2
		}
	}
	stable := "FIELD"
	if scene == 4 {
		stable = "FIELD3"
	} else if scene == 2 || scene == 5 {
		stable = "FIELD2"
	}
	d := &MissionData{generationGroups: p.groupWrites, Inputs: in, Scenario: w.Scenario, World: w.WorldName, Map: w.TerrainName + "MAP.PIC", StableScene: stable, Palette: "FIELD.PAL", Scene: scene, Conditions: c, Objects: p.objects, Objectives: map[string]MissionObjective{}, Draws: p.rng.trace, Decisions: p.decisions, SourceWorld: w}
	for role, index := range p.roles {
		name := map[int]string{0: "base", 1: "FARP", 2: "primary", 3: "secondary", 4: "tertiary"}[role]
		obj := p.objects[index]
		f := p.forms[obj.Formation]
		kind := 0
		if role >= 2 {
			kind = 3
		}
		count := 0
		for i := index; i < len(p.objects) && p.objects[i].Formation == obj.Formation; i++ {
			count++
		}
		if count > 2 {
			count -= count / 2
		}
		if role < 2 {
			kind = 1
		}
		d.Objectives[name] = MissionObjective{Role: name, Formation: obj.Formation, ObjectIndex: index, RawKind: kind, DisplayKind: t.DisplayKinds[kind], Count: count, Name: f.Name, Point: p.points[role], OriginalPoint: p.points[role]}
	}
	h := sha256.New()
	for _, o := range d.Objects {
		h.Write(o.Raw[:])
	}
	d.ObjectHash = hex.EncodeToString(h.Sum(nil))
	d.Intelligence = missionIntel(d.Objects, w.OBJL)
	return d
}

func missionIntel(objects []MissionObject, objl []byte) MissionIntelligence {
	ground, air := [230]byte{}, [230]byte{}
	count := 0
	for _, o := range objects {
		b := o.Raw
		if b[11] == 0 {
			continue
		}
		id := int(b[12])
		if id >= 230 {
			continue
		}
		if b[0]&0x30 == 0 && b[10] > 2 && b[10] < 16 {
			ground[id]++
		}
		if b[10] < 2 {
			air[id]++
			count++
		}
	}
	distinct := 0
	for _, v := range ground {
		if v != 0 {
			distinct++
		}
	}
	ground[34] += ground[42]
	ground[42] = 0
	result := MissionIntelligence{AirID: -1, AirCount: count}
	last := 0
	for n := 0; n < min(distinct, 4); n++ {
		best := byte(0)
		ix := last
		for i, v := range ground {
			if v > best {
				best = v
				ix = i
			}
		}
		last = ix
		result.GroundIDs = append(result.GroundIDs, ix)
		ground[ix] = 0
	}
	if count > 0 {
		best := byte(0)
		for i, v := range air {
			if v > best {
				best = v
				result.AirID = i
			}
		}
		result.AirDensity = "light"
		if count >= 4 {
			result.AirDensity = "heavy"
		}
		if count > 5 {
			result.AirDensity = "extremely heavy"
		}
	}
	names := map[int]string{}
	for i := 0; i+32 <= len(objl); i += 32 {
		id := int(objl[i+12])
		if _, ok := names[id]; !ok {
			names[id] = strings.TrimRight(strings.SplitN(string(objl[i+15:i+32]), "\x00", 2)[0], " ")
		}
	}
	for _, id := range result.GroundIDs {
		result.GroundNames = append(result.GroundNames, names[id])
	}
	result.AirName = names[result.AirID]
	return result
}

// Prepared files are mutable user input; reject a damaged table before indexing it.
func validateMissionTables(t assetdata.MissionTables) error {
	if len(t.Wind) != 14 || len(t.Temperature) != 6 || len(t.Callsigns) != 26 || len(t.Compass) != 16 || len(t.Ranks) != 12 || len(t.Consonants) != 20 || len(t.Templates) != 40 || len(t.Primary) != 4 || len(t.Secondary) != 4 || len(t.Masks) != 8 || len(t.DisplayKinds) != 8 || len(t.Leader) != 32 {
		return fmt.Errorf("incomplete prepared mission tables; rebuild assets")
	}
	for _, rows := range [][][]int{t.Wind, t.Temperature} {
		for _, row := range rows {
			if len(row) != 2 {
				return fmt.Errorf("malformed mission weight/value row")
			}
		}
	}
	for _, rows := range [][][]int{t.Primary, t.Secondary} {
		for _, row := range rows {
			if len(row) != 7 {
				return fmt.Errorf("malformed objective weight row")
			}
		}
	}
	for _, row := range t.Templates {
		if len(row) != 47 {
			return fmt.Errorf("malformed aircraft template")
		}
	}
	return nil
}

func (p *missionPopulation) validate() error {
	for _, o := range p.objects {
		if o.Raw[11] != 0 && o.Raw[12] >= 230 {
			return fmt.Errorf("active unit ID %d exceeds the source briefing histogram", o.Raw[12])
		}
	}
	return nil
}
