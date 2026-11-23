package state

import (
	"encoding/binary"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	"sync"
)

const AuthoredSelectionProfileP2 = "source-editor-selection-v1"
const AuthoredSelectionProfile = "source-editor-selection-v2"
const AuthoredSelectionProfileP5 = "source-editor-selection-v3"

// Source GS20f4:70d2 mutates the editor's loaded tables after NEW's built-ins
// have been copied. Keep that private interpretation out of immutable resources
// and historical placement expansion. Indices remain original FMHD/FRMD indices.
// Bounded immutable derivation cache. A session's prepared world pointer is
// identity, so independent bundles never alias; eviction only recomputes the
// same initializer and never changes an existing context or document.
var authoredPrivateWorlds = struct {
	sync.Mutex
	entries   map[*assetdata.MissionWorld]*assetdata.MissionWorld
	originals []*assetdata.MissionWorld
}{entries: make(map[*assetdata.MissionWorld]*assetdata.MissionWorld)}

func authoredEditorWorld(w *assetdata.MissionWorld) *assetdata.MissionWorld {
	authoredPrivateWorlds.Lock()
	defer authoredPrivateWorlds.Unlock()
	if private := authoredPrivateWorlds.entries[w]; private != nil {
		return private
	}
	private := buildAuthoredEditorWorld(w)
	if len(authoredPrivateWorlds.originals) >= 32 {
		old := authoredPrivateWorlds.originals[0]
		prior := authoredPrivateWorlds.entries[old]
		delete(authoredPrivateWorlds.entries, old)
		delete(authoredPrivateWorlds.entries, prior)
		authoredPrivateWorlds.originals = authoredPrivateWorlds.originals[1:]
	}
	authoredPrivateWorlds.entries[w] = private
	authoredPrivateWorlds.entries[private] = private
	authoredPrivateWorlds.originals = append(authoredPrivateWorlds.originals, w)
	return private
}

func buildAuthoredEditorWorld(w *assetdata.MissionWorld) *assetdata.MissionWorld {
	q := *w
	q.FRMD = append([]byte(nil), w.FRMD...)
	q.FMHD = append([]byte(nil), w.FMHD...)
	for ci := 0; ci < len(q.FRMD)/8; ci++ {
		model := q.FRMD[ci*8+1]
		switch model {
		case 0xdb, 0x16, 0x97, 0xe4, 0xb0, 0x42:
		default:
			continue
		}
		count := 1
		if q.FRMD[ci*8+2] == 1 {
			for at := ci + 1; at < len(q.FRMD)/8 && q.FRMD[at*8] == 255 && q.FRMD[at*8+2] == 1; at++ {
				count++
			}
		}
		if count == 1 {
			q.FRMD[ci*8+2] = 2
		}
	}
	for fi := 0; fi < len(q.FMHD)/39; fi++ {
		ci, e := authoredFirst(&q, fi)
		if e == nil && q.FRMD[ci*8+2] == 1 {
			f := q.FMHD[fi*39:]
			binary.LittleEndian.PutUint32(f[35:], binary.LittleEndian.Uint32(f[35:])|0x80000)
		}
	}
	return &q
}
func AuthoredEditorWorld(data *assetdata.MissionGenerationData, name string) (*assetdata.MissionWorld, error) {
	w, e := AuthoredWorld(data, name)
	if e != nil {
		return nil, e
	}
	return authoredEditorWorld(w), nil
}
func authoredNewSelection(d *AuthoredDocument) bool {
	for _, p := range d.Placements {
		if p.SelectionProfile == AuthoredSelectionProfile || authoredP5Selection(p.SelectionProfile) {
			return true
		}
	}
	return false
}

// Public panel counters read every consumed component (1609..164a), including
// protected built-ins/base/goals. References reuse their carrier and don't add
// a second physical formation.
func AuthoredDefenderCounts(data *assetdata.MissionGenerationData, d *AuthoredDocument) (out [10]int, err error) {
	w, e := AuthoredWorld(data, d.World)
	if e != nil {
		return out, e
	}
	refs := map[string]bool{}
	for _, p := range d.Placements {
		if p.Reference != "" {
			refs[p.Reference] = true
		}
	}
	for _, p := range d.Placements {
		if refs[p.ID] {
			continue
		}
		rows, e := authoredPlacementComponents(w, p)
		if e != nil {
			return out, e
		}
		for _, ci := range rows {
			model := int(w.FRMD[ci*8+1])
			if model*27+1 >= len(data.Tables.EditorObjectTypes) {
				return out, fmt.Errorf("component category unavailable")
			}
			cat := min(int(data.Tables.EditorObjectTypes[model*27+1]), 9)
			out[cat]++
		}
	}
	if d.Support != nil && d.Initial == nil {
		first := d.Support.Component
		for j := 0; j < AuthoredActiveSupportRows(d); j++ {
			ci := first + j
			if ci < 0 || ci >= len(w.FRMD)/8 {
				return out, fmt.Errorf("Support tally component unavailable")
			}
			model := int(w.FRMD[ci*8+1])
			if model*27+1 >= len(data.Tables.EditorObjectTypes) {
				return out, fmt.Errorf("Support tally category unavailable")
			}
			out[min(int(data.Tables.EditorObjectTypes[model*27+1]), 9)]++
		}
	}
	if d.SupportMemory != nil {
		for _, s := range d.SupportMemory.Reservations {
			for j := 0; j < authoredSupportCost(&s); j++ {
				ci := s.Component + j
				if ci < 0 || ci >= len(w.FRMD)/8 {
					return out, fmt.Errorf("retained Support tally component unavailable")
				}
				model := int(w.FRMD[ci*8+1])
				out[min(int(data.Tables.EditorObjectTypes[model*27+1]), 9)]++
			}
		}
	}
	return out, nil
}

func authoredBaseOptions(data *assetdata.MissionGenerationData, d *AuthoredDocument) ([]AuthoredOption, error) {
	w, e := AuthoredEditorWorld(data, d.World)
	if e != nil {
		return nil, e
	}
	out := []AuthoredOption{}
	add := func(fi int, name string) error {
		ci, e := authoredFirst(w, fi)
		if e != nil {
			return e
		}
		ti, templateErr := authoredTemplate(w, ci)
		// Deep Strike remains visible although its ingress constructor is not
		// admitted; absence of an OBJL model is not a reason to hide the menu.
		if templateErr != nil {
			if name != "Deep Strike" {
				return templateErr
			}
			ti = -1
		}
		rows, e := authoredComponents(w, fi, ci)
		if e != nil {
			return e
		}
		f := w.FMHD[fi*39:]
		flags := binary.LittleEndian.Uint32(f[35:])
		cost := len(rows)
		if flags&0x800000 != 0 {
			cost = 0
		}
		out = append(out, AuthoredOption{SelectionProfile: AuthoredSelectionProfile, Formation: fi, Component: ci, Template: ti, Cost: cost, Name: name, Fixed: flags&0x1000 != 0, X: int(int32(binary.LittleEndian.Uint32(f[27:])))/341 - 1, Y: int(int32(binary.LittleEndian.Uint32(f[31:])))/-455 + 1151})
		return nil
	}
	find := func(id int, mask uint32) int {
		for fi := 0; fi < len(w.FMHD)/39; fi++ {
			f := w.FMHD[fi*39:]
			if int(binary.LittleEndian.Uint16(f[25:])) == id && binary.LittleEndian.Uint32(f[35:])&mask == mask {
				return fi
			}
		}
		return -1
	}
	if !(w.Theatre == 0 && w.Suffix == 3) {
		for i, name := range []string{"Base", "Base #2"} {
			fi := find(i+1, 0x2000)
			if fi < 0 {
				return nil, fmt.Errorf("source base group%d is absent", i+1)
			}
			if e = add(fi, name); e != nil {
				return nil, e
			}
		}
	}
	fi := find(0, 0)
	if fi < 0 {
		return nil, fmt.Errorf("source Deep Strike descriptor absent")
	}
	if e = add(fi, "Deep Strike"); e != nil {
		return nil, e
	}
	for fi := 0; fi < len(w.FMHD)/39; fi++ {
		f := w.FMHD[fi*39:]
		if binary.LittleEndian.Uint16(f[25:]) == 0 && binary.LittleEndian.Uint32(f[35:])&0x800000 == 0 {
			if e = add(fi, "Sea Base"); e != nil {
				return nil, e
			}
		}
	}
	return out, nil
}

// New ordinary defender controls expose exactly categories1..10. Source Pickup
// models151/228 have raw category10 (UI11); their known goal/reserve/built-in
// owners do not authorize arbitrary ordinary actor aliases. Historical documents
// keep their recorded selection interpretation; no rows are silently discarded.
func authoredDefenderAdmission(data *assetdata.MissionGenerationData, w *assetdata.MissionWorld, p AuthoredPlacement) error {
	if (p.SelectionProfile != AuthoredSelectionProfile && !authoredP5Selection(p.SelectionProfile)) || p.Role != "defender" {
		return nil
	}
	if authoredP5Selection(p.SelectionProfile) {
		if why := authoredOptionUnavailable(w, p); why != "" {
			return fmt.Errorf("%s", why)
		}
	}
	rows, e := authoredPlacementComponents(w, p)
	if e != nil {
		return e
	}
	for _, ci := range rows {
		model := w.FRMD[ci*8+1]
		category := authoredCategory(data.Tables, model)
		if category < 1 || category > 10 {
			//The source display category does not identify a reserved aircraft.
			//Compound ground formations (including the actual headquarters
			//variants) contain an ordinary kind07/17 site member in category11.
			//20F4's construction copies that exact FRMD/OBJL row like its other
			//members; GS2's kind17 terrain/death owner is already admitted.
			//Keep independent pickup/rescue leaders under their own constructor.
			if authoredP5Selection(p.SelectionProfile) && category == 11 && ci != p.Component && w.FRMD[p.Component*8+2] == 1 && p.Formation >= 0 && p.Formation < len(w.FMHD)/39 && binary.LittleEndian.Uint32(w.FMHD[p.Formation*39+35:])&0x2000 != 0 {
				ti, e := authoredTemplate(w, ci)
				head, e2 := authoredTemplate(w, p.Component)
				if e == nil && e2 == nil && (w.OBJL[ti*32+10] == 7 || w.OBJL[ti*32+10] == 0x17) && w.OBJL[head*32+10] < 0x10 {
					continue
				}
			}
			// All also exposes the actual artillery formation. P5 binds its
			// OBJECT19 rows to the no-prefix controller owner; this is distinct
			// from the still-unadmitted pickup/reserve category aliases.
			ti, err := authoredTemplate(w, ci)
			if authoredP5Selection(p.SelectionProfile) && err == nil && w.OBJL[ti*32+10] == 0x19 && binary.LittleEndian.Uint32(w.FMHD[p.Formation*39+35:])&0x02000000 != 0 {
				continue
			}
			if p.Builtin && p.Formation >= 0 && p.Formation < len(w.FMHD)/39 && binary.LittleEndian.Uint32(w.FMHD[p.Formation*39+35:])&0x4000 != 0 {
				continue
			}
			return fmt.Errorf("ordinary defender source category%d (model%d, formation%d/component%d) is not admitted; pickup/reserve actor ownership is required", category, model, p.Formation, ci)
		}
	}
	return nil
}

func authoredOptionUnavailable(w *assetdata.MissionWorld, p AuthoredPlacement) string {
	if p.Formation < 0 || p.Formation >= len(w.FMHD)/39 {
		return "Source formation identity is unavailable."
	}
	flags := binary.LittleEndian.Uint32(w.FMHD[p.Formation*39+35:])
	if flags&0x200 != 0 {
		if _, err := authoredTrainPaths(w); err != nil {
			return err.Error()
		}
	}
	if flags&0x100000 != 0 && p.SelectionProfile != AuthoredSelectionProfileR23 {
		return "Pending AI implementation: this enemy-air ingress needs its source spawn and flight lifecycle."
	}
	rows, err := authoredPlacementComponents(w, p)
	if err != nil {
		return err.Error()
	}
	for _, ci := range rows {
		ti, e := authoredTemplate(w, ci)
		if e != nil {
			return e.Error()
		}
		switch w.OBJL[ti*32+10] {
		case 0x0a:
			if p.SelectionProfile != AuthoredSelectionProfileR23 {
				return "Pending AI implementation: this submarine needs its per-instance model lifecycle."
			}
			if w.OBJL[ti*32+12] != 219 {
				return "This submarine model lacks verified mutable-model provenance."
			}
		case 0x0b:
			return "Pending AI implementation: this enemy airborne object needs its source spawn lifecycle."
		}
	}
	return ""
}
