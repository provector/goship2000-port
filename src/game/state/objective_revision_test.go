package state

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"reflect"
	"testing"
)

func TestObjectiveRevisionCloneLifetimeAndJSON(t *testing.T) {
	prepared := &MissionTargets{Origin: "supplied full62 source"}
	for role := 0; role < 2; role++ {
		for i := range prepared.Records[role+2] {
			prepared.Records[role+2][i] = byte(17*role + i)
		}
		binary.LittleEndian.PutUint16(prepared.Records[role+2][32:], 2)
	}
	live := CloneMissionTargets(prepared)
	live.InitObjectiveRevisions()
	if prepared.ObjectiveRevisions != nil || live.ObjectiveRevisions == nil {
		t.Fatal("live revision initialization mutated preparation")
	}
	for role, revision := range live.ObjectiveRevisions {
		if revision.Seq != 1 || revision.Completed || revision.Source != prepared.Records[role+2] {
			t.Fatal("initial full source revision", role, revision)
		}
	}
	live.CompleteObjectiveRevision(0)
	live.Records[2][32], live.Records[2][52] = 0, 0
	live.InitObjectiveRevisions()
	if !live.ObjectiveRevisions[0].Completed || live.ObjectiveRevisions[0].Source != prepared.Records[2] {
		t.Fatal("idempotent init/completion reconstructed mutable TARGET")
	}
	cloned := CloneMissionTargets(live)
	if cloned.ObjectiveRevisions == live.ObjectiveRevisions {
		t.Fatal("revision array aliased during live clone")
	}
	cloned.ObjectiveRevisions[0].Source[61] ^= 0xff
	cloned.Records[3][61] ^= 0xff
	if live.ObjectiveRevisions[0].Source != prepared.Records[2] || live.Records[3] != prepared.Records[3] {
		t.Fatal("cloned live state modified its source")
	}
	encoded, err := json.Marshal(live)
	if err != nil {
		t.Fatal(err)
	}
	var restored MissionTargets
	if err = json.Unmarshal(encoded, &restored); err != nil || !reflect.DeepEqual(live, &restored) {
		t.Fatal("revision checkpoint round trip", err)
	}
	if CloneMissionTargets(nil) != nil {
		t.Fatal("nil clone invented targets")
	}
}

func TestObjectiveRevisionHistoricalOmissionAndExplicitPromotion(t *testing.T) {
	legacy := &MissionTargets{Origin: "legacy"}
	legacy.CompleteObjectiveRevision(0)
	legacy.PromoteSecondaryRevision()
	oldJSON, _ := json.Marshal(struct {
		Records         [5][62]byte `json:"records"`
		Origin          string      `json:"origin"`
		ProgressFailure string      `json:"progress_failure,omitempty"`
	}{legacy.Records, legacy.Origin, legacy.ProgressFailure})
	gotJSON, err := json.Marshal(legacy)
	if err != nil || !bytes.Equal(oldJSON, gotJSON) || legacy.ObjectiveRevisions != nil {
		t.Fatal("historical TARGET JSON changed or lineage invented", err)
	}
	legacy.InitObjectiveRevisions()
	beforePrimary := legacy.ObjectiveRevisions[0]
	for i := range legacy.Records[4] {
		legacy.Records[4][i] = byte(255 - i)
	}
	legacy.Records[3] = legacy.Records[4]
	legacy.PromoteSecondaryRevision()
	wantSource := legacy.Records[4]
	legacy.CompleteObjectiveRevision(1)
	legacy.Records[3][32], legacy.Records[3][52] = 0, 0
	if legacy.ObjectiveRevisions[0] != beforePrimary || legacy.ObjectiveRevisions[1].Seq != 2 || !legacy.ObjectiveRevisions[1].Completed || legacy.ObjectiveRevisions[1].Source != wantSource {
		t.Fatal("promotion lost full immutable source or primary owner", legacy.ObjectiveRevisions)
	}
}
