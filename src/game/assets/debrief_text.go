package assets

import (
	"fmt"
	"strings"
)

// CommanderParagraph selects the original warrant/commission/reoffer wording.
// The rank is a source rank ordinal, not an artwork selector or UI label index.
func (a *DebriefAssets) CommanderParagraph(rank int, reoffer bool) string {
	if a == nil || rank < 0 || rank >= len(a.RankNames) {
		return ""
	}
	ds := 0x4e6e
	if rank >= 5 {
		ds = 0x4eb3
		if reoffer {
			ds = 0x4f48
		}
	}
	return fmt.Sprintf(a.Text[ds], a.RankNames[rank])
}

type DebriefJumpInput struct {
	OldWings, NewWings, PromotedRank           int
	Medals                                     [8]bool
	Success, CommanderAccepted, SpecialOutcome bool
	Elapsed                                    uint16
}

// JumpParagraphs follows section3:212e medal order, then wings/promotion,
// elapsed admonishment and the otherwise-empty fallback. SpecialOutcome is
// resident861c; it is deliberately distinct from the commission decision.
func (a *DebriefAssets) JumpParagraphs(in DebriefJumpInput) []string {
	if a == nil {
		return nil
	}
	var out []string
	for i, v := range in.Medals {
		if v && (i != 6 || len(out) == 0) {
			out = append(out, a.MedalCitations[i]+".")
		}
	}
	if in.NewWings > in.OldWings && in.NewWings > 0 && in.NewWings < len(a.WingLevels) {
		out = append(out, fmt.Sprintf(a.Text[0x527b], a.WingQualifiers[in.NewWings], a.WingLevels[in.NewWings]))
	}
	if in.PromotedRank > 0 && in.PromotedRank < len(a.RankNames) {
		out = append(out, fmt.Sprintf(a.Text[0x52cb], a.RankNames[in.PromotedRank]))
	}
	minutes := int(in.Elapsed) / 360
	if minutes > 30 && in.Success && !in.SpecialOutcome {
		ds := 0x5311
		if minutes >= 40 {
			ds = 0x5375
		}
		if minutes >= 50 {
			ds = 0x53c4
		}
		if minutes >= 60 {
			ds = 0x5417
		}
		out = append(out, a.Text[ds])
	}
	if len(out) == 0 && !in.CommanderAccepted {
		ds := 0x5511
		if in.Success {
			ds = 0x5478
		}
		out = append(out, a.Text[ds])
	}
	return out
}

type DebriefFeedbackInput struct {
	Success, FollowerLost, MissingObjectives, NotReturned bool
	FriendlyCreditCount                                   int
	// Empty or rank/service/loss. These are source terminal-career branches,
	// not player damage or aircraft-loss substitutions.
	Retirement string
	// Presentation-only native deterministic choices; never gameplay RNG.
	Variant         uint32
	Variants        map[string]uint32 // Ordered GS result draws retained by the proposal.
	CampaignWarning bool
}

// FeedbackPage is the ordinary final section3:17d6 crew-chief controller.
// Campaign territorial messages require future Campaign state and are omitted.
func (a *DebriefAssets) FeedbackPage(in DebriefFeedbackInput) (scene, text string) {
	if a == nil {
		return "", ""
	}
	scene = "SAD.SCN"
	if in.Success && !in.FollowerLost {
		scene = "HAPPY.SCN"
	}
	choose := func(key string) string {
		b := a.Feedback[key]
		if len(b) == 0 {
			return ""
		}
		v := in.Variant
		if n, ok := in.Variants[key]; ok {
			v = n
		}
		return b[int(v%uint32(len(b)))]
	}
	if in.Retirement != "" {
		return scene, choose("retire-" + in.Retirement)
	}
	if in.FriendlyCreditCount > 0 {
		return scene, strings.TrimPrefix(choose("friendly"), "\n")
	}
	if in.Success && !in.FollowerLost {
		key := "success"
		if in.MissingObjectives {
			key = "partial"
		}
		text = choose(key)
		if in.CampaignWarning {
			text += choose("campaign-warning")
		}
		if in.NotReturned {
			text += choose("no-return")
		}
		return scene, text
	}
	if in.Success && in.FollowerLost {
		text = choose("loss")
		if in.CampaignWarning {
			text += choose("campaign-warning")
		}
		return scene, text
	}
	return scene, choose("failure")
}

// DebriefSurname reproduces1627:0084's suffix following the final ASCII space.
func DebriefSurname(name string) string {
	if i := strings.LastIndexByte(name, ' '); i >= 0 {
		return name[i+1:]
	}
	return name
}

// DrawJumpInsignia composes source2598's folded atlas phase, faction patch,
// admitted award ribbons and the separate CMOH sprite. The caller selects the
// old/new rank and wing tier at the source callback boundary; drawing owns no
// progression or animation clock.
func (a *DebriefAssets) DrawJumpInsignia(dst []byte, rank, wings, phase, patch int, medals [8]bool) error {
	if a == nil || len(dst) != 64000 || rank < 0 || rank > 11 || wings < 0 || wings > 3 || phase < 0 || phase > 5 {
		return fmt.Errorf("debrief insignia domain")
	}
	if phase > 2 {
		phase = 5 - phase
	}
	if patch >= 0 {
		s := a.Scenes["JUMP.SCN"]
		if len(s.Areas) < 2 {
			return fmt.Errorf("debrief patch area")
		}
		r := s.Areas[1].Rectangle
		if err := a.Lori(dst, "JUMP.SCN", 0, patch, r[0], r[1]); err != nil {
			return err
		}
	}
	draw := func(key, selector, x, y int) error { return a.Lori(dst, "JUMP.SCN", key, selector, x, y) }
	if rank >= 1 && rank <= 6 {
		s := 3*(rank-1) + phase
		if e := draw(1, s, 22, 27); e != nil {
			return e
		}
		if e := draw(2, s, 266, 24); e != nil {
			return e
		}
	}
	if rank >= 7 {
		s := 3*(rank-7) + phase
		if e := draw(3, s, 17, 22); e != nil {
			return e
		}
		if e := draw(4, s, 254, 19); e != nil {
			return e
		}
	}
	key, sel := 5, 3*rank+phase
	if rank >= 7 {
		key, sel = 6, 3*(rank-7)+phase
	}
	rankX := 186
	if medals[0] {
		rankX = 202
	}
	if e := draw(key, sel, rankX, 92); e != nil {
		return e
	}
	if wings > 0 {
		if e := draw(7, 3*(wings-1)+phase, 191, 67); e != nil {
			return e
		}
	}
	n := 0
	for i := 1; i < 8; i++ {
		if medals[i] {
			n++
		}
	}
	x := 220 - n*20
	for i := 1; i < 8; i++ {
		if medals[i] {
			if e := OfficeBlit(a.Images["MEDAL.PIC"], dst, [4]int{40 * (i - 1), 0, 40, 70}, x, 101, true); e != nil {
				return e
			}
			x += 40
		}
	}
	if medals[0] {
		for _, v := range a.Scenes["JUMP.SCN"].Sprites {
			if v.Key == 1 {
				if e := OfficeBlit(a.Images["RANKS.PIC"], dst, v.Source, 85, 63, true); e != nil {
					return e
				}
			}
		}
	}
	return nil
}
