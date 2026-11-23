package state

import (
	"encoding/json"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/internal/testfixture"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
)

func TestTrainingCanonical(t *testing.T) {
	raw, err := os.ReadFile(testfixture.Path("generation/source-data.json"))
	if os.IsNotExist(err) {
		t.Skip("source-derived development fixture absent")
	}
	if err != nil {
		t.Fatal(err)
	}
	var data assetdata.MissionGenerationData
	if err = json.Unmarshal(raw, &data); err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob(testfixture.Path("integrated-training/theatre*.json"))
	if err != nil || len(files) != 6 {
		t.Fatal("missing six canonical reference cases")
	}
	for _, path := range files {
		t.Run(filepath.Base(path), func(t *testing.T) {
			b, _ := os.ReadFile(path)
			var ref struct {
				Inputs     struct{ Theatre, Rank int }
				Population struct {
					ObjectsSHA256 string `json:"objects_sha256"`
				}
				Draws []struct{ Value, Bound int } `json:"generation_draws"`
			}
			if err := json.Unmarshal(b, &ref); err != nil {
				t.Fatal(err)
			}
			stream := []int{}
			for _, d := range ref.Draws {
				stream = append(stream, d.Value)
			}
			result, err := GenerateMission(&data, MissionInputs{MissionType: MissionTraining, Theatre: saveTheatres[ref.Inputs.Theatre], Rank: saveRanks[ref.Inputs.Rank], PilotName: "Test Pilot Name", DrawStream: stream})
			if err != nil {
				t.Fatal(err)
			}
			checkMissionCanonicalFields(t, result, b, true)
			t.Logf("objects=%d hash=%s draws=%d complete fields matched", len(result.Objects), result.ObjectHash, len(result.Draws))
			if result.ObjectHash != ref.Population.ObjectsSHA256 {
				t.Fatalf("complete object hash got %s want %s (%d draws)", result.ObjectHash, ref.Population.ObjectsSHA256, len(result.Draws))
			}
			if len(result.Draws) != len(stream) {
				t.Fatal("draw stream not consumed exactly")
			}
		})
	}
}

func TestMissionDrawBoundsAndTransforms(t *testing.T) {
	r := newMissionRNG(MissionInputs{DrawStream: []int{2}})
	if r.r0(1, "none") != 0 || r.cursor != 0 {
		t.Fatal("bound1 consumed RNG")
	}
	r.r0(2, "invalid")
	if r.err == nil {
		t.Fatal("accepted out-of-range draw")
	}
	x, y := MissionMapPoint(MissionPoint{356352, 446464}, 0, 0)
	if x != 127 || y != 36 || MissionGrid(356352) != 11 {
		t.Fatal(x, y)
	}
	if MissionGrid(-32768) != 0 {
		t.Fatal("signed division must truncate toward zero")
	}
	_, err := GenerateMission(&assetdata.MissionGenerationData{Version: 1}, MissionInputs{MissionType: MissionSimulated, Theatre: TheatrePersianGulf, Rank: RankWOC})
	if err == nil {
		t.Fatal("unsupported branch substituted")
	}
}

func TestGenericMissionCanonical(t *testing.T) {
	raw, err := os.ReadFile(testfixture.Path("generation/source-data.json"))
	if os.IsNotExist(err) {
		t.Skip("source development fixture absent")
	}
	if err != nil {
		t.Fatal(err)
	}
	var data assetdata.MissionGenerationData
	if err = json.Unmarshal(raw, &data); err != nil {
		t.Fatal(err)
	}
	files, _ := filepath.Glob(testfixture.Path("generic-missions/m*.json"))
	if len(files) != 36 {
		t.Fatalf("expected36cases got%d", len(files))
	}
	for _, path := range files {
		t.Run(filepath.Base(path), func(t *testing.T) {
			raw, _ := os.ReadFile(path)
			var ref struct {
				Inputs struct {
					Theatre, Rank int
					MissionType   int `json:"mission_type"`
					Previous      int `json:"previous_scenario"`
					Campaign      map[string]interface{}
				}
				Population struct {
					Hash string `json:"objects_sha256"`
				}
				Draws []struct{ Value, Bound int }
			}
			if err = json.Unmarshal(raw, &ref); err != nil {
				t.Fatal(err)
			}
			stream := []int{}
			for _, d := range ref.Draws {
				stream = append(stream, d.Value)
			}
			in := MissionInputs{MissionType: saveMissions[ref.Inputs.MissionType], Theatre: saveTheatres[ref.Inputs.Theatre], Rank: saveRanks[ref.Inputs.Rank], PreviousScenario: ref.Inputs.Previous, PilotName: "Test Pilot Name", DrawStream: stream}
			if in.MissionType == MissionCampaign {
				m := ref.Inputs.Campaign
				n := func(k string) int { v, _ := m[k].(float64); return int(v) }
				in.Campaign = &CampaignGenerationInputs{Stage: n("stage"), Raw50: n("raw50"), Raw51: n("raw51"), Raw52: n("raw52"), LossWord: n("loss_word"), ScenarioCacheAbsent: true}
			}
			result, err := GenerateMission(&data, in)
			if err != nil {
				t.Fatal(err)
			}
			checkMissionCanonicalFields(t, result, raw, false)
			t.Logf("objects=%d hash=%s draws=%d complete fields matched", len(result.Objects), result.ObjectHash, len(result.Draws))
			want := correctedReferenceHash(t, raw)
			if result.ObjectHash != want {
				t.Fatalf("objecthash got%s want%s draws%d/%d", result.ObjectHash, want, len(result.Draws), len(stream))
			}
			if len(result.Draws) != len(stream) {
				t.Fatalf("draws consumed%d want%d", len(result.Draws), len(stream))
			}
		})
	}
}

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
			name = missionRole(i)
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

func TestMissionGenerationDeterminismAndIsolation(t *testing.T) {
	data := missionTestData(t)
	for _, mission := range []MissionType{MissionTraining, MissionSingle, MissionFlight} {
		for ti := 0; ti < 4; ti++ {
			if mission == MissionTraining && ti > 1 {
				continue
			}
			for _, rank := range []Rank{RankWOC, RankCaptain} {
				in := MissionInputs{MissionType: mission, Theatre: saveTheatres[ti], Rank: rank, PilotName: "Alpha Pilot", Seed: 92}
				first, err := GenerateMission(data, in)
				if err != nil {
					t.Fatal(mission, ti, rank, err)
				}
				second, err := GenerateMission(data, in)
				if err != nil || first.ObjectHash != second.ObjectHash || !reflect.DeepEqual(first.Draws, second.Draws) {
					t.Fatal("mission RNG/session not independent")
				}
				if len(first.Objects) < 80 || len(first.Objects) > 81 {
					t.Fatal("active count confused with capacity")
				}
				if len(first.Intelligence.GroundIDs) > 4 {
					t.Fatal("invalid Intel frequency selection")
				}
			}
		}
	}
}

func TestMissionMalformedTablesAndStream(t *testing.T) {
	data := missionTestData(t)
	in := MissionInputs{MissionType: MissionTraining, Theatre: TheatrePersianGulf, Rank: RankWOC, DrawStream: []int{}}
	if _, err := GenerateMission(data, in); err == nil {
		t.Fatal("exhausted stream accepted")
	}
	in.DrawStream = nil
	copyData := *data
	copyData.Tables = data.Tables
	copyData.Tables.Primary = nil
	if _, err := GenerateMission(&copyData, in); err == nil {
		t.Fatal("truncated objective table accepted")
	}
	in.MissionType = MissionCampaign
	if _, err := GenerateMission(data, in); err == nil {
		t.Fatal("Campaign fabricated missing retained inputs")
	}
	in.MissionType = MissionTraining
	in.Theatre = TheatreAntarctica
	if _, err := GenerateMission(data, in); err == nil {
		t.Fatal("ineligible Training theatre accepted")
	}
	in.Theatre = TheatrePersianGulf
	in.RetainedObject = make([]byte, 31)
	if _, err := GenerateMission(data, in); err == nil {
		t.Fatal("partial retained object accepted")
	}
}

func TestMissionIntelAliasingAndTies(t *testing.T) {
	row := func(id, class byte) MissionObject {
		o := MissionObject{}
		o.Raw[11] = 1
		o.Raw[12] = id
		o.Raw[10] = class
		return o
	}
	intel := missionIntel([]MissionObject{row(34, 4), row(42, 4), row(7, 3), row(4, 1), row(5, 1)}, nil)
	if !reflect.DeepEqual(intel.GroundIDs, []int{34, 7, 7}) || intel.AirID != 4 || intel.AirCount != 2 {
		t.Fatal("alias-before-frequency selection and strict tie rules changed", intel)
	}
	p := &missionPopulation{objects: []MissionObject{row(230, 4)}}
	if p.validate() == nil {
		t.Fatal("histogram memory escape silently accepted")
	}
}

func TestMissionGreedyRouteAndCapacity(t *testing.T) {
	p := &missionPopulation{grid: make([]byte, 4096), source: &assetdata.MissionWorld{LAND: make([]byte, 64)}}
	p.source.LAND[0] = 8
	g := &genericMissionPopulation{missionPopulation: p}
	rows, ok := g.planner([2]int{10, 10}, [2]int{12, 12}, 0)
	if !ok || len(rows) != 2 || rows[0].Flag != 0x11 || rows[1].Flag != 0x40 || rows[1].Point != (MissionPoint{12*8192 + 4096, 12*8192 + 4096}) {
		t.Fatal("route control semantics changed")
	}
	p.source.LAND[0] = 0
	if _, ok = g.planner([2]int{10, 10}, [2]int{12, 12}, 0); ok {
		t.Fatal("blocked monotone route accepted")
	}
}

func TestPreparedGenerationMatchesIndependentSource(t *testing.T) {
	if _, err := os.Stat(testfixture.Path("prepared-missions")); os.IsNotExist(err) {
		t.Skip("prepared local asset witness absent")
	}
	raw, err := os.ReadFile(testfixture.Path("prepared-missions/GS2000.DAT/FIELD.SCN.missions.json"))
	if err != nil {
		t.Fatal(err)
	}
	var metadata assetdata.MissionMetadata
	if err = json.Unmarshal(raw, &metadata); err != nil {
		t.Fatal(err)
	}
	a := &assetdata.MissionAssets{Generation: &metadata.Generation}
	independent := missionTestData(t)
	if !reflect.DeepEqual(a.Generation.Tables, independent.Tables) {
		t.Fatal("production prepared tables differ from independent source extraction")
	}
	if len(a.Generation.Worlds) != len(independent.Worlds) {
		t.Fatal("world dependency count differs")
	}
	for _, w := range a.Generation.Worlds {
		found := false
		for _, ref := range independent.Worlds {
			if ref.Theatre == w.Theatre && ref.Suffix == w.Suffix {
				found = true
				if !reflect.DeepEqual(w, ref) {
					t.Fatal("world bytes differ", w.Scenario)
				}
			}
		}
		if !found {
			t.Fatal("unexpected world", w.Scenario)
		}
	}
}

func TestTrainingReservedRowCompatibility(t *testing.T) {
	d := missionTestData(t)
	retained := make([]byte, 32)
	for i := range retained {
		retained[i] = byte(i)
	}
	result, err := GenerateMission(d, MissionInputs{MissionType: MissionTraining, Theatre: TheatreCentralEurope, Rank: RankWOC, Seed: 1, RetainedObject: retained})
	if err != nil {
		t.Fatal(err)
	}
	for i, v := range retained {
		if i >= 2 && i < 10 {
			continue
		}
		if result.Objects[0].Raw[i] != v {
			t.Fatal("native reserved-row retention changed", i)
		}
	}
	if result.Objects[0].Point() != result.Objectives["base"].OriginalPoint {
		t.Fatal("player coordinates must follow base anchor")
	}
}
