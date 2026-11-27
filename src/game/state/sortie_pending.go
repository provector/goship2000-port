package state

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/game/flight"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
)

const sortieDocumentVersion = 6
const safeSortieDocumentVersion = 7

// SortieStart binds a native consume-once transaction to the accepted launch.
// Its identity is unrelated to gameplay RNG or the original SCORE mode byte.
type SortieStart struct {
	ID            string `json:"id"`
	PilotID       string `json:"pilot_id"`
	SourceProfile string `json:"source_profile"`
	MissionDigest string `json:"mission_sha256"`
	LaunchDigest  string `json:"launch_sha256"`
	ClockPolicy   string `json:"clock_policy"`
	PilotSHA256   string `json:"pilot_before_sha256"`
	BackupSHA256  string `json:"backup_sha256,omitempty"`
	// Read schema1 ownership for migration only. Launch no longer owns Phoenix.
	OwnBackup        bool                       `json:"owns_backup"`
	PilotBefore      *Pilot                     `json:"pilot_before"`
	ProposalVersion  int                        `json:"proposal_version,omitempty"` // Absent in historical N/N.4 starts means1.
	RecoveryRoster   []MissionRecoveryCandidate `json:"recovery_roster,omitempty"`
	RetireByMissions bool                       `json:"retire_by_missions,omitempty"`
	GameplayPolicy   string                     `json:"gameplay_policy,omitempty"`
}

type SortiePhase string

const (
	SortieActive      SortiePhase = "active"
	SortiePending     SortiePhase = "pending"
	SortieCommitted   SortiePhase = "committed"
	SortieUnsupported SortiePhase = "unsupported"
)

// PendingSortie is a cloned read model. Active means entry was retained but no
// complete flight snapshot/result exists; callers must not invent an outcome.
type PendingSortie struct {
	Start             SortieStart             `json:"start"`
	Phase             SortiePhase             `json:"phase"`
	Proposal          *SortieProposal         `json:"proposal,omitempty"`
	ResultSHA256      string                  `json:"result_sha256,omitempty"`
	CommissionChoice  *bool                   `json:"commission_choice,omitempty"`
	CrewChoices       *SortieCrewChoices      `json:"crew_choices,omitempty"`
	CommittedPilot    string                  `json:"committed_pilot_sha256,omitempty"`
	CommittedRecovery string                  `json:"committed_recovered_pilot_sha256,omitempty"`
	RecoveryOutcome   *MissionRecoveryOutcome `json:"recovery_outcome,omitempty"`
	UnsupportedReason string                  `json:"unsupported_reason,omitempty"`
}

type sortieDocument struct {
	SchemaVersion int `json:"schema_version"`
	PendingSortie
}

type sortieReference struct {
	SchemaVersion int    `json:"schema_version"`
	SortieID      string `json:"sortie_id"`
	PilotID       string `json:"pilot_id"`
}

func validSortieID(id string) bool {
	b, err := hex.DecodeString(id)
	return err == nil && len(b) == 16 && hex.EncodeToString(b) == id
}

func validSortieHash(hash string) bool {
	b, err := hex.DecodeString(hash)
	return err == nil && len(b) == 32 && hex.EncodeToString(b) == hash
}

func sortieFilename(id string) string          { return "sorties/" + id + ".json" }
func sortieReferenceFilename(id string) string { return "pilots/" + id + ".sortie.json" }

func isSortieTarget(name string) bool {
	if strings.HasPrefix(name, "sorties/") && strings.HasSuffix(name, ".json") {
		return validSortieID(strings.TrimSuffix(strings.TrimPrefix(name, "sorties/"), ".json"))
	}
	return false
}

func isSortieReferenceTarget(name string) bool {
	if strings.HasPrefix(name, "pilots/") && strings.HasSuffix(name, ".sortie.json") {
		return validPilotID(strings.TrimSuffix(strings.TrimPrefix(name, "pilots/"), ".sortie.json"))
	}
	return false
}

func validateSortieStart(start SortieStart) error {
	if !validGameplayPolicy(start.GameplayPolicy) {
		return fmt.Errorf("unsupported launch gameplay policy")
	}
	if start.ProposalVersion != 0 && start.ProposalVersion != 1 && start.ProposalVersion != 2 && start.ProposalVersion != 3 && start.ProposalVersion != SortieProposalVersion && start.ProposalVersion != SafeSortieProposalVersion {
		return fmt.Errorf("unsupported launch proposal interpretation")
	}
	if (start.GameplayPolicy == SafeGameplayPolicy) != (start.ProposalVersion == SafeSortieProposalVersion) {
		return fmt.Errorf("launch proposal version differs from gameplay policy")
	}
	if !validSortieID(start.ID) || !validPilotID(start.PilotID) || start.SourceProfile != assetdata.IslandIce469085.Name || start.ClockPolicy != flight.NativeClockPolicy {
		return fmt.Errorf("unsupported or invalid sortie identity/profile/clock")
	}
	if !validSortieHash(start.MissionDigest) || !validSortieHash(start.LaunchDigest) || !validSortieHash(start.PilotSHA256) {
		return fmt.Errorf("sortie identity needs complete mission/launch/pilot SHA256 values")
	}
	if start.BackupSHA256 != "" && !validSortieHash(start.BackupSHA256) || start.OwnBackup && start.BackupSHA256 == "" {
		return fmt.Errorf("invalid sortie recovery ownership")
	}
	if start.PilotBefore == nil || start.PilotBefore.ID != start.PilotID || start.PilotBefore.Status != StatusActive {
		return fmt.Errorf("sortie has no matching active pre-sortie pilot")
	}
	if start.PilotBefore.Statistics == nil || len(start.PilotBefore.Statistics.AwardCounts) != 8 || len(start.PilotBefore.LegacyRaw) != LegacyRecordSize {
		return fmt.Errorf("sortie entry lacks complete retained pilot progression ownership")
	}
	if start.PilotBefore.MissionType == MissionSimulated && start.ProposalVersion < 3 {
		return fmt.Errorf("historical launch cannot admit authored Simulation")
	}
	if !admittedSortieMission(start.PilotBefore.MissionType) {
		return fmt.Errorf("persistent sortie results require an admitted mission type")
	}
	if err := validateRecoveryCandidates(start.RecoveryRoster, start.PilotID); err != nil {
		return err
	}
	test := EmptySaveState()
	test.Slots[0] = start.PilotBefore
	return test.Validate()
}

func proposalDigest(proposal *SortieProposal) (string, error) {
	raw, err := assetdata.JsonBytes(proposal)
	if err != nil {
		return "", err
	}
	return assetdata.Hash(raw), nil
}

func validateSortieDocument(doc *sortieDocument) error {
	if doc.SchemaVersion != 1 && doc.SchemaVersion != 2 && doc.SchemaVersion != 3 && doc.SchemaVersion != 4 && doc.SchemaVersion != 5 && doc.SchemaVersion != sortieDocumentVersion && doc.SchemaVersion != safeSortieDocumentVersion {
		return fmt.Errorf("unsupported sortie record schema")
	}
	if err := validateSortieStart(doc.Start); err != nil {
		return err
	}
	if (doc.SchemaVersion == safeSortieDocumentVersion) != (doc.Start.GameplayPolicy == SafeGameplayPolicy) {
		return fmt.Errorf("sortie schema differs from gameplay policy")
	}
	if doc.SchemaVersion < 5 && doc.Start.ProposalVersion >= 3 {
		return fmt.Errorf("historical sortie schema cannot contain a later interpretation")
	}
	if doc.SchemaVersion == 4 && doc.Start.ProposalVersion != 2 || doc.SchemaVersion == 5 && doc.Start.ProposalVersion != 3 || doc.SchemaVersion == 6 && doc.Start.ProposalVersion != 4 || doc.SchemaVersion == 7 && doc.Start.ProposalVersion != 5 {
		return fmt.Errorf("sortie schema must pin its original proposal interpretation")
	}
	if doc.Start.ProposalVersion < 4 && doc.CrewChoices != nil && (len(doc.CrewChoices.Names) > 0 || doc.CrewChoices.Result != nil) {
		return fmt.Errorf("historical result cannot contain v4 replacement/crew continuation choices")
	}
	switch doc.Phase {
	case SortieActive, SortieUnsupported:
		if doc.Proposal != nil || doc.ResultSHA256 != "" || doc.CommissionChoice != nil || doc.CrewChoices != nil || doc.CommittedPilot != "" || doc.CommittedRecovery != "" {
			return fmt.Errorf("unfinished/unsupported sortie contains fabricated result state")
		}
		if doc.Phase == SortieActive && doc.UnsupportedReason != "" || doc.Phase == SortieUnsupported && (strings.TrimSpace(doc.UnsupportedReason) == "" || len(doc.UnsupportedReason) > 2048) {
			return fmt.Errorf("invalid unfinished sortie explanation")
		}
	case SortiePending, SortieCommitted:
		if doc.Proposal == nil || doc.UnsupportedReason != "" || !reflect.DeepEqual(doc.Proposal.PilotBefore, doc.Start.PilotBefore) {
			return fmt.Errorf("pending result differs from its accepted pilot snapshot")
		}
		if doc.Proposal.Facts.SortieID != doc.Start.ID || doc.Proposal.Facts.SourceProfile != doc.Start.SourceProfile || doc.Proposal.Facts.MissionType != doc.Start.PilotBefore.MissionType {
			return fmt.Errorf("pending result differs from its accepted sortie identity")
		}
		if err := ValidateSortieProposal(doc.Proposal); err != nil {
			return fmt.Errorf("invalid pending result: %w", err)
		}
		expectedVersion := doc.Start.ProposalVersion
		if expectedVersion == 0 {
			expectedVersion = 1
		}
		if expectedVersion >= 2 && len(doc.Start.RecoveryRoster) > 0 && (doc.Proposal.Facts.ResultContext == nil || doc.Proposal.Facts.ResultContext.MissionRecovery == nil) {
			return fmt.Errorf("accepted MIA roster requires its immutable launch recovery context")
		}
		if doc.Proposal.Version != expectedVersion {
			return fmt.Errorf("result proposal version differs from accepted launch")
		}
		policy := ""
		if ctx := doc.Proposal.Facts.ResultContext; ctx != nil {
			policy = ctx.GameplayPolicy
		}
		if policy != doc.Start.GameplayPolicy || doc.Start.RetireByMissions != (doc.Proposal.Facts.ResultContext != nil && doc.Proposal.Facts.ResultContext.RetireByMissions) {
			return fmt.Errorf("result gameplay policy differs from accepted launch")
		}
		if context := doc.Proposal.Facts.ResultContext; context != nil && context.MissionRecovery != nil && !reflect.DeepEqual(context.MissionRecovery.Candidates, doc.Start.RecoveryRoster) {
			return fmt.Errorf("mission recovery candidates differ from accepted launch roster")
		}
		hash, err := proposalDigest(doc.Proposal)
		if err != nil || hash != doc.ResultSHA256 {
			return fmt.Errorf("pending result digest mismatch")
		}
		if !doc.Proposal.Commission && doc.CommissionChoice != nil {
			return fmt.Errorf("sortie has a commission choice without a commission offer")
		}
		if doc.Proposal.Version >= 4 && doc.CrewChoices != nil && doc.Proposal.Result != nil && doc.CrewChoices.Result == nil {
			return fmt.Errorf("pending crew choices require their frozen source continuation")
		}
		if _, err := ResolveSortieProposalWithChoices(doc.Proposal, doc.CommissionChoice != nil && *doc.CommissionChoice, doc.CrewChoices); err != nil {
			return err
		}
		if doc.Phase == SortiePending {
			if doc.CommittedPilot != "" || doc.CommittedRecovery != "" {
				return fmt.Errorf("pending result incorrectly claims a career commit")
			}
		} else {
			if doc.Proposal.Commission && doc.CommissionChoice == nil {
				return fmt.Errorf("committed result is missing its player decision")
			}
			accepted := doc.CommissionChoice != nil && *doc.CommissionChoice
			pilot, err := ResolveSortieProposalWithChoices(doc.Proposal, accepted, doc.CrewChoices)
			if err != nil {
				return err
			}
			raw, err := assetdata.JsonBytes(pilotDocument{pilotSchemaVersion, pilot})
			if err != nil || assetdata.Hash(raw) != doc.CommittedPilot {
				return fmt.Errorf("committed result pilot digest mismatch")
			}
			if recovered := doc.Proposal.RecoveredPilot; recovered != nil && doc.Start.ProposalVersion < SafeSortieProposalVersion {
				raw, err := assetdata.JsonBytes(pilotDocument{pilotSchemaVersion, recovered.After})
				if err != nil || assetdata.Hash(raw) != doc.CommittedRecovery {
					return fmt.Errorf("committed recovered-pilot digest mismatch")
				}
			} else if doc.CommittedRecovery != "" && doc.Start.ProposalVersion < SafeSortieProposalVersion {
				return fmt.Errorf("committed result has an unrelated recovered-pilot digest")
			}
		}
	default:
		return fmt.Errorf("unknown sortie record phase %q", doc.Phase)
	}
	return validateRecoveryOutcome(doc)
}

func decodeSortieRecord(raw []byte, id string) (*sortieDocument, error) {
	var doc sortieDocument
	if err := assetdata.DecodeObject(raw, &doc); err != nil {
		return nil, fmt.Errorf("invalid sortie record: %w", err)
	}
	if doc.Start.ID != id {
		return nil, fmt.Errorf("sortie record identity differs from its filename")
	}
	if err := validateSortieDocument(&doc); err != nil {
		return nil, err
	}
	return &doc, nil
}

func decodeSortieReference(raw []byte, pilotID string) (*sortieReference, error) {
	var ref sortieReference
	if err := assetdata.DecodeObject(raw, &ref); err != nil || ref.SchemaVersion != 1 && ref.SchemaVersion != 2 && ref.SchemaVersion != 3 && ref.SchemaVersion != 4 && ref.SchemaVersion != 5 && ref.SchemaVersion != sortieDocumentVersion && ref.SchemaVersion != safeSortieDocumentVersion || ref.PilotID != pilotID || !validSortieID(ref.SortieID) {
		return nil, fmt.Errorf("invalid pilot sortie reference")
	}
	return &ref, nil
}

// loadSortieFiles validates restart state before any new save mutation. A missing
// referenced result is never interpreted as an empty score or a new sortie.
func (s *SaveStore) loadSortieFiles() error {
	for _, dir := range []string{"sorties"} {
		if info, err := os.Lstat(filepath.Join(s.Directory, dir)); err == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
			return fmt.Errorf("%s must be a non-symlink directory", dir)
		} else if err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	referenced := map[string]bool{}
	for _, pilot := range s.state.Slots {
		if pilot == nil {
			continue
		}
		name := sortieReferenceFilename(pilot.ID)
		raw, err := s.currentRecord(name)
		if err != nil {
			return err
		}
		if raw == nil {
			continue
		}
		ref, err := decodeSortieReference(raw, pilot.ID)
		if err != nil {
			return err
		}
		file := sortieFilename(ref.SortieID)
		resultRaw, err := s.readRecord(file)
		if err != nil {
			return fmt.Errorf("referenced sortie %s is unavailable; preserve saves before repair: %w", ref.SortieID, err)
		}
		doc, err := decodeSortieRecord(resultRaw, ref.SortieID)
		if err != nil || doc.Start.PilotID != pilot.ID {
			return fmt.Errorf("invalid referenced sortie %s: %v", ref.SortieID, err)
		}
		if doc.Phase == SortieActive || doc.Phase == SortiePending {
			if s.state.SelectedPilotID != pilot.ID || assetdata.Hash(s.files[pilotFilename(pilot.ID)]) != doc.Start.PilotSHA256 || !reflect.DeepEqual(doc.Start.PilotBefore, pilot) {
				return fmt.Errorf("pending sortie pilot was changed externally; no career result was applied")
			}
		}
		s.files[name], s.files[file] = raw, resultRaw
		referenced[ref.SortieID] = true
	}
	entries, err := s.sortieNames()
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		// A process can stop between atomic-write CreateTemp and Rename. Such
		// retained temporary bytes are not a result and must never be imported.
		if strings.HasPrefix(entry, ".save-pending-") && strings.HasSuffix(entry, ".json") {
			continue
		}
		id := strings.TrimSuffix(entry, ".json")
		if !validSortieID(id) || entry != id+".json" {
			return fmt.Errorf("unknown file in sortie result directory; preserve it before repair")
		}
		if referenced[id] {
			continue
		}
		raw, err := s.readRecord("sorties/" + entry)
		if err != nil {
			return err
		}
		doc, err := decodeSortieRecord(raw, id)
		if err != nil {
			return fmt.Errorf("invalid retained sortie %s: %w", id, err)
		}
		if doc.Phase == SortieActive || doc.Phase == SortiePending {
			return fmt.Errorf("unfinished sortie %s lost its pilot reference; no result was reset", id)
		}
	}
	return nil
}

func (s *SaveStore) currentSortie() (*sortieDocument, error) {
	pilot := s.state.SelectedPilot()
	if pilot == nil {
		return nil, nil
	}
	raw := s.files[sortieReferenceFilename(pilot.ID)]
	if raw == nil {
		return nil, nil
	}
	ref, err := decodeSortieReference(raw, pilot.ID)
	if err != nil {
		return nil, err
	}
	return decodeSortieRecord(s.files[sortieFilename(ref.SortieID)], ref.SortieID)
}

func (s *SaveStore) checkSortieSessionFiles() error {
	for name, expected := range s.files {
		actual, err := s.readRecord(name)
		if err != nil {
			return err
		}
		if !bytes.Equal(actual, expected) {
			return fmt.Errorf("save changed outside this session (%s); pending sortie preserved", name)
		}
	}
	return nil
}

func (s *SaveStore) guardPendingSortie() error {
	for name, raw := range s.files {
		if !isSortieReferenceTarget(name) {
			continue
		}
		pilotID := strings.TrimSuffix(strings.TrimPrefix(name, "pilots/"), ".sortie.json")
		ref, err := decodeSortieReference(raw, pilotID)
		if err != nil {
			return err
		}
		doc, err := decodeSortieRecord(s.files[sortieFilename(ref.SortieID)], ref.SortieID)
		if err != nil {
			return err
		}
		if doc.Phase == SortieActive || doc.Phase == SortiePending {
			return fmt.Errorf("sortie %s is %s; resolve or explicitly abandon it before editing/recovering this save", doc.Start.ID, doc.Phase)
		}
	}
	return nil
}

// BeginSortie is called only after accepted preflight settings were committed.
// It creates a native transaction identity, not a DOS result or replay record.
func (s *SaveStore) BeginSortie(request SortieStart) (*SortieStart, error) {
	pilot := s.state.SelectedPilot()
	if pilot == nil || pilot.Status != StatusActive || pilot.Statistics == nil || !admittedSortieMission(pilot.MissionType) {
		return nil, fmt.Errorf("persistent sortie entry requires a supported active pilot")
	}
	if request.SourceProfile != assetdata.IslandIce469085.Name || request.ClockPolicy != flight.NativeClockPolicy || !validSortieHash(request.MissionDigest) || !validSortieHash(request.LaunchDigest) {
		return nil, fmt.Errorf("sortie entry needs recognized profile, clock and accepted mission/launch digests")
	}
	if err := s.checkSortieSessionFiles(); err != nil {
		return nil, err
	}
	current, err := s.currentSortie()
	if err != nil {
		return nil, err
	}
	if current != nil && (current.Phase == SortieActive || current.Phase == SortiePending) {
		if current.Phase == SortieActive && current.Start.SourceProfile == request.SourceProfile && current.Start.ClockPolicy == request.ClockPolicy && current.Start.MissionDigest == request.MissionDigest && current.Start.LaunchDigest == request.LaunchDigest && current.Start.GameplayPolicy == request.GameplayPolicy && current.Start.RetireByMissions == request.RetireByMissions {
			return &current.Start, nil
		}
		return nil, fmt.Errorf("a pending sortie already owns this pilot; no new entry was created")
	}
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return nil, fmt.Errorf("sortie identifier: %w", err)
	}
	request.ID, request.PilotID = hex.EncodeToString(id[:]), pilot.ID
	request.PilotBefore = cloneSave(s.state).SelectedPilot()
	// Existing save commits normalize JSON formatting. Bind the accepted typed
	// pilot to the bytes this same journal will publish. The prior completed
	// mission's Phoenix backup stays untouched until a final result is committed.
	files, err := stateFiles(s.State())
	if err != nil {
		return nil, err
	}
	request.PilotSHA256 = assetdata.Hash(files[pilotFilename(pilot.ID)])
	request.ProposalVersion = SortieProposalVersion
	schema := sortieDocumentVersion
	if request.GameplayPolicy == SafeGameplayPolicy {
		request.ProposalVersion, schema = SafeSortieProposalVersion, safeSortieDocumentVersion
	}
	request.RecoveryRoster = sourceRecoveryRoster(s.State(), files)
	backupRaw, err := s.currentRecord(backupFilename(pilot.ID))
	if err != nil {
		return nil, err
	}
	extra := map[string]*[]byte{}
	request.OwnBackup = false
	if backupRaw != nil {
		if _, err := decodePilot(backupRaw, pilot.ID); err != nil {
			return nil, fmt.Errorf("existing recovery backup is invalid; nothing replaced: %w", err)
		}
		request.BackupSHA256 = assetdata.Hash(backupRaw)
	} else {
		request.BackupSHA256 = ""
	}
	doc := &sortieDocument{SchemaVersion: schema, PendingSortie: PendingSortie{Start: request, Phase: SortieActive}}
	if err := validateSortieDocument(doc); err != nil {
		return nil, err
	}
	raw, err := assetdata.JsonBytes(doc)
	if err != nil {
		return nil, err
	}
	ref, err := assetdata.JsonBytes(sortieReference{schema, request.ID, pilot.ID})
	if err != nil {
		return nil, err
	}
	extra[sortieFilename(request.ID)] = &raw
	extra[sortieReferenceFilename(pilot.ID)] = &ref
	if err := s.store(s.State(), extra); err != nil {
		return nil, err
	}
	loaded, err := s.currentSortie()
	if err != nil {
		return nil, err
	}
	return &loaded.Start, nil
}

func (s *SaveStore) PendingSortie() (*PendingSortie, error) {
	doc, err := s.currentSortie()
	if err != nil || doc == nil {
		return nil, err
	}
	if doc.Start.ProposalVersion == SafeSortieProposalVersion && doc.Proposal != nil && doc.Proposal.RecoveredPilot != nil {
		err = s.Locked(func() error {
			view, _, e := s.optionalRecoveryViewLocked(doc)
			if e == nil {
				s.adoptSortieView(view)
			}
			return e
		})
	} else {
		err = s.checkSortieSessionFiles()
	}
	if err != nil {
		return nil, err
	}
	if doc.Phase != SortieActive && doc.Phase != SortiePending {
		return nil, nil
	}
	return &doc.PendingSortie, nil
}

func (s *SaveStore) StageSortieResult(id string, proposal *SortieProposal) error {
	if proposal == nil || proposal.Version != SafeSortieProposalVersion {
		if err := s.checkSortieSessionFiles(); err != nil {
			return err
		}
	}
	doc, err := s.currentSortie()
	if err != nil || doc == nil || doc.Start.ID != id {
		return fmt.Errorf("sortie result has no matching accepted entry: %v", err)
	}
	if err := ValidateSortieProposal(proposal); err != nil {
		return err
	}
	hash, err := proposalDigest(proposal)
	if err != nil {
		return err
	}
	if doc.Phase == SortiePending || doc.Phase == SortieCommitted {
		if hash == doc.ResultSHA256 {
			if proposal.Version == SafeSortieProposalVersion {
				_, err := s.PendingSortie()
				return err
			}
			return nil
		}
		return fmt.Errorf("sortie already has a different frozen result; no outcome was replaced")
	}
	if doc.Phase != SortieActive || !reflect.DeepEqual(doc.Start.PilotBefore, proposal.PilotBefore) {
		return fmt.Errorf("sortie result does not match its immutable accepted pilot")
	}
	// JSON cloning prevents the caller's later entity/UI changes from mutating the
	// frozen proposal. No projectile drain or subsequent scoring occurs here.
	rawProposal, err := json.Marshal(proposal)
	if err != nil {
		return err
	}
	var frozen SortieProposal
	if err := json.Unmarshal(rawProposal, &frozen); err != nil {
		return err
	}
	doc.Phase, doc.Proposal, doc.ResultSHA256 = SortiePending, &frozen, hash
	return s.storeSortieDocument(doc)
}

func (s *SaveStore) SetSortieCommission(id string, accepted bool) error {
	doc, err := s.currentSortie()
	if err != nil || doc == nil || doc.Start.ID != id || doc.Phase != SortiePending || !doc.Proposal.Commission {
		return fmt.Errorf("sortie has no pending commission decision")
	}
	if doc.CommissionChoice != nil && *doc.CommissionChoice == accepted {
		if doc.Start.ProposalVersion == SafeSortieProposalVersion {
			_, err := s.PendingSortie()
			return err
		}
		return s.checkSortieSessionFiles()
	}
	doc.CommissionChoice = &accepted
	return s.storeSortieDocument(doc)
}

// SetSortieCrewChoices persists only the current pending UI decision. The
// proposal and ordered RNG decisions remain immutable; final acknowledgement
// publishes all player/crew/Campaign bytes through the existing one journal.
func (s *SaveStore) SetSortieCrewChoices(id string, choices SortieCrewChoices) error {
	doc, err := s.currentSortie()
	if err != nil || doc == nil || doc.Start.ID != id || doc.Phase != SortiePending {
		return fmt.Errorf("sortie has no current pending crew choice")
	}
	accepted := doc.CommissionChoice != nil && *doc.CommissionChoice
	if _, err := ResolveSortieProposalWithChoices(doc.Proposal, accepted, &choices); err != nil {
		return err
	}
	copyChoices := choices
	copyChoices.Recipients = append([]int(nil), choices.Recipients...)
	copyChoices.Names = append([]string(nil), choices.Names...)
	if doc.Proposal.Version >= 4 {
		result, err := resolveSortieResultForChoices(doc.Proposal, accepted, &copyChoices)
		if err != nil {
			return err
		}
		if result != nil {
			raw, err := json.Marshal(result)
			if err != nil {
				return err
			}
			copyChoices.Result = new(SortieResultDetails)
			if err = json.Unmarshal(raw, copyChoices.Result); err != nil {
				return err
			}
		}
	}
	doc.CrewChoices = &copyChoices
	return s.storeSortieDocument(doc)
}

func admittedSortieMission(t MissionType) bool {
	return t == MissionTraining || t == MissionSingle || t == MissionFlight || t == MissionCampaign || t == MissionSimulated
}

func (s *SaveStore) storeSortieDocument(doc *sortieDocument) error {
	if err := validateSortieDocument(doc); err != nil {
		return err
	}
	raw, err := assetdata.JsonBytes(doc)
	if err != nil {
		return err
	}
	extra := map[string]*[]byte{sortieFilename(doc.Start.ID): &raw}
	if doc.Start.ProposalVersion == SafeSortieProposalVersion {
		return s.Locked(func() error {
			view, preserve, err := s.optionalRecoveryViewLocked(doc)
			if err != nil {
				return err
			}
			for path, bytes := range preserve {
				copy := bytes
				extra[path] = &copy
			}
			if err = view.storeCheckedLocked(view.State(), extra, "", ""); err != nil {
				return err
			}
			s.adoptSortieView(view)
			return nil
		})
	}
	return s.storeChecked(s.State(), extra, "", "")
}

// CommitSortie materializes the already-frozen proposal once. Resuming the
// journal completes exact desired bytes; it never reruns score/award RNG.
func (s *SaveStore) CommitSortie(id string) error {
	return s.Locked(func() error {
		doc, err := s.currentSortie()
		if err != nil {
			return err
		}
		view, preserve, err := s.optionalRecoveryViewLocked(doc)
		if err != nil {
			return err
		}
		if err = view.commitSortieLocked(id, preserve); err != nil {
			return err
		}
		s.adoptSortieView(view)
		return nil
	})
}

func (s *SaveStore) commitSortieLocked(id string, preserve map[string][]byte) error {
	if err := s.checkSortieSessionFiles(); err != nil {
		return err
	}
	doc, err := s.currentSortie()
	if err != nil {
		return err
	}
	if doc == nil || doc.Start.ID != id {
		// A late duplicate can arrive after a later sortie has become current.
		// Read the immutable historical marker; never put its pilot back on disk.
		if validSortieID(id) {
			raw, e := s.currentRecord(sortieFilename(id))
			if e != nil {
				return e
			}
			if raw != nil {
				historical, e := decodeSortieRecord(raw, id)
				if e != nil {
					return e
				}
				pilot := s.state.SelectedPilot()
				if historical.Phase == SortieCommitted && pilot != nil && historical.Start.PilotID == pilot.ID {
					return nil
				}
			}
		}
		return fmt.Errorf("sortie has no matching pending record")
	}
	if doc.Phase == SortieCommitted {
		return nil
	}
	if doc.Phase != SortiePending || doc.Proposal == nil || doc.Proposal.Commission && doc.CommissionChoice == nil {
		return fmt.Errorf("sortie result/required player decision is incomplete")
	}
	accepted := doc.CommissionChoice != nil && *doc.CommissionChoice
	pilot, err := ResolveSortieProposalWithChoices(doc.Proposal, accepted, doc.CrewChoices)
	if err != nil {
		return err
	}
	next := s.State()
	for i, old := range next.Slots {
		if old != nil && old.ID == doc.Start.PilotID {
			next.Slots[i] = pilot
		}
	}
	// This status-only update shares the player's final-ack journal. The other
	// pilot's own Phoenix and consumed result reference are deliberately retained.
	if doc.Start.ProposalVersion == SafeSortieProposalVersion {
		doc.RecoveryOutcome = recoveryOutcomeForProposal(doc.Proposal)
	}
	if recovery := doc.Proposal.RecoveredPilot; recovery != nil {
		found := false
		for i, old := range next.Slots {
			if old != nil && old.ID == recovery.Before.ID {
				raw, e := assetdata.JsonBytes(pilotDocument{pilotSchemaVersion, old})
				if e != nil {
					return e
				}
				currentHash := assetdata.Hash(raw)
				if doc.RecoveryOutcome != nil {
					currentHash = assetdata.Hash(s.files[pilotFilename(old.ID)])
				}
				if currentHash != recovery.BeforeSHA256 {
					if doc.RecoveryOutcome != nil {
						doc.RecoveryOutcome.Status, doc.RecoveryOutcome.Reason, doc.RecoveryOutcome.ObservedSHA256 = "deferred", "peer-changed", assetdata.Hash(s.files[pilotFilename(old.ID)])
						found = true
						break
					}
					return fmt.Errorf("mission recovery pilot changed since launch")
				}
				next.Slots[i], found = cloneSortiePilot(recovery.After), true
			}
		}
		if !found {
			if doc.RecoveryOutcome != nil {
				doc.RecoveryOutcome.Status, doc.RecoveryOutcome.Reason = "deferred", "peer-removed"
			} else {
				return fmt.Errorf("mission recovery pilot is no longer in the roster")
			}
		}
		if doc.RecoveryOutcome == nil || doc.RecoveryOutcome.Status != "deferred" {
			raw, e := assetdata.JsonBytes(pilotDocument{pilotSchemaVersion, recovery.After})
			if e != nil {
				return e
			}
			doc.CommittedRecovery = assetdata.Hash(raw)
			if doc.RecoveryOutcome != nil {
				doc.RecoveryOutcome.Status, doc.RecoveryOutcome.ObservedSHA256, doc.RecoveryOutcome.AppliedSHA256 = "applied", recovery.BeforeSHA256, doc.CommittedRecovery
			}
		}
	}
	pilotRaw, err := assetdata.JsonBytes(pilotDocument{pilotSchemaVersion, pilot})
	if err != nil {
		return err
	}
	doc.Phase, doc.CommittedPilot = SortieCommitted, assetdata.Hash(pilotRaw)
	if err := validateSortieDocument(doc); err != nil {
		return err
	}
	raw, err := assetdata.JsonBytes(doc)
	if err != nil {
		return err
	}
	extra := map[string]*[]byte{sortieFilename(id): &raw}
	for path, bytes := range preserve {
		copy := bytes
		extra[path] = &copy
	}
	// Final acknowledgement is the native commit boundary. Publish the accepted
	// pre-result pilot as Phoenix in the same durable journal as career/consumed
	// identity; launch/abandonment never replace the previous recovery snapshot.
	recoveryRaw, err := assetdata.JsonBytes(pilotDocument{pilotSchemaVersion, doc.Start.PilotBefore})
	if err != nil {
		return err
	}
	extra[backupFilename(doc.Start.PilotID)] = &recoveryRaw
	return s.storeCheckedLocked(next, extra, backupFilename(doc.Start.PilotID), doc.Start.BackupSHA256)
}

// AbandonSortie discards only optional uncommitted session ownership. Canonical
// pilot and Phoenix bytes are never rolled back or consumed by abandonment.
func (s *SaveStore) AbandonSortie(id, reason string) error {
	return s.Locked(func() error {
		if err := resumeSaveCommit(s.Directory); err != nil {
			return err
		}
		if err := s.checkSortieSessionFiles(); err != nil {
			return err
		}
		doc, err := s.currentSortie()
		if err != nil || doc == nil || doc.Start.ID != id {
			return fmt.Errorf("no matching sortie to abandon")
		}
		if doc.Phase == SortieCommitted {
			return fmt.Errorf("a committed sortie cannot be abandoned or reapplied")
		}
		if err = s.quarantine(reason); err != nil {
			return err
		}
		return s.load()
	})
}

// PreserveUnresolvedSortie retains a diagnostic result snapshot independently
// of the active-session pointer. It carries no award or commit authority.
func (s *SaveStore) PreserveUnresolvedSortie(id string, raw json.RawMessage) error {
	return s.Locked(func() error {
		doc, err := s.currentSortie()
		if err != nil {
			return err
		}
		if doc == nil || doc.Start.ID != id || doc.Phase != SortieActive || !json.Valid(raw) {
			return fmt.Errorf("unresolved result has no matching active sortie")
		}
		dir := filepath.Join(s.Directory, "unresolved-results")
		if err = os.MkdirAll(dir, 0700); err != nil {
			return err
		}
		info, err := os.Lstat(dir)
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return fmt.Errorf("invalid unresolved results directory")
		}
		name := id + ".json"
		if prior, err := ReadRegularSave(filepath.Join(dir, name)); err == nil {
			if !bytes.Equal(prior, raw) {
				return fmt.Errorf("different unresolved evidence already retained")
			}
		} else if !os.IsNotExist(err) {
			return err
		} else if err = writeSaveAtomic(dir, name, raw); err != nil {
			return err
		}
		// Persist both the new entry and, on first use, its directory. Windows
		// uses the existing file-sync/rename policy used for managed films.
		if runtime.GOOS != "windows" {
			for _, path := range []string{dir, s.Directory} {
				d, err := os.Open(path)
				if err != nil {
					return err
				}
				err = d.Sync()
				closeErr := d.Close()
				if err != nil {
					return err
				}
				if closeErr != nil {
					return closeErr
				}
			}
		}
		return nil
	})
}

// ArchiveRejectedSortie runs the existing journal checks, evidence retention and
// abandonment against private read caches. The caller owns the live career and
// suspends all other mutations until it adopts the result on its UI thread.
// Observers can still read the old immutable roster while the disk work runs.
func (s *SaveStore) ArchiveRejectedSortie(id, reason string, evidence json.RawMessage) (func(), error) {
	worker := *s
	worker.liveOwner = false // worker only publishes the staged career transaction
	if s.packed != nil {
		worker.packed = &careerPacks{dir: s.packed.dir, cfg: s.packed.cfg, id: s.packed.id, root: s.Directory}
	}
	if len(evidence) != 0 {
		if err := worker.PreserveUnresolvedSortie(id, evidence); err != nil {
			return nil, err
		}
	}
	if err := worker.AbandonSortie(id, reason); err != nil {
		return nil, err
	}
	return func() { s.state, s.raw, s.files, s.packed = worker.state, worker.raw, worker.files, worker.packed }, nil
}
