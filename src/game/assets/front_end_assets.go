package assets

import (
	"fmt"
)

func LoadFrontEndAssets(directory string) (*OfficeAssets, error) {
	r, err := newPreparedReader(directory)
	if err != nil {
		return nil, err
	}
	defer r.close()
	return r.frontEnd()
}
func (r *preparedReader) frontEnd() (*OfficeAssets, error) {
	assets, err := r.loadOffice()
	if err != nil {
		return nil, err
	}
	clear(r.outputs)
	assets.Menus, err = r.loadMenu()
	if err != nil {
		return nil, fmt.Errorf("front-end preparation required: %w", err)
	}
	clear(r.outputs)
	assets.Startup, err = r.loadStartup()
	if err != nil {
		return nil, err
	}
	clear(r.outputs)
	assets.Missions, err = r.loadMission()
	if err != nil {
		return nil, fmt.Errorf("mission preparation assets required: %w", err)
	}
	clear(r.outputs)
	assets.Audio, err = r.loadAudio()
	if err != nil {
		return nil, fmt.Errorf("front-end audio preparation required: %w", err)
	}
	clear(r.outputs)
	reader, manifest, err := r.worldManifest()
	if err != nil {
		return nil, err
	}
	assets.World, err = loadWorldAssets(reader, manifest)
	if err != nil {
		return nil, fmt.Errorf("world preparation required: %w", err)
	}
	assets.Missions.Generation.NativeModels = assets.World.Geometries
	assets.Cockpit, err = loadCockpitAssets(reader, manifest)
	if err != nil {
		return nil, fmt.Errorf("cockpit preparation required: %w", err)
	}
	assets.Debrief, err = loadDebriefAssets(reader, manifest)
	if err != nil {
		return nil, fmt.Errorf("mission-result preparation required: %w", err)
	}
	assets.Replay, err = loadReplayAssets(reader, manifest, assets)
	if err != nil {
		return nil, fmt.Errorf("mission-replay preparation required: %w", err)
	}
	assets.Editor, err = loadEditorAssets(reader, manifest, assets)
	if err != nil {
		return nil, fmt.Errorf("mission-editor preparation required: %w", err)
	}
	return assets, nil
}
