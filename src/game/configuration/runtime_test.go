package configuration

import (
	"encoding/json"
	"testing"
)

func TestRuntimeDefaultsAndValidation(t *testing.T) {
	d := Defaults()
	if err := d.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(`{"presentation_mode":"stepped","collective_repeat_delay_ms":0,"mouse_capture":"disabled"}`), &d); err != nil {
		t.Fatal(err)
	}
	if d.CollectiveRepeatDelayMS != 0 || d.PresentationFPS != 60 {
		t.Fatal("lost explicit zero or omitted default")
	}
	if err := d.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`{"presentation_fps":0}`, `{"presentation_fps":144}`, `{"presentation_mode":"extrapolate"}`, `{"mouse_capture":"warp"}`, `{"collective_repeat_interval_ms":0}`, `{"command_queue_capacity":0}`} {
		v := Defaults()
		if err := json.Unmarshal([]byte(field), &v); err != nil {
			t.Fatal(err)
		}
		if v.Validate() == nil {
			t.Fatalf("accepted%s", field)
		}
	}
}

func TestRenderingOptions(t *testing.T) {
	for _, fragment := range []string{`{}`, `{"simulation_render_scale":2}`} {
		o := Defaults()
		if e := json.Unmarshal([]byte(fragment), &o); e != nil {
			t.Fatal(e)
		}
		if e := o.Validate(); e != nil {
			t.Fatal(e)
		}
	}
	for _, fragment := range []string{`{"simulation_render_scale":0}`, `{"simulation_render_scale":3}`} {
		o := Defaults()
		if e := json.Unmarshal([]byte(fragment), &o); e != nil {
			t.Fatal(e)
		}
		if o.Validate() == nil {
			t.Fatal("invalid renderer option", fragment)
		}
	}
}

func TestRetiredTerrainPatternIsIgnoredWithoutWeakeningUnknownKeys(t *testing.T) {
	for _, value := range []string{`"world"`, `"source"`, `"noise"`, `null`} {
		o := Defaults()
		if err := json.Unmarshal([]byte(`{"terrain_pattern_mode":`+value+`,"simulation_render_scale":2}`), &o); err != nil {
			t.Fatal(err)
		}
		if err := o.Validate(); err != nil || o.SimulationRenderScale != 2 {
			t.Fatal(o, err)
		}
		got, _ := json.Marshal(o)
		var fields map[string]json.RawMessage
		json.Unmarshal(got, &fields)
		if _, exists := fields["terrain_pattern_mode"]; exists {
			t.Fatal("retired option retained in configuration model")
		}
	}
	o := Defaults()
	if err := json.Unmarshal([]byte(`{"terrain_pattern_typo":"world"}`), &o); err == nil {
		t.Fatal("unknown-key validation was weakened")
	}
}

func TestGPSOverviewKeyboardConfiguration(t *testing.T) {
	for _, v := range []int{1, 4, 16} {
		o := Defaults()
		o.GPSOverviewKeyboardDivisor = v
		if e := o.Validate(); e != nil {
			t.Fatal(e)
		}
	}
	for _, fragment := range []string{`{"gps_overview_keyboard_divisor":0}`, `{"gps_overview_keyboard_divisor":17}`, `{"gps_overview_keyboard_divisor":-1}`} {
		o := Defaults()
		if e := json.Unmarshal([]byte(fragment), &o); e != nil {
			t.Fatal(e)
		}
		if o.Validate() == nil {
			t.Fatal("invalid GPS divisor admitted", fragment)
		}
	}
	o := Defaults()
	if e := json.Unmarshal([]byte(`{}`), &o); e != nil || o.GPSOverviewKeyboardDivisor != 4 {
		t.Fatal("missing GPS divisor did not retain default", o, e)
	}
	if e := json.Unmarshal([]byte(`{"gps_overview_keyboard_divisor":1.5}`), &o); e == nil {
		t.Fatal("noninteger GPS divisor accepted")
	}
}
