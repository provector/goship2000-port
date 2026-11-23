package avionics

import "gunship2000-assets/game/diagnostics"

type selectionContact struct{ Row, Flags, Kind byte }
type selectionProgress struct {
	Target, Cursor, Suppressed uint16
	BodyStatus, HeadStatus     uint16
	Candidates                 [CandidateCapacity]Candidate
	Contacts                   [CandidateCapacity + 1]selectionContact
	Count                      int
}

// A phase owns one immutable input and candidate snapshot. Only the complete
// retained selection inputs form progress; query positions/counts are reset by
// query, and host time/RNG/animation cannot make a failed state new.
func (s *State) revalidateSelected(in Input, validate func(Input) (bool, error)) error {
	if !in.ProgressAwareTargeting {
		for attempt := 0; s.Target != 0 && attempt < 8; attempt++ {
			done, err := validate(in)
			if err != nil {
				return err
			}
			if done {
				break
			}
			if attempt == 7 {
				s.QueryRetryBoundHits++
				s.Target = 0
			}
		}
		return nil
	}
	candidates := s.Candidates
	var present [EntityCapacity]bool
	for _, c := range candidates {
		if c.Distance != 0xffff && int(c.Row) < len(present) {
			present[c.Row] = true
		}
	}
	if s.Target > 0 && s.Target <= EntityCapacity {
		present[s.Target-1] = true
	}
	var rows []int
	for row, ok := range present {
		if ok {
			rows = append(rows, row)
		}
	}
	limit := 2*len(rows) + 2
	seen := make(map[selectionProgress]struct{}, limit)
	for visits := 0; s.Target != 0; visits++ {
		key := selectionProgress{Target: s.Target, Cursor: s.Cursor, Suppressed: s.Suppressed, BodyStatus: s.BodyQuery.Status, HeadStatus: s.HeadQuery.Status, Candidates: candidates, Count: len(rows)}
		for i, row := range rows {
			c := s.Contacts[row]
			key.Contacts[i] = selectionContact{byte(row), c.Flags, c.Kind}
		}
		if _, ok := seen[key]; ok {
			s.endTargetRetry(1, "repeated complete selection state", visits, limit, key)
			break
		}
		if visits >= limit {
			s.endTargetRetry(2, "selection progress watchdog", visits, limit, key)
			break
		}
		seen[key] = struct{}{}
		done, err := validate(in)
		if err != nil {
			return err
		}
		if done {
			break
		}
	}
	return nil
}

func (s *State) endTargetRetry(reason uint8, cause string, visits, limit int, key selectionProgress) {
	s.QueryRetryBoundHits++
	s.Target = 0
	if s.retryLoggedReasons&reason != 0 {
		return
	}
	s.retryLoggedReasons |= reason
	args := []any{"cause", cause, "target", key.Target, "cursor", key.Cursor, "visits", visits, "limit", limit, "rows", key.Contacts[:key.Count], "body_status", key.BodyStatus, "head_status", key.HeadStatus}
	if reason == 2 {
		diagnostics.Warn("avionics.target", "Target acquisition ended without a lock", args...)
	} else {
		diagnostics.Debug("avionics.target", "Target acquisition ended without a lock", args...)
	}
}

// validateSelected is one complete original4733..4D1C visit. A failed visit
// can publish new observations and reselect the same target; the caller decides
// whether that entry state represents progress. It consumes no random word.
func (s *State) validateSelected(in Input) (bool, error) {
	row := int(s.Target) - 1
	e := in.Entities[row]
	limit := Envelope(in.Player.Height, in.Speed, in.Switches&0x100 != 0, in.Switches&0x200 != 0, false)
	if !e.Present || !in.ModernTail && e.Participation != 1 {
		s.Target = 0
		return true, nil
	}
	outsideBody := Metric(Delta(e.Pose.H0, in.Player.H0), Delta(e.Pose.H1, in.Player.H1)) > limit
	if outsideBody && !in.Mast {
		s.Target = 0
		return true, nil
	}
	if outsideBody {
		s.BodyQuery.Status = 0
	} else if err := s.query(&s.BodyQuery, in.Player, e, in, 0); err != nil {
		return false, err
	}
	origin := in.Player
	if in.Mast {
		origin = in.Head
	}
	if err := s.query(&s.HeadQuery, origin, e, in, 0, origin.Angles[1]); err != nil {
		return false, err
	}
	if s.Target == 0 {
		return true, nil
	}
	if s.HeadQuery.Status != 0 {
		if in.Mast && s.BodyQuery.Status == 0 {
			s.Contacts[row].Flags |= 0xf8
		}
		return true, nil
	}
	s.Cursor = 0
	s.Target, s.Cursor = s.selectTarget(0)
	if s.Target == s.Suppressed {
		s.Target = 0
	}
	return false, nil
}
