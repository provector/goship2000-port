package assets

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"strings"
)

type OfficeChunk struct {
	Index           int    `json:"index"`
	Tag             string `json:"tag"`
	HeaderRelative  int    `json:"header_resource_offset"`
	PayloadRelative int    `json:"payload_resource_offset"`
	PayloadAbsolute uint64 `json:"payload_backing_file_offset"`
	Length          int    `json:"payload_length"`
	RawHex          string `json:"payload_raw_hex"`
	Payload         []byte `json:"-"`
}

// No generic IFF padding is imposed: BASE's original consumer advances by
// header8+declared length. The resource bound here intentionally excludes the
// original compatibility look-ahead into the following archive entry.
func officeChunks(raw []byte, backingOffset uint64) ([]OfficeChunk, error) {
	if len(raw) < 12 || string(raw[:4]) != "FORM" {
		return nil, fmt.Errorf("truncated or missing FORM header")
	}
	if uint64(binary.BigEndian.Uint32(raw[4:8]))+8 != uint64(len(raw)) {
		return nil, fmt.Errorf("FORM size does not equal bounded resource length")
	}
	var result []OfficeChunk
	for pos := 12; pos < len(raw); {
		if len(raw)-pos < 8 {
			return nil, fmt.Errorf("truncated chunk header at resource offset %d", pos)
		}
		n := uint64(binary.BigEndian.Uint32(raw[pos+4 : pos+8]))
		if n > uint64(len(raw)-pos-8) {
			return nil, fmt.Errorf("chunk at resource offset %d extends outside resource", pos)
		}
		body := raw[pos+8 : pos+8+int(n)]
		result = append(result, OfficeChunk{Index: len(result), Tag: string(raw[pos : pos+4]), HeaderRelative: pos, PayloadRelative: pos + 8, PayloadAbsolute: backingOffset + uint64(pos+8), Length: int(n), RawHex: hex.EncodeToString(body), Payload: body})
		pos += 8 + int(n)
	}
	return result, nil
}

func OfficeSignedWord(b []byte) int { return int(int16(binary.LittleEndian.Uint16(b))) }

func OfficeCString(b []byte) string { return string(bytes.SplitN(b, []byte{0}, 2)[0]) }

func OfficeRectFits(r [4]int) bool {
	return r[0] >= 0 && r[1] >= 0 && r[2] > 0 && r[3] > 0 && r[0]+r[2] <= 320 && r[1]+r[3] <= 200
}

type officeComponent struct {
	Index           int    `json:"index"`
	ChunkIndex      int    `json:"chunk_index"`
	GroupKey        int    `json:"group_key"`
	Marker          int    `json:"serialized_group_marker"`
	Continuation    bool   `json:"continuation"`
	Selector        int    `json:"selector_signed_byte"`
	SourceBank      int    `json:"source_bank"`
	SourceAsset     string `json:"source_asset"`
	Source          [4]int `json:"source_rectangle_xywh"`
	Additive        [2]int `json:"additive_destination_xy"`
	SourceFits      bool   `json:"source_in_320x200"`
	DestinationFits bool   `json:"destination_in_320x200_with_zero_caller_operands"`
}

type OfficeGroup struct {
	Key     int   `json:"key"`
	Records []int `json:"records"`
}

type OfficeArea struct {
	ChunkIndex   int    `json:"chunk_index"`
	Key          int    `json:"key_signed_word"`
	Rectangle    [4]int `json:"rectangle_xywh"`
	InclusiveEnd [2]int `json:"inclusive_end_xy"`
	VisualFlag   int    `json:"visual_flag_unsigned_word"`
	Priority     int    `json:"source_order"`
}

type officeLori struct {
	ChunkIndex  int    `json:"chunk_index"`
	Key         int    `json:"key_signed_byte"`
	Mode        int    `json:"mode_unsigned_byte"`
	SourceAsset string `json:"source_asset"`
	Base        [2]int `json:"source_base_xy"`
	Cell        [2]int `json:"cell_wh"`
	Divisor     int    `json:"divisor_signed_word"`
	UnknownTail int    `json:"unknown_tail_signed_word"`
}

type OfficeFrame struct {
	Index       int    `json:"index"`
	Source      [4]int `json:"source_rectangle_xywh"`
	Destination [2]int `json:"destination_xy"`
}

type OfficeAnimation struct {
	ChunkIndex      int               `json:"chunk_index"`
	Key             int               `json:"key"`
	SourceOrigin    [2]int            `json:"source_origin_xy"`
	Size            [2]int            `json:"frame_wh"`
	Divisor         int               `json:"frame_divisor"`
	Destination     [2]int            `json:"destination_xy"`
	FrameCount      int               `json:"frame_count"`
	HoldCount       int               `json:"hold_count"`
	Pause           int               `json:"serialized_pause"`
	Frame           int               `json:"serialized_frame"`
	Hold            int               `json:"serialized_hold_counter"`
	CallbackHex     string            `json:"serialized_callback_hex"`
	RuntimeConsumer string            `json:"runtime_consumer_analyst_label"`
	Asset           string            `json:"source_asset,omitempty"`
	Frames          []OfficeFrame     `json:"direct_frames,omitempty"`
	CallbackSources map[string][4]int `json:"callback_source_rectangles_by_old_counter_parity,omitempty"`
}

type OfficeTextSource struct {
	Selector    int     `json:"selector"`
	PointerFile int     `json:"pointer_word_file_offset,omitempty"`
	DS          int     `json:"initialized_DS_offset"`
	File        int     `json:"file_offset"`
	RawHex      string  `json:"raw_hex"`
	Text        string  `json:"text"`
	Rectangle   *[4]int `json:"office_source_rectangle_xywh,omitempty"`
	Destination *[2]int `json:"office_destination_xy,omitempty"`
}

type OfficeTheatre struct {
	Selector     int            `json:"selector"`
	Name         string         `json:"source_name"`
	Source       SourceLocation `json:"source"`
	HeaderOffset uint64         `json:"header_backing_file_offset"`
	HeaderHex    string         `json:"header_raw_hex"`
	Label        string         `json:"label"`
	Points       [4][2]int      `json:"points_xy"`
	Segments     [][4]int       `json:"office_segments_xyxy"`
	Colour       int            `json:"colour_index"`
}

type officeAsset struct {
	Name          string             `json:"logical_name"`
	Source        SourceLocation     `json:"source"`
	ExtractedPath string             `json:"extracted_path"`
	Outputs       []Output           `json:"prepared_outputs"`
	PixelSHA256   string             `json:"pixel_sha256,omitempty"`
	Palette       PaletteAssociation `json:"palette"`
}

type officeLetter struct {
	Byte     int    `json:"character_byte"`
	Selector int    `json:"selector"`
	Source   [4]int `json:"source_rectangle_xywh"`
	Advance  int    `json:"advance"`
}

type OfficeMetadata struct {
	Schema         int                `json:"schema_version"`
	ScreenSource   SourceLocation     `json:"screen_source"`
	CodeSource     *SourceLocation    `json:"code_source,omitempty"`
	OuterType      string             `json:"outer_type"`
	ChunkCounts    map[string]int     `json:"chunk_counts"`
	Chunks         []OfficeChunk      `json:"chunks"`
	SCHD           map[string]any     `json:"screen_header"`
	Components     []officeComponent  `json:"components"`
	Groups         []OfficeGroup      `json:"groups"`
	Areas          []OfficeArea       `json:"areas"`
	Lori           []officeLori       `json:"lori"`
	Animations     []OfficeAnimation  `json:"animations"`
	Assets         []officeAsset      `json:"assets"`
	Fonts          []officeAsset      `json:"fonts"`
	Ranks          []OfficeTextSource `json:"rank_abbreviations"`
	Missions       []OfficeTextSource `json:"mission_types"`
	Captions       []OfficeTextSource `json:"caption_strings"`
	Denials        []OfficeTextSource `json:"denial_strings"`
	PilotFormat    *OfficeTextSource  `json:"pilot_format,omitempty"`
	Theatres       []OfficeTheatre    `json:"theatres"`
	Lettering      []officeLetter     `json:"notice_board_atlas_lettering"`
	LetteringAtlas map[string]any     `json:"notice_board_lettering_atlas,omitempty"`
	Initialization json.RawMessage    `json:"initialization"`
	Rules          json.RawMessage    `json:"instruction_derived_rules"`
	Warnings       []string           `json:"warnings,omitempty"`
}

func parseOfficeScreen(r *Resource) (OfficeMetadata, error) {
	m := OfficeMetadata{Schema: officeMetadataSchema, ScreenSource: r.Source, ChunkCounts: map[string]int{}}
	chunks, err := officeChunks(r.Payload, r.Source.Offset)
	if err != nil {
		return m, err
	}
	if string(r.Payload[8:12]) != "SCRN" {
		return m, fmt.Errorf("office resource is not FORM/SCRN")
	}
	m.Chunks = chunks
	m.OuterType = "FORM/SCRN"
	for _, c := range chunks {
		b := c.Payload
		m.ChunkCounts[c.Tag]++
		switch c.Tag {
		case "SCHD":
			if len(b) != 36 {
				return m, fmt.Errorf("SCHD size %d, expected36", len(b))
			}
			m.SCHD = map[string]any{"basename_slots": []string{OfficeCString(b[:9]), OfficeCString(b[9:18]), OfficeCString(b[18:27])}, "initial_position_signed_words": [2]int{OfficeSignedWord(b[27:]), OfficeSignedWord(b[29:])}, "area_key_signed_word": OfficeSignedWord(b[31:]), "unknown_tail_hex": hex.EncodeToString(b[33:])}
		case "CMPX":
			if len(b) != 14 {
				return m, fmt.Errorf("CMPX size %d, expected14", len(b))
			}
			if b[0] != 255 {
				m.Groups = append(m.Groups, OfficeGroup{Key: int(b[0])})
			}
			if len(m.Groups) == 0 {
				return m, fmt.Errorf("CMPX continuation precedes its group")
			}
			group := &m.Groups[len(m.Groups)-1]
			x := officeComponent{Index: len(m.Components), ChunkIndex: c.Index, GroupKey: group.Key, Marker: int(b[0]), Continuation: b[0] == 255, Selector: int(int8(b[13])), SourceBank: int(int8(b[13])) + 2}
			for i := 0; i < 4; i++ {
				x.Source[i] = OfficeSignedWord(b[1+2*i:])
			}
			x.Additive = [2]int{OfficeSignedWord(b[9:]), OfficeSignedWord(b[11:])}
			x.SourceAsset = map[int]string{2: "BASE_AN.PIC", 3: "BASE_AN1.PIC", 4: "BASE_AN2.PIC"}[x.SourceBank]
			x.SourceFits = OfficeRectFits(x.Source)
			x.DestinationFits = OfficeRectFits([4]int{x.Additive[0], x.Additive[1], x.Source[2], x.Source[3]})
			if !x.SourceFits || !x.DestinationFits || x.SourceAsset == "" {
				return m, fmt.Errorf("BASE CMPX%d has unsupported bank or rectangle", x.Index)
			}
			group.Records = append(group.Records, x.Index)
			m.Components = append(m.Components, x)
		case "AREA":
			if len(b) != 9 && len(b) != 11 {
				return m, fmt.Errorf("AREA size %d unsupported", len(b))
			}
			var padded [12]byte
			copy(padded[:], b)
			a := OfficeArea{ChunkIndex: c.Index, Key: OfficeSignedWord(padded[:]), VisualFlag: le16(padded[10:]), Priority: len(m.Areas)}
			for i := 0; i < 4; i++ {
				a.Rectangle[i] = OfficeSignedWord(padded[2+2*i:])
			}
			a.InclusiveEnd = [2]int{a.Rectangle[0] + a.Rectangle[2] - 1, a.Rectangle[1] + a.Rectangle[3] - 1}
			m.Areas = append(m.Areas, a)
		case "LORI":
			if len(b) != 14 {
				return m, fmt.Errorf("LORI size %d, expected14", len(b))
			}
			m.Lori = append(m.Lori, officeLori{ChunkIndex: c.Index, Key: int(int8(b[0])), Mode: int(b[1]), SourceAsset: "BASE_AN.PIC", Base: [2]int{OfficeSignedWord(b[2:]), OfficeSignedWord(b[4:])}, Cell: [2]int{OfficeSignedWord(b[6:]), OfficeSignedWord(b[8:])}, Divisor: OfficeSignedWord(b[10:]), UnknownTail: OfficeSignedWord(b[12:])})
		case "ANIM":
			a, err := parseOfficeAnimation(c)
			if err != nil {
				return m, err
			}
			m.Animations = append(m.Animations, a)
		default:
			return m, fmt.Errorf("unsupported office chunk %q", c.Tag)
		}
	}
	return m, nil
}

func parseOfficeAnimation(c OfficeChunk) (OfficeAnimation, error) {
	b := c.Payload
	if len(b) != 38 {
		return OfficeAnimation{}, fmt.Errorf("ANIM size %d, expected38", len(b))
	}
	a := OfficeAnimation{ChunkIndex: c.Index, Key: int(b[0]), SourceOrigin: [2]int{OfficeSignedWord(b[1:]), OfficeSignedWord(b[3:])}, Size: [2]int{OfficeSignedWord(b[5:]), OfficeSignedWord(b[7:])}, Divisor: OfficeSignedWord(b[9:]), Destination: [2]int{OfficeSignedWord(b[11:]), OfficeSignedWord(b[13:])}, FrameCount: int(b[15]), HoldCount: int(b[16]), Pause: int(b[17]), Frame: int(b[18]), Hold: int(b[19]), CallbackHex: hex.EncodeToString(b[21:25])}
	if a.Key == 1 || a.Key == 2 {
		if a.Divisor <= 0 {
			return a, fmt.Errorf("direct ANIM has nonpositive divisor")
		}
		a.Asset = "BASE_AN.PIC"
		a.RuntimeConsumer = "resident A10052 direct vertical-grid copy"
		for i := 0; i < a.FrameCount; i++ {
			rect := [4]int{a.SourceOrigin[0] + (i/a.Divisor)*a.Size[0], a.SourceOrigin[1] + (i%a.Divisor)*a.Size[1], a.Size[0], a.Size[1]}
			if !OfficeRectFits(rect) {
				return a, fmt.Errorf("ANIM%d frame%d outside source", a.Key, i)
			}
			a.Frames = append(a.Frames, OfficeFrame{i, rect, a.Destination})
		}
	} else if a.Key == 4 {
		a.Asset = "BASE_AN2.PIC"
		a.RuntimeConsumer = "section0 O0f36 forced-redraw callback; not an ordinary selectable mode"
		// This callback uses its own old counter parity, not ANIM.currentFrame.
		// Do not present its two crops as a direct-frame sequence.
		a.CallbackSources = map[string][4]int{"even": {0, 191, a.Size[0], a.Size[1]}, "odd": {0, 182, a.Size[0], a.Size[1]}}
	} else if a.Key == 0 || a.Key == 3 {
		a.RuntimeConsumer = "section0 O0654 procedural computer callback; no atlas frame list"
	} else {
		return a, fmt.Errorf("unknown office animation key%d", a.Key)
	}
	return a, nil
}

func officeSourceString(gs []byte, ds int) (OfficeTextSource, error) {
	p := officeDataAnchor + ds
	// Restrict this helper to the established initialized-data block and small
	// NUL strings; this is not a resident or arbitrary BSS mapping formula.
	if ds < 0 || p < officeDataAnchor || p >= len(gs) || p >= IslandIce469085.OfficeLayout.DataFileEnd {
		return OfficeTextSource{}, fmt.Errorf("office DS string outside initialized source")
	}
	limit := p + 512 // The original recovery memo is a 332-byte NUL string.
	if limit > len(gs) {
		limit = len(gs)
	}
	if limit > IslandIce469085.OfficeLayout.DataFileEnd {
		limit = IslandIce469085.OfficeLayout.DataFileEnd
	}
	n := bytes.IndexByte(gs[p:limit], 0)
	if n < 0 {
		return OfficeTextSource{}, fmt.Errorf("unterminated office DS string%04x", ds)
	}
	return OfficeTextSource{DS: ds, File: p, RawHex: hex.EncodeToString(gs[p : p+n]), Text: string(gs[p : p+n])}, nil
}

func officeSourceWord(gs []byte, ds int) (int, error) {
	p := officeDataAnchor + ds
	if ds < 0 || p+2 > len(gs) || p+2 > IslandIce469085.OfficeLayout.DataFileEnd {
		return 0, fmt.Errorf("office DS table outside initialized source")
	}
	return le16(gs[p:]), nil
}

func addOfficeCodeTables(m *OfficeMetadata, gs []byte) error {
	if Hash(gs) != officeCodeHash {
		return fmt.Errorf("office table executable identity mismatch")
	}
	table, err := officeSourceWord(gs, 0x1a90)
	if err != nil {
		return err
	}
	for i := 0; i < 12; i++ {
		ptr, err := officeSourceWord(gs, table+2*i)
		if err != nil {
			return err
		}
		s, err := officeSourceString(gs, ptr)
		if err != nil {
			return err
		}
		s.Selector = i
		s.PointerFile = officeDataAnchor + table + 2*i
		m.Ranks = append(m.Ranks, s)
	}
	for i := 0; i < 5; i++ {
		ptr, err := officeSourceWord(gs, 0xd96+2*i)
		if err != nil {
			return err
		}
		s, err := officeSourceString(gs, ptr)
		if err != nil {
			return err
		}
		s.Selector = i
		s.PointerFile = officeDataAnchor + 0xd96 + 2*i
		r := [4]int{15 + (i/2)*33, 180 + (i%2)*3, 33, 3}
		d := [2]int{208, 163}
		s.Rectangle = &r
		s.Destination = &d
		m.Missions = append(m.Missions, s)
	}
	for key, ds := range []int{0x1e72, 0x1e9c, 0x1ea8, 0x1ebb, 0x1ec3, 0x1ed4, 0, 0, 0x1ed6, 0x1ef1, 0x1f0e} {
		if ds == 0 {
			continue
		}
		s, err := officeSourceString(gs, ds)
		if err != nil {
			return err
		}
		s.Selector = key
		m.Captions = append(m.Captions, s)
	}
	for _, ds := range []int{0x1f25, 0x1f55, 0x1f80} {
		s, err := officeSourceString(gs, ds)
		if err != nil {
			return err
		}
		m.Denials = append(m.Denials, s)
	}
	s, err := officeSourceString(gs, 0x1e67)
	m.PilotFormat = &s
	return err
}

func officeTheatreGeometry(r *Resource, selector int) (OfficeTheatre, error) {
	t := OfficeTheatre{Selector: selector, Name: r.name, Source: r.Source, Colour: 4}
	chunks, err := officeChunks(r.Payload, r.Source.Offset)
	if err != nil {
		return t, err
	}
	for _, c := range chunks {
		if c.Tag != "THHD" {
			continue
		}
		b := c.Payload
		if len(b) != 54 {
			return t, fmt.Errorf("THHD has%d bytes, expected54", len(b))
		}
		t.HeaderOffset = c.PayloadAbsolute
		t.HeaderHex = c.RawHex
		t.Label = OfficeCString(b[1:18])
		for i := 0; i < 4; i++ {
			t.Points[i] = [2]int{OfficeSignedWord(b[38+2*i:]), OfficeSignedWord(b[46+2*i:])}
		}
		for i := 0; i < 3; i++ {
			t.Segments = append(t.Segments, [4]int{t.Points[i][0], t.Points[i][1], t.Points[i+1][0], t.Points[i+1][1]})
		}
		if t.Points[0][0] < 319 {
			t.Segments = append(t.Segments, [4]int{t.Points[3][0], t.Points[3][1], t.Points[0][0], t.Points[0][1]})
		}
		return t, nil
	}
	return t, fmt.Errorf("theatre resource has no THHD")
}

func officeLettering(lori []officeLori) ([]officeLetter, error) {
	for _, l := range lori {
		if l.Key != 2 {
			continue
		}
		if l.Divisor <= 0 {
			return nil, fmt.Errorf("atlas lettering divisor must be positive")
		}
		var result []officeLetter
		for c := 32; c <= 127; c++ {
			i := c - 32
			selector := 2*(i%48) + i/48
			r := [4]int{l.Base[0] + (selector/l.Divisor)*l.Cell[0], l.Base[1] + (selector%l.Divisor)*l.Cell[1], l.Cell[0], l.Cell[1]}
			if !OfficeRectFits(r) {
				return nil, fmt.Errorf("atlas character%d outside source", c)
			}
			result = append(result, officeLetter{c, selector, r, 4})
		}
		return result, nil
	}
	return nil, fmt.Errorf("office lacks LORI key2 lettering")
}

func officePreparedAsset(r *Resource, fonts bool) officeAsset {
	a := officeAsset{Name: r.name, Source: r.Source, ExtractedPath: r.ExportPath, PixelSHA256: r.Conversion.PixelSHA256, Palette: r.Conversion.Palette}
	for _, o := range r.Outputs {
		if !fonts || strings.Contains(o.Path, ".font3.") || strings.Contains(o.Path, ".font5.") {
			a.Outputs = append(a.Outputs, o)
		}
	}
	return a
}

func exportOfficeMetadata(out *OutputWriter, alloc *pathAllocator, m *Manifest) error {
	var base, gs *Resource
	for _, r := range m.Resources {
		if r.Source.PayloadSHA256 == officeScreenHash {
			base = r
		}
		if r.Source.SHA256 == officeCodeHash && r.Source.Kind == "loose_file" {
			gs = r
		}
	}
	if base == nil {
		for _, r := range m.Resources {
			if r.name == "BASE.SCN" {
				m.UnsupportedVersionOperations = append(m.UnsupportedVersionOperations, UnsupportedVersionOperation{"office presentation metadata", IslandIce469085.Name, "BASE.SCN identity is unsupported; raw extraction is retained"})
				break
			}
		}
		return nil
	}
	profile, profileErr := recognizeOfficeProfile(m.Resources)
	if profileErr != nil {
		m.UnsupportedVersionOperations = append(m.UnsupportedVersionOperations, UnsupportedVersionOperation{"office presentation metadata", IslandIce469085.Name, profileErr.Error()})
		return nil
	}
	m.VersionProfile = profile.Name
	doc, err := parseOfficeScreen(base)
	if err != nil {
		return err
	}
	if doc.ChunkCounts["SCHD"] != 1 || doc.ChunkCounts["AREA"] != 10 || doc.ChunkCounts["ANIM"] != 5 || doc.ChunkCounts["LORI"] != 3 || doc.ChunkCounts["CMPX"] != 105 || len(doc.Groups) != 32 {
		return fmt.Errorf("exact BASE identity has unexpected structural counts")
	}
	if gs == nil {
		doc.Warnings = append(doc.Warnings, "Exact GS.GS2 absent: rank/mission/caption tables unavailable")
	} else {
		doc.CodeSource = &gs.Source
		if err = addOfficeCodeTables(&doc, gs.backing); err != nil {
			return err
		}
	}
	for _, name := range []string{"BASE.PIC", "BASE_AN.PIC", "BASE_AN1.PIC", "BASE_AN2.PIC", "POINTER.PIC"} {
		found := false
		for _, r := range m.Resources {
			if r.name == name && r.Source.SHA256 == baseCatalogHash && r.Source.Kind == "archive_entry" {
				doc.Assets = append(doc.Assets, officePreparedAsset(r, false))
				found = true
				break
			}
		}
		if !found {
			doc.Warnings = append(doc.Warnings, "Required GS2000.CAT asset missing: "+name)
		}
	}
	for _, r := range m.Resources {
		if r.Source.PayloadSHA256 == originalFontHash {
			doc.Fonts = append(doc.Fonts, officePreparedAsset(r, true))
			break
		}
	}
	for selector := 0; selector < 4; selector++ {
		name := fmt.Sprintf("SCENE_%02d.THR", selector)
		found := false
		for _, r := range m.Resources {
			if r.name == name && r.Source.SHA256 == catalogHashes["GS2000.DAT"] {
				t, err := officeTheatreGeometry(r, selector)
				if err != nil {
					return err
				}
				doc.Theatres = append(doc.Theatres, t)
				found = true
				break
			}
		}
		if !found {
			doc.Warnings = append(doc.Warnings, "Theatre source absent: "+name)
		}
	}
	doc.Lettering, err = officeLettering(doc.Lori)
	if err != nil {
		return err
	}
	if err = exportOfficeLettering(out, alloc, m, &doc); err != nil {
		return err
	}
	doc.Rules = json.RawMessage(officeInstructionRules)
	doc.Initialization = json.RawMessage(strings.ReplaceAll(officeInitializationRules, "${PROFILE_CODE_SHA256}", profile.Source(RoleMainExecutable).SHA256))
	data, err := JsonBytes(doc)
	if err != nil {
		return err
	}
	if err = addGenerated(out, base, alloc, ".office.json", "office_presentation_metadata", data); err != nil {
		return err
	}
	m.OfficeMetadata = base.Outputs[len(base.Outputs)-1].Path
	// Metadata is a supported bounded conversion even though it is not an image.
	// Other screen families remain separately unsupported.
	if base.Conversion.Status == "unsupported" {
		m.Summary.Unsupported--
		m.Summary.UnsupportedFormats[base.Format]--
		if m.Summary.UnsupportedFormats[base.Format] == 0 {
			delete(m.Summary.UnsupportedFormats, base.Format)
		}
	}
	base.Conversion.Status = "office_presentation_metadata_exported"
	base.Conversion.RuntimeValidation = "static source-derived metadata with prior BASE native anchors; no new runtime validation or universal SCRN support"
	return nil
}

func exportOfficeLettering(out *OutputWriter, alloc *pathAllocator, m *Manifest, doc *OfficeMetadata) error {
	var sheet, paletteSource *Resource
	for _, r := range m.Resources {
		if r.Source.SHA256 != baseCatalogHash {
			continue
		}
		if r.name == "BASE_AN.PIC" {
			sheet = r
		}
		if r.name == "BASE.PAL" && Hash(r.Payload) == officePaletteHash {
			paletteSource = r
		}
	}
	if sheet == nil || paletteSource == nil {
		doc.Warnings = append(doc.Warnings, "Source-backed lettering crop unavailable: BASE_AN or verified BASE palette absent")
		return nil
	}
	palettes, _, _, err := pictureHeader(paletteSource.Payload)
	if err != nil || len(palettes) != 1 {
		return fmt.Errorf("invalid verified office palette")
	}
	_, pos, end, err := pictureHeader(sheet.Payload)
	if err != nil {
		return err
	}
	begin, limit := int(sheet.Source.Offset)+pos, int(sheet.Source.Offset)+end
	if limit < len(sheet.backing) {
		limit++
	}
	pixels, _, err := m.decode(sheet.backing[begin:limit], 320, 200)
	if err != nil {
		return err
	}
	if Hash(pixels) != baselinePictures["BASE_AN.PIC"] {
		return fmt.Errorf("lettering source indices differ from validated BASE_AN")
	}
	img := image.NewPaletted(image.Rect(0, 0, 192, 10), paletteColors(&palettes[0]))
	for y := 0; y < 10; y++ {
		copy(img.Pix[y*192:(y+1)*192], pixels[(190+y)*320:(190+y)*320+192])
	}
	data, err := PngBytes(img)
	if err != nil {
		return err
	}
	if err = addGenerated(out, sheet, alloc, ".office-lettering.png", "office_lettering_atlas_png", data); err != nil {
		return err
	}
	pngPath := sheet.Outputs[len(sheet.Outputs)-1].Path
	if err = addGenerated(out, sheet, alloc, ".office-lettering.indices", "office_lettering_atlas_indices", img.Pix); err != nil {
		return err
	}
	var glyphs []officeLetter
	for _, g := range doc.Lettering {
		g.Source[1] -= 190
		glyphs = append(glyphs, g)
	}
	info := map[string]any{"schema_version": 1, "source": sheet.Source, "source_pixel_sha256": Hash(pixels), "source_rectangle_xywh": [4]int{0, 190, 192, 10}, "png": pngPath, "dimensions": [2]int{192, 10}, "index_sha256": Hash(img.Pix), "characters": glyphs, "palette_source": paletteSource.Source, "opacity": "PNG opaque; original atlas indices unchanged. LORI key2 placement subsequently treats0 as transparent; this crop is coloured imagery, not a recolourable font.", "coordinate_rule": "Each character's source_rectangle_xywh is relative to this exported crop; original full-sheet rectangles remain in office metadata.", "baseline": nil, "line_advance": nil, "usage": "fixed four-pixel advance, native lettering helper uses a single calculated wrap and six-pixel second-line offset; no TTF/general font semantics"}
	data, err = JsonBytes(info)
	if err != nil {
		return err
	}
	if err = addGenerated(out, sheet, alloc, ".office-lettering.json", "office_lettering_atlas_metrics", data); err != nil {
		return err
	}
	doc.LetteringAtlas = map[string]any{"png": pngPath, "metrics": sheet.Outputs[len(sheet.Outputs)-1].Path, "index_sha256": Hash(img.Pix)}
	for i := range doc.Assets {
		if doc.Assets[i].Name == "BASE_AN.PIC" {
			doc.Assets[i] = officePreparedAsset(sheet, false)
		}
	}
	return nil
}

// Rules below are instruction-derived application contracts, not serialized
// source tables or purported original names. Source records above are parsed
// afresh. These rules deliberately contain no invented millisecond durations.
const officeInstructionRules = `{
  "scope": "Office presentation only; no menu activation or persistence. Static rules with prior runtime anchors, not new guest execution.",
  "coordinates": "O=section0 original CS1a79, file=0x1cff0+O; A=verified resident analysis coordinates only. Initialized DS table source anchor0x3d2f0 applies only to bounded parsed tables, not BSS.",
  "composition": {
    "source_indices_preserved": true,
    "scene_remapping": "none baked into office pictures",
    "component_selector": "signed CMPX+13 plus2 selects source bank",
    "extraction": "copy complete source rectangle to a temporary before placement, including alias cases",
    "placement": "source index0 omits destination write; effective xy=record additive xy+caller operands+destination descriptor origin",
    "ambient_groups": [
      0,
      1,
      2,
      3,
      4,
      5,
      6,
      7,
      8,
      9,
      10
    ],
    "transition_groups": "11..31 are not an ambient sequential-frame list; folder transition continues current group+1 through31 and is deferred for navigation",
    "layers": [
      "BASE pristine background",
      "ordered initial preparation using BASE_AN/1/2",
      "pilot/rank/unit/mission/theatre areas",
      "officer and computer updates in original logical order",
      "caption strip",
      "non-destructive topmost pointer"
    ],
    "lori_mode_dispatch": {
      "zero": "opaque direct source-bank copy; BASE LORI key1 mission stripe has mode0",
      "nonzero": "extract then place with index0 transparency; BASE LORI keys0/2 have mode1",
      "source": "resident A20824..20859 mode0 branch, prior Step13 event187; serialized mode remains exported per record"
    }
  },
  "display_state": {
    "inputs": [
      "pilot name byte string",
      "squadron/unit byte string",
      "rank selector",
      "emblem selector",
      "mission type selector",
      "theatre selector",
      "active status narrow predicate==1",
      "snapshot-present narrow predicate",
      "computer mode0..3",
      "animation counters/state"
    ],
    "pilot_text": {
      "name_rule": "trim spaces at ends, final token after last space, at most30 bytes; format source rank abbreviation plus one space plus token",
      "destination": [
        157,
        46
      ],
      "width": 60,
      "max_lines": 1,
      "atlas": "BASE_AN.PIC LORI key2",
      "advance": 4,
      "validated_example": "WOC Name"
    },
    "unit_text": {
      "destination": [
        169,
        52
      ],
      "width": 48,
      "max_lines": 2,
      "line_spacing": 6,
      "atlas": "BASE_AN.PIC LORI key2",
      "wrap": "O0e12 computes one break by scanning backwards over ASCII letters/digits before limit; not a generic repeated word-wrap engine",
      "algorithm": {
        "capacity": "trunc_signed(width/4)-1",
        "break_index": "capacity-1; if capacity<len(text) and lineBudget>=2, scan backward while ASCII letter/digit; if reaches0 use original capacity-1",
        "loop": "while index<len(text) and advance<width: draw supported byte at(x+advance,y); advance+=4 even when byte rejected; if index==breakIndex decrement lineBudget, return if0, else y+=6 and advance=0; increment index",
        "single_break": "never recalculate on later line",
        "fixture_visible_lines": [
          "Test ",
          "Squadron Nam"
        ],
        "fixture_limit": "Expected commands for source input, not hard-coded pixels; final e falls beyond width."
      }
    },
    "emblem": {
      "lori_key": 0,
      "destination": [
        158,
        52
      ],
      "selector": "current record+53h signed byte; tiny10x10 BASE_AN cells, not PATCHES editor images"
    },
    "mission": {
      "lori_key": 1,
      "destination": [
        208,
        163
      ],
      "selector": "current record+65h signed byte; source tables provide known nonnegative0..4; signed IDIV, no invented negative-index policy"
    },
    "theatre": {
      "selector": "current record+64h signed byte",
      "outline": "accepted SCENE_00..03.THR filename constructor order; THHD points+38,+46, colour4; three segments and final3->0 only when firstX<319",
      "restoration": "save inclusive bounding rectangle before drawing; restore old outline before replacing"
    }
  },
  "caption": {
    "strip": [
      0,
      191,
      320,
      9
    ],
    "clear_index": 0,
    "origin_relative": [
      0,
      2
    ],
    "center": "origin+signed-truncated((320-measured_advance)/2)",
    "normal_foreground": 7,
    "font_by_key": {
      "0": 3,
      "otherwise": 5
    },
    "empty_keys": [
      5,
      6,
      7
    ],
    "empty_text": " ",
    "dynamic": {
      "2": "THEATER OF DUTY caption prefix plus selected THHD label; mission4 normalizes to key10 THEATER OF CAMPAIGN",
      "3": "TYPE prefix plus mission table label; convert ASCII a..z only to uppercase"
    },
    "denial": {
      "font": 3,
      "foreground": 12,
      "inactive": "keys2/3/4 when current+56h!=1",
      "training_theatre": "key4 when mission0 and signed theatre>1",
      "snapshot": "key9 when snapshot first byte==0"
    },
    "literal_roster": "key1 DUTY ROSTER is separate from pilot information",
    "bottom_clip_inclusive": 199,
    "normalization": "After denial checks: key2 with mission4 becomes10; otherwise negative or5<=key<8 becomes5. Empty keys share original single-space caption; data table source exports actual strings."
  },
  "hit_testing": {
    "order": "reverse AREA table order",
    "bounds": "signed inclusive rectangles",
    "visual_flag": "padded AREA+10 word determines cursor style; AREA+9 is high height byte, not an enable flag",
    "activation_deferred": true
  },
  "pointer": {
    "asset": "POINTER.PIC",
    "templates": "18 top-row cells [16*i,0,16,16]; office uses0..7",
    "ordinary": {
      "sequence": [
        0,
        1,
        2,
        3,
        2,
        1
      ],
      "hotspot": [
        0,
        0
      ]
    },
    "actionable": {
      "sequence": [
        4,
        5,
        6,
        7,
        6,
        5
      ],
      "hotspot": [
        7,
        7
      ]
    },
    "style": "AREA visual flag zero=>ordinary; nonzero=>actionable; reinstall resets sequence/subcounter/hotspot",
    "update": "nonzero advance argument increments subcounter; at5 reset0 and advance six-entry sequence; this counts helper calls, not milliseconds",
    "clip_exclusive": [
      0,
      0,
      320,
      191
    ],
    "blend": "extract then place with index0 transparent; preserve underlying pixels, do not bake cursor into background"
  },
  "scheduler": {
    "tick_source": "shared input wait calls A212a0(1), polling VGA3da bit3 set then clear; exact real-time Hz remains unmeasured",
    "duration_ms": null,
    "port_policy": "modern elapsed-time accumulator with documented configurable nominal tick; no busy waits",
    "order": [
      "shared office callback (officer/caption)",
      "ANIM scheduler forward order",
      "dirty presentation",
      "pointer"
    ],
    "officer": {
      "initial_group": 255,
      "cycle": [
        0,
        1,
        2,
        3,
        4,
        5,
        6,
        7,
        8,
        9,
        10
      ],
      "update": "if old byte callback counter>=12: reset0, increment group, wrap>10 to0, draw group operands0,0, mark dirty; otherwise increment counter",
      "callbacks_between_groups": 13,
      "initial_callback_counter": "Normal startup A10c95 explicitly zeroes SS:[742e,e2a0), then DS=SS. This includes9881/9882 and listed procedural state; office re-entry does not repeat the clear. First group0 is callback13 after zero unless intervening calls/writes advance it."
    },
    "direct_frames": "ANIM1/2 increment hold, then frame before drawing; frame wraps at count; vertical grid uses quotient forX and remainder forY",
    "early_return": "active record whose hold has not expired returns from entire ANIM update; later records do not tick on that call",
    "force_redraw": "A1028c visits4,3,2,1,0; nonnull callback executes even while paused; null callback skip depends stopCount+25, pause+17, redrawWhenPaused+37"
  },
  "computer": {
    "initial_mode": 0,
    "mode_cycle": [
      0,
      1,
      2,
      3,
      0
    ],
    "initial_mode_random": false,
    "switching": "mode change pauses all but matching ANIM0..3, does not reset all record frame/hold counters; mode3 entry calls generateBlock(0), sets reset flag",
    "mode0": {
      "kind": "procedural scroll/noise",
      "ordered_ops": [
        "if reset flag: fill[136,85,15,9] index0, clear threshold/cursor/reset and return",
        "when ANIM currentFrame==0 increment uint16 cursor",
        "if signed threshold<=signed cursor: opaque self-copy[136,86,15,8] to[136,85], clear inclusive bottom line136,93..150,93; threshold=RNG(16); cursor=0",
        "write pixel[136+cursor,93] using colours[10,2,2,0][RNG(cursor==0?3:4)]",
        "clear reset flag"
      ],
      "cursor_invariant": "generated threshold0..15; branch resets before write so cursor0..14"
    },
    "mode1": {
      "kind": "BASE_AN vertical-grid atlas",
      "frames": 40,
      "hold": 1
    },
    "mode2": {
      "kind": "BASE_AN vertical-grid atlas",
      "frames": 80,
      "hold": 4
    },
    "mode3": {
      "kind": "procedural blocks/cursor",
      "ordered_ops": [
        "save redrawNeeded=resetFlag!=0",
        "restore saved underlying cursor pixel at136+cursorX,85+cursorY",
        "move each signed cursor byte toward target block x/y by one",
        "on arrival redrawNeeded=true; adjust block count as original comparison, generateBlock(count) when adjusted; target=RNG(4)",
        "if redraw: fill[136,86,15,8] index9; top inclusive line136,85..150,85 index11; draw blocks ordered index0..<signed count",
        "save pixel under cursor; draw cursor index15",
        "clear reset flag"
      ],
      "arrival_count_rule": "if count>target or(count==target and count<3): if count==target count++; else if count-target-1==0 count--; generateBlock(count)",
      "block": {
        "generate_order": [
          "x=RNGInclusive(1,9)",
          "width=RNGInclusive(4,13-x)",
          "y=(index==0?0:RNGInclusive(0,4))",
          "height=RNGInclusive(3,8-y)"
        ],
        "interior": "fill x+1,y+1,width-2,height-2; colour index i==2?(width<height?2:4):3",
        "border": "inclusive outline x,y through x+width-1,y+height-1; colour i==2?(width<height?10:12):11",
        "shadow": "colour0 right line(x+width,y+1)..(x+width,y+height), bottom line(x+1,y+height)..(x+width,y+height)"
      }
    },
    "forced_callback4": {
      "selectable_mode": false,
      "operation": "opaque BASE_AN2 [0, old counter parity?182:191,15,9] to136,85, then increment byte counter; forced redraw may invoke before later callbacks overpaint"
    },
    "rng": {
      "generator": "state=uint32(state*0x343fd+0x269ec3); return(state>>16)&0x7fff",
      "range": "signed n<=1=>0 else floor(next15*n/32768)",
      "seed": "shared application RNG lazily seeded via BIOS INT1a wrapper; no exact stochastic history claimed"
    },
    "activation": "AREA6 event13 cycles mode0..3; Step B may expose this bounded visual interaction without implementing other navigation."
  }
}`

// Authored logical initialization recipe recovered from section0 and resident
// consumers. Resource fields remain parsed above; no evidence file is read.
const officeInitializationRules = `{
  "schema_version": 1,
  "scope": "Office initialization O024e..04b1 and ordinary first update order; port-facing logical recipe, not a replay of668 debugger events or a new raster renderer",
  "source": {
    "module": "GS.GS2",
    "sha256": "${PROFILE_CODE_SHA256}",
    "coordinate_policy": "O=section0 originalCS1a79 offset;file=0x1cff0+O. A=verified resident analysis load1000h,file=A-0xe000. No runtime addresses assumed.",
    "static_evidence": [
      "evidence/step14/office-prefix-source.asm",
      "evidence/step21/state/office-display-0410-0e12.asm",
      "evidence/step20/analysis/text/GS_GS2-0x24e66-d54024dd9f95877f.asm",
      "evidence/step20/analysis/text/GS_GS2-0x2534e-952ad39d76bec2e6.asm",
      "evidence/step20/analysis/text/GS_GS2-0x1028c-2da58ec168f53927.asm"
    ],
    "prior_runtime_anchor": "Step13 run03 original-source preparation and group0 comparisons; Step17 named-pilot atlas command witnesses"
  },
  "explicit_inputs": [
    "current pilot visual fields",
    "missionType",
    "selected acceptedTHHD record",
    "computer mode and procedural state",
    "officer shared-callback counter",
    "initial hoveredAREA key",
    "initial pointerposition",
    "ANIM serialized state plus any retained refresh state",
    "RNG state/seed policy for procedural output",
    "fadeIn boolean"
  ],
  "entry_semantics": {
    "normal_first_office": "O000d..0030 selects current pilot, resets computerMode0, calls initializer with initialAREA1 and fadeIn1",
    "refresh": "Existing return paths call same initializer with other AREA keys and fadeIn0. Do not clear every procedural/global counter on refresh: O024e explicitly resets only officerGroupFF here; startupBSS clear supplies earlier zeros on fresh process.",
    "dimension": [
      320,
      200
    ],
    "initial_resources": {
      "bank1": "BASE.PIC",
      "bank2": "BASE_AN.PIC",
      "bank3": "BASE_AN1.PIC",
      "bank4": "BASE_AN2.PIC",
      "palette": "BASE.PAL"
    },
    "immutable_source_rule": "Keep decoded assets immutable; maintain separate working bank/scratch state. Shared descriptor0892 starts bank2, descriptor08b6 starts bank4; neither descriptor name meansbank3."
  },
  "ordered_commands": [
    {
      "order": 1,
      "op": "load_screen_definition_and_initial_pictures",
      "screen": "BASE.SCN",
      "evidence": "O025b..0268 -> A24e66;A252b0..252e1",
      "effect": "Parse originalAREA/LORI/CMPX/ANIM data; BASE into workingbank1,BASE_AN into workingbank2. Initialize source descriptors and pointer-style defaults from existing source metadata. Do not treat the later resource-lookahead as screen content."
    },
    {
      "order": 2,
      "op": "initialize_officer_group",
      "value": 255,
      "evidence": "O026e",
      "effect": "Group leadFF so first advanced ambientgroup is0. Shared callback counter is separately retained or startup-zeroed."
    },
    {
      "order": 3,
      "op": "load_additional_original_atlases",
      "images": [
        {
          "name": "BASE_AN1.PIC",
          "bank": 3
        },
        {
          "name": "BASE_AN2.PIC",
          "bank": 4
        }
      ],
      "evidence": "O0273..028a",
      "effect": "Populate independent mutable banks before callbacks or CMPX consumers refer to them. No direct initial bank3-to-bank1 opaque copy is asserted merely because bank3 was loaded."
    },
    {
      "order": 4,
      "op": "select_theatre_and_save_background",
      "evidence": "O0293..02ab;O050a..05d3",
      "state_rule": "If signedtheatre > signedDSb830 then write0; original branch is strictgreater, not >=. A safe port can instead reject unsupportedstate explicitly.",
      "background_key": "theatreBackground",
      "rectangle_rule": "min/max of originalTHHD +38 Xwords and+46 Ywords; width=maxX-minX+1,height=maxY-minY+1",
      "effect": "Save exact bank1 rectangle before the outline is drawn; no captured pixels used."
    },
    {
      "order": 5,
      "op": "draw_theatre_outline",
      "evidence": "O05d8..0651",
      "colour_index": 4,
      "points_source": "selected THHD LE16 X[4]at+38,Y[4]at+46",
      "segments": "0->1,1->2,2->3,then3->0 only if X[0]<319",
      "endpoint_rule": "inclusive original line raster rule already recovered Step13",
      "effect": "Draw over pristineBASE before pilot annotations."
    },
    {
      "order": 6,
      "op": "save_board_background",
      "evidence": "O02ae..0309",
      "background_key": "boardBackground",
      "rectangle_from_AREA1": "[AREA1.x+5,AREA1.y+4,AREA1.width-5,16]",
      "baseline_rectangle": [
        157,
        46,
        60,
        16
      ],
      "effect": "Save background after theatreoutline but before tinyemblem/rank/unit. This save is later cleaned up on office exit; it is not an initial drawing or clear operation."
    },
    {
      "order": 7,
      "op": "draw_tiny_emblem",
      "evidence": "O030e..0326;A2077a..20821",
      "asset": "BASE_AN.PIC",
      "lori_key": 0,
      "selector": "sign-extendedcurrent+53h",
      "destination_from_AREA1": "[AREA1.x+6,AREA1.y+10]",
      "baseline_destination": [
        158,
        52
      ],
      "composition": "extract rectangle10x10 then place nonzeroindices; preserve zero transparency only for this operation"
    },
    {
      "order": 8,
      "op": "configure_board_text_region_without_fill",
      "evidence": "O037f..03b2;A1c8c0..1c924",
      "rectangle": [
        157,
        46,
        60,
        16
      ],
      "background_operand": -1,
      "font_setup": 1,
      "spacing_override": 5,
      "foreground_setup": 8,
      "effect": "Negative background operand skips fill at A1c908..1c924. These text globals do not recolour the following LORI atlas glyphs. Do NOT insert an opaque blank board rectangle."
    },
    {
      "order": 9,
      "op": "draw_rank_and_final_name_token",
      "evidence": "O0329..037c;O03cf..03e2;O0e12..0f34",
      "text_rule": "trim ranklabel, trim pilotname, final token max30bytes, format %s %s",
      "atlas": "BASE_AN.PIC",
      "lori_key": 2,
      "destination": [
        157,
        46
      ],
      "width": 60,
      "line_budget": 1,
      "advance": 4,
      "composition": "per-glyph extract/place nonzeroindices; exact original short-text termination"
    },
    {
      "order": 10,
      "op": "draw_unit_text",
      "evidence": "O03e5..0403;O0e12..0f34",
      "text_source": "current+34h",
      "atlas": "BASE_AN.PIC",
      "lori_key": 2,
      "destination": [
        169,
        52
      ],
      "width": 48,
      "line_budget": 2,
      "advance": 4,
      "line_spacing": 6,
      "layout_rule": "single backward-searched break position; see StepB display contract exact pseudocode",
      "fixture_visible_lines": [
        "Test ",
        "Squadron Nam"
      ],
      "fixture_limit": "final e notdrawn, witnessed by17 natural unitcommands"
    },
    {
      "order": 11,
      "op": "draw_mission_stripe_opaque",
      "evidence": "O0406..042e;A20824..20859;Step13 run03 event187",
      "asset": "BASE_AN.PIC",
      "source_descriptor": "0892 boundbank2/origin0,0 in prior native call",
      "lori_key": 1,
      "selector": "sign-extendedcurrent+65h",
      "source_rule": "x=15+33*quotient(selector/2),y=180+3*remainder(selector/2),w33,h3",
      "destination": [
        208,
        163
      ],
      "composition": "direct opaque copy including index0; NOT transparent extraction"
    },
    {
      "order": 12,
      "op": "bind_office_callbacks_and_active_mode",
      "evidence": "O0431..0478;O0a70..0af9",
      "callbacks": {
        "ANIM0": "O0654",
        "ANIM3": "O0654",
        "ANIM4": "O0f36",
        "shared_update": "O0afa",
        "postdirty_copy": "O0c9e"
      },
      "active_ANIM_rule": "mode0..3 enables exactly matching ANIM0..3; all otherspaused. A forced draw still invokes nonnullcallbacks despitepause.",
      "effect": "No immediate ambient CMPXgroup0 draw occurs solely from callback binding or mode enable."
    },
    {
      "order": 13,
      "op": "optional_fade_out_or_staging",
      "evidence": "O0479..048b",
      "condition": "fadeIn!=0",
      "effect": "Preserve existing palette staging/fade behavior separately from pixel generation. Only completedBASE.PAL is required for an initial unfaded port view; do not confuse black workingtable with finalofficepalette."
    },
    {
      "order": 14,
      "op": "initialize_pointer_at_AREA_and_force_reverse_ANIM_draw",
      "evidence": "O048e -> A2534e;A25357..253a5;A1028c..10327",
      "pointer_rule": "Move hitpoint to selectedAREA center plus signedAREA+32/+33 adjustments; normal initialAREA1 is184,70. Defaults templatebase0,hoverbase4/hotspot7,7.",
      "ANIM_visit_order": [
        4,
        3,
        2,
        1,
        0
      ],
      "forced_rule": "Call nonnullcallback first regardlesspause; otherwise apply originalflags before directopaquecopy. Clear dirtyfield aftervisit and apply sourceone-shotflag as established.",
      "initial_mode0_visible_operations": "ANIM4 callback copies BASE_AN2; ANIM3 callback executes mode0 procedural path; paused directANIM2/1 do not draw on normal zeroflagstate; ANIM0 callback again executes mode0 proceduralpath. Use recovered proceduralstate/RNG, not a fabricated atlasframe list.",
      "first_nested_copy": {
        "op": "forced_ANIM4_fixed_source_copy",
        "evidence": "O0f36..0f90;Step13 run03 event189",
        "asset": "BASE_AN2.PIC",
        "destination": [
          136,
          85,
          15,
          9
        ],
        "selector_rule": "oldbyte9882&1 before increment: odd->source(0,182,15,9);even->source(0,191,15,9)",
        "initial_fresh_process": "startup-zero makes firstcopy source(0,191,15,9)",
        "composition": "opaque including index0",
        "effect": "May be overpainted by later mode0 callbacks in same reverseANIMvisit; preserve logical order. Not a fifth selectable computer mode."
      }
    },
    {
      "order": 15,
      "op": "run_shared_office_callback_once_and_draw_hover_caption",
      "evidence": "A253aa..253ee;O0afa..0c99;A256ea",
      "effect": "Hit-test currentpoint, process emptyinlineAREAcaption, run sharedcallbackonce. Callback updates officer counter then state-dependent bottomcaption. Normal initialcallback starts counter0 and does not drawgroup0 until oldcounter>=12. Draw caption in black strip0,191,320,9 atY193,centered with font3or5 and colour7or denial12. No need to perform12extra redraws merely to establish caption pixels."
    },
    {
      "order": 16,
      "op": "compose_pointer_then_present_office",
      "evidence": "A253f2..25405;A235fc/A23768;A1d5aa",
      "effect": "Compose initial clippedtransparentcursor using existing source/hotspotmetadata, copy bank1 into presentation bank0. Keep pointer as a non-destructive final layer in the port, without permanently baking scratch or pointer pixels into independentassets."
    },
    {
      "order": 17,
      "op": "stage_and_install_completed_office_palette",
      "evidence": "O0493..04ab;Step10 actualDAC evidence",
      "condition": "fadeIn!=0",
      "palette": "BASE.PAL completed256entryRGB6",
      "effect": "If faithfully showingfade, use recovered factors independently of pixelstate; otherwise present completedverifiedpalette. Initialization finishes atO04b1."
    }
  ],
  "subsequent_update_order": [
    "A2544e wait boundary uses retrace source; modernelapsed-time tick replacesbusywait, rateparameter remains separate.",
    "Pointer update/hide oldrectangle, then shared O0afa callback: oldcounter>=12 advances officerGroup/wrap0..10 and applies groupCMPX commands; caption recalculated.",
    "A10052 applies active ANIM logical hold/frameupdates and proceduralcallbacks in established order. Retain sourceholdcounts and globalpausecounter effects from animationmetadata.",
    "Presentdirtyregions and pointer; never infer all105CMPX are sequentialofficerframes."
  ],
  "restoration": [
    {
      "phase": "officecleanup only",
      "evidence": "O04b2..0509",
      "effect": "Disable pointer; restore saved boardrectangle at157,46 using boardBackground; restore saved selectedtheatreboundingbox using theatreBackground; release savedtemporaries and closepicturecache. Do not run this cleanup between initialannotations or before each ambientframe."
    },
    {
      "phase": "pointermovement/redraw",
      "evidence": "A23768/A23962",
      "effect": "Save/restore clipped previousscene under pointer using descriptor0892 scratch or an equivalent isolated topmostlayer. Sourceatlasmustremainimmutable in port."
    }
  ],
  "validation_and_limits": {
    "initial_pixels": "A20824 stripeopacity and no boardclear are required to match actualinitialpixels; transparency for allLORI would be incorrect.",
    "previous_reference_boundary": "Step13 preparedbank boundary is AFTER repeatedsharedcallback/ANIMupdates immediately before firstgroup0, not simplyO04b1initializationreturn. Matchingthatreference requires matching proceduralstate/RNG and updatephase; do not force callbackcounts/timing to get its hash.",
    "fixed_copy_scope": "Existing Step13 boundedcopyevents show sourcebank4to1 event189. No sourcebank3to1 directcopy was found in that specific interval; BASE_AN1 remains requiredload/CMPXdependency elsewhere but no invented initialvisiblepatch is added.",
    "remaining_uncertainty": [
      "Exact elapsed-time cadence versus retrace/poll counts is unmeasured.",
      "ProceduralRNG seed/phase and natural earliercallbacks may change computerpixels; use explicitstate or documentedportseedpolicy, never capturedpixelpatches.",
      "This authoredlogicalrecipe is not itself a newcompleteframecomparison; reuse Step13 source-backed models/reference only undermatchingconditions."
    ]
  }
}`
