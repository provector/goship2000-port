package assets

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"gunship2000-assets/internal/audioopl"
	"io"
	"path"
	"sort"
	"strings"
	"sync"
)

// LogicalResourceProfileVersion pins the canonicalization as well as the full
// prepared interpretation. Physical paths remain untouched in the manifest.
const LogicalResourceProfileVersion = "logical-resource-profile-v1"

// These complete-profile fingerprints are established from byte-preserved
// references and fresh raw preparation. They are semantic profiles, not a list
// of folder spellings. A changed table/media/interpretation is not auto-admitted.
const LogicalPreparation24 = "2a962e98243ed29ecd40cd075f731d803728a499f426a19884d0dda988d8d52f"
const LogicalPreparation25 = "0e992514fc9534f742ba08092f5f6b16e95b4f26314b9e5afe21c49e4f0c4f01"
const LogicalPreparation26 = "dd7a8afab36fd63be9191977dc8894a571e2da3afabb14867f10cc763fc71ab2"

const LogicalPreparation27 = "187fb4162a09a23a07a328b682f5798ed87ba6ac56c4aa0c1a5a25f819ee3a2d"

// Candidate28 is retained only for independently verified diagnostic films.
// Current UI requires the later FORM/text capability; the media are identical.
const LogicalPreparation28Candidate = "e587caeb62c1b45ca6087b22ecb5d0204ad2a666bd1b719e81fb8cf61d82b4bd"
const LogicalPreparation28 = "e232326af8e9b418d364a6e0e9e9c4185e5963aa7f30a9c2f4cab3fba8c7b338"

// Preparation29 adds source feedback variants, warning constructors and typed
// death-history leaves. Complete prior media/field equality is recorded under
// evidence/stepP4/presentation/preparation29-comparison.json.
const LogicalPreparation29Candidate = "b32feea506e4cab7668829d52fb1ca8ce6fa42e466c478181a71f2a8d59a936c"
const LogicalPreparation29 = "90171ab56a05e90577fcf0650f4ae4ef49ceec904eab88f97d39c20ac9242ce9"

// Preparation30 adds only the original GPS Support type/count UI literals.
// Full inventory and prior media/table equality: evidence/stepP6/preparation30-comparison.json.
const LogicalPreparation30 = "a612576035e73b5c72e34046a267808e89585bb3f888665597f78c71f9ead24a"

// Preparation31 adds exact countermeasure constructors/PCM and source Delete
// text. All prior media/tables remain byte-identical; see the P7 comparison.
const LogicalPreparation31 = "1320ab2821ddf14a0ee66fb50b7803f5444800c4dfeb6ea6a7c7f18ec6ad743c"

// Candidate32 preceded the connected discovery of missing hostile gun cue29.
// Keep the complete diagnostic interpretation readable without admitting it as
// current startup capability. It was never a human-accepted release.
const LogicalPreparation32Candidate = "dd49dd6c1eddec1c3a64aada73342470b9b61e2af8703ff72d9f9624abe85c1a"

// Preparation32 adds immutable world ENEM and the source hostile gun cue29.
// Prior media/interpretations are verified by the complete31 projection.
const LogicalPreparation32 = "1cab1f436a2201c3ea9005db77a3087b512824ec43025207494adcb3c7ac670b"

// Preparation33 corrects only the exact known PHIL1 GPS atlas conversion.
// Its complete profile normalizes the bad/clean original carrier pair after
// validating every physical output; source files and manifest hashes stay raw.
const LogicalPreparation33 = "e19ce505e1d1ab71f9d2f7b7d206bc8ee1e5cac0f1a71698149a29ba782c8bad"

const LogicalPreparation34 = "bab62032ddbbbd781f402a7500689778e8c8c9785b98520b9f42095a91eced6e"

// A process can reuse decoded validation only after opening and hashing every pack
// and checking the exact manifest and catalogue of a previously verified generation.
var verifiedGeneration struct {
	sync.Mutex
	key string
}

func LogicalResourceProfile(root string) (string, error) {
	r, e := newPreparedReader(root)
	if e != nil {
		return "", e
	}
	defer r.close()
	raw, e := r.read("manifest.json", 64<<20)
	if e != nil {
		return "", e
	}
	var m Manifest
	if e = DecodeObject(raw, &m); e != nil {
		return "", fmt.Errorf("logical resource manifest: %w", e)
	}
	if !compatiblePreparation(m) || m.VersionProfile != IslandIce469085.Name || m.FatalError != "" {
		return "", fmt.Errorf("unsupported logical resource preparation/profile")
	}
	if r.packs != nil {
		cacheKey := Hash(raw) + r.packs.catalogHash
		verifiedGeneration.Lock()
		match := cacheKey == verifiedGeneration.key
		verifiedGeneration.Unlock()
		if match {
			return LogicalPreparation34, nil
		}
		data, err := canonicalLogicalResourceDataMode(r, &m, true)
		if err != nil {
			return "", err
		}
		if m.PreparationVersion != 34 || Hash(data) != decodedPreparation34 {
			return "", fmt.Errorf("packed logical resource identity differs from Preparation34")
		}
		verifiedGeneration.Lock()
		verifiedGeneration.key = cacheKey
		verifiedGeneration.Unlock()
		return LogicalPreparation34, nil
	}
	hash, e := canonicalLogicalResourceProfile(r, &m)
	if e != nil {
		return "", e
	}
	expected := map[int]string{24: LogicalPreparation24, 25: LogicalPreparation25, 26: LogicalPreparation26, 27: LogicalPreparation27, 28: LogicalPreparation28, 29: LogicalPreparation29, 30: LogicalPreparation30, 31: LogicalPreparation31, 32: LogicalPreparation32, 33: LogicalPreparation33, 34: LogicalPreparation34}[m.PreparationVersion]
	if m.PreparationVersion == 28 && hash == LogicalPreparation28Candidate {
		return hash, nil // Exact whole inventory, not a path/manifest bypass.
	}
	if m.PreparationVersion == 29 && hash == LogicalPreparation29Candidate {
		return hash, nil
	} // Complete diagnostic profile; current UI additionally requires Support cursor.
	if m.PreparationVersion == 32 && hash == LogicalPreparation32Candidate {
		return hash, nil
	}
	if expected != "" && hash == expected {
		return hash, nil
	}
	// Different lossless PNG encoders can frame identical prepared images
	// differently. Admit only an independently pinned COMPLETE decoded profile
	// profile, after the same original manifest/output checks. Existing films
	// and accepted v1 IDs remain immutable; there is no cache-hash whitelist.
	if decodedExpected := map[int]string{31: decodedPreparation31, 32: decodedPreparation32, 33: decodedPreparation33, 34: decodedPreparation34}[m.PreparationVersion]; decodedExpected != "" {
		data, err := canonicalLogicalResourceDataMode(r, &m, true)
		if err != nil {
			return "", err
		}
		if Hash(data) == decodedExpected {
			return expected, nil
		}
		if m.PreparationVersion == 32 && Hash(data) == decodedPreparation32Candidate {
			return LogicalPreparation32Candidate, nil
		}
	}
	return "", fmt.Errorf("unsupported logical resource content for Preparation%d (%s)", m.PreparationVersion, hash)
}

// These original DOS working files and user-authored legacy documents are not
// constructor inputs in the supported native profile. Generic extraction and
// world.Bundle.Sources retain their provenance; native mission facts come from
// accepted preparation/launch carriers. Integrity still verifies their outputs.
func nonSemanticOriginalWorkingFile(name string) bool {
	name = strings.ToUpper(path.Base(name))
	switch name {
	case "ENEMY.DAT", "OBJECT.DAT", "PATH.DAT", "SCORE.DAT", "SHIP.DAT", "SWITCHES.DAT", "TARGET.DAT", "WEAPON.DAT", "WRAPPER.DAT", "REPLAY", "ROSTER.DAT", "SUMMARY.TXT":
		return true
	}
	switch path.Ext(name) {
	case ".REP", ".RPL", ".SIM":
		return true
	}
	return false
}

type logicalProfileOutput struct {
	out    Output
	data   []byte
	digest string
	size   int
	active bool
	done   bool
}

func canonicalLogicalResourceProfile(reader *preparedReader, m *Manifest) (string, error) {
	b, e := canonicalLogicalResourceData(reader, m)
	if e != nil {
		return "", e
	}
	return Hash(b), nil
}
func canonicalLogicalResourceData(reader *preparedReader, m *Manifest) ([]byte, error) {
	return canonicalLogicalResourceDataMode(reader, m, false)
}
func canonicalLogicalResourceDataMode(reader *preparedReader, m *Manifest, decodedMedia bool) ([]byte, error) {
	inst, e := ResolveSourceInstallation(m)
	if e != nil {
		return nil, fmt.Errorf("logical resource installation: %w", e)
	}
	// Only a verified single installation root authorizes removing a prefix. No
	// suffix/basename search changes arbitrary provenance or resolves ambiguity.
	logicalSource := func(p string) (string, error) {
		if inst.Root == "" || inst.Root == "." {
			if path.IsAbs(p) || path.Clean(p) != p || strings.Contains(p, "\\") {
				return "", fmt.Errorf("unsafe logical source %q", p)
			}
			return p, nil
		}
		prefix := inst.Root + "/"
		if !strings.HasPrefix(p, prefix) {
			return "", fmt.Errorf("source %q is outside verified installation %s", p, inst.Root)
		}
		return strings.TrimPrefix(p, prefix), nil
	}
	paths := map[string]string{}
	add := func(physical, logical string) error {
		if v, ok := paths[physical]; ok && v != logical {
			return fmt.Errorf("ambiguous logical path %q", physical)
		}
		paths[physical] = logical
		return nil
	}
	outputs := map[string]*logicalProfileOutput{}
	if len(m.Resources) > 65536 || len(m.Containers) > maxRecords {
		return nil, fmt.Errorf("logical resource inventory exceeds bounds")
	}
	total := int64(0)
	for _, c := range m.Containers {
		v, e := logicalSource(c.Path)
		if e != nil {
			return nil, e
		}
		if e = add(c.Path, v); e != nil {
			return nil, e
		}
	}
	for _, s := range m.Resources {
		if s == nil {
			return nil, fmt.Errorf("null logical resource")
		}
		v, e := logicalSource(s.Source.Path)
		if e != nil {
			return nil, e
		}
		if e = add(s.Source.Path, v); e != nil {
			return nil, e
		}
		if s.Source.Container != "" {
			v, e = logicalSource(s.Source.Container)
			if e != nil {
				return nil, e
			}
			if e = add(s.Source.Container, v); e != nil {
				return nil, e
			}
		}
		export := s.ExportPath
		if export != "" {
			if s.Source.Kind == "archive_entry" {
				if s.Source.Record == nil {
					return nil, fmt.Errorf("archive resource lacks raw member")
				}
				raw, e := hex.DecodeString(s.Source.Record.NameHex)
				if e != nil || len(raw) != 12 {
					return nil, fmt.Errorf("archive member raw name invalid")
				}
				name := bytes.SplitN(raw, []byte{0}, 2)[0]
				export = escapedPath([]byte(v)) + "/" + escapedPath(name)
			} else if s.Source.Kind == "loose_file" {
				export = "loose/" + escapedPath([]byte(v))
			}
			if e = add(s.ExportPath, export); e != nil {
				return nil, e
			}
		}
		for _, o := range s.Outputs {
			if o.Size < 0 || o.Size > maxTotalBytes || len(o.SHA256) != 64 {
				return nil, fmt.Errorf("invalid output framing %s", o.Path)
			}
			if old, ok := outputs[o.Path]; ok {
				if old.out != o {
					return nil, fmt.Errorf("conflicting logical output %s", o.Path)
				}
				continue
			}
			name := o.Path
			if s.ExportPath != "" && strings.HasPrefix(name, s.ExportPath) {
				name = export + strings.TrimPrefix(name, s.ExportPath)
			}
			if e = add(o.Path, name); e != nil {
				return nil, e
			}
			total += int64(o.Size)
			if total > maxTotalBytes || len(outputs) >= 65536 {
				return nil, fmt.Errorf("logical resource outputs exceed bounds")
			}
			outputs[o.Path] = &logicalProfileOutput{out: o}
		}
	}
	if reader.packs != nil {
		// The supported runtime subset was measured from the complete Preparation34
		// frontend on a flat installation. Normalize nested source prefixes first.
		names := make([]string, 0, len(reader.packs.catalog.Files))
		for p := range reader.packs.catalog.Files {
			// W3 additionally keeps this one verified source member. Its
			// content still participates in the complete logical profile below.
			if o := outputs[p]; paths[p] == "MBUILDER.CAT/PHIL3MIS.PIC" && o != nil && o.out.Kind == "original_payload" && o.out.SHA256 == phil3EditorMemberSHA && o.out.Size == 9615 {
				continue
			}
			names = append(names, paths[p])
		}
		sort.Strings(names)
		if Hash([]byte(strings.Join(names, "\n"))) != "125d90d7bf5cebed9757ffd7cc5959f11946b5666d8fa736e16bdfe133db99ba" {
			return nil, fmt.Errorf("prepared runtime entry set differs from Preparation34")
		}
	}
	var normalize func(any, string) (any, error)
	var finish func(string) (*logicalProfileOutput, error)
	finish = func(p string) (*logicalProfileOutput, error) {
		o := outputs[p]
		if o == nil {
			return nil, fmt.Errorf("logical profile missing output %s", p)
		}
		if o.done {
			return o, nil
		}
		if o.active {
			return nil, fmt.Errorf("cyclic output metadata %s", p)
		}
		o.active = true
		if reader.packs != nil {
			if proof, ok := reader.packs.catalog.Omitted[p]; ok {
				o.digest, o.size, o.done, o.active = proof.Hash, proof.Size, true, false
				return o, nil
			}
			if media := reader.packs.catalog.Files[p]; media != nil && (media.Codec == "flac" || media.Codec == "frames") {
				if err := reader.packs.verifyMedia(p, o.out); err != nil {
					return nil, err
				}
				o.digest, o.size, o.done, o.active = o.out.SHA256, o.out.Size, true, false
				return o, nil
			}
		}
		b, err := reader.read(p, maxTotalBytes)
		if err != nil {
			return nil, err
		}
		if len(b) != o.out.Size || Hash(b) != o.out.SHA256 {
			return nil, fmt.Errorf("logical profile output %s size/SHA-256 differs", p)
		}

		if m.PreparationVersion >= 33 && strings.EqualFold(path.Base(p), "PHIL1BMP.GS2") {
			// Physical size/hash validation already occurred above. Only the
			// complete known pair is semantically equivalent in this profile.
			corrected, _, err := CorrectKnownMapAtlas("PHIL1BMP.GS2", b)
			if err != nil {
				return nil, err
			}
			b = corrected
		}
		if decodedMedia && preparedPNGOutput(o.out) {
			var err error
			b, err = decodedPNGIdentity(b)
			if err != nil {
				return nil, fmt.Errorf("logical PNG %s: %w", p, err)
			}
		} else if strings.HasSuffix(p, ".json") {
			var v any
			d := json.NewDecoder(bytes.NewReader(b))
			d.UseNumber()
			if e := d.Decode(&v); e != nil {
				return nil, fmt.Errorf("logical JSON %s: %w", p, e)
			}
			if e := d.Decode(new(any)); e != io.EOF {
				return nil, fmt.Errorf("logical JSON %s requires exactly one value", p)
			}
			// Only converter provenance varies here. Audio bytes, source revision,
			// protocol, tables, channels and every playback parameter remain in the
			// complete fingerprint. The physical file hash was verified above.
			if o.out.Kind == "audio metadata" && m.PreparationVersion == 34 {
				doc, _ := v.(map[string]any)
				component, _ := doc["component"].(map[string]any)
				platform, _ := component["platform"].(string)
				digest, _ := component["helper_sha256"].(string)
				reference, ok := audioopl.ReferenceHelper(platform, digest)
				if !ok {
					return nil, fmt.Errorf("unrecognized prepared audio helper for %q", platform)
				}
				component["platform"], component["helper_sha256"] = "linux-amd64", reference
			}
			q, e := normalize(v, o.out.Kind)
			if e != nil {
				return nil, e
			}
			b, e = json.Marshal(q)
			if e != nil {
				return nil, e
			}
		}
		o.digest = Hash(b)
		o.size = len(b)
		o.done = true
		o.active = false
		return o, nil
	}
	normalize = func(v any, context string) (any, error) {
		switch q := v.(type) {
		case string:
			return q, nil
		case []any:
			out := make([]any, len(q))
			for i, x := range q {
				v, e := normalize(x, context)
				if e != nil {
					return nil, e
				}
				out[i] = v
			}
			return out, nil
		case map[string]any:
			out := make(map[string]any, len(q))
			for k, x := range q {
				v, e := normalize(x, context)
				if e != nil {
					return nil, e
				}
				out[k] = v
			}
			// Normalize only schema-owned reference fields. Text that happens to
			// spell a path is semantic text and must retain its exact bytes.
			fields := []string{}
			_, sourceKind := q["payload_sha256"]
			if m.PreparationVersion >= 33 && sourceKind && q["kind"] == "loose_file" && q["payload_sha256"] == Phil1BadAtlasSHA256 && q["sha256"] == Phil1BadAtlasSHA256 {
				if name, ok := q["path"].(string); ok && strings.EqualFold(path.Base(name), "PHIL1BMP.GS2") {
					out["payload_sha256"], out["sha256"] = Phil1CleanAtlasSHA256, Phil1CleanAtlasSHA256
				}
			}
			_, outputKind := q["size_bytes"]
			if sourceKind {
				fields = append(fields, "path", "container")
			}
			if outputKind {
				fields = append(fields, "path")
			}
			if _, ok := q["record_count"]; ok {
				fields = append(fields, "source_path")
			}
			if _, ok := q["detected_format"]; ok {
				fields = append(fields, "exported_relative_path")
			}
			if _, ok := q["diagnostic"]; ok {
				if _, yes := q["evidence"]; yes {
					fields = append(fields, "source")
				}
			}
			if context == "bitmap_font_metrics" {
				fields = append(fields, "atlas", "source")
			}
			if context == "office_lettering_atlas_metrics" {
				fields = append(fields, "png")
			}
			if _, ok := q["logical_name"]; ok {
				if _, yes := q["prepared_outputs"]; yes {
					fields = append(fields, "extracted_path")
				}
			}
			if context == "office_presentation_metadata" {
				if _, ok := q["index_sha256"]; ok {
					fields = append(fields, "png", "metrics")
				}
			}
			if context == "manifest" {
				if _, ok := q["generator"]; ok {
					fields = append(fields, "office_metadata", "menu_metadata", "startup_metadata", "mission_metadata", "editor_metadata", "replay_metadata", "debrief_metadata", "audio_metadata", "world_metadata", "cockpit_metadata")
				}
			}
			for _, field := range fields {
				if physical, ok := q[field].(string); ok {
					if logical, yes := paths[physical]; yes {
						out[field] = logical
					}
				}
			}
			if context == "world_metadata" {
				if _, isBundle := q["geometries"]; isBundle {
					if sources, ok := out["source_sha256"].(map[string]any); ok {
						if m.PreparationVersion >= 33 && sources["PHIL1BMP.GS2"] == Phil1BadAtlasSHA256 {
							sources["PHIL1BMP.GS2"] = Phil1CleanAtlasSHA256
						}
						for name := range sources {
							if nonSemanticOriginalWorkingFile(name) {
								delete(sources, name)
							}
						}
					}
				}
			}
			// An Output is a typed reference, not just a convenient filename. Its
			// original size/digest must agree before replacing location-dependent JSON
			// framing with the recursively canonical content identity.
			if p, ok := q["path"].(string); ok {
				if o := outputs[p]; o != nil {
					_, hasSize := q["size_bytes"]
					_, hasKind := q["kind"]
					_, hasHash := q["sha256"]
					if hasSize && hasKind && hasHash {
						if q["kind"] != o.out.Kind || q["sha256"] != o.out.SHA256 || fmt.Sprint(q["size_bytes"]) != fmt.Sprint(o.out.Size) {
							return nil, fmt.Errorf("inconsistent output reference %s", p)
						}
						done, e := finish(p)
						if e != nil {
							return nil, e
						}
						out["sha256"] = done.digest
						out["size_bytes"] = done.size
					}
				}
			}
			return out, nil
		default:
			return v, nil
		}
	}
	// Decode through JSON numbers to avoid floating-point normalization of tables.
	raw, e := json.Marshal(m)
	if e != nil {
		return nil, e
	}
	var v map[string]any
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if e = d.Decode(&v); e != nil {
		return nil, e
	}
	delete(v, "input_directory")
	// Summary counts describe extraction diagnostics, never runtime mechanics;
	// mutable original films/saves must not become simulation dependencies.
	delete(v, "summary")
	if resources, ok := v["resources"].([]any); ok {
		kept := resources[:0]
		for _, value := range resources {
			entry, ok := value.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("invalid resource shape")
			}
			source, ok := entry["source"].(map[string]any)
			if !ok {
				return nil, fmt.Errorf("invalid source shape")
			}
			kind, _ := source["kind"].(string)
			p, _ := source["path"].(string)
			if kind == "loose_file" && nonSemanticOriginalWorkingFile(p) {
				continue
			}
			kept = append(kept, value)
		}
		v["resources"] = kept
	}

	// Verify every retained output even if a mutable original working file is excluded below.
	for p := range outputs {
		if _, err := finish(p); err != nil {
			return nil, err
		}
	}
	if reader.canonical != nil {
		for p, o := range outputs {
			reader.canonical[p] = leafProof{o.digest, o.size}
		}
	}
	canonical, e := normalize(v, "manifest")
	if e != nil {
		return nil, e
	}
	// Inventory ordering is source traversal, not resource semantics. Normalize
	// only its two top-level collections; ordered runtime tables remain ordered.
	top := canonical.(map[string]any)
	for _, key := range []string{"containers", "resources"} {
		if rows, ok := top[key].([]any); ok {
			sort.Slice(rows, func(i, j int) bool {
				a, _ := json.Marshal(rows[i])
				b, _ := json.Marshal(rows[j])
				return bytes.Compare(a, b) < 0
			})
			top[key] = rows
		}
	}
	version := LogicalResourceProfileVersion
	if decodedMedia {
		version = decodedResourceProfileVersion
	}
	data, e := json.Marshal(struct {
		Version  string
		Manifest any
	}{version, canonical})
	if e != nil {
		return nil, e
	}
	return data, nil
}
