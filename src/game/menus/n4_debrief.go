package menus

import (
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	gamestate "gunship2000-assets/game/state"
	"image/color"
	"strings"
)

// Section3:30ca rotates the last RGB triplet to the front of each four-colour
// group. The six-byte source descriptor selects exactly16..31; other palette
// entries and the immutable prepared bank stay untouched.
func rotateRescuePalette(p color.Palette) {
	if len(p) != 256 {
		return
	}
	for first := 16; first < 32; first += 4 {
		last := p[first+3]
		copy(p[first+1:first+4], p[first:first+3])
		p[first] = last
	}
}

func (c *FrontEnd) debriefPalette() color.Palette {
	if c.Debrief == nil {
		return nil
	}
	if len(c.Debrief.Palette) == 256 {
		return c.Debrief.Palette
	}
	a := c.Office.Assets.Debrief
	if _, palette, ok := c.frozenFeedbackMedia(); ok {
		return a.Palettes[palette]
	}
	return a.Palettes[a.Scenes[c.Debrief.Scene].Palette]
}

func (c *FrontEnd) frozenFeedbackMedia() (string, string, bool) {
	if c.Debrief == nil || c.Debrief.Pending == nil || c.Debrief.Pending.Proposal == nil {
		return "", "", false
	}
	v := c.Debrief
	ctx := v.Pending.Proposal.Facts.ResultContext
	if ctx == nil || ctx.Version < 3 || ctx.Scene == nil || (v.Scene != "SAD.SCN" && v.Scene != "HAPPY.SCN") {
		return "", "", false
	}
	picture, palette, err := assetdata.FeedbackSceneMedia(v.Scene, *ctx.Scene)
	return picture, palette, err == nil
}

// Source section3:1974..1993 draws LORI0 once using A26D (0..3),
// except the two ice scenes4/5. It does not register a background animation.
// Old fixtures without the source-scene carrier retain their prior selector2.
func (c *FrontEnd) feedbackScenerySelector() (int, bool) {
	if c.Debrief != nil && c.Debrief.Pending != nil && c.Debrief.Pending.Proposal != nil {
		ctx := c.Debrief.Pending.Proposal.Facts.ResultContext
		if ctx != nil && ctx.Version >= 3 && ctx.Scene != nil && *ctx.Scene <= 5 {
			return int(*ctx.Scene), *ctx.Scene != 4 && *ctx.Scene != 5
		}
	}
	return 2, true
}

func (c *FrontEnd) debriefAfterCommander(v *DebriefView) string {
	if v.Pending.Proposal.Score.NormalizedFlags&0x20 != 0 {
		return "rescue"
	}
	return c.debriefAfterRescue(v)
}

func (c *FrontEnd) debriefAfterRescue(v *DebriefView) string {
	p := v.Pending.Proposal
	if p.Result != nil {
		if p.Result.Loss {
			return "loss"
		}
		if p.Result.Campaign != nil && p.Result.Campaign.Concluded {
			return "campaign"
		}
	}
	return "jump"
}

func debriefMusicCue(p *gamestate.SortieProposal, variant uint32) uint16 {
	if p.Result != nil && p.Result.MusicCue != 0 {
		return p.Result.MusicCue
	}
	// Compatibility for pre-N4 supplied controller fixtures; production N4
	// proposals retain the ordered GS random draw and actual selected command.
	flags := p.Score.NormalizedFlags
	if flags&0x10 != 0 {
		return 0x20
	}
	if flags&8 != 0 {
		return 0x1c
	}
	if p.Result != nil && p.Result.Campaign != nil {
		if p.Result.Campaign.Verdict > 0 {
			return 0x1e
		}
		if p.Result.Campaign.Verdict < 0 {
			return 0x1f
		}
	}
	if !p.Success {
		return 0x1c
	}
	if p.InitialRankRecommendation == 0 {
		return 0x1b
	}
	return 0x19 + uint16(variant&1)
}

func (c *FrontEnd) n4DebriefFrame() []byte {
	v := c.Debrief
	a := c.Office.Assets.Debrief
	p := v.Pending.Proposal
	scene := a.Scenes[v.Scene]
	frame := append([]byte(nil), a.Images[scene.Background]...)
	if v.Page == "rescue" {
		frame = append([]byte(nil), v.Clean...)
	}
	if len(frame) != 64000 {
		return make([]byte, 64000)
	}
	switch v.Page {
	case "rescue":
		if v.Tick >= 324 {
			name := ""
			variant := 0
			if r := p.Result; r != nil {
				variant = r.RescueTextVariant
				if f := p.Facts.ResultContext; f != nil && r.RescuerActor >= 1 && r.RescuerActor < len(f.Actors) {
					name = f.Actors[r.RescuerActor].Name
				}
			}
			// Source31da/3205..3265:300-wide bank,308-wide style0
			// frame atx6; no global scene palette remapping.
			c.drawDebriefBody(frame, a.RescueParagraph(name, variant), 6, 300, 0)
		}
		assetdata.DrawBottomTooltip(frame, c.Menus.Fonts[3], a.Text[0x6051], 7)
	case "loss":
		mia := p.Score.NormalizedFlags&8 != 0
		phase := v.Tick / 7 % 20
		y := 104
		if !mia {
			y += min(39, max(0, v.Tick/7-20))
		}
		_ = a.Lori(frame, "KIA.SCN", 0, phase, 223, y)
		if v.Tick >= 140 {
			text := a.Text[0x515d]
			if !mia {
				var lines []string
				for i, n := range p.Awards {
					if n != 0 && (i != 6 || len(lines) == 0) {
						lines = append(lines, a.MedalCitations[i])
					}
				}
				text = strings.Join(lines, ".\n")
				if text != "" {
					text += a.Text[0x589b]
				}
			}
			if text != "" {
				c.drawDebriefBody(frame, text, 6, 200, 5)
			}
		}
		assetdata.DrawBottomTooltip(frame, c.Menus.Fonts[3], a.Text[0x5144], 7)
	case "campaign":
		if v.Tick >= 120 {
			c.drawDebriefBody(frame, a.CampaignParagraph(p.Result.Campaign.SortieSuccess), 16, 280, 0)
		}
		assetdata.DrawBottomTooltip(frame, c.Menus.Fonts[3], a.Text[0x2c0], 7)
	}
	return frame
}

func (c *FrontEnd) handleCrewDebrief(a Actions) error {
	v := c.Debrief
	p := v.Pending.Proposal
	if p.Result == nil {
		return fmt.Errorf("crew debrief lacks frozen result state")
	}
	n := len(p.Result.Recommendations)
	if a.Inside {
		v.CrewSelection = crewAreaAt(n, a.X, a.Y)
	}
	if a.Down || a.MenuTab {
		c.setCrewSelection((v.CrewSelection+1)%(n+1), n)
	}
	if a.Up || a.MenuBackTab {
		index := n
		if v.CrewSelection >= 0 {
			index = (v.CrewSelection + n) % (n + 1)
		}
		c.setCrewSelection(index, n)
	}
	if a.MenuHome {
		c.setCrewSelection(0, n)
	}
	if a.MenuEnd {
		c.setCrewSelection(n, n)
	}
	if !a.Selector1 && !a.Enter {
		return nil
	}
	if v.CrewSelection < 0 {
		return nil
	} // Source key0 gaps are inert.
	if v.CrewSelection < n {
		choices, err := gamestate.CycleSortieCrewRecipientForChoice(p, v.Pending.CrewChoices, v.CrewSelection, v.Pending.CommissionChoice != nil && *v.Pending.CommissionChoice)
		if err != nil {
			c.reportDiagnostic("career", err.Error(), err)
			return nil
		}
		if err = c.Store.SetSortieCrewChoices(v.Pending.Start.ID, *choices); err != nil {
			return err
		}
		v.Pending, err = c.Store.PendingSortie()
		if err != nil {
			return err
		}
		accept := v.Pending.CommissionChoice != nil && *v.Pending.CommissionChoice
		v.Pilot, err = gamestate.ResolveSortieProposalWithChoices(p, accept, v.Pending.CrewChoices)
		return err
	}
	// SourceAccept applies the recommendation once; native durable publication
	// stays at the later final acknowledgement. Escape is not award cancellation.
	c.enterDebriefPage(c.afterCrewReplacement())
	return nil
}

func (c *FrontEnd) crewDebriefFrame() []byte {
	v := c.Debrief
	p := v.Pending.Proposal
	a := c.Office.Assets.Debrief
	frame := make([]byte, 64000)
	if p.Result == nil || p.Facts.ResultContext == nil {
		return frame
	}
	recipients := make([]int, len(p.Result.Recommendations))
	for i, r := range p.Result.Recommendations {
		recipients[i] = r.Actor
	}
	if v.Pending.CrewChoices != nil {
		copy(recipients, v.Pending.CrewChoices.Recipients)
	}
	for _, metric := range p.Result.Crew {
		if metric.Actor < 1 || metric.Actor > 4 || metric.Actor >= len(p.Facts.ResultContext.Actors) {
			continue
		}
		actor := p.Facts.ResultContext.Actors[metric.Actor]
		x := (metric.Actor - 1) * 80
		drawPreflightFrame(c.Office.Assets.Missions, frame, [4]int{x, 0, 79, 136}, 0, 0)
		clip := [4]int{x + 4, 3, 71, 130}
		colour := 7
		if actor.Group == 0 {
			colour = 9
		}
		assetdata.OfficeFill(frame, x+4, 3, 71, 7, byte(colour))
		c.clippedText(frame, 3, strings.ToUpper(actor.Callsign), x+4, 3, 15, clip)
		portrait := int(actor.StoredRaw[36])
		_ = assetdata.OfficeBlit(a.Images["PILOT.PIC"], frame, [4]int{portrait / 6 * 48, portrait % 6 * 32, 48, 32}, x+4, 11, true)
		drawPreflightOutline(frame, [4]int{x + 4, 11, 48, 32}, byte(colour))
		ink := 10
		for i, recipient := range recipients {
			if recipient == metric.Actor && i == v.CrewSelection {
				ink = 15
			}
		}
		rank := ""
		if metric.Rank >= 0 && metric.Rank < len(c.Office.Assets.Missions.Preflight.OutfitRanks) {
			rank = c.Office.Assets.Missions.Preflight.OutfitRanks[metric.Rank]
		}
		c.clippedText(frame, 3, rank+" "+metric.Name, x+4, 44, ink, clip)
		section := a.Text[0x5041]
		if actor.Group != 0 {
			section = a.Text[0x503b]
		}
		section += " " + a.Text[0x5033]
		if actor.FlightLeader {
			section = strings.TrimSuffix(section, " "+a.Text[0x5033]) + " " + a.Text[0x502c]
		}
		c.clippedText(frame, 3, section, x+4, 50, ink, clip)
		if len(p.Result.Recommendations) > 0 && p.Result.Recommendations[0].Type == 0 && recipients[0] == metric.Actor {
			_ = a.Lori(frame, "CREWPRO.SCN", 1, actor.RankRaw+1, x+52, 14)
		}
		c.clippedText(frame, 3, c.crewAircraftName(actor.AcceptedSelector), x+4, 56, ink, clip)
		c.clippedText(frame, 3, a.Text[0x505a], x+4, 64, 2, clip)
		assetdata.OfficeFill(frame, x+4, 70, 71, 1, 2)
		if metric.Flags&0x18 != 0 {
			status := a.Text[0x50b0]
			if metric.Flags&0x10 == 0 {
				status = a.Text[0x50b6]
			}
			c.clippedText(frame, 3, strings.TrimPrefix(status, "%t"), x+4, 78, 12, clip)
		} else {
			for j, q := range []struct{ ds, value int }{{0x506a, metric.Flight}, {0x5084, metric.Attack}, {0x509e, metric.Defense}} {
				label := strings.Split(strings.TrimPrefix(a.Text[q.ds], "%t"), "%c")[0]
				c.clippedText(frame, 3, label, x+4, 72+j*6, 10, clip)
				c.clippedText(frame, 3, fmt.Sprintf("%d%%", q.value), x+41, 72+j*6, 10, clip)
				if j < 2 {
					ds := 0x507b
					if j == 1 {
						ds = 0x5095
					}
					suffix := strings.Split(a.Text[ds], "%c")[0]
					c.clippedText(frame, 3, suffix, x+57, 72+j*6, 10, clip)
				}
			}
			if metric.Flags&4 != 0 {
				c.clippedText(frame, 3, a.Text[0x50bf], x+4, 90, 12, clip)
			}
			if metric.Flags&0x20 != 0 {
				c.clippedText(frame, 3, a.Text[0x50c8], x+4, 96, 12, clip)
			}
		}
		// Existing and pending medals use the sourcePATCHES atlas, not UI glyphs.
		awards := [8]int8{}
		for i, b := range actor.StoredRaw[26:34] {
			awards[i] = int8(b)
		}
		for i, r := range p.Result.Recommendations {
			if recipients[i] == metric.Actor && r.Type > 0 && r.Type < 8 {
				awards[r.Type]++
			}
		}
		remaining := 79
		for medal, count := range awards {
			if count != 0 {
				remaining -= c.Menus.DecorationWidths[medal]
			}
		}
		mx := x + remaining/2 - 2
		for medal, count := range awards {
			if count != 0 {
				_ = a.Lori(frame, "CREWPRO.SCN", 0, medal, mx, 113)
				w := c.Menus.DecorationWidths[medal]
				if count > 1 {
					label := fmt.Sprint(count)
					c.clippedText(frame, 3, label, mx+(w-c.Menus.Fonts[3].Measure([]byte(label)))/2, 128, 2, clip)
				}
				mx += w + 1
			}
		}
	}
	drawPreflightFrame(c.Office.Assets.Missions, frame, [4]int{0, 137, 319, 54}, 0, 0)
	clip := [4]int{3, 141, 224, 42}
	c.clippedText(frame, 3, a.Text[0x50f5], 11, 141, 2, clip)
	assetdata.OfficeFill(frame, 11, 147, 100, 1, 2)
	if len(recipients) == 0 {
		c.clippedText(frame, 3, a.Text[0x5134], 12, 149, 12, clip)
	}
	for i, r := range p.Result.Recommendations {
		actor := p.Facts.ResultContext.Actors[recipients[i]]
		base, alternate := 10, 14
		if r.Type == 0 {
			alternate = 7
		}
		if i == v.CrewSelection {
			base, alternate = 15, 15
		}
		text := ""
		if r.Type == 0 {
			rank := actor.RankRaw + 1
			if rank >= 0 && rank < len(c.Office.Assets.Missions.Preflight.OutfitRanks) {
				text = crewRecommendationText(a.Text[0x511f], []string{actor.Name, c.Office.Assets.Missions.Preflight.OutfitRanks[rank]}, base, alternate)
			}
		} else if r.Type < len(a.MedalNames) {
			text = crewRecommendationText(a.Text[0x510c], []string{a.MedalNames[r.Type], actor.Name}, base, alternate)
		}
		c.clippedText(frame, 3, text, 12, 149+7*i, base, clip)
	}
	drawPreflightFrame(c.Office.Assets.Missions, frame, [4]int{11, 174, 79, 10}, 1, 0)
	c.clippedText(frame, 3, a.Text[0x5139], 36, 176, 12, [4]int{11, 174, 79, 10})
	_ = a.Lori(frame, "CREWPRO.SCN", 2, v.Pilot.Emblem, 230, 141)
	c.clippedText(frame, 3, v.Pilot.Squadron, 220, 182, 2, [4]int{195, 182, 122, 6})
	assetdata.DrawBottomTooltip(frame, c.Menus.Fonts[3], a.Text[0x4ff2], 7)
	return frame
}
