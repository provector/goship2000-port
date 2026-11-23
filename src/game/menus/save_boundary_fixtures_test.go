package menus

import (
	"encoding/json"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	gamestate "gunship2000-assets/game/state"
	"path/filepath"
)

func cloneSaveForTest(s gamestate.SaveState) gamestate.SaveState {
	b, _ := json.Marshal(s)
	var v gamestate.SaveState
	json.Unmarshal(b, &v)
	return v
}

// Integration faults are created through the documented JSON/filesystem boundary,
// not private SaveStore buffers or exported test-only production internals.
type testSaveOperation struct {
	Path   string `json:"path"`
	Before string `json:"before_sha256,omitempty"`
	After  []byte `json:"after_base64,omitempty"`
	Remove bool   `json:"remove,omitempty"`
}
type testSaveCommit struct {
	SchemaVersion int                 `json:"schema_version"`
	Operations    []testSaveOperation `json:"operations"`
}

func stateFilesForTest(s gamestate.SaveState) (map[string][]byte, error) {
	if s.Legacy != nil {
		return nil, fmt.Errorf("this native fault fixture requires no legacy import")
	}
	roster := struct {
		SchemaVersion int       `json:"schema_version"`
		Revision      uint64    `json:"revision"`
		Slots         [6]string `json:"roster_slots"`
		Selected      string    `json:"selected_pilot_id,omitempty"`
	}{SchemaVersion: 2, Revision: s.Revision, Selected: s.SelectedPilotID}
	files := map[string][]byte{}
	for i, p := range s.Slots {
		if p == nil {
			continue
		}
		roster.Slots[i] = p.ID
		raw, e := assetdata.JsonBytes(struct {
			Schema int `json:"schema_version"`
			*gamestate.Pilot
		}{1, p})
		if e != nil {
			return nil, e
		}
		files[filepath.ToSlash(filepath.Join("pilots", p.ID+".json"))] = raw
	}
	raw, e := assetdata.JsonBytes(roster)
	if e != nil {
		return nil, e
	}
	files["roster.json"] = raw
	return files, nil
}
