package cockpit

import "fmt"

// DrawSupportStrike is source015b:0ca7..0d05, after routes and before ownship.
// AA2 controls presence; AA5/AA9 are an independent confirmed strike point,
// unrelated to an aircraft/contact symbol or the tentative GPS edit cursor.
func (a *Assets) DrawSupportStrike(dst []byte, point *MapPoint, project func(uint32, uint32) (int, int), shift uint, clip Rect) error {
	if point == nil {
		return nil
	}
	vectors, ok := a.MapVectors["map-0805"]
	if !ok {
		return fmt.Errorf("prepared Support strike vector0805 is missing")
	}
	x, y := project(point.H0, point.H1)
	// Source adds0200 to the supplied style: black (+1,+1) pass then
	// the original literal12 colour. Preserve caller scale and local clipping.
	for _, v := range vectors {
		v.Colour = 0
		a.DrawVectors(dst, []Vector{v}, x+1, y+1, 0, shift, 0, clip)
	}
	a.DrawVectors(dst, vectors, x, y, 0, shift, 0, clip)
	return nil
}
