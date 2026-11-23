package state

import (
	"bytes"
	"encoding/json"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/internal/testfixture"
	"os"
	"reflect"
	"strings"
	"testing"
)

func failedWaterBuilder(t *testing.T) *missionBuilder {
	t.Helper()
	raw, err := os.ReadFile(testfixture.Path("campaign-continuations/europe-failed-relocation-checkpoint.json"))
	if err != nil {
		t.Fatal(err)
	}
	var phase struct {
		Anchor    MissionPoint
		Formation int
		Objects   []MissionObject
		Grid      []byte
		World     assetdata.MissionWorld
	}
	if err = json.Unmarshal(raw, &phase); err != nil {
		t.Fatal(err)
	}
	if phase.Formation != 193 || len(phase.Objects) != 30 || phase.Anchor != (MissionPoint{454656, 135168}) {
		t.Fatal("retained failed-start phase identity changed")
	}
	a := missionTestData(t)
	g, err := newGenericMissionPopulation(&phase.World, newMissionRNG(MissionInputs{Seed: 8}), 4, a.Tables.Leader)
	if err != nil {
		t.Fatal(err)
	}
	// The retained record callback has already appended the downed pilot.
	// Reconstruct the named immediately-before-expansion boundary for this
	// supplied phase, not a completed Campaign or a natural DOS observation.
	g.objects = append([]MissionObject(nil), phase.Objects[:29]...)
	g.grid = append([]byte(nil), phase.Grid...)
	g.current, g.selected, g.startFailed = phase.Anchor, phase.Formation, true
	b := &missionBuilder{g: g, in: MissionInputs{GenerationPolicy: CampaignGenerationPolicyPairedV1}, base: 22, upper: 9, kinds: map[int]int{3: 6}, refs: map[int]int{2: 28}}
	b.boundary()
	return b
}

func TestCampaignLimitedWaterFallbackKeepsAllConstraints(t *testing.T) {
	b := failedWaterBuilder(t)
	g := b.g
	original, grid := g.current, append([]byte(nil), g.grid...)
	objects := append([]MissionObject(nil), g.objects...)
	// Independent StepE/Step38 domain proof finds23 terrain cells. Its nearest
	// terrain-only candidate fails the source secondary/base separation.
	g.current = missionPoint(20*8192+4096, 25*8192+4096)
	if b.objectiveAllows(3, 193) {
		t.Fatal("terrain-only nearest incorrectly bypasses role constraints")
	}
	g.current = original
	if !b.recoverLimitedObjective(3, 193) || g.current != (MissionPoint{241664, 413696}) {
		t.Fatalf("global source-predicate candidate got %+v", g.current)
	}
	if b.distance(b.base) != 6 || b.distance(b.refs[2]) != 5 || b.edge(nil) != 12 || !b.objectiveAllows(3, 193) {
		t.Fatal("selected fallback loosens original distance predicates")
	}
	if len(g.rng.trace) != 0 || !bytes.Equal(g.grid, grid) || !reflect.DeepEqual(g.objects, objects) || g.selected != 193 || g.startFailed {
		t.Fatal("search consumed input/RNG, expanded early or replaced formation")
	}
	if len(g.decisions) != 1 || g.decisions[0].Operation != "limited_objective_global_fallback" || g.decisions[0].Point != g.current {
		t.Fatal("resolved fallback not recorded with mission generation")
	}
	start, center := len(g.objects), g.current
	g.expand()
	count := len(g.objects) - start
	if !b.recoveredObjectiveOwnsCell(3, start, 193, count, center, [2]int{29, 50}) {
		t.Fatal("same just-expanded site lost its free pre-expansion ownership")
	}
	g.objects[start].Formation = 191
	if b.recoveredObjectiveOwnsCell(3, start, 193, count, center, [2]int{29, 50}) {
		t.Fatal("another formation borrowed the own-cell exception")
	}
	b = failedWaterBuilder(t)
	b.g.grid[29+64*(63-50)] |= 128
	if b.recoverLimitedObjective(3, 193) || len(b.g.decisions) != 0 {
		t.Fatal("preexisting occupation became a self-occupation exception")
	}
	// No feasible water candidate remains a real failure; never invent land or
	// drop the objective. All2916 cells are examined once without random retry.
	b = failedWaterBuilder(t)
	for i := range b.g.grid {
		b.g.grid[i] |= 128
	}
	if b.recoverLimitedObjective(3, 193) || b.g.current != original || len(b.g.decisions) != 0 || len(b.g.rng.trace) != 0 {
		t.Fatal("empty terrain domain fabricated a placement")
	}
}

func TestCampaignEURO7RetainedHistoryNowPrepares(t *testing.T) {
	raw, err := os.ReadFile(testfixture.Path("campaign-continuations/central_europe-continuation.json"))
	if err != nil {
		t.Fatal(err)
	}
	var history []struct {
		Input *CampaignState `json:"input_campaign"`
	}
	if err = json.Unmarshal(raw, &history); err != nil || len(history) != 8 || history[7].Input == nil {
		t.Fatal("retained EURO input missing", err)
	}
	p := campaignTestPilot(t)
	p.Theatre, p.Campaign = TheatreCentralEurope, history[7].Input
	before, _ := json.Marshal(p)
	a := &assetdata.MissionAssets{Generation: missionTestData(t)}
	m, draft, err := CampaignStart(a, p, 8, nil, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if m.World != "EURO7.WRL" || len(m.Objects) != 80 || draft.GenerationPolicy != CampaignGenerationPolicyPairedV1 {
		t.Fatal("retained history replaced its world/population/owner")
	}
	found := 0
	for _, d := range m.Decisions {
		if d.Operation == "limited_objective_global_fallback" {
			found++
			if d.Formation != 193 || d.Point != (MissionPoint{241664, 413696}) {
				t.Fatalf("unexpected fallback %+v", d)
			}
		}
	}
	if found != 1 {
		t.Fatalf("got%d fallback receipts", found)
	}
	after, _ := json.Marshal(p)
	if !bytes.Equal(before, after) {
		t.Fatal("unaccepted preparation changed Campaign input")
	}
	if _, _, err = CampaignStart(a, p, 8, nil, nil); err == nil || !strings.Contains(err.Error(), "objective relocation diagnostic retry limit") {
		t.Fatal("historical interpretation changed", err)
	}
	again, _, err := CampaignStart(a, p, 8, nil, nil, true)
	if err != nil || !reflect.DeepEqual(m.Objects, again.Objects) || !reflect.DeepEqual(m.Draws, again.Draws) || !reflect.DeepEqual(m.Decisions, again.Decisions) {
		t.Fatal("resolved fallback/RNG is nondeterministic", err)
	}
}
