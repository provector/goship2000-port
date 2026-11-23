package assets

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
)

const menuMetadataSchema = 2

// These are initialized-data offsets for the recognized release, never runtime
// segment addresses. Export the literals so the player needs no executables.
var menuSourceStrings = map[string]int{
	"quit": 0x474, "memo_from": 0x1fb7, "memo_header": 0x1fc4,
	"mission_prompt": 0x1feb, "mission_availability": 0x205b, "theatre_prompt": 0x2556,
	"recovery_from": 0x1fa9, "recovery_text": 0x21ba, "recovery_prompt": 0x2307,
	"scores_format": 0x1c6d, "missions_format": 0x1c79,
	"campaign_underway": 0x1c8d, "campaigns_format": 0x1ca3,
}

type MenuScreen struct {
	Name       string         `json:"name"`
	Background string         `json:"background"`
	Basenames  [3]string      `json:"picture_basenames"`
	Source     SourceLocation `json:"source"`
	Chunks     []OfficeChunk  `json:"chunks"`
	Areas      []OfficeArea   `json:"areas"`
	Lori       []officeLori   `json:"lori"`
	InitialKey int            `json:"initial_area_key"`
}

type MenuEmblem struct {
	Selector int    `json:"selector"`
	Source   [4]int `json:"source_rectangle_xywh"`
	Small    [4]int `json:"small_source_rectangle_xywh"`
}

type MenuTheatre struct {
	Selector  int            `json:"selector"`
	Name      string         `json:"label"`
	Rectangle [4]int         `json:"rectangle_xywh"`
	Source    SourceLocation `json:"source"`
	RawHeader string         `json:"raw_THHD_hex"`
}

type menuMetadata struct {
	Schema                int                         `json:"schema_version"`
	Profile               string                      `json:"source_profile"`
	Screens               map[string]MenuScreen       `json:"screens"`
	Assets                []officeAsset               `json:"assets"`
	Fonts                 []officeAsset               `json:"fonts"`
	Emblems               []MenuEmblem                `json:"emblems"`
	Theatres              []MenuTheatre               `json:"theatres"`
	FullRanks             []OfficeTextSource          `json:"rank_full_labels"`
	Strings               map[string]OfficeTextSource `json:"source_strings"`
	Notes                 []string                    `json:"interpretation_limits"`
	DecorationWidths      []int                       `json:"decoration_advance_widths"`
	DecorationWidthSource OfficeTextSource            `json:"decoration_width_table_source"`
}

// MenuAssets contains immutable source pixels and screen-local source tables.
// Controllers own choice/highlight/edit state; none is stored in these assets.
type MenuAssets struct {
	Root             string
	Images           map[string][]byte
	Screens          map[string]MenuScreen
	Fonts            map[int]*BitmapFont
	Palette          color.Palette
	Emblems          []MenuEmblem
	Theatres         []MenuTheatre
	FullRanks        []string
	Strings          map[string]string
	DecorationWidths []int
}

func menuPaletteAssociation(name, source string) (PaletteAssociation, bool) {
	var evidence, confidence string
	switch name {
	case "ROSTER.PIC", "PATCHES.PIC":
		evidence = "Step15/17 natural roster loading and all256 live DAC entries equal BASE.PAL at roster/office endpoints"
		confidence = "prior_runtime_palette_endpoints"
	case "MEMO.PIC":
		evidence = "Step18 all256 installed DAC entries equal BASE.PAL at natural mission chooser ready/cancel/commit boundaries"
		confidence = "prior_runtime_palette_endpoints"
	case "THEATER.PIC":
		// No THEATER.PAL is referenced: the statically recovered handler preserves
		// the office working palette. This is not an additional DAC observation.
		evidence = "Step19 T0026..015c and Step20 palette-order: no palette staging between office and THEATER; BASE inheritance is a static prediction, not a captured destination DAC"
		confidence = "static_palette_inheritance"
	default:
		return PaletteAssociation{}, false
	}
	return PaletteAssociation{source, officePaletteHash, evidence, confidence, false}, true
}

func parseMenuScreen(r *Resource) (MenuScreen, error) {
	s := MenuScreen{Name: r.name, Source: r.Source}
	chunks, err := officeChunks(r.Payload, r.Source.Offset)
	if err != nil {
		return s, err
	}
	if string(r.Payload[8:12]) != "SCRN" {
		return s, fmt.Errorf("%s is not FORM/SCRN", r.name)
	}
	s.Chunks = chunks
	for _, c := range chunks {
		b := c.Payload
		switch c.Tag {
		case "SCHD":
			if len(b) != 36 || s.Background != "" {
				return s, fmt.Errorf("%s duplicate or unsupported SCHD", r.name)
			}
			for i := range s.Basenames {
				s.Basenames[i] = OfficeCString(b[i*9 : i*9+9])
			}
			// MEMO's picture is in slot1, unlike THEATER/ROSTER's slot0.
			for _, name := range s.Basenames {
				if name != "" {
					s.Background = name + ".PIC"
					break
				}
			}
			s.InitialKey = OfficeSignedWord(b[31:])
		case "AREA":
			if len(b) < 9 || len(b) > 34 {
				return s, fmt.Errorf("%s unsupported AREA size %d", r.name, len(b))
			}
			// Original loader zero-fills its fixed record before this bounded copy.
			// Nine-byte areas omit the height high byte and all visual fields.
			var padded [34]byte
			copy(padded[:], b)
			a := OfficeArea{ChunkIndex: c.Index, Key: OfficeSignedWord(padded[:]), VisualFlag: le16(padded[10:]), Priority: len(s.Areas)}
			for i := range a.Rectangle {
				a.Rectangle[i] = OfficeSignedWord(padded[2+2*i:])
			}
			if !OfficeRectFits(a.Rectangle) {
				return s, fmt.Errorf("%s AREA%d outside image", r.name, a.Key)
			}
			a.InclusiveEnd = [2]int{a.Rectangle[0] + a.Rectangle[2] - 1, a.Rectangle[1] + a.Rectangle[3] - 1}
			s.Areas = append(s.Areas, a)
		case "LORI":
			if len(b) != 14 {
				return s, fmt.Errorf("%s unsupported LORI size", r.name)
			}
			asset := s.Basenames[1] + ".PIC"
			if s.Basenames[1] == "" {
				asset = s.Background
			}
			s.Lori = append(s.Lori, officeLori{ChunkIndex: c.Index, Key: int(int8(b[0])), Mode: int(b[1]), SourceAsset: asset, Base: [2]int{OfficeSignedWord(b[2:]), OfficeSignedWord(b[4:])}, Cell: [2]int{OfficeSignedWord(b[6:]), OfficeSignedWord(b[8:])}, Divisor: OfficeSignedWord(b[10:]), UnknownTail: OfficeSignedWord(b[12:])})
		case "LABL":
			// Preserve serialized label/style bytes; rendering does not assume that
			// this record is a host string or an ordinary bitmap-font declaration.
		default:
			return s, fmt.Errorf("%s unsupported menu chunk %q", r.name, c.Tag)
		}
	}
	if s.Background == "" {
		return s, fmt.Errorf("%s missing picture SCHD", r.name)
	}
	return s, nil
}

func menuEmblems(lori []officeLori) ([]MenuEmblem, error) {
	var large, small *officeLori
	for i := range lori {
		switch lori[i].Key {
		case 0:
			large = &lori[i]
		case 4:
			small = &lori[i]
		}
	}
	if large == nil || small == nil || large.Divisor <= 0 || small.Divisor <= 0 {
		return nil, fmt.Errorf("roster emblem LORI0/4 absent or invalid")
	}
	var result []MenuEmblem
	for selector := 0; selector < 18; selector++ {
		rect := func(l *officeLori) [4]int {
			return [4]int{l.Base[0] + selector/l.Divisor*l.Cell[0], l.Base[1] + selector%l.Divisor*l.Cell[1], l.Cell[0], l.Cell[1]}
		}
		e := MenuEmblem{selector, rect(large), rect(small)}
		if !OfficeRectFits(e.Source) || !OfficeRectFits(e.Small) {
			return nil, fmt.Errorf("emblem%d exceeds PATCHES.PIC", selector)
		}
		result = append(result, e)
	}
	return result, nil
}

func exportMenuMetadata(out *OutputWriter, alloc *pathAllocator, m *Manifest) error {
	if m.OfficeMetadata == "" {
		return nil // Exact profile is already reported by office metadata gating.
	}
	profile, err := recognizeOfficeProfile(m.Resources)
	if err != nil {
		return nil
	}
	doc := menuMetadata{Schema: menuMetadataSchema, Profile: profile.Name, Screens: map[string]MenuScreen{}, Strings: map[string]OfficeTextSource{}, Notes: []string{
		"ROSTER/PATCHES/MEMO BASE.PAL endpoints were observed naturally; THEATER palette inheritance is a static prediction, not new runtime validation.",
		"All picture indices are offline decoded. These sheets do not acquire BASE/BASE_AN's complete native-bank validation status.",
		"Source AREA/LORI/LABL bytes remain in bounded chunks; generated chooser options and controllers are separate application behavior.",
	}}
	for _, resource := range m.Resources {
		if resource.Source.PayloadSHA256 != profile.Source(RoleMainExecutable).SHA256 {
			continue
		}
		for selector := 0; selector < 12; selector++ {
			pointerDS := 0xc0e + selector*2 // Step14 original full-rank pointer table.
			pointer, e := officeSourceWord(resource.backing, pointerDS)
			if e != nil {
				return e
			}
			text, e := officeSourceString(resource.backing, pointer)
			if e != nil {
				return e
			}
			text.Selector, text.PointerFile = selector, officeDataAnchor+pointerDS
			doc.FullRanks = append(doc.FullRanks, text)
		}
		start := officeDataAnchor + 0x1a82
		if start < 0 || start+8 > len(resource.backing) {
			return fmt.Errorf("roster decoration table outside initialized source")
		}
		doc.DecorationWidthSource = OfficeTextSource{DS: 0x1a82, File: start, RawHex: hex.EncodeToString(resource.backing[start : start+8])}
		for _, v := range resource.backing[start : start+8] {
			doc.DecorationWidths = append(doc.DecorationWidths, int(v))
		}
		for key, ds := range menuSourceStrings {
			text, e := officeSourceString(resource.backing, ds)
			if e != nil {
				return e
			}
			doc.Strings[key] = text
		}
	}
	var roster *Resource
	for _, name := range []string{"ROSTER.SCN", "MEMO.SCN", "THEATER.SCN"} {
		for _, r := range m.Resources {
			if r.name != name || r.Source.SHA256 != profile.Source(roleScreenCatalog).SHA256 {
				continue
			}
			screen, e := parseMenuScreen(r)
			if e != nil {
				return e
			}
			doc.Screens[name] = screen
			if name == "ROSTER.SCN" {
				roster = r
				doc.Emblems, err = menuEmblems(screen.Lori)
				if err != nil {
					return err
				}
			}
		}
	}
	if roster == nil || len(doc.Screens) != 3 {
		return fmt.Errorf("supported profile missing required ROSTER/MEMO/THEATER screens")
	}
	for _, name := range []string{"ROSTER.PIC", "PATCHES.PIC", "MEMO.PIC", "THEATER.PIC"} {
		for _, r := range m.Resources {
			if r.name == name && r.Source.SHA256 == baseCatalogHash {
				if r.Conversion.Status != "converted_supported_colour" {
					return fmt.Errorf("required menu picture %s has no supported colour decode", name)
				}
				doc.Assets = append(doc.Assets, officePreparedAsset(r, false))
				break
			}
		}
	}
	if len(doc.Assets) != 4 {
		return fmt.Errorf("required menu picture missing")
	}
	for _, r := range m.Resources {
		if r.Source.PayloadSHA256 == originalFontHash {
			// Office metadata intentionally lists only fonts3/5; menus also need
			// font4. Unrelated font selectors are not menu runtime requirements.
			asset := officePreparedAsset(r, false)
			asset.Outputs = nil
			for _, output := range r.Outputs {
				for _, selector := range []int{3, 4, 5} {
					if output.Path == fmt.Sprintf("%s.font%d.mask.png", r.ExportPath, selector) || output.Path == fmt.Sprintf("%s.font%d.metrics.json", r.ExportPath, selector) {
						asset.Outputs = append(asset.Outputs, output)
					}
				}
			}
			doc.Fonts = append(doc.Fonts, asset)
		}
	}
	for selector := 0; selector < 4; selector++ {
		name := fmt.Sprintf("SCENE_%02d.THR", selector)
		for _, r := range m.Resources {
			if r.name != name || r.Source.SHA256 != profile.Source(roleScreenCatalog).SHA256 {
				continue
			}
			chunks, e := officeChunks(r.Payload, r.Source.Offset)
			if e != nil {
				return e
			}
			for _, c := range chunks {
				if c.Tag != "THHD" || len(c.Payload) != 54 {
					continue
				}
				b := c.Payload
				rect := [4]int{OfficeSignedWord(b[0x1c:]), OfficeSignedWord(b[0x1e:]), int(int8(b[0x24])), int(int8(b[0x25]))}
				if !OfficeRectFits(rect) {
					return fmt.Errorf("%s invalid selector rectangle", name)
				}
				doc.Theatres = append(doc.Theatres, MenuTheatre{selector, OfficeCString(b[1:21]), rect, r.Source, hex.EncodeToString(b)})
			}
		}
	}
	if len(doc.Theatres) != 4 {
		return fmt.Errorf("required theatre option headers missing")
	}
	data, err := JsonBytes(doc)
	if err != nil {
		return err
	}
	if err = addGenerated(out, roster, alloc, ".menuui.json", "front_end_menu_metadata", data); err != nil {
		return err
	}
	m.MenuMetadata = roster.Outputs[len(roster.Outputs)-1].Path
	return nil
}

func LoadMenuAssets(directory string) (*MenuAssets, error) {
	reader, err := newPreparedReader(directory)
	if err != nil {
		return nil, err
	}
	defer reader.close()
	return reader.loadMenu()
}
func (reader *preparedReader) loadMenu() (*MenuAssets, error) {
	raw, err := reader.read("manifest.json", 64<<20)
	if err != nil {
		return nil, err
	}
	var manifest Manifest
	if err = json.Unmarshal(raw, &manifest); err != nil {
		return nil, fmt.Errorf("menu manifest: %w", err)
	}
	if !compatiblePreparation(manifest) || manifest.MenuMetadata == "" {
		return nil, fmt.Errorf("prepared assets lack Step C menu outputs; rebuild/upgrade from the supported original installation")
	}
	for _, r := range manifest.Resources {
		if r == nil {
			return nil, fmt.Errorf("menu manifest contains null resource")
		}
		for _, o := range r.Outputs {
			if prior, exists := reader.outputs[o.Path]; exists && prior != o {
				return nil, fmt.Errorf("conflicting menu output %q", o.Path)
			}
			reader.outputs[o.Path] = o
		}
	}
	meta, ok := reader.outputs[manifest.MenuMetadata]
	if !ok || meta.Kind != "front_end_menu_metadata" {
		return nil, fmt.Errorf("menu metadata is not a manifest output")
	}
	raw, err = reader.verified(meta, 4<<20)
	if err != nil {
		return nil, err
	}
	var doc menuMetadata
	if err = json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("menu metadata: %w", err)
	}
	if err = validateMenuMetadata(doc); err != nil {
		return nil, err
	}
	a := &MenuAssets{Root: reader.root, Images: map[string][]byte{}, Screens: doc.Screens, Fonts: map[int]*BitmapFont{}, Emblems: doc.Emblems, Theatres: doc.Theatres, Strings: map[string]string{}, DecorationWidths: doc.DecorationWidths}
	for _, rank := range doc.FullRanks {
		a.FullRanks = append(a.FullRanks, rank.Text)
	}
	for key, value := range doc.Strings {
		a.Strings[key] = value.Text
	}
	for _, resource := range doc.Assets {
		o, e := preparedOutput(resource.Outputs, "decoded_indices")
		if e != nil {
			return nil, e
		}
		pixels, e := reader.verified(o, 64000)
		if e != nil {
			return nil, e
		}
		if len(pixels) != 64000 || Hash(pixels) != resource.PixelSHA256 || resource.Palette.Diagnostic || resource.Palette.SHA256 != officePaletteHash {
			return nil, fmt.Errorf("menu picture %s requires matching indices and supported BASE palette", resource.Name)
		}
		a.Images[resource.Name] = pixels
	}
	for _, resource := range manifest.Resources {
		if resource.ExportPath != doc.Assets[0].Palette.Source {
			continue
		}
		if resource.Source.PayloadSHA256 != officePaletteHash {
			return nil, fmt.Errorf("menu palette payload identity differs")
		}
		o, e := preparedOutput(resource.Outputs, "exact_rgb6_components")
		if e != nil {
			return nil, e
		}
		components, e := reader.verified(o, 768)
		if e != nil || len(components) != 768 {
			return nil, fmt.Errorf("menu palette requires 768 verified RGB6 bytes: %v", e)
		}
		a.Palette = make(color.Palette, 256)
		for index := range a.Palette {
			rgb := components[index*3 : index*3+3]
			if rgb[0] > 63 || rgb[1] > 63 || rgb[2] > 63 {
				return nil, fmt.Errorf("menu palette component exceeds RGB6")
			}
			a.Palette[index] = color.NRGBA{rgb[0]<<2 | rgb[0]>>4, rgb[1]<<2 | rgb[1]>>4, rgb[2]<<2 | rgb[2]>>4, 255}
		}
	}
	if len(a.Palette) != 256 {
		return nil, fmt.Errorf("menu BASE palette output missing")
	}
	for _, fontResource := range doc.Fonts {
		for _, o := range fontResource.Outputs {
			if o.Kind != "bitmap_font_metrics" {
				continue
			}
			raw, e := reader.verified(o, 1<<20)
			if e != nil {
				return nil, e
			}
			var metrics FontMetrics
			if e = json.Unmarshal(raw, &metrics); e != nil {
				return nil, e
			}
			if metrics.Selector < 3 || metrics.Selector > 5 {
				continue
			}
			font, e := loadMenuFont(reader, metrics)
			if e != nil {
				return nil, e
			}
			if a.Fonts[metrics.Selector] != nil {
				return nil, fmt.Errorf("duplicate menu font%d", metrics.Selector)
			}
			a.Fonts[metrics.Selector] = font
		}
	}
	for _, selector := range []int{3, 4, 5} {
		if a.Fonts[selector] == nil {
			return nil, fmt.Errorf("menu font%d missing", selector)
		}
	}
	return a, nil
}

func loadMenuFont(reader *preparedReader, metrics FontMetrics) (*BitmapFont, error) {
	o, ok := reader.outputs[metrics.Atlas]
	if !ok || o.Kind != "bitmap_font_mask_atlas" {
		return nil, fmt.Errorf("menu font%d mask missing", metrics.Selector)
	}
	raw, err := reader.verified(o, 1<<20)
	if err != nil {
		return nil, err
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(raw))
	if err != nil || cfg.Width != metrics.AtlasSize[0] || cfg.Height != metrics.AtlasSize[1] || cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > 4096 || cfg.Height > 4096 || cfg.Width*cfg.Height > 1<<20 {
		return nil, fmt.Errorf("menu font%d invalid atlas dimensions", metrics.Selector)
	}
	decoded, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	mask, ok := decoded.(*image.Paletted)
	if !ok {
		return nil, fmt.Errorf("menu font%d mask is not indexed", metrics.Selector)
	}
	return NewBitmapFont(metrics, mask)
}

func validateMenuMetadata(doc menuMetadata) error {
	if doc.Schema != menuMetadataSchema || doc.Profile != IslandIce469085.Name || len(doc.Screens) != 3 || len(doc.Assets) != 4 || len(doc.Theatres) != 4 || len(doc.Emblems) != 18 || len(doc.FullRanks) != 12 || len(doc.Strings) != len(menuSourceStrings) {
		return fmt.Errorf("incompatible or incomplete menu metadata; rebuild/upgrade for C.1 roster and recovery presentation")
	}
	for key := range menuSourceStrings {
		if _, ok := doc.Strings[key]; !ok {
			return fmt.Errorf("menu source string %s missing", key)
		}
	}
	if len(doc.DecorationWidths) != 8 {
		return fmt.Errorf("roster decoration table missing; rebuild C.1 assets")
	}
	for _, width := range doc.DecorationWidths {
		if width < 1 || width > 34 {
			return fmt.Errorf("invalid decoration advance")
		}
	}
	for name, picture := range map[string]string{"ROSTER.SCN": "ROSTER.PIC", "MEMO.SCN": "MEMO.PIC", "THEATER.SCN": "THEATER.PIC"} {
		s, ok := doc.Screens[name]
		if !ok || s.Name != name || s.Background != picture {
			return fmt.Errorf("missing/invalid menu screen %s", name)
		}
		for _, a := range s.Areas {
			if !OfficeRectFits(a.Rectangle) {
				return fmt.Errorf("menu %s invalid AREA%d", name, a.Key)
			}
		}
	}
	for i, e := range doc.Emblems {
		if e.Selector != i || !OfficeRectFits(e.Source) || !OfficeRectFits(e.Small) {
			return fmt.Errorf("invalid emblem%d", i)
		}
	}
	for i, theatre := range doc.Theatres {
		if theatre.Selector != i || theatre.Name == "" || !OfficeRectFits(theatre.Rectangle) {
			return fmt.Errorf("invalid theatre%d", i)
		}
	}
	seen := map[string]bool{}
	for _, resource := range doc.Assets {
		if _, supported := menuPaletteAssociation(resource.Name, ""); !supported || seen[resource.Name] {
			return fmt.Errorf("duplicate or unknown menu image %s", resource.Name)
		}
		seen[resource.Name] = true
	}
	return nil
}
