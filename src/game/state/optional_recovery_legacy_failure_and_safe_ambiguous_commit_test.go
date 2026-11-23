package state

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func miaEntry(t *testing.T, ambiguous bool) (*SaveStore, *SortieStart, SortieFacts, string) {
	t.Helper()
	s, oldStart, f, peer, _ := miaRecoveryEntry(t)
	if err := s.AbandonSortie(oldStart.ID, "supplied R5 recovery comparison"); err != nil {
		t.Fatal(err)
	}
	if ambiguous {
		next := s.State()
		other := cloneSortiePilot(next.Slots[0])
		other.ID, other.Name = "r5-other-mia", "Other Missing Pilot"
		next.Slots[1] = other
		if err := s.Commit(next); err != nil {
			t.Fatal(err)
		}
	}
	request := pendingTestStart()
	request.GameplayPolicy = SafeGameplayPolicy
	start, err := s.BeginSortie(request)
	if err != nil {
		t.Fatal(err)
	}
	f.SortieID = start.ID
	f.ResultContext.GameplayPolicy = SafeGameplayPolicy
	c := f.ResultContext.MissionRecovery
	c.Policy, c.Candidates = OptionalMissionRecoveryPolicy, start.RecoveryRoster
	c.ObjectiveRevisions = &[2]ObjectiveRevision{{Seq: 1, Source: c.LaunchTargets[2], Completed: true}, {Seq: 1, Source: c.LaunchTargets[3]}}
	return s, start, f, peer
}

func peerWrite(t *testing.T, s *SaveStore, id string, pretty bool) []byte {
	t.Helper()
	var peer *Pilot
	for _, p := range s.State().Slots {
		if p != nil && p.ID == id {
			peer = p
		}
	}
	if peer == nil {
		t.Fatal("peer fixture missing")
	}
	peer.Name = "Externally amended missing pilot"
	raw, err := assetdata.JsonBytes(pilotDocument{pilotSchemaVersion, peer})
	if pretty {
		raw, err = json.Marshal(pilotDocument{pilotSchemaVersion, peer})
	}
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(s.Directory, pilotFilename(id)), raw, 0600); err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestOptionalRecoveryLegacyFailureAndSafeAmbiguousCommit(t *testing.T) {
	s, start, f, peer := miaEntry(t, true)
	old := f
	raw, _ := json.Marshal(f)
	_ = json.Unmarshal(raw, &old)
	old.ResultContext.GameplayPolicy = ""
	old.ResultContext.MissionRecovery.Policy = MissionRecoveryPolicy
	old.ResultContext.MissionRecovery.ObjectiveRevisions = nil
	if _, err := ProposeSortie(start.PilotBefore, old, pendingTestTables()); err == nil {
		t.Fatal("before-fix ambiguity guard unexpectedly accepted")
	} else {
		t.Log("legacy before: valid actual pickup rejected by optional roster ambiguity:", err)
	}
	p, err := ProposeSortie(start.PilotBefore, f, pendingTestTables())
	if err != nil || p.Version != SafeSortieProposalVersion || p.RecoveredPilot != nil || p.RecoveryDecision.Status != "unresolved" || p.RecoveryDecision.Reason != "ambiguous-historical-identity" {
		t.Fatal(p, err)
	}
	if p.AcceptedPilot.Rank != RankMajor || p.InitialRankRecommendation != 8 {
		t.Fatal("ambiguous peer suppressed ordinary promotion", p.AcceptedPilot.Rank, p.InitialRankRecommendation)
	}
	if err = s.StageSortieResult(start.ID, p); err != nil {
		t.Fatal(err)
	}
	before := start.PilotBefore.Statistics.MissionCampaignCounter
	if err = s.CommitSortie(start.ID); err != nil {
		t.Fatal(err)
	}
	for _, candidate := range s.State().Slots {
		if candidate != nil && candidate.ID != start.PilotID && candidate.Status != StatusMIA {
			t.Fatal("arbitrary peer revived", candidate.ID)
		}
	}
	if s.State().SelectedPilot().Statistics.MissionCampaignCounter != before+1 {
		t.Fatal("core result discarded")
	}
	out, err := s.SortieRecoveryOutcome(start.ID)
	if err != nil || out.Status != "unresolved" || out.Reason != "ambiguous-historical-identity" || out.AppliedSHA256 != "" {
		t.Fatal(out, err)
	}
	committed := s.State()
	if err = s.CommitSortie(start.ID); err != nil || !reflect.DeepEqual(committed, s.State()) {
		t.Fatal("duplicate result", err)
	}
	reopened, err := OpenSaveStore(s.Directory, "")
	if err != nil {
		t.Fatal(err)
	}
	if got, e := reopened.SortieRecoveryOutcome(start.ID); e != nil || !reflect.DeepEqual(got, out) {
		t.Fatal("receipt restart", got, e)
	}
	for _, candidate := range reopened.State().Slots {
		if candidate != nil && candidate.ID == peer && candidate.Status != StatusMIA {
			t.Fatal("restart rescanned unresolved receipt")
		}
	}
}

func TestOptionalRecoveryRevisionAndActualFacts(t *testing.T) {
	for _, mode := range []string{"unique", "retasked-pickup", "retasked-nonpickup", "missing-lineage", "no-cargo", "no-return", "no-completion", "wrong-actor", "no-match"} {
		t.Run(mode, func(t *testing.T) {
			s, start, f, peer := miaEntry(t, false)
			c := f.ResultContext.MissionRecovery
			want := "not-applicable"
			switch mode {
			case "unique":
				want = "resolved"
			case "retasked-pickup", "retasked-nonpickup":
				f.SourceFlags = 0x82
				f.ResultContext.Actors[0].Flags = f.SourceFlags
				c.ObjectiveAssignments = [2]uint16{0, 1}
				c.LaunchTargets[3] = c.LaunchTargets[2]
				source := c.LaunchTargets[3]
				source[61] ^= 1
				if mode == "retasked-nonpickup" {
					binary.LittleEndian.PutUint16(source[32:], 3)
				} else {
					want = "resolved"
				}
				c.ObjectiveRevisions[1] = ObjectiveRevision{Seq: 2, Source: source, Completed: true}
				f.Targets[3] = source
				f.Targets[3][32], f.Targets[3][33] = 0, 0
			case "missing-lineage":
				c.ObjectiveRevisions = nil
				want = "unresolved"
			case "no-cargo":
				f.ResultContext.Actors[0].Ship[0] = 6
			case "no-return":
				f.Reason = "voluntary"
				f.SourceFlags &^= 0x80
				f.ResultContext.Actors[0].Flags = f.SourceFlags
			case "no-completion":
				f.SourceFlags &^= 1
				f.ResultContext.Actors[0].Flags = f.SourceFlags
			case "wrong-actor":
				c.ObjectiveAssignments[0] = 2
			case "no-match":
				f.ResultContext.SelectedScenario++
			}
			p, err := ProposeSortie(start.PilotBefore, f, pendingTestTables())
			if err != nil || p.RecoveryDecision.Status != want {
				t.Fatal(mode, p, err)
			}
			if (p.RecoveredPilot != nil) != (want == "resolved") {
				t.Fatal("decision/delta mismatch", mode)
			}
			if err = ValidateSortieProposal(p); err != nil {
				t.Fatal(err)
			}
			if err = s.StageSortieResult(start.ID, p); err != nil {
				t.Fatal(err)
			}
			if err = s.CommitSortie(start.ID); err != nil {
				t.Fatal(err)
			}
			out, err := s.SortieRecoveryOutcome(start.ID)
			if err != nil || out.Status == "resolved" || want == "resolved" && out.Status != "applied" {
				t.Fatal("unsealed optional outcome", out, err)
			}
			if want == "resolved" {
				for _, candidate := range s.State().Slots {
					if candidate != nil && candidate.ID == peer && candidate.Status != StatusActive {
						t.Fatal("unique peer not restored")
					}
				}
				if p.AcceptedPilot.Rank != RankCaptain {
					t.Fatal("resolved source JUMP promotion bypass changed")
				}
			}
		})
	}
}

func TestOptionalPeerChangedBeforeStageOrAckAndCoreConflicts(t *testing.T) {
	for _, mode := range []string{"before-stage", "before-ack", "format-only", "removed", "core-conflict", "other-peer-conflict"} {
		t.Run(mode, func(t *testing.T) {
			s, start, f, peer := miaEntry(t, false)
			p, err := ProposeSortie(start.PilotBefore, f, pendingTestTables())
			if err != nil {
				t.Fatal(err)
			}
			var preserved []byte
			if mode == "before-stage" {
				preserved = peerWrite(t, s, peer, true)
			}
			if err = s.StageSortieResult(start.ID, p); err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "before-ack":
				preserved = peerWrite(t, s, peer, true)
			case "format-only":
				var obj any
				_ = json.Unmarshal(s.files[pilotFilename(peer)], &obj)
				preserved, _ = json.Marshal(obj)
				if err = os.WriteFile(filepath.Join(s.Directory, pilotFilename(peer)), preserved, 0600); err != nil {
					t.Fatal(err)
				}
			case "removed":
				next := s.State()
				for i, candidate := range next.Slots {
					if candidate != nil && candidate.ID == peer {
						next.Slots[i] = nil
					}
				}
				next.Revision++
				files, e := stateFiles(next)
				if e != nil {
					t.Fatal(e)
				}
				if err = os.WriteFile(filepath.Join(s.Directory, saveFilename), files[saveFilename], 0600); err != nil {
					t.Fatal(err)
				}
			case "core-conflict":
				changed := cloneSortiePilot(start.PilotBefore)
				changed.Name = "Concurrent active pilot"
				raw, _ := assetdata.JsonBytes(pilotDocument{pilotSchemaVersion, changed})
				if err = os.WriteFile(filepath.Join(s.Directory, pilotFilename(start.PilotID)), raw, 0600); err != nil {
					t.Fatal(err)
				}
			case "other-peer-conflict":
				changed := cloneSortiePilot(start.PilotBefore)
				changed.ID, changed.Name = "unrelated-new-owner", "Unrelated owner"
				next := s.State()
				next.Slots[2] = changed
				next.Revision++
				files, _ := stateFiles(next)
				for _, path := range []string{pilotFilename(changed.ID), saveFilename} {
					if err = os.WriteFile(filepath.Join(s.Directory, path), files[path], 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
			err = s.CommitSortie(start.ID)
			if mode == "core-conflict" || mode == "other-peer-conflict" {
				if err == nil {
					t.Fatal("required owner conflict bypassed")
				}
				if doc, e := s.currentSortie(); e != nil || doc.Phase != SortiePending {
					t.Fatal("conflict discarded pending result", e)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			out, err := s.SortieRecoveryOutcome(start.ID)
			if err != nil || out.Status != "deferred" || out.PilotID != peer || out.AppliedSHA256 != "" {
				t.Fatal(out, err)
			}
			if mode == "removed" {
				if out.Reason != "peer-removed" {
					t.Fatal(out)
				}
			} else if got := mustReadSaveBytes(t, filepath.Join(s.Directory, pilotFilename(peer))); !bytes.Equal(got, preserved) || out.Reason != "peer-changed" {
				t.Fatal("peer bytes overwritten", out)
			}
			if !reflect.DeepEqual(s.State().SelectedPilot(), p.AcceptedPilot) {
				t.Fatal("peer conflict rerolled frozen player branch")
			}
			if err = s.CommitSortie(start.ID); err != nil {
				t.Fatal("duplicate deferred ack", err)
			}
			if _, err = OpenSaveStore(s.Directory, ""); err != nil {
				t.Fatal("deferred restart", err)
			}
		})
	}
}

func TestRecoveryVersionBoundaries(t *testing.T) {
	s, start, f, _ := miaEntry(t, false)
	if start.ProposalVersion != 5 {
		t.Fatal("safe start did not freeze v5")
	}
	p, err := ProposeSortie(start.PilotBefore, f, pendingTestTables())
	if err != nil || p.Version != 5 {
		t.Fatal(p, err)
	}
	if _, err = proposeSortieVersion(start.PilotBefore, f, pendingTestTables(), 4); err == nil {
		t.Fatal("v4 admitted safe context")
	}
	legacy := f
	legacy.ResultContext = nil
	if _, err = proposeSortieVersion(start.PilotBefore, legacy, pendingTestTables(), 5); err == nil {
		t.Fatal("v5 admitted missing safe policy")
	}
	doc, _ := s.currentSortie()
	if doc.SchemaVersion != 7 {
		t.Fatal("safe schema", doc.SchemaVersion)
	}
	for version, schema := range map[int]int{1: 1, 2: 4, 3: 5, 4: 6} {
		old, err := proposeSortieVersion(nResultPilot(RankCaptain), nResultFacts(), pendingTestTables(), version)
		if err != nil {
			t.Fatal("historical neighbor", version, err)
		}
		old.RecoveryDecision = &MissionRecoveryDecision{Policy: OptionalMissionRecoveryPolicy}
		if err = ValidateSortieProposal(old); err == nil {
			t.Fatal("historical proposal admitted new decision", version)
		}
		oldDoc := *doc
		oldDoc.SchemaVersion, oldDoc.Start.ProposalVersion, oldDoc.Start.GameplayPolicy = schema, version, ""
		oldDoc.RecoveryOutcome = &MissionRecoveryOutcome{Policy: OptionalMissionRecoveryPolicy}
		if err = validateSortieDocument(&oldDoc); err == nil {
			t.Fatal("historical schema admitted new outcome", schema)
		}
	}
	for _, mode := range []string{"schema6", "start4", "start-no-policy", "active-outcome"} {
		copy := *doc
		switch mode {
		case "schema6":
			copy.SchemaVersion = 6
		case "start4":
			copy.Start.ProposalVersion = 4
		case "start-no-policy":
			copy.Start.GameplayPolicy = ""
		case "active-outcome":
			copy.RecoveryOutcome = &MissionRecoveryOutcome{}
		}
		if err = validateSortieDocument(&copy); err == nil {
			t.Fatal("invalid version boundary", mode)
		}
	}
	if err = s.StageSortieResult(start.ID, p); err != nil {
		t.Fatal(err)
	}
	if err = s.CommitSortie(start.ID); err != nil {
		t.Fatal(err)
	}
	doc, _ = s.currentSortie()
	doc.RecoveryOutcome.Status, doc.CommittedRecovery = "deferred", ""
	if err = validateSortieDocument(doc); err == nil {
		t.Fatal("unexplained peer write omission admitted")
	}
}

func TestOptionalRecoveryCancellationLeavesBothPilots(t *testing.T) {
	s, start, f, peer := miaEntry(t, false)
	p, err := ProposeSortie(start.PilotBefore, f, pendingTestTables())
	if err != nil {
		t.Fatal(err)
	}
	if err = s.StageSortieResult(start.ID, p); err != nil {
		t.Fatal(err)
	}
	before := s.State()
	if err = s.AbandonSortie(start.ID, "explicit fixture cancellation before acknowledgment"); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before.Slots, s.State().Slots) {
		t.Fatal("cancel applied core or optional peer")
	}
	for _, pilot := range s.State().Slots {
		if pilot != nil && pilot.ID == peer && pilot.Status != StatusMIA {
			t.Fatal("cancel revived peer")
		}
	}
	if out, err := s.SortieRecoveryOutcome(start.ID); err != nil || out != nil {
		t.Fatal("cancel forged committed receipt", out, err)
	}
}

func TestRecoveryAppliedDeferredJournalEveryBoundary(t *testing.T) {
	for _, deferred := range []bool{false, true} {
		maxCut := 5
		if deferred {
			maxCut = 4
		}
		for cut := 0; cut <= maxCut; cut++ {
			t.Run(fmt.Sprintf("deferred%v-cut%d", deferred, cut), func(t *testing.T) {
				s, start, f, peer := miaEntry(t, false)
				p, err := ProposeSortie(start.PilotBefore, f, pendingTestTables())
				if err != nil {
					t.Fatal(err)
				}
				if err = s.StageSortieResult(start.ID, p); err != nil {
					t.Fatal(err)
				}
				if deferred {
					peerWrite(t, s, peer, false)
					if err = s.load(); err != nil {
						t.Fatal(err)
					}
				}
				doc, _ := s.currentSortie()
				next := s.State()
				player, err := ResolveSortieProposal(p, false)
				if err != nil {
					t.Fatal(err)
				}
				for i, pilot := range next.Slots {
					if pilot != nil && pilot.ID == start.PilotID {
						next.Slots[i] = player
					}
					if !deferred && pilot != nil && pilot.ID == peer {
						next.Slots[i] = cloneSortiePilot(p.RecoveredPilot.After)
					}
				}
				next.Revision++
				desired, _ := stateFiles(next)
				doc.Phase, doc.CommittedPilot = SortieCommitted, assetdata.Hash(desired[pilotFilename(start.PilotID)])
				doc.RecoveryOutcome = &MissionRecoveryOutcome{Policy: OptionalMissionRecoveryPolicy, BasisSHA256: p.RecoveryDecision.BasisSHA256, PilotID: peer, ExpectedSHA256: p.RecoveredPilot.BeforeSHA256}
				if deferred {
					doc.RecoveryOutcome.Status, doc.RecoveryOutcome.Reason, doc.RecoveryOutcome.ObservedSHA256 = "deferred", "peer-changed", assetdata.Hash(s.files[pilotFilename(peer)])
				} else {
					doc.CommittedRecovery = assetdata.Hash(desired[pilotFilename(peer)])
					doc.RecoveryOutcome.Status, doc.RecoveryOutcome.ObservedSHA256, doc.RecoveryOutcome.AppliedSHA256 = "applied", p.RecoveredPilot.BeforeSHA256, doc.CommittedRecovery
				}
				docRaw, _ := assetdata.JsonBytes(doc)
				backup, _ := assetdata.JsonBytes(pilotDocument{pilotSchemaVersion, start.PilotBefore})
				journal := saveCommit{SchemaVersion: 1, Operations: []saveOperation{{Path: pilotFilename(start.PilotID), Before: assetdata.Hash(s.files[pilotFilename(start.PilotID)]), After: desired[pilotFilename(start.PilotID)]}}}
				if !deferred {
					journal.Operations = append(journal.Operations, saveOperation{Path: pilotFilename(peer), Before: assetdata.Hash(s.files[pilotFilename(peer)]), After: desired[pilotFilename(peer)]})
				}
				journal.Operations = append(journal.Operations, []saveOperation{
					{Path: sortieFilename(start.ID), Before: assetdata.Hash(s.files[sortieFilename(start.ID)]), After: docRaw},
					{Path: saveFilename, Before: assetdata.Hash(s.raw), After: desired[saveFilename]},
					{Path: backupFilename(start.PilotID), Before: start.BackupSHA256, After: backup},
				}...)
				raw, _ := assetdata.JsonBytes(journal)
				if err = os.WriteFile(filepath.Join(s.Directory, saveJournal), raw, 0600); err != nil {
					t.Fatal(err)
				}
				for _, op := range journal.Operations[:cut] {
					if err = os.WriteFile(filepath.Join(s.Directory, op.Path), op.After, 0600); err != nil {
						t.Fatal(err)
					}
				}
				loaded, err := OpenSaveStore(s.Directory, "")
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(loaded.State().SelectedPilot(), player) {
					t.Fatal("journal core result")
				}
				if err = loaded.CommitSortie(start.ID); err != nil {
					t.Fatal(err)
				}
				out, err := loaded.SortieRecoveryOutcome(start.ID)
				if err != nil || out.Status != doc.RecoveryOutcome.Status {
					t.Fatal("journal outcome", out, err)
				}
			})
		}
	}
}
