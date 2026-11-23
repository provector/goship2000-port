package state

import (
	"encoding/binary"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
)

const AuthoredRouteProfile = "source-editor-intercept-planner-v1"

// Endpoint is the reached second editor position. The source1590 planner is
// deterministic; immutable terrain plus this endpoint remain the single authored
// truth. SAVE freezes the realized nine-byte PATH span separately.
type AuthoredRoute struct {
	Profile string `json:"profile"`
	X       int    `json:"x"`
	Y       int    `json:"y"`
}

func authoredInterceptEligible(flags uint32) bool {
	return flags&0x08200380 != 0 && flags&0x00100000 == 0
}

func authoredPlannedRoute(data *assetdata.MissionGenerationData, d *AuthoredDocument, p AuthoredPlacement) ([]missionPath, error) {
	if p.Route != nil && p.Route.Profile == AuthoredTrainRouteProfile && p.Kind == 2 && (p.Role == "primary" || p.Role == "secondary") && authoredGoalHasCarrier(p) {
		w, err := AuthoredWorld(data, d.World)
		if err != nil {
			return nil, err
		}
		if d.Initial != nil || (d.Compiler != AuthoredCompilerP5 && d.Compiler != AuthoredCompilerSupportP5) {
			return nil, fmt.Errorf("Train route requires its source compiler and immutable preset")
		}
		want, err := authoredTrainRoute(w, p)
		if err != nil || *want != *p.Route {
			return nil, fmt.Errorf("Train route endpoint differs from the source preset")
		}
		return authoredTrainRows(w, p)
	}
	if p.Route == nil || p.Route.Profile != AuthoredRouteProfile || p.Kind != 2 || (p.Role != "primary" && p.Role != "secondary") || !authoredGoalHasCarrier(p) {
		return nil, fmt.Errorf("Intercept Moving needs its reached destination")
	}
	if d.Initial != nil || (d.Compiler != AuthoredCompilerP5 && d.Compiler != AuthoredCompilerSupportP5) {
		return nil, fmt.Errorf("intercept route requires source compiler v2")
	}
	r := p.Route
	if r.X < 0 || r.X >= 1536 || r.Y < 0 || r.Y >= 1152 {
		return nil, fmt.Errorf("intercept destination outside source map")
	}
	w, err := AuthoredWorld(data, d.World)
	if err != nil {
		return nil, err
	}
	if p.Formation < 0 || p.Formation >= len(w.FMHD)/39 {
		return nil, fmt.Errorf("intercept formation outside source table")
	}
	flags := binary.LittleEndian.Uint32(w.FMHD[p.Formation*39+35:])
	if !authoredInterceptEligible(flags) {
		return nil, fmt.Errorf("Intercept Moving requires an eligible moving formation")
	}
	if flags&0x200 != 0 {
		return nil, fmt.Errorf("source preset-road Intercept requires its exact private PATH span")
	}
	rows, err := authoredPlacementComponents(w, p)
	if err != nil {
		return nil, err
	}
	if len(rows) > 16 {
		return nil, fmt.Errorf("intercept components exceed packed source offset slots")
	}
	for _, ci := range rows {
		ti, e := authoredTemplate(w, ci)
		if e != nil {
			return nil, e
		}
		kind := w.OBJL[ti*32+10]
		if kind != 0 && (kind < 2 || kind > 7) {
			return nil, fmt.Errorf("Intercept Moving component kind%02x has no admitted private-route consumer", kind)
		}
	}
	g, err := authoredPopulation(w, authoredRNG(0))
	if err != nil {
		return nil, err
	}
	out, ok := g.planner([2]int{p.X / 24, 63 - p.Y/18}, [2]int{r.X / 24, 63 - r.Y/18}, flags)
	if !ok || g.rng.err != nil {
		return nil, fmt.Errorf("intercept route is blocked by source terrain; draft retained")
	}
	if len(out) < 2 || len(out) > 7281 {
		return nil, fmt.Errorf("intercept PATH capacity outside source domain")
	}
	return out, nil
}
func SetAuthoredRoutePoint(data *assetdata.MissionGenerationData, d *AuthoredDocument, role string, x, y int) error {
	q, err := CloneAuthored(d)
	if err != nil {
		return err
	}
	if q.Support != nil {
		q.Compiler = AuthoredCompilerSupportP5
	} else {
		q.Compiler = AuthoredCompilerP5
	}
	for i := range q.Placements {
		p := &q.Placements[i]
		if p.Role == role && p.Kind == 2 {
			p.Route = &AuthoredRoute{Profile: AuthoredRouteProfile, X: x, Y: y}
			if _, err = authoredPlannedRoute(data, q, *p); err != nil {
				return err
			}
			q.Compiled = nil
			*d = *q
			return nil
		}
	}
	return fmt.Errorf("no Intercept Moving goal for destination")
}

// Source1ABF1980..1B88 inserts DS2734 before the chosen span. Children keep
// copied template bytes, get flag1 and full signed FRMD offsets; the inserted
// header owns source group2/3. Neither row order nor target count is padded.
func authoredAttachRoute(data *assetdata.MissionGenerationData, d *AuthoredDocument, g *genericMissionPopulation, p AuthoredPlacement, start, count int, marks byte) ([]missionPath, error) {
	route, err := authoredPlannedRoute(data, d, p)
	if err != nil {
		return nil, err
	}
	if len(data.Tables.Leader) != 32 || len(g.objects) >= 80 {
		return nil, fmt.Errorf("intercept header/source capacity unavailable")
	}
	rows, err := authoredPlacementComponents(g.source, p)
	if err != nil {
		return nil, err
	}
	if len(rows) != count {
		return nil, fmt.Errorf("intercept component span differs")
	}
	children := append([]MissionObject(nil), g.objects[start:start+count]...)
	for j, ci := range rows {
		c := g.source.FRMD[ci*8 : ci*8+8]
		children[j].Raw[0] |= 1
		binary.LittleEndian.PutUint32(children[j].Raw[2:], uint32(int32(int16(binary.LittleEndian.Uint16(c[4:])))))
		binary.LittleEndian.PutUint32(children[j].Raw[6:], uint32(int32(int16(binary.LittleEndian.Uint16(c[6:])))))
	}
	var leader MissionObject
	leader.Formation = p.Formation
	copy(leader.Raw[:], data.Tables.Leader)
	leader.Raw[0] |= marks
	binary.LittleEndian.PutUint32(leader.Raw[6:], uint32(g.groups))
	g.groups++
	g.objects = append(g.objects[:start], leader)
	g.objects = append(g.objects, children...)
	g.paths = append(g.paths, route...)
	return route, nil
}

// AuthoredRouteDisplay derives the exact accepted source planner polyline. The
// editor caches this immutable result by descriptor/endpoint, not pointer frame.
func AuthoredRouteDisplay(data *assetdata.MissionGenerationData, d *AuthoredDocument, p AuthoredPlacement) ([][2]int, error) {
	rows, err := authoredPlannedRoute(data, d, p)
	if err != nil {
		return nil, err
	}
	out := make([][2]int, len(rows))
	for i, row := range rows {
		out[i] = [2]int{int(row.Point.X)/341 - 1, int(row.Point.Y)/-455 + 1151}
	}
	return out, nil
}

func authoredHasInterceptLayout(c *AuthoredRealization, kind MissionType) bool {
	if c == nil || kind != MissionSimulated {
		return false
	}
	for _, role := range []string{"primary", "secondary"} {
		g, ok := c.Objectives[role]
		if ok && g.Endpoint != nil && g.RawKind == 3 && g.DisplayKind == 3 && g.ObjectIndex >= 9 && g.ObjectIndex < len(c.Objects) && c.Objects[g.ObjectIndex].Raw[10] == 0x40 {
			return true
		}
	}
	return false
}
