package menus

import (
	"encoding/binary"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	gamestate "gunship2000-assets/game/state"
	"math"
)

// These are the three signed bytes read at B00c3..00d9. Their arithmetic
// interpretation is established; this helper does not name a campaign economy.
func briefingCampaignProgress(c gamestate.CampaignGenerationInputs) (uint16, error) {
	for _, raw := range []int{c.Raw50, c.Raw51, c.Raw52} {
		if raw < 0 || raw > 255 {
			return 0, fmt.Errorf("campaign illustration requires explicit byte-width progress inputs")
		}
	}
	total := int64(int8(c.Raw50))
	if total <= 0 {
		return 0, fmt.Errorf("campaign illustration requires a positive signed denominator")
	}
	// Byte-width validation bounds the product to +/-25500. Divide in int64
	// before clamping, preserving signed truncation without AL wrap.
	delta := (int64(int8(c.Raw51)) - int64(int8(c.Raw52))) * 100 / total
	return uint16(max(int64(0), min(int64(255), 128+delta))), nil
}

func briefingRevealWidth(width int, progress byte) int {
	if width < 0 || progress != 0 && int64(width) > math.MaxInt64/int64(progress) {
		return -1
	}
	return int(int64(width) * int64(progress) / 255)
}

type campaignCopy struct {
	source [4]int
	x, y   int
}

// All operands come from signed16 GRAF words or the checked reveal. Intersect
// both translated rectangles before narrowing; trimming one end trims the other
// by the same amount, preserving the exact tile-to-background mapping.
func clipCampaignCopy(sx, sy, w, h, dx, dy int64) (campaignCopy, bool) {
	x0 := max(int64(0), -sx, -dx)
	y0 := max(int64(0), -sy, -dy)
	x1 := min(w, 320-sx, 320-dx)
	y1 := min(h, 200-sy, 200-dy)
	if x0 >= x1 || y0 >= y1 {
		return campaignCopy{}, false
	}
	return campaignCopy{[4]int{int(sx + x0), int(sy + y0), int(x1 - x0), int(y1 - y0)}, int(dx + x0), int(dy + y0)}, true
}

func (v *BriefingView) drawCampaignIllustration() error {
	if v == nil || v.Mission == nil || v.Assets == nil || len(v.clean) != 64000 {
		return fmt.Errorf("campaign illustration requires a complete background owner")
	}
	state := v.Mission.Inputs.Campaign
	if state == nil {
		return fmt.Errorf("campaign illustration requires explicit campaign progress state")
	}
	progress, err := briefingCampaignProgress(*state)
	if err != nil {
		return err
	}
	screen, ok := v.Assets.Metadata.Screens[v.Mission.StableScene+".SCN"]
	if !ok {
		return fmt.Errorf("campaign illustration lacks prepared scene definition")
	}
	atlas := v.Assets.Images[screen.Basenames[1]+".PIC"]
	if len(atlas) != 64000 {
		return fmt.Errorf("campaign illustration atlas is unavailable or malformed")
	}
	var copies []campaignCopy
	found := false
	for _, chunk := range screen.Chunks {
		if chunk.Tag != "GRAF" {
			continue
		}
		raw := chunk.Payload
		if len(raw) != 18 {
			return fmt.Errorf("campaign GRAF record has unsupported length%d", len(raw))
		}
		if int(int8(raw[0])) != 0 {
			continue
		}
		found = true
		word := func(offset int) int64 { return int64(int16(binary.LittleEndian.Uint16(raw[offset:]))) }
		sx, sy, w, h, div, dx, dy := word(1), word(3), word(5), word(7), word(9), word(11), word(13)
		if raw[15]&1 != 0 {
			return fmt.Errorf("campaign illustration vertical GRAF branch is unsupported")
		}
		if div <= 0 || w <= 0 || h <= 0 {
			return fmt.Errorf("campaign illustration has invalid GRAF dimensions/divisor")
		}
		reveal := int64(briefingRevealWidth(int(w), byte(progress)))
		if reveal < 0 || reveal > w {
			return fmt.Errorf("campaign illustration reveal product overflows")
		}
		// A wholly absent source tile indicates an impossible optional layout;
		// partially visible pairs are clipped, not stretched or guessed.
		tx, ty := sx+(1/div)*w, sy+(1%div)*h
		for _, tile := range [][2]int64{{sx, sy}, {tx, ty}} {
			if tile[0] >= 320 || tile[1] >= 200 || tile[0]+w <= 0 || tile[1]+h <= 0 {
				return fmt.Errorf("campaign illustration tile does not intersect its atlas")
			}
		}
		if op, ok := clipCampaignCopy(sx+reveal, sy, w-reveal, h, dx+reveal, dy); ok {
			copies = append(copies, op)
		}
		if op, ok := clipCampaignCopy(tx, ty, reveal, h, dx, dy); ok {
			copies = append(copies, op)
		}
	}
	if !found {
		return fmt.Errorf("campaign illustration lacks GRAF key0")
	}
	// Publish only after every GRAF record and both copy pairs validate.
	draft := append([]byte(nil), v.clean...)
	for _, op := range copies {
		if err := assetdata.OfficeBlit(atlas, draft, op.source, op.x, op.y, false); err != nil {
			return fmt.Errorf("campaign illustration copy: %w", err)
		}
	}
	v.clean = draft
	return nil
}
