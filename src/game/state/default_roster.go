package state

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"gunship2000-assets/internal/rostertemplate"
	"io"
	"os"
	"path/filepath"
)

func defaultRoster(app string) (SaveState, error) {
	data := rostertemplate.Default
	z, e := zip.OpenReader(filepath.Join(app, "support.g2pak"))
	if e == nil {
		defer z.Close()
		found := false
		for _, f := range z.File {
			if f.Name != "defaults/roster.json" {
				continue
			}
			if found || f.Method != zip.Store || f.UncompressedSize64 > 1<<20 {
				return SaveState{}, fmt.Errorf("invalid default roster in support.g2pak")
			}
			reader, e := f.Open()
			if e != nil {
				return SaveState{}, e
			}
			data, e = io.ReadAll(io.LimitReader(reader, 1<<20+1))
			reader.Close()
			if e != nil {
				return SaveState{}, e
			}
			found = true
		}
		if !found {
			return SaveState{}, fmt.Errorf("support.g2pak lacks defaults/roster.json; use matching support pack")
		}
	} else if !os.IsNotExist(e) {
		return SaveState{}, e
	}
	var s SaveState
	if e = json.Unmarshal(data, &s); e != nil {
		return s, e
	}
	if e = s.Validate(); e != nil {
		return s, e
	}
	for _, pilot := range s.Slots {
		if pilot != nil {
			id, e := newStoreID()
			if e != nil {
				return s, e
			}
			if s.SelectedPilotID == pilot.ID {
				s.SelectedPilotID = id
			}
			pilot.ID = id
		}
	}
	return s, s.Validate()
}
