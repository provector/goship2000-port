package menus

import (
	"encoding/json"
	assetdata "gunship2000-assets/game/assets"
	. "gunship2000-assets/game/state"
	"gunship2000-assets/internal/testfixture"
	"os"
	"reflect"
	"strconv"
	"testing"
)

func checkMissionCanonicalFields(t *testing.T, d *MissionData, raw []byte, training bool) {
	t.Helper()
	var ref map[string]json.RawMessage
	if err := json.Unmarshal(raw, &ref); err != nil {
		t.Fatal(err)
	}
	var cond map[string]interface{}
	json.Unmarshal(ref["conditions"], &cond)
	number := func(k string) int { v, _ := cond[k].(float64); return int(v) }
	c := d.Conditions
	if int(c.WindDirection) != number("wind_word_a250") || c.WindSpeed != number("wind_speed_word_a252") || int(c.Temperature) != number("temperature_signed") || c.Callsign != number("callsign_byte_a26b") || c.SecondCallsign != number("callsign_byte_a26c") || c.Hour != number("hour") || c.Operation != cond["operation_identifier"] {
		t.Fatal("conditions differ from independent reference")
	}
	flags := number("condition_word_mask_0x300")
	if !training {
		var final map[string]int
		json.Unmarshal(ref["final_switch_flags"], &final)
		flags = final["a248_u16"]
	}
	if int(c.Flags) != flags {
		t.Fatal("final switchflags differ")
	}
	var intel struct {
		GroundIDs     []int    `json:"ground_ids"`
		AirCount      int      `json:"air_count"`
		AirID         *int     `json:"air_id"`
		GroundNames   []string `json:"ground_names"`
		AirName       *string  `json:"air_name"`
		AirWord       *string  `json:"air_word"`
		FriendlyTitle *string  `json:"friendly_title"`
	}
	json.Unmarshal(ref["intelligence"], &intel)
	if !reflect.DeepEqual(d.Intelligence.GroundIDs, intel.GroundIDs) || !reflect.DeepEqual(d.Intelligence.GroundNames, intel.GroundNames) || d.Intelligence.AirCount != intel.AirCount {
		t.Fatalf("Intel differs: %+v / %+v", d.Intelligence, intel)
	}
	if intel.AirID != nil && (d.Intelligence.AirID != *intel.AirID || d.Intelligence.AirName != *intel.AirName || d.Intelligence.AirDensity != *intel.AirWord) {
		t.Fatal("air Intel differs")
	}
	if intel.FriendlyTitle != nil && d.Intelligence.FriendlyTitle != *intel.FriendlyTitle {
		t.Fatal("optional friendly clause differs")
	}
	var objectives map[string]map[string]json.RawMessage
	json.Unmarshal(ref["objectives"], &objectives)
	for role, obj := range objectives {
		name := role
		if !training {
			i, _ := strconv.Atoi(role)
			name = map[int]string{0: "base", 1: "FARP", 2: "primary", 3: "secondary", 4: "tertiary"}[i]
		}
		got, ok := d.Objectives[name]
		if !ok {
			t.Fatal("missing objective", name)
		}
		field := "point"
		if training {
			field = "position"
		}
		var point [2]int32
		json.Unmarshal(obj[field], &point)
		if got.Point != (MissionPoint{point[0], point[1]}) {
			t.Fatal("objective point differs", name, got.Point, point)
		}
		if !training {
			var count, kind int
			json.Unmarshal(obj["display_count"], &count)
			json.Unmarshal(obj["raw_kind"], &kind)
			if got.Count != count || got.RawKind != kind {
				t.Fatal("objective count/kind differs", name)
			}
			var ep *[2]int32
			json.Unmarshal(obj["endpoint"], &ep)
			if ep != nil && (got.Endpoint == nil || *got.Endpoint != (MissionPoint{ep[0], ep[1]})) {
				t.Fatal("objective route endpoint differs", name)
			}
		}
	}
	var recorded []struct{ Value, Bound int }
	key := "draws"
	if training {
		key = "generation_draws"
	}
	json.Unmarshal(ref[key], &recorded)
	for i, draw := range d.Draws {
		if i >= len(recorded) || draw.Value != recorded[i].Value || draw.Bound != recorded[i].Bound {
			t.Fatal("bounded draw sequence differs", i)
		}
	}
	var scene int
	json.Unmarshal(ref["scene"], &scene)
	if d.Scene != scene {
		t.Fatal("scene differs")
	}
}

func missionTestData(t *testing.T) *assetdata.MissionGenerationData {
	t.Helper()
	b, e := os.ReadFile(testfixture.Path("generation/source-data.json"))
	if os.IsNotExist(e) {
		t.Skip("source development fixture absent")
	}
	if e != nil {
		t.Fatal(e)
	}
	var d assetdata.MissionGenerationData
	if e = json.Unmarshal(b, &d); e != nil {
		t.Fatal(e)
	}
	return &d
}

func missionReadinessAssets(t *testing.T) *assetdata.MissionAssets {
	t.Helper()
	ready := os.Getenv("GS2000_TEST_MISSIONS")
	if ready == "" {
		ready = testfixture.Path("prepared-preflight")
	}
	if _, err := os.Stat(ready); err == nil {
		a, err := assetdata.LoadMissionAssets(ready)
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	path := testfixture.Path("prepared-missions")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skip("source-backed prepared test witness absent")
	}
	raw, err := os.ReadFile(path + "/GS2000.DAT/FIELD.SCN.missions.json")
	if err != nil {
		t.Fatal(err)
	}
	var metadata assetdata.MissionMetadata
	if err = json.Unmarshal(raw, &metadata); err != nil {
		t.Fatal(err)
	}
	a := &assetdata.MissionAssets{Generation: &metadata.Generation, AircraftDefinitions: metadata.AircraftDefinitions, WeaponDefinitions: metadata.WeaponDefinitions}
	return a
}
