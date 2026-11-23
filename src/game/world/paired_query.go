package world

import "fmt"

// PairedTerrainHeight is resident037f:3289. The first pose chooses the cell,
// containment point and plane evaluation. Only the second height gates regions;
// second-endpoint inclusion does not make this a generic segment intersection.
// No region returns -1, unlike RegionHeight's zero for ordinary contact queries.
func (b *Bundle) PairedTerrainHeight(worldID string, first, second QueryPose) (int16, error) {
	g, x, z, err := b.queryCell(worldID, first)
	if err != nil {
		return 0, err
	}
	for _, r := range g.Heights {
		if r.Threshold != 0 && r.Threshold > second.Height {
			continue
		}
		inside, err := SourceContains(r.Points, x, z)
		if err != nil {
			return 0, err
		}
		if !inside {
			continue
		}
		n := int32(int64(r.Plane[0])*int64(x) + int64(r.Plane[2])*int64(z) + int64(r.Plane[3]))
		if r.Plane[1] == 0 {
			return 0, fmt.Errorf("paired height plane divisor zero")
		}
		q := int64(n) / int64(r.Plane[1])
		if q < -32768 || q > 32767 {
			return 0, fmt.Errorf("paired height plane division overflow")
		}
		return -int16(q), nil
	}
	return -1, nil
}
