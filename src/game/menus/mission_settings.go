package menus

import (
	assetdata "gunship2000-assets/game/assets"
	gamestate "gunship2000-assets/game/state"
)

func settingsHit(a *assetdata.MissionAssets, x, y int) int {
	areas := a.Metadata.Screens["REALITY.SCN"].Areas
	for i := len(areas) - 1; i >= 0; i-- {
		if areas[i].Key > 0 && insideRect(x, y, areas[i].Rectangle) {
			return areas[i].Key
		}
	}
	return -1
}

func renderPreflight(a *assetdata.MissionAssets, settings gamestate.PreflightSettings, hover int) []byte {
	return renderCockpit(a, settings, CockpitState{}, hover)
}

func renderCockpit(a *assetdata.MissionAssets, settings gamestate.PreflightSettings, cockpit CockpitState, hover int) []byte {
	return renderCockpitWithHelp(a, settings, cockpit, hover, SettingsHelpState{})
}

func renderCockpitWithHelp(a *assetdata.MissionAssets, settings gamestate.PreflightSettings, cockpit CockpitState, hover int, help SettingsHelpState) []byte {
	frame := append([]byte(nil), a.Images["REALITY.PIC"]...)
	if len(frame) != 64000 {
		return make([]byte, 64000)
	}
	values := settings.Values()
	for _, chunk := range a.Metadata.Screens["REALITY.SCN"].Chunks {
		raw := chunk.Payload
		switch chunk.Tag {
		case "ANIM":
			if len(raw) != 38 {
				continue
			}
			key, choice := int(raw[0]), 0
			if key < 7 {
				choice = values[key]
			} else if key == 8 {
				choice = int(cockpit.APU)
			} else if key == 9 || key == 10 {
				choice = int(cockpit.Knobs[key-9])
			}
			if int(raw[25]) <= choice || raw[25] == 0 {
				continue
			}
			// A10614 selects the stored frame through the per-choice mapping,
			// rather than treating choice zero as atlas frame zero.
			index := int(raw[26+choice])
			divisor := assetdata.OfficeSignedWord(raw[9:])
			if divisor <= 0 {
				continue
			}
			w, h := assetdata.OfficeSignedWord(raw[5:]), assetdata.OfficeSignedWord(raw[7:])
			r := [4]int{assetdata.OfficeSignedWord(raw[1:]) + index/divisor*w, assetdata.OfficeSignedWord(raw[3:]) + index%divisor*h, w, h}
			_ = assetdata.OfficeBlit(a.Images["REALI_AN.PIC"], frame, r, assetdata.OfficeSignedWord(raw[11:]), assetdata.OfficeSignedWord(raw[13:]), false)
		case "GRAF":
			if len(raw) == 18 {
				drawDifficultyBar(a.Images["REALI_AN.PIC"], frame, raw, cockpit.Bar)
			}
		}
	}
	for _, chunk := range a.Metadata.Screens["REALITY.SCN"].Chunks {
		if chunk.Tag != "LABL" || len(chunk.Payload) < 12 {
			continue
		}
		r := chunk.Payload
		colour := int(r[2])
		if r[8] == 1 && int(r[9]) == hover {
			colour = int(r[3])
		}
		if r[8] == 2 && int(r[9]) < 7 && values[int(r[9])] == assetdata.OfficeSignedWord(r[10:]) {
			colour = int(r[3])
		}
		if font := a.Fonts[int(r[1])]; font != nil {
			_ = font.Draw(frame, r[12:], assetdata.OfficeSignedWord(r[4:]), assetdata.OfficeSignedWord(r[6:]), colour, 0, 0)
		}
	}
	assetdata.OfficeFill(frame, 0, 191, 320, 9, 0)
	if font := a.Fonts[3]; font != nil {
		assetdata.DrawBottomTooltip(frame, font, a.Metadata.Templates[0x4cc9], 7)
	}
	drawSettingsHelpMode(frame, a, settings, hover, help, cockpit.APU)
	return frame
}

func drawSettingsHelp(frame []byte, a *assetdata.MissionAssets, settings gamestate.PreflightSettings, hover int) {
	drawSettingsHelpMode(frame, a, settings, hover, SettingsHelpState{}, 0)
}

func drawSettingsHelpMode(frame []byte, a *assetdata.MissionAssets, settings gamestate.PreflightSettings, hover int, state SettingsHelpState, apu byte) {
	if a == nil || a.Preflight == nil || len(a.Preflight.SettingsHelp) != 9 {
		return
	}
	font := a.Fonts[2]
	if font == nil {
		return
	}
	// Player AREA keys are1..9; the recovered MFD table uses zero-based0..8.
	key := hover - 1
	if key < 0 || key > 8 {
		return
	}
	help := a.Preflight.SettingsHelp[key]
	clip := [4]int{200, 8, 100, 56}
	assetdata.OfficeFill(frame, clip[0], clip[1], clip[2], clip[3], 0)
	y := 8
	advance := font.Metrics.Height + font.Metrics.Spacing
	if advance < 1 {
		advance = 1
	}
	paragraph := func(text string, colour int) {
		for _, line := range settingsHelpLines(font, text, clip[2]) {
			if y < clip[1]+clip[3] {
				_ = font.DrawClipped(frame, []byte(line), 200, y, colour, 0, 0, clip)
			}
			y += advance
		}
	}
	// C003a passes style1 to C0578: the heading is centered; body C01e0
	// passes style0. Original source strings (including double spaces) survive.
	if help.Heading != "" {
		x := clip[0] + (clip[2]-font.Measure([]byte(help.Heading)))/2
		_ = font.DrawClipped(frame, []byte(help.Heading), x, y, 41, 0, 0, clip)
		y += advance
	}
	if key == 7 {
		return
	} // The source intentionally omits the blank eighth body.
	if state.Detailed && key < 7 {
		value := settings.Values()[key]
		if value >= 0 && value < len(help.Details) {
			paragraph(help.Details[value], 41)
		}
		return
	}
	paragraph(help.Body, 36)
	if key < 7 || key == 8 {
		value := int(apu)
		if key < 7 {
			value = settings.Values()[key]
		}
		if value >= 0 && value < len(help.Choices) {
			paragraph(help.Choices[value], 41)
		}
	}
}
