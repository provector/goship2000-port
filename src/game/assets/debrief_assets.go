package assets

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image/color"
	"sort"
	"strings"
)

const debriefSchema = 4

type DebriefComponent struct {
	Group       int
	SourceAsset string
	Source      [4]int
	Destination [2]int
}

type DebriefLori struct {
	Key, Mode     int
	SourceAsset   string
	Base, Cell    [2]int
	Divisor, Tail int
}

type DebriefSprite struct {
	Key         int
	Source      [4]int
	Destination [2]int
	Tail        [2]byte
}

// These are prepared semantic screen descriptions, not executable SCN bytecode.
// Controller phase, rank/award choice and mutable pixels belong to the caller.
type DebriefScene struct {
	Name, Background, Palette string
	Areas                     []OfficeArea
	Lori                      []DebriefLori
	Sprites                   []DebriefSprite
	Components                []DebriefComponent
	Source                    SourceLocation
}

type debriefImage struct {
	Name    string
	Source  SourceLocation
	Indices Output
}

type debriefMetadata struct {
	Schema                               int
	Profile                              string
	Scenes                               map[string]DebriefScene
	Images                               []debriefImage
	Palettes                             []missionPreparedPalette
	Text                                 map[int]string
	Feedback                             map[string][]string
	RankThresholds                       [12]int16
	MedalThresholds, MedalPenaltyCredits [6]int16
	RankNames                            [12]string
	MedalNames, MedalCitations           [8]string
	WingQualifiers, WingLevels           [4]string
	Sources                              []SourceLocation
	Limits                               []string
}

// DebriefAssets is immutable prepared source data. No runtime original executable,
// research export or legacy pointer/segment is needed to compose admitted pages.
type DebriefAssets struct {
	Scenes                               map[string]DebriefScene
	Images                               map[string][]byte
	Palettes                             map[string]color.Palette
	Text                                 map[int]string
	Feedback                             map[string][]string
	RankThresholds                       [12]int16
	MedalThresholds, MedalPenaltyCredits [6]int16
	RankNames                            [12]string
	MedalNames, MedalCitations           [8]string
	WingQualifiers, WingLevels           [4]string
}

func parseDebriefScene(r *Resource) (DebriefScene, error) {
	var d DebriefScene
	if r == nil {
		return d, fmt.Errorf("debrief scene missing")
	}
	s, err := missionScreen(r)
	if err != nil {
		return d, err
	}
	d = DebriefScene{Name: s.Name, Areas: s.Areas, Source: r.Source}
	switch r.name {
	case "COMMISSN.SCN":
		d.Background, d.Palette = "COMMISSN.PIC", "COMM.PAL"
	case "JUMP.SCN":
		d.Background, d.Palette = "JUMP.PIC", "JUMP.PAL"
	case "HAPPY.SCN", "SAD.SCN":
		d.Background, d.Palette = r.name[:len(r.name)-4]+"_2.PIC", "HAPPY.PAL"
		if r.name == "SAD.SCN" {
			d.Palette = "SAD_2.PAL"
		}
	case "KIA.SCN":
		d.Background, d.Palette = "KIA.PIC", "KIA.PAL"
	case "RESCUE.SCN":
		d.Background, d.Palette = "RESCUE.PIC", "RESCUE.PAL"
	case "REPLACE.SCN":
		d.Background, d.Palette = "FIELD.PIC", "FIELD.PAL"
	case "CREWPRO.SCN":
		// SCHD has no background. The controller constructs its four panels.
		d.Palette = "FIELD.PAL"
	case "WIN.SCN", "LOSE.SCN":
		d.Background, d.Palette = r.name[:len(r.name)-4]+".PIC", r.name[:len(r.name)-4]+".PAL"
	default:
		return d, fmt.Errorf("unadmitted debrief scene %s", r.name)
	}
	for _, l := range s.Lori {
		sheet := "COMM_AN.PIC"
		if r.name == "HAPPY.SCN" {
			sheet = "HAPPY_AN.PIC"
		}
		if r.name == "SAD.SCN" {
			sheet = "SAD_AN.PIC"
		}
		if r.name == "JUMP.SCN" {
			// Source transparent LORI0 retains BPATCHES; the later explicit
			// RANKS load supplies opaque rank/wing LORI1..7.
			sheet = "RANKS.PIC"
			if l.Key == 0 {
				sheet = "BPATCHES.PIC"
			}
		}
		if r.name == "KIA.SCN" {
			sheet = "KIA_AN.PIC"
		}
		if r.name == "REPLACE.SCN" {
			sheet = "MEMO.PIC"
		}
		if r.name == "CREWPRO.SCN" {
			sheet = "PATCHES.PIC"
		}
		d.Lori = append(d.Lori, DebriefLori{l.Key, l.Mode, sheet, l.Base, l.Cell, l.Divisor, l.UnknownTail})
	}
	group := -1
	for _, c := range s.Chunks {
		if c.Tag == "CMPX" && r.name == "RESCUE.SCN" {
			b := c.Payload
			if len(b) != 14 {
				return d, fmt.Errorf("rescue CMPX width")
			}
			if b[0] != 255 {
				group = int(b[0])
			}
			bank := int(int8(b[13]))
			if group < 0 || bank < 0 || bank > 1 {
				return d, fmt.Errorf("rescue CMPX bank/group")
			}
			v := DebriefComponent{Group: group, SourceAsset: []string{"RESC_AN.PIC", "RESC_AN1.PIC"}[bank]}
			for i := range v.Source {
				v.Source[i] = OfficeSignedWord(b[1+2*i:])
			}
			v.Destination = [2]int{OfficeSignedWord(b[9:]), OfficeSignedWord(b[11:])}
			if !OfficeRectFits(v.Source) || !OfficeRectFits([4]int{v.Destination[0], v.Destination[1], v.Source[2], v.Source[3]}) {
				return d, fmt.Errorf("rescue CMPX bounds")
			}
			d.Components = append(d.Components, v)
			continue
		}
		if c.Tag == "SPRT" {
			if len(c.Payload) != 15 {
				return d, fmt.Errorf("debrief SPRT width")
			}
			b := c.Payload
			v := DebriefSprite{Key: int(b[0]), Tail: [2]byte{b[13], b[14]}}
			for i := range v.Source {
				v.Source[i] = OfficeSignedWord(b[1+2*i:])
			}
			for i := range v.Destination {
				v.Destination[i] = OfficeSignedWord(b[9+2*i:])
			}
			if !OfficeRectFits(v.Source) {
				return d, fmt.Errorf("debrief SPRT bounds")
			}
			d.Sprites = append(d.Sprites, v)
		} else if c.Tag != "SCHD" && c.Tag != "AREA" && c.Tag != "LORI" {
			return d, fmt.Errorf("unadmitted debrief scene chunk %s", c.Tag)
		}
	}
	return d, nil
}

func prepareDebriefMetadata(resources map[string]*Resource, gs *Resource, p *AssetVersionProfile) (debriefMetadata, error) {
	if p == nil {
		return debriefMetadata{}, fmt.Errorf("debrief profile missing")
	}
	d := debriefMetadata{Schema: debriefSchema, Profile: p.Name, Scenes: map[string]DebriefScene{}, Text: map[int]string{}, Limits: []string{
		"Source COMMISSN/JUMP/HAPPY/SAD, RESCUE/KIA, CREWPRO and WIN/LOSE controllers; replay library remains deferred.",
		"Native presentation cadence maps original callback counters to the existing configurable UI cadence; original historical wall time is not calibrated.",
	}}
	if p.DebriefLayout == nil || gs == nil || !p.SourceMatches(RoleMainExecutable, Hash(gs.Payload)) {
		return d, fmt.Errorf("debrief requires recognized original GS profile")
	}
	l := p.DebriefLayout
	read := func(ds, n int) ([]byte, error) {
		at := p.OfficeLayout.DataFileAnchor + ds
		if ds < 0 || n < 0 || at+n > p.OfficeLayout.DataFileEnd || at+n > len(gs.Payload) {
			return nil, fmt.Errorf("debrief source table outside profile")
		}
		b := gs.Payload[at : at+n]
		s := gs.Source
		s.Kind, s.Offset, s.Length, s.PayloadSHA256 = "executable_slice", uint64(at), uint64(n), Hash(b)
		d.Sources = append(d.Sources, s)
		return b, nil
	}
	words := func(ds int, dst []int16) error {
		b, e := read(ds, 2*len(dst))
		if e != nil {
			return e
		}
		for i := range dst {
			dst[i] = int16(binary.LittleEndian.Uint16(b[2*i:]))
		}
		return nil
	}
	if err := words(l.RankThresholdsDS, d.RankThresholds[:]); err != nil {
		return d, err
	}
	if err := words(l.MedalThresholdsDS, d.MedalThresholds[:]); err != nil {
		return d, err
	}
	if err := words(l.MedalPenaltyCreditsDS, d.MedalPenaltyCredits[:]); err != nil {
		return d, err
	}
	stringsAt := func(ds int, dst []string) error {
		b, e := read(ds, 2*len(dst))
		if e != nil {
			return e
		}
		for i := range dst {
			at := le16(b[2*i:])
			if at == 0 {
				continue
			}
			s, e := officeSourceString(gs.Payload, at)
			if e != nil {
				return e
			}
			dst[i] = s.Text
			d.Text[at] = s.Text
		}
		return nil
	}
	if err := stringsAt(l.RankNamesDS, d.RankNames[:]); err != nil {
		return d, err
	}
	if err := stringsAt(l.MedalCitationsDS, d.MedalCitations[:]); err != nil {
		return d, err
	}
	if err := stringsAt(l.WingQualifiersDS, d.WingQualifiers[:]); err != nil {
		return d, err
	}
	if err := stringsAt(l.WingLevelsDS, d.WingLevels[:]); err != nil {
		return d, err
	}
	awardPointer, e := read(p.PreflightLayout.AwardPointerDS, 2)
	if e != nil {
		return d, e
	}
	if e = stringsAt(le16(awardPointer), d.MedalNames[:]); e != nil {
		return d, e
	}
	// The bounded source range also contains pointer tables. Keep only complete
	// printable NUL strings; typed tables above retain their own numeric meaning.
	rangeBytes, e := read(l.TextFirstDS, l.TextEndDS-l.TextFirstDS)
	if e != nil {
		return d, e
	}
	for i := 0; i < len(rangeBytes); {
		end := bytes.IndexByte(rangeBytes[i:], 0)
		if end < 0 {
			break
		}
		end += i
		b := rangeBytes[i:end]
		valid := len(b) > 0
		for _, c := range b {
			if c < 32 || c > 126 {
				valid = false
				break
			}
		}
		if valid {
			d.Text[l.TextFirstDS+i] = string(b)
		}
		i = end + 1
	}
	// This footer follows two non-NUL pointer words at51c0. A printable-run
	// scan cannot establish its start; the actual19cd producer supplies51c4.
	footer, e := officeSourceString(gs.Payload, 0x51c4)
	if e != nil {
		return d, e
	}
	d.Text[0x51c4] = footer.Text
	// These independent controller banks are outside the original StepN range.
	// Explicit pointers avoid treating code addresses or printable table bytes as text.
	for _, ds := range []int{0x2c0, 0x2e8, 0x36e, 0x4ff2, 0x5028, 0x502c, 0x5033, 0x503b, 0x5041, 0x5047, 0x504e, 0x5056, 0x505a, 0x506a, 0x5076, 0x507b, 0x5084, 0x5090, 0x5095, 0x509e, 0x50ab, 0x50b0, 0x50b6, 0x50bc, 0x50bf, 0x50c8, 0x50d0, 0x50df, 0x50e2, 0x50e8, 0x50f2, 0x50f5, 0x510c, 0x511f, 0x5134, 0x5139, 0x5144, 0x515d, 0x5f84, 0x5feb, 0x6051, 0x606a, 0x5b82, 0x5b88, 0x5b90, 0x5b96, 0x5d71, 0x5f4e} {
		s, err := officeSourceString(gs.Payload, ds)
		if err != nil {
			return d, err
		}
		d.Text[ds] = s.Text
		if _, err := read(ds, len(s.Text)+1); err != nil {
			return d, err
		}
	}
	// Independent far-string bank: section3 feedback consumes these exact
	// four-byte offset/segment records. Canonical bank293f is file3b360;
	// it is not a resident address under the A-e000 mapping.
	d.Feedback = map[string][]string{}
	for _, t := range []struct {
		key    string
		off, n int
	}{{"success", 0xee9, 10}, {"failure", 0xf11, 6}, {"loss", 0xf29, 3}, {"no-return", 0xf35, 4}, {"partial", 0xf45, 8}, {"campaign-warning", 0xf65, 3}, {"friendly", 0xf71, 2}, {"retire-rank", 0xf79, 1}, {"retire-service", 0xf7d, 1}, {"retire-loss", 0xf81, 1}} {
		for i := 0; i < t.n; i++ {
			at := l.FeedbackBankFile + t.off + 4*i
			if at+4 > len(gs.Payload) || le16(gs.Payload[at+2:]) != l.FeedbackBankSegment {
				return d, fmt.Errorf("debrief feedback bank identity")
			}
			start := l.FeedbackBankFile + le16(gs.Payload[at:])
			end := start
			for end < len(gs.Payload) && end-start < 1024 && gs.Payload[end] != 0 {
				end++
			}
			if end == len(gs.Payload) || end-start >= 1024 {
				return d, fmt.Errorf("debrief feedback string boundary")
			}
			d.Feedback[t.key] = append(d.Feedback[t.key], string(gs.Payload[start:end]))
			for _, v := range [][2]int{{at, 4}, {start, end - start + 1}} {
				src := gs.Source
				src.Kind, src.Offset, src.Length, src.PayloadSHA256 = "executable_slice", uint64(v[0]), uint64(v[1]), Hash(gs.Payload[v[0]:v[0]+v[1]])
				d.Sources = append(d.Sources, src)
			}
		}
	}
	for _, name := range debriefSceneNames {
		s, e := parseDebriefScene(resources[name])
		if e != nil {
			return d, e
		}
		d.Scenes[name] = s
		d.Sources = append(d.Sources, s.Source)
	}
	return d, validateDebriefMetadata(d, false)
}

func exportDebriefMetadata(out *OutputWriter, alloc *pathAllocator, m *Manifest) error {
	if m.OfficeMetadata == "" {
		return nil
	}
	p, e := recognizeOfficeProfile(m.Resources)
	if e != nil {
		return nil
	}
	resources, gs := missionResources(m)
	d, e := prepareDebriefMetadata(resources, gs, p)
	if e != nil {
		return e
	}
	for _, name := range debriefImageNames {
		r := resources[name]
		if r == nil || r.Conversion.Width != 320 || r.Conversion.Height != 200 {
			return fmt.Errorf("debrief picture%s missing or invalid", name)
		}
		o, e := preparedOutput(r.Outputs, "decoded_indices")
		if e != nil {
			return e
		}
		d.Images = append(d.Images, debriefImage{name, r.Source, o})
	}
	for _, name := range debriefPaletteNames {
		r := resources[name]
		if r == nil {
			return fmt.Errorf("debrief palette%s missing", name)
		}
		o, e := preparedOutput(r.Outputs, "exact_rgb6_components")
		if e != nil {
			return e
		}
		d.Palettes = append(d.Palettes, missionPreparedPalette{name, r.Source, o})
	}
	if e = validateDebriefMetadata(d, true); e != nil {
		return e
	}
	sort.Slice(d.Sources, func(i, j int) bool {
		if ap, bp := strings.ToUpper(d.Sources[i].Path), strings.ToUpper(d.Sources[j].Path); ap != bp {
			return ap < bp
		}
		return d.Sources[i].Offset < d.Sources[j].Offset
	})
	b, e := JsonBytes(d)
	if e != nil {
		return e
	}
	r := resources["JUMP.SCN"]
	if e = addGenerated(out, r, alloc, ".debrief.json", "mission_result_metadata", b); e != nil {
		return e
	}
	m.DebriefMetadata = r.Outputs[len(r.Outputs)-1].Path
	return nil
}

func validateDebriefMetadata(d debriefMetadata, outputs bool) error {
	scenes, images, palettes := debriefSceneNames, debriefImageNames, debriefPaletteNames
	if d.Schema <= 3 {
		images = images[:len(images)-4]
		palettes = palettes[:len(palettes)-2]
	}
	if d.Schema == 2 {
		scenes = scenes[:len(scenes)-1]
		images = images[:len(images)-2]
	}
	if d.Schema != 2 && d.Schema != 3 && d.Schema != debriefSchema || d.Profile != IslandIce469085.Name || len(d.Scenes) != len(scenes) || len(d.Sources) == 0 {
		return fmt.Errorf("incompatible debrief metadata; rebuild assets")
	}
	for _, s := range scenes {
		v, ok := d.Scenes[s]
		if !ok || v.Name != s || (s != "CREWPRO.SCN" && s != "RESCUE.SCN" && len(v.Areas) == 0) {
			return fmt.Errorf("debrief scene%s incomplete", s)
		}
		for _, l := range v.Lori {
			if l.Divisor <= 0 || l.Cell[0] <= 0 || l.Cell[1] <= 0 || l.Mode < 0 || l.Mode > 1 {
				return fmt.Errorf("debrief LORI invalid")
			}
		}
	}
	if len(d.Scenes["RESCUE.SCN"].Components) != 197 || len(d.Scenes["KIA.SCN"].Lori) != 1 || len(d.Scenes["CREWPRO.SCN"].Lori) != 3 {
		return fmt.Errorf("N4 debrief source animation descriptors incomplete")
	}
	for _, s := range d.RankNames {
		if s == "" {
			return fmt.Errorf("debrief rank label missing")
		}
	}
	for i, s := range d.MedalCitations {
		if s == "" || d.MedalNames[i] == "" {
			return fmt.Errorf("debrief medal label missing")
		}
	}
	for _, ds := range []int{0x4d52, 0x4d6b, 0x4d91, 0x4e6e, 0x4eb3, 0x4f48, 0x51c4, 0x525a, 0x527b, 0x52cb, 0x5311, 0x5375, 0x53c4, 0x5417, 0x5478, 0x5511, 0x2c0, 0x2e8, 0x36e, 0x4ff2, 0x505a, 0x506a, 0x5084, 0x509e, 0x50f5, 0x510c, 0x511f, 0x5134, 0x5139, 0x5144, 0x515d, 0x5f84, 0x5feb, 0x6051, 0x606a} {
		if d.Text[ds] == "" {
			return fmt.Errorf("debrief literal%x missing", ds)
		}
	}
	for key, n := range map[string]int{"success": 10, "failure": 6, "partial": 8, "loss": 3, "no-return": 4, "friendly": 2, "retire-rank": 1, "retire-service": 1, "retire-loss": 1, "campaign-warning": 3} {
		if len(d.Feedback[key]) != n {
			return fmt.Errorf("debrief feedback table%s incomplete", key)
		}
	}
	if outputs && (len(d.Images) != len(images) || len(d.Palettes) != len(palettes)) {
		return fmt.Errorf("debrief images/palettes incomplete")
	}
	if d.Schema >= 3 {
		for _, ds := range []int{0x5b82, 0x5b88, 0x5b90, 0x5b96, 0x5d71, 0x5f4e} {
			if d.Text[ds] == "" {
				return fmt.Errorf("replacement source literal%x missing", ds)
			}
		}
		if len(d.Scenes["REPLACE.SCN"].Areas) != 2 || len(d.Scenes["REPLACE.SCN"].Lori) != 1 {
			return fmt.Errorf("replacement scene source ABI incomplete")
		}
	}
	return nil
}

func loadDebriefAssets(reader *preparedReader, m Manifest) (*DebriefAssets, error) {
	o, ok := reader.outputs[m.DebriefMetadata]
	if !ok || o.Kind != "mission_result_metadata" {
		return nil, fmt.Errorf("prepared assets lack StepN debrief metadata; prepare a staged upgrade")
	}
	raw, e := reader.verified(o, 4<<20)
	if e != nil {
		return nil, e
	}
	var d debriefMetadata
	if e = DecodeObject(raw, &d); e != nil {
		return nil, e
	}
	if e = validateDebriefMetadata(d, true); e != nil {
		return nil, e
	}
	a := &DebriefAssets{Scenes: d.Scenes, Images: map[string][]byte{}, Palettes: map[string]color.Palette{}, Text: d.Text, Feedback: d.Feedback, RankThresholds: d.RankThresholds, MedalThresholds: d.MedalThresholds, MedalPenaltyCredits: d.MedalPenaltyCredits, RankNames: d.RankNames, MedalNames: d.MedalNames, MedalCitations: d.MedalCitations, WingQualifiers: d.WingQualifiers, WingLevels: d.WingLevels}
	for _, v := range d.Images {
		if _, ok := a.Images[v.Name]; ok {
			return nil, fmt.Errorf("duplicate debrief image")
		}
		b, e := reader.verified(v.Indices, 64000)
		if e != nil {
			return nil, e
		}
		if len(b) != 64000 {
			return nil, fmt.Errorf("debrief image size")
		}
		a.Images[v.Name] = b
	}
	for _, v := range d.Palettes {
		if _, ok := a.Palettes[v.Name]; ok {
			return nil, fmt.Errorf("duplicate debrief palette")
		}
		b, e := reader.verified(v.Components, 768)
		if e != nil {
			return nil, e
		}
		if len(b) != 768 {
			return nil, fmt.Errorf("debrief palette size")
		}
		pal := make(color.Palette, 256)
		for i := range pal {
			q := b[3*i : 3*i+3]
			if q[0] > 63 || q[1] > 63 || q[2] > 63 {
				return nil, fmt.Errorf("debrief RGB6 component")
			}
			pal[i] = color.NRGBA{q[0]<<2 | q[0]>>4, q[1]<<2 | q[1]>>4, q[2]<<2 | q[2]>>4, 255}
		}
		a.Palettes[v.Name] = pal
	}
	return a, nil
}

// Lori resolves the original signed-divide atlas grid. Mode0 is opaque;
// Mode1 preserves source index0. The caller owns its destination framebuffer.
func (a *DebriefAssets) Lori(dst []byte, scene string, key, selector, x, y int) error {
	if a == nil {
		return fmt.Errorf("debrief assets unavailable")
	}
	s, ok := a.Scenes[scene]
	if !ok {
		return fmt.Errorf("debrief scene unavailable")
	}
	for _, l := range s.Lori {
		if l.Key != key {
			continue
		}
		if selector < 0 || l.Divisor <= 0 {
			return fmt.Errorf("debrief LORI selector")
		}
		r := [4]int{l.Base[0] + selector/l.Divisor*l.Cell[0], l.Base[1] + selector%l.Divisor*l.Cell[1], l.Cell[0], l.Cell[1]}
		return OfficeBlit(a.Images[l.SourceAsset], dst, r, x, y, l.Mode != 0)
	}
	return fmt.Errorf("debrief LORI key%d missing", key)
}
