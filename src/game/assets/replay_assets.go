package assets

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"gunship2000-assets/game/cockpit"
	"image/color"
)

const replayMetadataSchema = 1

// ReplayRegion uses the viewer's strict interior bounds. GS front-end AREA
// rectangles deliberately keep their separate inclusive pixel convention.
type ReplayRegion struct {
	X1, Y1, X2, Y2 int
	Command        uint16
}

func (r ReplayRegion) Contains(x, y int) bool { return r.X1 < x && x < r.X2 && r.Y1 < y && y < r.Y2 }

type ReplayStyle struct {
	FrameStyle, FrameFill                                                                   int
	TitleInset                                                                              [2]int
	TitleFont, TitleInk, ListFont, ListInk, SelectedInk, ListClear, DetailFont, DetailClear int
	// Clear values are historical style operands, not palette fill indices.
	ListFrameStyle, DetailFrameStyle, ScrollFrameStyle, ThumbFrameStyle int
	PointerBounds                                                       [4]int
	ClockMinutes, ClockSeconds                                          [2]int
	ClockStyle                                                          uint16
	HighlightOld, HighlightNew                                          byte
}

type replayMetadata struct {
	Schema                                int
	Profile                               string
	Browser                               MenuScreen
	BrowserLabels                         map[int]string
	BrowserImage, ViewerPanel             Output
	BrowserPalette                        PaletteAssociation
	BrowserPixelSHA256, ViewerPixelSHA256 string
	ViewerRegions                         []ReplayRegion
	ViewerPointer                         []byte
	Text                                  map[string]string
	Styles                                ReplayStyle
	Sources                               []SourceLocation
	Notes                                 []string
}

// ReplayAssets is immutable source-derived display data. Film files and mutable
// browser/viewer choices belong to controllers, never to prepared assets.
type ReplayAssets struct {
	Browser                     MenuScreen
	BrowserLabels               map[int]string
	BrowserImage, ViewerPanel   []byte
	Palette                     color.Palette
	Fonts                       map[int]*BitmapFont
	PointerAtlas, ViewerPointer []byte
	ViewerRegions               []ReplayRegion
	Text                        map[string]string
	Styles                      ReplayStyle
}

func (s MenuScreen) Area(key int) (OfficeArea, bool) {
	for _, a := range s.Areas {
		if a.Key == key {
			return a, true
		}
	}
	return OfficeArea{}, false
}
func (a *ReplayAssets) RegionHit(x, y int) int {
	if a != nil {
		for i, r := range a.ViewerRegions {
			if r.Contains(x, y) {
				return i
			}
		}
	}
	return -1
}

// Source196a changes only exact palette indices inside the source rectangle;
// active controls are not an invented solid fill or new beveled button image.
func (a *ReplayAssets) ReplaceRegionIndices(dst []byte, index int, old, new byte) {
	if a == nil || !cockpit.LogicalTarget(dst) || index < 0 || index >= len(a.ViewerRegions) {
		return
	}
	r := a.ViewerRegions[index]
	for y := r.Y1; y < r.Y2; y++ {
		for x := r.X1; x < r.X2; x++ {
			p := y*320 + x
			if dst[p] == old {
				cockpit.PutLogical(dst, p, new)
			}
		}
	}
}
func (a *ReplayAssets) DrawViewerPointer(dst []byte, x, y int) {
	if a == nil || !cockpit.LogicalTarget(dst) || len(a.ViewerPointer) != 100 {
		return
	}
	for dy := 0; dy < 10; dy++ {
		for dx := 0; dx < 10; dx++ {
			v := a.ViewerPointer[10*dy+dx]
			px, py := x+dx, y+dy
			if v != 0 && px >= 0 && px < 320 && py >= 0 && py < 200 {
				cockpit.PutLogical(dst, py*320+px, v)
			}
		}
	}
}

// REPLAY has one zero-height scrollbar on disk. Only that exact carrier is
// admitted; broadening shared menu rectangle validation would change other UIs.
func parseReplayScreen(r *Resource) (MenuScreen, map[int]string, error) {
	var s MenuScreen
	labels := map[int]string{}
	if r == nil || Hash(r.Payload) != "5af0b3fa6efaab1489f70333a1bf00ba62af9267b71ba5cf47a4a2f445a16db5" {
		return s, nil, fmt.Errorf("replay screen profile mismatch")
	}
	chunks, e := officeChunks(r.Payload, r.Source.Offset)
	if e != nil {
		return s, nil, e
	}
	s = MenuScreen{Name: r.name, Background: "REPLAY.PIC", Basenames: [3]string{"", "REPLAY", ""}, Source: r.Source, Chunks: chunks}
	for _, c := range chunks {
		b := c.Payload
		switch c.Tag {
		case "SCHD":
			if len(b) != 36 {
				return s, nil, fmt.Errorf("replay SCHD")
			}
			s.InitialKey = OfficeSignedWord(b[31:])
		case "AREA":
			if len(b) < 9 || len(b) > 34 {
				return s, nil, fmt.Errorf("replay AREA width")
			}
			var p [34]byte
			copy(p[:], b)
			a := OfficeArea{Key: OfficeSignedWord(p[:]), ChunkIndex: c.Index, VisualFlag: le16(p[10:]), Priority: len(s.Areas)}
			for i := range a.Rectangle {
				a.Rectangle[i] = OfficeSignedWord(p[2+2*i:])
			}
			if a.Key != 14 && !OfficeRectFits(a.Rectangle) || a.Key == 14 && a.Rectangle != [4]int{299, 14, 15, 0} {
				return s, nil, fmt.Errorf("replay AREA%d bounds", a.Key)
			}
			a.InclusiveEnd = [2]int{a.Rectangle[0] + a.Rectangle[2] - 1, a.Rectangle[1] + a.Rectangle[3] - 1}
			s.Areas = append(s.Areas, a)
			if len(b) > 12 {
				labels[a.Key] = OfficeCString(b[12:])
			}
		}
	}
	if len(s.Areas) != 21 {
		return s, nil, fmt.Errorf("replay requires21 areas")
	}
	return s, labels, nil
}

var replayBrowserStrings = map[string]int{"title": 0x245e, "export_unavailable": 0x247e, "original_score": 0x24a8, "original_score_format": 0x24b8, "resumed_score": 0x24bb, "resumed_score_format": 0x24cb, "last_mission": 0x250e, "delete_prompt": 0x485}
var replayViewerStrings = map[string]int{"rescued_by": 0x21c6, "and": 0x21d3, "secondary_achieved": 0x21d9, "primary_achieved": 0x21ed, "goes_in": 0x21ff, "takes_hit": 0x2208, "destroyed_by": 0x2218, "event_time": 0x2227, "altitude": 0x2237, "heading": 0x2242, "airspeed": 0x224c, "time_separator": 0x2153}

func prepareReplayMetadata(m *Manifest, p *AssetVersionProfile) (replayMetadata, *Resource, error) {
	var d replayMetadata
	res, gs := missionResources(m)
	screen := res["REPLAY.SCN"]
	if gs == nil || !p.SourceMatches(RoleMainExecutable, Hash(gs.Payload)) {
		return d, nil, fmt.Errorf("replay GS source identity")
	}
	var sim *Resource
	for _, r := range m.Resources {
		if r.Source.Kind == "loose_file" && p.SourceMatches(RoleSimulationExecutable, r.Source.PayloadSHA256) {
			sim = r
			break
		}
	}
	if sim == nil || p.CockpitLayout == nil {
		return d, nil, fmt.Errorf("replay GS2 source identity")
	}
	browser, labels, e := parseReplayScreen(screen)
	if e != nil {
		return d, nil, e
	}
	d = replayMetadata{Schema: replayMetadataSchema, Profile: p.Name, Browser: browser, BrowserLabels: labels, Text: map[string]string{}, Sources: []SourceLocation{screen.Source, gs.Source, sim.Source}, Styles: ReplayStyle{FrameStyle: 0, FrameFill: 0, TitleInset: [2]int{6, 4}, TitleFont: 5, TitleInk: 2, ListFont: 5, ListInk: 2, SelectedInk: 15, ListClear: 8, DetailFont: 3, DetailClear: 6, ListFrameStyle: 8, DetailFrameStyle: 6, ScrollFrameStyle: 6, ThumbFrameStyle: 1, PointerBounds: [4]int{0, 168, 300, 190}, ClockMinutes: [2]int{197, 173}, ClockSeconds: [2]int{209, 173}, ClockStyle: 0x0602, HighlightOld: 12, HighlightNew: 7}, Notes: []string{"GS catalog X0 REPLAY.PIC/BASE.PAL and separate loose GS2 F5 viewer panel are different carriers.", "Original browser header writes are not part of preparation or native browsing.", "VCR source196a is exact-index replacement; viewer regions are strict interior, separate from GS AREA hit tests."}}
	for key, ds := range replayBrowserStrings {
		v, e := officeSourceString(gs.Payload, ds)
		if e != nil {
			return d, nil, e
		}
		d.Text[key] = v.Text
	}
	// Section3 is a source-relative locator, not a historical live CS mapping.
	span, _, e := cockpitSourceBytes(sim.Payload, p.CockpitLayout, 3, 0x1975, 0x20e7, 0x2464-0x20e7)
	if e != nil {
		return d, nil, e
	}
	source := func(ip, n int) []byte { return span[ip-0x20e7 : ip-0x20e7+n] }
	if !bytes.Equal(source(0x215e, 4), []byte{10, 0, 10, 0}) {
		return d, nil, fmt.Errorf("replay source pointer header")
	}
	d.ViewerPointer = bytes.Clone(source(0x2162, 100))
	for i := 0; i < 18; i++ {
		b := source(0x226d+10*i, 10)
		d.ViewerRegions = append(d.ViewerRegions, ReplayRegion{OfficeSignedWord(b), OfficeSignedWord(b[2:]), OfficeSignedWord(b[4:]), OfficeSignedWord(b[6:]), binary.LittleEndian.Uint16(b[8:])})
	}
	for key, ip := range replayViewerStrings {
		b := source(ip, 512)
		n := bytes.IndexByte(b, 0)
		if n < 0 {
			return d, nil, fmt.Errorf("replay viewer string%s", key)
		}
		d.Text[key] = string(b[:n])
	}
	// These literals belong to two other source banks, not the loose viewer
	// panel's section3 text neighborhood. Preserve those distinct locators.
	for key, loc := range map[string]cockpitSourceSlice{
		"mission_score":  {Section: 0, CS: 0x975, IP: 0x184a, Length: 16},
		"time_taken":     {Section: 0, CS: 0x975, IP: 0x183d, Length: 13},
		"total_kills":    {Section: 0, CS: 0x975, IP: 0x182f, Length: 14},
		"demo_title":     {Section: 8, CS: 0x2a29, IP: 0x248, Length: 24},
		"demo_copyright": {Section: 8, CS: 0x2a29, IP: 0x260, Length: 40},
	} {
		b, _, err := cockpitSourceBytes(sim.Payload, p.CockpitLayout, loc.Section, loc.CS, loc.IP, loc.Length)
		if err != nil {
			return d, nil, err
		}
		if b[len(b)-1] != 0 || bytes.IndexByte(b[:len(b)-1], 0) >= 0 {
			return d, nil, fmt.Errorf("replay literal %s framing", key)
		}
		d.Text[key] = string(b[:len(b)-1])
	}
	for i := 1; i <= 11; i++ {
		ip := int(binary.LittleEndian.Uint16(source(0x2255+2*i, 2)))
		b := source(ip, 100)
		n := bytes.IndexByte(b, 0)
		if n < 0 {
			return d, nil, fmt.Errorf("replay subsystem string")
		}
		d.Text[fmt.Sprintf("damage_%d", i)] = string(b[:n])
	}
	image := res["REPLAY.PIC"]
	if image == nil || image.Source.PayloadSHA256 != "601f68ffe9d349f56b4fa591020cb54f140cf5ede7220411b80dba10923f6065" {
		return d, nil, fmt.Errorf("replay catalog image identity")
	}
	d.Sources = append(d.Sources, image.Source)
	base := res["BASE.PAL"]
	if base == nil || base.Source.PayloadSHA256 != officePaletteHash {
		return d, nil, fmt.Errorf("replay BASE source identity")
	}
	d.Sources = append(d.Sources, base.Source)
	d.BrowserPalette = PaletteAssociation{Source: base.ExportPath, SHA256: officePaletteHash, Evidence: "GS section0 1c07:00c0 explicitly queries BASE for catalog REPLAY browser", Confidence: "static_source_palette_caller"}
	d.BrowserImage, e = preparedOutput(image.Outputs, "decoded_indices")
	if e != nil {
		return d, nil, e
	}
	d.BrowserPixelSHA256 = "45540cbd4a1340e4527f152e92d7d4417edc753b5c1391bdc71ef0ad65d4f247"
	for _, r := range m.Resources {
		for _, o := range r.Outputs {
			if o.Path == "world/cockpit/images/REPLAY.PIC.indices" && o.Kind == "cockpit_indexed_pixels" {
				d.ViewerPanel = o
				d.ViewerPixelSHA256 = o.SHA256
			}
		}
	}
	return d, screen, validateReplayMetadata(d, true)
}

func validateReplayMetadata(d replayMetadata, outputs bool) error {
	if d.Schema != replayMetadataSchema || d.Profile != IslandIce469085.Name || len(d.Browser.Areas) != 21 || len(d.ViewerRegions) != 18 || len(d.ViewerPointer) != 100 || d.Text["title"] != "MISSION FILMS" || d.Text["primary_achieved"] != "PRIMARY achieved!" || d.Text["secondary_achieved"] != "SECONDARY achieved!" || d.Text["damage_11"] != "no damage" {
		return fmt.Errorf("incomplete or incompatible replay metadata")
	}
	for i, r := range d.ViewerRegions {
		if r.X1 < 0 || r.Y1 < 0 || r.X2 <= r.X1 || r.Y2 <= r.Y1 || r.X2 > 320 || r.Y2 > 200 || r.Command == 0 {
			return fmt.Errorf("invalid viewer region%d", i)
		}
	}
	if d.BrowserPalette.Diagnostic || d.BrowserPalette.SHA256 != officePaletteHash {
		return fmt.Errorf("replay BASE palette identity")
	}
	if outputs && (d.BrowserImage.Path == "" || d.ViewerPanel.Path == "" || d.BrowserImage.SHA256 != d.BrowserPixelSHA256 || d.ViewerPanel.SHA256 != d.ViewerPixelSHA256) {
		return fmt.Errorf("replay pixel outputs missing or inconsistent")
	}
	return nil
}

// Catalog images may consume one independently retained backing byte during
// odd-word refill. Reuse the established resource envelope; never append zero.
func replayPicturePixels(r *Resource) ([]byte, error) {
	_, pos, end, e := pictureHeader(r.Payload)
	if e != nil {
		return nil, e
	}
	begin, limit := int(r.Source.Offset)+pos, int(r.Source.Offset)+end
	if begin < 0 || limit > len(r.backing) {
		return nil, fmt.Errorf("replay picture missing catalog backing")
	}
	if limit < len(r.backing) {
		limit++
	}
	pixels, _, e := decodePicture(r.backing[begin:limit])
	return pixels, e
}

func exportReplayMetadata(out *OutputWriter, alloc *pathAllocator, m *Manifest) error {
	if m.OfficeMetadata == "" {
		return nil
	}
	p, e := recognizeOfficeProfile(m.Resources)
	if e != nil {
		return nil
	}
	d, r, e := prepareReplayMetadata(m, p)
	if e != nil {
		return e
	}
	resources, _ := missionResources(m)
	image, palette := resources["REPLAY.PIC"], resources["BASE.PAL"]
	pixels, e := replayPicturePixels(image)
	if e != nil {
		return e
	}
	palettes, _, _, e := pictureHeader(palette.Payload)
	if e != nil || len(palettes) != 1 || len(palettes[0].RGB6) != 768 {
		return fmt.Errorf("replay palette components: %v", e)
	}
	png, e := contextPNG(pixels, 320, 200, palettes[0].RGB6)
	if e != nil {
		return e
	}
	if e = addPalettedGenerated(out, image, alloc, ".BASE-replay.png", "replay_browser_context_colour_png", png, d.BrowserPalette); e != nil {
		return e
	}
	raw, e := JsonBytes(d)
	if e != nil {
		return e
	}
	if e = addGenerated(out, r, alloc, ".replay.json", "mission_replay_metadata", raw); e != nil {
		return e
	}
	m.ReplayMetadata = r.Outputs[len(r.Outputs)-1].Path
	return nil
}

func loadReplayAssets(reader *preparedReader, m Manifest, office *OfficeAssets) (*ReplayAssets, error) {
	if m.ReplayMetadata == "" && m.PreparationVersion < 24 {
		return nil, nil
	}
	o, ok := reader.outputs[m.ReplayMetadata]
	if !ok || o.Kind != "mission_replay_metadata" {
		return nil, fmt.Errorf("prepared assets lack StepO film metadata; prepare24")
	}
	raw, e := reader.verified(o, 2<<20)
	if e != nil {
		return nil, e
	}
	var d replayMetadata
	if e = DecodeObject(raw, &d); e != nil {
		return nil, e
	}
	if e = validateReplayMetadata(d, true); e != nil {
		return nil, e
	}
	if m.PreparationVersion >= 25 {
		for name, want := range map[string]string{"mission_score": "MISSION SCORE: ", "time_taken": "TIME TAKEN: ", "total_kills": "TOTAL KILLS: ", "demo_title": "GUNSHIP 2000: DEMO MODE", "demo_copyright": "Copyright 1991 MicroProse Software Inc."} {
			if d.Text[name] != want {
				return nil, fmt.Errorf("prepared replay literal %s missing/incompatible; prepare25", name)
			}
		}
	}
	if office == nil || office.Menus == nil || len(office.Palette) != 256 || len(office.Images["POINTER.PIC"]) != 64000 {
		return nil, fmt.Errorf("replay needs existing BASE/fonts/pointer owners")
	}
	a := &ReplayAssets{Browser: d.Browser, BrowserLabels: d.BrowserLabels, Palette: office.Palette, Fonts: office.Menus.Fonts, PointerAtlas: office.Images["POINTER.PIC"], ViewerPointer: d.ViewerPointer, ViewerRegions: d.ViewerRegions, Text: d.Text, Styles: d.Styles}
	if a.BrowserImage, e = reader.verified(d.BrowserImage, 64000); e != nil {
		return nil, e
	}
	if a.ViewerPanel, e = reader.verified(d.ViewerPanel, 64000); e != nil {
		return nil, e
	}
	if len(a.BrowserImage) != 64000 || len(a.ViewerPanel) != 64000 || Hash(a.BrowserImage) != d.BrowserPixelSHA256 || Hash(a.ViewerPanel) != d.ViewerPixelSHA256 {
		return nil, fmt.Errorf("replay image dimensions/identity")
	}
	return a, nil
}
