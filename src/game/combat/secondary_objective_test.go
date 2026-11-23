package combat

import (
	"encoding/binary"
	"encoding/json"
	"reflect"
	"testing"

	gamestate "gunship2000-assets/game/state"
)

func secondaryFixture(t *testing.T) (*State, Input) {
	t.Helper()
	s, in := objectiveFixture(t, 1, 3)
	m := s.init
	m.SafeGameplay = true
	// Keep the primary candidate distinct from every expired secondary row.
	m.Entities[10].Object.Raw[0], m.Entities[10].Kind = 0x10, 0x15
	for row, flags := range map[int]byte{11: 0x40, 12: 0x20, 14: 0x20, 15: 0x64} {
		m.Entities[row].RuntimeRow = row
		m.Entities[row].Object.Raw[0], m.Rows[row].Raw[0] = flags, flags
		m.Entities[row].Kind = 0x15
	}
	m.Entities[11].Kind = 4
	binary.LittleEndian.PutUint16(m.Targets.Records[3][52:], 1)
	for i := range m.Targets.Records[4] {
		m.Targets.Records[4][i] = byte(100 + i)
	}
	binary.LittleEndian.PutUint16(m.Targets.Records[4][32:], 3)
	binary.LittleEndian.PutUint16(m.Targets.Records[4][50:], 77)
	binary.LittleEndian.PutUint16(m.Targets.Records[4][52:], 19)
	m.Targets.InitObjectiveRevisions()
	return s, in
}

func TestSecondaryRetiresCurrentRowsAcrossPrimaryLocals(t *testing.T) {
	for name, trace := range map[string]primaryScanTrace{
		"never-written":            {},
		"different-valid-primary":  {executed: true, finalIndex: 10},
		"exhausted80":              {executed: true, finalIndex: 80},
		"unimplemented-old-writer": {executed: true, finalIndex: 10, unsupportedActor: true},
	} {
		t.Run(name, func(t *testing.T) {
			s, in := secondaryFixture(t)
			m := s.init
			primary := m.Entities[10]
			primaryRevision := m.Targets.ObjectiveRevisions[0]
			wantSource := m.Targets.Records[4]
			s.Secondary.Destroyed, m.ObjectiveAssignments[1] = 6, 3
			if !s.advanceSecondary(in, trace) || m.Targets.ProgressFailure != "" {
				t.Fatal("legitimate alternate rejected", m.Targets.ProgressFailure)
			}
			if m.Entities[10] != primary {
				t.Fatal("unrelated valid primary candidate retired")
			}
			for row, wantFlag := range map[int]byte{12: 0x40, 14: 0x40, 15: 4} {
				if m.Entities[row].Kind != 0x41 || m.Entities[row].DynamicRaw[0] != 0x41 || m.Entities[row].Object.Raw[0] != wantFlag || m.Rows[row].Raw[0] != wantFlag {
					t.Fatal("wrong cleanup row/flag", row, m.Entities[row])
				}
			}
			if m.Entities[11].Kind != 4 || m.Entities[11].Object.Raw[0] != 0x20 {
				t.Fatal("alternate flag owner incorrectly retired")
			}
			if m.Targets.Records[3] != wantSource || m.ObjectiveAssignments[1] != 3 || s.Secondary.Destroyed != 6 || s.Secondary.Required != 77 {
				t.Fatal("full62 copy invented a counter/assignment reset")
			}
			if m.Targets.ObjectiveRevisions[0] != primaryRevision || m.Targets.ObjectiveRevisions[1] != (gamestate.ObjectiveRevision{Seq: 2, Source: wantSource}) {
				t.Fatal("promotion lineage disagrees with source", m.Targets.ObjectiveRevisions)
			}
			var retired []int
			for _, event := range events(s, "objective-contact-kind") {
				retired = append(retired, event.Target)
			}
			if !reflect.DeepEqual(retired, []int{12, 14, 15}) {
				t.Fatal("cleanup contact order", retired)
			}
			before, _ := json.Marshal(m.Targets)
			if !s.advanceSecondary(in, trace) || binary.LittleEndian.Uint16(m.Targets.Records[3][52:]) != 18 || m.Targets.ObjectiveRevisions[1].Seq != 2 {
				t.Fatal("promotion repeated or new timer decremented twice")
			}
			var restored gamestate.MissionTargets
			if err := json.Unmarshal(before, &restored); err != nil || restored.ObjectiveRevisions[1].Seq != 2 || restored.ObjectiveRevisions[1].Source != wantSource {
				t.Fatal("checkpoint lost current revision", err)
			}
		})
	}
}

func TestSecondaryNoCandidateCountdownAndSuppressorControls(t *testing.T) {
	for _, mode := range []string{"no-candidate", "zero", "suppressed", "legacy"} {
		t.Run(mode, func(t *testing.T) {
			s, in := secondaryFixture(t)
			m := s.init
			before := m.Targets.Records[3]
			beforeRevision := *m.Targets.ObjectiveRevisions
			if mode == "no-candidate" {
				for _, row := range []int{12, 14, 15} {
					m.Entities[row].Kind = 4
				}
			}
			if mode == "zero" {
				binary.LittleEndian.PutUint16(m.Targets.Records[3][52:], 0)
			}
			if mode == "suppressed" {
				m.Entities[11].Kind = 0x1c
			}
			if mode == "legacy" {
				m.SafeGameplay = false
				m.Targets.ObjectiveRevisions = nil
			}
			s.advanceSecondary(in)
			switch mode {
			case "no-candidate":
				if m.Targets.ProgressFailure != "" || m.Targets.Records[3] != m.Targets.Records[4] || len(events(s, "objective-contact-kind")) != 0 || m.Entities[12].Object.Raw[0] != 0x40 {
					t.Fatal("no-candidate promotion fabricated cleanup or rejected")
				}
			case "zero", "suppressed":
				wantTimer := uint16(0)
				if mode == "zero" {
					wantTimer = 0xffff
				}
				binary.LittleEndian.PutUint16(before[52:], wantTimer)
				if m.Targets.Records[3] != before || *m.Targets.ObjectiveRevisions != beforeRevision || len(s.Events) != 0 || m.Entities[12].Kind != 0x15 {
					t.Fatal("source boundary performed promotion")
				}
			case "legacy":
				if m.Targets.ProgressFailure == "" || m.Targets.ObjectiveRevisions != nil || m.Entities[12].Kind != 0x15 {
					t.Fatal("historical unknown-local guard changed")
				}
			}
		})
	}
}

func TestSecondaryImmediatelyExecutesActualAlternatePickup(t *testing.T) {
	s, in := objectiveFixture(t, 1, 3)
	m := s.init
	m.SafeGameplay = true
	binary.LittleEndian.PutUint16(m.Targets.Records[3][52:], 1)
	alt := &m.Targets.Records[4]
	binary.LittleEndian.PutUint16(alt[32:], 2)
	binary.LittleEndian.PutUint32(alt[34:], m.Entities[0].Pose.H0)
	binary.LittleEndian.PutUint32(alt[38:], m.Entities[0].Pose.H1)
	binary.LittleEndian.PutUint16(alt[50:], 1)
	binary.LittleEndian.PutUint16(alt[52:], 19)
	alt[61] = 0xa5
	m.Entities[12].Object.Raw[0], m.Entities[12].Kind, m.Entities[12].Participation = 0x40, 0x15, 1
	m.Targets.InitObjectiveRevisions()
	wantSource := *alt
	s.AdvanceObjectives(in)
	if m.Targets.ProgressFailure != "" || m.Entities[10].Kind != 0x41 || m.Entities[12].Kind != 0x41 || !s.Secondary.Achieved || m.ObjectiveAssignments[1] != 1 || m.Stores.Ships[0].Flags&0xf8 != 0x18 {
		t.Fatal("alternate did not execute real pickup", m.Targets.ProgressFailure, s.Secondary, m.ObjectiveAssignments)
	}
	if m.Targets.ObjectiveRevisions[1] != (gamestate.ObjectiveRevision{Seq: 2, Source: wantSource, Completed: true}) || binary.LittleEndian.Uint16(m.Targets.Records[3][32:]) != 0 || binary.LittleEndian.Uint16(m.Targets.Records[3][52:]) != 19 {
		t.Fatal("same-tick completion lost precompletion revision")
	}
	notices := events(s, "notification")
	if len(notices) != 2 || notices[0].Notification != 0x0d || notices[1].Notification != 0x11 {
		t.Fatal("retask and pickup source notice order", notices)
	}
	before := len(s.Events)
	s.AdvanceObjectives(in)
	if len(s.Events) != before || m.Targets.ObjectiveRevisions[1].Seq != 2 {
		t.Fatal("completed alternate notified twice")
	}
}

func TestObjectiveRevisionMarksRealPlayerAndFollowerRecon(t *testing.T) {
	for _, follower := range []bool{false, true} {
		s, in, _ := followerFixture(t)
		m := s.init
		m.SafeGameplay = true
		s.SetLogisticsEnabled(follower)
		actor := 0
		if follower {
			actor = 1
		}
		m.Stores.Ships[actor].Flags = 0x26
		goal := &m.Targets.Records[2]
		binary.LittleEndian.PutUint16(goal[32:], 4)
		binary.LittleEndian.PutUint32(goal[34:], m.Entities[actor].Pose.H0)
		binary.LittleEndian.PutUint32(goal[38:], m.Entities[actor].Pose.H1)
		goal[61] = 0xbc
		m.Targets.InitObjectiveRevisions()
		want := *goal
		in.Random = func() uint16 { t.Fatal("objective revision consumed RNG"); return 0 }
		s.AdvanceObjectives(in)
		if !s.Primary.Achieved || goal[32] != 0 || m.Targets.ObjectiveRevisions[0] != (gamestate.ObjectiveRevision{Seq: 1, Source: want, Completed: true}) {
			t.Fatal("actual player/follower recon did not own completion", follower, s.Primary, m.Targets.ObjectiveRevisions)
		}
	}
}

func TestSecondaryPostPromotionRestoreDoesNotRepeatTransition(t *testing.T) {
	s, in := secondaryFixture(t)
	if !s.advanceSecondary(in) {
		t.Fatal("initial promotion did not execute")
	}
	encoded, err := json.Marshal(s.init)
	if err != nil {
		t.Fatal(err)
	}
	var restored gamestate.InitializedMission
	if err = json.Unmarshal(encoded, &restored); err != nil {
		t.Fatal(err)
	}
	continued := *s
	continued.init = &restored
	continued.Events = append([]Event(nil), s.Events...)
	s.AdvanceObjectives(in)
	continued.AdvanceObjectives(in)
	left, _ := json.Marshal(s.init)
	right, _ := json.Marshal(continued.init)
	if !reflect.DeepEqual(left, right) || !reflect.DeepEqual(s.FilmState(), continued.FilmState()) || !reflect.DeepEqual(s.Events, continued.Events) {
		t.Fatal("restored promotion did not continue the same authoritative state")
	}
	if restored.Targets.ObjectiveRevisions[1].Seq != 2 || binary.LittleEndian.Uint16(restored.Targets.Records[3][52:]) != 18 || len(events(&continued, "notification")) != 1 {
		t.Fatal("restoration repeated promotion, timer decrement or notice")
	}
}
