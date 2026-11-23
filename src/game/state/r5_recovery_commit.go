package state

import (
	"bytes"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	"reflect"
)

func (s *SaveStore) adoptSortieView(view *SaveStore) {
	if view != s {
		s.state, s.raw, s.files = view.state, view.raw, view.files
	}
}

// optionalRecoveryViewLocked refreshes one selected optional owner only. It
// cannot rebase an active pilot, Campaign, result, legacy import or another
// roster member. A fresh validated roster may remove/move that exact peer.
func (s *SaveStore) optionalRecoveryViewLocked(doc *sortieDocument) (*SaveStore, map[string][]byte, error) {
	if doc == nil || doc.Start.ProposalVersion != SafeSortieProposalVersion || doc.Proposal == nil || doc.Proposal.RecoveredPilot == nil || doc.Phase == SortieCommitted {
		return s, nil, s.checkSortieSessionFiles()
	}
	peer := doc.Proposal.RecoveredPilot.Before.ID
	path := pilotFilename(peer)
	for name, expected := range s.files {
		actual, err := s.currentRecord(name)
		if err != nil {
			return nil, nil, err
		}
		if name != path && name != saveFilename && !bytes.Equal(actual, expected) {
			return nil, nil, fmt.Errorf("save changed outside this session (%s); pending sortie preserved", name)
		}
	}
	view := &SaveStore{Directory: s.Directory, packed: s.packed}
	if err := view.load(); err != nil {
		return nil, nil, err
	}
	before, after := s.State(), view.State()
	if after.Revision < before.Revision {
		return nil, nil, fmt.Errorf("optional recovery cannot rebase an older roster")
	}
	for _, state := range []*SaveState{&before, &after} {
		state.Revision = 0
		for i, pilot := range state.Slots {
			if pilot != nil && pilot.ID == peer {
				state.Slots[i] = nil
			}
		}
	}
	if !reflect.DeepEqual(before, after) {
		return nil, nil, fmt.Errorf("required career/roster owner changed; optional recovery cannot rebase it")
	}
	preserve := map[string][]byte{}
	if raw, exists := view.files[path]; exists && !bytes.Equal(raw, s.files[path]) {
		preserve[path] = append([]byte(nil), raw...)
	}
	// Preserve even an earlier refresh's changed peer byte-for-byte at later UI
	// publications/final ack; a frozen launch digest remains the write condition.
	if raw, exists := view.files[path]; exists {
		recovery := doc.Proposal.RecoveredPilot
		if hash := recovery.BeforeSHA256; hash != "" && assetdata.Hash(raw) != hash {
			preserve[path] = append([]byte(nil), raw...)
		}
	}
	return view, preserve, nil
}
