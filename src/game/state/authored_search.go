package state

import (
	"fmt"
	assetdata "gunship2000-assets/game/assets"
)

const AuthoredSearchProfile = "source-editor-search-point-v1"

func authoredSearchWorld(p AuthoredSearchPoint) MissionPoint {
	return MissionPoint{int32(341*int(int16(p.X)) - 1), int32(-455*int(int16(p.Y)) + 0x7ffff)}
}
func validateAuthoredSearch(d *AuthoredDocument, p AuthoredPlacement) error {
	if p.Search == nil {
		return nil
	}
	if d.Initial != nil {
		return fmt.Errorf("retained film geometry cannot own an editable search marker")
	}
	s := p.Search
	if d.Compiler != AuthoredCompilerP5 && d.Compiler != AuthoredCompilerSupportP5 {
		return fmt.Errorf("search marker requires source compiler v2")
	}
	if (p.Role != "primary" && p.Role != "secondary") || p.Kind > 1 || p.Kind == 0 && !authoredGoalHasCarrier(p) || s.Profile != AuthoredSearchProfile {
		return fmt.Errorf("search marker has no admitted search-objective owner")
	}
	w := authoredSearchWorld(*s)
	if s.X < 0 || s.X >= 1536 || s.Y < 0 || s.Y >= 1152 || w.X < 0 || w.Y < 0 || w.X > 0x7ffff || w.Y > 0x7ffff {
		return fmt.Errorf("search marker outside source world")
	}
	return nil
}

// Source1CF6 stores raw reached editor coordinates (no formation snap), then
// compiler221F..224A transforms the independent TARGET original/search position.
func SetAuthoredSearchPoint(data *assetdata.MissionGenerationData, d *AuthoredDocument, role string, x, y int) error {
	if d.Initial != nil {
		return fmt.Errorf("retained film search geometry requires inverse construction")
	}
	q, e := CloneAuthored(d)
	if e != nil {
		return e
	}
	if q.Support != nil {
		q.Compiler = AuthoredCompilerSupportP5
	} else {
		q.Compiler = AuthoredCompilerP5
	}
	for i := range q.Placements {
		p := &q.Placements[i]
		if p.Role == role && p.Kind < 2 {
			p.Search = &AuthoredSearchPoint{Profile: AuthoredSearchProfile, X: x, Y: y}
			if e = validateAuthoredSearch(q, *p); e != nil {
				return e
			}
			q.Compiled = nil
			*d = *q
			return nil
		}
	}
	return fmt.Errorf("no search goal for marker")
}
