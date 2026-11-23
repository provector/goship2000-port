package world

// SourceAngle is resident037f:2814. The sign words contain both a base angle
// and a low-byte subtract marker. Equal and zero vectors use the last table
// entry; ideal atan2 would produce different native results at these boundaries.
// Callers validate the prepared eight bases and4096 lookup words at load time.
func SourceAngle(y, x int16, a *Bundle) uint16 {
	flags := 0
	xx, yy := uint16(x), uint16(y)
	if x < 0 {
		flags += 4
		xx = uint16(-x)
	}
	if y < 0 {
		flags += 2
		yy = uint16(-y)
	}
	if xx < yy {
		flags++
		xx, yy = yy, xx
	}
	index := 4095
	if yy < xx && xx != 0 {
		index = int((uint32(yy) << 16) / uint32(xx) >> 4)
	}
	base := a.AngleBases[flags]
	value := a.Atan[index]
	if byte(base)&0x80 != 0 {
		return (base & 0xff00) - value
	}
	return (base & 0xff00) + value
}

// SourceSinAmplitude/SourceCosAmplitude are the signed Q14 multiply wrappers
// at037f:278e/2776. Angle lookup discards four low angle bits before indexing.
func SourceSinAmplitude(angle uint16, n int16, quarter []int16) int16 {
	phase := int(angle >> 4)
	i := phase & 1023
	if phase&1024 != 0 {
		i = 1024 - i
	}
	value := quarter[i]
	if phase&2048 != 0 {
		value = -value
	}
	return int16((int32(value) * int32(n)) >> 14)
}
func SourceCosAmplitude(angle uint16, n int16, quarter []int16) int16 {
	return SourceSinAmplitude(angle+0x4000, n, quarter)
}
