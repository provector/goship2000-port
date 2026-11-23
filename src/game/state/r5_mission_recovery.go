package state

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
)

type MissionRecoveryIdentity struct {
	PilotID string `json:"pilot_id"`
	SHA256  string `json:"sha256"`
}

// Decision is frozen with the player proposal. Outcome is a separate final-ack
// receipt: a valid later peer edit must not reroll the player's awards or revive
// that changed peer. Missing identity is never replaced by a roster first match.
type MissionRecoveryDecision struct {
	Policy          string                    `json:"policy"`
	Status          string                    `json:"status"`
	BasisSHA256     string                    `json:"basis_sha256"`
	Reason          string                    `json:"reason,omitempty"`
	Candidates      []MissionRecoveryIdentity `json:"candidates,omitempty"`
	SelectedPilotID string                    `json:"selected_pilot_id,omitempty"`
}

type MissionRecoveryOutcome struct {
	Policy         string `json:"policy"`
	Status         string `json:"status"`
	BasisSHA256    string `json:"basis_sha256"`
	Reason         string `json:"reason,omitempty"`
	PilotID        string `json:"pilot_id,omitempty"`
	ExpectedSHA256 string `json:"expected_sha256,omitempty"`
	ObservedSHA256 string `json:"observed_sha256,omitempty"`
	AppliedSHA256  string `json:"applied_sha256,omitempty"`
}

func prepareOptionalMissionRecovery(p *SortieProposal) (*MissionRecoveredPilot, *MissionRecoveryDecision, error) {
	f, ctx := p.Facts, p.Facts.ResultContext
	d := &MissionRecoveryDecision{Policy: OptionalMissionRecoveryPolicy, Status: "not-applicable"}
	// The complete frozen context, including revision provenance, is immutable
	// result input; no live roster or final TARGET reconstruction enters this hash.
	basis, err := json.Marshal(struct {
		Sortie  string
		Flags   byte
		Context *SortieResultContext
	}{f.SortieID, f.SourceFlags, ctx})
	if err != nil {
		return nil, nil, err
	}
	d.BasisSHA256 = assetdata.Hash(basis)
	unresolved := func(reason string) (*MissionRecoveredPilot, *MissionRecoveryDecision, error) {
		d.Status, d.Reason = "unresolved", reason
		return nil, d, nil
	}
	if ctx == nil || ctx.MissionRecovery == nil {
		return unresolved("missing-recovery-context")
	}
	c := ctx.MissionRecovery
	if c.Policy != OptionalMissionRecoveryPolicy || len(c.LaunchTargets) != 5 {
		return nil, nil, fmt.Errorf("unsupported optional recovery launch contract")
	}
	if err = validateRecoveryCandidates(c.Candidates, p.PilotBefore.ID); err != nil {
		return nil, nil, err
	}
	if p.Result == nil || p.Result.Loss || f.SourceFlags&0x80 == 0 || len(ctx.Actors) == 0 || ctx.Actors[0].Ship[0]&0xf8 != 0x18 {
		return nil, d, nil
	}
	qualified, missing := false, false
	for role, owner := range c.ObjectiveAssignments {
		if owner != 1 || f.SourceFlags&(1<<role) == 0 {
			continue
		}
		if c.ObjectiveRevisions == nil || c.ObjectiveRevisions[role].Seq == 0 {
			missing = true
			continue
		}
		r := c.ObjectiveRevisions[role]
		if binary.LittleEndian.Uint16(r.Source[32:]) != 2 {
			continue
		}
		if !r.Completed {
			missing = true
			continue
		}
		qualified = true
	}
	if !qualified && !missing {
		return nil, d, nil
	}
	var chosen *MissionRecoveryCandidate
	unknown := false
	for i := range c.Candidates {
		r := &c.Candidates[i]
		before := r.Before
		if before.Theatre.Raw() < 0 || len(before.LegacyRaw) != LegacyRecordSize || before.LegacyStatus != 3 || before.LegacyRaw[0x56] != 3 || before.Rank.Raw() < 0 || before.Rank.Raw() > 11 {
			unknown = true
			continue
		}
		if before.Theatre != p.PilotBefore.Theatre || before.LegacyRaw[0x5b] != ctx.SelectedScenario {
			continue
		}
		d.Candidates = append(d.Candidates, MissionRecoveryIdentity{before.ID, r.BeforeSHA256})
		chosen = r
	}
	if unknown {
		return unresolved("incomplete-historical-identity")
	}
	if len(d.Candidates) == 0 {
		return nil, d, nil
	}
	if missing {
		return unresolved("missing-pickup-revision")
	}
	if len(d.Candidates) > 1 {
		return unresolved("ambiguous-historical-identity")
	}
	after := cloneSortiePilot(chosen.Before)
	after.Status, after.LegacyStatus, after.LegacyRaw[0x56] = StatusActive, 1, 1
	d.Status, d.SelectedPilotID = "resolved", chosen.Before.ID
	return &MissionRecoveredPilot{OptionalMissionRecoveryPolicy, chosen.SourceRow, chosen.BeforeSHA256, cloneSortiePilot(chosen.Before), after}, d, nil
}

func recoveryOutcomeForProposal(p *SortieProposal) *MissionRecoveryOutcome {
	if p == nil || p.RecoveryDecision == nil {
		return nil
	}
	d := p.RecoveryDecision
	o := &MissionRecoveryOutcome{Policy: d.Policy, Status: d.Status, BasisSHA256: d.BasisSHA256, Reason: d.Reason, PilotID: d.SelectedPilotID}
	if p.RecoveredPilot != nil {
		o.ExpectedSHA256 = p.RecoveredPilot.BeforeSHA256
	}
	return o
}

func validateRecoveryOutcome(doc *sortieDocument) error {
	if doc.Start.ProposalVersion != SafeSortieProposalVersion {
		if doc.RecoveryOutcome != nil {
			return fmt.Errorf("historical result cannot contain an optional recovery outcome")
		}
		return nil
	}
	if doc.Phase != SortieCommitted {
		if doc.RecoveryOutcome != nil {
			return fmt.Errorf("uncommitted result claims an optional recovery outcome")
		}
		return nil
	}
	p, o := doc.Proposal, doc.RecoveryOutcome
	if p == nil || p.RecoveryDecision == nil || o == nil {
		return fmt.Errorf("R5 committed result needs its optional recovery receipt")
	}
	want := recoveryOutcomeForProposal(p)
	if o.Policy != want.Policy || o.BasisSHA256 != want.BasisSHA256 || o.PilotID != want.PilotID || o.ExpectedSHA256 != want.ExpectedSHA256 {
		return fmt.Errorf("optional recovery receipt differs from its frozen decision")
	}
	if p.RecoveredPilot == nil {
		if *o != *want || doc.CommittedRecovery != "" {
			return fmt.Errorf("unresolved/unused recovery receipt claims a peer write")
		}
		return nil
	}
	switch o.Status {
	case "applied":
		raw, err := assetdata.JsonBytes(pilotDocument{pilotSchemaVersion, p.RecoveredPilot.After})
		if err != nil || o.AppliedSHA256 != assetdata.Hash(raw) || doc.CommittedRecovery != o.AppliedSHA256 || o.ObservedSHA256 != o.ExpectedSHA256 || o.Reason != "" {
			return fmt.Errorf("applied optional recovery digest mismatch")
		}
	case "deferred":
		if doc.CommittedRecovery != "" || o.AppliedSHA256 != "" || o.Reason != "peer-changed" && o.Reason != "peer-removed" || o.Reason == "peer-changed" && (!validSortieHash(o.ObservedSHA256) || o.ObservedSHA256 == o.ExpectedSHA256) || o.Reason == "peer-removed" && o.ObservedSHA256 != "" {
			return fmt.Errorf("deferred optional recovery receipt invalid")
		}
	default:
		return fmt.Errorf("resolved optional recovery lacks applied/deferred disposition")
	}
	return nil
}

// SortieRecoveryOutcome reads an immutable committed receipt, never rescans the
// current roster or retries a deferred optional restoration.
func (s *SaveStore) SortieRecoveryOutcome(id string) (*MissionRecoveryOutcome, error) {
	if !validSortieID(id) {
		return nil, fmt.Errorf("invalid recovery receipt sortie identity")
	}
	raw, err := s.currentRecord(sortieFilename(id))
	if err != nil || raw == nil {
		return nil, err
	}
	doc, err := decodeSortieRecord(raw, id)
	if err != nil || doc.Phase != SortieCommitted || doc.RecoveryOutcome == nil {
		return nil, err
	}
	out := *doc.RecoveryOutcome
	return &out, nil
}
