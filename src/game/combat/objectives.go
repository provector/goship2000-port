package combat

// AdvanceObjectives owns the reached source5d0c phase after projectiles/defenses.
// A live TARGET carrier admits bounded player cargo/recon and safe retasking.
// Legacy supplied fixtures without that carrier retain only the count arms.
func (s *State) AdvanceObjectives(in Input) {
	if s.init != nil && s.init.Targets != nil {
		s.advanceMissionObjectives(in)
		return
	}
	for _, v := range []struct {
		objective *Objective
		kind      byte
		command   uint16
		source    string
	}{
		{&s.Primary, 4, 0x3e, "5d0c:636a..63b4"},
		{&s.Secondary, 5, 0x3f, "5d0c:6ad8..6b22"},
	} {
		if v.kind == 5 && !in.SecondaryCountReady {
			continue
		}
		o := v.objective
		// CMPcounter,required/JL is signed. DisplayKind is cleared before
		// sound/result, so a later hit cannot count or notify this objective again.
		if o.DisplayKind != 3 || o.Destroyed < o.Required {
			continue
		}
		o.DisplayKind, o.Achieved = 0, true
		if in.SpeechEnabled {
			s.Events = append(s.Events, Event{Kind: "sound", Slot: -1, Actor: 0, Target: 0, Command: v.command, Source: v.source})
		}
		// Source0000:0bc2 receives kind4/5, actor0,target0. These transient
		// records are not debriefing or persistent mission-completion outcomes.
		s.appendResult(Result{Kind: v.kind, Actor: 0, Target: 0, Timestamp: in.Phase})
		s.Events = append(s.Events, Event{Kind: "objective-complete", Slot: -1, Actor: 0, Target: 0, Value: int16(v.kind), Source: v.source})
	}
}
