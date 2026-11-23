package assets

import (
	"encoding/binary"
	"fmt"
	"image/color"
	"sort"
	"strings"
)

const editorMetadataSchema = 1
const phil3EditorMemberSHA = "5cb1cd18f675881202373e7ebacd484d1354166216be373e1b74df748fc83aa3"

type EditorRect struct {
	Group                      string
	Index, X, Y, Width, Height int
	State                      byte
}

// Editor AREA endpoints are inclusive, unlike the VCR's strict interior.
func (r EditorRect) Contains(x, y int) bool {
	return r.X <= x && x <= r.X+r.Width && r.Y <= y && y <= r.Y+r.Height
}

type EditorBinding struct {
	Scan            int
	Segment, Offset uint16
}
type EditorPreview struct {
	Pixels              []byte
	Width, Height, X, Y int
}
type EditorMap struct {
	Pixels      []byte
	Palette     color.Palette
	Unavailable string
}
type EditorAssets struct {
	WindVectors [][2]int

	ModelInformation, ModelNames map[int]string
	ModelInformationRaw          map[int][]byte
	WeaponTypes                  map[string][]byte
	FormationTypes               map[string][]byte
	FillLabels, DeleteLabels     []string
	ExitRects                    map[string][4]int
	GoalLabels                   []string
	Previews                     map[string]EditorPreview
	Browser                      MenuScreen
	BrowserLabels                map[int]string
	Background, Panel, Icons     []byte
	Maps                         map[string]EditorMap
	Rectangles                   []EditorRect
	Keyboard                     []EditorBinding
	IconRects                    [][4]int
	IconAnchors                  [][2]int
	Text                         map[string]string
	Font                         *BitmapFont
	Fonts                        map[int]*BitmapFont
	PointerAtlas                 []byte
}
type editorPreparedPreview struct {
	Name                string
	Data                Output
	Width, Height, X, Y int
}
type editorPreparedMap struct {
	Name            string
	Pixels, Palette Output
	Unavailable     string
}
type editorMetadata struct {
	WindVectors [][2]int `json:",omitempty"`

	ModelInformation    map[int]string    `json:",omitempty"`
	ModelInformationRaw map[int][]byte    `json:",omitempty"`
	WeaponTypes         map[string][]byte `json:",omitempty"`
	FormationTypes      map[string][]byte `json:",omitempty"`
	ModelNames          map[int]string    `json:",omitempty"`
	ExtendedSchema      int               `json:",omitempty"`
	FillLabels          []string          `json:",omitempty"`
	DeleteLabels        []string          `json:",omitempty"`
	ExitRects           map[string][4]int
	GoalLabels          []string
	Previews            []editorPreparedPreview
	Schema              int
	Profile             string
	Browser             MenuScreen
	BrowserLabels       map[int]string
	Images              map[string]Output
	Maps                []editorPreparedMap
	Rectangles          []EditorRect
	Keyboard            []EditorBinding
	IconRects           [][4]int
	IconAnchors         [][2]int
	Text                map[string]string
	Sources             []SourceLocation
	Notes               []string
}

var editorStrings = map[string]int{
	"abort_title": 0x2fd2, "abort1": 0x2fe1, "abort2": 0x3007, "abort3": 0x3023, "abort4": 0x304d,
	"browser_title": 0x244b, "begin": 0x246c, "edit": 0x2472, "new": 0x2477,
	"save": 0x27aa, "abort": 0x27af, "editor": 0x27b5, "exit_title": 0x27bc,
	"too_many": 0x27d1, "base_missing": 0x27f4, "primary_missing": 0x2817,
	"secondary_missing": 0x283b, "farp_missing": 0x2861, "valid": 0x2883,
	"invalid": 0x28b5, "place": 0x290e, "base": 0x2936, "primary": 0x293e,
	"secondary": 0x2948, "deep_strike": 0x2957, "map_title": 0x2e90,
}

// Preparation31 adds the source Delete dialog. Historical metadata remains
// readable; current UI readiness checks these additional strings separately.
var editorP7Strings = map[string]int{
	"delete_title": 0x3077, "delete1": 0x3088, "delete2": 0x30a3,
}

// Additional original chooser/detail text is a Preparation27 capability.
var editorExtendedStrings = map[string]int{
	"goal_coordinate_format": 0x271e, "goal_detail_format": 0x2728,
	"farp_location_format": 0x2d59, "base_location_format": 0x2d72, "deep_location_format": 0x2d69,
	"formation_guidance": 0x2d7e, "single_guidance": 0x2d9b, "limited_prefix": 0x2dac,
	"special_formation": 0x2db6, "count_name_format": 0x2db9, "special_label_format": 0x2dc1,
	"scale_format": 0x2dc4, "component_format": 0x2dcd, "weapon_format": 0x2dd7,
	"secondary_weapon_format": 0x2ddc, "guarded_formation_info": 0x2df4,
}

func editorSourceData(gs []byte, ds, n int) ([]byte, error) {
	at := officeDataAnchor + ds
	if ds < 0 || n < 0 || at+n > len(gs) || at+n > IslandIce469085.OfficeLayout.DataFileEnd {
		return nil, fmt.Errorf("editor source DS%04x+%d outside initialized profile", ds, n)
	}
	return gs[at : at+n], nil
}
func prepareEditorMetadata(m *Manifest, p *AssetVersionProfile) (editorMetadata, *Resource, error) {
	var d editorMetadata
	installation, e := ResolveSourceInstallation(m)
	if e != nil {
		return d, nil, fmt.Errorf("editor source installation: %w", e)
	}
	gs, e := installation.Loose("GS.GS2")
	if e != nil {
		return d, nil, e
	}
	replayScreen, e := installation.Archive("GS2000.DAT", "REPLAY.SCN")
	if e != nil {
		return d, nil, e
	}
	if gs == nil || !p.SourceMatches(RoleMainExecutable, Hash(gs.Payload)) {
		return d, nil, fmt.Errorf("editor source identity")
	}
	browser, labels, e := parseReplayScreen(replayScreen)
	if e != nil {
		return d, nil, e
	}
	// R0198..020d splits EXPORT's source AREA into EDIT and NEW in mode1.
	for i := range browser.Areas {
		if browser.Areas[i].Key == 19 {
			a := browser.Areas[i]
			half := a.Rectangle[2]/2 - 1
			browser.Areas[i].Rectangle[2] = half
			browser.Areas[i].InclusiveEnd = [2]int{a.Rectangle[0] + half - 1, a.Rectangle[1] + a.Rectangle[3] - 1}
			a.Key = 21
			a.Rectangle[0] += half + 1
			a.Rectangle[2] = half
			a.InclusiveEnd = [2]int{a.Rectangle[0] + half - 1, a.Rectangle[1] + a.Rectangle[3] - 1}
			browser.Areas = append(browser.Areas, a)
			break
		}
	}
	d = editorMetadata{Schema: editorMetadataSchema, Profile: p.Name, Browser: browser, BrowserLabels: labels, Images: map[string]Output{}, Text: map[string]string{}, Sources: []SourceLocation{gs.Source, replayScreen.Source}, Notes: []string{
		"GS source DS/file anchor is static; metadata never opens executables at runtime.",
		"Editor images use GS EDITOR then MBUILDER panel/icons; selected MIS palette owns editor colors.",
		"GS front-end chooser cues21h/22h are not GS2 effects. Optional idle cues4a/25/27/06/26 remain separately admitted by their source caller.",
		"Original conditional idle feedback shares CRT history; native compilation seed is explicitly independent of host-timed UI feedback.",
	}}
	for name, ds := range editorStrings {
		s, e := officeSourceString(gs.Payload, ds)
		if e != nil {
			return d, nil, e
		}
		d.Text[name] = s.Text
	}
	for name, ds := range editorP7Strings {
		s, e := officeSourceString(gs.Payload, ds)
		if e != nil {
			return d, nil, e
		}
		d.Text[name] = s.Text
	}
	// Section4 source2896 text and its far-pointer table read by1da4:1538.
	for i, want := range []uint16{0, 19, 37, 54, 69, 82, 98, 104} {
		at := 0x3a8d0 + 0x8f4 + 4*i
		if at+4 > len(gs.Payload) || binary.LittleEndian.Uint16(gs.Payload[at:]) != want || binary.LittleEndian.Uint16(gs.Payload[at+2:]) != 0x2896 {
			return d, nil, fmt.Errorf("editor goal pointer%d", i)
		}
		start := 0x3a8d0 + int(want)
		end := start
		for end < len(gs.Payload) && gs.Payload[end] != 0 && end-start < 100 {
			end++
		}
		if end == len(gs.Payload) || end-start == 100 {
			return d, nil, fmt.Errorf("editor goal text%d", i)
		}
		d.GoalLabels = append(d.GoalLabels, string(gs.Payload[start:end]))
	}

	// Verified GS section4 text consumers for the normal panel and component pane.
	// Keep this separate from mainDS/editorSourceString's different bank.
	for key, off := range map[string]int{"initial_status": 0x71a, "object_number": 0x2a9, "weapons_prefix": 0x2b2, "not_located": 0x232, "not_designated": 0x23e, "day": 0x24d, "poor": 0x251, "excellent": 0x256, "located_tail": 0x27f} {
		start := 0x3a8d0 + off
		end := start
		for end < len(gs.Payload) && end-start < 100 && gs.Payload[end] != 0 {
			end++
		}
		if end >= len(gs.Payload) || end-start >= 100 {
			return d, nil, fmt.Errorf("editor panel text %s truncated", key)
		}
		d.Text[key] = string(gs.Payload[start:end])
	}

	vectors, e := editorSourceData(gs.Payload, 0x2c06, 32)
	if e != nil {
		return d, nil, e
	}
	for i := 0; i < 16; i++ {
		d.WindVectors = append(d.WindVectors, [2]int{int(int8(vectors[i*2])), int(int8(vectors[i*2+1]))})
	}

	// Source2896 pointer table, not translated menu guesses. Original counts
	// are6 Fill choices and19 Delete choices; runtime needs no original files.
	d.ExtendedSchema = 1
	db, e := installation.Loose("OBJDBASE.GS2")
	if e != nil {
		return d, nil, e
	}
	d.Sources = append(d.Sources, db.Source)
	if len(db.Payload) < 230*8 {
		return d, nil, fmt.Errorf("editor object information index truncated")
	}
	d.ModelInformation = map[int]string{}
	d.ModelInformationRaw = map[int][]byte{}
	d.WeaponTypes = map[string][]byte{}
	d.FormationTypes = map[string][]byte{}
	for _, prefix := range []string{"GULF", "EURO", "PHIL", "ANTC"} {
		//20f4:4c3c..4c64 loads the selected theatre's FORM.DAT. The
		//26-byte record's name+1 is not FMHD's per-site name or OBJTYPES.
		f, e := installation.Loose(prefix + "FORM.DAT")
		if e != nil {
			return d, nil, e
		}
		if len(f.Payload) == 0 || len(f.Payload)%26 != 0 {
			return d, nil, fmt.Errorf("editor formation table %s length%d", prefix, len(f.Payload))
		}
		d.FormationTypes[prefix] = append([]byte(nil), f.Payload...)
		d.Sources = append(d.Sources, f.Source)
		w, e := installation.Loose(prefix + "WEAP.DAT")
		if e != nil {
			return d, nil, e
		}
		if len(w.Payload) != 16*27 {
			return d, nil, fmt.Errorf("editor weapon table %s length%d", prefix, len(w.Payload))
		}
		d.WeaponTypes[prefix] = append([]byte(nil), w.Payload...)
		d.Sources = append(d.Sources, w.Source)
	}
	d.ModelNames = map[int]string{}
	for model := 0; model < 230; model++ {
		for word, dst := range []map[int]string{d.ModelNames, d.ModelInformation} {
			at := int(binary.LittleEndian.Uint16(db.Payload[model*8+word*2:]))
			limit := 50
			if word == 1 {
				limit = 200
			}
			if at < 0 || at >= len(db.Payload) {
				return d, nil, fmt.Errorf("editor information offset model%d", model)
			}
			end := at
			for end < len(db.Payload) && end-at < limit && db.Payload[end] != 0 {
				end++
			}
			// Original bounded fread allows a shorter final span; retain actual bytes.
			if word == 1 {
				d.ModelInformationRaw[model] = append([]byte(nil), db.Payload[at:end]...)
			}
			// Original model124's description points into its binary index. Retain
			// the actual bytes, but never manufacture readable help from that anomaly.
			if at < 230*8 {
				continue
			}
			dst[model] = string(db.Payload[at:end])
		}
	}

	for key, ds := range editorExtendedStrings {
		v, e := officeSourceString(gs.Payload, ds)
		if e != nil {
			return d, nil, e
		}
		d.Text[key] = v.Text
	}
	for _, set := range []struct {
		base    int
		offsets []uint16
		dst     *[]string
	}{
		{242228, []uint16{0x1da, 0x1cf, 0x1e9, 0x1f7, 0x204, 0x214}, &d.FillLabels},
		{242220, []uint16{0x1b7, 0x1a8, 0x194, 0x17f, 0x16b, 0x159, 0x14a, 0x137, 0x121, 0x10d, 0xfc, 0xed, 0xd4, 0xbd, 0xb6, 0x9d, 0x90, 0x85, 0x77}, &d.DeleteLabels},
	} {
		for i, off := range set.offsets {
			at := set.base + 4*i
			if set.base == 242220 {
				at = set.base - 4*i
			}
			// Fill's first two entries are reversed in the physical pointer table.
			if set.base == 242228 && i >= 2 {
				at = set.base + 4*(i-1)
			}
			if set.base == 242228 && i == 1 {
				at = 242224
			}
			if at+4 > len(gs.Payload) || binary.LittleEndian.Uint16(gs.Payload[at:]) != off || binary.LittleEndian.Uint16(gs.Payload[at+2:]) != 0x2896 {
				return d, nil, fmt.Errorf("editor extended menu pointer%d", i)
			}
			start := 0x3a8d0 + int(off)
			end := start
			for end < len(gs.Payload) && end-start < 100 && gs.Payload[end] != 0 {
				end++
			}
			if end >= len(gs.Payload) || end-start >= 100 {
				return d, nil, fmt.Errorf("editor extended text%d", i)
			}
			*set.dst = append(*set.dst, string(gs.Payload[start:end]))
		}
	}
	d.BrowserLabels[15] = d.Text["begin"]
	d.BrowserLabels[19] = d.Text["edit"]
	d.BrowserLabels[21] = d.Text["new"]
	for _, set := range []struct {
		name      string
		ds, count int
	}{{"category_and_view", 0x2976, 21}, {"parameter", 0x2a34, 15}} {
		raw, e := editorSourceData(gs.Payload, set.ds, set.count*9)
		if e != nil {
			return d, nil, e
		}
		for i := 0; i < set.count; i++ {
			b := raw[i*9:]
			d.Rectangles = append(d.Rectangles, EditorRect{set.name, i, OfficeSignedWord(b), OfficeSignedWord(b[2:]), OfficeSignedWord(b[4:]), OfficeSignedWord(b[6:]), b[8]})
		}
	}
	// Exit controls use the other source ABI: state byte then four words.
	// Valid SAVE/EDITOR geometry adjustments belong to validator2696.
	exit, e := editorSourceData(gs.Payload, 0x2abb, 27)
	if e != nil {
		return d, nil, e
	}
	var er [3][4]int
	for i := range er {
		b := exit[9*i+1:]
		er[i] = [4]int{OfficeSignedWord(b), OfficeSignedWord(b[2:]), OfficeSignedWord(b[4:]), OfficeSignedWord(b[6:])}
	}
	save, edit := er[0], er[1]
	save[0] -= 6
	save[2] -= 12
	edit[0] += 18
	edit[2] -= 12
	d.ExitRects = map[string][4]int{"valid_save": save, "valid_editor": edit, "valid_abort": er[2], "invalid_abort": er[0], "invalid_editor": er[1], "abort_no": er[0], "abort_yes": er[1]}

	raw, e := editorSourceData(gs.Payload, 0x25c4, 70*4)
	if e != nil {
		return d, nil, e
	}
	for i := 0; i < 70; i++ {
		off, seg := binary.LittleEndian.Uint16(raw[4*i:]), binary.LittleEndian.Uint16(raw[4*i+2:])
		at := 0x2000 + int(seg)*16 + int(off)
		if at < 0 || at+10 > len(gs.Payload) || gs.Payload[at] != 0xe8 || gs.Payload[at+3] != 0xea {
			return d, nil, fmt.Errorf("editor key thunk%d", i)
		}
		d.Keyboard = append(d.Keyboard, EditorBinding{i, binary.LittleEndian.Uint16(gs.Payload[at+6:]), binary.LittleEndian.Uint16(gs.Payload[at+4:])})
	}
	raw, e = editorSourceData(gs.Payload, 0x2ad8, 14*12)
	if e != nil {
		return d, nil, e
	}
	for i := 0; i < 14; i++ {
		b := raw[12*i:]
		d.IconRects = append(d.IconRects, [4]int{OfficeSignedWord(b), OfficeSignedWord(b[2:]), OfficeSignedWord(b[4:]), OfficeSignedWord(b[6:])})
		d.IconAnchors = append(d.IconAnchors, [2]int{OfficeSignedWord(b[8:]), OfficeSignedWord(b[10:])})
	}
	for _, image := range [][2]string{{"background", "GS2000.CAT/EDITOR.PIC"}, {"panel", "MBUILDER.CAT/MBEDITOR.PIC"}, {"icons", "MBUILDER.CAT/MBICONS.PIC"}} {
		key, path := image[0], image[1]
		parts := strings.SplitN(path, "/", 2)
		r, e := installation.Archive(parts[0], parts[1])
		if e != nil {
			return d, nil, e
		}
		o, e := preparedOutput(r.Outputs, "decoded_indices")
		if e != nil {
			return d, nil, e
		}
		d.Images[key] = o
		d.Sources = append(d.Sources, r.Source)
	}
	rows, e := editorSourceData(gs.Payload, 0x2c2c, 14*19)
	if e != nil {
		return d, nil, e
	}
	for i := 0; i < 14; i++ {
		row := rows[19*i:]
		name := OfficeCString(row[:13])
		r, e := installation.Loose(name)
		if e != nil {
			return d, nil, e
		}
		if len(r.Payload) < 4 {
			return d, nil, fmt.Errorf("editor map preview %s missing", name)
		}
		w, h := int(binary.LittleEndian.Uint16(r.Payload)), int(binary.LittleEndian.Uint16(r.Payload[2:]))
		if w < 1 || w > 127 || h < 1 || h > 127 || len(r.Payload) != 4+w*h {
			return d, nil, fmt.Errorf("editor preview %s dimensions", name)
		}
		o, e := preparedOutput(r.Outputs, "original_payload")
		if e != nil {
			return d, nil, e
		}
		d.Previews = append(d.Previews, editorPreparedPreview{strings.TrimSuffix(name, "SMP.GS2"), o, w, h, OfficeSignedWord(row[13:]), OfficeSignedWord(row[15:])})
	}

	// Exact source map inventory; suffix scans cannot silently omit a nested
	// container or select an unrelated similarly named member.
	mapRows := make([]*Resource, 0, len(d.Previews))
	for _, preview := range d.Previews {
		r, e := installation.Archive("MBUILDER.CAT", preview.Name+"MIS.PIC")
		if e != nil {
			return d, nil, e
		}
		mapRows = append(mapRows, r)
	}
	// Keep the original catalog-record emission order as well as exact logical
	// membership. Relocation does not change existing metadata array ordering.
	sort.Slice(mapRows, func(i, j int) bool { return mapRows[i].Source.Record.Index < mapRows[j].Source.Record.Index })
	for _, r := range mapRows {
		name, _ := sourceMemberName(r)
		v := editorPreparedMap{Name: strings.TrimSuffix(name, "MIS.PIC")}
		v.Pixels, e = preparedOutput(r.Outputs, "decoded_indices")
		if e != nil {
			v.Unavailable = "Source picture conversion did not produce complete indices"
		}
		v.Palette, e = preparedOutput(r.Outputs, "exact_rgb6_components")
		if e != nil {
			return d, nil, e
		}
		d.Maps = append(d.Maps, v)
	}
	return d, replayScreen, validateEditorMetadata(d)
}
func validateEditorMetadata(d editorMetadata) error {
	if d.ExtendedSchema != 0 && (d.ExtendedSchema != 1 || len(d.FillLabels) != 6 || len(d.DeleteLabels) != 19) {
		return fmt.Errorf("editor extended menus incompatible")
	}
	if d.Schema != editorMetadataSchema || d.Profile != IslandIce469085.Name || len(d.ExitRects) != 7 || len(d.GoalLabels) != 8 || len(d.Rectangles) != 36 || len(d.Keyboard) != 70 || len(d.IconRects) != 14 || len(d.IconAnchors) != 14 || len(d.Maps) != 14 || len(d.Previews) != 14 || d.Text["browser_title"] != "SIMULATED MISSIONS" || d.Text["exit_title"] != "EXIT MISSION BUILDER" || d.BrowserLabels[15] != "BEGIN" || d.BrowserLabels[19] != "EDIT" || d.BrowserLabels[21] != "NEW" {
		return fmt.Errorf("incomplete or incompatible Mission Editor metadata")
	}
	for name := range editorStrings {
		if d.Text[name] == "" {
			return fmt.Errorf("editor text %s missing", name)
		}
	}
	for i, want := range []string{"Search and Destroy", "Search and Rescue", "Intercept Moving", "Drop off Cargo", "Point Attack", "Drop off Troops", "Recon", "Pick up Troops"} {
		if d.GoalLabels[i] != want {
			return fmt.Errorf("editor goal label%d incompatible", i)
		}
	}

	seen := map[string]bool{}
	for _, v := range d.Previews {
		if v.Width < 1 || v.Width > 127 || v.Height < 1 || v.Height > 127 || v.X < 0 || v.Y < 0 || v.X+v.Width > 320 || v.Y+v.Height > 200 || v.Data.Size != 4+v.Width*v.Height || seen[v.Name] {
			return fmt.Errorf("editor preview bounds/name")
		}
		seen[v.Name] = true
	}
	for _, v := range d.Maps {
		if !seen[v.Name] {
			return fmt.Errorf("editor map has no source preview")
		}
		delete(seen, v.Name)
		if v.Palette.Size != 768 || v.Unavailable == "" && v.Pixels.Size != 64000 {
			return fmt.Errorf("editor map outputs missing")
		}
	}
	for _, v := range d.IconRects {
		if !preparedOfficeRectFits(v) {
			return fmt.Errorf("editor icon crop outside source atlas")
		}
	}

	for _, r := range d.Rectangles {
		if r.X < -1 || r.Y < -1 || r.X > 320 || r.Y > 254 || r.Width < 0 || r.Width > 320 || r.Height < 0 || r.Height > 254 {
			return fmt.Errorf("editor rectangle outside source domain")
		}
	}
	for _, k := range []string{"background", "panel", "icons"} {
		if d.Images[k].Size != 64000 {
			return fmt.Errorf("editor %s indices missing", k)
		}
	}
	return nil
}
func exportEditorMetadata(out *OutputWriter, alloc *pathAllocator, m *Manifest) error {
	if m.OfficeMetadata == "" {
		return nil
	}
	p, e := recognizeOfficeProfile(m.Resources)
	if e != nil {
		return nil
	}
	d, r, e := prepareEditorMetadata(m, p)
	if e != nil {
		return e
	}
	raw, e := JsonBytes(d)
	if e != nil {
		return e
	}
	if e = addGenerated(out, r, alloc, ".editor.json", "mission_editor_metadata", raw); e != nil {
		return e
	}
	m.EditorMetadata = r.Outputs[len(r.Outputs)-1].Path
	return nil
}

// The loose inventory and newly built packs keep the exact archive member.
// Read its last byte without prefetching beyond the member. This editor-only
// decode leaves the prepared resource identity used by historical films intact.
func phil3EditorPixels(reader *preparedReader) ([]byte, error) {
	for _, out := range reader.outputs {
		if out.Kind != "original_payload" || out.SHA256 != phil3EditorMemberSHA {
			continue
		}
		if reader.packs != nil {
			if proof, ok := reader.packs.catalog.Omitted[out.Path]; ok && proof.Hash == phil3EditorMemberSHA && proof.Size == 9615 {
				// Pre-W3 packs retain the logical digest but omit these bytes.
				return nil, nil
			}
		}
		raw, err := reader.verified(out, 9615)
		if err != nil {
			return nil, err
		}
		_, start, end, err := pictureHeader(raw)
		if err != nil {
			return nil, err
		}
		d, err := newPictureDecoder(raw[start:end])
		if err != nil {
			return nil, err
		}
		if d.width != 320 || d.height != 200 {
			return nil, fmt.Errorf("unexpected PHIL3 editor dimensions")
		}
		d.byteTail = true
		pixels := make([]byte, d.width*d.height)
		for i := range pixels {
			pixels[i], err = d.pixel()
			if err != nil {
				return nil, fmt.Errorf("PHIL3 editor pixel %d: %w", i, err)
			}
		}
		return pixels, nil
	}
	return nil, nil
}

func loadEditorAssets(reader *preparedReader, m Manifest, office *OfficeAssets) (*EditorAssets, error) {
	if m.EditorMetadata == "" && m.PreparationVersion < 26 {
		return nil, nil
	}
	o, ok := reader.outputs[m.EditorMetadata]
	if !ok || o.Kind != "mission_editor_metadata" {
		return nil, fmt.Errorf("prepared content lacks Mission Editor metadata; prepare26")
	}
	raw, e := reader.verified(o, 2<<20)
	if e != nil {
		return nil, e
	}
	var d editorMetadata
	if e = DecodeObject(raw, &d); e != nil {
		return nil, e
	}
	if e = validateEditorMetadata(d); e != nil {
		return nil, e
	}
	if m.PreparationVersion >= 28 {
		if len(d.WindVectors) != 16 {
			return nil, fmt.Errorf("editor wind16 direction metadata missing")
		}
		for _, key := range []string{"initial_status", "object_number", "weapons_prefix", "not_located", "not_designated", "day", "poor", "excellent"} {
			if d.Text[key] == "" {
				return nil, fmt.Errorf("editor source text %s missing", key)
			}
		}
	}

	if office == nil || office.Menus == nil || office.Missions == nil || office.Missions.Fonts[2] == nil || office.Menus.Fonts[3] == nil {
		return nil, fmt.Errorf("editor fonts unavailable")
	}
	fonts := map[int]*BitmapFont{}
	for key, font := range office.Missions.Fonts {
		fonts[key] = font
	}
	for key, font := range office.Menus.Fonts {
		fonts[key] = font
	}
	a := &EditorAssets{FormationTypes: d.FormationTypes, WindVectors: d.WindVectors, ModelInformationRaw: d.ModelInformationRaw, WeaponTypes: d.WeaponTypes, ModelInformation: d.ModelInformation, ModelNames: d.ModelNames, FillLabels: d.FillLabels, DeleteLabels: d.DeleteLabels, ExitRects: d.ExitRects, GoalLabels: d.GoalLabels, Browser: d.Browser, BrowserLabels: d.BrowserLabels, Maps: map[string]EditorMap{}, Previews: map[string]EditorPreview{}, Rectangles: d.Rectangles, Keyboard: d.Keyboard, IconRects: d.IconRects, IconAnchors: d.IconAnchors, Text: d.Text, Font: office.Menus.Fonts[3], Fonts: fonts, PointerAtlas: office.Images["POINTER.PIC"]}
	for key, dst := range map[string]*[]byte{"background": &a.Background, "panel": &a.Panel, "icons": &a.Icons} {
		*dst, e = reader.verified(d.Images[key], 64000)
		if e != nil || len(*dst) != 64000 {
			return nil, fmt.Errorf("editor %s pixels: %v", key, e)
		}
	}
	for _, v := range d.Previews {
		raw, e := reader.verified(v.Data, 65536)
		if e != nil || len(raw) != 4+v.Width*v.Height || v.Width <= 0 || v.Height <= 0 || int(binary.LittleEndian.Uint16(raw)) != v.Width || int(binary.LittleEndian.Uint16(raw[2:])) != v.Height {
			return nil, fmt.Errorf("editor preview %s framing: %v", v.Name, e)
		}
		a.Previews[v.Name] = EditorPreview{raw[4:], v.Width, v.Height, v.X, v.Y}
	}

	for _, v := range d.Maps {
		x := EditorMap{Unavailable: v.Unavailable}
		pal, e := reader.verified(v.Palette, 768)
		if e != nil || len(pal) != 768 {
			return nil, fmt.Errorf("editor %s palette: %v", v.Name, e)
		}
		x.Palette = make(color.Palette, 256)
		for i := range x.Palette {
			q := pal[3*i : 3*i+3]
			if q[0] > 63 || q[1] > 63 || q[2] > 63 {
				return nil, fmt.Errorf("editor RGB6 component")
			}
			x.Palette[i] = color.NRGBA{q[0]<<2 | q[0]>>4, q[1]<<2 | q[1]>>4, q[2]<<2 | q[2]>>4, 255}
		}
		if v.Unavailable == "" {
			x.Pixels, e = reader.verified(v.Pixels, 64000)
			if e != nil || len(x.Pixels) != 64000 {
				return nil, fmt.Errorf("editor map%s indices: %v", v.Name, e)
			}
		} else if v.Name == "PHIL3" {
			x.Pixels, e = phil3EditorPixels(reader)
			if e != nil {
				return nil, e
			}
			if x.Pixels == nil && reader.packs != nil {
				// The briefing and editor overview share a 166-pixel-wide map at
				// the sheet origin. Keep the editor panel palette and convert only
				// the map layer. Slots 13 and 82 animate in the editor.
				src, pal := office.Missions.Images["PHIL3MAP.PIC"], office.Missions.Palettes["FIELD.PAL"]
				if len(src) == 64000 && len(pal) == 256 {
					target := append(color.Palette(nil), x.Palette...)
					target[13], target[82] = target[0], target[0]
					var indices [256]byte
					for i, col := range pal {
						indices[i] = byte(target.Index(col))
					}
					x.Pixels = make([]byte, len(src))
					for i, value := range src {
						x.Pixels[i] = indices[value]
					}
				}
			}
			if x.Pixels != nil {
				x.Unavailable = ""
			}
		}
		a.Maps[v.Name] = x
	}
	return a, nil
}

// Historical readers stay permissive. Ordinary front-end startup separately
// requires the current UI capabilities; no film/document count is examined.
func ValidateCurrentFrontEndCapabilities(a *OfficeAssets) error {
	if err := ValidatePreparation33Capabilities(a); err != nil {
		return err
	}
	if !a.Audio.SupportsThreatWarningAudio() {
		return fmt.Errorf("source threat warning sound06 is missing or outdated; prepare34 (previous cache preserved)")
	}
	return nil
}

// ValidatePreparation33Capabilities retains the historical readiness contract
// for explicit old-cache compatibility and transactional regression fixtures.
func ValidatePreparation33Capabilities(a *OfficeAssets) error {
	if a == nil {
		return fmt.Errorf("front-end content is unavailable")
	}
	if a.World != nil {
		if w := a.World.Worlds["PHIL1"]; w != nil && Hash(w.TileAtlas) == Phil1BadAtlasSHA256 {
			return fmt.Errorf("known Philippines Map1 GPS atlas corruption; prepare33 from configured originals (previous cache preserved)")
		}
	}
	if !a.Audio.SupportsDefenseReleaseAudio() {
		return fmt.Errorf("countermeasure release sounds are missing or outdated")
	}
	if !a.Audio.SupportsStationaryHostileAudio() {
		return fmt.Errorf("stationary hostile gun sound is missing or outdated; prepare32")
	}
	if a.Replay == nil {
		return fmt.Errorf("Mission Films screen is missing")
	}
	if a.Cockpit == nil || len(a.Cockpit.MapVectors["gps-support-cursor"]) != 3 {
		return fmt.Errorf("Mission Support map cursor is missing or outdated")
	}
	// GS2 section4:2222/222D; the last artillery digit is patched by the
	// reached AA4 high nibble. These are UI capabilities, not content counts.
	for key, want := range map[string]string{"gps-support-aircraft": "- Aircraft", "gps-support-artillery": "- Artillery : 3"} {
		if a.Cockpit.Literals[key] != want {
			return fmt.Errorf("Mission Support availability text is missing or outdated")
		}
	}
	for _, entry := range [][2]string{{"mission_score", "MISSION SCORE: "}, {"time_taken", "TIME TAKEN: "}, {"total_kills", "TOTAL KILLS: "}, {"demo_title", "GUNSHIP 2000: DEMO MODE"}, {"demo_copyright", "Copyright 1991 MicroProse Software Inc."}} {
		key, want := entry[0], entry[1]
		if a.Replay.Text[key] != want {
			return fmt.Errorf("Mission Films text %s is missing or outdated", key)
		}
	}
	if a.Editor == nil || len(a.Editor.Background) != 64000 || len(a.Editor.Panel) != 64000 || len(a.Editor.Icons) != 64000 || len(a.Editor.Rectangles) != 36 || len(a.Editor.Keyboard) != 70 || len(a.Editor.FillLabels) != 6 || len(a.Editor.DeleteLabels) != 19 {
		return fmt.Errorf("Mission Editor screen is missing or outdated")
	}
	for key, want := range map[string]string{"delete_title": "DELETE DEFENDERS", "delete1": "Are you absolutely certain", "delete2": "you wish to completely remove"} {
		if a.Editor.Text[key] != want {
			return fmt.Errorf("Mission Editor deletion text %s is missing or outdated", key)
		}
	}
	for key, want := range map[string]string{"initial_status": "GUNSHIP 2000: Mission Builder", "object_number": "Object #", "weapons_prefix": "Weapons: ", "not_located": "Not located", "not_designated": "Not designated", "day": "DAY", "poor": "POOR", "excellent": "EXCELLENT"} {
		if a.Editor.Text[key] != want {
			return fmt.Errorf("Mission Editor text %s is missing or outdated", key)
		}
	}
	if len(a.Editor.WindVectors) != 16 {
		return fmt.Errorf("Mission Editor wind direction metadata is missing or outdated")
	}
	for key, want := range map[string]string{"located_tail": "ocated at ", "goal_coordinate_format": "%02d/%02d", "goal_detail_format": "%S %cat %s", "farp_location_format": "L%S%s", "base_location_format": "%S l%S%s", "deep_location_format": "%S at %s"} {
		if a.Editor.Text[key] != want {
			return fmt.Errorf("Mission Editor parameter text %s is missing or outdated", key)
		}
	}
	for prefix, n := range map[string]int{"GULF": 144, "EURO": 158, "PHIL": 109, "ANTC": 144} {
		if len(a.Editor.FormationTypes[prefix]) != n*26 {
			return fmt.Errorf("Mission Editor formation captions %s are missing or outdated", prefix)
		}
	}
	if a.Debrief == nil {
		return fmt.Errorf("Crew assignment screen is missing")
	}
	for _, name := range []string{"HAPPY.PIC", "HAPPY_2.PIC", "HAPPY_3.PIC", "SAD.PIC", "SAD_2.PIC", "SAD_3.PIC"} {
		if len(a.Debrief.Images[name]) != 64000 {
			return fmt.Errorf("Mission feedback picture %s is missing", name)
		}
	}
	for _, name := range []string{"HAPPY.PAL", "SAD.PAL", "SAD_2.PAL", "SAD_3.PAL"} {
		if len(a.Debrief.Palettes[name]) != 256 {
			return fmt.Errorf("Mission feedback palette %s is missing", name)
		}
	}
	replace := a.Debrief.Scenes["REPLACE.SCN"]
	if replace.Background != "FIELD.PIC" || replace.Palette != "FIELD.PAL" || len(replace.Areas) != 2 || len(replace.Lori) != 1 {
		return fmt.Errorf("Crew assignment scene is missing or outdated")
	}
	for _, name := range []string{"FIELD.PIC", "MEMO.PIC", "PILOT.PIC"} {
		if len(a.Debrief.Images[name]) != 64000 {
			return fmt.Errorf("Crew assignment picture %s is missing", name)
		}
	}
	for _, ds := range []int{0x5b82, 0x5b88, 0x5b90, 0x5b96, 0x5d71, 0x5f4e} {
		if a.Debrief.Text[ds] == "" {
			return fmt.Errorf("Crew assignment text %x is missing", ds)
		}
	}
	if a.Missions == nil || a.Missions.Fonts[2] == nil || a.Menus == nil || a.Menus.Fonts[3] == nil {
		return fmt.Errorf("Crew assignment fonts are missing")
	}
	if err := a.Missions.Generation.ValidateEnemyDefinitions(); err != nil {
		return err
	}

	return nil
}
