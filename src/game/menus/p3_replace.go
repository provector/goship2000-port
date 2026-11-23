package menus

import (
	assetdata "gunship2000-assets/game/assets"
	gamestate "gunship2000-assets/game/state"
	"strings"
)

// The source's offer flag is not the player's acceptance flag. Readers choose a
// frozen CRT continuation; neither display nor typing advances either RNG.
func (c *FrontEnd) selectedDebriefResult() *gamestate.SortieResultDetails {
	if c.Debrief == nil {
		return nil
	}
	p := c.Debrief.Pending
	return gamestate.SortieResultForChoice(p.Proposal, p.CommissionChoice != nil && *p.CommissionChoice, p.CrewChoices)
}
func (c *FrontEnd) afterCrewReplacement() string {
	r := c.selectedDebriefResult()
	if c.Debrief.Pending.Proposal.Version >= 4 && r != nil && r.ReplacementCount > 0 && c.Debrief.Pilot.Status != gamestate.StatusRetired {
		return "replace"
	}
	return "feedback"
}
func (c *FrontEnd) replacePointerPresented() bool {
	return c.Screen == ScreenDebrief && c.Debrief != nil && c.Debrief.Page == "replace"
}

func (c *FrontEnd) cueReplacementPointer(row int) {
	areas, _, _ := c.replacementLayout()
	if row < 0 || row >= len(areas) {
		return
	}
	r := areas[row]
	// Source2c72..2ca9 centres the first dynamic AREA with inclusive extents.
	// Retain the cue through the shared Handle path until real host movement;
	// Office.SetPointer alone is overwritten by the next stationary raw sample.
	c.pointerCue = &[2]int{r[0] + (r[2]-1)/2, r[1] + (r[3]-1)/2}
	c.Office.SetPointer(c.pointerCue[0], c.pointerCue[1], true)
}

func (c *FrontEnd) replacementLayout() ([4][4]int, []string, int) {
	var areas [4][4]int
	v := c.Debrief
	r := c.selectedDebriefResult()
	if v == nil || r == nil {
		return areas, nil, 0
	}
	a := c.Office.Assets.Debrief
	ds := 0x5d71
	if r.ReplacementInitial {
		ds = 0x5b96
	}
	// 298e first calls common MEMO(0), original1b72:0022/section0:0fb2:
	// window77,7,224,183, font4, ink0; its header leaves cursor77,47.
	// D02:068c(2) marks a cache bank, not a choice of font2.
	font := c.Menus.Fonts[4]
	lines := debriefWrap(font, a.Text[ds], 224)
	advance := font.Metrics.Height + font.Metrics.Extra
	rankWidth := 0
	for _, row := range v.Pilot.SquadronPilots {
		rank := row.Rank.Raw()
		if rank >= 0 && rank < len(c.Office.Assets.Missions.Preflight.OutfitRanks) {
			rankWidth = max(rankWidth, font.Measure([]byte(c.Office.Assets.Missions.Preflight.OutfitRanks[rank])))
		}
	}
	// 2a1b reads the actual cursor, including the paragraph's trailing LF.
	// The final split entry is that cursor line, not another source newline.
	y := 47 + max(0, len(lines)-1)*advance
	for i := range areas {
		// 2c46/2c4b registers a 200x8 AREA. The source viewport clips its
		// visible right edge; its text-input width is still the full AREA.
		areas[i] = [4]int{77 + rankWidth + 4, y + 8*i, 200, 8}
	}
	return areas, lines, advance
}
func (c *FrontEnd) handleReplacement(a Actions) error {
	v := c.Debrief
	if v.ReplaceEditing {
		if a.Cancel {
			v.ReplaceEditing = false
			v.ReplaceBuffer = ""
			return nil
		}
		if a.Backspace || a.Delete {
			if len(v.ReplaceBuffer) > 0 {
				v.ReplaceBuffer = v.ReplaceBuffer[:len(v.ReplaceBuffer)-1]
			}
		}
		areas, _, _ := c.replacementLayout()
		width := areas[v.ReplaceRow][2]
		for _, r := range a.Text {
			if r < 0x20 || r > 0x7f || r == ' ' && len(v.ReplaceBuffer) == 0 {
				continue
			}
			next := v.ReplaceBuffer + string(r)
			if len(next) <= 10 && c.Menus.Fonts[4].Measure([]byte(next)) <= width {
				v.ReplaceBuffer = next
			}
		}
		if !a.Enter && !a.Selector1 {
			return nil
		}
		if e := gamestate.ValidateReplacementName(v.ReplaceBuffer); e != nil {
			// 2906/2922 reenters the helper, which backs up the stored
			// surname and clears its edit buffer again (section0:1725).
			v.ReplaceBuffer = ""
			return nil
		}
		choices := gamestate.DefaultSortieCrewChoices(v.Pending.Proposal)
		if v.Pending.CrewChoices != nil {
			choices = &gamestate.SortieCrewChoices{Recipients: append([]int(nil), v.Pending.CrewChoices.Recipients...), Names: append([]string(nil), v.Pending.CrewChoices.Names...), Result: v.Pending.CrewChoices.Result}
		}
		if len(choices.Names) == 0 {
			choices.Names = make([]string, 4)
		}
		choices.Names[v.ReplaceRow] = strings.TrimRight(v.ReplaceBuffer, " ")
		if err := c.Store.SetSortieCrewChoices(v.Pending.Start.ID, *choices); err != nil {
			return err
		}
		var err error
		v.Pending, err = c.Store.PendingSortie()
		if err != nil {
			return err
		}
		accepted := v.Pending.CommissionChoice != nil && *v.Pending.CommissionChoice
		v.Pilot, err = gamestate.ResolveSortieProposalWithChoices(v.Pending.Proposal, accepted, v.Pending.CrewChoices)
		if err != nil {
			return err
		}
		v.ReplaceEditing = false
		v.ReplaceBuffer = ""
		return nil
	}
	areas, _, _ := c.replacementLayout()
	// Keyboard owns selection until real pointer motion/press. A parked mouse
	// cannot replace the visible keyboard-selected row on the next frame.
	moved := a.PointerMoved || a.PointerPressed || a.PointerRelative && (a.PointerDX != 0 || a.PointerDY != 0)
	if a.Inside && moved {
		v.ReplaceRow = -1 // Source AREA0 outside the four dynamic surname AREAs.
		for i, r := range areas {
			if insideRect(a.X, a.Y, r) {
				v.ReplaceRow = i
				v.ReplacePortrait = i
				// Real pointer movement owns its reached position; hovering a
				// row does not re-centre the pointer as keyboard navigation does.
			}
		}
	}
	if a.MenuHome {
		v.ReplaceRow = 0
	}
	if a.MenuEnd {
		v.ReplaceRow = 3
	}
	if a.Down || a.MenuTab {
		v.ReplaceRow = (v.ReplaceRow + 1) % 4
	}
	if a.Up || a.MenuBackTab {
		if v.ReplaceRow < 0 {
			v.ReplaceRow = 3
		} else {
			v.ReplaceRow = (v.ReplaceRow + 3) % 4
		}
	}
	if a.Up || a.Down || a.MenuTab || a.MenuBackTab || a.MenuHome || a.MenuEnd {
		v.ReplacePortrait = v.ReplaceRow
		c.cueReplacementPointer(v.ReplaceRow)
	}
	// Enter has both the shared Selector2 alias and the source key13 value.
	// The explicit key wins here; a right-click/Backspace opens rename.
	if a.Enter || a.Selector1 {
		c.enterDebriefPage("feedback")
		return nil
	}
	if a.Selector2 || a.Backspace {
		if v.ReplaceRow < 0 || v.ReplaceRow >= 4 {
			return nil
		}
		v.ReplaceEditing = true
		v.ReplaceBuffer = ""
		return nil
	}
	// Escape is not an implicit acceptance of an outer source REPLACE loop.
	return nil
}
func (c *FrontEnd) replacementFrame() []byte {
	v := c.Debrief
	a := c.Office.Assets.Debrief
	// FIELD is the palette/scene setup descriptor. Source common MEMO(0)
	// establishes the full MEMO drawable bank before REPLACE's callback.
	// It is not an AREA1 crop composited over FIELD's tent artwork.
	frame := append([]byte(nil), a.Images["MEMO.PIC"]...)
	if len(frame) != 64000 {
		return make([]byte, 64000)
	}
	scene := a.Scenes["REPLACE.SCN"]
	clip := [4]int{77, 7, 224, 183}
	// Exact MEMO header1fc4 uses the reached pending pilot, not the still
	// uncommitted roster's earlier rank. Its tab stop puts values at117.
	c.replacementText(frame, "TO:", 77, 15, 0, clip)
	c.replacementText(frame, c.Menus.FullRanks[v.Pilot.Rank.Raw()]+" "+assetdata.DebriefSurname(v.Pilot.Name), 117, 15, 0, clip)
	c.replacementText(frame, "UNIT:", 77, 23, 0, clip)
	c.replacementText(frame, v.Pilot.Squadron, 117, 23, 0, clip)
	c.replacementText(frame, "FROM:", 77, 31, 0, clip)
	c.replacementText(frame, c.Menus.Strings["memo_from"], 117, 31, 0, clip)
	if len(v.Pilot.SquadronPilots) == 4 {
		p := v.Pilot.SquadronPilots[v.ReplacePortrait].Portrait
		_ = assetdata.OfficeBlit(a.Images["PILOT.PIC"], frame, [4]int{p / 6 * 48, p % 6 * 32, 48, 32}, 247, 13, false)
		// 29d6 explicitly loads PILOT into the logical bank initially named
		// MEMO by SCHD. 2a72 then applies LORI0 from that reached bank: its
		// black-transparent centre retains the portrait and its rim is paper.
		if len(scene.Lori) > 0 {
			l := scene.Lori[0]
			_ = assetdata.OfficeBlit(a.Images["PILOT.PIC"], frame, [4]int{l.Base[0], l.Base[1], l.Cell[0], l.Cell[1]}, 238, 0, l.Mode == 1)
		}
	}
	areas, lines, advance := c.replacementLayout()
	for i, line := range lines {
		c.replacementText(frame, line, 77, 47+i*advance, 0, clip)
	}
	for i, row := range v.Pilot.SquadronPilots {
		r := areas[i]
		rank := row.Rank.Raw()
		label := ""
		if rank >= 0 && rank < len(c.Office.Assets.Missions.Preflight.OutfitRanks) {
			label = c.Office.Assets.Missions.Preflight.OutfitRanks[rank]
		}
		c.replacementText(frame, label, 77, r[1], 0, clip)
		name := row.Name
		if v.ReplaceEditing && v.ReplaceRow == i {
			name = v.ReplaceBuffer
		}
		// AREA changes update portrait/pointer, not a fabricated grey name bar.
		// The source edit helper's argument15 is its erase-background ink;
		// its inherited text foreground remains0.
		c.replacementText(frame, name, r[0], r[1], 0, r)
		if v.ReplaceEditing && v.ReplaceRow == i {
			continue
		}
		counts := make([]int8, 8)
		for j, b := range row.LegacyRaw[26:34] {
			counts[j] = int8(b)
		}
		suffix := outfitAwardText(counts, c.Office.Assets.Missions.Preflight.AwardNames)
		maxName := 0
		for _, other := range v.Pilot.SquadronPilots {
			maxName = max(maxName, c.Menus.Fonts[4].Measure([]byte(other.Name)))
		}
		c.replacementText(frame, suffix, r[0]+maxName+8, r[1], 0, clip)
	}
	assetdata.OfficeFill(frame, 0, 191, 320, 9, 0)
	assetdata.DrawBottomTooltip(frame, c.Menus.Fonts[3], a.Text[0x5f4e], 7)
	return frame
}
func (c *FrontEnd) replacementText(frame []byte, text string, x, y, ink int, clip [4]int) {
	_ = c.Menus.Fonts[4].DrawClipped(frame, []byte(text), x, y, ink, 0, 0, clip)
}
func (c *FrontEnd) drawReplacementPointer(frame []byte) {
	if c.Office.PointerInside {
		c.Office.drawPointer(frame)
	}
}
