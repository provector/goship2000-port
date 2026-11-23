package menus

import (
	"fmt"
)

func (c *FrontEnd) openOutfit() error {
	s := c.Mission
	if s == nil || s.ReadinessError != nil {
		return fmt.Errorf("aircraft preparation is unavailable")
	}
	s.Final = nil
	s.PreflightWarning = ""
	c.Message = ""
	if len(s.ReadinessRows) == 1 {
		s.Outfit = nil
		return c.openOrdnance(0)
	}
	v, err := NewOutfitView(c.Office.Assets.Missions, s.Data, s.Flight, s.ReadinessRows)
	if err != nil {
		return err
	}
	s.Outfit = v
	c.Screen = ScreenOutfit
	return nil
}

func (c *FrontEnd) openOrdnance(slot int) error {
	s := c.Mission
	v, err := NewOrdnanceView(c.Office.Assets.Missions, s.Data, s.Flight, s.ReadinessRows, slot, s.Data.Inputs.Rank)
	if err != nil {
		return err
	}
	s.Ordnance = v
	s.PreflightWarning = ""
	v.seedRowPointer(c.pointerY)
	s.Final = nil
	c.Screen = ScreenOrdnance
	c.Message = ""
	return nil
}

func (c *FrontEnd) handleLoadout(a Actions) error {
	s := c.Mission
	s.Final = nil
	if c.Screen == ScreenOrdnance {
		done, err := s.Ordnance.Handle(a)
		for _, cue := range s.Ordnance.audioCues {
			c.emitAudioCue(cue)
		}
		s.Ordnance.audioCues = nil
		if err != nil {
			return err
		}
		if done {
			s.Ordnance = nil
			c.Message = ""
			if s.Outfit != nil {
				c.Screen = ScreenOutfit
			} else {
				c.Screen = ScreenBriefing
				s.View.ResumeScene()
			}
		}
		return nil
	}
	result, err := s.Outfit.Handle(a)
	if err != nil {
		return err
	}
	// A source RETURN cue is deferred until the port's stricter whole-flight
	// guard passes; a rejected port-only return uses the ordinary denial cue.
	if result.Kind != "return" && result.AudioCue != 0 {
		c.emitAudioCue(result.AudioCue)
	}
	if result.Message != "" {
		c.reportStatus("preflight", result.Message)
	}
	switch result.Kind {
	case "ordnance":
		return c.openOrdnance(result.Index)
	case "return":
		// The original summary can return before the later readiness check. The
		// port keeps invalid aircraft inside preparation, as requested, without
		// treating a nested menu's Back as loadout acceptance.
		if err := validatePreparedFlight(s.ReadinessRows); err != nil {
			c.emitAudioCue(gs4Denied)
			if !c.showPreflightRejection(err) {
				c.reportDiagnostic("preflight", err.Error(), err)
			}
			return nil
		}
		c.emitAudioCue(gs4Accepted)
		c.Screen = ScreenBriefing
		s.View.ResumeScene()
		c.Message = ""
	}
	return nil
}
