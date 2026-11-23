package combat

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"testing"
)

func TestProductionRocketLaunchContactAndDamageTraces(t *testing.T) {
	for _, speed := range []int16{0, 120} {
		for _, rangeUnits := range []uint32{1000, 8000, 23200} {
			t.Run(fmt.Sprintf("E2%d/range%d", speed, rangeUnits), func(t *testing.T) {
				s, in := fixture(t, 0x1e21)
				for i := range s.init.Entities {
					s.init.Entities[i].Participation = 0
				}
				s.init.Entities[1].Participation = 1
				s.init.Entities[0].Pose.Height = 256
				s.init.Entities[1].Pose.H1 = 0x40000 + rangeUnits
				s.init.Entities[1].Pose.Height = 256
				s.init.Stores.Definitions[0].RawWSHDID = 7
				in.DF, in.PlayerSpeed, in.Target, in.TargetAvailable = 6, speed, 2, true
				in.Random = func() uint16 { return 0 }
				beforeAmmo := s.init.Stores.Ships[0].Stations[1].Ammunition
				if err := s.Fire(in); err != nil {
					t.Fatal(err)
				}
				shot := s.Slots[s.LastSlot]
				if shot.Speed != speed+50 || shot.Target != 1 || s.init.Stores.Ships[0].Stations[1].Ammunition != beforeAmmo-1 {
					t.Fatal("first launch producer divergence", shot)
				}
				for n := 0; n < 50; n++ {
					if err := s.Step(in); err != nil {
						t.Fatal(err)
					}
					q := s.Slots[s.LastSlot]
					acc := int16(binary.LittleEndian.Uint16(s.init.Entities[1].DynamicRaw[4:]))
					raw, err := json.Marshal(map[string]any{"E2": speed, "range": rangeUnits, "step": n + 1, "slot": s.LastSlot, "projectile": q, "target": s.init.Entities[1].Pose, "target_participation": s.init.Entities[1].Participation, "damage_accumulator": acc, "credit": s.Credit})
					if err != nil {
						t.Fatal(err)
					}
					t.Log("WEAPON_TRACE", string(raw))
					if q.Lifetime == 0 {
						break
					}
				}
				wantHit := rangeUnits < 23200 || speed == 120
				hit := len(events(s, "impact")) != 0
				if hit != wantHit {
					t.Fatal("source practical contact boundary changed", hit, wantHit, s.Slots[s.LastSlot])
				}
				acc := int16(binary.LittleEndian.Uint16(s.init.Entities[1].DynamicRaw[4:]))
				if hit && acc != 100 || !hit && acc != 0 {
					t.Fatal("guidance/contact accepted but damage first diverged", acc, hit)
				}
				if hit && s.init.Entities[1].Kind != 3 {
					t.Fatal("one100-point hit against251 resistance was mislabelled destruction")
				}
			})
		}
	}
}

func TestProductionHellfireSelectionLossAndReacquisitionTrace(t *testing.T) {
	s, in := capturedHellfire(t, 0x0e10)
	in.Target, in.TargetAvailable = 8, true
	if err := s.Step(in); err != nil {
		t.Fatal(err)
	}
	if p := s.Slots[5]; p.Target != 38 || p.Pose.Angles[1] != 65285 || p.Pose.Angles[0] != 65244 {
		t.Fatal("first original steering witness divergence", p)
	}
	for i, available := range []bool{false, true} {
		before := s.Slots[5]
		in.TargetAvailable = available
		if err := s.Step(in); err != nil {
			t.Fatal(err)
		}
		after := s.Slots[5]
		if after.Target != 38 || !available && after.Pose.Angles != before.Pose.Angles {
			t.Fatal("selection loss changed stored identity or applied local steering", before, after)
		}
		raw, _ := json.Marshal(map[string]any{"step": i + 2, "selected": in.Target, "available": available, "stored": after.Target, "before": before, "after": after})
		t.Log("GUIDANCE_TRACE", string(raw))
	}
}
