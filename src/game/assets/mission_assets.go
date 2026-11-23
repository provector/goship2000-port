package assets

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const missionMetadataSchema = 1

var missionRequiredPictures = []string{"TENT.PIC", "SHIP.PIC", "ICE.PIC", "FIELD.PIC", "FIELD2.PIC", "FIELD3.PIC", "FIELD_AN.PIC", "FIELD_A2.PIC", "COMMANDR.PIC", "COMMAND2.PIC", "HANDS.PIC", "PAGE.PIC", "PAGE2.PIC", "PAGE3.PIC", "REALITY.PIC", "REALI_AN.PIC", "REAL_PCH.PIC"}

type CommanderPose struct {
	Index      int
	Triple     [3]int
	Body, Arms [4]int
}

type MissionScreen struct {
	Name      string
	Basenames [3]string
	Chunks    []OfficeChunk
	Areas     []OfficeArea
	Lori      []officeLori
}

type MissionPresentationData struct {
	Poses          []CommanderPose
	Transitions    map[string][]int
	Templates      map[int]string
	CampaignStatus []string `json:",omitempty"`
	Screens        map[string]MissionScreen
}

type missionPreparedImage struct {
	Asset         officeAsset
	Palette       string
	Width, Height int
}

type missionPreparedPalette struct {
	Name       string
	Source     SourceLocation
	Components Output
}

type MissionMetadata struct {
	Schema              int
	Profile             string
	Generation          MissionGenerationData
	Presentation        MissionPresentationData
	Images              []missionPreparedImage
	Palettes            []missionPreparedPalette
	Fonts               []officeAsset
	Sources             []SourceLocation
	Notes               []string
	AircraftDefinitions map[int][]byte
	WeaponDefinitions   map[int][]byte
	Preflight           *PreflightAssets
}

// MissionAssets contains immutable source data. Session/animation state never
// belongs here, and decoded map indices are not prepainted with mission markers.
type MissionAssets struct {
	Generation          *MissionGenerationData
	Images              map[string][]byte
	Palettes            map[string]color.Palette
	Fonts               map[int]*BitmapFont
	Metadata            MissionPresentationData
	AircraftDefinitions map[int][]byte
	WeaponDefinitions   map[int][]byte
	Preflight           *PreflightAssets
}

func missionResources(m *Manifest) (map[string]*Resource, *Resource) {
	resources := map[string]*Resource{}
	var executable *Resource
	for _, r := range m.Resources {
		if r.Source.Kind == "archive_entry" && (r.Source.SHA256 == baseCatalogHash || r.Source.SHA256 == catalogHashes["GS2000.DAT"]) {
			resources[r.name] = r
		}
		if r.Source.Kind == "loose_file" && r.Source.PayloadSHA256 == officeCodeHash {
			executable = r
		}
	}
	return resources, executable
}

func missionChunkMap(r *Resource) (map[string][]byte, error) {
	if r == nil {
		return nil, fmt.Errorf("required mission source is missing")
	}
	chunks, err := officeChunks(r.Payload, r.Source.Offset)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", r.name, err)
	}
	result := map[string][]byte{}
	for _, c := range chunks {
		if _, ok := result[c.Tag]; ok {
			if c.Tag == "WEAP" || c.Tag == "COMP" {
				continue
			} // SHIN carries repeated per-slot definitions; we request SHHD only here.
			return nil, fmt.Errorf("%s repeated required chunk %s", r.name, c.Tag)
		}
		result[c.Tag] = bytes.Clone(c.Payload)
	}
	return result, nil
}

func missionScreen(r *Resource) (MissionScreen, error) {
	s := MissionScreen{Name: r.name}
	chunks, err := officeChunks(r.Payload, r.Source.Offset)
	if err != nil {
		return s, err
	}
	s.Chunks = chunks
	for _, c := range chunks {
		b := c.Payload
		switch c.Tag {
		case "SCHD":
			if len(b) != 36 {
				return s, fmt.Errorf("%s invalid SCHD", r.name)
			}
			for i := range s.Basenames {
				s.Basenames[i] = OfficeCString(b[i*9 : i*9+9])
			}
		case "AREA":
			if len(b) < 9 || len(b) > 34 {
				return s, fmt.Errorf("%s invalid AREA", r.name)
			}
			var raw [34]byte
			copy(raw[:], b)
			a := OfficeArea{ChunkIndex: c.Index, Key: OfficeSignedWord(raw[:]), VisualFlag: le16(raw[10:]), Priority: len(s.Areas)}
			for i := range a.Rectangle {
				a.Rectangle[i] = OfficeSignedWord(raw[2+2*i:])
			}
			if !OfficeRectFits(a.Rectangle) {
				return s, fmt.Errorf("%s AREA outside screen", r.name)
			}
			a.InclusiveEnd = [2]int{a.Rectangle[0] + a.Rectangle[2] - 1, a.Rectangle[1] + a.Rectangle[3] - 1}
			s.Areas = append(s.Areas, a)
		case "LORI":
			if len(b) != 14 {
				return s, fmt.Errorf("%s invalid LORI", r.name)
			}
			s.Lori = append(s.Lori, officeLori{ChunkIndex: c.Index, Key: int(int8(b[0])), Mode: int(b[1]), Base: [2]int{OfficeSignedWord(b[2:]), OfficeSignedWord(b[4:])}, Cell: [2]int{OfficeSignedWord(b[6:]), OfficeSignedWord(b[8:])}, Divisor: OfficeSignedWord(b[10:]), UnknownTail: OfficeSignedWord(b[12:])})
		}
	}
	return s, nil
}

func missionGenerationTables(gs []byte, p *AssetVersionProfile, resources map[string]*Resource) (MissionTables, error) {
	var t MissionTables
	layout := p.MissionLayout
	at := func(ds, n int) ([]byte, error) {
		start := p.OfficeLayout.DataFileAnchor + ds
		if ds < 0 || n < 0 || start+n > p.OfficeLayout.DataFileEnd || start+n > len(gs) {
			return nil, fmt.Errorf("mission initialized source table %x+%d outside profile", ds, n)
		}
		return gs[start : start+n], nil
	}
	pairs := func(ds, n int) ([][]int, error) {
		b, e := at(ds, n*2)
		if e != nil {
			return nil, e
		}
		r := make([][]int, n)
		for i := range r {
			r[i] = []int{int(b[2*i]), int(b[2*i+1])}
		}
		return r, nil
	}
	pointerStrings := func(ds, n int) ([]string, error) {
		b, e := at(ds, 2*n)
		if e != nil {
			return nil, e
		}
		r := make([]string, n)
		for i := range r {
			s, e := officeSourceString(gs, le16(b[i*2:]))
			if e != nil {
				return nil, e
			}
			r[i] = s.Text
		}
		return r, nil
	}
	var err error
	if t.Wind, err = pairs(layout.WindDS, 14); err != nil {
		return t, err
	}
	if t.Temperature, err = pairs(layout.TemperatureDS, 6); err != nil {
		return t, err
	}
	c, err := at(layout.ConsonantsDS, 20)
	if err != nil {
		return t, err
	}
	t.Consonants = string(c)
	if t.Callsigns, err = pointerStrings(layout.CallsignsDS, 26); err != nil {
		return t, err
	}
	if t.Compass, err = pointerStrings(layout.CompassDS, 16); err != nil {
		return t, err
	}
	ptr, err := at(layout.RankPointerDS, 2)
	if err != nil {
		return t, err
	}
	if t.Ranks, err = pointerStrings(le16(ptr), 12); err != nil {
		return t, err
	}
	for i, ds := range layout.VisibilityDS {
		s, e := officeSourceString(gs, ds)
		if e != nil {
			return t, e
		}
		t.VisibilityPairs[i/2][i%2] = s.Text
	}
	b, err := at(layout.TemplatesDS, 40*47)
	if err != nil {
		return t, err
	}
	for i := 0; i < 40; i++ {
		t.Templates = append(t.Templates, bytes.Clone(b[47*i:47*(i+1)]))
	}
	t.Leader, err = at(layout.LeaderDS, 32)
	if err != nil {
		return t, err
	}
	t.Leader = bytes.Clone(t.Leader)
	t.Thresholds = map[int]int{}
	for name, r := range resources {
		if strings.HasPrefix(name, "SHIN_") {
			cs, e := missionChunkMap(r)
			if e != nil {
				return t, e
			}
			sh := cs["SHHD"]
			if len(sh) < 6 {
				return t, fmt.Errorf("%s lacks SHHD rank threshold", name)
			}
			t.Thresholds[int(sh[0])] = int(int8(sh[5]))
		}
	}
	for _, item := range []struct {
		ds   int
		dest *[][]int
	}{{layout.PrimaryDS, &t.Primary}, {layout.SecondaryDS, &t.Secondary}} {
		b, e := at(item.ds, 28)
		if e != nil {
			return t, e
		}
		for i := 0; i < 4; i++ {
			row := []int{}
			for _, v := range b[7*i : 7*(i+1)] {
				row = append(row, int(v))
			}
			*item.dest = append(*item.dest, row)
		}
	}
	b, err = at(layout.MasksDS, 32)
	if err != nil {
		return t, err
	}
	for i := 0; i < 8; i++ {
		t.Masks = append(t.Masks, binary.LittleEndian.Uint32(b[4*i:]))
	}
	b, err = at(layout.DisplayKindsDS, 16)
	if err != nil {
		return t, err
	}
	for i := 0; i < 8; i++ {
		t.DisplayKinds = append(t.DisplayKinds, le16(b[2*i:]))
	}
	b, err = at(layout.WindMultiplierDS, 8)
	if err != nil {
		return t, err
	}
	t.WindMultiplier = math.Float64frombits(binary.LittleEndian.Uint64(b))
	// Editor3348/3460: source inventory costs and nineteen-byte map rows.
	// These tables extend preparation26 only; old metadata remains readable.
	if b, e := at(0x0da0, 230*2); e != nil {
		return t, e
	} else {
		for i := 0; i < 230; i++ {
			t.EditorModelCosts = append(t.EditorModelCosts, binary.LittleEndian.Uint16(b[2*i:]))
		}
	}
	if b, e := at(0x2c28, 4); e != nil {
		return t, e
	} else {
		for _, v := range b {
			t.EditorTheatreOffsets = append(t.EditorTheatreOffsets, int8(v))
		}
	}
	if b, e := at(0x2c3d, 19*13+2); e != nil {
		return t, e
	} else {
		for i := 0; i < 14; i++ {
			t.EditorWorldBaseCosts = append(t.EditorWorldBaseCosts, binary.LittleEndian.Uint16(b[19*i:]))
		}
	}
	return t, nil
}

func exportMissionMetadata(out *OutputWriter, alloc *pathAllocator, m *Manifest) error {
	if m.OfficeMetadata == "" {
		return nil
	}
	profile, err := recognizeOfficeProfile(m.Resources)
	if err != nil {
		return nil
	}
	resources, gs := missionResources(m)
	if gs == nil {
		return fmt.Errorf("mission preparation requires profile executable")
	}
	doc := MissionMetadata{Schema: missionMetadataSchema, Profile: profile.Name, Generation: MissionGenerationData{Version: 1}, Presentation: MissionPresentationData{Templates: map[int]string{}, Screens: map[string]MissionScreen{}, Transitions: map[string][]int{"1->2": {1, 2}, "1->3": {4, 5, 6, 7}, "2->1": {3, 2}, "2->3": {9, 10, 11, 12}, "3->1": {8, 7, 6, 5}, "3->2": {13, 12, 11, 10}}}, Notes: []string{
		"Step23 source-derived mission and briefing data; offline decoding is not new original-game runtime verification.",
		"FIELD stable scenes, commander, papers, maps and REALITY inherit FIELD.PAL; interstitials use matching TENT/SHIP/ICE.PAL. Final factor256 preserves source RGB6.",
		"Source CAT cache entries take precedence here. Loose F5 map files are not interchangeable supported X0 resources.",
		"Commander elapsed-time cadence and retained-head compatibility policy remain explicit runtime approximations; original uninitialized stack bytes are not fabricated.",
	}}
	doc.Generation.Tables, err = missionGenerationTables(gs.Payload, profile, resources)
	if err != nil {
		return err
	}
	var simulation *Resource
	for _, r := range m.Resources {
		if r.Source.Kind == "loose_file" && profile.SourceMatches(RoleSimulationExecutable, Hash(r.Payload)) {
			simulation = r
			break
		}
	}
	if simulation == nil {
		return fmt.Errorf("mission initialization requires supported GS2.GS2")
	}
	doc.Generation.Tables.Initialization, err = prepareMissionInitialization(gs.Payload, simulation.Payload, profile)
	if err != nil {
		return err
	}
	doc.Sources = append(doc.Sources, simulation.Source)
	doc.Sources = append(doc.Sources, gs.Source)
	doc.AircraftDefinitions = map[int][]byte{}
	doc.WeaponDefinitions = map[int][]byte{}
	for name, r := range resources {
		if strings.HasPrefix(name, "SHIN_") || strings.HasPrefix(name, "WEAP_") {
			cs, e := officeChunks(r.Payload, r.Source.Offset)
			if e != nil {
				return e
			}
			for _, c := range cs {
				b := c.Payload
				if c.Tag == "SHHD" && len(b) == 252 {
					doc.AircraftDefinitions[int(b[0])] = bytes.Clone(b)
				}
				if c.Tag == "WSHD" && len(b) == 214 {
					doc.WeaponDefinitions[int(b[0])] = bytes.Clone(b)
				}
			}
			doc.Sources = append(doc.Sources, r.Source)
		}
	}
	for _, ds := range profile.MissionLayout.TemplateStrings {
		s, e := officeSourceString(gs.Payload, ds)
		if e != nil {
			return e
		}
		doc.Presentation.Templates[ds] = s.Text
	}
	// GS P0407 reads this signed-counter-selected seven-pointer table. Keep
	// prose source-derived and typed; it is independent of the wall GRAF.
	statusAt := profile.OfficeLayout.DataFileAnchor + profile.MissionLayout.CampaignStatusPointersDS
	if statusAt < 0 || statusAt+14 > len(gs.Payload) || statusAt+14 > profile.OfficeLayout.DataFileEnd {
		return fmt.Errorf("campaign paper status table outside source profile")
	}
	for i, want := range []uint16{0x3119, 0x317d, 0x31d4, 0x321f, 0x3278, 0x32b8, 0x330b} {
		ds := binary.LittleEndian.Uint16(gs.Payload[statusAt+2*i:])
		if ds != want {
			return fmt.Errorf("campaign status pointer%d differs from profile", i)
		}
		text, err := officeSourceString(gs.Payload, int(ds))
		if err != nil {
			return err
		}
		doc.Presentation.CampaignStatus = append(doc.Presentation.CampaignStatus, text.Text)
	}
	start := profile.OfficeLayout.DataFileAnchor + profile.MissionLayout.PoseDS
	if start+42 > profile.OfficeLayout.DataFileEnd || start+42 > len(gs.Payload) {
		return fmt.Errorf("commander pose table outside initialized profile")
	}
	for i := 0; i < 14; i++ {
		b := gs.Payload[start+i*3 : start+i*3+3]
		pose := CommanderPose{Index: i, Triple: [3]int{int(b[0]), int(b[1]), int(b[2])}, Body: [4]int{80 * int(b[0]), 32, 80, 56}, Arms: [4]int{40 * int(b[2]), 88, 40, 62}}
		if b[1] != 0 {
			pose.Arms = [4]int{53 * int(b[2]), 150, 53, 40}
		}
		if !OfficeRectFits(pose.Body) || !OfficeRectFits(pose.Arms) {
			return fmt.Errorf("commander pose%d outside atlas", i)
		}
		doc.Presentation.Poses = append(doc.Presentation.Poses, pose)
	}
	loose := map[string]*Resource{}
	for _, r := range m.Resources {
		if r.Source.Kind == "loose_file" {
			loose[r.name] = r
		}
	}
	// Preserve all twenty source banks, including worlds that do not occur in
	// this scenario list. Authored launches select by their actual world name.
	doc.Generation.EnemyDefinitions = map[string][]EnemyDefinition{}
	for name := range originalEnemyDefinitionHashes {
		wrld, err := missionChunkMap(resources[name])
		if err != nil {
			return err
		}
		rows, err := decodeEnemyDefinitions(name, wrld["ENEM"])
		if err != nil {
			return err
		}
		doc.Generation.EnemyDefinitions[name] = rows
	}
	for theatre := 0; theatre < 4; theatre++ {
		th, err := missionChunkMap(resources[fmt.Sprintf("SCENE_%02d.THR", theatre)])
		if err != nil {
			return err
		}
		header := th["THHD"]
		if len(header) != 54 {
			return fmt.Errorf("invalid THHD%d", theatre)
		}
		// THHD+16 is both Training suffix and exclusive ordinary-scenario count.
		for suffix := 0; suffix <= int(header[0x16]); suffix++ {
			scenario := OfficeCString(header[1:5]) + fmt.Sprintf("_%02d.SNR", suffix)
			sr := resources[scenario]
			if sr == nil {
				continue
			}
			sn, err := missionChunkMap(sr)
			if err != nil {
				return err
			}
			snhead := sn["SNHD"]
			if len(snhead) < 11 {
				return fmt.Errorf("%s invalid SNHD", scenario)
			}
			worldName := OfficeCString(snhead[1:10]) + ".WRL"
			wr := resources[worldName]
			world, err := missionChunkMap(wr)
			if err != nil {
				return err
			}
			terrain := OfficeCString(world["WDHD"])
			grid := loose[terrain]
			if grid == nil || len(grid.Payload) < 4096 {
				return fmt.Errorf("%s requires complete terrain grid %s", worldName, terrain)
			}
			w := MissionWorld{Theatre: theatre, Suffix: suffix, Scenario: scenario, WorldName: worldName, TerrainName: terrain, THHD: bytes.Clone(header), SNHD: bytes.Clone(snhead), FMHD: world["FMHD"], FRMD: world["FRMD"], OBJL: world["OBJL"], LAND: world["LAND"], PATH: world["PATH"], Grid: bytes.Clone(grid.Payload[:4096])}
			doc.Generation.Worlds = append(doc.Generation.Worlds, w)
			doc.Sources = append(doc.Sources, sr.Source, wr.Source, grid.Source)
		}
	}
	for _, name := range []string{"BRIEFING.SCN", "FIELD.SCN", "FIELD2.SCN", "FIELD3.SCN", "REALITY.SCN", "OUTFIT.SCN", "ORDNANCE.SCN"} {
		r := resources[name]
		if r == nil {
			return fmt.Errorf("mission screen %s missing", name)
		}
		s, e := missionScreen(r)
		if e != nil {
			return e
		}
		doc.Presentation.Screens[name] = s
		doc.Sources = append(doc.Sources, r.Source)
	}
	doc.Preflight, err = preparePreflightAssets(gs, resources, profile)
	if err != nil {
		return err
	}
	doc.Generation.Tables.EditorObjectiveVerbs = append([]string(nil), doc.Preflight.ObjectiveSummary.Verbs...)
	doc.Generation.Tables.EditorObjectiveAbbreviations = append([][2]string(nil), doc.Preflight.ObjectiveSummary.Abbreviations...)

	for _, r := range m.Resources {
		if r.Source.Kind == "loose_file" && r.name == "OBJTYPES.DAT" {
			if len(r.Payload) != 230*27 || Hash(r.Payload) != "a85143130d48e80b24d8a96f99c6583385f7e85c481df30a772417754224e24a" {
				return fmt.Errorf("editor OBJTYPES profile differs")
			}
			doc.Generation.Tables.EditorObjectTypes = bytes.Clone(r.Payload)
			doc.Sources = append(doc.Sources, r.Source)
			break
		}
	}
	if len(doc.Generation.Tables.EditorObjectTypes) != 230*27 {
		return fmt.Errorf("editor OBJTYPES missing")
	}

	paletteNames := []string{"FIELD.PAL", "TENT.PAL", "SHIP.PAL", "ICE.PAL"}
	paletteData := map[string]*Palette{}
	for _, name := range paletteNames {
		r := resources[name]
		if r == nil {
			return fmt.Errorf("mission palette %s missing", name)
		}
		ps, _, _, e := pictureHeader(r.Payload)
		if e != nil || len(ps) != 1 || ps[0].First != 0 || ps[0].Last != 255 {
			return fmt.Errorf("mission palette %s requires all256 RGB6 entries", name)
		}
		p := ps[0]
		paletteData[name] = &p
		o, e := preparedOutput(r.Outputs, "exact_rgb6_components")
		if e != nil {
			return e
		}
		doc.Palettes = append(doc.Palettes, missionPreparedPalette{name, r.Source, o})
	}
	names := append([]string{}, missionRequiredPictures...)
	names = append(names, preflightPictureNames()...)
	maps := map[string]bool{}
	for _, w := range doc.Generation.Worlds {
		maps[w.TerrainName+"MAP.PIC"] = true
	}
	var mapNames []string
	for n := range maps {
		mapNames = append(mapNames, n)
	}
	sort.Strings(mapNames)
	names = append(names, mapNames...)
	for _, name := range names {
		r := resources[name]
		if r == nil {
			return fmt.Errorf("mission image %s missing", name)
		}
		if r.Conversion.PixelSHA256 == "" || r.Conversion.Width != 320 || r.Conversion.Height != 200 {
			return fmt.Errorf("mission image %s lacks full supported320x200 decode", name)
		}
		o, e := preparedOutput(r.Outputs, "decoded_indices")
		if e != nil {
			return e
		}
		pixels, e := os.ReadFile(filepath.Join(out.Root, filepath.FromSlash(o.Path)))
		if e != nil {
			return e
		}
		if len(pixels) != 64000 || Hash(pixels) != o.SHA256 {
			return fmt.Errorf("mission image%s indices changed", name)
		}
		palName := "FIELD.PAL"
		if name == "TENT.PIC" || name == "SHIP.PIC" || name == "ICE.PIC" {
			palName = strings.TrimSuffix(name, ".PIC") + ".PAL"
		}
		palResource := resources[palName]
		asset := officePreparedAsset(r, false)
		asset.Palette = PaletteAssociation{palResource.ExportPath, palResource.Source.PayloadSHA256, "Step23 source initialization/staging order and identity final RGB6 fade; not a new destination DAC observation", "source_path_and_prior_hardware_writer", false}
		img := image.NewPaletted(image.Rect(0, 0, 320, 200), paletteColors(paletteData[palName]))
		copy(img.Pix, pixels)
		pngData, e := PngBytes(img)
		if e != nil {
			return e
		}
		if e = addPalettedGenerated(out, r, alloc, ".mission.png", "mission_supported_colour_png", pngData, asset.Palette); e != nil {
			return e
		}
		asset.Outputs = r.Outputs
		// Generic extraction initially lacks screen ownership. Only this exact
		// source profile supplies the later FIELD/interstitial palette association.
		if r.Conversion.Status == "converted_diagnostic_grayscale" {
			m.Summary.DiagnosticPictures--
			m.Summary.ColourPictures++
			r.Conversion.Status = "converted_supported_colour"
			r.Conversion.Palette = asset.Palette
			r.Conversion.RuntimeValidation = "source-selected palette path and offline full-image decode; no new native bank or destination DAC observation"
		}
		// POINTER already has an office BASE-colour export. Its mission output
		// owns FIELD above; preserve the earlier default and validation scope.
		doc.Images = append(doc.Images, missionPreparedImage{asset, palName, 320, 200})
	}
	for _, r := range m.Resources {
		if r.Source.PayloadSHA256 == originalFontHash {
			asset := officePreparedAsset(r, false)
			asset.Outputs = nil
			for _, o := range r.Outputs {
				for _, selector := range []int{1, 2, 3, 5} {
					if o.Path == fmt.Sprintf("%s.font%d.mask.png", r.ExportPath, selector) || o.Path == fmt.Sprintf("%s.font%d.metrics.json", r.ExportPath, selector) {
						asset.Outputs = append(asset.Outputs, o)
					}
				}
			}
			doc.Fonts = append(doc.Fonts, asset)
		}
	}
	if err = validateMissionMetadata(doc); err != nil {
		return err
	}
	sort.Slice(doc.Sources, func(i, j int) bool {
		a, b := doc.Sources[i], doc.Sources[j]
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		return a.Offset < b.Offset
	})
	raw, err := JsonBytes(doc)
	if err != nil {
		return err
	}
	if err = addGenerated(out, resources["FIELD.SCN"], alloc, ".missions.json", "mission_preparation_metadata", raw); err != nil {
		return err
	}
	m.MissionMetadata = resources["FIELD.SCN"].Outputs[len(resources["FIELD.SCN"].Outputs)-1].Path
	return nil
}

// Exact source text identities are tied to this sole admitted executable
// profile. Older preparations omit the new table; new readiness requires it.
func validateCampaignStatusParagraphs(text []string) error {
	if len(text) == 0 {
		return nil
	}
	want := []string{
		"9cf17b5c6b5ff6480ddb5f2f8f77cb90d9c19f36dde35457443833077be0dbdb",
		"0fd52718c6d0b5a813c59f179be11d16e42505006bc2dd5bc50decef2a7e95b7",
		"c1ef159944a44924b4802b16184b433af7ef64254dad4286b0ca96cce44f4187",
		"78e3b01c489c0bdd6d48085d65fd31d5b92dd91e3ca677dcda6def2ff9bda70f",
		"437115e438a613c215a993b069436374422bf492228baa6e21d68ff3d25797f2",
		"c01fb54f421af4b32e7027d3869336290c064b5c274e1ce80702a3151a32b0bb",
		"7f4a79184826553a5342d61845f740c3ff295ccb998e8396ad17f2734a8f2be4",
	}
	if len(text) != len(want) {
		return fmt.Errorf("incomplete Campaign paper source table")
	}
	for i, s := range text {
		if Hash([]byte(s)) != want[i] {
			return fmt.Errorf("Campaign paper paragraph%d source identity differs", i)
		}
	}
	return nil
}

func validateMissionMetadata(doc MissionMetadata) error {
	if doc.Schema != missionMetadataSchema || doc.Profile != IslandIce469085.Name || doc.Generation.Version != 1 || len(doc.Generation.Worlds) != 16 || len(doc.Presentation.Poses) != 14 || len(doc.Presentation.Screens) != 7 || len(doc.Palettes) != 4 || len(doc.Fonts) != 1 {
		return fmt.Errorf("incomplete or incompatible mission preparation metadata (schema%d, worlds%d, poses%d, screens%d, palettes%d, fonts%d); rebuild assets", doc.Schema, len(doc.Generation.Worlds), len(doc.Presentation.Poses), len(doc.Presentation.Screens), len(doc.Palettes), len(doc.Fonts))
	}
	if err := validateCampaignStatusParagraphs(doc.Presentation.CampaignStatus); err != nil {
		return err
	}
	if err := validatePreflightAssets(doc.Preflight); err != nil {
		return err
	}
	t := doc.Generation.Tables
	if len(t.Wind) != 14 || len(t.Temperature) != 6 || len(t.Consonants) != 20 || len(t.Ranks) != 12 || len(t.Callsigns) != 26 || len(t.Compass) != 16 || len(t.Templates) != 40 || len(t.Thresholds) != 8 || len(t.Primary) != 4 || len(t.Secondary) != 4 || len(t.Masks) != 8 || len(t.DisplayKinds) != 8 || len(t.Leader) != 32 || !isFinite(t.WindMultiplier) {
		return fmt.Errorf("mission source tables incomplete")
	}
	seen := map[[2]int]bool{}
	for _, w := range doc.Generation.Worlds {
		k := [2]int{w.Theatre, w.Suffix}
		if seen[k] || w.Theatre < 0 || w.Theatre > 3 || w.Suffix < 0 || w.Suffix > 4 || (w.Theatre >= 2 && w.Suffix >= 3) || len(w.THHD) != 54 || len(w.SNHD) < 11 || len(w.Grid) != 4096 || len(w.FMHD) == 0 || len(w.FMHD)%39 != 0 || len(w.FRMD)%8 != 0 || len(w.OBJL)%32 != 0 || len(w.PATH)%9 != 0 || len(w.LAND) < 64 || (len(w.LAND)-64)%17 != 0 {
			return fmt.Errorf("invalid mission world %s", w.WorldName)
		}
		seen[k] = true
	}
	for _, rows := range [][][]int{t.Wind, t.Temperature} {
		for _, r := range rows {
			if len(r) != 2 {
				return fmt.Errorf("malformed weighted source row")
			}
		}
	}
	for _, rows := range [][][]int{t.Primary, t.Secondary} {
		for _, r := range rows {
			if len(r) != 7 {
				return fmt.Errorf("malformed objective weight row")
			}
		}
	}
	for _, s := range []string{"BRIEFING.SCN", "FIELD.SCN", "FIELD2.SCN", "FIELD3.SCN", "REALITY.SCN", "OUTFIT.SCN", "ORDNANCE.SCN"} {
		screen, ok := doc.Presentation.Screens[s]
		if !ok || screen.Name != s || len(screen.Chunks) == 0 {
			return fmt.Errorf("missing mission screen%s", s)
		}
	}
	pals := map[string]bool{}
	for _, p := range doc.Palettes {
		if pals[p.Name] || p.Components.Size != 768 || p.Components.Kind != "exact_rgb6_components" {
			return fmt.Errorf("invalid mission palette output%s", p.Name)
		}
		pals[p.Name] = true
	}
	for _, name := range []string{"FIELD.PAL", "TENT.PAL", "SHIP.PAL", "ICE.PAL"} {
		if !pals[name] {
			return fmt.Errorf("missing mission palette%s", name)
		}
	}
	images := map[string]bool{}
	for _, i := range doc.Images {
		if images[i.Asset.Name] || i.Width != 320 || i.Height != 200 || i.Asset.Palette.Diagnostic || !pals[i.Palette] || len(i.Asset.PixelSHA256) != 64 {
			return fmt.Errorf("invalid mission image%s", i.Asset.Name)
		}
		images[i.Asset.Name] = true
	}
	for _, name := range append(append([]string{}, missionRequiredPictures...), preflightPictureNames()...) {
		if !images[name] {
			return fmt.Errorf("missing mission picture%s", name)
		}
	}
	for _, w := range doc.Generation.Worlds {
		if !images[w.TerrainName+"MAP.PIC"] {
			return fmt.Errorf("missing mission map for%s", w.WorldName)
		}
	}
	if len(doc.AircraftDefinitions) != 8 || len(doc.WeaponDefinitions) != 28 {
		return fmt.Errorf("incomplete aircraft/system readiness definitions")
	}
	for _, row := range doc.AircraftDefinitions {
		if len(row) != 252 {
			return fmt.Errorf("malformed SHHD")
		}
	}
	for _, row := range doc.WeaponDefinitions {
		if len(row) != 214 {
			return fmt.Errorf("malformed WSHD")
		}
	}
	for _, selector := range []int{1, 2, 3, 5} {
		metrics, mask := false, false
		for _, font := range doc.Fonts {
			for _, o := range font.Outputs {
				if o.Kind == "bitmap_font_metrics" && strings.HasSuffix(o.Path, fmt.Sprintf(".font%d.metrics.json", selector)) {
					metrics = true
				}
				if o.Kind == "bitmap_font_mask_atlas" && strings.HasSuffix(o.Path, fmt.Sprintf(".font%d.mask.png", selector)) {
					mask = true
				}
			}
		}
		if !metrics || !mask {
			return fmt.Errorf("mission font%d metadata/mask missing; rebuild assets", selector)
		}
	}
	if err := ValidateMissionInitialization(t.Initialization); err != nil {
		return err
	}
	for _, row := range t.Templates {
		if len(row) != 47 {
			return fmt.Errorf("invalid47-byte flight template")
		}
	}
	for _, p := range doc.Presentation.Poses {
		if !OfficeRectFits(p.Body) || !OfficeRectFits(p.Arms) {
			return fmt.Errorf("commander pose outside atlas")
		}
	}
	return nil
}

func isFinite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

func loadMissionAssets(reader *preparedReader, manifest Manifest, existingFonts map[int]*BitmapFont) (*MissionAssets, error) {
	output, ok := reader.outputs[manifest.MissionMetadata]
	if !ok || output.Kind != "mission_preparation_metadata" {
		return nil, fmt.Errorf("prepared assets lack Step D mission data; rebuild/upgrade assets")
	}
	raw, err := reader.verified(output, 16<<20)
	if err != nil {
		return nil, err
	}
	var doc MissionMetadata
	if err = json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	if err = validateMissionMetadata(doc); err != nil {
		return nil, err
	}
	if manifest.PreparationVersion >= 32 {
		if err = doc.Generation.ValidateEnemyDefinitions(); err != nil {
			return nil, err
		}
	}
	if manifest.PreparationVersion >= 25 {
		if len(doc.Presentation.CampaignStatus) != 7 {
			return nil, fmt.Errorf("prepared Campaign paper status table missing; prepare25")
		}
		for _, s := range doc.Presentation.CampaignStatus {
			if s == "" {
				return nil, fmt.Errorf("prepared Campaign paper paragraph missing; prepare25")
			}
		}
	}
	if manifest.PreparationVersion >= 26 && (len(doc.Generation.Tables.EditorObjectTypes) != 230*27 || len(doc.Generation.Tables.EditorModelCosts) != 230 || len(doc.Generation.Tables.EditorTheatreOffsets) != 4 || len(doc.Generation.Tables.EditorWorldBaseCosts) != 14 || len(doc.Generation.Tables.EditorObjectiveVerbs) != 8 || len(doc.Generation.Tables.EditorObjectiveAbbreviations) != 4) {
		return nil, fmt.Errorf("prepared Mission Editor compilation tables missing; prepare26")
	}

	a := &MissionAssets{Generation: &doc.Generation, Metadata: doc.Presentation, Images: map[string][]byte{}, Palettes: map[string]color.Palette{}, Fonts: map[int]*BitmapFont{}, AircraftDefinitions: doc.AircraftDefinitions, WeaponDefinitions: doc.WeaponDefinitions, Preflight: doc.Preflight}
	for k, v := range existingFonts {
		a.Fonts[k] = v
	}
	for _, p := range doc.Palettes {
		raw, e := reader.verified(p.Components, 768)
		if e != nil {
			return nil, e
		}
		if len(raw) != 768 {
			return nil, fmt.Errorf("palette%s lacks768RGB6 bytes", p.Name)
		}
		pal := make(color.Palette, 256)
		for i := range pal {
			v := raw[i*3 : i*3+3]
			if v[0] > 63 || v[1] > 63 || v[2] > 63 {
				return nil, fmt.Errorf("palette%s component exceedsRGB6", p.Name)
			}
			pal[i] = color.NRGBA{v[0]<<2 | v[0]>>4, v[1]<<2 | v[1]>>4, v[2]<<2 | v[2]>>4, 255}
		}
		a.Palettes[p.Name] = pal
	}
	for _, image := range doc.Images {
		o, e := preparedOutput(image.Asset.Outputs, "decoded_indices")
		if e != nil {
			return nil, e
		}
		raw, e := reader.verified(o, 64000)
		if e != nil {
			return nil, e
		}
		if len(raw) != 64000 || Hash(raw) != image.Asset.PixelSHA256 || image.Asset.Palette.Diagnostic || len(a.Palettes[image.Palette]) != 256 {
			return nil, fmt.Errorf("mission image%s invalid pixels/palette", image.Asset.Name)
		}
		a.Images[image.Asset.Name] = raw
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
			if metrics.Selector != 1 && metrics.Selector != 2 && metrics.Selector != 3 && metrics.Selector != 5 {
				continue
			}
			font, e := loadMenuFont(reader, metrics)
			if e != nil {
				return nil, e
			}
			a.Fonts[metrics.Selector] = font
		}
	}
	for _, selector := range []int{1, 2, 3, 5} {
		if a.Fonts[selector] == nil {
			return nil, fmt.Errorf("mission font%d missing", selector)
		}
	}
	for name, s := range a.Metadata.Screens {
		for i := range s.Chunks {
			b, e := hex.DecodeString(s.Chunks[i].RawHex)
			if e != nil || len(b) != s.Chunks[i].Length {
				return nil, fmt.Errorf("%s raw chunk mismatch", name)
			}
			s.Chunks[i].Payload = b
		}
		a.Metadata.Screens[name] = s
	}
	return a, nil
}

// LoadMissionAssets verifies prepared outputs without reading original executables.
func LoadMissionAssets(directory string) (*MissionAssets, error) {
	reader, err := newPreparedReader(directory)
	if err != nil {
		return nil, err
	}
	defer reader.close()
	return reader.loadMission()
}
func (reader *preparedReader) loadMission() (*MissionAssets, error) {
	raw, err := reader.read("manifest.json", 64<<20)
	if err != nil {
		return nil, err
	}
	var manifest Manifest
	if err = json.Unmarshal(raw, &manifest); err != nil {
		return nil, err
	}
	if !compatiblePreparation(manifest) {
		return nil, fmt.Errorf("mission prepared manifest schema/version incompatible; rebuild assets")
	}
	for _, r := range manifest.Resources {
		if r == nil {
			return nil, fmt.Errorf("null prepared mission resource")
		}
		for _, o := range r.Outputs {
			if old, ok := reader.outputs[o.Path]; ok && old != o {
				return nil, fmt.Errorf("conflicting prepared output%s", o.Path)
			}
			reader.outputs[o.Path] = o
		}
	}
	return loadMissionAssets(reader, manifest, nil)
}
