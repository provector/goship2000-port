package menus

// WeaponTriggerWitness is a bounded normalized-input/attempt diagnostic. It is
// not source replay state and cannot feed the firing gate, clocks or RNG.
type WeaponTriggerWitness struct {
	TimeNS, LogicalNS  int64
	Tick               uint64
	Phase, Reason      string
	Held, Pressed      bool
	Station, Ammo      int
	NextSlot, LastSlot uint16
	LastSlotLife       byte
}

func (r *FlightRuntime) appendWeaponTrace(v WeaponTriggerWitness) {
	v.Tick, v.LogicalNS = r.Steps, int64(r.commandInput.elapsed)
	if r.Combat != nil {
		v.NextSlot, v.LastSlot = r.Combat.NextSlot, r.Combat.LastSlot
		if int(v.LastSlot) < len(r.Combat.Slots) {
			v.LastSlotLife = r.Combat.Slots[v.LastSlot].Lifetime
		}
	}
	r.TriggerTrace = append(r.TriggerTrace, v)
	if len(r.TriggerTrace) > 256 {
		r.TriggerTrace = r.TriggerTrace[len(r.TriggerTrace)-256:]
	}
}

func (r *FlightRuntime) traceWeaponReceipt(a Actions, gps bool) {
	if !a.WeaponTriggerValid && !a.WeaponTriggerPressed && !a.FlightFocusLost {
		return
	}
	owner := "flight"
	switch {
	case a.FlightFocusLost:
		owner = "focus-lost"
	case gps:
		owner = "gps"
	case r.Paused || a.FlightPause:
		owner = "pause"
	case r.requireRelease:
		owner = "release-gate"
	case r.triggerNeedsRelease:
		owner = "start-handoff-gate"
	case r.State != nil && r.State.Terminal != nil:
		owner = "terminal"
	}
	if !r.triggerReceiptSeen || r.triggerReceiptHeld != a.WeaponTriggerHeld || r.triggerReceiptOwner != owner || a.WeaponTriggerPressed {
		r.appendWeaponTrace(WeaponTriggerWitness{TimeNS: int64(a.InputTime), Phase: "receipt", Reason: owner, Held: a.WeaponTriggerHeld, Pressed: a.WeaponTriggerPressed})
	}
	r.triggerReceiptSeen, r.triggerReceiptHeld, r.triggerReceiptOwner = true, a.WeaponTriggerHeld, owner
}

func (p *RenderingPreview) traceWeaponAttempt(reason string) {
	r := p.Flight
	station := p.Initialized.SelectedStation
	ammo := int(p.Initialized.Stores.Ships[0].Stations[station].Ammunition)
	r.appendWeaponTrace(WeaponTriggerWitness{TimeNS: int64(r.commandInput.elapsed), Phase: "attempt", Reason: reason, Held: r.commandInput.triggerHeld, Pressed: r.commandInput.TriggerPressed, Station: station, Ammo: ammo})
}
