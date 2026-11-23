package render3d

// composePadStyleFootprint retains the accepted display recipe's clipped
// admission. It is independent of actual indexed writes and typed source owner
// coverage, which are produced together by fillPolygonMaterialOwned.
func composePadStyleFootprint(mask []byte, points []Point, bounds Rect, clip edgeClipper) error {
	if clip == nil && targetWidth(mask) != Width {
		clip = func(_ int, a, z Point, b Rect) (Point, Point, bool, int, error) { return clipLineTarget(a, z, b, true) }
	}
	var s spans
	if err := s.polygon(points, bounds, clip); err != nil {
		return err
	}
	owner := spanOwnership{mask: mask, material: materialBinding{role: GroundContactPad}}
	for y := bounds.Top; y <= bounds.Bottom; y++ {
		l, r := s.left[y], s.right[y]
		if l > r || l >= bounds.Right || r <= bounds.Left {
			continue
		}
		l = max(l, bounds.Left)
		r = min(r, bounds.Right)
		owner.span(mask, y, l, r, 0xec00)
	}
	return nil
}
