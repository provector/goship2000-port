// Package configuration owns shared game/SETUP defaults, typed validation and
// configuration documents. It has no terminal or rendering dependency.
package configuration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"gunship2000-assets/internal/rasterlayout"
)

// PreviewVisualRate is diagnostic presentation policy, not simulation Hz.
const PreviewVisualRate = 12.0

type AvoidancePolicy string

const (
	AvoidanceLegacy    AvoidancePolicy = "legacy"
	AvoidanceCorrected AvoidancePolicy = "corrected"
)

func (p AvoidancePolicy) Valid() bool { return p == AvoidanceLegacy || p == AvoidanceCorrected }

type RuntimeOptions struct {
	AvoidancePolicy            AvoidancePolicy `json:"avoidance_policy"`
	DisableEnemyAI             bool            `json:"disable_enemy_ai"`
	DisableWingmenAI           bool            `json:"disable_wingmen_ai"`
	SwapYawRollAxis            bool            `json:"swap_yaw_roll_axis"`
	FloatingGeometry           bool            `json:"floating_point_rendering,omitempty"`
	SimulationResolution       string          `json:"simulation_resolution,omitempty"`
	SimulationRenderScale      int             `json:"simulation_render_scale"`
	PresentationMode           string          `json:"presentation_mode"`
	PresentationFPS            int             `json:"presentation_fps"`
	MouseCapture               string          `json:"mouse_capture"`
	GPSOverviewKeyboardDivisor int             `json:"gps_overview_keyboard_divisor"`
	CollectiveRepeatDelayMS    int             `json:"collective_repeat_delay_ms"`
	CollectiveRepeatIntervalMS int             `json:"collective_repeat_interval_ms"`
	CommandQueueCapacity       int             `json:"command_queue_capacity"`
}

func Defaults() RuntimeOptions {
	return RuntimeOptions{AvoidancePolicy: AvoidanceCorrected, SwapYawRollAxis: false, SimulationRenderScale: 1, PresentationMode: "smooth", PresentationFPS: 60, MouseCapture: "click", GPSOverviewKeyboardDivisor: 4, CollectiveRepeatDelayMS: 500, CollectiveRepeatIntervalMS: 100, CommandQueueCapacity: 64}
}

// Normal presentation is capped at60. Higher diagnostic schedules are exercised
// by offline tests; they do not change the validated authoritative flight clock.
func (o RuntimeOptions) Validate() error {
	if !o.AvoidancePolicy.Valid() {
		return fmt.Errorf("avoidance_policy must be legacy or corrected")
	}
	if o.SimulationResolution != "" && rasterlayout.ByName(o.SimulationResolution) == nil {
		return fmt.Errorf("unknown simulation resolution %q", o.SimulationResolution)
	}
	if o.SimulationRenderScale != 1 && o.SimulationRenderScale != 2 {
		return fmt.Errorf("simulation_render_scale must be1 or2")
	}
	if o.PresentationMode != "smooth" && o.PresentationMode != "stepped" {
		return fmt.Errorf("presentation_mode must be smooth or stepped")
	}
	if o.PresentationFPS < 10 || o.PresentationFPS > 60 {
		return fmt.Errorf("presentation_fps must be an integer in 10..60")
	}
	if o.MouseCapture != "click" && o.MouseCapture != "disabled" {
		return fmt.Errorf("mouse_capture must be click or disabled")
	}
	if o.GPSOverviewKeyboardDivisor < 1 || o.GPSOverviewKeyboardDivisor > 16 {
		return fmt.Errorf("gps_overview_keyboard_divisor must be an integer in1..16")
	}
	if o.CollectiveRepeatDelayMS < 0 || o.CollectiveRepeatDelayMS > 2000 {
		return fmt.Errorf("collective_repeat_delay_ms must be0..2000")
	}
	if o.CollectiveRepeatIntervalMS < 99 || o.CollectiveRepeatIntervalMS > 2000 {
		return fmt.Errorf("collective_repeat_interval_ms must be99..2000 (one command per logical flight tick)")
	}
	if o.CommandQueueCapacity < 1 || o.CommandQueueCapacity > 256 {
		return fmt.Errorf("command_queue_capacity must be1..256")
	}
	return nil
}

// Effective supplies defaults for programmatic callers predating these options.
// JSON loading instead decodes over Defaults, preserving valid explicit zero.
func (o RuntimeOptions) Effective() RuntimeOptions {
	if o == (RuntimeOptions{}) {
		return Defaults()
	}
	if o.AvoidancePolicy == "" {
		o.AvoidancePolicy = Defaults().AvoidancePolicy
	}
	return o
}

// UnmarshalJSON tolerates only the retired terrain-pattern key. It has no stored
// value or rendering effect; all other unknown runtime options remain errors.
func (o *RuntimeOptions) UnmarshalJSON(data []byte) error {
	type plain RuntimeOptions
	input := struct {
		*plain
		Retired json.RawMessage `json:"terrain_pattern_mode"`
	}{plain: (*plain)(o)}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return decoder.Decode(&input)
}
