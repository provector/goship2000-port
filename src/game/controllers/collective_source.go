package controllers

import "strings"

// CollectiveSource is stable profile evidence, not a live centering heuristic.
// Raw SDL has no mechanical spring-centering property. The narrow hardware/axis
// witnesses below are Q3.1 actual raw travel + operator release observations:
// evidence/stepQ31/hardware01.json and working/stepQ31/hardware01/raw.jsonl.
// They identify a physical axis, not a model-wide default for every axis.
// Unknown axes use a declared conservative centered fallback; SETUP's per-device
// override provides explicit correction without changing the pilot's mode.
func CollectiveSource(p Profile) (source, reason string) {
	if p.CollectiveSource == "centered" || p.CollectiveSource == "slider" {
		return p.CollectiveSource, "selected-profile override"
	}
	b := p.Bindings["flight.collective"]
	if b.Kind == "axis" && strings.HasSuffix(p.Device.Backend, "/linux") {
		d := p.Device
		if d.Vendor == 0x044f && d.Product == 0xb68d && d.Revision == 0x0111 && d.Axes == 8 && b.Axis == 2 {
			return "slider", "Q3.1 observed 044f:b68d revision0111 raw axis3 lever retained at midpoint"
		}
		if d.Vendor == 0x20d6 && d.Product == 0x57c7 && d.Revision == 0x0110 && d.Axes == 4 && (b.Axis == 0 || b.Axis == 1 || b.Axis == 2 || b.Axis == 3) {
			return "centered", "Q3.1 observed 20d6:57c7 revision0110 stick release; mapped stick axis"
		}
		// Quickshot endpoint X/Y is a spring stick, not an SDL hat or throttle.
		if d.Vendor == 0x14d8 && d.Product == 0xcd07 && d.Revision == 0x0111 && d.Axes == 3 && (b.Axis == 0 || b.Axis == 1) {
			return "centered", "Q2/Q3.1 observed Quickshot signed endpoint X/Y stick"
		}
	}
	return "centered", "unknown mechanics: stable centered fallback; use Collective source override for a lever"
}
func CollectiveMode(p Profile) string {
	if p.CollectiveMode != "" {
		return p.CollectiveMode
	}
	source, _ := CollectiveSource(p)
	if source == "slider" {
		return "absolute"
	}
	return "relative_height"
}
