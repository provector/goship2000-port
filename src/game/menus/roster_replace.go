package menus

import (
	"fmt"
	gamestate "gunship2000-assets/game/state"
)

// Roster replacements stay detached through name/emblem/unit and the outer
// roster. Posting is the publication boundary; outer Escape/restart discards.
// E remains the explicitly labelled safe native edit adaptation.
func (c *FrontEnd) rosterState() gamestate.SaveState {
	if c.rosterWorking != nil {
		return *c.rosterWorking
	}
	return c.Store.State()
}
func (c *FrontEnd) beginRosterDraft(replace bool) error {
	state := c.rosterState()
	if c.Highlight < 0 || c.Highlight >= len(state.Slots) {
		return nil
	}
	old := state.Slots[c.Highlight]
	var p *gamestate.Pilot
	var err error
	if replace || old == nil {
		p, err = gamestate.NewPilot()
		if err != nil {
			return err
		}
		if old != nil {
			p.Emblem = old.Emblem
			if len(old.LegacyRaw) > 0x4e {
				p.LegacyRaw[0x4e] = old.LegacyRaw[0x4e]
			}
		}
		// A248c0 and A249ca are byte writes after the complete290-byte clear.
		p.LegacyRaw[0x64], p.LegacyRaw[0x56] = 1, 1
	} else {
		canonicalState := c.Store.State()
		for slot, working := range state.Slots {
			saved := canonicalState.Slots[slot]
			if (working == nil) != (saved == nil) || working != nil && saved != nil && working.ID != saved.ID {
				c.reportDiagnostic("roster", "Post or cancel staged replacements before using safe Edit.", nil)
				return nil
			}
		}
		canonical := canonicalState.Slots[c.Highlight]
		if canonical == nil || canonical.ID != old.ID {
			c.reportDiagnostic("roster", "Post or cancel this replacement before using safe Edit.", nil)
			return nil
		}
		q := *old
		q.LegacyRaw = append([]byte(nil), old.LegacyRaw...)
		p = &q
	}
	c.emitAudioCue(gs4Accepted)
	c.Draft = &RosterDraft{Slot: c.Highlight, Pilot: *p, Replacement: replace || old == nil}
	c.Message = ""
	return nil
}
func (c *FrontEnd) finishRosterReplacement(d *RosterDraft) error {
	if d == nil || !d.Replacement {
		return fmt.Errorf("no detached replacement")
	}
	if c.rosterWorking == nil {
		state := c.Store.State()
		c.rosterWorking = &state
	}
	p := d.Pilot
	if len(p.LegacyRaw) != gamestate.LegacyRecordSize {
		return fmt.Errorf("fresh replacement lacks source record")
	}
	p.LegacyRaw[0x53] = byte(p.Emblem)
	c.rosterWorking.Slots[d.Slot] = &p
	c.Draft = nil
	c.reportStatus("roster", "Replacement ready. SELECTOR #1 posts; ESC discards. E safely edits a posted unit.")
	return nil
}
