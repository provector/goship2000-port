package state

import (
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	"os"
	"strings"
)

// RecoveryCandidateForPilot separates Phoenix ownership from duty posting.
// The snapshot's selected ID is display-only; the canonical posted ID is untouched.
func (s *SaveStore) RecoveryCandidateForPilot(id string) (*RecoverySnapshot, error) {
	if !validPilotID(id) {
		return nil, fmt.Errorf("invalid recovery pilot identity")
	}
	state := s.State()
	index := -1
	for i, p := range state.Slots {
		if p != nil && p.ID == id {
			index = i
		}
	}
	if index < 0 {
		return nil, fmt.Errorf("recovery pilot is no longer in this roster")
	}
	raw, err := s.readRecord(backupFilename(id))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	backed, err := decodePilot(raw, id)
	if err != nil {
		return nil, fmt.Errorf("invalid pre-mission backup: %w", err)
	}
	state.Slots[index] = backed
	state.SelectedPilotID = id
	return &RecoverySnapshot{ID: id + ":" + assetdata.Hash(raw), State: state}, nil
}

// RestoreForPilot consumes exactly the inspected pilot's hash-bound backup.
// storeChecked also rejects external canonical edits and retains consumed results.
func (s *SaveStore) RestoreForPilot(id, snapshotID string) error {
	if err := s.guardPendingSortie(); err != nil {
		return err
	}
	candidate, err := s.RecoveryCandidateForPilot(id)
	if err != nil {
		return err
	}
	if candidate == nil || candidate.ID != snapshotID {
		return fmt.Errorf("inspected pilot pre-mission backup is unavailable or has changed")
	}
	backed := candidate.State.SelectedPilot()
	if backed == nil || backed.ID != id || backed.Statistics == nil {
		return fmt.Errorf("cannot recover pilot with unknown Phoenix/statistics state")
	}
	next := s.State()
	for i, p := range next.Slots {
		if p != nil && p.ID == id {
			next.Slots[i] = backed
		}
	}
	backed.Statistics.PhoenixCounter = int32(uint32(backed.Statistics.PhoenixCounter) - 1)
	return s.storeChecked(next, map[string]*[]byte{backupFilename(id): nil}, backupFilename(id), strings.TrimPrefix(snapshotID, id+":"))
}
