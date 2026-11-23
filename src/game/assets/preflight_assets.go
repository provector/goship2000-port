package assets

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sort"
	"strings"
)

const preflightMetadataSchema = 4

// SHIP keeps its five-byte fit choices and thirteen-byte composition records in
// original order. IDs and array ordinals are deliberately separate.
type ShipFit struct {
	System, Quantity, Station, Preview int
}

type ShipComponent struct {
	Group       int
	Source      [4]int
	Destination [2]int
}

type ShipPresentation struct {
	Aircraft   int
	Fits       []ShipFit
	Components []ShipComponent
	Source     SourceLocation
}

type PreflightAssets struct {
	Schema           int
	Ships            map[int]ShipPresentation
	Difficulty       []int16
	SystemOrder      []int
	SquadronNames    []string
	OutfitRanks      []string
	AwardNames       []string
	Text             map[int]string
	Sources          []SourceLocation
	SettingsHelp     []SettingsHelp
	StationMarkers   []StationMarker
	ObjectiveSummary *OutfitObjectiveText
}

// OUTFIT reads the shortened TARGET name after its six-byte coordinate prefix.
// Action labels belong to raw objective kinds, not mapped drawing categories.
type OutfitObjectiveText struct {
	Headers       [2]string
	RecordFormat  string
	Verbs         []string
	Abbreviations [][2]string
}

// Template identifies the global POINTER handle, not an ORDNANCE SPRT ordinal.
// The latter describes unrelated screen resources and is not the marker source.
type StationMarker struct {
	Template int
	Source   [4]int
}

type SettingsHelp struct {
	Key           int
	Heading, Body string
	Choices       []string
	Details       []string // Same source value slots; selected/changed feedback replaces generic help.
}

func preflightPictureNames() []string {
	names := []string{"POINTER.PIC", "PILOT.PIC", "MENU3037.PIC", "MENU0021.PIC", "MENU5057.PIC", "MENU8085.PIC"}
	for i := 0; i < 8; i++ {
		names = append(names, fmt.Sprintf("SHIP_%02d.PIC", i))
	}
	for i := 0; i < 5; i++ {
		names = append(names, fmt.Sprintf("TARMC_%02d.PIC", i))
	}
	return names
}

func preparePreflightAssets(gs *Resource, resources map[string]*Resource, profile *AssetVersionProfile) (*PreflightAssets, error) {
	p := &PreflightAssets{Schema: preflightMetadataSchema, Ships: map[int]ShipPresentation{}, Text: map[int]string{}}
	var systemFiles []string
	for name := range resources {
		if strings.HasPrefix(name, "WEAP_") {
			systemFiles = append(systemFiles, name)
		}
	}
	sort.Strings(systemFiles)
	for _, name := range systemFiles {
		cs, err := officeChunks(resources[name].Payload, resources[name].Source.Offset)
		if err != nil {
			return nil, err
		}
		for _, c := range cs {
			if c.Tag == "WSHD" && len(c.Payload) == 214 {
				p.SystemOrder = append(p.SystemOrder, int(int8(c.Payload[0])))
			}
		}
	}
	for id := 0; id < 8; id++ {
		r := resources[fmt.Sprintf("SHIP_%02d.SHP", id)]
		if r == nil {
			return nil, fmt.Errorf("preflight SHIP%d missing", id)
		}
		cs, err := officeChunks(r.Payload, r.Source.Offset)
		if err != nil {
			return nil, err
		}
		ship := ShipPresentation{Aircraft: id, Source: r.Source}
		for _, c := range cs {
			b := c.Payload
			switch c.Tag {
			case "WEAP":
				if len(b) != 5 {
					return nil, fmt.Errorf("%s invalid WEAP width", r.name)
				}
				ship.Fits = append(ship.Fits, ShipFit{int(int8(b[0])), OfficeSignedWord(b[1:]), int(int8(b[3])), int(int8(b[4]))})
			case "COMP":
				if len(b) != 13 {
					return nil, fmt.Errorf("%s invalid COMP width", r.name)
				}
				var component ShipComponent
				component.Group = int(int8(b[0]))
				for i := range component.Source {
					component.Source[i] = OfficeSignedWord(b[1+2*i:])
				}
				component.Destination = [2]int{OfficeSignedWord(b[9:]), OfficeSignedWord(b[11:])}
				ship.Components = append(ship.Components, component)
			}
		}
		p.Ships[id] = ship
		p.Sources = append(p.Sources, r.Source)
	}
	layout := profile.PreflightLayout
	var err error
	p.StationMarkers, err = prepareStationMarkers(gs, profile, &p.Sources)
	if err != nil {
		return nil, err
	}
	p.ObjectiveSummary, err = prepareOutfitObjectiveText(gs, profile, &p.Sources)
	if err != nil {
		return nil, err
	}
	for _, ds := range layout.AdditionalStringsDS {
		s, err := officeSourceString(gs.Payload, ds)
		if err != nil {
			return nil, err
		}
		p.Text[ds] = s.Text
	}
	// The OUTFIT rank formatter uses the short table, not briefing's full ranks.
	for _, table := range []struct {
		ds, count int
		output    *[]string
	}{
		{layout.OutfitRankPointerDS, 12, &p.OutfitRanks},
		{layout.AwardPointerDS, 8, &p.AwardNames},
	} {
		at := profile.OfficeLayout.DataFileAnchor + table.ds
		if at < 0 || at+2 > len(gs.Payload) {
			return nil, fmt.Errorf("preflight label pointer outside source profile")
		}
		pointer := profile.OfficeLayout.DataFileAnchor + le16(gs.Payload[at:])
		if pointer < 0 || pointer+2*table.count > profile.OfficeLayout.DataFileEnd || pointer+2*table.count > len(gs.Payload) {
			return nil, fmt.Errorf("preflight label table outside source profile")
		}
		for i := 0; i < table.count; i++ {
			value, err := officeSourceString(gs.Payload, le16(gs.Payload[pointer+2*i:]))
			if err != nil {
				return nil, err
			}
			*table.output = append(*table.output, value.Text)
		}
	}
	nameTable := profile.OfficeLayout.DataFileAnchor + layout.SquadronNamesDS
	if nameTable < 0 || nameTable+148 > len(gs.Payload) || nameTable+148 > profile.OfficeLayout.DataFileEnd {
		return nil, fmt.Errorf("squadron name table outside supported profile")
	}
	for i := 0; i < 74; i++ {
		ds := le16(gs.Payload[nameTable+2*i:])
		s, err := officeSourceString(gs.Payload, ds)
		if err != nil {
			return nil, err
		}
		p.SquadronNames = append(p.SquadronNames, s.Text)
	}
	at := profile.OfficeLayout.DataFileAnchor + layout.DifficultyDS
	if at < 0 || at+layout.DifficultyBytes > len(gs.Payload) || at+layout.DifficultyBytes > profile.OfficeLayout.DataFileEnd || layout.DifficultyBytes%2 != 0 {
		return nil, fmt.Errorf("preflight difficulty table outside supported source profile")
	}
	for i := 0; i < layout.DifficultyBytes; i += 2 {
		p.Difficulty = append(p.Difficulty, int16(binary.LittleEndian.Uint16(gs.Payload[at+i:])))
	}
	for ds := layout.StringsDS; ds < layout.StringsEndDS; ds++ {
		at := profile.OfficeLayout.DataFileAnchor + ds
		if at >= len(gs.Payload) || at >= profile.OfficeLayout.DataFileEnd {
			return nil, fmt.Errorf("preflight strings outside supported source profile")
		}
		if ds != layout.StringsDS && gs.Payload[at-1] != 0 {
			continue
		}
		s, err := officeSourceString(gs.Payload, ds)
		if err != nil {
			return nil, err
		}
		p.Text[ds] = s.Text
	}
	if profile.CockpitLayout != nil {
		var err error
		p.SettingsHelp, err = prepareSettingsHelp(gs.Payload, profile)
		if err != nil {
			return nil, err
		}
	}
	return p, validatePreflightAssets(p)
}

func prepareStationMarkers(gs *Resource, profile *AssetVersionProfile, sources *[]SourceLocation) ([]StationMarker, error) {
	l := profile.PreflightLayout
	read := func(at, n int) ([]byte, error) {
		if at < 0 || at+n > len(gs.Payload) {
			return nil, fmt.Errorf("station-marker source outside supported profile")
		}
		b := gs.Payload[at : at+n]
		s := gs.Source
		s.Kind, s.Offset, s.Length, s.PayloadSHA256 = "executable_slice", uint64(at), uint64(n), Hash(b)
		*sources = append(*sources, s)
		return b, nil
	}
	init, err := read(l.PointerInitializerFile, 0x6e)
	if err != nil {
		return nil, err
	}
	draw, err := read(l.StationMarkerConsumerFile, 0x5e)
	if err != nil {
		return nil, err
	}
	// Check the caller's extraction operands and indexed handle load before
	// compiling them to ordinary atlas rectangles. No DOS handles remain at run time.
	if !bytes.Equal(init[0x2b:0x2e], []byte{0x83, 0x7e, 0xfe}) || init[0x31] != 0x6a || init[0x33] != 0x6a || init[0x38] != 0xb9 ||
		!bytes.Equal(init[0x59:0x5d], []byte{0xd1, 0xe3, 0x89, 0x87}) || !bytes.Equal(draw[0x48:0x4c], []byte{0xd1, 0xe3, 0xff, 0xb7}) {
		return nil, fmt.Errorf("station-marker instruction operands incompatible with source profile")
	}
	height, width, columns, count := int(init[0x32]), int(init[0x34]), le16(init[0x39:]), int(init[0x2e])
	base, first := le16(init[0x5d:]), le16(draw[0x4c:])
	if width != 16 || height != 16 || columns <= 0 || first < base || (first-base)%2 != 0 {
		return nil, fmt.Errorf("station-marker template layout incompatible with source profile")
	}
	var result []StationMarker
	for i := 0; i < 2; i++ {
		template := (first-base)/2 + i
		r := [4]int{template % columns * width, template / columns * height, width, height}
		if template >= count || !OfficeRectFits(r) {
			return nil, fmt.Errorf("station-marker template outside POINTER atlas")
		}
		result = append(result, StationMarker{Template: template, Source: r})
	}
	return result, nil
}

func prepareSettingsHelp(gs []byte, profile *AssetVersionProfile) ([]SettingsHelp, error) {
	l := profile.CockpitLayout
	if l == nil {
		return nil, fmt.Errorf("source profile has no settings help tables")
	}
	readPointer := func(ds int) (int, error) {
		at := profile.OfficeLayout.DataFileAnchor + ds
		if at < 0 || at+2 > len(gs) || at+2 > profile.OfficeLayout.DataFileEnd {
			return 0, fmt.Errorf("settings help pointer outside source profile")
		}
		return le16(gs[at:]), nil
	}
	text := func(ds int) (string, error) { s, e := officeSourceString(gs, ds); return s.Text, e }
	result := make([]SettingsHelp, 9)
	for key := range result {
		r := SettingsHelp{Key: key}
		h, e := readPointer(l.HeadingTableDS + 2*key)
		if e != nil {
			return nil, e
		}
		r.Heading, e = text(h)
		if e != nil {
			return nil, e
		}
		if key != 7 {
			p, e := readPointer(l.BodyTableDS + 2*key)
			if e != nil {
				return nil, e
			}
			r.Body, e = text(p)
			if e != nil {
				return nil, e
			}
		}
		if key < 7 || key == 8 {
			for value := 0; value < 4; value++ {
				p, e := readPointer(l.ChoiceTableDS + 8*key + 2*value)
				if e != nil {
					return nil, e
				}
				if p == 0 {
					break
				}
				s, e := text(p)
				if e != nil {
					return nil, e
				}
				r.Choices = append(r.Choices, s)
				if key == 8 {
					continue
				} // APU has three short states, but always uses the generic body.
				pointer, e := readPointer(l.DetailedChoiceTableDS + 8*key + 2*value)
				if e != nil {
					return nil, e
				}
				if pointer == 0 {
					return nil, fmt.Errorf("settings detailed help missing at key%d value%d", key, value)
				}
				detail, e := text(pointer)
				if e != nil {
					return nil, e
				}
				r.Details = append(r.Details, detail)
			}
		}
		result[key] = r
	}
	return result, nil
}

func validatePreflightAssets(p *PreflightAssets) error {
	if p == nil || p.Schema != preflightMetadataSchema || len(p.Ships) != 8 || len(p.Difficulty) != 33 || len(p.SystemOrder) != 28 || len(p.SquadronNames) != 74 || len(p.OutfitRanks) != 12 || len(p.AwardNames) != 8 || len(p.Text) == 0 {
		return fmt.Errorf("preflight metadata is missing/incompatible; rebuild assets")
	}
	if p.ObjectiveSummary == nil || len(p.ObjectiveSummary.Verbs) != 8 || len(p.ObjectiveSummary.Abbreviations) != 4 || p.ObjectiveSummary.RecordFormat != "%02d/%02d %s %Fs" || p.ObjectiveSummary.Headers[0] != "%cPRIMARY:%c  %s" || p.ObjectiveSummary.Headers[1] != "%cSECONDARY:%c  %s" {
		return fmt.Errorf("OUTFIT objective text metadata missing/incompatible; rebuild assets")
	}
	if len(p.StationMarkers) != 2 {
		return fmt.Errorf("station-marker metadata missing; rebuild assets")
	}
	for _, marker := range p.StationMarkers {
		if marker.Template < 0 || marker.Source[2] != 16 || marker.Source[3] != 16 || !OfficeRectFits(marker.Source) {
			return fmt.Errorf("station-marker atlas bounds invalid; rebuild assets")
		}
	}
	if p.Text[0x3d34] != "NO BAY" || p.Text[0x3d3b] != "EMPTY" {
		return fmt.Errorf("runtime-store sentinel source labels missing/incompatible; rebuild assets")
	}
	if len(p.SettingsHelp) != 9 {
		return fmt.Errorf("settings help metadata missing; rebuild assets")
	}
	for key, count := range []int{3, 4, 2, 2, 2, 2, 2, 0, 3} {
		r := p.SettingsHelp[key]
		detailCount := count
		if key == 8 {
			detailCount = 0
		}
		if r.Key != key || len(r.Choices) != count || len(r.Details) != detailCount || key != 7 && (r.Heading == "" || r.Body == "") {
			return fmt.Errorf("settings help key%d incomplete/incompatible; rebuild assets", key)
		}
		for value := 0; value < count; value++ {
			if r.Choices[value] == "" || (key < 7 && r.Details[value] == "") {
				return fmt.Errorf("settings help key%d value%d missing; rebuild assets", key, value)
			}
		}
	}
	ids := map[int]bool{}
	for _, id := range p.SystemOrder {
		if id < 0 || id > 127 || ids[id] {
			return fmt.Errorf("preflight definition order has duplicate/invalid ID%d", id)
		}
		ids[id] = true
	}
	for id := 0; id < 8; id++ {
		s, ok := p.Ships[id]
		if !ok || s.Aircraft != id || len(s.Fits) == 0 || len(s.Fits) > 50 || len(s.Components) == 0 || len(s.Components) > 150 {
			return fmt.Errorf("preflight SHIP%d metadata incomplete", id)
		}
		groups := map[int]bool{}
		for _, c := range s.Components {
			if c.Group < -1 || !OfficeRectFits(c.Source) || c.Source[2] <= 0 || c.Source[3] <= 0 {
				return fmt.Errorf("SHIP%d COMP exceeds source atlas", id)
			}
			groups[c.Group] = true
		}
		for _, f := range s.Fits {
			if f.System < 0 || f.Station < 0 || f.Station > 3 || f.Quantity < 0 || (f.Preview != -1 && !groups[f.Preview]) {
				return fmt.Errorf("SHIP%d WEAP has invalid role/group", id)
			}
		}
	}
	return nil
}

// Ice scenes transform only the temporary SHIP atlas. Retain prepared indices
// unchanged: this is scene-specific drawing state, not another disk decoder.
func PreflightShipPixels(a *MissionAssets, aircraft, scene int) []byte {
	pixels := bytes.Clone(a.Images[fmt.Sprintf("SHIP_%02d.PIC", aircraft)])
	if scene == 4 || scene == 5 {
		for i, v := range pixels {
			switch {
			case v >= 69 && v <= 79:
				pixels[i] = byte((20*int(v)-1380)/11 - 24)
			case v >= 192 && v <= 223:
				pixels[i] = v + 32
			}
		}
	}
	return pixels
}
