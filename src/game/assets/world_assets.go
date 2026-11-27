package assets

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	worlddata "gunship2000-assets/game/world"
	"path/filepath"
	"strings"
)

func worldResources(m *Manifest) map[string]*Resource {
	out := map[string]*Resource{}
	for _, r := range m.Resources {
		if r.Source.Kind == "loose_file" {
			name := strings.ToUpper(filepath.Base(r.Source.Path))
			out[name] = r
		}
	}
	return out
}

func exportWorldAssets(writer *OutputWriter, alloc *pathAllocator, m *Manifest) error {
	return exportWorldAssetsProgress(writer, alloc, m, nil, nil)
}
func exportWorldAssetsProgress(writer *OutputWriter, alloc *pathAllocator, m *Manifest, maps, models func(int, int)) error {
	p := &IslandIce469085
	res := worldResources(m)
	var reason []string
	for _, role := range []SourceRole{RoleSimulationExecutable, RoleModelGeometry, RoleModelTypes} {
		s := p.Source(role)
		r := res[s.Name]
		if r == nil {
			reason = append(reason, s.Name+" missing")
		} else if role == RoleSimulationExecutable && !p.SourceMatches(role, r.Source.PayloadSHA256) {
			reason = append(reason, s.Name+" unsupported identity")
		}
	}
	if len(reason) > 0 {
		m.UnsupportedVersionOperations = append(m.UnsupportedVersionOperations, UnsupportedVersionOperation{Operation: "world/cockpit conversion", Profile: p.Name, Reason: strings.Join(reason, "; ")})
		return nil
	}
	gs := res[p.Source(RoleSimulationExecutable).Name]
	obj := res[p.Source(RoleModelGeometry).Name]
	types := res[p.Source(RoleModelTypes).Name]
	b := &worlddata.Bundle{Schema: worlddata.Schema, Profile: p.Name, Models: []string{}, Geometries: map[string]*worlddata.Geometry{}, Worlds: map[string]*worlddata.World{}, Palettes: map[string]worlddata.Palette{}, Sources: map[string]string{}, Notes: []string{
		"Selected VGA source profile; native units uncalibrated. GLB coordinates(X,Y,-Z), reversed triangle winding; actual source polygons/order/materials live in typed metadata.",
		"Live-memory Wreck fills remain appearance-partial; renderer must disclose diagnostic treatment or consume a validated carrier.",
		"Source heights/query planes are separate from replaceable GLB visuals. No gameplay, physics, natural first-frame pose or simulation rate is established.",
	}}
	for name, r := range res {
		b.Sources[name] = r.Source.PayloadSHA256
	}
	// The native renderer implements the released VGA profile. DOS device preferences
	// are user persistence, independent of this port's own graphics configuration.
	b.GraphicsMode, b.GraphicsOptions = 0, 0xdf
	at := p.WorldLayout.SineFile
	if at < 0 || at+2050 > len(gs.Payload) {
		return fmt.Errorf("source sine table bounds")
	}
	b.Sine = make([]int16, 1025)
	for i := range b.Sine {
		b.Sine[i] = int16(binary.LittleEndian.Uint16(gs.Payload[at+2*i:]))
	}
	// These are separate source carriers, not an atan approximation derived from
	// the sine table or RGB pixels inferred from the rendered weather effect.
	words := func(at, count int) ([]uint16, error) {
		if at < 0 || count > (len(gs.Payload)-at)/2 || at > len(gs.Payload) {
			return nil, fmt.Errorf("world camera table bounds")
		}
		out := make([]uint16, count)
		for i := range out {
			out[i] = binary.LittleEndian.Uint16(gs.Payload[at+2*i:])
		}
		return out, nil
	}
	var tableErr error
	if b.AngleBases, tableErr = words(p.WorldLayout.AngleBasesFile, 8); tableErr != nil {
		return tableErr
	}
	if b.Atan, tableErr = words(p.WorldLayout.AtanFile, 4096); tableErr != nil {
		return tableErr
	}
	at = p.WorldLayout.WeatherRemapFile
	if at < 0 || at+256 > len(gs.Payload) {
		return fmt.Errorf("weather remap bounds")
	}
	b.WeatherRemap = append([]byte(nil), gs.Payload[at:at+256]...)
	if err := prepareTerrainFeedback(gs, p, b); err != nil {
		return err
	}

	entries := []uint16{0, 0x16f3, 0x1728, 0x1761, 0x17c5, 0x1828, 0x17f6, 0x1863}
	kinds := []string{"none", "cyclic_first8", "cyclic_first10", "fixed28_xz_q15", "swap_two6_banks", "second_vertex_z_wave32", "swap_two12_banks", "guard_only"}
	counts := []int{0, 8, 10, 28, 12, 2, 24, 0}
	if p.WorldLayout.CallbackTableFile < 0 || p.WorldLayout.CallbackTableFile+16 > len(gs.Payload) {
		return fmt.Errorf("model modifier table bounds")
	}
	for i, entry := range entries {
		got := binary.LittleEndian.Uint16(gs.Payload[p.WorldLayout.CallbackTableFile+2*i:])
		if got != entry {
			return fmt.Errorf("model modifier table mismatch at%d", i)
		}
		if i == 0 {
			continue
		}
		transform := "ordinary"
		if i == 1 || i == 2 || i == 4 || i == 6 {
			transform = "oriented"
		}
		coefficient := int16(0)
		if i == 3 {
			coefficient = 0x6eda
		}
		b.Modifiers = append(b.Modifiers, worlddata.Modifier{ID: i, Kind: kinds[i], Vertices: counts[i], SourceEntry: entry, CoefficientQ15: coefficient, Guard: byte(1 << (i - 1)), Ownership: "shared source stream; one mutation per family logical visual update; immutable asset copied first", Transform: transform})
	}

	geometries, e := worlddata.ParseModels(obj.Payload, types.Payload)
	if e != nil {
		return sourceFailure(fmt.Errorf("OBJFILE / OBJTYPES.DAT model conversion: %w", e))
	}
	for _, g := range geometries {
		b.Models = append(b.Models, g.ID)
		b.Geometries[g.ID] = g
	}
	for worldIndex, name := range p.WorldLayout.WorldNames {
		r, atlas := res[name], res[name+"BMP.GS2"]
		if r == nil || atlas == nil {
			return sourceFailure(fmt.Errorf("world preparation requires %s and %sBMP.GS2", name, name))
		}
		if maps != nil {
			maps(worldIndex, len(p.WorldLayout.WorldNames))
		}
		atlasPayload, _, correctionErr := CorrectKnownMapAtlas(name+"BMP.GS2", atlas.Payload)
		if correctionErr != nil {
			return fmt.Errorf("%s map conversion: %w", name, correctionErr)
		}
		w, gs, e := worlddata.ParseWorld(name, r.Payload, atlasPayload)
		if e != nil {
			return sourceFailure(fmt.Errorf("%s:%w", name, e))
		}
		overview := res[name+"SMP.GS2"]
		if overview == nil {
			return sourceFailure(fmt.Errorf("world preparation requires%sSMP.GS2", name))
		}
		w.Overview, e = DecodeOverview(overview.Payload)
		if e != nil {
			return sourceFailure(fmt.Errorf("%sSMP.GS2:%w", name, e))
		}
		w.OverviewSourceHash = overview.Source.PayloadSHA256
		b.Worlds[name] = w
		for _, g := range gs {
			if b.Geometries[g.ID] != nil {
				return fmt.Errorf("duplicate geometry%s", g.ID)
			}
			b.Geometries[g.ID] = g
		}
	}
	if maps != nil {
		maps(len(p.WorldLayout.WorldNames), len(p.WorldLayout.WorldNames))
	}
	for _, prefix := range []string{"ANTC", "EURO", "GULF", "PHIL"} {
		for _, kind := range []string{"DAY", "GREY", "DARK"} {
			name := prefix + kind + ".PAL"
			r := res[name]
			if r == nil || len(r.Payload) != 768 {
				return sourceFailure(fmt.Errorf("world palette%s missing/invalid", name))
			}
			for _, c := range r.Payload {
				if c > 63 {
					return sourceFailure(fmt.Errorf("world palette%s component >63", name))
				}
			}
			b.Palettes[prefix+kind] = worlddata.Palette{ID: prefix + kind, RGB6: append([]byte(nil), r.Payload...)}
		}
	}
	write := func(relative string, data []byte) error {
		out, e := writer.Write("world/"+relative, data)
		if e != nil {
			return e
		}
		out.Kind = "world_prepared_data"
		if relative == "index.json" {
			out.Kind = "world_metadata"
			m.WorldMetadata = out.Path
		}
		gs.Outputs = append(gs.Outputs, out)
		return nil
	}
	// Preserve complete raw carriers and each exact model record alongside GLB.
	for _, name := range []string{"GS2.GS2", "OBJFILE", "OBJTYPES.DAT"} {
		if e = write("raw/"+name, res[name].Payload); e != nil {
			return e
		}
	}
	for i, g := range geometries {
		lo := int(binary.LittleEndian.Uint32(obj.Payload[4*i:]))
		hi := len(obj.Payload)
		if i+1 < len(geometries) {
			hi = int(binary.LittleEndian.Uint32(obj.Payload[4*i+4:]))
		}
		if e = write(g.Raw, obj.Payload[lo:hi]); e != nil {
			return e
		}
	}
	for _, name := range p.WorldLayout.WorldNames {
		if e = write("raw/"+name, res[name].Payload); e != nil {
			return e
		}
		if e = write("raw/"+name+"SMP.GS2", res[name+"SMP.GS2"].Payload); e != nil {
			return e
		}
		if e = write("raw/"+name+"BMP.GS2", res[name+"BMP.GS2"].Payload); e != nil {
			return e
		}
	}
	if e = prepareCombatPresentation(gs, p, b, write); e != nil {
		return fmt.Errorf("combat presentation:%w", e)
	}
	if e = worlddata.EncodeBundleProgress(b, write, models); e != nil {
		return fmt.Errorf("write world assets:%w", e)
	}
	if e = prepareWorldCockpit(writer, alloc, m, p); e != nil {
		return fmt.Errorf("cockpit preparation:%w", e)
	}
	return nil
}

func loadWorldAssets(reader *preparedReader, m Manifest) (*worlddata.Bundle, error) {
	if m.PreparationVersion < 17 || m.WorldMetadata != "world/index.json" {
		return nil, fmt.Errorf("world assets require preparation17; rebuild from supported originals")
	}
	b, e := worlddata.LoadFiles(func(relative string) ([]byte, error) {
		if !worlddata.SafeRelative(relative) {
			return nil, fmt.Errorf("unsafe world path")
		}
		name := "world/" + relative
		out, ok := reader.outputs[name]
		if !ok {
			return nil, fmt.Errorf("world output %q missing", name)
		}
		if out.Kind != "world_prepared_data" && out.Kind != "world_metadata" {
			return nil, fmt.Errorf("world output %q kind", name)
		}
		return reader.verified(out, 64<<20)
	})
	if e != nil {
		return nil, e
	}
	p := &IslandIce469085
	if b.Profile != p.Name {
		return nil, fmt.Errorf("unsupported world profile %q", b.Profile)
	}
	if b.Combat == nil {
		return nil, fmt.Errorf("combat presentation assets missing; rebuild from supported originals")
	}
	// N.1 preparation19 adds the bounded native terrain lookup. Older prepared
	// fixtures deliberately retain the accepted fallback, never an invented table.
	if m.PreparationVersion >= 19 && !b.TerrainFeedback.Ready() {
		return nil, fmt.Errorf("terrain feedback assets missing; rebuild from supported originals")
	}
	for _, role := range []SourceRole{RoleSimulationExecutable, RoleModelGeometry, RoleModelTypes} {
		src := p.Source(role)
		if (role == RoleSimulationExecutable || m.PreparationVersion < 35) && !p.SourceMatches(role, b.Sources[src.Name]) {
			return nil, fmt.Errorf("world source identity for%s is unsupported", src.Name)
		}
	}
	// R4 derives the two source-backed player decoy descriptors from the raw
	// payload already retained and manifest-verified in every supported cache.
	// No preparation output, canonical resource identity or shared model changes.
	rawGS2, err := func() ([]byte, error) {
		out, ok := reader.outputs["world/raw/GS2.GS2"]
		if !ok || out.Kind != "world_prepared_data" {
			return nil, fmt.Errorf("retained decoy executable carrier missing")
		}
		return reader.verified(out, 64<<20)
	}()
	if err != nil {
		return nil, err
	}
	decoys, err := worlddata.PlayerDecoyGeometries(rawGS2)
	if err != nil {
		return nil, err
	}
	for _, g := range decoys {
		b.Geometries[g.ID] = g
	}
	for _, name := range p.WorldLayout.WorldNames {
		if b.Worlds[name] == nil || len(b.Worlds[name].Overview) != 89*65 {
			return nil, fmt.Errorf("required world identity%s missing", name)
		}
	}
	return b, nil
}

func LoadWorldAssets(directory string) (*worlddata.Bundle, error) {
	reader, m, e := worldPreparedReader(directory)
	if e != nil {
		return nil, e
	}
	defer reader.close()
	return loadWorldAssets(reader, m)
}

func worldPreparedReader(directory string) (*preparedReader, Manifest, error) {
	r, e := newPreparedReader(directory)
	if e != nil {
		return nil, Manifest{}, e
	}
	reader, m, e := r.worldManifest()
	if e != nil {
		r.close()
	}
	return reader, m, e
}
func (r *preparedReader) worldManifest() (*preparedReader, Manifest, error) {
	raw, e := r.read("manifest.json", 64<<20)
	if e != nil {
		return nil, Manifest{}, e
	}
	var m Manifest
	if e = json.Unmarshal(raw, &m); e != nil {
		return nil, m, e
	}
	if !compatiblePreparation(m) {
		return nil, m, fmt.Errorf("prepared manifest incompatible")
	}
	for _, res := range m.Resources {
		if res == nil {
			return nil, m, fmt.Errorf("nullresource")
		}
		for _, out := range res.Outputs {
			if prev, ok := r.outputs[out.Path]; ok && prev != out {
				return nil, m, fmt.Errorf("conflicting output%s", out.Path)
			}
			r.outputs[out.Path] = out
		}
	}
	return r, m, nil
}
