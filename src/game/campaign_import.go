package main

import (
	"flag"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/game/configuration"
	gamestate "gunship2000-assets/game/state"
	"os"
	"path/filepath"
	"strings"
)

type campaignImportPaths []string

func (p *campaignImportPaths) String() string { return strings.Join(*p, ",") }

func (p *campaignImportPaths) Set(value string) error { *p = append(*p, value); return nil }

// This maintenance command creates a separate save tree. It never attaches a
// legacy cache to an existing user profile or imports over authoritative saves.
func runCampaignImport(args []string) int {
	flags := flag.NewFlagSet("gs2000 import-campaign", flag.ContinueOnError)
	app := flags.String("app-dir", "", "application owning this imported career")
	assets := flags.String("assets", "", "compatible prepared asset directory")
	roster := flags.String("roster", "", "matching2030-byte legacy ROSTER.DAT (read only)")
	cache := flags.String("cache", "", "complete suspended CAMPGN...9.DAT (read only)")
	output := flags.String("output", "", "new save directory; must not exist")
	slot := flags.Int("slot", 0, "optional roster slot1..6; defaultfirst Active import policy")
	var intermediate campaignImportPaths
	flags.Var(&intermediate, "intermediate", "optional snapshot-associated intermediate cache (read only)")
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 1
	}
	if flags.NArg() != 0 || *assets == "" || *roster == "" || *cache == "" || *output == "" || *slot < 0 || *slot > 6 {
		fmt.Fprintln(os.Stderr, "usage: gs2000 import-campaign -assets DIRECTORY -roster FILE -cache FILE [-intermediate FILE] [-slot 1..6] -output NEW_DIRECTORY")
		return 1
	}

	owner, ownerErr := configuration.OpenLocation(*app, applicationMode, *assets, false, *output)
	if ownerErr != nil {
		return startupError(ownerErr)
	}
	defer owner.Close()
	if err := importCampaignSave(*assets, *roster, *cache, intermediate, *output, *slot); err != nil {
		fmt.Fprintln(os.Stderr, "Campaign import:", err)
		return 1
	}
	store, e := gamestate.OpenManagedCareerAt(*output, "", owner.Data, owner.Install, true)
	if e != nil {
		return startupError(e)
	}
	defer store.Close()
	fmt.Printf("Campaign import saved to %s; source files preserved under imports/. Normal saves were not changed.\n", *output)
	return 0
}

func importCampaignSave(assets, roster, cache string, intermediates []string, output string, slot int) error {
	if slot < 0 || slot > 6 || len(intermediates) > 1 {
		return fmt.Errorf("choose slot0..6 and at most the one snapshot-associated intermediate cache")
	}
	assets, err := filepath.Abs(assets)
	if err != nil {
		return err
	}
	output, err = assetdata.GeneratedPath(output, assets)
	if err != nil {
		return err
	}
	type input struct {
		Role, Path, StoredPath, SHA256 string
		Data                           []byte `json:"-"`
	}
	inputs := []input{{Role: "roster", Path: roster}, {Role: "full-cache", Path: cache}}
	for _, path := range intermediates {
		inputs = append(inputs, input{Role: "intermediate", Path: path})
	}
	for i := range inputs {
		path, err := filepath.Abs(inputs[i].Path)
		if err != nil {
			return err
		}
		if _, _, err = assetdata.ValidatePaths(filepath.Dir(path), output); err != nil {
			return err
		}
		raw, err := gamestate.ReadRegularSave(path)
		if err != nil {
			return err
		}
		inputs[i].Path, inputs[i].Data, inputs[i].SHA256 = path, raw, assetdata.Hash(raw)
	}
	a, err := assetdata.LoadMissionAssets(assets)
	if err != nil {
		return err
	}
	state, err := gamestate.ParseLegacyRoster(inputs[0].Path, inputs[0].Data)
	if err != nil {
		return err
	}
	if slot != 0 {
		if state.Slots[slot-1] == nil {
			return fmt.Errorf("roster slot%d is empty", slot)
		}
		state.SelectedPilotID = state.Slots[slot-1].ID
	}
	p := state.SelectedPilot()
	if p == nil {
		return fmt.Errorf("matching roster has no selected Active pilot")
	}
	fragments := map[string][]byte{}
	for _, in := range inputs[2:] {
		fragments[filepath.Base(in.Path)] = in.Data
	}
	p.Campaign, err = gamestate.ImportCampaignCache(a, p, inputs[1].Data, fragments)
	if err != nil {
		return err
	}
	if err = state.Validate(); err != nil {
		return err
	}
	// Validate the complete typed restore before creating any output. This is a
	// static source-data conversion, not an original-game runtime verification.
	if _, _, err = gamestate.CampaignStart(a, p, 1, nil, nil); err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(output), 0700); err != nil {
		return err
	}
	if err = os.Mkdir(output, 0700); err != nil {
		return err
	}
	prefix := "imports/campaign-" + inputs[1].SHA256
	for i := range inputs {
		inputs[i].StoredPath = filepath.ToSlash(filepath.Join(prefix, inputs[i].Role, filepath.Base(inputs[i].Path)))
		if _, err = assetdata.WriteNew(output, inputs[i].StoredPath, inputs[i].Data); err != nil {
			return err
		}
	}
	provenance := struct {
		Version         int     `json:"version"`
		SourceProfile   string  `json:"source_profile"`
		Operation       string  `json:"operation"`
		SelectedPilot   string  `json:"selected_pilot_id"`
		SelectionPolicy string  `json:"selection_policy"`
		Inputs          []input `json:"inputs"`
	}{1, assetdata.IslandIce469085.Name, "Read-only compatible suspended Campaign conversion; producing executable/action history is not independently verified.", p.ID, "Explicit slot if supplied; otherwise first Active occupied slot. No original persisted selected-index byte inferred.", inputs}
	raw, err := assetdata.JsonBytes(provenance)
	if err != nil {
		return err
	}
	if _, err = assetdata.WriteNew(output, prefix+"/provenance.json", raw); err != nil {
		return err
	}
	store := &gamestate.SaveStore{Directory: output}
	return store.Locked(func() error { return store.Initialize(state) })
}
