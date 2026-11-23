package combat

// Native bounded friendly navigation. The original route, update, movement,
// altitude and arrival owners remain authoritative. Only the temporary heading
// targets below are a native QoL interpretation selected before launch.
import (
	"encoding/binary"
	"fmt"
	"gunship2000-assets/game/diagnostics"
	gamestate "gunship2000-assets/game/state"
	"gunship2000-assets/game/world"
	"log/slog"
)

type NavigationPoint struct{ H0, H1 uint32 }
type NavigationMember struct {
	Active bool
	Model  int16
	Parent int8
	Code   byte
	Offset [2]int16
}
type NavigationKey struct {
	Kind, Index     uint8
	Goal            NavigationPoint
	Altitude, Speed uint16
	LowLimit        int16
	Support         uint16
	Members         [5]NavigationMember
}
type FollowerNavigation struct {
	Key           NavigationKey
	Start         NavigationPoint
	Points        [8]NavigationPoint
	Count, Cursor uint8
	Direct        bool
	Failed        bool
	FailedAt      NavigationPoint
	FailedHeights [5]int16
	Retry         uint8
	BestRemaining uint32 `json:",omitempty"` // progress toward this temporary point in horizontal units
	StalledVisits uint8  `json:",omitempty"`
	Clearance     int16  `json:",omitempty"` // temporary body-height target for a verified local obstruction
	Failure       uint8  //1 bounded/no path;2 unsupported query/model. No error text is authoritative.
}

func (s *State) SetObstacleClearance(enabled bool) { s.obstacleClearance = enabled }

type NavigationState [5]FollowerNavigation
type navigationPlanStats struct {
	Requests, Hits, Entries, Queries int
	Clearance                        int16
	Direct, Budget                   bool
	Error                            string
}

func (s *State) SetNavigationEnabled(enabled bool) {
	s.FlushDiagnostics()
	s.navigationEnabled = enabled
	s.navigation = NavigationState{}
	s.navigationDiagnostics = [5]navigationDiagnostic{}
}
func (s *State) ResetNavigation(actor int) {
	if s.navigationEnabled && actor > 0 && actor < len(s.navigation) {
		s.flushNavigationDiagnostic(actor)
		s.navigationDiagnostics[actor] = navigationDiagnostic{}
		s.navigation[actor] = FollowerNavigation{}
	}
}
func (s *State) filmNavigation() *NavigationState {
	if !s.navigationEnabled {
		return nil
	}
	n := s.navigation
	return &n
}
func navigationDistance(a, b NavigationPoint) int64 {
	x, z := int64(a.H0)-int64(b.H0), int64(a.H1)-int64(b.H1)
	return x*x + z*z
}
func navigationAbs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// Passing a segment is distinct from completing a source waypoint. A point
// behind a fresh start never passes this test. A displaced aircraft replans
// rather than skipping a remote corner or circling back indefinitely.
func navigationPassed(from, end, pos NavigationPoint) (bool, bool) {
	x, z := int64(end.H0)-int64(from.H0), int64(end.H1)-int64(from.H1)
	px, pz := int64(pos.H0)-int64(from.H0), int64(pos.H1)-int64(from.H1)
	if navigationDistance(pos, end) < 128*128 {
		return true, false
	}
	if (int64(end.H0)-int64(pos.H0))*x+(int64(end.H1)-int64(pos.H1))*z > 0 {
		return false, false
	}
	cross := x*pz - z*px
	if cross < 0 {
		cross = -cross
	}
	if cross > 512*int64(max(navigationAbs(int(x)), navigationAbs(int(z)))) {
		return false, true
	}
	return true, false
}
func (s *State) navigationHeights(key NavigationKey) (out [5]int16) {
	for actor, m := range key.Members {
		if m.Active && actor < len(s.init.Entities) {
			out[actor] = int16(s.init.Entities[actor].Pose.Height)
		}
	}
	return
}
func (s *State) navigationKey(actor int, e *gamestate.InitializedEntity, goal NavigationPoint, in Input) (NavigationKey, error) {
	k := NavigationKey{Kind: e.Kind, Index: e.ControlRaw[0], Goal: goal, Altitude: binary.LittleEndian.Uint16(e.ControlRaw[6:]), Speed: binary.LittleEndian.Uint16(e.ControlRaw[8:]), LowLimit: in.LowLimit, Support: s.init.Switches & 0x1800}
	for member := 1; member < 5 && member < len(s.init.Entities); member++ {
		v := &s.init.Entities[member]
		if member != actor && (v.Kind != 0x21 || int(int8(v.ControlRaw[5])) != actor) {
			continue
		}
		m := NavigationMember{Active: true, Model: int16(v.ModelID), Parent: int8(v.ControlRaw[5]), Code: v.DynamicRaw[2]}
		if member != actor {
			off, err := s.formationOffset(v)
			if err != nil {
				return k, err
			}
			m.Offset = off
		}
		k.Members[member] = m
	}
	return k, nil
}

// The caller MUST evaluate followerRouteBlocked first. Invariant source
// booleans are sufficient: the native planner's extra queries have their own
// explicit current/cruise-reference height and never invent incoming BP-30.
func (s *State) navigationHeading(actor int, e *gamestate.InitializedEntity, heading uint16, x, z uint32, blocked bool, in Input) (uint16, bool) {
	if !s.navigationEnabled || actor < 1 || actor > 4 {
		return heading, false
	}
	if (e.Kind != 0x22 && e.Kind != 0x23) || e.Participation != 1 {
		s.ResetNavigation(actor)
		return heading, false
	}
	n := &s.navigation[actor]
	goal := NavigationPoint{x, z}
	pos := NavigationPoint{e.Pose.H0, e.Pose.H1}
	key, err := s.navigationKey(actor, e, goal, in)
	if err != nil {
		if diagnostics.Enabled(slog.LevelWarn) {
			s.noteNavigationDiagnostic(actor, key, "unavailable", 0, navigationPlanStats{Error: err.Error()})
		}
		*n = FollowerNavigation{Key: key, Failed: true, FailedAt: pos, Failure: 2}
		return heading, false
	}
	fresh := n.Key != key || (n.Clearance > 0 && !s.sectionCanClimb(key))
	if fresh {
		*n = FollowerNavigation{Key: key}
	}
	forced := fresh
	if n.Cursor < n.Count {
		if n.Direct && n.Points[n.Cursor] != goal && navigationDistance(pos, n.Points[n.Cursor]) < 4096*4096 {
			n.Count, n.Cursor = 0, 0
			forced = true
		}
		for n.Cursor < n.Count {
			from := n.Start
			if n.Cursor > 0 {
				from = n.Points[n.Cursor-1]
			}
			passed, off := navigationPassed(from, n.Points[n.Cursor], pos)
			if off {
				n.Count, n.Cursor = 0, 0
				forced = true
				break
			}
			if !passed {
				break
			}
			n.Cursor++
			n.BestRemaining, n.StalledVisits = 0, 0
		}
		if s.corridorNavigation && n.Cursor < n.Count && !n.Direct {
			next := n.Points[n.Cursor]
			remaining := uint32(max(navigationAbs(int(pos.H0)-int(next.H0)), navigationAbs(int(pos.H1)-int(next.H1))))
			if n.BestRemaining == 0 || remaining+32 <= n.BestRemaining {
				n.BestRemaining, n.StalledVisits = remaining, 0
			} else {
				n.StalledVisits++
			}
			// Allow the source turn to settle. Never orbit an unpassed temporary
			// corner indefinitely when steering has stopped making forward progress.
			if n.StalledVisits >= 48 {
				n.Count, n.Cursor = 0, 0
			}
		}
		if n.Cursor < n.Count {
			if n.Direct {
				return heading, true
			}
			p := n.Points[n.Cursor]
			dx := int16((int32(p.H0) >> 4) - (int32(pos.H0) >> 4))
			dz := int16((int32(p.H1) >> 4) - (int32(pos.H1) >> 4))
			return s.angle(-dx, dz), true
		}
		n.Count, n.Cursor = 0, 0
		forced = true
	}
	if forced && n.Count == 0 {
		n.Clearance = 0 // Re-evaluate ordinary flight at the end of the local leg.
	}
	if !blocked && !forced && n.Clearance == 0 {
		return heading, false
	}
	if n.Failed && n.Retry != 16 && navigationDistance(pos, n.FailedAt) < 2048*2048 {
		if s.navigationHeights(key) == n.FailedHeights {
			n.Retry = 0
			return heading, false
		}
		n.Retry++
		if n.Retry < 16 {
			return heading, false
		}
		n.Retry = 0
	}
	points, stats := s.planFollowerNavigation(actor, e, in, pos, goal)
	if len(points) == 0 {
		failure := uint8(1)
		if stats.Error != "" {
			failure = 2
		}
		outcome := "no path"
		if stats.Error != "" {
			outcome = "query failed"
		}
		s.noteNavigationDiagnostic(actor, key, outcome, 0, stats)
		previousClearance := n.Clearance
		if s.obstacleClearance && failure == 1 && (blocked || previousClearance > 0) && s.sectionCanClimb(key) {
			n.Clearance = max(n.Clearance, stats.Clearance)
		}
		n.Failed, n.FailedAt, n.Failure = true, pos, failure
		n.FailedHeights = s.navigationHeights(key)
		n.Retry = 0
		if n.Clearance > previousClearance {
			// Raising the planned altitude changes the clearance test even when the
			// aircraft is already high enough. Discard the obsolete failure cache.
			n.Retry = 16
		}
		return heading, false
	}
	n.Failed, n.Failure = false, 0
	n.FailedHeights = [5]int16{}
	n.Retry = 0
	n.BestRemaining, n.StalledVisits = 0, 0
	n.Start = pos
	n.Points = [8]NavigationPoint{}
	n.Count = uint8(copy(n.Points[:], points))
	n.Cursor = 0
	n.Direct = stats.Direct
	s.ResetRouteAvoidance(actor) // Never combine an old release tail with a native verified heading.
	s.noteNavigationDiagnostic(actor, key, "path", int(n.Count), stats)
	if n.Direct {
		return heading, true
	}
	p := n.Points[0]
	dx := int16((int32(p.H0) >> 4) - (int32(pos.H0) >> 4))
	dz := int16((int32(p.H1) >> 4) - (int32(pos.H1) >> 4))
	return s.angle(-dx, dz), true
}

// Only powered, noncritical members can enter a climbing recovery. Critical
// fall/descent and fuel exhaustion keep their existing successor owners.
func (s *State) sectionCanClimb(k NavigationKey) bool {
	for i, m := range k.Members {
		if m.Active && (s.init.Stores == nil || i >= len(s.init.Stores.Damage) || s.init.Stores.Damage[i][9] != 0) {
			return false
		}
	}
	return true
}

// Local bounded33x33 grid with actual source-model footprints. Search and
// temporary points are native QoL; no actor pose or CONTROL byte is written.
func (s *State) planFollowerNavigation(actor int, e *gamestate.InitializedEntity, in Input, start, goal NavigationPoint) (points []NavigationPoint, stats navigationPlanStats) {
	const side = 33
	const center = side / 2
	const step = 1024
	const total = side * side
	queries := 0
	queryLimit, searchLimit := 32768, 28672
	if s.corridorNavigation {
		queryLimit, searchLimit = 8192, 7168
	}
	gate, returning := s.corridorGoal(e)
	returning = returning && gate == goal
	// Only the portion of the footprint still over the world has terrain on
	// the final approach. Sample the actual border there; other paths fail closed.
	atGate := func(p NavigationPoint) NavigationPoint {
		if !returning || p.H0 < worldEdge && p.H1 < worldEdge {
			return p
		}
		dx, dz := int64(int32(p.H0))-int64(gate.H0), int64(int32(p.H1))-int64(gate.H1)
		if dx*dx+dz*dz > 1024*1024 {
			return p
		}
		q := gridProbe(e.Pose, gamestate.InitializedPose{H0: p.H0, H1: p.H1})
		return NavigationPoint{q.H0, q.H1}
	}
	objects := s.deathQueryObjects()
	type refKey struct {
		H0, H1         uint32
		Actual, Parent int16
		Child          bool
	}
	cityRoute := s.lowCityRoute(actor, e)
	var approach int16
	if cityRoute {
		approach = s.city.approach(e.Pose, goal.H0, goal.H1)
	}
	refMemo := make(map[refKey]int16, 1024)
	referenceBody := func(p NavigationPoint, actual int16, child bool, parent int16) (int16, bool) {
		if returning {
			p = atGate(p)
		}
		if p.H0 >= 0x80000 || p.H1 >= 0x80000 {
			return 0, false
		}
		key := refKey{p.H0, p.H1, actual, parent, child}
		if v, ok := refMemo[key]; ok {
			return v, true
		}
		target := int16(0)
		cell, err := s.assets.SourceCellID(s.init.World, world.QueryPose{H0: p.H0, H1: p.H1})
		if err != nil {
			if stats.Error == "" {
				stats.Error = err.Error()
			}
			return 0, false
		}
		if s.init.Switches&0x1000 != 0 && cell == 0x3e && !child {
			target = s.sourceFollowerAltitude(binary.LittleEndian.Uint16(e.ControlRaw[6:])) + in.LowLimit + 360
		} else {
			if queries >= queryLimit {
				stats.Budget = true
				return 0, false
			}
			queries++
			q := world.QueryPose{H0: p.H0, H1: p.H1, Height: actual}
			terrain, err := s.assets.PairedTerrainHeight(s.init.World, q, q)
			if err != nil {
				if stats.Error == "" {
					stats.Error = err.Error()
				}
				return 0, false
			}
			if child {
				target = max(parent, terrain+in.LowLimit)
			} else {
				target = terrain + in.LowLimit + s.sourceFollowerAltitude(binary.LittleEndian.Uint16(e.ControlRaw[6:]))
			}
		}
		if floor := s.navigation[actor].Clearance; floor > 0 {
			target = max(target, floor)
		}
		if cityRoute {
			target = max(target, approach, s.city.at(p.H0, p.H1))
		}
		v := min(actual, target)
		if len(refMemo) < 8192 {
			refMemo[key] = v
		}
		return v, true
	}
	reference := func(p NavigationPoint) (int16, bool) { return referenceBody(p, int16(e.Pose.Height), false, 0) }
	type queryKey struct {
		X, Z   uint32
		Height int16
	}
	memo := make(map[queryKey]int16, 1024)
	defer func() {
		stats.Queries = queries
		stats.Entries = len(memo)
		if stats.Error != "" {
			points = nil
		}
	}()
	height := func(p NavigationPoint, ref int16) (int16, bool) {
		stats.Requests++
		if returning {
			p = atGate(p)
		}
		if p.H0 >= 0x80000 || p.H1 >= 0x80000 {
			return 0, false
		}
		key := queryKey{p.H0, p.H1, ref}
		if v, ok := memo[key]; ok {
			stats.Hits++
			return v, true
		}
		if queries >= queryLimit {
			stats.Budget = true
			return 0, false
		}
		queries++
		h, err := s.assets.ContactHeight(s.init.World, world.QueryPose{H0: p.H0, H1: p.H1, Height: ref}, objects)
		if err != nil {
			if stats.Error == "" {
				stats.Error = err.Error()
			}
			return 0, false
		}
		if len(memo) < 8192 {
			memo[key] = h
		}
		return h, true
	}
	clearCenter := func(p NavigationPoint) bool {
		ref, valid := reference(p)
		if !valid {
			return false
		}
		h, ok := height(p, ref)
		return ok && h <= ref
	}
	type footprint struct {
		off          [2]int16
		vertices     [][2]int16
		minY         int16
		actualHeight int16
		alternate    int16
		child        bool
	}
	fps := []footprint{}
	for member := 1; member < 5; member++ {
		v := &s.init.Entities[member]
		if member != actor && (v.Kind != 0x21 || int(int8(v.ControlRaw[5])) != actor) {
			continue
		}
		if v.ModelID < 0 || v.ModelID >= len(s.assets.Models) {
			stats.Error = fmt.Sprintf("follower model%d outside prepared directory", v.ModelID)
			return nil, stats
		}
		g := s.assets.Geometries[s.assets.Models[v.ModelID]]
		if g == nil || len(g.Vertices) == 0 {
			stats.Error = fmt.Sprintf("follower model%d geometry missing", v.ModelID)
			return nil, stats
		}
		minx, minz, miny := int16(32767), int16(32767), int16(32767)
		maxx, maxz := int16(-32768), int16(-32768)
		for _, v := range g.Vertices {
			minx = min(minx, v[0])
			maxx = max(maxx, v[0])
			minz = min(minz, v[2])
			maxz = max(maxz, v[2])
			miny = min(miny, v[1])
		}
		fp := footprint{minY: miny, actualHeight: int16(v.Pose.Height)}
		for _, x := range []int16{minx, 0, maxx} {
			for _, z := range []int16{minz, 0, maxz} {
				fp.vertices = append(fp.vertices, [2]int16{x, z})
			}
		}
		if member != actor {
			off, err := s.formationOffset(v)
			if err != nil {
				stats.Error = err.Error()
				return nil, stats
			}
			fp.off = off
			fp.child = true
			fp.alternate = int16(int16(off[1]-64)*2) - ((off[0] &^ int16(2)) >> 1)
		}
		fps = append(fps, fp)
	}
	clearFootprints := func(p NavigationPoint, heading uint16) bool {
		for _, fp := range fps {
			leaderRef, valid := reference(p)
			if !valid {
				return false
			}
			ref := leaderRef
			if fp.child {
				ref, valid = referenceBody(p, fp.actualHeight, true, leaderRef)
				if !valid {
					return false
				}
			}
			center := NavigationPoint{p.H0 + uint32(int32(s.cos(heading, fp.off[0]))) - uint32(int32(s.sin(heading, fp.off[1]))), p.H1 + uint32(int32(s.sin(heading, fp.off[0]))) + uint32(int32(s.cos(heading, fp.off[1])))}
			if fp.child {
				for _, d := range [][2]int32{{512, 512}, {512, -512}, {-512, 512}, {-512, -512}} {
					h, ok := height(NavigationPoint{center.H0 + uint32(d[0]), center.H1 + uint32(d[1])}, leaderRef)
					if !ok {
						return false
					}
					if h > leaderRef+16 {
						center = NavigationPoint{p.H0 - uint32(int32(s.sin(heading, fp.alternate))), p.H1 + uint32(int32(s.cos(heading, fp.alternate)))}
						break
					}
				}
			}
			for _, v := range fp.vertices {
				sample := NavigationPoint{center.H0 + uint32(int32(s.cos(heading, v[0]))) - uint32(int32(s.sin(heading, v[1]))), center.H1 + uint32(int32(s.sin(heading, v[0]))) + uint32(int32(s.cos(heading, v[1])))}
				h, ok := height(sample, ref)
				if !ok {
					return false
				}
				if int32(h) > int32(ref)+int32(fp.minY) {
					// Reuse a measured footprint rejection inside the existing look-ahead.
					// The reserve covers integer convergence of the leader and child altitude
					// controllers. Never convert an unknown or unrepresentable top to lift.
					if s.obstacleClearance && h > 0 && navigationDistance(start, sample) <= 4096*4096 {
						required := int32(h) - int32(fp.minY) + 16
						if required > 0 && required <= 32767 {
							stats.Clearance = max(stats.Clearance, int16(required))
						}
					}
					return false
				}
			}
		}
		return true
	}
	segment := func(a, b NavigationPoint) bool {
		dx, dz := int64(b.H0)-int64(a.H0), int64(b.H1)-int64(a.H1)
		heading := s.angle(-int16(dx>>4), int16(dz>>4))
		length := max(navigationAbs(int(dx)), navigationAbs(int(dz)))
		count := max(1, (length+255)/256)
		for i := 0; i <= count; i++ {
			p := NavigationPoint{uint32(int64(a.H0) + dx*int64(i)/int64(count)), uint32(int64(a.H1) + dz*int64(i)/int64(count))}
			if !clearFootprints(p, heading) {
				return false
			}
		}
		return true
	}
	dx, dz := int64(goal.H0)-int64(start.H0), int64(goal.H1)-int64(start.H1)
	if dx == 0 && dz == 0 {
		return nil, stats
	}
	target := goal
	exactGoal := max(navigationAbs(int(dx)), navigationAbs(int(dz))) <= 12*step
	if !exactGoal {
		heading := s.angle(-int16(dx>>4), int16(dz>>4))
		target = NavigationPoint{start.H0 - uint32(int32(s.sin(heading, 12*step))), start.H1 + uint32(int32(s.cos(heading, 12*step)))}
	}
	if s.navigation[actor].Clearance > 0 && max(navigationAbs(int(dx)), navigationAbs(int(dz))) > 2048 {
		h := s.angle(-int16(dx>>4), int16(dz>>4))
		target = NavigationPoint{start.H0 - uint32(int32(s.sin(h, 2048))), start.H1 + uint32(int32(s.cos(h, 2048)))}
		exactGoal = false
	}
	if segment(start, target) {
		stats.Direct = s.navigation[actor].Clearance == 0 || target == goal
		return []NavigationPoint{target}, stats
	}
	// Before grid expansion try short, verified legs toward the goal. They
	// use exactly the same sampled section footprint as the grid edges. A
	// bounded search can publish an already verified prefix without rechecking it.
	if s.corridorNavigation {
		h := s.angle(-int16(dx>>4), int16(dz>>4))
		for _, length := range []int16{4096, 2048, 1024} {
			for _, turn := range []uint16{0, 0x1000, 0xf000, 0x2000, 0xe000, 0x3000, 0xd000} {
				p := NavigationPoint{start.H0 - uint32(int32(s.sin(h+turn, length))), start.H1 + uint32(int32(s.cos(h+turn, length)))}
				if p.H0 < worldEdge && p.H1 < worldEdge && navigationDistance(p, goal) < navigationDistance(start, goal) && segment(start, p) {
					return []NavigationPoint{p}, stats
				}
			}
		}
	}
	rounded := func(v int64) int {
		if v < 0 {
			return -int((-v + step/2) / step)
		}
		return int((v + step/2) / step)
	}
	gx := rounded(int64(target.H0)-int64(start.H0)) + center
	gz := rounded(int64(target.H1)-int64(start.H1)) + center
	if gx < 0 || gx >= side || gz < 0 || gz >= side {
		return nil, stats
	}
	point := func(id int) NavigationPoint {
		return NavigationPoint{uint32(int64(start.H0) + int64(id%side-center)*step), uint32(int64(start.H1) + int64(id/side-center)*step)}
	}
	var cache [total]int8
	free := func(id int) bool {
		if cache[id] == 0 {
			if clearCenter(point(id)) {
				cache[id] = 1
			} else {
				cache[id] = -1
			}
		}
		return cache[id] > 0
	}
	startID, targetID := center+center*side, gx+gz*side
	cache[startID] = 1
	if !exactGoal && !free(targetID) {
		chosen := -1
		for radius := 1; radius <= 8 && chosen < 0; radius++ {
			best := 1 << 30
			for zz := max(0, gz-radius); zz <= min(side-1, gz+radius); zz++ {
				for xx := max(0, gx-radius); xx <= min(side-1, gx+radius); xx++ {
					ddx, ddz := xx-gx, zz-gz
					if max(navigationAbs(ddx), navigationAbs(ddz)) != radius {
						continue
					}
					id := xx + zz*side
					if (xx-center)*(gx-center)+(zz-center)*(gz-center) <= 0 || !free(id) {
						continue
					}
					score := ddx*ddx + ddz*ddz
					if score < best {
						chosen, best = id, score
					}
				}
			}
		}
		if chosen >= 0 {
			targetID = chosen
			gx, gz = targetID%side, targetID/side
		}
	}

	var cost [total]int
	var parent [total]int
	var parentProved [total]bool
	var done [total]bool
	for i := range cost {
		cost[i] = 1 << 29
		parent[i] = -1
	}
	cost[startID] = 0
	foundExact := false
	for visits := 0; visits < total; visits++ {
		if queries >= searchLimit {
			break
		}
		current, best := -1, 1<<30
		for id := 0; id < total; id++ {
			if done[id] || cost[id] == 1<<29 {
				continue
			}
			ddx, ddz := navigationAbs(id%side-gx), navigationAbs(id/side-gz)
			estimate := cost[id] + 10*max(ddx, ddz) + 4*min(ddx, ddz)
			if estimate < best {
				current, best = id, estimate
			}
		}
		if current < 0 {
			break
		}
		done[current] = true
		if exactGoal && navigationDistance(point(current), goal) <= 4096*4096 && segment(point(current), goal) {
			targetID = current
			foundExact = true
			break
		}
		if !exactGoal && current == targetID {
			break
		}
		for _, d := range [][2]int{{1, 0}, {0, 1}, {-1, 0}, {0, -1}, {1, 1}, {-1, 1}, {-1, -1}, {1, -1}} {
			nx, nz := current%side+d[0], current/side+d[1]
			if nx < 0 || nz < 0 || nx >= side || nz >= side {
				continue
			}
			next := nx + nz*side
			if done[next] || !free(next) {
				continue
			}
			if d[0] != 0 && d[1] != 0 && (!free(nx+(current/side)*side) || !free(current%side+nz*side)) {
				continue
			}
			add := 10
			if d[0] != 0 && d[1] != 0 {
				add = 14
			}
			if cost[current]+add < cost[next] && segment(point(current), point(next)) {
				cost[next] = cost[current] + add
				parent[next] = current
				parentProved[next] = true
			}
		}
	}
	if parent[targetID] < 0 && !foundExact {
		best := navigationDistance(start, goal)
		chosen := -1
		for id := 0; id < total; id++ {
			eligible := done[id] && navigationDistance(start, point(id)) >= 4096*4096
			if s.corridorNavigation {
				eligible = parentProved[id]
			}
			if !eligible {
				continue
			}
			d := navigationDistance(point(id), goal)
			if d < best {
				best = d
				chosen = id
			}
		}
		if chosen < 0 {
			return nil, stats
		}
		targetID = chosen
	}
	reverse := []NavigationPoint{}
	for id := targetID; id != startID; id = parent[id] {
		if id < 0 || !parentProved[id] {
			return nil, stats
		}
		reverse = append(reverse, point(id))
		if len(reverse) > 64 {
			return nil, stats
		}
	}
	path := []NavigationPoint{start}
	for i := len(reverse) - 1; i >= 0; i-- {
		path = append(path, reverse[i])
	}
	if foundExact {
		path = append(path, goal)
	}
	result := []NavigationPoint{}
	at := 0
	for at < len(path)-1 {
		next := -1
		for end := len(path) - 1; end > at; end-- {
			if segment(path[at], path[end]) {
				next = end
				break
			}
		}
		if next < 0 {
			next = at + 1
		}
		result = append(result, path[next])
		if len(result) == 8 {
			break
		}
		at = next
	}
	return result, stats
}
