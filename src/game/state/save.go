package state

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/game/configuration"
	"gunship2000-assets/internal/userpack"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var careerWrites sync.Mutex

const saveSchemaVersion = 1

const saveFilename = "roster.json"

const oldSaveFilename = "office-save.json"

const rosterSchemaVersion = 2

const pilotSchemaVersion = 1

const LegacyRecordSize = 290

type Rank string

type PilotStatus string

type MissionType string

type Theatre string

const (
	RankWOC               Rank        = "warrant_officer_candidate"
	RankWO1               Rank        = "warrant_officer_1"
	RankCW2               Rank        = "chief_warrant_officer_2"
	RankCW3               Rank        = "chief_warrant_officer_3"
	RankCW4               Rank        = "chief_warrant_officer_4"
	RankSecondLieutenant  Rank        = "second_lieutenant"
	RankFirstLieutenant   Rank        = "first_lieutenant"
	RankCaptain           Rank        = "captain"
	RankMajor             Rank        = "major"
	RankLieutenantColonel Rank        = "lieutenant_colonel"
	RankColonel           Rank        = "colonel"
	RankBrigadierGeneral  Rank        = "brigadier_general"
	StatusInactive        PilotStatus = "inactive"
	StatusActive          PilotStatus = "active"
	StatusRetired         PilotStatus = "retired"
	StatusMIA             PilotStatus = "mia"
	StatusKIA             PilotStatus = "kia"
	StatusLegacyUnknown   PilotStatus = "legacy_unknown"
	MissionTraining       MissionType = "training"
	MissionSingle         MissionType = "single"
	MissionSimulated      MissionType = "simulated"
	MissionFlight         MissionType = "flight"
	MissionCampaign       MissionType = "campaign"
	TheatreCentralEurope  Theatre     = "central_europe"
	TheatrePersianGulf    Theatre     = "persian_gulf"
	TheatreAntarctica     Theatre     = "antarctica"
	TheatrePhilippines    Theatre     = "philippines"
)

var saveRanks = []Rank{RankWOC, RankWO1, RankCW2, RankCW3, RankCW4, RankSecondLieutenant, RankFirstLieutenant, RankCaptain, RankMajor, RankLieutenantColonel, RankColonel, RankBrigadierGeneral}

var saveMissions = []MissionType{MissionTraining, MissionSingle, MissionSimulated, MissionFlight, MissionCampaign}

var saveTheatres = []Theatre{TheatreCentralEurope, TheatrePersianGulf, TheatreAntarctica, TheatrePhilippines}

func enumIndex[T ~string](values []T, v T) int {
	for i, known := range values {
		if known == v {
			return i
		}
	}
	return -1
}

func (v Rank) Raw() int { return enumIndex(saveRanks, v) }

func (v MissionType) Raw() int { return enumIndex(saveMissions, v) }

func (v Theatre) Raw() int { return enumIndex(saveTheatres, v) }

func RankFromRaw(v int) (Rank, error) {
	if v < 0 || v >= len(saveRanks) {
		return "", fmt.Errorf("unsupported rank byte %d", v)
	}
	return saveRanks[v], nil
}

func MissionTypeFromRaw(v int) (MissionType, error) {
	if v < 0 || v >= len(saveMissions) {
		return "", fmt.Errorf("unsupported mission byte %d", v)
	}
	return saveMissions[v], nil
}

func TheatreFromRaw(v int) (Theatre, error) {
	if v < 0 || v >= len(saveTheatres) {
		return "", fmt.Errorf("unsupported theatre row %d", v)
	}
	return saveTheatres[v], nil
}

type PilotStatistics struct {
	// Original signed-byte award counters; nil means the optional legacy state is unknown.
	AwardCounts            []int8 `json:"award_counters,omitempty"`
	LastMissionScore       int32  `json:"last_mission_score"`
	BestMissionScore       int32  `json:"best_mission_score"`
	CareerScore            int32  `json:"career_score"`
	MissionCampaignCounter int32  `json:"mission_campaign_counter"`
	// The original decrements this signed accumulator on recovery.
	PhoenixCounter int32 `json:"phoenix_counter"`
}

func (s PilotStatistics) Missions() int32 { return s.MissionCampaignCounter % 100 }

func (s PilotStatistics) Campaigns() int32 { return s.MissionCampaignCounter / 100 }

func (s PilotStatistics) PhoenixCount() int16 { return -int16(s.PhoenixCounter) }

type Pilot struct {
	Campaign       *CampaignState     `json:"campaign,omitempty"`
	SquadronPilots []SquadronPilot    `json:"squadron_pilots,omitempty"`
	Preflight      *PreflightSettings `json:"preflight_settings,omitempty"`
	Statistics     *PilotStatistics   `json:"statistics"`
	ID             string             `json:"id"`
	Name           string             `json:"name"`
	Squadron       string             `json:"squadron"`
	Emblem         int                `json:"emblem"`
	Rank           Rank               `json:"rank"`
	Status         PilotStatus        `json:"status"`
	MissionType    MissionType        `json:"mission_type"`
	Theatre        Theatre            `json:"theatre"`
	LegacyStatus   byte               `json:"legacy_status_byte,omitempty"`
	// Opaque original state is retained, not treated as a complete save specification.
	LegacyRaw []byte `json:"legacy_record_base64,omitempty"`
}

func NewPilot() (*Pilot, error) {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return nil, fmt.Errorf("pilot identifier: %w", err)
	}
	// The original new-entry path clears its unimplemented fields. This zero
	// witness supports its +4f eligibility branch; typed fields below remain
	// authoritative and are never written back into a fabricated legacy file.
	preferences := defaultPreflightSettings()
	return &Pilot{Preflight: &preferences, Statistics: &PilotStatistics{AwardCounts: make([]int8, 8)}, ID: hex.EncodeToString(id[:]), Rank: RankWOC, Status: StatusActive, MissionType: MissionTraining, Theatre: TheatrePersianGulf, LegacyRaw: make([]byte, LegacyRecordSize)}, nil
}

func (p *Pilot) LegacyByte(offset int) (byte, bool) {
	if p == nil || offset < 0 || offset >= len(p.LegacyRaw) {
		return 0, false
	}
	return p.LegacyRaw[offset], true
}

type LegacyImport struct {
	SourcePath       string `json:"source_path"`
	SourceSHA256     string `json:"source_sha256"`
	SupportedProfile string `json:"supported_profile"`
	PreservedPath    string `json:"preserved_path"`
	SelectionPolicy  string `json:"selection_policy"`
	RawRoster        []byte `json:"raw_roster_base64"`
	RecoveryRecord   []byte `json:"recovery_record_base64"`
}

type SaveState struct {
	SchemaVersion   int           `json:"schema_version"`
	Revision        uint64        `json:"revision"`
	Slots           [6]*Pilot     `json:"roster_slots"`
	SelectedPilotID string        `json:"selected_pilot_id,omitempty"`
	Legacy          *LegacyImport `json:"legacy_import,omitempty"`
	// Read only for Step C migration. Split-file saves never persist undo history.
	RecoveryBefore *uint64 `json:"recovery_before_revision,omitempty"`
}

func EmptySaveState() SaveState { return SaveState{SchemaVersion: saveSchemaVersion} }

func (s SaveState) SelectedPilot() *Pilot {
	for _, p := range s.Slots {
		if p != nil && p.ID == s.SelectedPilotID {
			return p
		}
	}
	return nil
}

func (s SaveState) OfficeState() OfficeState {
	// No selection uses the original empty-record display values, not the named
	// research fixture. A newly created pilot separately defaults to Gulf duty.
	o := OfficeState{}
	if p := s.SelectedPilot(); p != nil {
		o.PilotName = p.Name
		o.Squadron = p.Squadron
		o.Rank = p.Rank.Raw()
		o.Emblem = p.Emblem
		o.MissionType = p.MissionType.Raw()
		o.Theatre = p.Theatre.Raw()
		o.Active = p.Status == StatusActive
	}
	return o
}

func validSaveText(name, value string, limit int) error {
	if len(value) > limit {
		return fmt.Errorf("%s exceeds %d characters", name, limit)
	}
	for _, c := range []byte(value) {
		if c < 32 || c > 127 {
			return fmt.Errorf("%s requires original text bytes 32..127", name)
		}
	}
	return nil
}

func (s SaveState) Validate() error {
	if s.SchemaVersion != saveSchemaVersion {
		return fmt.Errorf("save schema %d is unsupported (expected %d); preserve this save and use a compatible port", s.SchemaVersion, saveSchemaVersion)
	}
	ids := map[string]bool{}
	for i, p := range s.Slots {
		if p == nil {
			continue
		}
		if !validPilotID(p.ID) || ids[p.ID] {
			return fmt.Errorf("roster slot %d has empty, duplicate or oversized pilot ID", i+1)
		}
		ids[p.ID] = true
		if strings.TrimSpace(p.Name) == "" {
			return fmt.Errorf("roster slot %d has no pilot name", i+1)
		}
		if err := validSaveText("pilot name", p.Name, 35); err != nil {
			return err
		}
		if err := validSaveText("squadron", p.Squadron, 25); err != nil {
			return err
		}
		if p.Rank.Raw() < 0 || p.MissionType.Raw() < 0 || p.Theatre.Raw() < 0 || p.Emblem < 0 || p.Emblem >= 18 {
			return fmt.Errorf("roster slot %d has unsupported rank, mission, theatre or emblem", i+1)
		}
		if p.Status != StatusInactive && p.Status != StatusActive && p.Status != StatusRetired && p.Status != StatusMIA && p.Status != StatusKIA && p.Status != StatusLegacyUnknown {
			return fmt.Errorf("roster slot %d has unknown status %q", i+1, p.Status)
		}
		if p.Status == StatusLegacyUnknown && (p.LegacyStatus < 2 || len(p.LegacyRaw) != LegacyRecordSize || p.LegacyRaw[0x56] != p.LegacyStatus) {
			return fmt.Errorf("roster slot %d unsupported legacy status lacks raw witness", i+1)
		}
		if p.Status == StatusRetired && (p.LegacyStatus != 2 || len(p.LegacyRaw) != LegacyRecordSize || p.LegacyRaw[0x56] != 2) {
			return fmt.Errorf("roster slot %d retired status lacks source status2 ownership", i+1)
		}
		if p.Status == StatusMIA || p.Status == StatusKIA {
			expected := byte(3)
			if p.Status == StatusKIA {
				expected = 4
			}
			if p.LegacyStatus != expected || len(p.LegacyRaw) != LegacyRecordSize || p.LegacyRaw[0x56] != expected {
				return fmt.Errorf("roster slot %d loss status lacks original status ownership", i+1)
			}
		}
		if p.Statistics != nil && p.Statistics.AwardCounts != nil && len(p.Statistics.AwardCounts) != 8 {
			return fmt.Errorf("roster slot %d award_counters must contain exactly eight signed bytes, or be omitted when unknown", i+1)
		}
		if len(p.LegacyRaw) != 0 && len(p.LegacyRaw) != LegacyRecordSize {
			return fmt.Errorf("roster slot %d opaque record must have %d bytes", i+1, LegacyRecordSize)
		}
		if err := p.Campaign.Validate(); err != nil {
			return fmt.Errorf("pilot %s: %w", p.ID, err)
		}
		if len(p.SquadronPilots) != 0 && len(p.SquadronPilots) != 4 {
			return fmt.Errorf("pilot %s must have four stored squadron pilots", p.ID)
		}
		for _, wingman := range p.SquadronPilots {
			if err := validateSquadronMember(wingman); err != nil {
				return fmt.Errorf("pilot %s: %w", p.ID, err)
			}
		}
		if p.Preflight != nil {
			if err := p.Preflight.Validate(); err != nil {
				return fmt.Errorf("pilot %s: %w", p.ID, err)
			}
		}
	}
	if s.SelectedPilotID != "" && !ids[s.SelectedPilotID] {
		return fmt.Errorf("selected pilot %q does not exist", s.SelectedPilotID)
	}
	if s.RecoveryBefore != nil && *s.RecoveryBefore > s.Revision {
		return fmt.Errorf("recovery cursor exceeds save revision")
	}
	if s.Legacy != nil {
		l := s.Legacy
		if len(l.RawRoster) != 7*LegacyRecordSize || len(l.RecoveryRecord) != LegacyRecordSize || !bytes.Equal(l.RawRoster[6*LegacyRecordSize:], l.RecoveryRecord) || assetdata.Hash(l.RawRoster) != l.SourceSHA256 {
			return fmt.Errorf("legacy provenance/raw roster mismatch")
		}
		if l.PreservedPath != "imports/"+l.SourceSHA256+"/ROSTER.DAT" {
			return fmt.Errorf("legacy import preservation path is not canonical")
		}
	}
	return nil
}

func cloneSave(s SaveState) SaveState {
	raw, _ := json.Marshal(s)
	var result SaveState
	_ = json.Unmarshal(raw, &result)
	return result
}

func readSave(path string) (SaveState, []byte, error) {
	var state SaveState
	if info, err := os.Lstat(path); err == nil && (!info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0) {
		return state, nil, fmt.Errorf("save must be a regular non-symlink file: %s", path)
	}
	raw, err := assetdata.ReadLimitedFile(path, 1<<20)
	if err != nil {
		return state, nil, err
	}
	if err = assetdata.DecodeObject(raw, &state); err != nil {
		return state, nil, fmt.Errorf("invalid save %s: %w; no progress was reset", path, err)
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(raw, &fields)
	var slots []json.RawMessage
	if err = json.Unmarshal(fields["roster_slots"], &slots); err != nil || len(slots) != 6 {
		return state, nil, fmt.Errorf("invalid save %s: roster_slots must contain exactly six ordered slots", path)
	}
	if err = state.Validate(); err != nil {
		return state, nil, fmt.Errorf("invalid save %s: %w", path, err)
	}
	return state, raw, nil
}

type RecoverySnapshot struct {
	ID    string    `json:"id"`
	State SaveState `json:"state"`
}

type SaveStore struct {
	packed    *careerPacks
	liveOwner bool
	Directory string
	state     SaveState
	raw       []byte
	files     map[string][]byte
}

func OpenSaveStore(directory, original string) (*SaveStore, error) {
	return openSaveStore(directory, original, false, "", "", true)
}

// The executable already owns the store. A live career may relocate its libraries.
func OpenLiveSaveStore(directory, original string) (*SaveStore, error) {
	return openSaveStore(directory, original, true, "", "", true)
}

func (s *SaveStore) Close() error {
	s.liveOwner = false
	return nil
}

const careerLease = ".career-owner.lease"

// OpenManagedCareer binds a career root to this application's configuration.
func OpenManagedCareer(directory, original, application string, live bool) (*SaveStore, error) {
	return OpenManagedCareerAt(directory, original, application, application, live)
}
func OpenManagedCareerAt(directory, original, data, install string, live bool) (*SaveStore, error) {
	return openSaveStore(directory, original, live, data, install, true)
}

// MigrateCareer transfers durable records without applying the ordinary game-start
// abandonment policy, under the calling application’s ownership.
func MigrateCareer(directory, original, application string) error {
	return MigrateCareerAt(directory, original, application, application)
}
func MigrateCareerAt(directory, original, application, install string) error {
	s, e := openSaveStore(directory, original, true, application, install, false)
	if e != nil {
		return e
	}
	defer s.Close()
	return s.RelocateLibraries(application)
}
func openSaveStore(directory, original string, live bool, application, install string, startup bool) (*SaveStore, error) {
	if strings.TrimSpace(directory) == "" {
		return nil, fmt.Errorf("saves directory is empty")
	}
	abs, err := filepath.Abs(directory)
	if err != nil {
		return nil, err
	}
	if err = checkSaveDirectory(abs, original); err != nil {
		return nil, err
	}
	if err = os.MkdirAll(abs, 0700); err != nil {
		return nil, fmt.Errorf("create saves directory: %w", err)
	}
	if application != "" {
		if _, _, e := configuration.InitializeAt(filepath.Join(application, configuration.Filename), install); e != nil {
			return nil, e
		}
	}
	store := &SaveStore{Directory: abs, liveOwner: live}
	err = store.Locked(func() error {
		if application == "" {
			if existingPacks(abs) {
				application = filepath.Dir(abs)
			} else {
				return store.openLoose(original, startup)
			}
		}
		if e := store.openPacked(application, install, original, startup); e != nil {
			return e
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return store, nil
}

func (store *SaveStore) openLoose(original string, startup bool) error {
	abs := store.Directory
	if err := resumeSaveCommit(abs); err != nil {
		return err
	}
	if _, err := os.Lstat(filepath.Join(abs, saveFilename)); err == nil {
		return store.loadWithSessions(startup)
	} else if !os.IsNotExist(err) {
		return err
	}
	if _, err := os.Lstat(filepath.Join(abs, "pilots")); err == nil {
		return fmt.Errorf("%s is missing but pilot files exist; preserve the directory and explicitly recover the roster instead of remigrating older progress", saveFilename)
	} else if !os.IsNotExist(err) {
		return err
	}
	if _, err := os.Lstat(filepath.Join(abs, oldSaveFilename)); err == nil {
		state, _, err := readSave(filepath.Join(abs, oldSaveFilename))
		if err != nil {
			return fmt.Errorf("migrate Step C save: %w", err)
		}
		if err = preserveOldSaveDirectory(abs); err != nil {
			return err
		}
		// Only previously unmodelled statistics come from retained legacy bytes;
		// typed names/rank/selections in the user's edited JSON are authoritative.
		raw, err := assetdata.ReadLimitedFile(filepath.Join(abs, oldSaveFilename), 1<<20)
		if err != nil {
			return err
		}
		var old struct {
			Slots []map[string]json.RawMessage `json:"roster_slots"`
		}
		if err = json.Unmarshal(raw, &old); err != nil {
			return err
		}
		for i, p := range state.Slots {
			if p != nil {
				if _, present := old.Slots[i]["statistics"]; !present {
					p.Statistics = statisticsFromLegacy(p.LegacyRaw)
				}
			}
		}
		state.RecoveryBefore = nil
		return store.Initialize(state)
	} else if !os.IsNotExist(err) {
		return err
	}
	for _, name := range []string{"technical-last-good.json", "backups", "pilots"} {
		if _, err := os.Lstat(filepath.Join(abs, name)); err == nil {
			return fmt.Errorf("%s is missing but %s exists; explicitly recover preserved progress instead of initializing an empty roster", saveFilename, name)
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	state, err := importLegacyRoster(original)
	if err != nil {
		return err
	}
	return store.Initialize(state)
}

func checkSaveDirectory(directory, original string) error {
	if strings.TrimSpace(directory) == "" {
		return fmt.Errorf("saves directory is empty")
	}
	if original != "" {
		a, err := assetdata.ResolvedNewPath(original)
		if err != nil {
			return err
		}
		b, err := assetdata.ResolvedNewPath(directory)
		if err != nil {
			return err
		}
		if pathsOverlap(a, b) {
			return fmt.Errorf("save directory must not overlap original installation")
		}
	}
	// The shared generated-output guard also rejects immutable research trees and symlink ancestors.
	_, err := assetdata.GeneratedPath(filepath.Join(directory, ".save-path-check"), filepath.Join(directory, ".unused-input"))
	return err
}

func pathsOverlap(a, b string) bool {
	inside := func(base, path string) bool {
		r, e := filepath.Rel(base, path)
		return e == nil && (r == "." || r != ".." && !strings.HasPrefix(r, ".."+string(filepath.Separator)) && !filepath.IsAbs(r))
	}
	return inside(a, b) || inside(b, a)
}

func (s *SaveStore) State() SaveState { return cloneSave(s.state) }

func (s *SaveStore) Locked(fn func() error) error {
	careerWrites.Lock()
	defer careerWrites.Unlock()
	if s.packed != nil {
		if err := s.packed.refresh(); err != nil {
			return err
		}
	}
	return fn()
}

// Rename replacement is atomic on common same-filesystem implementations, not
// universally durable. Sync the staged file; never remove the old save to make a rename work.
func writeSaveAtomic(directory, name string, raw []byte) error {
	target := filepath.Join(directory, name)
	if info, err := os.Lstat(target); err == nil && (!info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0) {
		return fmt.Errorf("refusing non-regular save target: %s", target)
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	f, err := os.CreateTemp(directory, ".save-pending-*.json")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err = f.Write(raw); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = userpack.Replace(tmp, target); err != nil {
		return fmt.Errorf("publish %s: %w", target, err)
	}
	return nil
}

func writeNewSaveFile(path string, raw []byte) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0400)
	if err != nil {
		return err
	}
	_, err = f.Write(raw)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	return err
}
