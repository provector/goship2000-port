package menus

import (
	"encoding/binary"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/game/diagnostics"
	"gunship2000-assets/game/replay"
	gamestate "gunship2000-assets/game/state"
	"image/color"
	"log/slog"
	"strings"
)

// DebriefView owns presentation only. Its proposal and any commission choice
// already live in the checked pending transaction; drawing never applies score.
type DebriefView struct {
	Pending           *gamestate.PendingSortie
	Page, Scene       string
	Tick, SpritePhase int
	Pilot             *gamestate.Pilot
	Variant           uint32
	CrewSelection     int    // recommendation ordinal, then the source ACCEPT row
	Clean             []byte // RESCUE's retained composite bank; logical ticks own it
	ReplaceRow        int
	ReplacePortrait   int // last source row whose portrait was actually drawn
	ReplaceEditing    bool
	ReplaceBuffer     string
	Palette           color.Palette // RESCUE's private16..31 colour-cycle state.
}

func (c *FrontEnd) enterSortieDebrief() error {
	pending, err := c.Store.PendingSortie()
	if err != nil {
		return err
	}
	if pending == nil {
		return nil
	}
	v := &DebriefView{Pending: pending}
	if pending.Phase != gamestate.SortiePending {
		return fmt.Errorf("debrief requires a result from the current flight")
	}
	if c.Office.Assets.Debrief == nil {
		return fmt.Errorf("pending result needs prepared debrief assets; saves were retained")
	}
	if pending.Proposal == nil {
		return fmt.Errorf("pending result proposal is missing")
	}
	accept := pending.CommissionChoice != nil && *pending.CommissionChoice
	v.Pilot, err = gamestate.ResolveSortieProposalWithChoices(pending.Proposal, accept, pending.CrewChoices)
	if err != nil {
		return err
	}
	// Native visual alternatives are deterministic per stored sortie ID.
	// They replace only original presentation rand(2/3), never gameplay RNG.
	v.Variant = binary.LittleEndian.Uint32([]byte(pending.Start.ID)[:4])
	v.Page = c.debriefAfterCommander(v)
	if pending.Proposal.CommanderRank > 0 && (!pending.Proposal.Commission || pending.CommissionChoice == nil) {
		v.Page = "commander"
	}
	c.Debrief = v
	c.Screen = ScreenDebrief
	c.fade = nil
	c.SceneEntry = nil
	c.sceneEndConfirmation = false
	c.Rendering = nil // The frozen proposal owns facts; no future projectile can hit.
	c.Message = ""
	// A1e5d0 calls A1eaec once before all pages. Training is distinct
	// from Single's admitted loss arguments;861b is the initial rank query,
	// not a rank later accepted in COMMISSN. Native cosmetic choice is
	// stable across restart and never consumes the gameplay RNG stream.
	cue := uint16(0x1c)
	if pending.Proposal.Success {
		cue = 0x1b
		if pending.Proposal.InitialRankRecommendation != 0 {
			cue = 0x19 + uint16(v.Variant%2)
		}
	}
	if p := pending.Proposal.Result; p != nil {
		cue = debriefMusicCue(pending.Proposal, v.Variant)
	}
	c.emitAudioCue(cue)
	c.enterDebriefPage(v.Page)
	return nil
}

func (c *FrontEnd) enterDebriefPage(page string) {
	v := c.Debrief
	if v.Page == "replace" && page != "replace" {
		c.pointerCue = nil
	}
	v.Page, v.Tick, v.SpritePhase = page, 0, 0
	v.Clean, v.Palette = nil, nil
	switch page {
	case "commander":
		v.Scene = "COMMISSN.SCN"
	case "jump":
		v.Scene = "JUMP.SCN"
	case "feedback":
		v.Scene, _ = c.feedbackPresentation()
	case "rescue":
		v.Scene = "RESCUE.SCN"
		v.Clean = append([]byte(nil), c.Office.Assets.Debrief.Images["RESCUE.PIC"]...)
		v.Palette = append(color.Palette(nil), c.Office.Assets.Debrief.Palettes["RESCUE.PAL"]...)
	case "loss":
		v.Scene = "KIA.SCN"
	case "replace":
		v.Scene = "REPLACE.SCN"
		v.ReplaceRow = 0
		v.ReplacePortrait = 0
		v.ReplaceEditing = false
		v.ReplaceBuffer = ""
		c.cueReplacementPointer(0)
	case "crew":
		v.Scene = "CREWPRO.SCN"
		v.CrewSelection = 0
		c.crewPointer = missionPointerState{}
	case "campaign":
		v.Scene = "LOSE.SCN"
		if v.Pending.Proposal.Result.Campaign.Verdict > 0 {
			v.Scene = "WIN.SCN"
		}
	}
}

func (c *FrontEnd) tickDebrief() {
	v := c.Debrief
	if v == nil {
		return
	}
	v.Tick++
	if v.Page == "rescue" && v.Tick%4 == 0 {
		group := v.Tick/4 - 1
		if group >= 81 {
			group = 78 + (group-81)%3
		}
		_ = c.Office.Assets.Debrief.CompositeGroup(v.Clean, v.Scene, group)
		rotateRescuePalette(v.Palette)
	}
	if v.Tick > 120 {
		v.SpritePhase = min(5, (v.Tick-120)/22)
	}
}

func (c *FrontEnd) handleDebrief(a Actions) error {
	v := c.Debrief
	if v == nil {
		return nil
	} // A staging error remains safely quit-only.
	if v.Page == "replace" {
		return c.handleReplacement(a)
	}
	if v.Page == "crew" {
		return c.handleCrewDebrief(a)
	}
	pressed := a.Selector1 || a.Selector2 || a.Enter || a.AnyKey || a.Cancel || len(a.Text) > 0
	if !pressed {
		return nil
	}

	p := v.Pending.Proposal
	ready := 120
	if v.Page == "loss" {
		ready = 140
	}
	if v.Page == "rescue" {
		ready = 324
	}
	if v.Tick < ready {
		if v.Page == "rescue" {
			for group := v.Tick / 4; group < 81; group++ {
				_ = c.Office.Assets.Debrief.CompositeGroup(v.Clean, v.Scene, group)
			}
		}
		v.Tick = ready // The skip edge never acknowledges the next page.
		return nil
	}
	switch v.Page {
	case "commander":
		if p.Commission {
			if !a.Cancel && !a.Selector1 && !a.Enter {
				return nil
			}
			accept := !a.Cancel
			if err := c.Store.SetSortieCommission(v.Pending.Start.ID, accept); err != nil {
				return err
			}
			var err error
			v.Pending, err = c.Store.PendingSortie()
			if err != nil {
				return err
			}
			v.Pilot, err = gamestate.ResolveSortieProposalWithChoices(p, accept, v.Pending.CrewChoices)
			if err != nil {
				return err
			}
		} else if a.Cancel {
			return nil
		}
		c.enterDebriefPage(c.debriefAfterCommander(v))
	case "rescue":
		c.enterDebriefPage(c.debriefAfterRescue(v))
	case "loss":
		return c.commitDebriefResult()
	case "campaign":
		c.enterDebriefPage("jump")
	case "jump":
		// A1e4f6 returns immediately for Training0; HAPPY/SAD is only
		// reached by the admitted non-Training Single branch.
		if p.Facts.MissionType == gamestate.MissionTraining {
			return c.commitDebriefResult()
		}
		if r := c.selectedDebriefResult(); r != nil && r.CrewPro {
			c.enterDebriefPage("crew")
		} else {
			c.enterDebriefPage(c.afterCrewReplacement())
		}
	case "feedback":
		return c.commitDebriefResult()
	}
	return nil
}

func (c *FrontEnd) commitDebriefResult() error {
	if c.Debrief == nil {
		return nil // An obsolete page callback cannot commit or generate again.
	}
	id, pilotID := c.Debrief.Pending.Start.ID, c.Debrief.Pending.Start.PilotID
	// Check the durable pending owner rather than a stale debrief callback.
	// Duplicate acknowledgements of a committed/older sortie must not replace
	// the latest report. Report errors remain optional after the valid commit.
	pending, pendingErr := c.Store.PendingSortie()
	newResult := pendingErr == nil && pending != nil && pending.Start.ID == id && pending.Phase == gamestate.SortiePending
	var summary []byte
	var summaryErr error
	if newResult {
		summary, summaryErr = c.acknowledgedMissionSummary(pending)
	}
	film, filmErr := c.sealedRecording()
	if film != nil {
		film.End.Score = c.Debrief.Pending.Proposal.Score.ExportedScoreWord
		if film.End.Summary != nil {
			film.End.Summary.Score = film.End.Score
		}
	}
	if err := c.Store.CommitSortie(c.Debrief.Pending.Start.ID); err != nil {
		diagnostics.Report(id+"/commit", err.Error(), "Your mission result could not be saved.")
		c.Message = "Result commit unavailable; pending result retained."
		diagnostics.Error("career", "Result commit failed; pending result retained", "status", c.Message, "error", err)
		return err
	}
	diagnostics.Info("career", "Final acknowledgment committed once", "session", id)
	if newResult {
		if outcome, err := c.Store.SortieRecoveryOutcome(id); err == nil && outcome != nil && (outcome.Status == "unresolved" || outcome.Status == "deferred") {
			diagnostics.Warn("career", "Historical MIA restoration deferred; actual sortie result committed", "status", outcome.Status, "cause", outcome.Reason, "pilot", outcome.PilotID)
		}
		c.publishAcknowledgedMissionSummary(summary, summaryErr)
	}
	// Film I/O is deliberately outside the career journal. An already durable
	// result cannot be undone by a disk-full or unsupported recording failure.
	if filmErr == nil && film != nil {
		filmErr = (replay.Store{Root: c.FilmDirectory}).PublishLast(film)
		if filmErr != nil && c.filmDiagnostics != nil {
			c.filmDiagnostics.RecordFailure("publish", uint64(film.OperationCount()), film.End.Steps, filmErr)
		}
	}
	if newResult && c.resultWarning == "" {
		c.resultWarning = diagnostics.RecallWarning(id)
	}
	diagnostics.Phase("career committed")
	if filmErr != nil {
		c.resultWarning += " " + filmErr.Error()
	}
	if newResult && c.resultWarning != "" {
		if diagnostics.Report(id+"/saved", c.resultWarning, "Your mission result was saved.") {
			diagnostics.ForgetWarning(id)
		}
	}
	c.resultWarning = ""
	c.filmPending = nil
	if err := c.openFilmLibrary(true, id, pilotID); err != nil {
		c.recordDiagnostic(slog.LevelWarn, "replay", "Film library unavailable after saved result", err)
		nextErr := c.continueAfterSortie()
		if nextErr == nil && c.Message == "" {
			c.reportDiagnostic("career", "Result saved. Film Library requires Preparation25; mission successor retained.", nil)
		}
		return nextErr
	}
	if filmErr != nil {
		c.recordDiagnostic(slog.LevelWarn, "replay", "Mission film not saved", filmErr)
		c.Library.Status = "Result saved. Mission film unavailable."
	}
	return nil
}

func (c *FrontEnd) feedbackPresentation() (string, string) {
	v := c.Debrief
	pCopy := *v.Pending.Proposal
	pCopy.Result = c.selectedDebriefResult()
	p := &pCopy
	flags := p.Score.NormalizedFlags
	retirement := ""
	if v.Pilot.Status == gamestate.StatusRetired {
		retirement = "loss"
		if v.Pilot.Rank.Raw() == 11 {
			retirement = "rank"
		} else if v.Pilot.Statistics.MissionCampaignCounter/100 >= 50 {
			retirement = "service"
		}
	}
	friendly := 0
	for i, value := range p.Facts.ObjectValues {
		if value < 0 && (p.Facts.ObjectCredits[i/2]>>uint((i&1)*4))&15 == 1 {
			friendly++
		}
	}
	lost := p.Result != nil && p.Result.LostFollowers > 0 && !p.SpecialOutcome
	var variants map[string]uint32
	warning := false
	if p.Result != nil {
		variants = p.Result.FeedbackVariants
	}
	if p.Result != nil && p.Result.Campaign != nil {
		warning = p.Success && p.Facts.MissionType == gamestate.MissionCampaign && p.Result.Campaign.Quality < 30
	}
	return c.Office.Assets.Debrief.FeedbackPage(assetdata.DebriefFeedbackInput{Success: p.Success, FollowerLost: lost, MissingObjectives: flags&3 != 3, NotReturned: flags&0x80 == 0, FriendlyCreditCount: friendly, Retirement: retirement, Variant: v.Variant, Variants: variants, CampaignWarning: warning})
}

func (c *FrontEnd) jumpPresentation() (assetdata.DebriefJumpInput, [8]bool, int, int) {
	v := c.Debrief
	p := v.Pending.Proposal
	var medals [8]bool
	for i, count := range v.Pilot.Statistics.AwardCounts {
		medals[i] = count != p.PilotBefore.Statistics.AwardCounts[i]
	}
	commanderAccepted := p.CommanderRank > 0 && (!p.Commission || v.Pending.CommissionChoice != nil && *v.Pending.CommissionChoice)
	baseRank := p.PilotBefore.Rank.Raw()
	if commanderAccepted {
		baseRank = p.CommanderRank
	}
	promoted := 0
	if v.Pilot.Rank.Raw() != baseRank {
		promoted = v.Pilot.Rank.Raw()
	}
	oldWings := int(p.PilotBefore.LegacyRaw[0x55] >> 2 & 3)
	newWings := int(v.Pilot.LegacyRaw[0x55] & 3)
	in := assetdata.DebriefJumpInput{OldWings: oldWings, NewWings: newWings, PromotedRank: promoted, Medals: medals,
		Success: p.Success, CommanderAccepted: commanderAccepted, SpecialOutcome: p.SpecialOutcome, Elapsed: p.Facts.ElapsedDA}
	return in, medals, baseRank, v.Pilot.Rank.Raw()
}

// Font3's source API returns nominal6+extra1 for line advance. The text-box
// measure reports cursorY, not ink height: n lines yield(n−1)*7. Its copy uses
// height+8, retaining the final glyph without inventing an extra paragraph row.
func debriefWrap(font *assetdata.BitmapFont, text string, width int) []string {
	var lines []string
	for _, paragraph := range strings.Split(text, "\n") {
		rest := paragraph
		for {
			used, lastBreak, n := 0, 0, 0
			for n < len(rest) {
				ch := rest[n]
				if ch == ' ' || ch == '-' {
					lastBreak = n + 1
				}
				next := used + font.Measure([]byte{ch})
				if next > width {
					break
				}
				used, n = next, n+1
			}
			if n == len(rest) {
				lines = append(lines, rest)
				break
			}
			if lastBreak > 0 {
				n = lastBreak
			}
			if n == 0 {
				n = 1
			} // Prepared fonts cannot exceed the admitted112px body.
			lines = append(lines, strings.TrimRight(rest[:n], " "))
			rest = strings.TrimLeft(rest[n:], " ")
		}
	}
	return lines
}

// The source172px bank constrains its baseline after every newline. Overflow
// therefore revisits164/171; it does not resize the font or grow a DOS buffer.
func debriefBaselines(count, advance int) ([]int, int) {
	ys := make([]int, count)
	y := 0
	for i := range ys {
		if y > 171 {
			y = 171 - advance
		}
		ys[i] = y
		if i+1 < count {
			y += advance
		}
	}
	return ys, y
}

func (c *FrontEnd) drawDebriefBody(frame []byte, text string, x, width, style int) {
	font := c.Menus.Fonts[3]
	lines := debriefWrap(font, text, width)
	ys, height := debriefBaselines(len(lines), font.Metrics.Height+font.Metrics.Extra)
	panelWidth := width + 8
	// Render a potentially top-clipped source frame at a safe local origin,
	// then copy only its visible rectangle. No whole-frame clearing is involved.
	panel := make([]byte, 64000)
	drawPreflightFrame(c.Office.Assets.Missions, panel, [4]int{x, 0, panelWidth, height + 16}, style, 0)
	for row := 0; row < height+16; row++ {
		dy := 166 - height + row
		if dy >= 0 && dy < 200 {
			copy(frame[dy*320+x:dy*320+x+panelWidth], panel[row*320+x:row*320+x+panelWidth])
		}
	}
	bank := make([]byte, 64000)
	// D02:0c4e clears the bank with the cached POINTER body sample for
	// this style, just as the frame does. Glyph holes retain that same ink.
	body := byte(0)
	if pointer := c.Office.Assets.Missions.Images["POINTER.PIC"]; len(pointer) == 64000 {
		body = pointer[105*320+5*style]
	}
	assetdata.OfficeFill(bank, 0, 0, width, 172, body)
	for i, line := range lines {
		_ = font.DrawClipped(bank, []byte(line), 0, ys[i], 10, int(body), 0, [4]int{0, 0, width, 172})
	}
	for row := 0; row < min(172, height+8); row++ {
		dy := 170 - height + row
		if dy >= 0 && dy < 200 {
			copy(frame[dy*320+x+4:dy*320+x+4+width], bank[row*320:row*320+width])
		}
	}
}

func (c *FrontEnd) debriefFrame() []byte {
	v := c.Debrief
	if v == nil {
		return c.background("MEMO.PIC")
	}

	a := c.Office.Assets.Debrief
	p := v.Pending.Proposal
	if v.Page == "replace" {
		return c.replacementFrame()
	}
	if v.Page == "crew" {
		return c.crewDebriefFrame()
	}
	if v.Page == "loss" || v.Page == "rescue" || v.Page == "campaign" {
		return c.n4DebriefFrame()
	}
	scene := a.Scenes[v.Scene]
	background := scene.Background
	if picture, _, ok := c.frozenFeedbackMedia(); ok {
		background = picture
	}
	if v.Page == "jump" && v.Pilot.Statistics.AwardCounts[0] != p.PilotBefore.Statistics.AwardCounts[0] {
		background = "JUMPCMOH.PIC"
	}
	frame := append([]byte(nil), a.Images[background]...)
	if len(frame) != 64000 {
		return make([]byte, 64000)
	}
	text, prompt := "", a.Text[0x525a]
	x, width, style := 16, 280, 0
	if v.Page == "commander" {
		if len(scene.Areas) > 2 {
			r := scene.Areas[2].Rectangle
			_ = a.Lori(frame, v.Scene, 0, int(v.Variant%3), r[0], r[1])
		}
		if v.Tick >= 120 {
			text = a.CommanderParagraph(p.CommanderRank, p.PilotBefore.LegacyRaw[0x55]&0x10 != 0)
		}
		prompt = a.Text[0x4d52]
		if p.Commission {
			prompt = a.Text[0x4d6b]
		}
	} else if v.Page == "jump" {
		in, medals, oldRank, newRank := c.jumpPresentation()
		rank, rankPhase := oldRank, 0
		if newRank != oldRank {
			rankPhase = v.SpritePhase
			if v.SpritePhase >= 3 {
				rank = newRank
			}
		}
		_ = a.DrawJumpInsignia(frame, rank, 0, rankPhase, v.Pilot.Emblem, medals)
		wings, wingPhase := in.OldWings, 0
		if in.OldWings != in.NewWings {
			wingPhase = v.SpritePhase
			if v.SpritePhase >= 3 {
				wings = in.NewWings
			}
		}
		if wingPhase > 2 {
			wingPhase = 5 - wingPhase
		}
		if wings > 0 {
			_ = a.Lori(frame, "JUMP.SCN", 7, 3*(wings-1)+wingPhase, 191, 67)
		}
		c.clippedText(frame, 3, assetdata.DebriefSurname(v.Pilot.Name), 185, 85, 15, [4]int{185, 85, 69, 6})
		text = strings.Join(a.JumpParagraphs(in), "\n")
		if recovered := p.RecoveredPilot; recovered != nil {
			// Source JUMP nonzero roster argument selects DS4d91, bypassing
			// its ordinary rank/commission paragraph and promotion action.
			text = fmt.Sprintf(a.Text[0x4d91], rosterCrewRanks[recovered.Before.Rank.Raw()], recovered.Before.Name)
		}
		x, width, style = 6, 112, 4
	} else {
		selector, scenery := c.feedbackScenerySelector()
		if scenery && len(scene.Areas) > 1 {
			r := scene.Areas[1].Rectangle
			_ = a.Lori(frame, v.Scene, 0, selector, r[0], r[1])
		}
		if v.Tick >= 120 {
			_, text = c.feedbackPresentation()
		}
		prompt = a.Text[0x51c4]
		style = 0
	}
	if text != "" {
		c.drawDebriefBody(frame, text, x, width, style)
	}
	assetdata.DrawBottomTooltip(frame, c.Menus.Fonts[3], prompt, 7)
	return frame
}
