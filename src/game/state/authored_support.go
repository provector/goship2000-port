package state

import (
	"encoding/binary"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
)

const AuthoredCompilerSupport = "native-authored-support-seed-v1"
const AuthoredCompilerSupportP5 = "native-authored-support-seed-v2"
const AuthoredSupportProfile = "source-editor-support-v1"
const SupportInitializationProfile = "source-support-prefix-v1"

// These are sourceC510/C512/C514/C516 identities, not mutable flight actors.
// A nil descriptor is the strict historical schema1 None interpretation.
type AuthoredSupport struct {
	Profile   string `json:"profile"`
	Formation int    `json:"formation"`
	Component int    `json:"component"`
	Template  int    `json:"template"`
	Kind      byte   `json:"kind"`
}

// The complete saved realization freezes sourceA270/A26E/A26F and rows77..79.
// Prefix IDs are rawWSHD definitions, never OBJECT kinds.
type SupportConfiguration struct {
	Profile              string          `json:"profile"`
	Selection            AuthoredSupport `json:"selection"`
	Mode                 byte            `json:"mode"`
	PrefixRaw            [2]byte         `json:"prefix_raw"`
	Rows                 []int           `json:"rows"`
	RetainedRows         []MissionObject `json:"retained_rows,omitempty"`
	RetainedSourceDigest string          `json:"retained_source_digest,omitempty"`
}

// SourceDATA272D:AA2,AA4,AA5/AA9: one shared counter, not one per battery.
type SupportRuntime struct {
	Configuration SupportConfiguration `json:"configuration"`
	Request       uint16               `json:"request"`
	Availability  byte                 `json:"availability"`
	Target        InitializedPose      `json:"target"`
}

func (s *SupportRuntime) Command(target InitializedPose, cancel bool) error {
	return s.command(target, cancel, false)
}

// CommandP5 is selected only by the new launch/input admission; old films
// retain Command's existing unsupported Air command boundary.
func (s *SupportRuntime) CommandP5(target InitializedPose, cancel bool) error {
	return s.command(target, cancel, true)
}
func (s *SupportRuntime) command(target InitializedPose, cancel bool, allowAir bool) error {
	if s == nil || (s.Configuration.Profile != SupportInitializationProfile && !(allowAir && s.Configuration.HasRetainedPrefixPolicy())) {
		return fmt.Errorf("no admitted support command owner")
	}
	mode := s.Configuration.Mode
	if allowAir && s.Configuration.HasRetainedPrefixPolicy() {
		mode = supportMemorySourceMode(s.Configuration)
	}
	if mode != 1 && !(allowAir && mode == 2) {
		return fmt.Errorf("air Support commands are unavailable until the flight-fire-return-death controller is implemented")
	}
	if cancel {
		s.Request = 0
		return nil
	}
	if target.H0 > 0x7ffff || target.H1 > 0x7ffff {
		return fmt.Errorf("support target outside source world")
	}
	s.Target = target
	s.Request = 255
	return nil
}
func AuthoredSupportOptions(data *assetdata.MissionGenerationData, d *AuthoredDocument) ([]AuthoredOption, error) {
	w, err := AuthoredEditorWorld(data, d.World)
	if err != nil {
		return nil, err
	}
	out := []AuthoredOption{{Name: "None", Formation: -1, Component: -1, Template: -1}}
	for fi := 0; fi < len(w.FMHD)/39; fi++ {
		f := w.FMHD[fi*39:][:39]
		flags := binary.LittleEndian.Uint32(f[35:])
		if flags&0x06000000 == 0 {
			continue
		}
		first, e := authoredFirst(w, fi)
		if e != nil {
			return nil, e
		}
		for ci := first; ci < len(w.FRMD)/8 && (ci == first || w.FRMD[ci*8] == 255); ci++ {
			if flags&0x02000000 != 0 && ci != first {
				break
			}
			if flags&0x04000000 != 0 && w.FRMD[ci*8+2] != 2 {
				continue
			}
			ti, e := authoredTemplate(w, ci)
			if e != nil {
				return nil, e
			}
			rows, e := authoredComponents(w, fi, ci)
			if e != nil {
				return nil, e
			}
			name := missionCString(f[:25])
			if flags&0x04000000 != 0 {
				model := int(w.FRMD[ci*8+1])
				at := model*27 + 2
				if at+25 > len(data.Tables.EditorObjectTypes) {
					return nil, fmt.Errorf("support model caption unavailable")
				}
				name = missionCString(data.Tables.EditorObjectTypes[at : at+25])
			}
			out = append(out, AuthoredOption{SelectionProfile: AuthoredSupportProfile, Formation: fi, Component: ci, Template: ti, Cost: len(rows), Name: name})
		}
	}
	return out, nil
}
func authoredSupportValidate(data *assetdata.MissionGenerationData, d *AuthoredDocument) error {
	if d.Support == nil {
		return nil
	}
	if (d.Compiler != AuthoredCompilerSupport && d.Compiler != AuthoredCompilerSupportP5) || d.Support.Profile != AuthoredSupportProfile {
		return fmt.Errorf("active support requires recognized document/compiler profile")
	}
	options, err := AuthoredSupportOptions(data, d)
	if err != nil {
		return err
	}
	for _, o := range options[1:] {
		if o.Formation == d.Support.Formation && o.Component == d.Support.Component && o.Template == d.Support.Template {
			w, _ := AuthoredWorld(data, d.World)
			kind := w.OBJL[o.Template*32+10]
			if kind != d.Support.Kind || kind != 0x19 && kind != 0x11 {
				return fmt.Errorf("support template kind differs")
			}
			if kind == 0x19 && o.Cost != 3 || kind == 0x11 && o.Cost != 1 {
				return fmt.Errorf("support source span differs")
			}
			return nil
		}
	}
	return fmt.Errorf("support formation/component/template identity is unavailable")
}
func AuthoredSupportRows(d *AuthoredDocument) int {
	return AuthoredActiveSupportRows(d) + authoredLeakedSupportRows(d)
}

func setAuthoredSupportSource(data *assetdata.MissionGenerationData, d *AuthoredDocument, o AuthoredOption) error {
	if d.Initial != nil {
		return fmt.Errorf("retained film Support needs a verified source inverse constructor")
	}
	q := *d
	q.Compiled = nil
	if o.Component < 0 {
		if q.SupportMemory != nil {
			memory := *q.SupportMemory
			memory.ShowCaption = false
			q.SupportMemory = &memory
		}
		q.Support = nil
		*d = q
		return nil
	}
	w, err := AuthoredWorld(data, d.World)
	if err != nil {
		return err
	}
	if o.Template < 0 || o.Template >= len(w.OBJL)/32 {
		return fmt.Errorf("support template unavailable")
	}
	q.Compiler = AuthoredCompilerSupportP5
	q.Support = &AuthoredSupport{Profile: AuthoredSupportProfile, Formation: o.Formation, Component: o.Component, Template: o.Template, Kind: w.OBJL[o.Template*32+10]}
	if err = authoredSupportValidate(data, &q); err != nil {
		return err
	}
	if q.Support.Kind == 0x19 {
		bases, e := authoredBaseOptions(data, &q)
		if e != nil {
			return e
		}
		for _, p := range q.Placements {
			if p.Role == "base" {
				for mode, b := range bases {
					if b.Formation == p.Formation && mode > 1 {
						return fmt.Errorf("artillery Support is unavailable for this source base-entry mode")
					}
				}
			}
		}
	}

	if _, err = AuthoredBudget(data, &q); err != nil {
		return err
	}
	placements := append(append([]AuthoredPlacement(nil), q.Placements...), AuthoredPlacement{Formation: o.Formation, Component: o.Component, Template: o.Template})
	cost, err := authoredModelAdmission(data, &q, w, placements, nil)
	if err != nil {
		return err
	}
	if !d.NativeModelResources() && cost >= 0x9d42 {
		return fmt.Errorf("support exceeds source model memory")
	}
	*d = q
	return nil
}
func (r *missionRNG) authoredRaw(site string) int {
	if r.authoredCRT == nil {
		r.err = fmt.Errorf("support compilation CRT stream missing")
		return 0
	}
	*r.authoredCRT = *r.authoredCRT*214013 + 2531011
	v := int((*r.authoredCRT >> 16) & 32767)
	r.trace = append(r.trace, MissionDraw{Site: site, Bound: 32768, Value: v})
	return v
}

// Source1abf:1e9b..2124. The special span has been reserved before Fill.
// SourceCRT rand is0..32767;2fc8 is signed-long remainder by65536,
// so rand()%65536-32768 intentionally lies on the negative side of base.
func compileAuthoredSupport(data *assetdata.MissionGenerationData, d *AuthoredDocument, g *genericMissionPopulation, base MissionPoint) (*SupportConfiguration, error) {
	if d.Support == nil {
		return nil, nil
	}
	if err := authoredSupportValidate(data, d); err != nil {
		return nil, err
	}
	if d.Support.Kind == 0x19 && len(g.objects) != 77 {
		return nil, fmt.Errorf("artillery needs exactly77 ordinary/reserved rows before source77..79 (got%d)", len(g.objects))
	}
	w := g.source
	config := &SupportConfiguration{Profile: SupportInitializationProfile, Selection: *d.Support, Mode: 1, Rows: []int{77, 78, 79}}
	var bp AuthoredPlacement
	for _, p := range d.Placements {
		if p.Role == "base" {
			bp = p
			break
		}
	}
	// Signed editor coordinate coarse conversion is exactly1ea6..1edc.
	bx := int(int16(bp.X/24)) / 4
	by := int(int16(bp.Y/-18))/4 + 15
	ex, ey := bx, by
	if bx < 8 && bx < missionAbs(by-8) {
		ex = 0
	} else if bx >= 8 && missionAbs(by-8) > 16-bx {
		ex = 16
	} else if by < 8 {
		ey = 0
	} else {
		ey = 16
	}
	boundary := missionAbs(ey-by) + missionAbs(ex-bx)
	if d.Support.Kind == 0x11 {
		if len(g.objects) != 79 {
			return nil, fmt.Errorf("air Support needs exactly79 rows before reserved row79")
		}
		config.Mode = 2
		config.Rows = []int{79}
		config.PrefixRaw = [2]byte{23, 24}
		if w.FRMD[d.Support.Component*8+1] == 0 {
			config.PrefixRaw[0] = 22
		}
		var o MissionObject
		o.Formation = d.Support.Formation
		copy(o.Raw[:], w.OBJL[d.Support.Template*32:d.Support.Template*32+32])
		binary.LittleEndian.PutUint32(o.Raw[2:], uint32(max(1, min(62, ex*4))<<13))
		binary.LittleEndian.PutUint32(o.Raw[6:], uint32(max(1, min(62, ey*4))<<13))
		o.Raw[11] = w.FRMD[d.Support.Component*8+3]
		g.objects = append(g.objects, o)
		return config, nil
	}
	config.PrefixRaw[0] = byte(g.rng.inclusive(25, 27, "1abf:1f79/005c"))
	var origin MissionPoint
	found := false
	for attempt := 0; attempt < 10000; attempt++ {
		origin = missionPoint(int(int32(base.X)+int32(g.rng.authoredRaw("1abf:1f8d"))-32768), int(int32(base.Y)+int32(g.rng.authoredRaw("1abf:1fbb"))-32768))
		if origin.X < 0 || origin.Y < 0 || origin.X >= 0x80000 || origin.Y >= 0x80000 {
			continue
		}
		cell := missionCell(origin, 8192)
		if g.tile(cell[0], cell[1])&0x80 != 0 {
			continue
		}
		coarse := missionCell(origin, 32768)
		dist := missionMetric(coarse, missionCell(base, 32768))
		edgeArgs := [2]int{ex, ey}
		if d.Compiler == AuthoredCompilerSupportP5 {
			// 1ABF:2033/2037 pass candidate world LOW words, not the
			// nearest-edge locals. 065C:238A compares these signed words
			// against the current coarse coordinates; keep the odd source ABI.
			edgeArgs = [2]int{int(int16(origin.X)), int(int16(origin.Y))}
		}
		if dist < 0 || dist > 1 || missionMetric(coarse, edgeArgs) <= boundary {
			continue
		}
		found = true
		break
	}
	if !found {
		return nil, fmt.Errorf("artillery Support placement exhausted source candidates; draft retained")
	}
	//065c:1242 publishes selected formation/cell; offsets remain full signed
	// component words relative to the reached unquantized Support origin.
	for j := 0; j < 3; j++ {
		ci := d.Support.Component + j
		ti, err := authoredTemplate(w, ci)
		if err != nil {
			return nil, err
		}
		var o MissionObject
		o.Formation = d.Support.Formation
		copy(o.Raw[:], w.OBJL[ti*32:ti*32+32])
		if o.Raw[10] != 0x19 {
			return nil, fmt.Errorf("artillery row%d is not sourcekind19", 77+j)
		}
		c := w.FRMD[ci*8 : ci*8+8]
		binary.LittleEndian.PutUint32(o.Raw[2:], uint32(origin.X+int32(int16(binary.LittleEndian.Uint16(c[4:])))))
		binary.LittleEndian.PutUint32(o.Raw[6:], uint32(origin.Y+int32(int16(binary.LittleEndian.Uint16(c[6:])))))
		//2118 uses the FIRST selected component's weaponbyte for all three.
		o.Raw[11] = w.FRMD[d.Support.Component*8+3]
		g.objects = append(g.objects, o)
	}
	return config, g.rng.err
}
func authoredSupportName(data *assetdata.MissionGenerationData, d *AuthoredDocument) string {
	options, _ := AuthoredSupportOptions(data, d)
	for _, o := range options {
		if d.Support != nil && o.Formation == d.Support.Formation && o.Component == d.Support.Component {
			return o.Name
		}
	}
	return "Selected"
}

// Execution admission is separate from faithful author/save of a known option.
func AuthoredExecutionReady(data *assetdata.MissionGenerationData, d *AuthoredDocument) error {
	if d == nil {
		return fmt.Errorf("mission document unavailable")
	}
	if d.Support != nil && d.Support.Kind == 0x11 {
		return fmt.Errorf("%s air Support cannot BEGIN: sourceC8A3..D028/0616 launch-fire-return-death controller is not implemented", authoredSupportName(data, d))
	}
	return nil
}

// Generated support is placed by the separate source A1A45E/A1A661 owner;
// its already-built special span need not occupy editor's77..79 reservation.
func generatedSupportConfiguration(g *genericMissionPopulation, start, end, mode int, raw byte) *SupportConfiguration {
	if start < 1 || end > 80 || start >= end {
		g.rng.err = fmt.Errorf("generated Support span is outside active source rows")
		return nil
	}
	first := g.objects[start]
	fi := first.Formation
	if fi < 0 || fi >= len(g.source.FMHD)/39 {
		g.rng.err = fmt.Errorf("generated Support provenance unavailable")
		return nil
	}
	component, err := authoredFirst(g.source, fi)
	if err != nil {
		g.rng.err = err
		return nil
	}
	found := -1
	ti := -1
	for ci := component; ci < len(g.source.FRMD)/8 && (ci == component || g.source.FRMD[ci*8] == 255); ci++ {
		candidate, e := authoredTemplate(g.source, ci)
		if e == nil && g.source.OBJL[candidate*32+12] == first.Raw[12] {
			found, ti = ci, candidate
			break
		}
	}
	if found < 0 {
		g.rng.err = fmt.Errorf("generated Support actual template component unavailable")
		return nil
	}
	cfg := &SupportConfiguration{Profile: SupportInitializationProfile, Mode: byte(mode), Selection: AuthoredSupport{Profile: AuthoredSupportProfile, Formation: fi, Component: found, Template: ti, Kind: first.Raw[10]}}
	for row := start; row < end; row++ {
		cfg.Rows = append(cfg.Rows, row)
	}
	if mode == 1 {
		cfg.PrefixRaw = [2]byte{raw, 0}
	} else {
		cfg.PrefixRaw = [2]byte{23, 24}
		if first.Raw[12] == 0 {
			cfg.PrefixRaw[0] = 22
		}
	}
	return cfg
}
func ValidateSupportInitialization(m *MissionData) error {
	if m == nil || m.Support == nil {
		return nil
	}
	if m.Support.HasRetainedPrefixPolicy() {
		return retainedSupportInitialization(m)
	}
	if m.Support.Profile != SupportInitializationProfile {
		return fmt.Errorf("unsupported mission-event Support profile")
	}
	s := m.Support
	expectedRows := 3
	expectedKind := byte(0x19)
	prefixOK := s.PrefixRaw[0] >= 25 && s.PrefixRaw[0] <= 27 && s.PrefixRaw[1] == 0
	if s.Mode == 2 {
		expectedRows = 1
		expectedKind = 0x11
		prefixOK = (s.PrefixRaw[0] == 22 || s.PrefixRaw[0] == 23) && s.PrefixRaw[1] == 24
	}
	if (s.Mode != 1 && s.Mode != 2) || len(s.Rows) != expectedRows {
		return fmt.Errorf("Support source span/mode is inconsistent")
	}
	if !prefixOK || s.Selection.Profile != AuthoredSupportProfile || s.Selection.Kind != expectedKind {
		return fmt.Errorf("Support source weapon/selection interpretation differs")
	}
	w := m.SourceWorld
	if w == nil || s.Selection.Formation < 0 || s.Selection.Formation >= len(w.FMHD)/39 {
		return fmt.Errorf("Support source world/formation is unavailable")
	}
	flags := binary.LittleEndian.Uint32(w.FMHD[s.Selection.Formation*39+35:])
	required := uint32(0x02000000)
	if s.Mode == 2 {
		required = 0x04000000
	}
	if flags&required == 0 {
		return fmt.Errorf("Support formation has the wrong source role flags")
	}
	first, err := authoredTemplate(w, s.Selection.Component)
	if err != nil || first != s.Selection.Template {
		return fmt.Errorf("Support selected template/component differs")
	}
	if s.Mode == 2 {
		raw := byte(23)
		if w.FRMD[s.Selection.Component*8+1] == 0 {
			raw = 22
		}
		if s.PrefixRaw[0] != raw {
			return fmt.Errorf("air Support raw weapon prefix differs from selected source component")
		}
	}
	minimum := 1
	if m.Inputs.MissionType.Raw() > 1 && !m.SoloBuilder {
		minimum = 9
	}
	for j, row := range s.Rows {
		if j > 0 && row != s.Rows[j-1]+1 {
			return fmt.Errorf("Support row order differs from source component span")
		}
		ci := s.Selection.Component + j
		ti, e := authoredTemplate(w, ci)
		if e != nil || row < minimum || row >= len(m.Objects) || m.Objects[row].Formation != s.Selection.Formation || m.Objects[row].Raw[12] != w.OBJL[ti*32+12] {
			return fmt.Errorf("Support row%d source template/provenance differs", row)
		}
	}
	for _, row := range m.Support.Rows {
		if row < 1 || row >= len(m.Objects) || m.Objects[row].Raw[10] != expectedKind {
			return fmt.Errorf("Support source row%d template kind differs", row)
		}
	}
	return nil
}

func authoredModelCostWithSupport(t assetdata.MissionTables, w *assetdata.MissionWorld, placements []AuthoredPlacement, support *AuthoredSupport) (uint16, error) {
	if support == nil {
		return authoredModelCost(t, w, placements)
	}
	p := append(append([]AuthoredPlacement(nil), placements...), AuthoredPlacement{Formation: support.Formation, Component: support.Component, Template: support.Template})
	return authoredModelCost(t, w, p)
}

// Structural source-carrier reconstruction is also used by historical/static
// initialization checks; active execution is a separate accepted-launch owner.
func ValidateSupportLaunch(m *MissionData) error {
	if err := ValidateSupportInitialization(m); err != nil {
		return err
	}
	if m != nil && m.Support != nil && m.Support.HasRetainedPrefixPolicy() {
		return fmt.Errorf("retained Support memory requires the P5 launch interpretation")
	}
	if m != nil && m.Support != nil && m.Support.Mode == 2 {
		return fmt.Errorf("air Support launch is unavailable: sourceC8A3..D028/0616 flight-fire-return-death controller is not implemented")
	}
	return nil
}

// AuthoredExecutionReadyP5 is separate from historical admission. The fresh
// accepted launch chooses the P5 bounded mission-actor interpretation; an old
// unchanged saved realization need not be recompiled to invent new geometry.
func AuthoredExecutionReadyP5(data *assetdata.MissionGenerationData, d *AuthoredDocument) error {
	if err := ValidateAuthored(data, d); err != nil {
		return err
	}
	return nil
}
