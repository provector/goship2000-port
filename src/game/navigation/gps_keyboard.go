package navigation

// KeyboardInput applies the named native overview-speed policy after the
// source's signed SAR4 pixel conversion. Fractional pixels are retained so
// short/reversed presses do not acquire a new rounding bias. Detail panning
// keeps its source word-shifted world-coordinate producer unchanged.
func (g *GPS) KeyboardInput(x, y int16, divisor int) Input {
	if g.Low || divisor <= 1 {
		g.overviewKeyRemainder = [2]int{}
		return Input{AxisX: x, AxisY: y}
	}
	axis := func(n int, v int16) int16 {
		g.overviewKeyRemainder[n] += int(v >> 4)
		pixels := g.overviewKeyRemainder[n] / divisor
		g.overviewKeyRemainder[n] %= divisor
		return int16(pixels << 4) // Handle consumes the already-quantized SAR4 delta.
	}
	return Input{AxisX: axis(0, x), AxisY: axis(1, y)}
}
