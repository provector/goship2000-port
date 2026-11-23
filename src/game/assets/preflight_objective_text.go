package assets

import (
	"encoding/binary"
	"fmt"
)

func prepareOutfitObjectiveText(gs *Resource, profile *AssetVersionProfile, sources *[]SourceLocation) (*OutfitObjectiveText, error) {
	l := profile.PreflightLayout
	read := func(ds, size int) ([]byte, error) {
		at := profile.OfficeLayout.DataFileAnchor + ds
		if ds < 0 || size < 0 || at+size > profile.OfficeLayout.DataFileEnd || at+size > len(gs.Payload) {
			return nil, fmt.Errorf("OUTFIT objective text outside supported initialized data")
		}
		b := gs.Payload[at : at+size]
		s := gs.Source
		s.Kind, s.Offset, s.Length, s.PayloadSHA256 = "executable_slice", uint64(at), uint64(size), Hash(b)
		*sources = append(*sources, s)
		return b, nil
	}
	text := func(ds int) (string, error) {
		s, err := officeSourceString(gs.Payload, ds)
		if err != nil {
			return "", err
		}
		if _, err = read(ds, len(s.Text)+1); err != nil {
			return "", err
		}
		return s.Text, nil
	}
	out := &OutfitObjectiveText{}
	var err error
	for i, ds := range l.OutfitHeaderDS {
		if out.Headers[i], err = text(ds); err != nil {
			return nil, err
		}
	}
	if out.RecordFormat, err = text(l.TargetNameFormatDS); err != nil {
		return nil, err
	}
	verbs, err := read(l.ObjectiveVerbsDS, 16)
	if err != nil {
		return nil, err
	}
	for i := 0; i < 8; i++ {
		s, err := text(int(binary.LittleEndian.Uint16(verbs[2*i:])))
		if err != nil {
			return nil, err
		}
		out.Verbs = append(out.Verbs, s)
	}
	from, err := read(l.ObjectiveAbbreviationDS[0], 8)
	if err != nil {
		return nil, err
	}
	to, err := read(l.ObjectiveAbbreviationDS[1], 8)
	if err != nil {
		return nil, err
	}
	for i := 0; i < 4; i++ {
		a, err := text(int(binary.LittleEndian.Uint16(from[2*i:])))
		if err != nil {
			return nil, err
		}
		b, err := text(int(binary.LittleEndian.Uint16(to[2*i:])))
		if err != nil {
			return nil, err
		}
		out.Abbreviations = append(out.Abbreviations, [2]string{a, b})
	}
	return out, nil
}
