package state

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
)

type missionBuilder struct {
	placementPrefix                                            []byte
	placementObjects                                           []MissionObject
	placementRoles                                             []placementRole
	placementPlan                                              *RequiredFARPPlan
	g                                                          *genericMissionPopulation
	tables                                                     assetdata.MissionTables
	in                                                         MissionInputs
	conditions                                                 MissionConditions
	kinds                                                      map[int]int
	targets                                                    map[int]MissionObjective
	refs                                                       map[int]int
	scene, flags, mode, special, base, oldBase, upper, rawE278 int
	unavailableAircraft, unavailableSystems                    []int
	initialCache                                               *CampaignFormationCache
	support                                                    *SupportConfiguration
	recoveredObjectives                                        map[int]missionRecoveredObjective
}

type missionRecoveredObjective struct {
	Point            MissionPoint
	Formation, Start int
	Tile             byte // Free pre-expansion terrain byte, not an occupancy exception for other sites.
}

func generateOtherMission(data *assetdata.MissionGenerationData, in MissionInputs) (*MissionData, error) {
	mission := in.MissionType.Raw()
	if mission != 1 && mission != 3 && mission != 4 {
		return nil, fmt.Errorf("%s preparation is not yet supported by this runtime; office state remains unchanged", in.MissionType)
	}
	if mission == 4 && in.Campaign == nil {
		return nil, fmt.Errorf("Campaign requires explicit consistent source state")
	}
	if in.PlacementPolicy != "" && (in.PlacementPolicy != RequiredFARPPlacementV1 || in.GenerationPolicy != CampaignGenerationPolicyPairedV1) {
		return nil, fmt.Errorf("mission placement policy is unsupported")
	}
	if mission == 4 && (in.Campaign.GenerationPolicy != in.GenerationPolicy || in.Campaign.PlacementPolicy != in.PlacementPolicy) {
		return nil, fmt.Errorf("Campaign and mission generation policies disagree")
	}
	if mission == 4 && (in.Campaign.Stage != 1 || in.Campaign.Raw50 < 0 || in.Campaign.Raw50 > 255 || in.Campaign.Raw51 < 0 || in.Campaign.Raw51 > 255 || in.Campaign.Raw52 < 0 || in.Campaign.Raw52 > 255 || in.Campaign.LossWord < 0 || in.Campaign.LossWord > 65535) {
		return nil, fmt.Errorf("Campaign requires active stage1 and explicit byte/word-width progress inputs")
	}
	var header []byte
	for _, w := range data.Worlds {
		if w.Theatre == in.Theatre.Raw() {
			header = w.THHD
			break
		}
	}
	if len(header) != 54 {
		return nil, fmt.Errorf("prepared theatre header missing")
	}
	rng := in.rng
	if rng == nil {
		rng = newMissionRNG(in)
	}
	count := int(int8(header[22]))
	if mission == 1 && in.Theatre.Raw() == 0 && count == 4 {
		count = 3
	}
	avoid := in.PreviousScenario >= 0 && in.PreviousScenario < count
	bound := count
	if avoid {
		bound--
	}
	suffix := rng.r0(bound, "A16661")
	if avoid && suffix >= in.PreviousScenario {
		suffix++
	}
	var w *assetdata.MissionWorld
	for i := range data.Worlds {
		if data.Worlds[i].Theatre == in.Theatre.Raw() && data.Worlds[i].Suffix == suffix {
			w = &data.Worlds[i]
			break
		}
	}
	if w == nil {
		return nil, fmt.Errorf("prepared scenario %d missing in %s", suffix, in.Theatre)
	}
	conditions, err := missionConditions(w, data.Tables, rng, false)
	if err != nil {
		return nil, err
	}
	limitDraw := rng.r0(100, "A1a017")
	g, err := newGenericMissionPopulation(w, rng, mission, data.Tables.Leader)
	if err != nil {
		return nil, err
	}
	if mission == 4 && in.Campaign.Retained != nil {
		r := in.Campaign.Retained
		if err := r.Validate(); err != nil {
			return nil, err
		}
		for i := range g.objects {
			if in.Campaign.ProvenanceKnown != nil && !in.Campaign.ProvenanceKnown[i] {
				return nil, fmt.Errorf("Campaign reserved provenance row%d is unknown", i)
			}
			if i < len(r.Reserved) {
				g.objects[i].Raw = r.Reserved[i]
			}
			g.objects[i].Formation = int(r.Provenance[i])
		}
		g.endpoint = r.Endpoint
	}
	if mission == 3 || mission == 4 {
		if in.Theatre.Raw() == 0 {
			if limitDraw < 10 {
				g.limitMax = 2
			} else if limitDraw < 30 {
				g.limitMax = 1
			}
		} else if limitDraw < 15 {
			g.limitMax = 1
		}
	}
	b := &missionBuilder{g: g, tables: data.Tables, in: in, conditions: conditions, kinds: map[int]int{0: 1, 1: 0, 2: 0, 3: 0, 4: 0}, targets: map[int]MissionObjective{}, refs: map[int]int{}, scene: int(w.SNHD[10] & 15), flags: int(conditions.Flags >> 8), special: -1, upper: 9}
	if mission == 4 && int8(in.Campaign.Raw51) > int8(in.Campaign.Raw52) {
		b.upper = 11
	}
	if err = b.run(); err != nil {
		return nil, err
	}
	if err := g.validate(); err != nil {
		return nil, err
	}
	if mission == 4 && in.Campaign.ProvenanceKnown != nil {
		// All successful full-population rows were either known input or a
		// source appender's selectedFormation write; publish no unknown state.
		if len(g.objects) != 80 {
			return nil, fmt.Errorf("partial Campaign provenance requires complete final80 population")
		}
		for i, object := range g.objects {
			if object.Formation < 0 || object.Formation > 255 {
				return nil, fmt.Errorf("Campaign final provenance row%d is undefined", i)
			}
		}
	}
	stable := "FIELD"
	if b.scene == 4 {
		stable = "FIELD3"
	} else if b.scene == 2 || b.scene == 5 {
		stable = "FIELD2"
	}
	conditions.Flags = uint16(b.flags << 8)
	result := &MissionData{generationGroups: g.groupWrites, PlacementPlan: b.placementPlan, Support: b.support, Inputs: in, Scenario: w.Scenario, World: w.WorldName, Map: w.TerrainName + "MAP.PIC", StableScene: stable, Palette: "FIELD.PAL", Scene: b.scene, Objects: g.objects, Objectives: map[string]MissionObjective{}, Conditions: conditions, Intelligence: missionIntel(g.objects, w.OBJL), Draws: rng.trace, Decisions: g.decisions, SourceWorld: w, RawE278: b.rawE278, Paths: g.paths, CampaignUnavailableAircraft: b.unavailableAircraft, CampaignUnavailableSystems: b.unavailableSystems}
	if mission == 4 {
		result.campaignInitialCache = b.initialCache
		result.campaignRetained = campaignRetainedFromPopulation(g)
	}
	for role, target := range b.targets {
		name := missionRole(role)
		target.Role = name
		result.Objectives[name] = target
	}
	// GS A1b11a..1b18c uses the effective secondary OBJECT and the actual
	// start OBJECT local, not the relocated paper coordinates. Keep that
	// producer here while both source references and PATH ownership exist.
	if b.kinds[4] != 0 {
		seed, e := SourceObjectiveCountdown(g.point(b.refs[3]), g.rawPoint(b.base))
		if e != nil {
			return nil, e
		}
		result.TargetCountdown = &seed
	}
	if b.mode != 0 && b.special >= 0 {
		formation, err := g.objectFormation(b.special, "friendly Intelligence title")
		if err != nil {
			return nil, err
		}
		result.Intelligence.FriendlyTitle = formation.Name
	}
	first, second := 0, 0
	for i, role := range []int{2, 3} {
		k := data.Tables.DisplayKinds[b.kinds[role]]
		value := 0
		if k == 1 || k == 2 {
			value = 1
		} else if k == 4 {
			value = 2
		}
		if i == 0 {
			first = value
		} else {
			second = value
		}
	}
	result.FlightTemplate, err = missionChooseTemplate(data.Tables, rng, first, second, g.e282, in.Rank.Raw())
	if err != nil {
		return nil, err
	}
	if mission == 4 {
		b.restrictions()
		result.CampaignUnavailableAircraft = b.unavailableAircraft
		result.CampaignUnavailableSystems = b.unavailableSystems
	}
	if rng.err != nil {
		return nil, rng.err
	}
	result.Draws = rng.trace
	h := sha256.New()
	for _, o := range result.Objects {
		h.Write(o.Raw[:])
	}
	result.ObjectHash = hex.EncodeToString(h.Sum(nil))
	if result.PlacementPlan != nil {
		result.PlacementPlan.PopulationDigest = authoredDigest(struct {
			Objects    []MissionObject
			Paths      []missionPath
			Objectives map[string]MissionObjective
		}{result.Objects, result.Paths, result.Objectives})
	}
	return result, nil
}

func missionRole(r int) string {
	return map[int]string{0: "base", 1: "FARP", 2: "primary", 3: "secondary", 4: "tertiary"}[r]
}

func (b *missionBuilder) until(site string, required, forbidden uint32, allow func(int) bool) int {
	for n := 0; n < 10000; n++ {
		fi := b.g.pick(required, forbidden)
		if fi < 0 {
			if b.g.rng.err == nil {
				b.g.rng.err = fmt.Errorf("%s: no formation for masks %08x/%08x", site, required, forbidden)
			}
			return -1
		}
		if allow(fi) {
			return fi
		}
		b.g.log(site, "reject_outer_candidate", fi, b.g.current, "")
		if b.g.rng.err != nil {
			return -1
		}
	}
	b.g.rng.err = fmt.Errorf("%s: mission placement diagnostic retry limit", site)
	return -1
}

func (b *missionBuilder) distance(index int) int {
	return missionMetric(missionCell(b.g.current, 32768), missionCell(b.g.point(index), 32768))
}

func (b *missionBuilder) edge(point *[2]int) int {
	p := b.g.boundary
	if point != nil {
		p = *point
	}
	return missionMetric(missionCell(b.g.current, 32768), p)
}

func (b *missionBuilder) chooseKinds() {
	row := 0
	if b.g.mission == 3 {
		row = 1
	} else if b.g.mission == 4 {
		row = 3
		if int8(b.in.Campaign.Raw51) > int8(b.in.Campaign.Raw52) {
			row = 2
		}
	}
	weighted := func(weights []int, roll int) int {
		for i, v := range weights {
			roll -= missionI8(v)
			if roll <= 0 {
				return i + 1
			}
		}
		b.g.rng.err = fmt.Errorf("objective weights do not cover draw")
		return 0
	}
	if len(b.tables.Primary) < 4 || len(b.tables.Secondary) < 4 {
		b.g.rng.err = fmt.Errorf("objective weight tables missing")
		return
	}
	primary := weighted(b.tables.Primary[row], 1+b.g.rng.r0(100, "A192e4/R1"))
	secondaryRow := 0
	if b.g.mission == 1 {
		secondaryRow = 3
	} else if primary > 6 {
		secondaryRow = 2
	} else if primary > 3 {
		secondaryRow = 1
	}
	secondary := 0
	for n := 0; n < 10000; n++ {
		secondary = weighted(b.tables.Secondary[secondaryRow], 1+b.g.rng.r0(100, "A1935a/R1"))
		if b.g.mission == 1 || secondary != primary {
			break
		}
		if b.g.rng.err != nil {
			return
		}
	}
	b.kinds[2], b.kinds[3], b.kinds[4] = primary, secondary, 3
	reserved := b.g.objects
	b.g.objects = nil
	fi := b.g.pick(0x20, 0x00800000)
	b.g.objects = reserved
	if fi < 0 && b.g.rng.err == nil {
		b.kinds[2] = 2
	}
	if b.g.rng.r0(5, "A1a092") != 0 {
		b.kinds[4] = 0
	}
	if b.kinds[2] == 2 {
		b.flags |= 8
		b.kinds[4] = 0
	}
	if b.kinds[3] == 4 || b.kinds[3] == 6 {
		b.kinds[4] = 0
	}
	if b.flags&8 != 0 {
		b.kinds[0] = 2
	}
}

func (b *missionBuilder) initial() {
	before := len(b.g.objects)
	if b.g.mission == 4 {
		if err := checkCampaignCacheRead(b.in.Campaign, b.g.source); err != nil {
			b.g.rng.err = err
			return
		}
		for _, cache := range b.in.Campaign.Caches {
			if cache.Scenario == b.g.source.Scenario {
				b.g.rng.err = applyCampaignIntermediateKnown(b.g, cache, b.in.Campaign.Retained, b.in.Campaign.ProvenanceKnown, b.in.Campaign.GenerationPolicy == CampaignGenerationPolicyPairedV1)
				return
			}
		}
		if b.in.Campaign.CacheKnowledgeVersion < 2 && !b.in.Campaign.ScenarioCacheAbsent {
			b.g.rng.err = fmt.Errorf("Campaign scenario cache provenance is unavailable")
			return
		}
	}
	optional := 0
	for fi, f := range b.g.forms {
		if f.Flags&0x4000 == 0 {
			if optional >= 10 || f.Flags&0x8000 == 0 {
				continue
			}
			bound := 5
			if b.g.mission == 1 {
				bound = 2
			}
			if b.g.rng.r0(bound, "A1886c") != 0 {
				continue
			}
		}
		if f.Flags&0x8000 != 0 {
			optional++
		}
		b.g.selected = fi
		b.g.reused = -1
		b.g.start(fi)
		b.g.expand()
		if b.g.rng.err != nil {
			return
		}
	}
	if b.g.mission == 4 && b.g.rng.err == nil {
		b.initialCache = &CampaignFormationCache{Groups: append([]GenerationGroup(nil), b.g.groupWrites...), Scenario: b.g.source.Scenario, World: b.g.source.WorldName, Ordinal: int(b.g.source.SNHD[0]), BeforeCount: before, Objects: append([]MissionObject(nil), b.g.objects[before:]...)}
		for _, object := range b.g.objects[before:] {
			b.initialCache.Provenance = append(b.initialCache.Provenance, byte(object.Formation))
		}
	}
}

func (b *missionBuilder) record(role, index, fi, count int) {
	g := b.g
	point := g.point(index)
	original := point
	kind := b.kinds[role]
	offsets := [2]int{}
	selfOccupied := false
	if role >= 2 && g.rawPoint(index).X != 0 && (kind == 1 || kind == 6) {
		accepted := false
		for attempt := 0; attempt < 10000; attempt++ {
			minimum := 0
			if attempt < 100 {
				minimum = 100
			} else if attempt < 400 {
				minimum = 50
			}
			raw := [2]int{g.rng.inclusive(-100, 100, "A18f29"), g.rng.inclusive(-100, 100, "A18f3c")}
			for i := 0; i < 2; i++ {
				offsets[i] = raw[i] + minimum
				if raw[i] < 0 {
					offsets[i] = raw[i] - minimum
				}
			}
			candidate := MissionPoint{original.X + (int32(offsets[0])<<15)/100, original.Y + (int32(offsets[1])<<15)/100}
			cell := [2]int{MissionI16(int((candidate.X + 4096) / 8192)), MissionI16(int((candidate.Y + 4096) / 8192))}
			allowed := g.acceptable(cell[0], cell[1], g.forms[fi].Flags)
			if !allowed && b.recoveredObjectiveOwnsCell(role, index, fi, count, original, cell) {
				allowed, selfOccupied = true, true
			}
			if allowed {
				point = missionPoint(cell[0]*8192+4096, cell[1]*8192+4096)
				accepted = true
				break
			}
			if g.rng.err != nil {
				return
			}
		}
		if !accepted {
			cell := missionCell(original, 8192)
			if b.recoveredObjectiveOwnsCell(role, index, fi, count, original, cell) {
				// This source retry can have no unoccupied local point after the
				// accepted site occupied its own cell. Keep the already validated
				// center, never a guessed neighboring water point.
				point, accepted, selfOccupied = original, true, true
			} else {
				g.rng.err = fmt.Errorf("objective relocation diagnostic retry limit: %s rawkind%d formation%d at (%d,%d) in %s", missionRole(role), kind, fi, original.X, original.Y, g.source.WorldName)
				return
			}
		}
	}
	if selfOccupied {
		g.log("A18eea", "limited_objective_own_cell", fi, point, "validated free fallback cell occupied only by this just-expanded site")
	}
	if role >= 2 && (g.rawPoint(index).X == 0 || offsets != [2]int{}) {
		point.X = min(int32(0x76000), max(int32(0x8000), point.X))
		point.Y = min(int32(0x76000), max(int32(0x8000), point.Y))
	}
	mapped := b.tables.DisplayKinds[kind]
	amount := count
	if mapped == 2 {
		amount = 1
	} else if count > 2 {
		amount = count - count/2
	}
	name := g.forms[fi].Name
	for off := 0; off < len(g.source.FRMD); off += 8 {
		if int(g.source.FRMD[off]) == g.forms[fi].ID {
			if g.source.FRMD[off+2] == 2 {
				obj := g.staged[index]
				if index < len(g.objects) {
					obj = g.objects[index]
				}
				name = missionCString(obj.Raw[15:27])
			}
			break
		}
	}
	target := MissionObjective{Role: missionRole(role), Formation: fi, ObjectIndex: index, RawKind: kind, DisplayKind: mapped, Count: amount, Name: name, Point: point, OriginalPoint: original}
	if g.endpoint.X != 0 {
		ep := g.endpoint
		target.Endpoint = &ep
	}
	b.targets[role] = target
	b.refs[role] = index
	g.roles[role] = index
	g.points[role] = point
	if role == 0 {
		g.baseReference = point
		g.baseIndex = index
	}
	if role >= 2 && mapped == 1 {
		b.rawE278 = 1
		if count > 2 {
			b.rawE278 = 2
		}
	}
}

func (b *missionBuilder) recoveredObjectiveOwnsCell(role, index, fi, count int, original MissionPoint, cell [2]int) bool {
	if b.in.GenerationPolicy != CampaignGenerationPolicyPairedV1 {
		return false
	}
	r, ok := b.recoveredObjectives[role]
	g := b.g
	if !ok || r.Point != original || r.Formation != fi || r.Start != index || r.Tile&128 != 0 || index < 0 || count <= 0 || index+count != len(g.objects) || cell != missionCell(r.Point, 8192) {
		return false
	}
	for _, object := range g.objects[index:] {
		if object.Formation != fi {
			return false
		}
	}
	return g.tile(cell[0], cell[1]) == r.Tile|128 && g.source.LAND[r.Tile&63]&2 != 0
}

func missionCString(v []byte) string {
	for i, b := range v {
		if b == 0 {
			return string(v[:i])
		}
	}
	return string(v)
}

// Platform bases are staged at count without incrementing it; the following append may alias that physical row.
func (b *missionBuilder) baseTarget() {
	g := b.g
	before := len(g.objects)
	if b.flags&8 != 0 {
		fi := b.until("A1a1bd", 0x00800020, 0, func(_ int) bool {
			point := g.current
			if point.X == 0 {
				point.X = int32(g.rng.inclusive(2, 13, "A1a1eb/A1a22c") << 15)
			}
			if point.Y == 0 {
				point.Y = int32(g.rng.inclusive(2, 13, "A1a1eb/A1a22c") << 15)
			}
			g.current = point
			x, y := int(point.X/8192), int(point.Y/8192)
			for xx := x - 1; xx <= x+1; xx++ {
				for yy := y - 1; yy <= y+1; yy++ {
					if g.source.LAND[g.tile(xx, yy)&63]&0x40 != 0 {
						return false
					}
				}
			}
			return true
		})
		if fi < 0 {
			return
		}
		obj := MissionObject{}
		binary.LittleEndian.PutUint32(obj.Raw[2:], uint32(g.current.X))
		binary.LittleEndian.PutUint32(obj.Raw[6:], uint32(g.current.Y))
		g.staged[before] = obj
		b.record(0, before, fi, 0)
		binary.LittleEndian.PutUint16(g.objects[0].Raw[:], 0)
		binary.LittleEndian.PutUint32(g.objects[0].Raw[2:], uint32(g.current.X))
		binary.LittleEndian.PutUint32(g.objects[0].Raw[6:], uint32(g.current.Y))
		b.base = 0
		b.oldBase = before
	} else {
		fi := b.until("A1a32a", 0x20, 0x00800000, func(_ int) bool { return true })
		if fi < 0 {
			return
		}
		g.expand()
		b.record(0, before, fi, len(g.objects)-before)
		b.base = before
		b.oldBase = before
	}
	formation, err := g.objectFormation(b.base, "base scene selection")
	if err != nil {
		g.rng.err = err
		return
	}
	if formation.Flags&0x40000000 != 0 {
		if b.scene == 4 {
			b.scene = 5
		} else {
			b.scene = 2
		}
	}
}

func (b *missionBuilder) boundary() {
	p := missionCell(b.g.rawPoint(b.base), 32768)
	bx, by := p[0], p[1]
	ex, ey := bx, by
	if bx < 8 && missionAbs(by-8) > bx {
		ex = 0
	} else if bx >= 8 && missionAbs(by-8) > 16-bx {
		ex = 16
	} else if by < 8 {
		ey = 0
	} else {
		ey = 16
	}
	b.g.boundary = [2]int{ex, ey}
	b.g.boundaryMinimum = missionAbs(ex-bx) + missionAbs(ey-by)
}

func (b *missionBuilder) departure() {
	g := b.g
	if g.rng.r0(5, "A1a3fd") == 0 {
		b.mode = 2
		if g.rng.r0(100, "A1a412") >= 60 {
			b.mode = 1
		}
		if b.mode == 1 && (b.flags&8 != 0 || b.scene == 2 || b.scene == 5) {
			b.mode = 2
		}
	}
	if b.mode == 2 {
		b.special = len(g.objects)
		if b.until("A1a45e", 0x04000000, 0, func(_ int) bool { return true }) < 0 {
			return
		}
		g.expand()
		b.support = generatedSupportConfiguration(g, b.special, len(g.objects), 2, 0)
	}
	b.boundary()
	if b.mode == 2 {
		x, y := g.boundary[0], g.boundary[1]
		xx, yy := MissionI16(x*4+(8-x)/8), MissionI16(y*4+(8-y)/8)
		point := missionPoint(xx*8192, yy*8192)
		if x == 16 {
			point.X--
		}
		if y == 16 {
			point.Y--
		}
		binary.LittleEndian.PutUint32(g.objects[b.special].Raw[2:], uint32(point.X))
		binary.LittleEndian.PutUint32(g.objects[b.special].Raw[6:], uint32(point.Y))
	}
	target := len(g.objects) + 1
	if g.e282 == 1 {
		target = len(g.objects) + 9
	}
	if b.mode == 1 {
		b.special = len(g.objects)
		if b.until("A1a661", 0x02000000, 0, func(_ int) bool {
			return b.distance(b.base) >= 0 && b.distance(b.base) <= 1 && b.edge(nil) > g.boundaryMinimum
		}) < 0 {
			return
		}
		g.expand()
		raw := g.rng.inclusive(25, 27, "A1a6c0")
		b.support = generatedSupportConfiguration(g, b.special, len(g.objects), 1, byte(raw))
	}
	if g.mission == 1 || b.mode == 1 {
		for pass := 0; len(g.objects) < target; pass++ {
			if pass > 1000 {
				g.rng.err = fmt.Errorf("extra formation diagnostic limit")
				return
			}
			ok := false
			for trial := 0; trial < 100; trial++ {
				if g.pick(0x00400000, 0x06000000) < 0 {
					return
				}
				if b.distance(b.base) >= 0 && b.distance(b.base) <= 1 && b.edge(nil) > g.boundaryMinimum {
					g.expand()
					ok = true
					break
				}
			}
			if !ok {
				target = 0
			}
		}
	}
	for i := b.oldBase; i < len(g.objects); i++ {
		word := int16(binary.LittleEndian.Uint16(g.objects[i].Raw[27:]))
		if word > 0 {
			binary.LittleEndian.PutUint16(g.objects[i].Raw[27:], uint16(-word))
		}
	}
}

func (b *missionBuilder) objectiveAllows(role, fi int) bool {
	g := b.g
	if role >= 3 && fi == g.objects[b.refs[2]].Formation {
		return false
	}
	if role == 4 && fi == g.objects[b.refs[3]].Formation {
		return false
	}
	distance := b.distance(b.base)
	if role == 2 {
		return !(b.flags&8 != 0 && g.forms[fi].Flags&0x00200380 != 0) && distance >= 5 && distance <= b.upper && b.edge(nil) >= g.boundaryMinimum
	}
	if distance < 6 || distance > b.upper {
		return false
	}
	if role >= 3 && b.distance(b.refs[2]) < 4 {
		return false
	}
	if role == 4 && b.distance(b.refs[3]) < 2 {
		return false
	}
	return b.edge(nil) >= g.boundaryMinimum
}

// The original ignores A17995's failed spiral return. For the new typed policy
// only, give a limited/water objective one finite global search, retaining the
// same formation, terrain and complete role predicate. This does not repair a
// genuinely empty domain, move a cached object or add a random draw.
func (b *missionBuilder) recoverLimitedObjective(role, fi int) bool {
	g := b.g
	flags := g.forms[fi].Flags
	if !g.startFailed || g.reused >= 0 || flags&0x00200400 == 0 || flags&0x101000 != 0 || g.rng.err != nil {
		return b.objectiveAllows(role, fi)
	}
	original := g.current
	anchor := missionCell(original, 8192)
	bestDistance, candidates := int(^uint(0)>>1), 0
	best := original
	for y := 5; y < 59; y++ {
		for x := 5; x < 59; x++ {
			if !g.acceptable(x, y, flags) {
				continue
			}
			g.current = missionPoint(x*8192+4096, y*8192+4096)
			if !b.objectiveAllows(role, fi) {
				continue
			}
			candidates++
			distance := missionMetric([2]int{x, y}, anchor)
			// y then x iteration gives the declared deterministic tie break.
			if distance < bestDistance {
				bestDistance, best = distance, g.current
			}
		}
	}
	g.current = original
	if candidates == 0 || g.rng.err != nil {
		return false
	}
	g.current, g.startFailed = best, false
	if b.recoveredObjectives == nil {
		b.recoveredObjectives = make(map[int]missionRecoveredObjective)
	}
	cell := missionCell(best, 8192)
	b.recoveredObjectives[role] = missionRecoveredObjective{Point: best, Formation: fi, Start: len(g.objects), Tile: g.tile(cell[0], cell[1])}
	g.log("A17995", "limited_objective_global_fallback", fi, best, fmt.Sprintf("policy%s; %s; failed anchor(%d,%d); %d full-constraint candidates", b.in.GenerationPolicy, missionRole(role), original.X, original.Y, candidates))
	return true
}

func (b *missionBuilder) marked(role int) {
	kind := b.kinds[role]
	if kind == 0 {
		return
	}
	g := b.g
	forbidden := uint32(0)
	if role == 4 {
		forbidden = 0x08200380
	} else {
		if b.flags&8 != 0 || g.mission == 1 {
			forbidden |= 0x08000000
		}
		if kind == 1 {
			forbidden |= 0x1000
		}
	}
	fi := b.until("objective", b.tables.Masks[kind], forbidden, func(fi int) bool {
		if b.in.GenerationPolicy == CampaignGenerationPolicyPairedV1 {
			return b.recoverLimitedObjective(role, fi)
		}
		return b.objectiveAllows(role, fi)
	})
	if fi < 0 {
		return
	}
	before := len(g.objects)
	anchor, reused := g.current, g.reused
	g.expand()
	start := before
	if g.reused >= 0 {
		start = g.reused
	}
	end := len(g.objects)
	if g.reused >= 0 {
		end = start
		for end < len(g.objects) && g.objects[end].Formation == fi {
			end++
		}
	}
	mark := byte(0x10)
	if role == 3 {
		mark = 0x20
	} else if role == 4 {
		mark = 0x40
	}
	// GS A1A8E1/AA5B/ABBB constructs TARGET before the new-span mark
	// loop. 18E46 retains raw kind separately, but overwrites TARGET's kind
	// with DATA1B94's mapping. Its reached mapped2 (raw4/6), not raw2,
	// terminates the new-span loop. Reused spans precede that constructor.
	if g.reused >= 0 {
		markMissionObjectiveSpan(g.objects, start, end, mark, false)
		b.record(role, start, fi, end-start)
	} else {
		b.record(role, start, fi, end-start)
		markMissionObjectiveSpan(g.objects, start, end, mark, b.targets[role].DisplayKind == 2)
	}
	if b.in.PlacementPolicy == RequiredFARPPlacementV1 {
		b.capturePlacementRole(role, fi, start, end, anchor, reused)
	}
}

func markMissionObjectiveSpan(objects []MissionObject, start, end int, mark byte, firstOnly bool) {
	for i := start; i < end; i++ {
		objects[i].Raw[0] |= mark
		if firstOnly {
			break
		}
	}
}

func (b *missionBuilder) farp() {
	g := b.g
	include := b.flags&8 != 0 || b.scene == 2 || b.scene == 5
	if !include {
		include = g.rng.r0(10, "A1ac1b") < 3
	}
	if !include {
		return
	}
	b.kinds[1] = 1
	if b.in.PlacementPolicy == RequiredFARPPlacementV1 {
		if planned, err := b.planRequiredFARP(); err != nil {
			g.rng.err = err
			return
		} else if planned {
			return
		}
	}
	before := len(g.objects)
	fi := b.until("A1ac38", 0x40, 0, func(_ int) bool {
		bd, pd, sd, ed := b.distance(b.base), b.distance(b.refs[2]), b.distance(b.refs[3]), b.edge(nil)
		return bd > 5 && (pd <= 5 || sd <= 5) && pd >= 2 && sd >= 2 && ed >= g.boundaryMinimum
	})
	if fi < 0 {
		return
	}
	g.expand()
	for i := before; i < len(g.objects); i++ {
		g.objects[i].Raw[0] |= 0x80
		word := int16(binary.LittleEndian.Uint16(g.objects[i].Raw[27:]))
		if word > 0 {
			binary.LittleEndian.PutUint16(g.objects[i].Raw[27:], uint16(-word))
		}
	}
	b.record(1, before, fi, len(g.objects)-before)
}

func (b *missionBuilder) midpoint(a, c MissionPoint, third bool) {
	g := b.g
	if len(g.objects) >= 80 {
		return
	}
	g.current = MissionPoint{(a.X + c.X) / 2, (a.Y + c.Y) / 2}
	if b.distance(b.base) <= 3 {
		return
	}
	grid := missionCell(g.current, 32768)
	for n := 0; n < 100; n++ {
		if g.pick(0x10000, 0xc000) < 0 {
			return
		}
		if third && b.distance(b.base) < 2 {
			return
		}
		limit := 1
		if third {
			limit = 0
		}
		if b.edge(&grid) <= limit {
			g.expand()
			return
		}
	}
}

func (b *missionBuilder) extras(role, reference int) {
	g := b.g
	if len(g.pool) == 0 {
		g.initializePool()
	}
	center := [2]int{32, 32}
	radius, minimum := 64, 3
	var origin *[2]int
	if role != 0 {
		g.extraLocal = 0
		index := b.refs[role]
		formation, err := g.objectFormation(index, "extra formation placement")
		if err != nil {
			g.rng.err = err
			return
		}
		flags := formation.Flags
		point := g.point(index)
		v := missionCell(point, 8192)
		origin = &v
		context := point
		if g.rawPoint(index).X == 0 {
			context = MissionPoint{}
			if b.targets[2].Endpoint != nil {
				context = *b.targets[2].Endpoint
			}
		} else if b.kinds[role] != 1 && b.kinds[role] != 6 {
			context = g.rawPoint(reference)
		}
		center = missionCell(context, 8192)
		distance := missionMetric(*origin, center)
		if distance != 0 {
			for i := 0; i < 2; i++ {
				center[i] = MissionI16(origin[i] + MissionI16((center[i]-origin[i])*4)/distance)
			}
		}
		radius, minimum = 6, 1
		mapped := b.targets[role].DisplayKind
		if mapped == 1 || mapped == 2 {
			radius, minimum = 8, 3
		}
		if flags&0x00200400 != 0 {
			radius *= 2
			minimum *= 2
		}
	}
	for n := 0; n < 100; n++ {
		if g.randomGeneric(center, radius) < 0 {
			return
		}
		center = missionCell(g.current, 8192)
		if origin != nil && missionMetric(center, *origin) <= minimum {
			continue
		}
		if b.base != 0 {
			if g.extraLocal < 0 {
				g.rng.err = fmt.Errorf("missing explicit extra-local state")
				return
			}
			threshold := 12
			if g.extraLocal != 0 {
				threshold = 8
			}
			if missionMetric(center, missionCell(b.targets[0].OriginalPoint, 8192)) <= threshold {
				continue
			}
		}
		g.expand()
		return
	}
}

// These source-generated deny lists are Campaign-local restrictions, not a finite-stock logistics model.
func (b *missionBuilder) restrictions() {
	g := b.g
	loss := uint16(b.in.Campaign.LossWord)
	b.unavailableAircraft = []int{-1, -1, -1, -1}
	b.unavailableSystems = []int{-1, -1, -1, -1}
	contains := func(values []int, v int) bool {
		for _, x := range values {
			if x == v {
				return true
			}
		}
		return false
	}
	count := 3
	if int16(loss) > 1 {
		count = 4
	}
	for i := 0; i < count; i++ {
		for n := 0; n < 10000; n++ {
			v := g.rng.inclusive(0, 13, "campaign_system")
			if !contains(b.unavailableSystems[:i], v) {
				b.unavailableSystems[i] = v
				break
			}
			if g.rng.err != nil {
				return
			}
		}
	}
	a, c := g.rng.inclusive(0, 7, "campaign_aircraft0"), g.rng.inclusive(0, 6, "campaign_aircraft1")
	if c >= a {
		c++
	}
	b.unavailableAircraft[0], b.unavailableAircraft[1] = a, c
	if loss != 0 {
		for n := 0; n < 10000; n++ {
			v := g.rng.inclusive(0, 7, "campaign_aircraft2")
			if v != a && v != c {
				b.unavailableAircraft[2] = v
				break
			}
			if g.rng.err != nil {
				return
			}
		}
	}
}

// Keep orchestration order: placement consumes the same bounded draw stream as weather and defaults.
func (b *missionBuilder) run() error {
	g := b.g
	b.chooseKinds()
	if g.rng.err != nil {
		return g.rng.err
	}
	b.initial()
	if g.rng.err != nil {
		return g.rng.err
	}
	b.baseTarget()
	if g.rng.err != nil {
		return g.rng.err
	}
	b.departure()
	if g.rng.err != nil {
		return g.rng.err
	}
	if b.in.PlacementPolicy == RequiredFARPPlacementV1 {
		b.placementPrefix = append([]byte(nil), g.grid...)
		b.placementObjects = append([]MissionObject(nil), g.objects...)
	}
	for _, role := range []int{2, 3, 4} {
		b.marked(role)
		if g.rng.err != nil {
			return g.rng.err
		}
	}
	b.farp()
	if g.rng.err != nil {
		return g.rng.err
	}
	base := g.rawPoint(b.base)
	b.midpoint(base, g.point(b.refs[2]), false)
	b.midpoint(base, g.point(b.refs[3]), false)
	b.midpoint(g.point(b.refs[2]), g.point(b.refs[3]), true)
	g.initializePool()
	g.filling = true
	for pass := 0; len(g.objects) < 80; pass++ {
		if pass > 1000 {
			return fmt.Errorf("mission fill diagnostic retry limit")
		}
		for _, role := range []int{2, 3} {
			if len(g.objects) < 80 {
				b.extras(role, b.base)
				if g.rng.err != nil {
					return g.rng.err
				}
			}
		}
		extra := g.rng.r0(2, "A1b092")
		if extra != 0 && b.kinds[4] != 0 && len(g.objects) < 80 {
			b.extras(4, b.refs[3])
		}
		if len(g.objects) < 80 {
			b.extras(0, 0)
		}
		if g.rng.err != nil {
			return g.rng.err
		}
	}
	point := g.rawPoint(b.base)
	binary.LittleEndian.PutUint32(g.objects[0].Raw[2:], uint32(point.X))
	binary.LittleEndian.PutUint32(g.objects[0].Raw[6:], uint32(point.Y))
	return g.rng.err
}
