package state

import (
	"bytes"
	"encoding/json"
	assetdata "gunship2000-assets/game/assets"
	"os"
	"path/filepath"
	"testing"
)

func TestSquadronIdentityReadback(t *testing.T) {
	proposalPath, backupPath := os.Getenv("GS2000_D3_IDENTITY_PROPOSAL"), os.Getenv("GS2000_D3_IDENTITY_BACKUP")
	if proposalPath == "" || backupPath == "" {
		t.Skip("explicit identity proposal and pre-edit backup required")
	}
	var proposal struct {
		Saves    string `json:"save_directory"`
		PilotID  string `json:"pilot_id"`
		Squadron []struct {
			Name     string `json:"name"`
			Portrait int    `json:"portrait"`
		} `json:"squadron_identities"`
	}
	raw, err := os.ReadFile(proposalPath)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &proposal); err != nil {
		t.Fatal(err)
	}
	raw, err = os.ReadFile(filepath.Join(proposal.Saves, pilotFilename(proposal.PilotID)))
	if err != nil {
		t.Fatal(err)
	}
	p, err := decodePilot(raw, proposal.PilotID)
	if err != nil {
		t.Fatal(err)
	}
	raw, err = os.ReadFile(backupPath)
	if err != nil {
		t.Fatal(err)
	}
	old, err := decodePilot(raw, proposal.PilotID)
	if err != nil {
		t.Fatal(err)
	}
	f, err := InitFlightPilots(p, &MissionData{Inputs: MissionInputs{MissionType: MissionFlight}})
	if err != nil {
		t.Fatal(err)
	}
	if len(f) != 5 || len(proposal.Squadron) != 4 {
		t.Fatal("incorrect row counts")
	}
	for i, want := range proposal.Squadron {
		got := f[i+1]
		if got.Name != want.Name || got.Portrait != want.Portrait {
			t.Fatal("loaded display identity", got)
		}
		if got.Rank != old.SquadronPilots[i].Rank || !bytes.Equal(got.StoredRaw[:], old.SquadronPilots[i].LegacyRaw) {
			t.Fatal("rank/awards/raw witness changed")
		}
	}
}

// This opt-in probe emits a proposal only. It never opens a SaveStore on the
// real save directory or writes the player's progress; application is separately
// audited with a byte-exact backup and a names/portraits-only JSON diff.
func TestSquadronIdentityProposal(t *testing.T) {
	root, output := os.Getenv("GS2000_D3_IDENTITY_APP"), os.Getenv("GS2000_D3_IDENTITY_PROPOSAL")
	if root == "" || output == "" {
		t.Skip("explicit application and NEW proposal path required")
	}
	// Historical read-only proposal uses only these two runtime paths.
	paths := struct{ Assets, Saves string }{"assets", "saves"}
	config, err := os.ReadFile(filepath.Join(root, "config.cfg"))
	if err == nil {
		if err = json.Unmarshal(config, &paths); err != nil {
			t.Fatal(err)
		}
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if !filepath.IsAbs(paths.Assets) {
		paths.Assets = filepath.Join(root, paths.Assets)
	}
	if !filepath.IsAbs(paths.Saves) {
		paths.Saves = filepath.Join(root, paths.Saves)
	}

	raw, err := os.ReadFile(filepath.Join(paths.Saves, "roster.json"))
	if err != nil {
		t.Fatal(err)
	}
	var roster rosterDocument
	if err = assetdata.DecodeObject(raw, &roster); err != nil {
		t.Fatal(err)
	}
	if !validPilotID(roster.SelectedPilotID) {
		t.Fatal("no supported selected pilot")
	}
	state := EmptySaveState()
	state.SelectedPilotID = roster.SelectedPilotID
	for i, id := range roster.Slots {
		if id == "" {
			continue
		}
		raw, err = os.ReadFile(filepath.Join(paths.Saves, pilotFilename(id)))
		if err != nil {
			t.Fatal(err)
		}
		state.Slots[i], err = decodePilot(raw, id)
		if err != nil {
			t.Fatal(err)
		}
	}
	if err = state.Validate(); err != nil {
		t.Fatal(err)
	}
	a, err := assetdata.LoadMissionAssets(paths.Assets)
	if err != nil {
		t.Fatal(err)
	}
	nextPortrait, nextName := 0, 0
	rows, err := InitializeSquadron(state.SelectedPilot(), state.Slots[:], a.Preflight.SquadronNames, func(bound int) int {
		if bound == 33 {
			n := nextPortrait % 33
			nextPortrait++
			return n
		}
		n := nextName % 74
		nextName++
		return n
	})
	if err != nil {
		t.Fatal(err)
	}
	type identity struct {
		Name     string `json:"name"`
		Portrait int    `json:"portrait"`
	}
	proposal := struct {
		Application string     `json:"application_directory"`
		Saves       string     `json:"save_directory"`
		PilotID     string     `json:"pilot_id"`
		Squadron    []identity `json:"squadron_identities"`
		Policy      string     `json:"policy"`
	}{Application: root, Saves: paths.Saves, PilotID: roster.SelectedPilotID, Policy: "Use only initializer-proposed names/portraits. Preserve existing ranks, awards, raw witnesses and every other field."}
	seenNames, seenPortraits := map[string]bool{}, map[int]bool{}
	for _, row := range rows {
		if row.Name == "" || seenNames[row.Name] || row.Portrait < 1 || row.Portrait > 33 || seenPortraits[row.Portrait] {
			t.Fatal("initializer did not produce distinct supported identities")
		}
		seenNames[row.Name], seenPortraits[row.Portrait] = true, true
		proposal.Squadron = append(proposal.Squadron, identity{row.Name, row.Portrait})
	}
	data, err := json.MarshalIndent(proposal, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.Write(append(data, '\n')); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
}
