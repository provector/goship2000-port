package sounds

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/internal/testfixture"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func audioTestSequence(t *testing.T, name string) *audioSequence {
	t.Helper()
	l := assetdata.IslandIce469085.AudioLayout.Drivers[name]
	source, err := os.ReadFile(filepath.Join(testfixture.Path("installation"), assetdata.IslandIce469085.Source(l.Role).Name))
	if os.IsNotExist(err) {
		t.Skip("original audio test source unavailable")
	}
	if err != nil {
		t.Fatal(err)
	}
	if assetdata.Hash(source) != assetdata.IslandIce469085.Source(l.Role).SHA256 {
		t.Fatal("test source identity differs")
	}
	q, err := newAudioSequence(source, l, assetdata.IslandIce469085.AudioLayout)
	if err != nil {
		t.Fatal(err)
	}
	return q
}

func audioRunFinite(t *testing.T, q *audioSequence, event int) {
	t.Helper()
	if err := q.command(event); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 30000; i++ {
		q.interrupt()
		if q.err != nil {
			t.Fatal(q.err)
		}
		if q.busy() == 0 {
			return
		}
	}
	t.Fatal("finite diagnostic bound reached")
}

func audioReferenceWrites(t *testing.T, name string) []audioTimedWrite {
	t.Helper()
	raw, err := os.ReadFile(testfixture.Path(name))
	if os.IsNotExist(err) {
		t.Skip("qualified research reference unavailable")
	}
	if err != nil {
		t.Fatal(err)
	}
	type row struct {
		Kind     string   `json:"kind"`
		PIT      uint64   `json:"pit_clocks"`
		Register byte     `json:"register"`
		Value    int      `json:"value"`
		Time     []uint64 `json:"time_fraction"`
	}
	var rows []row
	if bytes.HasPrefix(bytes.TrimSpace(raw), []byte("[")) {
		if err = json.Unmarshal(raw, &rows); err != nil {
			t.Fatal(err)
		}
	} else {
		s := bufio.NewScanner(bytes.NewReader(raw))
		for s.Scan() {
			var r row
			if err = json.Unmarshal(s.Bytes(), &r); err != nil {
				t.Fatal(err)
			}
			rows = append(rows, r)
		}
		if err = s.Err(); err != nil {
			t.Fatal(err)
		}
	}
	var out []audioTimedWrite
	for _, r := range rows {
		if r.Kind == "opl_write" {
			pit := r.PIT
			if len(r.Time) == 2 {
				numerator := r.Time[0] * 1193182
				if numerator%r.Time[1] != 0 {
					t.Fatal("nonintegral source PIT reference")
				}
				pit = numerator / r.Time[1]
			}
			out = append(out, audioTimedWrite{pit, r.Register, byte(r.Value)})
		}
	}
	return out
}

func compareAudioReference(t *testing.T, actual, expected []audioTimedWrite) {
	t.Helper()
	for i := 0; i < len(actual) && i < len(expected); i++ {
		if actual[i] != expected[i] {
			t.Fatalf("first source/model divergence at%d: production%+v reference%+v", i, actual[i], expected[i])
		}
	}
	if len(actual) != len(expected) {
		t.Fatalf("write count production%d reference%d", len(actual), len(expected))
	}
}

func TestAudioLiteralSourceVectorsAndSeed(t *testing.T) {
	// Literal stream bytes/row5 jumps independently transcribed from original
	// instruction windows, not calculated by the sequencer being tested.
	vectors := []struct {
		event, ds, end int
		raw            string
	}{{0x0d, 0x1158, 9, "fc0ff93c54080000"}, {0x0e, 0x1160, 13, "fc0ff93c5404540454040000"}, {0x13, 0x1158, 9, "fc0ff93c54080000"}, {0x21, 0x11f8, 11, "fc14f934100a0000"}, {0x22, 0x11f0, 9, "fc0ef93048080000"}, {0x23, 0x1218, 31, "fc29f9370c1e0000"}, {0x24, 0x11e8, 11, "fc27f92d090a0000"}, {0x42, 0x1220, 31, "fc2ef937181e0000"}}
	for _, v := range vectors {
		t.Run(hex.EncodeToString([]byte{byte(v.event)}), func(t *testing.T) {
			q := audioTestSequence(t, "GS4")
			if q.word(q.layout.RNG) != 1234 {
				t.Fatal("source seed must be decimal1234/04d2, not1234hex")
			}
			b, _ := hex.DecodeString(v.raw)
			if !bytes.Equal(q.source[q.layout.DSFile+v.ds:q.layout.DSFile+v.ds+len(b)], b) {
				t.Fatal("original stream bytes differ")
			}
			handler := int(audioWord(q.source, 512+q.layout.EventTableCS+v.event*2))
			ins := q.source[512+handler : 512+handler+7]
			if !bytes.Equal(ins[:4], []byte{0x8d, 0x0e, byte(v.ds), byte(v.ds >> 8)}) || ins[4] != 0xe9 || handler+7+int(int16(audioWord(ins, 5))) != 0x6cc {
				t.Fatal("original row5 constructor differs")
			}
			audioRunFinite(t, q, v.event)
			if int(q.ticks) != v.end {
				t.Fatalf("source duration+firstservice expected%d got%d", v.end, q.ticks)
			}
		})
	}
}

func TestAudioCanonicalCueRegisterReferences(t *testing.T) {
	for _, event := range []int{0x0d, 0x0e, 0x21, 0x22, 0x23, 0x24, 0x42} {
		t.Run(hex.EncodeToString([]byte{byte(event)}), func(t *testing.T) {
			q := audioTestSequence(t, "GS4")
			audioRunFinite(t, q, event)
			expected := audioReferenceWrites(t, filepath.Join("audio-reference/sequencing/canonical-cues-source-seed02", "event-"+hex.EncodeToString([]byte{byte(event)}), "events.jsonl"))
			compareAudioReference(t, q.writes, expected)
		})
	}
}

func TestAudioLogoAndTitleRegisterReferences(t *testing.T) {
	t.Run("logo", func(t *testing.T) {
		q := audioTestSequence(t, "LOG")
		audioRunFinite(t, q, 2)
		if q.ticks != 401 {
			t.Fatal("source logical end", q.ticks)
		}
		compareAudioReference(t, q.writes, audioReferenceWrites(t, "audio-reference/synthesis/logo-release-only01/events-no-final-reset.json"))
	})
	t.Run("title", func(t *testing.T) {
		q := audioTestSequence(t, "GS3")
		audioRunFinite(t, q, 2)
		if q.ticks != 3842 {
			t.Fatal("source logical end", q.ticks)
		}
		compareAudioReference(t, q.writes, audioReferenceWrites(t, "audio-reference/title-canonical-fallback01/audio.jsonl"))
	})
}

func TestAudioRNGAndRepeatAndBoundaryRules(t *testing.T) {
	q := audioTestSequence(t, "GS4")
	if got := q.rng(); got != 0x52e3 {
		t.Fatalf("hand-derived04d2+9248 rotatedright3=%04x", got)
	}
	if got := q.rng(); got != 0x7ca5 {
		t.Fatalf("secondhanddraw%04x", got)
	}
	q.mem[0x5000] = 0xfd
	q.known[0x5000] = true
	q.initialize(0, 0x5000, 0)
	q.putword(q.base(0)+16, 0xffff)
	q.known[0xffff] = false
	q.update(0)
	if q.err == nil {
		t.Fatal("uninitialized source cursor accepted")
	}
	if _, err := newAudioSequence([]byte("MZ"), q.layout, q.clocks); err == nil {
		t.Fatal("short source accepted")
	}
}

func TestAudioDigitalWAVExactUnsignedBytes(t *testing.T) {
	raw := []byte{0, 127, 128, 255, 128}
	wav := audioWAV(raw, 8000, 8)
	if string(wav[:4]) != "RIFF" || string(wav[8:12]) != "WAVE" || binary.LittleEndian.Uint32(wav[24:28]) != 8000 || binary.LittleEndian.Uint16(wav[34:36]) != 8 || !bytes.Equal(wav[44:], raw) {
		t.Fatal("U8 source bytes or rate changed")
	}
}

func TestAudioPreparationProfileRejectsUnknownWithoutOutputs(t *testing.T) {
	var resources []*assetdata.Resource
	roles := append(append([]assetdata.SourceRole{}, assetdata.AudioSourceRoles...), assetdata.RoleMainExecutable, assetdata.RoleLogoPlayer, assetdata.RoleIntroPlayer, assetdata.RoleSimulationExecutable)
	for _, role := range roles {
		s := assetdata.IslandIce469085.Source(role)
		raw, err := os.ReadFile(filepath.Join(testfixture.Path("installation"), s.Name))
		if os.IsNotExist(err) {
			t.Skip("original unavailable")
		}
		if err != nil {
			t.Fatal(err)
		}
		resources = append(resources, &assetdata.Resource{Source: assetdata.SourceLocation{Path: s.Name, Kind: "loose_file", SHA256: assetdata.Hash(raw), PayloadSHA256: assetdata.Hash(raw)}, Payload: raw})
	}
	p, err := recognizeAudioProfile(resources)
	if err != nil || p != &assetdata.IslandIce469085 {
		t.Fatal(err)
	}
	resources[0].Payload = bytes.Clone(resources[0].Payload)
	resources[0].Payload[20] ^= 1
	if _, err = recognizeAudioProfile(resources); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatal(err)
	}
	for _, r := range resources {
		if !reflect.DeepEqual(r.Outputs, []assetdata.Output(nil)) {
			t.Fatal("recognition wrote outputs")
		}
	}
}

func TestAudioProductionBakeReferenceAgreement(t *testing.T) {
	var resources []*assetdata.Resource
	for _, role := range append(append([]assetdata.SourceRole{}, assetdata.AudioSourceRoles...), assetdata.RoleMainExecutable, assetdata.RoleLogoPlayer, assetdata.RoleIntroPlayer, assetdata.RoleSimulationExecutable) {
		s := assetdata.IslandIce469085.Source(role)
		raw, err := os.ReadFile(filepath.Join(testfixture.Path("installation"), s.Name))
		if os.IsNotExist(err) {
			t.Skip("original sources unavailable")
		}
		if err != nil {
			t.Fatal(err)
		}
		resources = append(resources, &assetdata.Resource{Source: assetdata.SourceLocation{Path: s.Name, Kind: "loose_file", SHA256: assetdata.Hash(raw), PayloadSHA256: assetdata.Hash(raw)}, Payload: raw})
	}
	out := t.TempDir()
	name, err := prepareAudioResources(out, resources, &assetdata.IslandIce469085)
	if err != nil {
		t.Fatal(err)
	}
	if name != "audio/audio.json" {
		t.Fatal(name)
	}
	b, err := os.ReadFile(filepath.Join(out, name))
	if err != nil {
		t.Fatal(err)
	}
	var meta assetdata.AudioMetadata
	if err = json.Unmarshal(b, &meta); err != nil {
		t.Fatal(err)
	}
	if meta.Schema != 9 || len(meta.Tracks) != 79 || len(meta.DigitalEvents) != 22 {
		t.Fatalf("expected40FM+39digital including source hostile-gun29 and warning06; got schema%d tracks%d/digitallists%d", meta.Schema, len(meta.Tracks), len(meta.DigitalEvents))
	}
	// This is an independent source-domain list, not the production admission
	// slice used by the baker. P.7 adds only the source0a/2a fixed constructors.
	wantFixed := map[uint16]bool{3: true, 4: true, 5: true, 6: true, 7: true, 8: true, 9: true, 0xa: true, 0xf: true, 0x10: true, 0x11: true, 0x14: true, 0x17: true, 0x18: true, 0x25: true, 0x27: true, 0x28: true, 0x29: true, 0x2a: true, 0x4a: true}
	for _, c := range meta.GS2Commands {
		if !wantFixed[c.EventID] {
			t.Fatal("unexpected/duplicate fixed source event", c.EventID)
		}
		delete(wantFixed, c.EventID)
	}
	if len(wantFixed) != 0 {
		t.Fatal("missing fixed source events", wantFixed)
	}
	for _, o := range meta.Files {
		b, e := os.ReadFile(filepath.Join(out, o.Path))
		if e != nil || len(b) != o.Size || assetdata.Hash(b) != o.SHA256 {
			t.Fatalf("output identity%s: %v", o.Path, e)
		}
	}
	for _, track := range meta.Tracks {
		raw, e := os.ReadFile(filepath.Join(out, track.Output.Path))
		if e != nil {
			t.Fatal(e)
		}
		if track.Output.SHA256 != assetdata.Hash(raw) || len(raw) != 44+int(track.Frames)*track.Bits/8 || !bytes.Equal(raw[:4], []byte("RIFF")) {
			t.Fatalf("badWAV%s", track.Key)
		}
		var ref string
		switch track.Key {
		case "logo":
			ref = "audio-reference/synthesis/logo-release-render01/diagnostic.wav"
		case "title":
			ref = "audio-reference/title-canonical-render01/diagnostic.wav"
		case "office_intro":
			ref = "audio-reference/office-loop-candidate01/office-intro.canonical.wav"
		case "office_loop":
			ref = "audio-reference/office-loop-candidate01/office-loop.canonical-phase.wav"
		case InitializationStartClip:
			// New source-checked GS2 family has no historical Step27 PCM witness.
			// First-note/ramp hand cases are separate independent checks.
			if track.Module != "ASOUND.GS2" || track.EventID != 0x0b || track.BusyEndPIT != 5742800 || track.BusyEndFrame != 239283 || track.Frames != 487863 {
				t.Fatal("missing bounded GS2 provenance")
			}
			if assetdata.Hash(raw) != "1cefbf85eada55a769e608ee0072f8c52254fd129c3f0d03c2fbf05abfbc2c60" {
				t.Fatal("previously delivered rotor WAV changed")
			}
			t.Logf("%s: new GS2 source-checked bounded bake %s; frames%d busy%d; no historical PCM reference", track.Key, assetdata.Hash(raw), track.Frames, track.BusyEndFrame)
			continue
		case InitializationEarlyClip:
			// Independent hand boundaries: updates511/766/1021, reload4c90h,
			// absolute floor at49716/1193182. This is a canonical PCM phase,
			// not a claim that a logical FD loop has identical OPL state.
			if track.Module != "ASOUND.GS2" || track.EventID != 0x15 || track.LoopStartFrame != 417317 || track.LoopEndFrame != 625567 || track.Frames != 625567 || track.SourcePeriodPIT != 4998000 || track.PreparationPolicy != assetdata.InitializationEarlyBakePolicy {
				t.Fatal("early initialization source loop provenance", track)
			}
			witness, e := os.ReadFile(filepath.Join(out, "audio/tables/gs2_initialization_early.loop.json"))
			if e != nil {
				t.Fatal(e)
			}
			t.Logf("early initialization WAV%s; loop417317..625567; source/hand checked, not original audible reference; witness%s", assetdata.Hash(raw), witness)
			continue
		default:
			if track.PreparationPolicy == "gs4-debrief-music-isolated-source-v1" {
				if track.Module != "ASOUND.GS4" || track.EventID < 0x19 || track.EventID > 0x20 || track.EventID == 0x1d || track.ClippedSamples != 0 || track.Frames <= audioFMRate {
					t.Fatal("GS4 debrief isolated bake source domain", track)
				}
				// No Step27 PCM golden exists for these newly reached commands.
				// Original constructors and bounded stream semantics are checked
				// separately; never bless current output as original audibility.
				continue
			}
			if strings.HasPrefix(track.Key, "gs2_") {
				// New fixed GS2 bakes have raw-operand/row/retained-state checks,
				// not an invented Step27 PCM golden or original audible witness.
				if track.Module != "ASOUND.GS2" || track.PreparationPolicy != "gs2-fixed-command-isolated-dbopl-v1" || track.ClippedSamples != 0 || track.Frames <= 5*audioFMRate {
					t.Fatal("GS2 gameplay isolated-bake provenance", track)
				}
				continue
			}
			if strings.HasPrefix(track.Key, "gs4_") {
				ref = filepath.Join("audio-reference/sequencing/canonical-cues-source-seed02", "event-"+strings.TrimPrefix(track.Key, "gs4_"), "render-tail-1s/diagnostic.wav")
			} else {
				ref = filepath.Join("audio-reference/samples/export01", track.Key+".wav")
			}
		}
		expected, e := os.ReadFile(testfixture.Path(ref))
		if e != nil {
			t.Fatal(e)
		}
		t.Logf("%s: production %s reference %s frames %d", track.Key, assetdata.Hash(raw), assetdata.Hash(expected), track.Frames)
		if !bytes.Equal(raw, expected) {
			t.Errorf("productionbake%s differsreference: prod%s ref%s frames%d", track.Key, assetdata.Hash(raw), assetdata.Hash(expected), track.Frames)
		}
		if track.Key == "gs4_0d" && !reflect.DeepEqual(track.Aliases, []int{0x13}) {
			t.Fatal("knobaliasmissing")
		}
		if track.Key == "office_loop" && (track.SourcePeriodPIT != 57271200 || track.SourcePeriodDenominator != 1193182 || track.Frames != 2386304 || track.LoopEndFrame != track.Frames) {
			t.Fatal("source-derivedloopmetadata differs", track)
		}
	}
	// The first original END2786/START1 descriptor is deliberately even-rounded.
	found := false
	for _, event := range meta.DigitalEvents {
		for _, block := range event.Blocks {
			if block.StartWord == 1 && block.EndWord == 2786 {
				found = true
				if block.BankStart != 1 || block.BankEnd != 2785 {
					t.Fatal("inclusive-end or roundingerror", block)
				}
			}
		}
	}
	if !found {
		t.Fatal("independentdigital descriptor witnessmissing")
	}
}
