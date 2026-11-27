package menus

import (
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/game/cockpit"
	gamestate "gunship2000-assets/game/state"
	"strings"
)

func (c *FrontEnd) Frame(pointer bool) []byte {
	if c.Screen == ScreenReplay {
		return c.replayFrame(pointer)
	}
	if c.Screen == ScreenStartup {
		pixels, _ := c.Startup.Current()
		return append([]byte(nil), pixels...)
	}
	if c.Screen == ScreenOffice {
		frame := c.Office.Frame(false)
		if pointer {
			c.Office.drawPointer(frame)
		}
		return frame
	}
	frame := c.frameBody()
	if pointer && c.Screen == ScreenMissionEditor {
		c.drawEditorPointer(frame)
	}
	if pointer && c.replacePointerPresented() {
		c.drawReplacementPointer(frame)
	} else if pointer && c.crewPointerPresented() {
		c.drawCrewPointer(frame)
	} else if pointer && !c.startupPresented() && !c.renderingPresented() && !c.sceneConfirmationPresented() && !c.sceneInitializationPresented() && c.Screen != ScreenDebrief && c.Screen != ScreenMissionEditor {
		if c.Screen == ScreenOrdnance && c.Mission != nil && c.Mission.Ordnance != nil {
			c.Mission.Ordnance.drawRowPointer(frame, c.Office.Assets.Images["POINTER.PIC"], c.Office.pointerFrame)
		} else {
			c.Office.drawPointer(frame)
		}
	}
	return frame
}

func (c *FrontEnd) text(frame []byte, font int, text string, x, y, colour int) {
	if f := c.Menus.Fonts[font]; f != nil {
		_ = f.Draw(frame, []byte(text), x, y, colour, 0, 0)
	}
}

func (c *FrontEnd) clippedText(frame []byte, font int, text string, x, y, colour int, clip [4]int) {
	if f := c.Menus.Fonts[font]; f != nil {
		_ = f.DrawClipped(frame, []byte(text), x, y, colour, 0, 0, clip)
	}
}

func (c *FrontEnd) centered(frame []byte, font int, text string, y, colour int) {
	if f := c.Menus.Fonts[font]; f != nil {
		_ = f.DrawCentered(frame, []byte(text), y, colour, 0, 0)
	}
}

func (c *FrontEnd) background(name string) []byte {
	return append([]byte(nil), c.Menus.Images[name]...)
}

func (c *FrontEnd) frameBody() []byte {
	var frame []byte
	switch c.Screen {
	case ScreenReplay:
		// Global Alt+Q freezes the reached GS2 viewer, not its parent office.
		// The saved indices and CurrentPalette must have the same scene owner.
		return c.replayFrame(false)
	case ScreenMissionFiles:
		return c.missionFilesFrame()
	case ScreenMissionEditor:
		return c.editorFrame()
	case ScreenFilms:
		return c.filmLibraryFrame()
	case ScreenDebrief:
		return c.debriefFrame()
	case ScreenScene:
		if c.Rendering != nil && c.Rendering.GPS != nil {
			return c.gpsFrame()
		}
		return c.renderingFrame()
	case ScreenSceneInitialization:
		return c.initializationFrame()
	case ScreenInterstitial, ScreenBriefing, ScreenPapers, ScreenSettings, ScreenPaperTransition, ScreenOutfit, ScreenOrdnance:
		frame = c.missionFrame()
	case ScreenStartup:
		pixels, _ := c.Startup.Current()
		frame = append([]byte(nil), pixels...)
	case ScreenOffice:
		frame = c.Office.Frame(false)
	case ScreenFolder:
		frame = append([]byte(nil), c.Office.Scene...)
		assetdata.OfficeFill(frame, 0, 191, 320, 9, 0)
		assetdata.DrawBottomTooltip(frame, c.Menus.Fonts[3], "PLEASE STAND BY", 7)
	case ScreenRoster:
		frame = c.background("ROSTER.PIC")
		c.drawRoster(frame)
		if c.Draft != nil {
			c.drawEditor(frame)
		}
	case ScreenTheatre:
		frame = c.background("THEATER.PIC")
		c.drawTheatre(frame)
	case ScreenMission:
		frame = c.background("MEMO.PIC")
		c.drawMission(frame)
	case ScreenRecovery:
		frame = c.background("MEMO.PIC")
		c.drawRecovery(frame)
	case ScreenQuit:
		frame = append([]byte(nil), c.quitBackground...)
		if c.sceneConfirmationPresented() {
			if err := cockpit.DrawSceneConfirmationScaled(frame, c.Office.Assets.Cockpit, c.sceneEndConfirmation, func() int {
				if len(frame) == 256000 {
					return 2
				}
				return 1
			}()); err != nil {
				c.reportDiagnostic("menu", err.Error(), err)
			}
		} else {
			if len(frame) > 256000 {
				var s cockpit.OverlayScratch
				_ = s.DrawRect(frame, nil, 1, cockpit.Rect{Left: 90, Top: 85, Right: 229, Bottom: 104}, func(dst []byte) error { c.drawQuit(dst); return nil })
			} else {
				c.drawQuit(frame)
			}
		}

	default:
		frame = c.Office.Frame(false)
	}
	return frame
}

func (c *FrontEnd) drawRoster(frame []byte) {
	state := c.rosterState()
	c.text(frame, 4, "SQUADRON RECORDS", 26, 77, 0)
	for i, p := range state.Slots {
		x, y := 23+(i/3)*124, 85+(i%3)*34
		if c.Highlight == i {
			// Clear only the interior; the source sheet owns the grid/artwork border.
			w, h := 123, 33
			if i >= 3 {
				w--
			}
			if i%3 == 2 {
				h--
			}
			assetdata.OfficeFill(frame, x+1, y+1, w, h, 15)
		}
		rank, name, unit, status := "WOC", "", "", "Replacement"
		if p == nil {
			c.drawEmblem(frame, 0, true, x+2, y+16)
		}
		if p != nil {
			rank = c.Office.Assets.Metadata.Ranks[p.Rank.Raw()].Text
			name = p.Name
			unit = p.Squadron
			status = rosterStatusLabel(p)
			c.drawEmblem(frame, p.Emblem, true, x+2, y+16)
		}
		c.clippedText(frame, 3, strings.TrimSpace(rank+" "+name), x+2, y+2, 0, [4]int{x, y, 124, 34})
		c.clippedText(frame, 3, unit, x+2, y+9, 0, [4]int{x, y, 124, 34})
		c.text(frame, 3, "STATUS:", x+22, y+16, 9)
		c.text(frame, 3, status, x+64, y+16, 9)
		score, missions := "?", "?"
		if p == nil {
			score, missions = "0", "0"
		} else if p.Statistics != nil {
			score = fmt.Sprint(p.Statistics.CareerScore)
			missions = fmt.Sprint(p.Statistics.Missions())
		}
		c.text(frame, 3, "SCORE:", x+22, y+22, 9)
		c.text(frame, 3, score, x+64, y+22, 9)
		c.text(frame, 3, "MISSIONS:", x+22, y+28, 9)
		c.text(frame, 3, missions, x+64, y+28, 9)
	}
	if c.Draft == nil && c.Highlight >= 0 && c.Highlight < 6 {
		p := state.Slots[c.Highlight]
		if p == nil {
			p = &gamestate.Pilot{Rank: gamestate.RankWOC, Statistics: &gamestate.PilotStatistics{}}
		}
		c.drawEmblem(frame, p.Emblem, false, 23, 33)
		rank := p.Rank.Raw()
		r := [4]int{188 + 50*(rank/6), 20 * (rank % 6), 50, 20}
		_ = assetdata.OfficeBlit(c.Menus.Images["PATCHES.PIC"], frame, r, 21, 10, true)
		c.clippedText(frame, 5, p.Name, 78, 10, 0, [4]int{78, 10, 200, 8})
		c.clippedText(frame, 5, p.Squadron, 78, 40, 0, [4]int{78, 40, 200, 8})
		c.drawPilotStatistics(frame, p)
		c.drawRosterCrew(frame, p)
	}

	assetdata.OfficeFill(frame, 0, 191, 320, 9, 0)
	// Retained original caption, GS roster-entry summary Step15. Native safe
	// Edit remains E, but its help is documented outside the original-only strip.
	prompt := originalRosterPrompt
	if c.Highlight >= 0 && c.Highlight < len(state.Slots) {
		if p := state.Slots[c.Highlight]; p != nil && p.Status != gamestate.StatusActive && p.Status != gamestate.StatusInactive {
			prompt = originalUnavailableRosterPrompt
		}
	}
	assetdata.DrawBottomTooltip(frame, c.Menus.Fonts[3], prompt, 7)
}

func (c *FrontEnd) drawEmblem(frame []byte, selector int, small bool, x, y int) {
	if selector < 0 || selector >= len(c.Menus.Emblems) {
		return
	}
	r := c.Menus.Emblems[selector].Source
	if small {
		r = c.Menus.Emblems[selector].Small
	}
	_ = assetdata.OfficeBlit(c.Menus.Images["PATCHES.PIC"], frame, r, x, y, true)
}

func (c *FrontEnd) drawEditor(frame []byte) {
	d := c.Draft
	// The clean roster sheet supplies the writing surface and its lower margin.
	c.clippedText(frame, 5, d.Pilot.Name, 25, 12, 0, [4]int{25, 12, 200, 8})
	c.drawEmblem(frame, d.Pilot.Emblem, false, 23, 20)
	c.clippedText(frame, 5, d.Pilot.Squadron, 25, 66, 0, [4]int{25, 66, 200, 8})
	if d.Stage != 1 {
		text, y := d.Pilot.Name, 12
		if d.Stage == 2 {
			text, y = d.Pilot.Squadron, 66
		}
		c.clippedText(frame, 5, "_", 25+c.Menus.Fonts[5].Measure([]byte(text)), y, 0, [4]int{25, y, 200, 8})
	}

	prompt := "Type your new name, then press ENTER.  ESC to cancel."
	if d.Stage == 1 {
		prompt = "Use the Arrow keys to choose your unit patch, then press ENTER.  ESC to cancel."
	}
	if d.Stage == 2 {
		prompt = "Enter the name of your unit, then press ENTER.  ESC to cancel."
	}
	assetdata.OfficeFill(frame, 0, 191, 320, 9, 0)
	assetdata.DrawBottomTooltip(frame, c.Menus.Fonts[3], prompt, 7)
}

func (c *FrontEnd) drawTheatre(frame []byte) {
	for _, t := range c.Menus.Theatres {
		r := t.Rectangle
		r[2]++
		r[3]++ // Native line endpoints are inclusive; hit areas remain unchanged.
		outline(frame, r, 4)
	}

	assetdata.OfficeFill(frame, 0, 191, 320, 9, 0)
	text := c.Menus.Strings["theatre_prompt"]
	if c.Highlight >= 0 && c.Highlight < len(c.Menus.Theatres) {
		text = c.Menus.Theatres[c.Highlight].Name
	}
	assetdata.DrawBottomTooltip(frame, c.Menus.Fonts[3], text, 7)
}

func (c *FrontEnd) drawMission(frame []byte) {
	p := c.Store.State().SelectedPilot()
	if p == nil {
		return
	}
	name := strings.TrimSpace(p.Name)
	if fields := strings.Fields(name); len(fields) > 0 {
		name = fields[len(fields)-1]
	}
	rank := c.Menus.FullRanks[p.Rank.Raw()]
	c.text(frame, 4, "TO:", 77, 15, 0)
	c.text(frame, 4, rank+" "+name, 117, 15, 0)
	c.text(frame, 4, "UNIT:", 77, 23, 0)
	c.text(frame, 4, p.Squadron, 117, 23, 0)
	c.text(frame, 4, "FROM:", 77, 31, 0)
	c.text(frame, 4, c.Menus.Strings["memo_from"], 117, 31, 0)
	c.wrapMemo(frame, c.Menus.Strings["mission_availability"])
	for i, m := range c.Choices {
		colour := 0
		if c.Highlight == i {
			colour = 9
		}
		row := missionChoiceRect(i)
		c.text(frame, 4, c.Office.Assets.Metadata.Missions[m.Raw()].Text, row[0]+20, row[1]+2, colour)
	}
	assetdata.OfficeFill(frame, 0, 191, 320, 9, 0)
	assetdata.DrawBottomTooltip(frame, c.Menus.Fonts[3], c.Menus.Strings["mission_prompt"], 7)
}

// Memo's explicit newlines and leading spaces are source content. Wrapping
// uses the original 224-pixel text context and eight-pixel line advance.
func (c *FrontEnd) wrapMemo(frame []byte, text string) {
	y := 47
	for _, paragraph := range strings.Split(text, "\n") {
		indent := paragraph[:len(paragraph)-len(strings.TrimLeft(paragraph, " "))]
		line := indent
		for _, word := range strings.Fields(paragraph) {
			separator := " "
			if strings.TrimSpace(line) == "" {
				separator = ""
			}
			next := line + separator + word
			if c.Menus.Fonts[4].Measure([]byte(next)) > 224 && strings.TrimSpace(line) != "" {
				c.clippedText(frame, 4, line, 77, y, 0, [4]int{77, 7, 224, 183})
				y += 8
				line = word
			} else {
				line = next
			}
		}
		c.clippedText(frame, 4, line, 77, y, 0, [4]int{77, 7, 224, 183})
		y += 8
	}
}

func (c *FrontEnd) wrap(frame []byte, text string, x, y, width, font, colour int) {
	f := c.Menus.Fonts[font]
	line := ""
	for _, word := range strings.Fields(text) {
		trial := word
		if line != "" {
			trial = line + " " + word
		}
		if f.Measure([]byte(trial)) > width && line != "" {
			c.text(frame, font, line, x, y, colour)
			y += 10
			line = word
		} else {
			line = trial
		}
	}
	if line != "" {
		c.text(frame, font, line, x, y, colour)
	}
}

func (c *FrontEnd) drawRecovery(frame []byte) {
	if c.Recovery == nil {
		return
	}
	// The confirmation shows the selected pilot's pre-mission state. It never
	// changes the live selected record just to construct this view.
	p := c.Recovery.State.SelectedPilot()
	if p == nil {
		return
	}
	c.text(frame, 4, "TO:", 77, 15, 0)
	c.clippedText(frame, 4, c.Menus.FullRanks[p.Rank.Raw()]+" "+p.Name, 117, 15, 0, [4]int{117, 15, 160, 8})
	c.text(frame, 4, "UNIT:", 77, 23, 0)
	c.text(frame, 4, p.Squadron, 117, 23, 0)
	c.text(frame, 4, "FROM:", 77, 31, 0)
	c.text(frame, 4, c.Menus.Strings["recovery_from"], 117, 31, 0)
	c.wrapMemo(frame, c.Menus.Strings["recovery_text"])
	// MEMO's LORI0 assembles this emblem from three 20x9 source strips.
	for strip := 0; strip < 3; strip++ {
		c.menuLori(frame, "MEMO.SCN", 0, strip, 280, 18+9*strip)
	}
	assetdata.OfficeFill(frame, 0, 191, 320, 9, 0)
	assetdata.DrawBottomTooltip(frame, c.Menus.Fonts[3], c.Menus.Strings["recovery_prompt"], 7)
}

func (c *FrontEnd) menuLori(frame []byte, screen string, key, selector, x, y int) {
	for _, l := range c.Menus.Screens[screen].Lori {
		if l.Key != key || l.Divisor <= 0 {
			continue
		}
		r := [4]int{l.Base[0] + selector/l.Divisor*l.Cell[0], l.Base[1] + selector%l.Divisor*l.Cell[1], l.Cell[0], l.Cell[1]}
		_ = assetdata.OfficeBlit(c.Menus.Images[l.SourceAsset], frame, r, x, y, l.Mode != 0)
		return
	}
}

func (c *FrontEnd) drawPilotStatistics(frame []byte, p *gamestate.Pilot) {
	if p.Statistics == nil {
		c.text(frame, 3, "? ? ?", 78, 19, 0)
		c.text(frame, 3, "? Missions flown", 78, 25, 0)
		c.text(frame, 3, c.rosterCampaignLine(p), 78, 31, 0)
		return
	}
	stats := p.Statistics
	// Native %D is signed 32-bit decimal; the argument order is last/best/career.
	format := strings.ReplaceAll(c.Menus.Strings["scores_format"], "%D", "%d")
	c.text(frame, 3, fmt.Sprintf(format, stats.LastMissionScore, stats.BestMissionScore, stats.CareerScore), 78, 19, 0)
	suffix := func(n int32) string {
		if int16(n) == 1 {
			return ""
		}
		return "s"
	}
	format = strings.TrimPrefix(c.Menus.Strings["missions_format"], "\n")
	c.text(frame, 3, fmt.Sprintf(format, stats.Missions(), suffix(stats.Missions())), 78, 25, 0)
	c.text(frame, 3, c.rosterCampaignLine(p), 78, 31, 0)
	// Eight raw award counters select original atlas cells. Their icon ordering
	// is established; no unverified medal names become progression rules here.
	x := 149
	for i, count := range stats.AwardCounts {
		if count == 0 {
			continue
		}
		c.menuLori(frame, "ROSTER.SCN", 3, i, x, 19)
		width := c.Menus.DecorationWidths[i]
		if count > 1 {
			label := fmt.Sprint(count)
			c.text(frame, 3, label, x+(width-c.Menus.Fonts[3].Measure([]byte(label)))/2, 34, 3)
		}
		x += width + 1
	}
	if count := stats.PhoenixCount(); count != 0 {
		c.menuLori(frame, "ROSTER.SCN", 6, 0, 258, 13)
		if count > 1 {
			label := fmt.Sprint(count)
			c.text(frame, 3, label, 258+(20-c.Menus.Fonts[3].Measure([]byte(label)))/2, 34, 3)
		}
	}
}

// The shared GS prompt reads E276, independently of the text panel's style.
// JUMP's wrapper sets4 until it returns; admitted COMMISSN/HAPPY/SAD use0.
// GS2 flight confirmations have their own destination-remap renderer.
func (c *FrontEnd) quitFrameStyle() int {
	if c.previousScreen == ScreenDebrief && c.Debrief != nil {
		switch c.Debrief.Scene {
		case "JUMP.SCN":
			return 4
		case "KIA.SCN":
			return 5 // Source13da owns E276 independently of body ink.
		}
	}
	return 0
}

func (c *FrontEnd) drawQuit(frame []byte) { c.drawSourcePrompt(frame, c.Menus.Strings["quit"]) }

func (c *FrontEnd) drawSourcePrompt(frame []byte, text string) {
	// Keep source palette indices; choose the wrapper-owned bevel/body style.
	// The larger source rectangle saves the underlying scene, not the bevel.
	atlas := c.Office.Assets.Images["POINTER.PIC"]
	sx := 5 * c.quitFrameStyle()
	x, y, w, h := 90, 85, 140, 20
	for _, p := range [][2]int{{x, y}, {x + w - 5, y}, {x, y + h - 5}, {x + w - 5, y + h - 5}} {
		_ = assetdata.OfficeBlit(atlas, frame, [4]int{sx, 100, 5, 5}, p[0], p[1], false)
	}
	at := func(px, py int) byte { return atlas[py*320+sx+px] }
	for _, edge := range []struct {
		r      [4]int
		colour byte
	}{
		{[4]int{x + 2, y, w - 4, 1}, at(2, 100)}, {[4]int{x + 2, y + h - 1, w - 4, 1}, at(2, 104)},
		{[4]int{x, y + 2, 1, h - 4}, at(0, 102)}, {[4]int{x + w - 1, y + 2, 1, h - 4}, at(4, 102)},
		{[4]int{x + 2, y + 1, w - 4, 1}, at(2, 101)}, {[4]int{x + 2, y + h - 2, w - 4, 1}, at(2, 103)},
		{[4]int{x + 1, y + 2, 1, h - 4}, at(1, 102)}, {[4]int{x + w - 2, y + 2, 1, h - 4}, at(3, 102)},
	} {
		assetdata.OfficeFill(frame, edge.r[0], edge.r[1], edge.r[2], edge.r[3], edge.colour)
	}
	assetdata.OfficeFill(frame, x+2, y+2, w-4, h-4, at(0, 105))
	c.centered(frame, 5, text, 92, 10)
}

func outline(frame []byte, r [4]int, colour byte) {
	x, y, w, h := r[0], r[1], r[2], r[3]
	assetdata.OfficeFill(frame, x, y, w, 1, colour)
	assetdata.OfficeFill(frame, x, y+h-1, w, 1, colour)
	assetdata.OfficeFill(frame, x, y, 1, h, colour)
	assetdata.OfficeFill(frame, x+w-1, y, 1, h, colour)
}
