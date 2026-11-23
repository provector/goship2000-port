package assets

import (
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
)

type FontGlyph struct {
	Byte              int    `json:"character_byte"`
	ASCII             string `json:"ascii,omitempty"`
	Rect              [4]int `json:"atlas_rect_xywh"`
	Width             int    `json:"width_before_spacing"`
	Advance           int    `json:"advance"`
	Source            int    `json:"first_row_file_offset"`
	SourceRowStep     int    `json:"source_row_stride"`
	AdvanceBeyondCell bool   `json:"advance_exceeds_stored_cell"`
}

// Font masks retain the complete stored cell, including the optional extra row.
// Width/advance and the selected rendering flags determine the drawn subset.
type FontMetrics struct {
	SchemaVersion   int         `json:"schema_version"`
	Selector        int         `json:"selector"`
	Source          string      `json:"source"`
	SourceSHA256    string      `json:"source_sha256"`
	Atlas           string      `json:"atlas"`
	AtlasSize       [2]int      `json:"atlas_dimensions"`
	AtlasMaskSHA256 string      `json:"atlas_mask_sha256"`
	HeaderHex       string      `json:"header_raw_hex"`
	WidthRange      [2]int      `json:"width_table_file_interval"`
	HeaderRange     [2]int      `json:"header_file_interval"`
	BitmapRange     [2]int      `json:"bitmap_storage_file_interval"`
	First           int         `json:"first_character_byte"`
	Last            int         `json:"last_character_byte"`
	RowBytes        int         `json:"bytes_per_glyph_row"`
	RowStride       int         `json:"source_row_stride"`
	Height          int         `json:"nominal_height"`
	Spacing         int         `json:"intercharacter_spacing"`
	Extra           int         `json:"optional_extra_rows"`
	Fixed           int         `json:"fixed_width_zero_means_table"`
	Unknown         int         `json:"header_byte7_unknown"`
	Baseline        *int        `json:"baseline"`
	LineAdvance     *int        `json:"line_advance"`
	Rules           []string    `json:"rendering_rules"`
	Evidence        []string    `json:"evidence"`
	Validation      string      `json:"validation_scope"`
	Glyphs          []FontGlyph `json:"glyphs"`
}

func parseOfficeFont(source []byte, selector int) (FontMetrics, *image.Paletted, error) {
	var m FontMetrics
	if len(source) < 2 {
		return m, nil, fmt.Errorf("truncated font count")
	}
	count := le16(source)
	if count > 256 || 2+2*count > len(source) || selector < 1 || selector > count {
		return m, nil, fmt.Errorf("invalid font directory or selector %d", selector)
	}
	bitmap := le16(source[2*selector:])
	if bitmap < 8 || bitmap > len(source) {
		return m, nil, fmt.Errorf("font%d header outside source", selector)
	}
	h := source[bitmap-8 : bitmap]
	first, last := int(h[0]), int(h[1])
	if last < first || (h[2] != 1 && h[2] != 2) || h[4] == 0 || h[4] >= 128 {
		return m, nil, fmt.Errorf("font%d unsupported header %x", selector, h)
	}
	n := last - first + 1
	rowBytes := 1 << (h[2] - 1)
	stride := n * rowBytes
	height, extra := int(h[4]), int(h[6])
	storedRows := height + extra
	widthStart := bitmap - 8 - n
	end := bitmap + storedRows*stride
	if widthStart < 2+2*count || storedRows > 256 || end > len(source) {
		return m, nil, fmt.Errorf("font%d truncated widths/bitmap including extra rows", selector)
	}
	cellW, cellH := rowBytes*8, storedRows
	columns := 16
	img := image.NewPaletted(image.Rect(0, 0, columns*cellW, ((n+columns-1)/columns)*cellH), color.Palette{color.NRGBA{0, 0, 0, 0}, color.NRGBA{255, 255, 255, 255}})
	m = FontMetrics{
		SchemaVersion: 1, Selector: selector, SourceSHA256: Hash(source), HeaderHex: hex.EncodeToString(h),
		WidthRange: [2]int{widthStart, bitmap - 8}, HeaderRange: [2]int{bitmap - 8, bitmap}, BitmapRange: [2]int{bitmap, end},
		First: first, Last: last, RowBytes: rowBytes, RowStride: stride, Height: height, Spacing: int(h[5]), Extra: extra, Fixed: int(h[3]), Unknown: int(h[7]),
		AtlasSize: [2]int{img.Rect.Dx(), img.Rect.Dy()},
		Rules: []string{
			"Mask indices: 0=no foreground write, 1=foreground; recolour set bits to the caller's palette index. Atlas transparency is font-specific, not a picture-wide index0 rule.",
			"Stored rows are glyph bands; each glyph row is most-significant-bit first. Full cells preserve raw row bits; width/advance metadata limits the drawn subset.",
			"For one-byte glyph rows the native MOV word also reads the adjacent byte. An advance exceeding the stored cell is flagged per glyph; do not invent a blank spacing bit or claim these other-font render cases validated.",
			"Advance=(fixed width if nonzero, otherwise width-table byte)+spacing. Nominal height is header byte4; flags bit0 adds header byte6 rows.",
			"Observed transparent flags0 path leaves zero bits and intercharacter spacing unchanged; last glyph follows the full-advance branch. Flags exactly1 writes foreground/background over the full advance.",
			"Text is NUL-terminated byte data. Bytes with bit7 set select foreground=byte&127; supported low-bit character range uses the glyph table, other bytes are skipped.",
			"Coordinates refer to glyph top-left, not a recovered typographic baseline. Baseline and automatic line advance are unestablished (null).",
			"Step13 flags0 office caption at Y193 is clipped to seven rows by inclusive bottom199. Native routines also have left/right/top clipping branches; no new broad clipping compatibility claim is made by this export.",
		},
		Evidence:   []string{"Step13 MGRAPHIC offset1361..13c3 directory/header/stride setup", "Step13 MGRAPHIC offset1011..1098 widths, MSB-first transparent glyphs and advance", "Step13 MGRAPHIC offset109b..10bc opaque foreground/background path", "Step13 MGRAPHIC offsetfb4..fc3 and ffb..1011 control bytes", "Step13 MGRAPHIC offset12d4..130b inclusive vertical clipping"},
		Validation: "Source-backed structural export; only font5 office text has prior native rendering validation. Other selectors share established storage but are not universally runtime-validated.",
	}
	for i := 0; i < n; i++ {
		width := m.Fixed
		if width == 0 {
			width = int(source[widthStart+i])
		}
		if width > cellW {
			return FontMetrics{}, nil, fmt.Errorf("font%d glyph%d width exceeds stored row bits", selector, first+i)
		}
		x, y := (i%columns)*cellW, (i/columns)*cellH
		g := FontGlyph{Byte: first + i, Rect: [4]int{x, y, cellW, cellH}, Width: width, Advance: width + m.Spacing, Source: bitmap + i*rowBytes, SourceRowStep: stride, AdvanceBeyondCell: width+m.Spacing > cellW}
		if g.Byte >= 32 && g.Byte <= 126 {
			g.ASCII = string(rune(g.Byte))
		}
		m.Glyphs = append(m.Glyphs, g)
		for row := 0; row < storedRows; row++ {
			pos := g.Source + row*stride
			for bit := 0; bit < cellW; bit++ {
				img.SetColorIndex(x+bit, y+row, (source[pos+bit/8]>>uint(7-bit%8))&1)
			}
		}
	}
	m.AtlasMaskSHA256 = Hash(img.Pix)
	return m, img, nil
}

// Exact identity gate prevents treating unrelated GS2 files as this font layout.
// The complete original file remains an independently extracted output.
func exportOfficeFonts(writer *OutputWriter, r *Resource, a *pathAllocator) (bool, error) {
	if !IslandIce469085.SourceMatches(roleBitmapFonts, Hash(r.Payload)) {
		return false, nil
	}
	for selector := 1; selector <= le16(r.Payload); selector++ {
		metrics, atlas, err := parseOfficeFont(r.Payload, selector)
		if err != nil {
			return true, err
		}
		metrics.Source = r.ExportPath
		data, err := PngBytes(atlas)
		if err != nil {
			return true, err
		}
		if err = addGenerated(writer, r, a, fmt.Sprintf(".font%d.mask.png", selector), "bitmap_font_mask_atlas", data); err != nil {
			return true, err
		}
		metrics.Atlas = r.Outputs[len(r.Outputs)-1].Path
		data, err = JsonBytes(metrics)
		if err != nil {
			return true, err
		}
		if err = addGenerated(writer, r, a, fmt.Sprintf(".font%d.metrics.json", selector), "bitmap_font_metrics", data); err != nil {
			return true, err
		}
	}
	return true, nil
}
