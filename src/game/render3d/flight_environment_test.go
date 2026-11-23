package render3d

import "testing"

func TestFlightWeatherAndLateCompassOrdering(t *testing.T) {
	e := &EnvironmentState{Prefix: "ANTC", RNG: 16}
	e.StepFlight(0, false, 256)
	if e.RNG != 16 || e.CompassDraw != 0 {
		t.Fatal("ungated DA value ran weather")
	}
	e.StepFlight(0, true, 256)
	// First source LFSR draw16→8 passes&7==0. Physics must consume8,
	// before the later HUD update changes it to8+(8SAR1)=12.
	if e.CompassDraw != 8 || e.CompassShown != 0 {
		t.Fatalf("prephysics state%+v", e)
	}
	e.FinishFlightFrame()
	if e.CompassShown != 8 || e.CompassDraw != 12 {
		t.Fatalf("later HUD writer%+v", e)
	}
}
