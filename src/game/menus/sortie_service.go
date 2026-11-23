package menus

import (
	"gunship2000-assets/game/flight"
	gamestate "gunship2000-assets/game/state"
)

// This follows a logical physics update; it cannot run from rendering. Service
// restores accepted runtime stores and fuel, never regenerates preparation.
func (c *FrontEnd) advanceSortieService() error {
	p := c.Rendering
	if p == nil || p.Flight == nil || p.Initialized == nil || c.Mission == nil {
		return nil
	}
	r := p.Flight
	serviced, err := p.Initialized.AdvanceServiceWithFuelLoss(c.Mission.Data, &r.Systems, p.correctiveFlight && r.Systems.Damage[3]&3 == 2)
	if err != nil {
		return err
	}
	for slot, fuel := range r.Systems.Fuel {
		if slot < len(p.Initialized.Stores.Ships) {
			p.Initialized.Stores.Ships[slot].Fuel = fuel
		}
	}
	if serviced {
		ship := p.Initialized.Stores.Ships[p.Initialized.PlayerRow]
		r.Defenses.Chaff, r.Defenses.Flares = ship.Chaff, ship.Flares
		var stations [4]flight.LoadStation
		for i, s := range ship.Stations {
			if p.Initialized.Stores.WeaponTable != nil {
				if !s.Present {
					continue
				}
				w, err := p.Initialized.Stores.LookupDefinition(s.CompactID)
				if err != nil {
					return err
				}
				stations[i] = flight.LoadStation{Present: true, Flags: s.Flags, Quantity: s.Ammunition, UnitTerm: gamestate.RowWord(w.Raw[:], 11), FixedTerm: gamestate.RowWord(w.Raw[:], 13)}
				continue
			}
			w := c.Office.Assets.Missions.WeaponDefinitions[s.RawWSHDID]
			if !s.Present || len(w) != 214 {
				continue
			}
			stations[i] = flight.LoadStation{Present: true, Flags: s.Flags, Quantity: s.Ammunition, UnitTerm: gamestate.RowWord(w, 13), FixedTerm: gamestate.RowWord(w, 15)}
		}
		// Resident0000:0000 recomputes from runtime station counts/flags. The
		// existing accepted-loadout launch calculation remains unchanged.
		r.MassTerm = flight.LaunchLoad(r.Parameters, stations, ship.Flags&7, ship.Flags&0x18)
	}
	return nil
}

// Call after publishing the current contact/camera frame. Source prompts after
// drawing; freeze only transient simulation/input while the modal is shown.
func (c *FrontEnd) presentNaturalSortieEnd() bool {
	if c.filmIsolation != nil && c.filmIsolation.Mode == "playback" {
		return false
	}
	p := c.Rendering
	if c.Screen != ScreenScene || p == nil || p.Initialized == nil || p.Initialized.Sortie == nil || p.Initialized.Sortie.Pending == "" {
		return false
	}
	p.clearFlightInput()
	c.quitBackground = c.frameBody()
	c.quitFadeBackground = nil
	c.previousScreen = ScreenScene
	c.sceneEndConfirmation = true
	c.Screen = ScreenQuit
	return true
}

// The HUD clock displays one second per six DA phases and 360 phases per minute.
const missionClockLimit = 99 * 60 * 6

func (c *FrontEnd) liveMissionClockLimit(phase uint16) bool {
	return phase >= missionClockLimit && (c.filmIsolation == nil || c.filmIsolation.Mode == "practice")
}

func (c *FrontEnd) confirmSortieEnd() error {
	// A timed confirmation can occur inside the last update. Finish its
	// ordinary checkpoint before applying the separately recorded end action.
	c.filmFinishStep()
	if c.Rendering != nil && c.Rendering.Initialized != nil {
		m := c.Rendering.Initialized
		if m.Sortie != nil && m.Sortie.Pending != "" {
			m.AnswerNaturalEnd(true)
		} else {
			m.EndSortieForQoL()
		}
	}
	return c.finishSortieResult()
}

// Called only when dismissing an end modal, not the separate quit overlay.
func (c *FrontEnd) cancelNaturalSortieEnd() {
	if c.sceneEndConfirmation && c.Rendering != nil && c.Rendering.Initialized != nil {
		m := c.Rendering.Initialized
		if m.Sortie != nil && m.Sortie.Pending != "" && m.PlayerFlight != nil {
			// The reached flight step owns its pre-answer checkpoint. N is a
			// separate source transaction, not another physics quantum.
			c.filmFinishStep()
			m.AnswerNaturalEnd(false)
			c.recordFilmBoundary(filmBoundary{Kind: "natural-end-cancel"})
		}
	}
}
