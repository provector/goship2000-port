package sounds

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/internal/audioopl"
	"path/filepath"
	"sort"
	"strings"
)

func audioResource(resources []*assetdata.Resource, p *assetdata.AssetVersionProfile, role assetdata.SourceRole) (*assetdata.Resource, error) {
	s := p.Source(role)
	var found *assetdata.Resource
	for _, r := range resources {
		if r.Source.Kind == "loose_file" && strings.EqualFold(filepath.Base(r.Source.Path), s.Name) {
			if assetdata.Hash(r.Payload) != s.SHA256 {
				return nil, fmt.Errorf("%s has an unsupported source identity", s.Name)
			}
			if found != nil {
				return nil, fmt.Errorf("ambiguous duplicate audio source %s", s.Name)
			}
			found = r
		}
	}
	if found == nil {
		return nil, fmt.Errorf("audio source %s is missing", s.Name)
	}
	return found, nil
}

func recognizeAudioProfile(resources []*assetdata.Resource) (*assetdata.AssetVersionProfile, error) {
	p := &assetdata.IslandIce469085
	if p.AudioLayout == nil {
		return nil, fmt.Errorf("audio source layout is unavailable")
	}
	roles := append(append([]assetdata.SourceRole{}, assetdata.AudioSourceRoles...), assetdata.RoleMainExecutable, assetdata.RoleLogoPlayer, assetdata.RoleIntroPlayer, assetdata.RoleSimulationExecutable)
	for _, role := range roles {
		if _, err := audioResource(resources, p, role); err != nil {
			return nil, err
		}
	}
	return p, nil
}

func audioWAV(pcm []byte, rate, bits int) []byte {
	b := make([]byte, 44+len(pcm))
	copy(b, "RIFF")
	binary.LittleEndian.PutUint32(b[4:], uint32(36+len(pcm)))
	copy(b[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(b[16:], 16)
	binary.LittleEndian.PutUint16(b[20:], 1)
	binary.LittleEndian.PutUint16(b[22:], 1)
	binary.LittleEndian.PutUint32(b[24:], uint32(rate))
	binary.LittleEndian.PutUint32(b[28:], uint32(rate*bits/8))
	binary.LittleEndian.PutUint16(b[32:], uint16(bits/8))
	binary.LittleEndian.PutUint16(b[34:], uint16(bits))
	copy(b[36:], "data")
	binary.LittleEndian.PutUint32(b[40:], uint32(len(pcm)))
	copy(b[44:], pcm)
	return b
}

func audioPCM16(samples []int32) ([]byte, uint64, int64) {
	b := make([]byte, len(samples)*2)
	var clips uint64
	last := int64(-1)
	for i, x := range samples {
		if x != 0 {
			last = int64(i)
		}
		if x > 32767 {
			x = 32767
			clips++
		}
		if x < -32768 {
			x = -32768
			clips++
		}
		binary.LittleEndian.PutUint16(b[i*2:], uint16(int16(x)))
	}
	return b, clips, last
}

func prepareAudioResources(out string, resources []*assetdata.Resource, p *assetdata.AssetVersionProfile) (string, error) {
	return prepareAudioResourcesProgress(out, resources, p, nil)
}
func prepareAudioResourcesProgress(out string, resources []*assetdata.Resource, p *assetdata.AssetVersionProfile, progress func(int, int)) (string, error) {
	if p == nil || p.AudioLayout == nil {
		return "", fmt.Errorf("supported audio source profile required")
	}
	// Profile recognition happens before interpreting offsets, including auxiliary
	// players whose source code supplies the scheduling contract.
	recognized, err := recognizeAudioProfile(resources)
	if err != nil {
		return "", err
	}
	if recognized != p {
		return "", fmt.Errorf("audio profile mismatch")
	}
	writer := &assetdata.OutputWriter{Root: out}
	meta := assetdata.AudioMetadata{Schema: assetdata.AudioMetadataSchema, Profile: p.Name, Policy: assetdata.AudioBakePolicy, Component: audioopl.Identity(), Adaptations: []string{
		"Canonical initialized source bytes plus deterministic zero uninitialized storage; source RNG04d2h/decimal1234, no captured DOS memory.",
		"Mono49716Hz signed16LE gain1; no peak normalization, hidden crossfade, or MIDI substitution. Absolute PIT timestamps map by floor(time*rate).",
		"Title uses PLAYER fallback C19886/Q5/resync disabled; not measured original timing. Source busy and PCM release remain separate.",
		"Office repeats a measured canonical second-cycle FM phase/rounded sample period, not continuously evolving chip state.",
		"Page cue is one canonical noise variant. It does not retime background playback; replacement stops old noise/release instead of source shared-noise corruption.",
		"Digital WAVs preserve logical unsigned8bit mono8000Hz source ranges/order, not physical DMA-page artifacts, IRQ gaps or speech scheduling.",
		"GS2 initialization0bh(1) uses its own source ramp through the first clamped steady index21, then command0 reset/release; the indefinite inflight sustain is deliberately excluded.",
		"Early GS2 initialization15h retains its source prelude and repeats one canonical sustained FM cycle until0bh replaces it; PCM phase repetition is an explicit baking adaptation, not continuous chip-state equivalence.",
		"GS4 debrief19h..20h except1dh are music commands baked from source constructors; they use the music gate/volume. On source busy-end, the native owner resumes its canonical office loop rather than retaining arbitrary old OPL release history.",
	}}
	add := func(r *assetdata.Resource, path, kind string, data []byte) (assetdata.Output, error) {
		o, e := writer.Write(path, data)
		if e == nil {
			o.Kind = kind
			meta.Files = append(meta.Files, o)
			r.Outputs = append(r.Outputs, o)
		}
		return o, e
	}
	for _, role := range []assetdata.SourceRole{assetdata.RoleMainExecutable, assetdata.RoleLogoPlayer, assetdata.RoleIntroPlayer, assetdata.RoleSimulationExecutable} {
		r, e := audioResource(resources, p, role)
		if e != nil {
			return "", e
		}
		meta.ContextSources = append(meta.ContextSources, r.Source)
	}
	owners := map[assetdata.SourceRole]*assetdata.Resource{}
	for _, role := range assetdata.AudioSourceRoles {
		r, e := audioResource(resources, p, role)
		if e != nil {
			return "", e
		}
		owners[role] = r
		name := p.Source(role).Name
		raw, e := add(r, "audio/raw/"+name, "audio original carrier", r.Payload)
		if e != nil {
			return "", e
		}
		meta.Carriers = append(meta.Carriers, assetdata.AudioCarrierMetadata{Name: name, Source: r.Source, Raw: raw})
	}
	layouts := p.AudioLayout
	completed, total := 0, 13+len(assetdata.DebriefMusicEvents)+len(layouts.Drivers["GS2"].FixedGameplayEvents)+layouts.DigitalEvents
	mark := func() {
		completed++
		if progress != nil {
			progress(completed, total)
		}
	}
	if progress != nil {
		progress(0, total)
	}
	for _, name := range []string{"LOG", "GS3", "GS4", "GS2"} {
		l := layouts.Drivers[name]
		r := owners[l.Role]
		if l.DSFile >= len(r.Payload) {
			return "", fmt.Errorf("%s short source data", name)
		}
		bank, e := add(r, "audio/tables/"+name+".initialized.bin", "source initialized audio data", r.Payload[l.DSFile:])
		if e != nil {
			return "", e
		}
		type instrument struct {
			Index, FileOffset, Length int
			SHA256, RawHex            string
		}
		instruments := make([]instrument, l.InstrumentCount)
		for i := range instruments {
			at := l.DSFile + 0x50 + i*44
			if at+44 > len(r.Payload) {
				return "", fmt.Errorf("instrument bounds")
			}
			data := r.Payload[at : at+44]
			instruments[i] = instrument{i, at, 44, assetdata.Hash(data), hex.EncodeToString(data)}
		}
		type sourceTable struct {
			Name                 string
			DSOffset, FileOffset int
			RawHex, SHA256       string
		}
		var tables []sourceTable
		for _, v := range []struct {
			name  string
			at, n int
		}{{"pitches", l.Pitches, 24}, {"operator_pairs", l.Pairs, 18}, {"operator_offsets", l.Operators, 18}, {"depth_flags", l.Depth, 3}} {
			at := l.DSFile + v.at
			if at+v.n > len(r.Payload) {
				return "", fmt.Errorf("audio table bounds")
			}
			b := r.Payload[at : at+v.n]
			tables = append(tables, sourceTable{v.name, v.at, at, hex.EncodeToString(b), assetdata.Hash(b)})
		}
		events := make([]int, l.EventCount)
		for i := range events {
			at := 512 + l.EventTableCS + i*2
			if at+2 > len(r.Payload) {
				return "", fmt.Errorf("audio event table bounds")
			}
			events[i] = int(audioWord(r.Payload, at))
		}
		data, _ := assetdata.JsonBytes(struct {
			Module, SourceSHA256 string
			Layout               assetdata.AudioDriverLayout
			Instruments          []instrument
			Tables               []sourceTable
			EventHandlersCS      []int
		}{name, assetdata.Hash(r.Payload), l, instruments, tables, events})
		table, e := add(r, "audio/tables/"+name+".json", "audio source tables/instruments/events", data)
		if e != nil {
			return "", e
		}
		for i := range meta.Carriers {
			if meta.Carriers[i].Name == p.Source(l.Role).Name {
				meta.Carriers[i].InitializedData = &bank
				meta.Carriers[i].Tables = &table
			}
		}
	}
	exportTrack := func(key string, event int, q *audioSequence, samples []int32, busyPIT uint64, clockPolicy string) (assetdata.AudioTrackMetadata, error) {
		pcm, clipped, last := audioPCM16(samples)
		if clipped != 0 {
			return assetdata.AudioTrackMetadata{}, fmt.Errorf("%s clips %d samples; gain policy must be explicit", key, clipped)
		}
		r := owners[q.layout.Role]
		output, e := add(r, "audio/"+key+".wav", "prepared audio WAV", audioWAV(pcm, audioFMRate, 16))
		if e != nil {
			return assetdata.AudioTrackMetadata{}, e
		}
		type controlRange struct {
			DSStart, FileStart, Length int
			SHA256, RawHex             string
		}
		var ranges []controlRange
		for _, v := range q.sourceRanges() {
			at := q.layout.DSFile + v.Start
			end := q.layout.DSFile + v.End
			if end > len(q.source) {
				return assetdata.AudioTrackMetadata{}, fmt.Errorf("source stream range outside initialized carrier")
			}
			b := q.source[at:end]
			ranges = append(ranges, controlRange{v.Start, at, len(b), assetdata.Hash(b), hex.EncodeToString(b)})
		}
		seqdata, e := assetdata.JsonBytes(struct {
			SourceSHA256     string            `json:"source_sha256"`
			SourceRanges     []controlRange    `json:"source_control_ranges"`
			Writes           []audioTimedWrite `json:"ordered_opl_writes"`
			PITFrequency     int               `json:"pit_frequency_convention_hz"`
			Policy           string            `json:"policy"`
			ControlSemantics string            `json:"control_semantics"`
		}{assetdata.Hash(q.source), ranges, q.writes, layouts.PITFrequency, assetdata.AudioBakePolicy, "Source F3..FF operand/control/state semantics retained in converter; original carrier and source ranges are authoritative, writes are a bounded canonical execution."})
		if e != nil {
			return assetdata.AudioTrackMetadata{}, e
		}
		sequence, e := add(r, "audio/tables/"+key+".sequence.json", "audio canonical sequence/control provenance", seqdata)
		if e != nil {
			return assetdata.AudioTrackMetadata{}, e
		}
		track := assetdata.AudioTrackMetadata{Key: key, Module: "ASOUND." + q.layout.Name, EventID: event, Output: output, SampleRate: audioFMRate, Bits: 16, Channels: 1, Frames: uint64(len(samples)), BusyEndFrame: busyPIT * audioFMRate / uint64(layouts.PITFrequency), LastNonzeroFrame: last, TailEndFrame: uint64(len(samples)), ClippedSamples: clipped, OutputGain: 1, PCMSHA256: assetdata.Hash(pcm), BusyEndPIT: busyPIT, RNGSeed: 1234, ClockPolicy: clockPolicy, SourceRanges: q.sourceRanges(), Sequence: sequence}
		return track, nil
	}
	renderFinite := func(key, profile string, event int, tail uint64) (assetdata.AudioTrackMetadata, error) {
		l := layouts.Drivers[profile]
		q, e := newAudioSequence(owners[l.Role].Payload, l, layouts)
		if e != nil {
			return assetdata.AudioTrackMetadata{}, e
		}
		if e = q.command(event); e != nil {
			return assetdata.AudioTrackMetadata{}, e
		}
		for i := 0; i < 30000; i++ {
			q.interrupt()
			if q.err != nil {
				return assetdata.AudioTrackMetadata{}, q.err
			}
			if q.busy() == 0 && q.mem[q.layout.Life]|q.mem[q.layout.Life+1] == 0 {
				break
			}
			if i == 29999 {
				return assetdata.AudioTrackMetadata{}, fmt.Errorf("%s finite cue exceeded diagnostic IRQ bound", key)
			}
		}
		frames := q.sampleFrame() + tail*audioFMRate
		pcm, e := audioopl.Render(audioFMRate, frames, q.componentWrites())
		if e != nil {
			return assetdata.AudioTrackMetadata{}, fmt.Errorf("%s preparation synthesis: %w", key, e)
		}
		quarter := audioFMRate / 4
		if len(pcm) < quarter {
			return assetdata.AudioTrackMetadata{}, fmt.Errorf("%s incomplete audio release", key)
		}
		for _, v := range pcm[len(pcm)-quarter:] {
			if v != 0 {
				return assetdata.AudioTrackMetadata{}, fmt.Errorf("%s audio release remains nonzero; refusing incomplete bake", key)
			}
		}
		policy := "source PIT baseline/fast dividers"
		if profile == "GS3" {
			policy = "PLAYER source fallback C19886/Q5/resync off"
		}
		return exportTrack(key, event, q, pcm, q.pit, policy)
	}
	for _, v := range []struct {
		key, profile string
		event        int
		tail         uint64
	}{{"logo", "LOG", 2, 5}, {"title", "GS3", 2, 5}, {"gs4_0d", "GS4", 0x0d, 1}, {"gs4_0e", "GS4", 0x0e, 1}, {"gs4_21", "GS4", 0x21, 1}, {"gs4_22", "GS4", 0x22, 1}, {"gs4_23", "GS4", 0x23, 1}, {"gs4_24", "GS4", 0x24, 1}, {"gs4_42", "GS4", 0x42, 1}} {
		track, e := renderFinite(v.key, v.profile, v.event, v.tail)
		if e != nil {
			return "", e
		}
		if v.event == 0x0d {
			track.Aliases = []int{0x13}
		}
		meta.Tracks = append(meta.Tracks, track)
		mark()
	}
	for _, event := range assetdata.DebriefMusicEvents {
		var track assetdata.AudioTrackMetadata
		var err error
		if event == 0x1f {
			// GS4's low-result music is an actual FD loop, not a finite cue.
			// Preserve its prelude and one canonical second chip-state cycle.
			l := layouts.Drivers["GS4"]
			q, e := newAudioSequence(owners[l.Role].Payload, l, layouts)
			if e != nil {
				return "", e
			}
			if e = q.command(event); e != nil {
				return "", e
			}
			for i := 0; i < 30000 && len(q.loops[0]) < 3; i++ {
				q.interrupt()
				if q.err != nil {
					return "", q.err
				}
			}
			if len(q.loops[0]) < 3 {
				return "", fmt.Errorf("GS4 result loop missing restart witnesses")
			}
			a, b, c := q.loops[0][0], q.loops[0][1], q.loops[0][2]
			if b-a != c-b {
				return "", fmt.Errorf("GS4 result loop period changed")
			}
			for ch := 0; ch < 6; ch++ {
				v := q.loops[ch]
				if len(v) < 3 || v[0] != a || v[1] != b || v[2] != c {
					return "", fmt.Errorf("GS4 result loop channel ownership")
				}
			}
			start, end := a*audioFMRate/uint64(layouts.PITFrequency), b*audioFMRate/uint64(layouts.PITFrequency)
			pcm, e := audioopl.Render(audioFMRate, q.sampleFrame()+1, q.componentWrites())
			if e != nil {
				return "", e
			}
			track, err = exportTrack("gs4_1f", event, q, pcm[:end], 0, "GS4 source PIT baseline; indefinite six-channel FD loop, canonical second-cycle PCM")
			track.LoopStartFrame, track.LoopEndFrame = start, end
			track.SourcePeriodPIT, track.SourcePeriodDenominator = b-a, uint64(layouts.PITFrequency)
		} else {
			track, err = renderFinite(fmt.Sprintf("gs4_%02x", event), "GS4", event, 1)
		}
		if err != nil {
			return "", err
		}
		track.PreparationPolicy = "gs4-debrief-music-isolated-source-v1"
		meta.Tracks = append(meta.Tracks, track)
		mark()
	}
	// Fixed GS2 commands share row/chip state with the live rotor. These isolated
	// WAVs are reproducible fallback/reference assets, not mixed-history truth.
	gs2Layout := layouts.Drivers["GS2"]
	for _, event := range gs2Layout.FixedGameplayEvents {
		command, e := prepareGS2Command(owners[gs2Layout.Role].Payload, gs2Layout, layouts, event)
		if e != nil {
			return "", e
		}
		var track assetdata.AudioTrackMetadata
		if event == 0x17 {
			q, err := newAudioSequence(owners[gs2Layout.Role].Payload, gs2Layout, layouts)
			if err != nil {
				return "", err
			}
			if err = q.command(event); err != nil {
				return "", err
			}
			for irq := 0; len(q.loops[6]) < 3; irq++ {
				if irq >= 30000 {
					return "", fmt.Errorf("GS2 projectile-view sustain exceeded bounded loop probe")
				}
				q.interrupt()
				if q.err != nil {
					return "", q.err
				}
			}
			bounds := q.loops[6]
			period := bounds[1] - bounds[0]
			if bounds[2]-bounds[1] != period {
				return "", fmt.Errorf("GS2 projectile-view source sustain period changed")
			}
			start, end := bounds[0]*audioFMRate/uint64(layouts.PITFrequency), bounds[1]*audioFMRate/uint64(layouts.PITFrequency)
			pcm, err := audioopl.Render(audioFMRate, q.sampleFrame()+1, q.componentWrites())
			if err != nil {
				return "", err
			}
			track, e = exportTrack(command.TrackKey, event, q, pcm[:end], 0, "GS2 source PIT clock; projectile-view prelude plus canonical sustained cycle until0bh replacement")
			track.LoopStartFrame, track.LoopEndFrame = start, end
			track.SourcePeriodPIT, track.SourcePeriodDenominator = period, uint64(layouts.PITFrequency)
		} else {
			track, e = renderFinite(command.TrackKey, "GS2", event, 5)
		}
		if e != nil {
			return "", e
		}
		track.PreparationPolicy = "gs2-fixed-command-isolated-dbopl-v1"
		meta.Tracks = append(meta.Tracks, track)
		mark()
		meta.GS2Commands = append(meta.GS2Commands, command)
	}
	meta.Adaptations = append(meta.Adaptations, "GS2 combat WAVs are isolated reset-chip reference/fallback executions. Normal live flight dispatch uses their compiled fixed row constructors on the retained rotor's sequencer/chip, preserving row replacement and shared noise state. Audio uses its private source RNG, never gameplay RNG.")
	meta.Adaptations = append(meta.Adaptations, "GS2 projectile-view17h clears retained rotor noise and replaces row6 with a sustained alternate stream until0bh restores it. Its isolated fallback repeats a canonical source-period PCM cycle, not arbitrary live chip phase; the normal shared worker runs continuously.")
	initializationLayout := layouts.Drivers["GS2"]
	early, e := initializationEarlySequence(owners[initializationLayout.Role].Payload, initializationLayout, layouts)
	if e != nil {
		return "", e
	}
	boundaries := early.loops[6]
	period := boundaries[1] - boundaries[0]
	if boundaries[2]-boundaries[1] != period {
		return "", fmt.Errorf("early initialization source loop periods differ")
	}
	earlyPCM, e := audioopl.Render(audioFMRate, early.sampleFrame()+1, early.componentWrites())
	if e != nil {
		return "", fmt.Errorf("early GS2 initialization synthesis: %w", e)
	}
	start, end, next := boundaries[0]*audioFMRate/uint64(layouts.PITFrequency), boundaries[1]*audioFMRate/uint64(layouts.PITFrequency), boundaries[2]*audioFMRate/uint64(layouts.PITFrequency)
	if start == 0 || end <= start || next <= end || next >= uint64(len(earlyPCM)) {
		return "", fmt.Errorf("early initialization PCM loop bounds invalid")
	}
	earlyTrack, e := exportTrack(InitializationEarlyClip, 0x15, early, earlyPCM[:end], 0, "GS2 source PIT baseline; source row6 prelude then canonical second sustained cycle; busy until replacement")
	if e != nil {
		return "", e
	}
	earlyTrack.PreparationPolicy = assetdata.InitializationEarlyBakePolicy
	earlyTrack.LoopStartFrame, earlyTrack.LoopEndFrame = start, end
	earlyTrack.SourcePeriodPIT, earlyTrack.SourcePeriodDenominator = period, uint64(layouts.PITFrequency)
	meta.Tracks = append(meta.Tracks, earlyTrack)
	mark()
	// Compare successive natural synth cycles and expose the actual splice. A
	// logical FD period alone is not proof of identical PCM/chip phase. Do not
	// hide a phase difference with a crossfade or normalize either cycle.
	count := end - start
	if next-end < count {
		count = next - end
	}
	var different uint64
	var maximum int32
	for i := uint64(0); i < count; i++ {
		d := earlyPCM[start+i] - earlyPCM[end+i]
		if d < 0 {
			d = -d
		}
		if d != 0 {
			different++
		}
		if d > maximum {
			maximum = d
		}
	}
	loopWitness, e := assetdata.JsonBytes(struct {
		Policy                                              string
		SourceLoopPIT                                       []uint64
		LoopStartFrame, LoopEndFrame, FollowingEndFrame     uint64
		ComparedFrames, DifferentSamples                    uint64
		MaximumSuccessiveCycleDifference, SpliceSampleDelta int32
		Qualification                                       string
	}{assetdata.InitializationEarlyBakePolicy, boundaries, start, end, next, count, different, maximum, earlyPCM[start] - earlyPCM[end-1], "Canonical-phase repeat, not seamless-PCM or natural chip-state identity; no crossfade. Busy remains state-dependent until source0bh replacement. Native MFD staging controls the replacement time."})
	if e != nil {
		return "", e
	}
	if _, e = add(owners[initializationLayout.Role], "audio/tables/gs2_initialization_early.loop.json", "early initialization loop verification", loopWitness); e != nil {
		return "", e
	}
	initialization, e := initializationSoundSequence(owners[initializationLayout.Role].Payload, initializationLayout, layouts)
	if e != nil {
		return "", e
	}
	initializationPCM, e := audioopl.Render(audioFMRate, initialization.sampleFrame()+5*audioFMRate, initialization.componentWrites())
	if e != nil {
		return "", fmt.Errorf("GS2 initialization synthesis: %w", e)
	}
	for _, sample := range initializationPCM[len(initializationPCM)-audioFMRate/4:] {
		if sample != 0 {
			return "", fmt.Errorf("GS2 initialization release remains nonzero; refusing incomplete bake")
		}
	}
	initializationTrack, e := exportTrack(InitializationStartClip, 0x0b, initialization, initializationPCM, initialization.pit, "GS2 source PIT baseline/fast dividers;0bh argument1; native bounded ramp then source0 reset, no inflight sustain")
	if e != nil {
		return "", e
	}
	initializationTrack.PreparationPolicy = assetdata.InitializationStartBakePolicy
	meta.Tracks = append(meta.Tracks, initializationTrack)
	mark()
	// Find actual FD restart timestamps in this production sequence. The loop
	// ranges are derived again, not copied from Step27's sample-index witnesses.
	l := layouts.Drivers["GS4"]
	q, e := newAudioSequence(owners[l.Role].Payload, l, layouts)
	if e != nil {
		return "", e
	}
	if e = q.command(0x1d); e != nil {
		return "", e
	}
	for i := 0; i < 30000 && len(q.loops[0]) < 3; i++ {
		q.interrupt()
		if q.err != nil {
			return "", q.err
		}
	}
	if len(q.loops[0]) < 3 {
		return "", fmt.Errorf("office song did not reach three source restart boundaries")
	}
	starts := q.loops[0]
	period = starts[1] - starts[0]
	if starts[2]-starts[1] != period {
		return "", fmt.Errorf("office song restart periods differ")
	}
	for ch := 0; ch < 5; ch++ {
		if len(q.loops[ch]) < 3 || q.loops[ch][0] != starts[0] || q.loops[ch][1] != starts[1] || q.loops[ch][2] != starts[2] {
			return "", fmt.Errorf("office music channel%d restart does not share full period", ch)
		}
	}
	pcm, e := audioopl.Render(audioFMRate, q.sampleFrame()+1, q.componentWrites())
	if e != nil {
		return "", fmt.Errorf("office audio synthesis: %w", e)
	}
	start, end = starts[0]*audioFMRate/uint64(layouts.PITFrequency), starts[1]*audioFMRate/uint64(layouts.PITFrequency)
	if start == 0 || end <= start || end >= uint64(len(pcm)) {
		return "", fmt.Errorf("production office loop bounds invalid")
	}
	intro, e := exportTrack("office_intro", 0x1d, q, pcm[:start], 0, "GS4 source PIT baseline; first cycle opening")
	if e != nil {
		return "", e
	}
	intro.SourcePeriodPIT = period
	intro.SourcePeriodDenominator = uint64(layouts.PITFrequency)
	meta.Tracks = append(meta.Tracks, intro)
	mark()
	loop, e := exportTrack("office_loop", 0x1d, q, pcm[start:end], 0, "GS4 source PIT baseline; canonical second cycle phase repeats")
	if e != nil {
		return "", e
	}
	loop.LoopEndFrame = end - start
	loop.SourceStartPIT = starts[0]
	loop.SourcePeriodPIT = period
	loop.SourcePeriodDenominator = uint64(layouts.PITFrequency)
	meta.Tracks = append(meta.Tracks, loop)
	mark()
	if err = exportAudioDigitalProgress(&meta, owners, p, add, mark); err != nil {
		return "", err
	}
	// Verify deterministic manifest ordering regardless of source directory order.
	sort.Slice(meta.Files, func(i, j int) bool { return meta.Files[i].Path < meta.Files[j].Path })
	data, e := assetdata.JsonBytes(meta)
	if e != nil {
		return "", e
	}
	o, e := writer.Write("audio/audio.json", data)
	if e != nil {
		return "", e
	}
	o.Kind = "audio metadata"
	owners[assetdata.RoleAudioGS4].Outputs = append(owners[assetdata.RoleAudioGS4].Outputs, o)
	return o.Path, nil
}

func exportAudioDigital(meta *assetdata.AudioMetadata, owners map[assetdata.SourceRole]*assetdata.Resource, p *assetdata.AssetVersionProfile, add func(*assetdata.Resource, string, string, []byte) (assetdata.Output, error)) error {
	return exportAudioDigitalProgress(meta, owners, p, add, nil)
}
func exportAudioDigitalProgress(meta *assetdata.AudioMetadata, owners map[assetdata.SourceRole]*assetdata.Resource, p *assetdata.AssetVersionProfile, add func(*assetdata.Resource, string, string, []byte) (assetdata.Output, error), completed func()) error {
	a, d, bank := owners[assetdata.RoleAudioADigital], owners[assetdata.RoleAudioDDigital], owners[assetdata.RoleAudioTarget]
	layout := p.AudioLayout
	aa, dd := 512+layout.DigitalListCS, 512+layout.CompanionListCS
	if aa+layout.DigitalListBytes > len(a.Payload) || dd+layout.DigitalListBytes > len(d.Payload) || !bytes.Equal(a.Payload[aa:aa+layout.DigitalListBytes], d.Payload[dd:dd+layout.DigitalListBytes]) {
		return fmt.Errorf("digital companion descriptor blocks differ")
	}
	clips := map[[2]int]bool{}
	export := func(key string, pcm []byte) (assetdata.AudioTrackMetadata, error) {
		last := int64(-1)
		for i, value := range pcm {
			if value != 0x80 {
				last = int64(i)
			}
		}
		o, e := add(bank, "audio/samples/"+key+".wav", "digital logical PCM WAV", audioWAV(pcm, 8000, 8))
		if e != nil {
			return assetdata.AudioTrackMetadata{}, e
		}
		_, e = add(bank, "audio/samples/"+key+".u8", "digital exact raw sample bytes", pcm)
		return assetdata.AudioTrackMetadata{Key: key, Module: "TARGET.BIN/ASOUND.GS2", EventID: -1, Output: o, SampleRate: 8000, Bits: 8, Channels: 1, Frames: uint64(len(pcm)), BusyEndFrame: uint64(len(pcm)), TailEndFrame: uint64(len(pcm)), LastNonzeroFrame: last, SilenceValue: 0x80, OutputGain: 1, PCMSHA256: assetdata.Hash(pcm), ClockPolicy: "unsigned8bit mono8000Hz; logical source ranges; no inserted DMA gaps"}, e
	}
	for event := 0; event < layout.DigitalEvents; event++ {
		if event > 0 && completed != nil {
			completed()
		}
		at := 512 + layout.DigitalEventTableCS + 2*event
		if at+2 > len(a.Payload) {
			return fmt.Errorf("digital event table bounds")
		}
		target := int(audioWord(a.Payload, at))
		f := 512 + target
		if f+7 > len(a.Payload) {
			return fmt.Errorf("digital handler bounds")
		}
		ins := a.Payload[f : f+7]
		if ins[0] != 0xb9 || ins[3] != 0x8d || ins[4] != 0x1e {
			continue
		}
		desc := int(audioWord(ins, 5))
		if desc < layout.DigitalListCS || desc >= layout.DigitalListCS+layout.DigitalListBytes {
			continue
		}
		counterpart := 512 + layout.CompanionEventTableCS + event*2
		if counterpart+2 > len(d.Payload) {
			return fmt.Errorf("companion digital event table bounds")
		}
		ct := 512 + int(audioWord(d.Payload, counterpart))
		if ct+7 > len(d.Payload) || d.Payload[ct] != 0xb9 || d.Payload[ct+3] != 0x8d || d.Payload[ct+4] != 0x1e {
			return fmt.Errorf("companion digital initializer mismatch")
		}
		row := assetdata.AudioDigitalEvent{EventID: event, HandlerCS: target, Priority: int(audioWord(ins, 1)), CompanionPriority: int(audioWord(d.Payload, ct+1)), TrackKey: fmt.Sprintf("event-%02x-logical-list", event), AssemblyPolicy: "Ordered logical source concatenation; repeats retained; no DMA page-wrap bytes, IRQ gaps, filtering or invented utterance labels."}
		var pcm []byte
		ended := false
		for index := 0; index < 32; index++ {
			at := 512 + desc + index*6
			if at+2 > len(a.Payload) {
				return fmt.Errorf("digital descriptor sentinel bounds")
			}
			end := audioWord(a.Payload, at)
			if end == 0 {
				ended = true
				break
			}
			if index >= 2 || at+6 > len(a.Payload) {
				return fmt.Errorf("digital list exceeds supported two-block framing")
			}
			start, paragraph := audioWord(a.Payload, at+2), audioWord(a.Payload, at+4)
			if end <= start {
				return fmt.Errorf("reversed/empty digital sample interval")
			}
			n := int(end-start) & 0xfffe
			bs := int(paragraph)*16 + int(start)
			if n == 0 || bs < 0 || bs+n > len(bank.Payload) {
				return fmt.Errorf("digital sample outside bank")
			}
			b := bank.Payload[bs : bs+n]
			row.Blocks = append(row.Blocks, assetdata.AudioDigitalBlock{EndWord: end, StartWord: start, ParagraphWord: paragraph, BankStart: bs, BankEnd: bs + n, DescriptorFileOffset: at, PCMHash: assetdata.Hash(b)})
			key := [2]int{bs, bs + n}
			if !clips[key] {
				track, e := export(fmt.Sprintf("sample-%04x-%04x", bs, bs+n), b)
				if e != nil {
					return e
				}
				meta.Tracks = append(meta.Tracks, track)
				clips[key] = true
			}
			pcm = append(pcm, b...)
		}
		if !ended || len(row.Blocks) != 2 {
			return fmt.Errorf("unterminated/short digital event list")
		}
		track, e := export(row.TrackKey, pcm)
		if e != nil {
			return e
		}
		track.EventID = event
		meta.Tracks = append(meta.Tracks, track)
		meta.DigitalEvents = append(meta.DigitalEvents, row)
	}
	if completed != nil {
		completed()
	}
	if len(clips) != 17 || len(meta.DigitalEvents) != 22 {
		return fmt.Errorf("digital source count differs: %d unique/%d lists", len(clips), len(meta.DigitalEvents))
	}
	return nil
}
