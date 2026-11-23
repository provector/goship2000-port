package state

import (
	"fmt"
	assetdata "gunship2000-assets/game/assets"
)

const AuthoredSupportMemoryProfile = "source-editor-retained-support-memory-v1"
const AuthoredSupportProcessMemoryProfile = "source-editor-prior-compiled-process-memory-v1"
const SupportRetainedInitializationProfile = "source-support-retained-memory-v1"
const SupportRowsInitializationProfile = "source-support-actual-rows-v1"

// A cancelled chooser re-debits its old cost then clears C516. Later None does
// not credit that now-inactive reservation. Source compilation leaves the
// corresponding unwritten OBJECT holes intact. The exact prior document is a
// bounded provenance witness, never invented padding or an active selection.
type AuthoredSupportMemory struct {
	Profile       string            `json:"profile"`
	SourceProfile string            `json:"source_profile,omitempty"`
	Reservations  []AuthoredSupport `json:"reservations"`
	Prior         *AuthoredDocument `json:"prior"`
	ShowCaption   bool              `json:"show_caption,omitempty"`
}

func authoredSupportCost(s *AuthoredSupport) int {
	if s == nil {
		return 0
	}
	if s.Kind == 0x19 {
		return 3
	}
	if s.Kind == 0x11 {
		return 1
	}
	return 0
}
func AuthoredActiveSupportRows(d *AuthoredDocument) int {
	if d == nil {
		return 0
	}
	return authoredSupportCost(d.Support)
}
func authoredLeakedSupportRows(d *AuthoredDocument) int {
	n := 0
	if d != nil && d.SupportMemory != nil {
		for _, s := range d.SupportMemory.Reservations {
			n += authoredSupportCost(&s)
		}
	}
	return n
}
func (s SupportConfiguration) HasRetainedPrefixPolicy() bool {
	return s.Profile == SupportRetainedInitializationProfile || s.Profile == SupportRowsInitializationProfile
}
func (s SupportConfiguration) ControllerKind(actor int) byte {
	if s.HasRetainedPrefixPolicy() {
		for i, row := range s.Rows {
			if row == actor && i < len(s.RetainedRows) {
				k := s.RetainedRows[i].Raw[10]
				if k == 0x11 || k == 0x19 {
					return k
				}
				return 0
			}
		}
		return 0
	}
	if s.Mode == 1 {
		return 0x19
	}
	if s.Mode == 2 {
		return 0x11
	}
	return 0
}
func (s SupportConfiguration) LastControllerKind() byte {
	var kind byte
	last := -1
	for _, row := range s.Rows {
		if k := s.ControllerKind(row); k != 0 && row > last {
			kind = k
			last = row
		}
	}
	return kind
}
func CancelAuthoredSupport(data *assetdata.MissionGenerationData, d, prior *AuthoredDocument) error {
	if d == nil || d.Support == nil {
		return nil
	}
	q, err := CloneAuthored(d)
	if err != nil {
		return err
	}
	var snapshot *AuthoredDocument
	if prior != nil && prior.Compiled != nil {
		snapshot, err = CloneAuthored(prior)
		if err != nil {
			return err
		}
	}
	if q.SupportMemory == nil {
		q.SupportMemory = &AuthoredSupportMemory{Profile: AuthoredSupportMemoryProfile}
	}
	q.SupportMemory.Prior = snapshot
	if snapshot != nil && snapshot.ID != q.ID {
		q.SupportMemory.SourceProfile = AuthoredSupportProcessMemoryProfile
	} else {
		q.SupportMemory.SourceProfile = ""
	}
	q.SupportMemory.ShowCaption = true
	q.SupportMemory.Reservations = append(q.SupportMemory.Reservations, *q.Support)
	q.Support = nil
	q.Compiler = AuthoredCompilerSupportP5
	q.Compiled = nil
	if snapshot != nil {
		if err = validateAuthoredSupportMemory(data, q); err != nil {
			return err
		}
	}

	if _, err = AuthoredBudget(data, q); err != nil {
		return err
	}
	*d = *q
	return nil
}
func validateAuthoredSupportMemory(data *assetdata.MissionGenerationData, d *AuthoredDocument) error {
	m := d.SupportMemory
	if m == nil {
		return nil
	}
	if m.Profile != AuthoredSupportMemoryProfile || d.Compiler != AuthoredCompilerSupportP5 || d.Initial != nil || len(m.Reservations) == 0 || len(m.Reservations) > 71 {
		return fmt.Errorf("unsupported retained Support memory")
	}
	depth := 0
	for p := d; p != nil && p.SupportMemory != nil; p = p.SupportMemory.Prior {
		depth++
		if depth > 8 {
			return fmt.Errorf("retained Support provenance exceeds eight saved generations; prior file retained")
		}
	}
	prior := m.Prior
	sameDocument := prior != nil && prior.ID == d.ID && prior.Seed == d.Seed && m.SourceProfile == ""
	priorProcess := prior != nil && m.SourceProfile == AuthoredSupportProcessMemoryProfile
	if prior == nil || (!sameDocument && !priorProcess) || prior.World != d.World || prior.Dependency != d.Dependency || prior.Initial != nil || prior.Compiled == nil || prior.Compiled.Revision != AuthoredRevision(prior) {
		return fmt.Errorf("retained Support has no same-document complete predecessor")
	}
	for _, s := range m.Reservations {
		q := *d
		q.Support = &s
		if err := authoredSupportValidate(data, &q); err != nil {
			return fmt.Errorf("retained reservation: %w", err)
		}
	}
	p, err := CloneAuthored(prior)
	if err != nil {
		return err
	}
	w, err := AuthoredWorld(data, d.World)
	if err != nil {
		return err
	}
	rank := prior.Compiled.Rank
	if rank < 0 || rank >= len(saveRanks) {
		return fmt.Errorf("retained Support predecessor rank unavailable")
	}
	_, err = CompileAuthoredMission(data, p, MissionInputs{MissionType: MissionSimulated, Theatre: saveTheatres[w.Theatre], Rank: saveRanks[rank], Seed: int64(p.Seed), PilotName: "Retained source memory"})
	if err != nil {
		return fmt.Errorf("retained Support predecessor: %w", err)
	}
	return nil
}
func retainedSupportInitialization(m *MissionData) error {
	s := m.Support
	if s == nil || !s.HasRetainedPrefixPolicy() {
		return fmt.Errorf("retained Support interpretation missing")
	}
	actualRows := s.Profile == SupportRowsInitializationProfile
	if len(s.Rows) == 0 || len(s.Rows) > 71 || len(s.Rows) != len(s.RetainedRows) || len(s.RetainedSourceDigest) != 64 || !actualRows && s.Rows[len(s.Rows)-1] != 79 {
		return fmt.Errorf("retained Support suffix provenance incomplete")
	}
	if actualRows && s.RetainedSourceDigest != actualSupportRowsDigest(m.SourceWorld, s.Rows, s.RetainedRows) {
		return fmt.Errorf("actual Support row/source identity differs")
	}
	switch s.Mode {
	case 0:
		if s.PrefixRaw != [2]byte{} {
			return fmt.Errorf("inactive Support cannot insert a weapon prefix")
		}
	case 1:
		if s.PrefixRaw[0] < 25 || s.PrefixRaw[0] > 27 || s.PrefixRaw[1] != 0 {
			return fmt.Errorf("retained artillery prefix invalid")
		}
	case 2:
		if (s.PrefixRaw[0] != 22 && s.PrefixRaw[0] != 23) || s.PrefixRaw[1] != 24 {
			return fmt.Errorf("retained air prefix invalid")
		}
	default:
		return fmt.Errorf("retained Support raw mode invalid")
	}
	if m.SourceWorld == nil || len(m.Objects) != 80 {
		return fmt.Errorf("retained Support world/OBJECT incomplete")
	}
	w := m.SourceWorld
	for i, row := range s.Rows {
		if row < 9 || row >= 80 || i > 0 && (row <= s.Rows[i-1] || !actualRows && row != s.Rows[i-1]+1) || s.RetainedRows[i] != m.Objects[row] {
			return fmt.Errorf("retained Support row%d differs from frozen source memory", row)
		}
		o := m.Objects[row]
		if o.Raw[0]&3 != 0 || o.Formation < 0 || o.Formation >= len(w.FMHD)/39 || o.Point().X < 0 || o.Point().X > 0x7ffff || o.Point().Y < 0 || o.Point().Y > 0x7ffff {
			return fmt.Errorf("retained Support row%d requires an unproven relative/controller memory alias", row)
		}
		known := false
		for at := 0; at+32 <= len(w.OBJL); at += 32 {
			r := w.OBJL[at : at+32]
			same := true
			for j := 0; j < 32; j++ {
				if j >= 2 && j < 10 || j == 11 {
					continue
				}
				if j == 0 {
					same = same && o.Raw[j]&^byte(0x30) == r[j]&^byte(0x30)
				} else if j == 10 && o.Raw[0]&0x30 != 0 && o.Raw[j] == 7 && r[j] >= 0x10 { // source1CE6 leading goal conversion
					same = same && true
				} else {
					same = same && o.Raw[j] == r[j]
				}
			}
			if same {
				known = true
				break
			}
		}
		if !known {
			return fmt.Errorf("retained Support row%d immutable template operands differ", row)
		}
	}
	if actualRows {
		for row := 9; row < 80; row++ {
			k := m.Objects[row].Raw[10]
			if k != 0x11 && k != 0x19 {
				continue
			}
			if s.ControllerKind(row) != k {
				return fmt.Errorf("actual Support row%d omitted from controller ownership", row)
			}
		}
	}
	if s.Mode != 0 {
		active := *s
		active.Profile = SupportInitializationProfile
		active.RetainedRows = nil
		active.RetainedSourceDigest = ""
		active.Rows = []int{79}
		if s.Mode == 1 {
			active.Rows = []int{77, 78, 79}
		}
		q := *m
		q.Support = &active
		if err := ValidateSupportInitialization(&q); err != nil {
			return err
		}
	}
	return nil
}

// Source GS2 initializes AA4 by scanning actual OBJECT kinds independently of
// A270. Ordinary editor formations can contain those same kinds without the
// special chooser's prefix. Freeze the complete actual row set under P5 rather
// than leaving such placed batteries inert or inventing a prefix.
func authoredActualSupportConfiguration(d *AuthoredDocument, g *genericMissionPopulation, s *SupportConfiguration) *SupportConfiguration {
	if d.Compiler != AuthoredCompilerP5 && d.Compiler != AuthoredCompilerSupportP5 {
		return s
	}
	var rows []int
	var objects []MissionObject
	selected := map[int]bool{}
	if s != nil {
		for _, row := range s.Rows {
			selected[row] = true
		}
	}
	extra := false
	for row := 9; row < len(g.objects); row++ {
		k := g.objects[row].Raw[10]
		if k == 0x19 || k == 0x11 {
			if !selected[row] {
				extra = true
			}
			selected[row] = true
		}
		if selected[row] {
			rows = append(rows, row)
			objects = append(objects, g.objects[row])
		}
	}
	if len(rows) == 0 || s != nil && !extra {
		return s
	}
	q := SupportConfiguration{Profile: SupportRowsInitializationProfile}
	if s != nil {
		q.Mode, q.PrefixRaw, q.Selection = s.Mode, s.PrefixRaw, s.Selection
	}
	q.Rows, q.RetainedRows = rows, objects
	q.RetainedSourceDigest = actualSupportRowsDigest(g.source, rows, objects)
	return &q
}

func actualSupportRowsDigest(w *assetdata.MissionWorld, rows []int, objects []MissionObject) string {
	if w == nil {
		return ""
	}
	return authoredDigest(struct {
		Profile string
		Source  string
		Rows    []int
		Objects []MissionObject
	}{SupportRowsInitializationProfile, authoredDependency(w), rows, objects})
}
func supportMemoryConfiguration(d *AuthoredDocument, g *genericMissionPopulation, from int, active *SupportConfiguration) *SupportConfiguration {
	if d.SupportMemory == nil {
		return active
	}
	s := &SupportConfiguration{Profile: SupportRetainedInitializationProfile, Mode: 0, RetainedSourceDigest: authoredDigest(d.SupportMemory.Prior.Compiled)}
	if active != nil {
		s.Mode, s.PrefixRaw, s.Selection = active.Mode, active.PrefixRaw, active.Selection
	} else {
		s.Selection = d.SupportMemory.Reservations[len(d.SupportMemory.Reservations)-1]
	}
	for i := from; i < 80; i++ {
		s.Rows = append(s.Rows, i)
		s.RetainedRows = append(s.RetainedRows, g.objects[i])
	}
	return s
}
func appendRetainedSupportHoles(d *AuthoredDocument, g *genericMissionPopulation) (int, error) {
	from := len(g.objects)
	if d.SupportMemory == nil {
		return from, nil
	}
	end := 80 - AuthoredActiveSupportRows(d)
	if from < 9 || from > end || end > 80 || len(d.SupportMemory.Prior.Compiled.Objects) != 80 {
		return from, fmt.Errorf("retained Support reservation/OBJECT capacity differs")
	}
	g.objects = append(g.objects, d.SupportMemory.Prior.Compiled.Objects[from:end]...)
	return from, nil
}
func supportMemorySourceMode(s SupportConfiguration) byte {
	if s.LastControllerKind() == 0x19 {
		return 1
	}
	if s.LastControllerKind() == 0x11 {
		return 2
	}
	return 0
}

func AuthoredSupportCaption(d *AuthoredDocument) *AuthoredSupport {
	if d == nil {
		return nil
	}
	if d.Support != nil {
		return d.Support
	}
	if d.SupportMemory != nil && d.SupportMemory.ShowCaption && len(d.SupportMemory.Reservations) > 0 {
		return &d.SupportMemory.Reservations[len(d.SupportMemory.Reservations)-1]
	}
	return nil
}
