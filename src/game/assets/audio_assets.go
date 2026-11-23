package assets

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
)

var requiredAudioKeys = []string{"logo", "title", "office_intro", "office_loop", "gs4_0d", "gs4_0e", "gs4_21", "gs4_22", "gs4_23", "gs4_24", "gs4_42", "gs2_initialization_start", "gs2_initialization_early"}

// SupportsDefenseReleaseAudio is a current capability, separate from admitting
// historical audio metadata. The full loader verifies provenance/media first;
// both ordinary readiness and verified practice use these same source carriers.
func (a *AudioAssets) SupportsDefenseReleaseAudio() bool {
	if a == nil {
		return false
	}
	for _, tc := range []struct {
		id                  uint16
		row, start, handler int
		raw                 []byte
	}{
		{0x0a, 2, 0x924, 0x93a, []byte{0x8d, 0x0e, 0x24, 0x09, 0xe9, 0xcf, 0xfd}},
		{0x2a, 0, 0x99a, 0x933, []byte{0x8d, 0x0e, 0x9a, 0x09, 0xe9, 0xc8, 0xfd}},
	} {
		c, ok := a.GS2Commands[tc.id]
		if !ok || c.EventID != tc.id || c.Channel != tc.row || c.Start != tc.start || c.Alternate != 0 || c.ClearNoise || c.HandlerCS != tc.handler || c.HandlerFile != 512+tc.handler || c.HandlerHex != fmt.Sprintf("%x", tc.raw) || c.HandlerSHA256 != Hash(tc.raw) || c.TrackKey != fmt.Sprintf("gs2_%02x", tc.id) {
			return false
		}
		clip := a.Clips[c.TrackKey]
		if clip == nil || clip.SampleRate != AudioSampleRate || len(clip.Samples) == 0 || clip.LoopStartFrame != 0 || clip.LoopEndFrame != 0 || len(a.ASoundGS2Data) <= tc.start {
			return false
		}
	}
	return true
}

// SupportsThreatWarningAudio validates the actual source06 fixed constructor.
func (a *AudioAssets) SupportsThreatWarningAudio() bool {
	if a == nil {
		return false
	}
	raw := []byte{0x8d, 0x0e, 0xb4, 0x08, 0xe9, 0x0e, 0xfe}
	c, ok := a.GS2Commands[6]
	if !ok || c.EventID != 6 || c.Channel != 3 || c.Start != 0x8b4 || c.Alternate != 0 || c.ClearNoise || c.HandlerCS != 0x902 || c.HandlerFile != 0xb02 || c.HandlerHex != fmt.Sprintf("%x", raw) || c.HandlerSHA256 != Hash(raw) || c.TrackKey != "gs2_06" {
		return false
	}
	clip := a.Clips[c.TrackKey]
	return clip != nil && clip.SampleRate == AudioSampleRate && len(clip.Samples) > 0 && len(a.ASoundGS2Data) > 0x8b4
}

// SupportsStationaryHostileAudio is the source A527 direct29 capability.
// Older preparations remain readable but cannot promise this new live caller.
func (a *AudioAssets) SupportsStationaryHostileAudio() bool {
	if a == nil {
		return false
	}
	raw := []byte{0x8d, 0x0e, 0xe0, 0x08, 0xe9, 0xf2, 0xfd}
	c, ok := a.GS2Commands[0x29]
	if !ok || c.EventID != 0x29 || c.Channel != 1 || c.Start != 0x8e0 || c.Alternate != 0 || c.ClearNoise || c.HandlerCS != 0x910 || c.HandlerFile != 0xb10 || c.HandlerHex != fmt.Sprintf("%x", raw) || c.HandlerSHA256 != Hash(raw) || c.TrackKey != "gs2_29" {
		return false
	}
	clip := a.Clips[c.TrackKey]
	return clip != nil && clip.SampleRate == AudioSampleRate && len(clip.Samples) > 0 && clip.LoopStartFrame == 0 && clip.LoopEndFrame == 0 && len(a.ASoundGS2Data) > 0x8e0
}

func LoadAudioAssets(directory string) (*AudioAssets, error) {
	reader, err := newPreparedReader(directory)
	if err != nil {
		return nil, err
	}
	defer reader.close()
	return reader.loadAudio()
}
func (reader *preparedReader) loadAudio() (*AudioAssets, error) {
	raw, err := reader.read("manifest.json", 64<<20)
	if err != nil {
		return nil, err
	}
	var manifest Manifest
	if err = json.Unmarshal(raw, &manifest); err != nil {
		return nil, err
	}
	if !compatiblePreparation(manifest) || manifest.AudioMetadata == "" {
		for _, u := range manifest.UnsupportedVersionOperations {
			if u.Operation == "startup/front-end audio" {
				return nil, fmt.Errorf("audio preparation unsupported: %s", u.Reason)
			}
		}
		return nil, fmt.Errorf("prepared assets require Step F audio metadata; rebuild with the supported original installation")
	}
	for _, resource := range manifest.Resources {
		if resource == nil {
			return nil, fmt.Errorf("null audio manifest resource")
		}
		for _, o := range resource.Outputs {
			if old, ok := reader.outputs[o.Path]; ok && old != o {
				return nil, fmt.Errorf("conflicting audio output %s", o.Path)
			}
			reader.outputs[o.Path] = o
		}
	}
	output, ok := reader.outputs[manifest.AudioMetadata]
	if !ok || output.Kind != "audio metadata" {
		return nil, fmt.Errorf("audio metadata is not an indexed prepared output")
	}
	raw, err = reader.verified(output, 8<<20)
	if err != nil {
		return nil, err
	}
	var meta AudioMetadata
	if err = DecodeObject(raw, &meta); err != nil {
		return nil, fmt.Errorf("audio metadata: %w", err)
	}
	if meta.Schema != 5 && meta.Schema != 6 && meta.Schema != 7 && meta.Schema != 8 && meta.Schema != audioMetadataSchema || meta.Policy != AudioBakePolicy || meta.Profile != IslandIce469085.Name {
		return nil, fmt.Errorf("unsupported prepared audio schema/profile/policy; rebuild assets")
	}
	if len(meta.Tracks) != 52+len(DebriefMusicEvents)+len(expectedGS2GameplayEvents(meta.Schema)) || len(meta.DigitalEvents) != 22 || len(meta.Carriers) != 6 || len(meta.ContextSources) != 4 {
		return nil, fmt.Errorf("incomplete prepared audio: expected core and GS2 gameplay tracks/22digitallists/6carriers")
	}
	files := map[string]Output{}
	var total int64
	fmCount, digitalCount := 0, 0
	digitalByKey := map[string]*AudioClip{}
	for _, o := range meta.Files {
		if _, exists := files[o.Path]; exists {
			return nil, fmt.Errorf("duplicate audio file %s", o.Path)
		}
		if o.Size < 0 || o.Size > 32<<20 {
			return nil, fmt.Errorf("oversized audio file %s", o.Path)
		}
		total += int64(o.Size)
		if total > 256<<20 {
			return nil, fmt.Errorf("prepared audio exceeds256MiB")
		}
		files[o.Path] = o
	}
	// Playback opens no MZ code. The bounded retained rotor owner additionally
	// needs the already prepared GS2 initialized-data carrier, verified below.
	carriers := map[string]bool{}
	var rotorData *Output
	for _, c := range meta.Carriers {
		var source KnownSourceIdentity
		for _, role := range AudioSourceRoles {
			s := IslandIce469085.Source(role)
			if s.Name == c.Name {
				source = s
				break
			}
		}
		if source.Name == "" || carriers[c.Name] || c.Source.PayloadSHA256 != source.SHA256 || c.Raw.SHA256 != source.SHA256 {
			return nil, fmt.Errorf("invalid audio carrier identity %s", c.Name)
		}
		carriers[c.Name] = true
		if files[c.Raw.Path] != c.Raw {
			return nil, fmt.Errorf("unindexed audio carrier %s", c.Name)
		}
		if c.Name == IslandIce469085.Source(RoleAudioADigital).Name {
			if c.InitializedData == nil || files[c.InitializedData.Path] != *c.InitializedData || c.InitializedData.Kind != "source initialized audio data" {
				return nil, fmt.Errorf("unindexed GS2 initialized audio carrier; rebuild prepared assets")
			}
			copy := *c.InitializedData
			rotorData = &copy
		}
	}
	contextRoles := map[SourceRole]bool{}
	for _, c := range meta.ContextSources {
		found := false
		for _, role := range []SourceRole{RoleMainExecutable, RoleLogoPlayer, RoleIntroPlayer, RoleSimulationExecutable} {
			if c.PayloadSHA256 == IslandIce469085.Source(role).SHA256 && !contextRoles[role] {
				contextRoles[role] = true
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("unsupported audio timing-context source")
		}
	}
	trackByPath := map[string]AudioTrackMetadata{}
	seenKeys := map[string]bool{}
	for _, t := range meta.Tracks {
		if seenKeys[t.Key] || t.Key == "" || t.Channels != 1 || t.OutputGain != 1 || t.ClippedSamples != 0 {
			return nil, fmt.Errorf("invalid or duplicate audio track %q", t.Key)
		}
		seenKeys[t.Key] = true
		if _, exists := trackByPath[t.Output.Path]; exists {
			return nil, fmt.Errorf("duplicate audio PCM path")
		}
		if files[t.Output.Path] != t.Output {
			return nil, fmt.Errorf("unindexed track %s", t.Key)
		}
		if t.Frames == 0 || t.BusyEndFrame > t.Frames || t.TailEndFrame != t.Frames || (t.LastNonzeroFrame < -1 || t.LastNonzeroFrame >= int64(t.Frames)) {
			return nil, fmt.Errorf("audio lifetime outside track %s", t.Key)
		}
		if t.Bits == 16 {
			fmCount++
		} else {
			digitalCount++
		}
		if t.Bits == 16 && (t.SampleRate != AudioSampleRate || t.RNGSeed != 1234) {
			return nil, fmt.Errorf("unsupported canonical FM profile %s", t.Key)
		}
		if t.Bits != 16 && !(t.Bits == 8 && t.SampleRate == 8000) {
			return nil, fmt.Errorf("unsupported audio encoding %s", t.Key)
		}
		if t.Key == "gs4_0d" && (t.EventID != 0x0d || len(t.Aliases) != 1 || t.Aliases[0] != 0x13) {
			return nil, fmt.Errorf("missing GS4 alias13h to0dh")
		}
		if t.Key == "gs2_initialization_start" && (t.Module != "ASOUND.GS2" || t.EventID != 0x0b || len(t.Aliases) != 0 || t.PreparationPolicy != InitializationStartBakePolicy) {
			return nil, fmt.Errorf("invalid GS2 initialization source event")
		}
		if t.Key == "gs2_initialization_early" && (t.Module != "ASOUND.GS2" || t.EventID != 0x15 || len(t.Aliases) != 0 || t.PreparationPolicy != InitializationEarlyBakePolicy || t.LoopStartFrame == 0 || t.LoopEndFrame != t.Frames || t.LoopStartFrame >= t.LoopEndFrame || t.SourcePeriodPIT == 0 || t.SourcePeriodDenominator == 0) {
			return nil, fmt.Errorf("invalid early GS2 initialization event/loop")
		}
		if t.Key == "office_loop" && (t.LoopStartFrame != 0 || t.LoopEndFrame != t.Frames || t.SourcePeriodPIT == 0 || t.SourcePeriodDenominator == 0) {
			return nil, fmt.Errorf("invalid office loop range/clock")
		}
		trackByPath[t.Output.Path] = t
	}
	if fmCount != 13+len(DebriefMusicEvents)+len(expectedGS2GameplayEvents(meta.Schema)) || digitalCount != 39 {
		return nil, fmt.Errorf("incomplete FM/digital track families")
	}
	assets := &AudioAssets{Clips: map[string]*AudioClip{}, Policy: meta.Policy}
	if rotorData == nil {
		return nil, fmt.Errorf("prepared GS2 initialized audio carrier missing; rebuild assets")
	}
	assets.ASoundGS2Data, err = reader.verified(*rotorData, 65536)
	if err != nil {
		return nil, fmt.Errorf("prepared GS2 flight sound data: %w", err)
	}
	for _, o := range meta.Files {
		t, isTrack := trackByPath[o.Path]
		if !isTrack {
			continue
		}
		if reader.packs != nil && t.Bits == 16 && (t.Key == "logo" || t.Key == "title" || t.Key == "office_intro" || t.Key == "office_loop" || t.Module == "ASOUND.GS4" && t.EventID >= 0x19 && t.EventID <= 0x20) {
			if reader.outputs[o.Path] != o {
				return nil, fmt.Errorf("unregistered packed music %s", t.Key)
			}
			if reader.observed != nil {
				reader.observed[o.Path] = true
			}
			music, e := reader.packs.music(o, t.Frames, t.LoopStartFrame)
			if e != nil {
				return nil, e
			}
			assets.Clips[t.Key] = &AudioClip{Music: music, SampleRate: t.SampleRate, BusyEndFrame: t.BusyEndFrame, LoopStartFrame: t.LoopStartFrame, LoopEndFrame: t.LoopEndFrame}
			continue
		}
		raw, e := reader.verified(o, 32<<20)
		if e != nil {
			return nil, e
		}
		clip, bits, e := decodeAudioWAV(raw)
		if e != nil {
			return nil, fmt.Errorf("audio %s: %w", t.Key, e)
		}
		if bits != t.Bits || clip.SampleRate != t.SampleRate || uint64(len(clip.Samples)) != t.Frames {
			return nil, fmt.Errorf("WAV/metadata layout differs for %s", t.Key)
		}
		// The converter emits a canonical44-byte WAV header. Hash the exact stored
		// PCM bytes, including U8 midpoint values, instead of the signed playback view.
		pcm, e := audioWAVPayload(raw)
		if e != nil || Hash(pcm) != t.PCMSHA256 {
			return nil, fmt.Errorf("decoded PCM hash differs for %s", t.Key)
		}
		if bits == 16 {
			clip.BusyEndFrame = t.BusyEndFrame
			clip.LoopStartFrame, clip.LoopEndFrame = t.LoopStartFrame, t.LoopEndFrame
			assets.Clips[t.Key] = clip
		} else {
			digitalByKey[t.Key] = clip
		}
	}
	assets.GS2Digital = map[uint16]AudioDigitalPlayback{}
	for _, event := range IslandIce469085.AudioLayout.Drivers["GS2"].DigitalGameplayEvents {
		for _, entry := range meta.DigitalEvents {
			if entry.EventID != event {
				continue
			}
			clip := digitalByKey[entry.TrackKey]
			if clip == nil || clip.SampleRate != 8000 || entry.Priority < 0 || entry.Priority > 32767 {
				return nil, fmt.Errorf("invalid digital GS2 gameplay event%02x", event)
			}
			assets.GS2Digital[uint16(event)] = AudioDigitalPlayback{Clip: clip, Priority: entry.Priority}
		}
		if _, ok := assets.GS2Digital[uint16(event)]; !ok {
			return nil, fmt.Errorf("missing digital GS2 gameplay event%02x", event)
		}
	}
	assets.GS2Commands = make(map[uint16]AudioGS2Command)
	expected := expectedGS2GameplayEvents(meta.Schema)
	if len(meta.GS2Commands) != len(expected) {
		return nil, fmt.Errorf("missing GS2 gameplay command metadata; rebuild assets")
	}
	for _, command := range meta.GS2Commands {
		if _, found := assets.GS2Commands[command.EventID]; found {
			return nil, fmt.Errorf("duplicate GS2 command")
		}
		if command.Channel < 0 || command.Channel > 6 || command.Start < 0 || command.Start >= len(assets.ASoundGS2Data) || command.Alternate < 0 || command.Alternate >= len(assets.ASoundGS2Data) || assets.Clips[command.TrackKey] == nil || command.HandlerFile != 512+command.HandlerCS || len(command.HandlerSHA256) != 64 || len(command.HandlerHex) != 14 && len(command.HandlerHex) != 28 {
			return nil, fmt.Errorf("invalid GS2 command%02x metadata", command.EventID)
		}
		if command.EventID == 0x17 {
			if !command.ClearNoise || command.Channel != 6 || command.Start != 0x97c || command.Alternate != 0x984 || command.HandlerHex != "a3920c8d0e7c098d068409e929fd" {
				return nil, fmt.Errorf("invalid GS2 projectile-view constructor")
			}
		} else if command.ClearNoise || command.Alternate != 0 || len(command.HandlerHex) != 14 {
			return nil, fmt.Errorf("invalid GS2 fixed constructor domain")
		}
		if command.TrackKey != fmt.Sprintf("gs2_%02x", command.EventID) {
			return nil, fmt.Errorf("GS2 fixed track/event domain mismatch")
		}
		matched := false
		for _, track := range meta.Tracks {
			if track.Key == command.TrackKey {
				matched = track.Module == "ASOUND.GS2" && track.EventID == int(command.EventID) && track.PreparationPolicy == "gs2-fixed-command-isolated-dbopl-v1"
				break
			}
		}
		if !matched {
			return nil, fmt.Errorf("GS2 fixed track provenance mismatch%02x", command.EventID)
		}
		assets.GS2Commands[command.EventID] = command
	}
	for _, event := range expected {
		if _, ok := assets.GS2Commands[uint16(event)]; !ok {
			return nil, fmt.Errorf("required GS2 command%02x missing", event)
		}
	}
	for _, key := range requiredAudioKeys {
		if assets.Clips[key] == nil {
			return nil, fmt.Errorf("required audio track missing: %s", key)
		}
	}
	for _, event := range DebriefMusicEvents {
		key := fmt.Sprintf("gs4_%02x", event)
		clip := assets.Clips[key]
		if clip == nil {
			return nil, fmt.Errorf("missing GS4 debrief music %02x", event)
		}
		found := false
		for _, t := range meta.Tracks {
			if t.Key == key {
				found = t.Module == "ASOUND.GS4" && t.EventID == event
			}
		}
		if !found {
			return nil, fmt.Errorf("debrief music source/event mismatch %02x", event)
		}
	}
	return assets, nil
}

func audioWAVPayload(raw []byte) ([]byte, error) {
	for at := 12; at+8 <= len(raw); {
		size := uint64(binary.LittleEndian.Uint32(raw[at+4 : at+8]))
		end := uint64(at) + 8 + size
		if end > uint64(len(raw)) {
			break
		}
		if string(raw[at:at+4]) == "data" {
			return raw[at+8 : int(end)], nil
		}
		at = int(end + (size & 1))
	}
	return nil, fmt.Errorf("WAV data absent")
}

// Schema5 is the immutable Preparation28-and-earlier command family. Schema6
// adds only the two original row2 altitude-warning constructors. Schema7 adds
// the original successful manual chaff/flare fixed constructors. Schema8 adds
// hostile gun29. Schema9 adds source threatwarning06; older families stay frozen.
func expectedGS2GameplayEvents(schema int) []int {
	events := IslandIce469085.AudioLayout.Drivers["GS2"].FixedGameplayEvents
	if schema >= 9 {
		return events
	}
	out := make([]int, 0, len(events))
	for _, event := range events {
		if event != 6 && (schema >= 8 || event != 0x29) && (schema >= 7 || event != 0x0a && event != 0x2a) && (schema >= 6 || event != 4 && event != 5) {
			out = append(out, event)
		}
	}
	return out
}
