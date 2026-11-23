package menus

import (
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	gamestate "gunship2000-assets/game/state"
	"strings"
)

// A18e46 builds TARGET names from raw kind and FMHD; A18d94 shortens the
// complete name before A21d18 skips its six-byte coordinate prefix. Paper text
// has a separate OBJECT-name override and must not replace this producer.
func (v *OutfitView) objectiveSummary(role int, o gamestate.MissionObjective) string {
	if raw := v.Mission.SourcePreparation; raw != nil && len(raw.TargetBlock) == 5*62 {
		return assetdata.OfficeCString(raw.TargetBlock[(role+2)*62+6 : (role+2)*62+32])
	}
	t := v.Assets.Preflight.ObjectiveSummary
	if t == nil || o.RawKind < 0 || o.RawKind >= len(t.Verbs) {
		return ""
	}
	name := o.Name
	if w := v.Mission.SourceWorld; w != nil && o.Formation >= 0 && o.Formation < len(w.FMHD)/39 {
		name = assetdata.OfficeCString(w.FMHD[o.Formation*39 : o.Formation*39+25])
	}
	format := strings.ReplaceAll(t.RecordFormat, "%Fs", "%s")
	text := fmt.Sprintf(format, gamestate.MissionGrid(o.Point.X), gamestate.MissionGrid(o.Point.Y), t.Verbs[o.RawKind], name)
	for len(text) > 31 {
		changed := false
		for _, pair := range t.Abbreviations {
			if pair[0] != "" && len(pair[1]) < len(pair[0]) && strings.Contains(text, pair[0]) {
				text = strings.Replace(text, pair[0], pair[1], 1)
				changed = true
				break
			}
		}
		if !changed {
			text = text[:31]
			break
		}
	}
	if len(text) <= 6 {
		return ""
	}
	return text[6:]
}

func (v *OutfitView) objectiveLines() []string {
	t := v.Assets.Preflight.ObjectiveSummary
	if t == nil {
		return nil
	} // Incomplete synthetic test carriers have no source text.
	var lines []string
	for i, role := range []string{"primary", "secondary"} {
		description := ""
		if o, ok := v.Mission.Objectives[role]; ok {
			description = v.objectiveSummary(i, o)
		}
		// Source %c toggles configured15/10 colours without consuming arguments.
		header := strings.Replace(t.Headers[i], "%c", "\x8f", 1)
		header = strings.Replace(header, "%c", "\x8a", 1)
		text := fmt.Sprintf(header, description)
		wrapped := ordnanceWrap(v.Assets.Fonts[3], text, 100)
		for row := range wrapped {
			if row != 0 {
				wrapped[row] = "\x8a" + wrapped[row]
			}
		}
		lines = append(lines, wrapped...)
	}
	return lines
}
