package menus

import (
	gamestate "gunship2000-assets/game/state"
	"math"
)

// P0f10..0fbe marks the special base corridor. The orientation test uses
// X/32768, independently of Y and of the rounded coordinate printed in Orders.
func briefingMapEdgeSegment(p gamestate.MissionPoint, flags uint16) ([4]int, bool) {
	if flags&0x800 == 0 {
		return [4]int{}, false
	}
	x, y := gamestate.MissionMapPoint(p, 0, 0)
	x = gamestate.MissionI16(x + 1)
	cellX := gamestate.MissionI16(int(p.X / 32768))
	if cellX < 2 || cellX > 14 {
		return [4]int{x, gamestate.MissionI16(y - 16), x, gamestate.MissionI16(y + 16)}, true
	}
	return [4]int{gamestate.MissionI16(x - 20), y, gamestate.MissionI16(x + 20), y}, true
}

// P112e draws a short direction arrow, not a line all the way to the route
// endpoint. Its cheap distance and shifted numerators are signed16 operations.
func briefingMapArrowSegments(x, y, ex, ey int) [][4]int {
	dx, dy := gamestate.MissionI16(ex-x), gamestate.MissionI16(ey-y)
	absWord := func(n int) int {
		if n < 0 {
			return gamestate.MissionI16(-n)
		}
		return n
	}
	ax, ay := absWord(dx), absWord(dy)
	distance := gamestate.MissionI16(max(ax, ay) + min(ax, ay)/2)
	// Defensive port policy: degenerate/overflow vectors must not divide by
	// zero or draw arbitrary memory. Native exceptional behavior is unverified.
	if distance <= 0 || ax < 0 || ay < 0 {
		return nil
	}
	sx := gamestate.MissionI16(x + gamestate.MissionI16(dx<<2)/distance)
	sy := gamestate.MissionI16(y + gamestate.MissionI16(dy<<2)/distance)
	tx := gamestate.MissionI16(x + gamestate.MissionI16(dx<<4)/distance)
	ty := gamestate.MissionI16(y + gamestate.MissionI16(dy<<4)/distance)
	angle := math.Atan2(float64(gamestate.MissionI16(sy-ty)), float64(gamestate.MissionI16(sx-tx)))
	lines := [][4]int{{sx, sy, tx, ty}}
	for _, offset := range []float64{-0.5, 0.5} {
		// P1234..1299 adds the tip before truncating the floating result;
		// truncating the negative arm first moves some endpoints by one pixel.
		hx := gamestate.MissionI16(int(float64(tx) + 6*math.Cos(angle+offset)))
		hy := gamestate.MissionI16(int(float64(ty) + 6*math.Sin(angle+offset)))
		lines = append(lines, [4]int{hx, hy, tx, ty})
	}
	return lines
}
