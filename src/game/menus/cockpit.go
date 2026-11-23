package menus

import (
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	gamestate "gunship2000-assets/game/state"
)

// CockpitState is screen-local presentation, not a saved renderer snapshot.
// Source callbacks are advanced by the common logical clock; seconds remain an
// approximation. Neither knob has a claimed audio/simulation meaning.
type CockpitState struct {
	Knobs      [2]byte
	APU        byte
	Bar        byte
	phase      int
	readyTicks int
	Score      int16
	Ready      bool
}

const cockpitBarCallbackPeriod = 6

const cockpitReadyHoldTicks = 61

func (s *CockpitState) Enter() {
	phase := s.phase // The original entry resets the bar, not its callback phase.
	*s = CockpitState{phase: phase}
}

func preflightDifficulty(a *assetdata.MissionAssets, m *gamestate.MissionData, settings gamestate.PreflightSettings) (int16, int16, error) {
	if err := settings.Validate(); err != nil {
		return 0, 0, err
	}
	if a == nil || a.Preflight == nil || len(a.Preflight.Difficulty) != 33 || m == nil || m.SourceWorld == nil || len(m.SourceWorld.THHD) != 54 {
		return 0, 0, fmt.Errorf("difficulty source tables/theatre are missing; rebuild assets")
	}
	weights := a.Preflight.Difficulty
	typeID := m.Inputs.MissionType.Raw()
	if typeID < 0 || typeID >= 5 {
		return 0, 0, fmt.Errorf("unsupported difficulty mission type")
	}
	sum := int32(weights[typeID])
	if sum != 0 {
		sum += int32(gamestate.RowWord(m.SourceWorld.THHD, 0x17))
		for key, choice := range settings.Values() {
			sum += int32(weights[5+4*key+choice])
		}
	}
	// Integer multiplication deliberately wraps32 bits before signed division.
	score := int16(((sum + 49) * 25) / 10000)
	target := int16((sum * 255) / 10000)
	return score, target, nil
}

func (s *CockpitState) Tick(a *assetdata.MissionAssets, m *gamestate.MissionData, settings gamestate.PreflightSettings) error {
	s.phase++
	if s.phase >= cockpitBarCallbackPeriod {
		s.phase = 0
		score, target, err := preflightDifficulty(a, m, settings)
		if err != nil {
			return err
		}
		s.Score = score
		delta := int16(target - int16(s.Bar))
		if delta < -1 || delta > 1 {
			delta /= 2
		}
		s.Bar = byte(int16(s.Bar) + delta)
	}
	if s.APU == 2 {
		s.readyTicks++
		if s.readyTicks >= cockpitReadyHoldTicks {
			s.Ready = true
		}
	} else {
		s.readyTicks = 0
		s.Ready = false
	}
	return nil
}

func drawDifficultyBar(atlas, frame, raw []byte, value byte) {
	if len(raw) != 18 {
		return
	}
	sx, sy, w, h, div, dx, dy := assetdata.OfficeSignedWord(raw[1:]), assetdata.OfficeSignedWord(raw[3:]), assetdata.OfficeSignedWord(raw[5:]), assetdata.OfficeSignedWord(raw[7:]), assetdata.OfficeSignedWord(raw[9:]), assetdata.OfficeSignedWord(raw[11:]), assetdata.OfficeSignedWord(raw[13:])
	if div <= 0 || raw[15]&1 != 0 {
		return
	}
	filled := int(int16(w*int(value))) / 255
	if filled < w {
		_ = assetdata.OfficeBlit(atlas, frame, [4]int{sx + filled, sy, w - filled, h}, dx+filled, dy, false)
	}
	if filled > 0 {
		_ = assetdata.OfficeBlit(atlas, frame, [4]int{sx + (1/div)*w, sy + (1%div)*h, filled, h}, dx, dy, false)
	}
}

// FinalPreflightConfiguration is the typed future execution input. Constructing
// it never launches another module, advances campaign outcomes, or makes a backup.
type FinalPreflightConfiguration struct {
	Mission          *gamestate.MissionData
	PilotID          string
	Flight           []gamestate.FlightPilot
	Aircraft         [][36]byte
	Settings         gamestate.PreflightSettings
	SecondaryControl byte
}

func finalPreflight(s *MissionSession) (*FinalPreflightConfiguration, error) {
	if s == nil || s.Data == nil {
		return nil, fmt.Errorf("no prepared mission")
	}
	if err := s.Settings.Validate(); err != nil {
		return nil, err
	}
	if err := validatePreparedFlight(s.ReadinessRows); err != nil {
		return nil, err
	}
	return &FinalPreflightConfiguration{Mission: s.Data, PilotID: s.PilotID, Flight: append([]gamestate.FlightPilot(nil), s.Flight...), Aircraft: append([][36]byte(nil), s.ReadinessRows...), Settings: s.Settings, SecondaryControl: s.SecondaryControl}, nil
}

func preflightArmed(r [36]byte) bool {
	if int8(r[2]) >= 0 && gamestate.RowWord(r[:], 21) > 0 {
		return true
	}
	for i := 0; i < 3; i++ {
		if gamestate.RowWord(r[:], 12+2*i) != 0 {
			return true
		}
	}
	return false
}
