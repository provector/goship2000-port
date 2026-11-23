package avionics

// TargetMetric is resident00db:016f..0194's target-display range producer.
// Its max comparison is unsigned JBE, unlike the signed metric used by other
// contact consumers. In particular word ABS(8000h) remains unsigned32768.
func TargetMetric(dx, dz int16) uint16 {
	x, z := AbsWord(dx), AbsWord(dz)
	if z > x {
		return z + (x >> 1)
	}
	return x + (z >> 1)
}
