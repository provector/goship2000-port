package state

import (
	"encoding/binary"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	"strings"
)

// MissionTargets retains the entire source record, including unused string and
// endpoint bytes. A native generated mission starts at the source fresh-GS BSS
// initializer; imported preparation retains its exact supplied310 bytes.
type MissionTargets struct {
	Records            [5][62]byte           `json:"records"`
	Origin             string                `json:"origin"`
	ProgressFailure    string                `json:"progress_failure,omitempty"`
	ObjectiveRevisions *[2]ObjectiveRevision `json:"objective_revisions,omitempty"`
}

// ObjectiveRevision keeps the source goal before its live category/countdown
// changes. It identifies an actual goal, never a historical pilot or a reward.
// Only fresh R5 live TARGET copies initialize it; old films omit this owner.
type ObjectiveRevision struct {
	Seq       uint32   `json:"seq"`
	Source    [62]byte `json:"source"`
	Completed bool     `json:"completed"`
}

func CloneMissionTargets(t *MissionTargets) *MissionTargets {
	if t == nil {
		return nil
	}
	copy := *t
	if t.ObjectiveRevisions != nil {
		revisions := *t.ObjectiveRevisions
		copy.ObjectiveRevisions = &revisions
	}
	return &copy
}

// InitObjectiveRevisions is explicit and idempotent. Restoring a checkpoint or
// reading an older film must not manufacture lineage from its final TARGETs.
func (t *MissionTargets) InitObjectiveRevisions() {
	if t == nil || t.ObjectiveRevisions != nil {
		return
	}
	t.ObjectiveRevisions = &[2]ObjectiveRevision{
		{Seq: 1, Source: t.Records[2]},
		{Seq: 1, Source: t.Records[3]},
	}
}

func (t *MissionTargets) CompleteObjectiveRevision(role int) {
	if t != nil && t.ObjectiveRevisions != nil && role >= 0 && role < 2 {
		t.ObjectiveRevisions[role].Completed = true
	}
}

// PromoteSecondaryRevision follows the actual full TARGET4-to-3 copy. The new
// source remains immutable if immediate dispatch completes it in this tick.
func (t *MissionTargets) PromoteSecondaryRevision() {
	if t != nil && t.ObjectiveRevisions != nil {
		r := &t.ObjectiveRevisions[1]
		r.Seq++
		r.Source, r.Completed = t.Records[3], false
	}
}

// SourceObjectiveCountdown is the proven normal-domain software-FP result.
// Its units are progression invocations, not seconds or physics DF quanta.
func SourceObjectiveCountdown(a, b MissionPoint) (uint16, error) {
	abs := func(v int64) int64 {
		if v < 0 {
			return -v
		}
		return v
	}
	x, y := abs(int64(a.X)-int64(b.X)), abs(int64(a.Y)-int64(b.Y))
	if x < y {
		x, y = y, x
	}
	d := x + y/2
	if d > 786432 {
		return 0, fmt.Errorf("objective countdown outside verified normal coordinate domain")
	}
	return uint16(27 * d / 2048), nil
}

// PrepareMissionTargets runs once before the accepted launch. No result-stage
// reconstruction discards opaque tails or rerolls mission generation.
func PrepareMissionTargets(a *assetdata.MissionAssets, m *MissionData) error {
	if m == nil {
		return fmt.Errorf("TARGET preparation needs a mission")
	}
	if m.Targets != nil {
		return nil
	}
	t := &MissionTargets{Origin: "fresh-gs-bss-and-A18e46-v1"}
	if raw := m.SourcePreparation; raw != nil {
		if len(raw.TargetBlock) != 310 {
			return fmt.Errorf("retained TARGET block is not310 bytes")
		}
		for i := range t.Records {
			copy(t.Records[i][:], raw.TargetBlock[i*62:(i+1)*62])
		}
		t.Origin = "retained-source310"
		m.Targets = t
		return nil
	}
	if a == nil || a.Preflight == nil || a.Preflight.ObjectiveSummary == nil || m.SourceWorld == nil {
		return fmt.Errorf("TARGET preparation needs source labels and world")
	}
	labels := a.Preflight.ObjectiveSummary
	if labels.RecordFormat != "%02d/%02d %s %Fs" || len(labels.Verbs) != 8 {
		return fmt.Errorf("TARGET name format/profile unsupported")
	}
	for i, name := range []string{"base", "FARP", "primary", "secondary", "tertiary"} {
		o, ok := m.Objectives[name]
		if !ok {
			continue
		} // Source fresh BSS for an inactive/unwritten role.
		if o.Formation < 0 || o.Formation >= len(m.SourceWorld.FMHD)/39 || o.RawKind < 0 || o.RawKind >= len(labels.Verbs) {
			return fmt.Errorf("TARGET %s source formation/kind unavailable", name)
		}
		formation := assetdata.OfficeCString(m.SourceWorld.FMHD[o.Formation*39 : o.Formation*39+25])
		verb := ""
		if i >= 2 {
			verb = labels.Verbs[o.RawKind]
		}
		text := fmt.Sprintf("%02d/%02d %s %s", MissionGrid(o.Point.X), MissionGrid(o.Point.Y), verb, formation)
		for len(text) > 31 {
			changed := false
			for _, pair := range labels.Abbreviations {
				if pair[0] != "" && len(pair[1]) < len(pair[0]) && strings.Contains(text, pair[0]) {
					text = strings.Replace(text, pair[0], pair[1], 1)
					changed = true
					break
				}
			}
			if !changed {
				text = text[:31]
				break
			}
		}
		r := &t.Records[i]
		copy(r[:32], text)
		binary.LittleEndian.PutUint16(r[32:], uint16(o.DisplayKind))
		binary.LittleEndian.PutUint32(r[34:], uint32(o.OriginalPoint.X))
		binary.LittleEndian.PutUint32(r[38:], uint32(o.OriginalPoint.Y))
		binary.LittleEndian.PutUint32(r[42:], uint32(o.Point.X))
		binary.LittleEndian.PutUint32(r[46:], uint32(o.Point.Y))
		binary.LittleEndian.PutUint16(r[50:], uint16(o.Count))
		if o.Endpoint != nil {
			binary.LittleEndian.PutUint32(r[54:], uint32(o.Endpoint.X))
			binary.LittleEndian.PutUint32(r[58:], uint32(o.Endpoint.Y))
		}
	}
	if q, ok := m.Objectives["tertiary"]; ok && q.DisplayKind != 0 {
		if m.TargetCountdown == nil {
			return fmt.Errorf("TARGET alternate requires retained source countdown producer")
		}
		binary.LittleEndian.PutUint16(t.Records[3][52:], *m.TargetCountdown)
	}
	m.Targets = t
	if m.PlacementPlan != nil {
		m.PlacementPlan.TargetsDigest = authoredDigest(t)
	}
	return nil
}
