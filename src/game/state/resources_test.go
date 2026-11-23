package state

import (
	"encoding/json"
	"gunship2000-assets/game/render3d"
	"gunship2000-assets/game/world"
	"gunship2000-assets/internal/testfixture"
	"os"
	"reflect"
	"testing"
)

func resourceWitness(t *testing.T) (*AuthoredDocument, AuthoredOption) {
	t.Helper()
	raw, e := os.ReadFile(testfixture.Path("model-budget/fast-observations.json"))
	if os.IsNotExist(e) {
		t.Skip("retained Step58 model-resource witness unavailable")
	}
	if e != nil {
		t.Fatal(e)
	}
	var v []struct {
		Document *AuthoredDocument `json:"final_accepted_document"`
		Witness  struct {
			Option AuthoredOption `json:"option"`
		} `json:"witness"`
	}
	if e = json.Unmarshal(raw, &v); e != nil || len(v) != 1 || v[0].Document == nil {
		t.Fatal(e)
	}
	return v[0].Document, v[0].Witness.Option
}

func TestModelResourceWitnessSaveReopenCompile(t *testing.T) {
	a := initializationAssets(t)
	data := a.Missions.Generation
	d, o := resourceWitness(t)
	if cost, e := AuthoredModelEstimate(data, d); e != nil || cost != 40009 {
		t.Fatal(cost, e)
	}
	if len(d.Placements) != 32 {
		t.Fatal("wrong witness graph")
	}
	if e := PlaceAuthoredOption(data, d, "defender", 0, o, 100, 100); e == nil {
		t.Fatal("legacy source limit changed")
	}
	if e := PlaceNativeAuthoredOption(data, d, "defender", 0, o, 100, 100); e != nil {
		t.Fatal(e)
	}
	if cost, e := AuthoredModelEstimate(data, d); e != nil || cost != 40564 {
		t.Fatal(cost, e)
	}
	if n, e := AuthoredBudget(data, d); e != nil || n != 30 || len(d.Placements) != 33 {
		t.Fatal("real budget changed", n, e)
	}
	in := MissionInputs{MissionType: MissionSimulated, Theatre: TheatreCentralEurope, Rank: RankCaptain, Seed: 1}
	m, e := CompileAuthoredMission(data, d, in)
	if e != nil || len(m.Objects) != 80 {
		t.Fatal(e)
	}
	exported, ex := NewAuthoredFromFilmSource(data, d, m, "R6 source export")
	if ex != nil || exported.ResourcePolicy != NativeModelResourcesV1 {
		t.Fatal("authored Export", ex)
	}
	store := AuthoredStore{Directory: t.TempDir()}
	rev, e := store.Save(d, "")
	if e != nil {
		t.Fatal(e)
	}
	q, rev2, e := store.Load(d.ID)
	if e != nil || rev != rev2 {
		t.Fatal(e)
	}
	m2, e := CompileAuthoredMission(data, q, in)
	if e != nil || m.ObjectHash != m2.ObjectHash || !reflect.DeepEqual(m.Paths, m2.Paths) || !reflect.DeepEqual(m.Targets, m2.Targets) {
		t.Fatal("reopened graph", e)
	}
	renderer, re := render3d.New(a.World)
	if re != nil {
		t.Fatal(re)
	}
	for _, scale := range []int{1, 2} {
		scene := render3d.Scene{World: "EURO6", View: render3d.FullView(), RenderScale: scale}
		pixels := make([]byte, scene.TargetPixels())
		draw, e := renderer.DrawModelPreview(scene, nil, pixels, render3d.ModelPreview{ID: "actual-AT5", ModelID: 13, Translation: world.Vertex{0, 0, 32}})
		if e != nil || draw.Primitives == 0 {
			t.Fatal("actual AT-5 prepared model draw", scale, draw, e)
		}
	}
	// Missing resource and invalid geometry remain real admission failures.
	model := data.NativeModels["model/013"]
	delete(data.NativeModels, "model/013")
	if _, e = AuthoredModelEstimate(data, q); e == nil {
		t.Fatal("missing model admitted")
	}
	data.NativeModels["model/013"] = model
	bad := *model
	bad.Root = 99999
	data.NativeModels["model/013"] = &bad
	if _, e = AuthoredModelEstimate(data, q); e == nil {
		t.Fatal("invalid model admitted")
	}
	data.NativeModels["model/013"] = model
}

func TestModelWideDiagnosticsAndSharedConsumers(t *testing.T) {
	a := initializationAssets(t)
	data := *a.Missions.Generation
	data.Tables = a.Missions.Generation.Tables
	data.Tables.EditorModelCosts = append([]uint16(nil), data.Tables.EditorModelCosts...)
	for i := range data.Tables.EditorModelCosts {
		data.Tables.EditorModelCosts[i] = 65535
	}
	d, in := authoredReady(t, &data)
	d.ResourcePolicy = NativeModelResourcesV1
	if cost, e := AuthoredModelEstimate(&data, d); e != nil || cost <= 65535 {
		t.Fatal("wide sum wrapped", cost, e)
	}
	if _, e := CompileAuthoredMission(&data, d, in); e != nil {
		t.Fatal("required Fill reapplied DOS limit", e)
	}
	if _, e := CheckAuthoredFARPArm(&data, d); e != nil {
		t.Fatal("FARP reapplied DOS limit", e)
	}
	op, e := AuthoredSupportOptions(&data, d)
	if e != nil || len(op) < 2 {
		t.Fatal(e)
	}
	d.SupportEdits = &AuthoredSupportEdits{Profile: OwnedSupportChoiceV1}
	d.Compiled = nil
	if e = SetAuthoredSupport(&data, d, op[1]); e != nil {
		t.Fatal("Support reapplied DOS limit", e)
	}
	if _, e = CompileAuthoredMission(&data, d, in); e != nil {
		t.Fatal("Support/Fill compile", e)
	}
	before, _ := json.Marshal(d)
	q, _ := CloneAuthored(d)
	for len(q.Placements) <= 75 {
		p := q.Placements[0]
		p.ID, _ = authoredID()
		q.Placements = append(q.Placements, p)
	}
	if _, e = CompileAuthoredMission(&data, q, in); e == nil {
		t.Fatal("real descriptor limit removed")
	}
	after, _ := json.Marshal(d)
	if string(before) != string(after) {
		t.Fatal("failed candidate changed accepted document")
	}
}
