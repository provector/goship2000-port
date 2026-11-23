package assets

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"gunship2000-assets/game/configuration"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const (
	maxSourceBytes = 64 << 20
	maxTotalBytes  = 512 << 20
	maxPixels      = 1 << 20
	maxRecords     = 16384
)

type Entry struct {
	Index           int    `json:"index"`
	DirectoryOffset int    `json:"directory_offset"`
	RawHex          string `json:"directory_raw_hex"`
	NameHex         string `json:"raw_name_hex"`
	NameReadable    string `json:"name_readable"`
	OpaqueHex       string `json:"opaque_raw_hex"`
	OpaqueLE        uint32 `json:"opaque_le_candidate"`
	Offset          uint64 `json:"payload_offset"`
	Length          uint64 `json:"payload_length"`
	End             uint64 `json:"payload_end_exclusive"`
	Name            []byte `json:"-"`
}

type SourceLocation struct {
	Path          string `json:"path"`
	SHA256        string `json:"sha256"`
	Kind          string `json:"kind"`
	Container     string `json:"container,omitempty"`
	Record        *Entry `json:"directory_record,omitempty"`
	Offset        uint64 `json:"payload_offset"`
	Length        uint64 `json:"payload_length"`
	PayloadSHA256 string `json:"payload_sha256"`
}

type Output struct {
	Path   string `json:"path"`
	Kind   string `json:"kind"`
	Size   int    `json:"size_bytes"`
	SHA256 string `json:"sha256"`
	// A shared index atlas can have different palette contexts per export.
	Palette *PaletteAssociation `json:"palette,omitempty"`
}

type PaletteAssociation struct {
	Source     string `json:"source,omitempty"`
	SHA256     string `json:"source_payload_sha256,omitempty"`
	Evidence   string `json:"evidence"`
	Confidence string `json:"confidence"`
	Diagnostic bool   `json:"diagnostic"`
}

type ConversionResult struct {
	Status            string             `json:"status"`
	Errors            []string           `json:"errors,omitempty"`
	Width             int                `json:"width,omitempty"`
	Height            int                `json:"height,omitempty"`
	PixelSHA256       string             `json:"pixel_sha256,omitempty"`
	Palette           PaletteAssociation `json:"palette"`
	RuntimeValidation string             `json:"runtime_validation"`
	Stats             *DecodeStats       `json:"decoder_statistics,omitempty"`
	ImageOffset       int                `json:"image_header_relative_offset,omitempty"`
	ImageEnd          int                `json:"image_end_relative_exclusive,omitempty"`
	RemainingBytes    int                `json:"bytes_after_image_block,omitempty"`
}

type Resource struct {
	Source     SourceLocation   `json:"source"`
	ExportPath string           `json:"exported_relative_path,omitempty"`
	Format     string           `json:"detected_format"`
	Basis      string           `json:"detection_basis"`
	Extraction string           `json:"extraction_status"`
	Conversion ConversionResult `json:"conversion"`
	Outputs    []Output         `json:"outputs,omitempty"`
	backing    []byte
	Payload    []byte `json:"-"`
	name       string
}

type Container struct {
	Path             string `json:"source_path"`
	SHA256           string `json:"sha256"`
	Layout           string `json:"layout"`
	BaselineIdentity bool   `json:"matches_known_source_identity"`
	Count            int    `json:"record_count"`
	DirectoryEnd     int    `json:"directory_end_exclusive"`
}

type Counts struct {
	Containers         int            `json:"containers"`
	LooseFiles         int            `json:"loose_files_inventoried"`
	LooseCopied        int            `json:"loose_files_copied_and_verified"`
	Executables        int            `json:"executables_inventory_only"`
	Entries            int            `json:"archive_entries_extracted_and_verified"`
	Pictures           int            `json:"pictures_converted"`
	ColourPictures     int            `json:"supported_colour_pictures"`
	DiagnosticPictures int            `json:"diagnostic_grayscale_pictures"`
	Palettes           int            `json:"palettes_exported"`
	Unsupported        int            `json:"unsupported_resources"`
	ConversionFailures int            `json:"resource_conversion_failures"`
	FontSelectors      int            `json:"font_selectors_exported"`
	StartupAnimations  int            `json:"startup_animations_exported"`
	StartupFrames      int            `json:"startup_frames_exported"`
	AudioFMTracks      int            `json:"audio_fm_tracks"`
	AudioDigitalTracks int            `json:"audio_digital_tracks"`
	UnsupportedFormats map[string]int `json:"unsupported_formats"`
}

type Manifest struct {
	Generator                    string                        `json:"generator,omitempty"`
	PreparationVersion           int                           `json:"preparation_version,omitempty"`
	SchemaVersion                int                           `json:"schema_version"`
	InputDirectory               string                        `json:"input_directory"`
	Result                       string                        `json:"result"`
	Policy                       []string                      `json:"policy"`
	Containers                   []Container                   `json:"containers"`
	Resources                    []*Resource                   `json:"resources"`
	Summary                      Counts                        `json:"summary"`
	FatalError                   string                        `json:"fatal_error,omitempty"`
	OfficeMetadata               string                        `json:"office_metadata,omitempty"`
	MenuMetadata                 string                        `json:"menu_metadata,omitempty"`
	StartupMetadata              string                        `json:"startup_metadata,omitempty"`
	MissionMetadata              string                        `json:"mission_metadata,omitempty"`
	EditorMetadata               string                        `json:"editor_metadata,omitempty"`
	ReplayMetadata               string                        `json:"replay_metadata,omitempty"`
	DebriefMetadata              string                        `json:"debrief_metadata,omitempty"`
	AudioMetadata                string                        `json:"audio_metadata,omitempty"`
	WorldMetadata                string                        `json:"world_metadata,omitempty"`
	CockpitMetadata              string                        `json:"cockpit_metadata,omitempty"`
	VersionProfile               string                        `json:"recognized_office_profile,omitempty"`
	UnsupportedVersionOperations []UnsupportedVersionOperation `json:"unsupported_version_operations,omitempty"`
	attemptedPixels              int64
}

func Hash(data []byte) string { value := sha256.Sum256(data); return hex.EncodeToString(value[:]) }

func le16(b []byte) int { return int(binary.LittleEndian.Uint16(b)) }

// Bounds and resource limits are inspector safeguards, not original DOS behavior.
func parseDirectory(data []byte) ([]Entry, error) {
	if len(data) < 2 {
		return nil, errors.New("truncated LE16 directory count")
	}
	n := le16(data)
	end := 2 + 24*n
	if n > maxRecords {
		return nil, fmt.Errorf("record count %d exceeds defensive limit %d", n, maxRecords)
	}
	if end > len(data) {
		return nil, fmt.Errorf("truncated directory: need %d bytes, have %d", end, len(data))
	}
	entries := make([]Entry, 0, n)
	for i := 0; i < n; i++ {
		pos := 2 + 24*i
		raw := data[pos : pos+24]
		name := bytes.SplitN(raw[:12], []byte{0}, 2)[0]
		offset := uint64(binary.LittleEndian.Uint32(raw[20:]))
		length := uint64(binary.LittleEndian.Uint32(raw[16:]))
		last := offset + length
		if last > uint64(len(data)) || offset > uint64(len(data)) {
			return nil, fmt.Errorf("record %d range [%d,%d) outside %d bytes", i, offset, last, len(data))
		}
		if length > 0 && offset < uint64(end) {
			return nil, fmt.Errorf("record %d intrudes into directory ending %d", i, end)
		}
		entries = append(entries, Entry{i, pos, hex.EncodeToString(raw), hex.EncodeToString(raw[:12]), readable(name), hex.EncodeToString(raw[12:16]), binary.LittleEndian.Uint32(raw[12:16]), offset, length, last, bytes.Clone(name)})
	}
	// Shared or overlapping in-bounds slices remain extractable; no silent clipping.
	return entries, nil
}

func readable(raw []byte) string {
	var out strings.Builder
	for _, b := range raw {
		if b >= 32 && b <= 126 && b != '\\' {
			out.WriteByte(b)
		} else {
			fmt.Fprintf(&out, "\\x%02x", b)
		}
	}
	return out.String()
}

// %HH escapes original bytes. Literal percent and tilde are always escaped;
// generated ~entry directories cannot coincide with an original name.
func escapedPath(raw []byte) string {
	// Split as bytes: names are opaque, not assumed UTF-8.
	segments := bytes.Split(bytes.ReplaceAll(raw, []byte{'\\'}, []byte{'/'}), []byte{'/'})
	result := make([]string, 0, len(segments))
	for _, component := range segments {
		var s strings.Builder
		for i, b := range component {
			safe := b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z' || b >= '0' && b <= '9' || b == '_' || b == '-' || b == '.'
			if b == '.' && (i == len(component)-1 || bytes.Equal(component, []byte(".")) || bytes.Equal(component, []byte(".."))) {
				safe = false
			}
			if safe {
				s.WriteByte(b)
			} else {
				fmt.Fprintf(&s, "%%%02X", b)
			}
		}
		name := s.String()
		if name == "" {
			name = "%00"
		}
		stem := strings.ToUpper(strings.SplitN(name, ".", 2)[0])
		if stem == "CON" || stem == "PRN" || stem == "AUX" || stem == "NUL" || len(stem) == 4 && (strings.HasPrefix(stem, "COM") || strings.HasPrefix(stem, "LPT")) && stem[3] >= '1' && stem[3] <= '9' {
			name = "%00" + name
		}
		result = append(result, name)
	}
	return strings.Join(result, "/")
}

type pathAllocator struct {
	used map[string]bool
	next int
}

func (a *pathAllocator) reserve(desired string) string {
	if a.used == nil {
		a.used = make(map[string]bool)
	}
	conflicts := func(p string) bool {
		p = strings.ToLower(p)
		for old := range a.used {
			if p == old || strings.HasPrefix(p, old+"/") || strings.HasPrefix(old, p+"/") {
				return true
			}
		}
		return false
	}
	answer := desired
	for conflicts(answer) {
		a.next++
		answer = fmt.Sprintf("~entry-%06d/%s", a.next, desired)
	}
	a.used[strings.ToLower(answer)] = true
	return answer
}

func Within(child, parent string) bool         { return configuration.Within(child, parent) }
func ResolvedNewPath(p string) (string, error) { return configuration.ResolvedNewPath(p) }

func ValidatePaths(input, output string) (string, string, error) {
	in, err := filepath.Abs(input)
	if err != nil {
		return "", "", err
	}
	in, err = filepath.EvalSymlinks(in)
	if err != nil {
		return "", "", err
	}
	info, err := os.Stat(in)
	if err != nil {
		return "", "", err
	}
	if !info.IsDir() {
		return "", "", errors.New("input must be a directory")
	}
	out, err := ResolvedNewPath(output)
	if err != nil {
		return "", "", err
	}
	if Within(out, in) || Within(in, out) {
		return "", "", errors.New("input and output directories must not overlap")
	}
	if _, err := os.Lstat(out); !os.IsNotExist(err) {
		return "", "", errors.New("output must not exist; no overwrite option is provided")
	}
	// Locate this project's protected trees from input ancestry or current directory.
	cwd, _ := os.Getwd()
	for _, start := range []string{in, cwd, out} {
		for p := start; ; p = filepath.Dir(p) {
			if isSourceTree(p) && Within(out, filepath.Join(p, "test-data")) {
				return "", "", fmt.Errorf("output is inside protected test-data tree")
			}
			if _, err := os.Stat(filepath.Join(p, "evidence", "step01", "baseline-manifest.json")); err == nil {
				for _, name := range []string{"original", "fixtures", "evidence"} {
					if Within(out, filepath.Join(p, name)) {
						return "", "", fmt.Errorf("output is inside protected %s tree", name)
					}
				}
			}
			if filepath.Dir(p) == p {
				break
			}
		}
	}
	return in, out, nil
}

func WriteNew(root, relative string, data []byte) (Output, error) {
	if path.IsAbs(relative) || path.Clean(relative) != relative || strings.Contains(relative, "\\") || relative == ".." || strings.HasPrefix(relative, "../") {
		return Output{}, errors.New("unsafe generated path")
	}
	target := filepath.Join(root, filepath.FromSlash(relative))
	if !Within(target, root) {
		return Output{}, errors.New("output escapes root")
	}
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return Output{}, err
	}
	f, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return Output{}, err
	}
	_, writeErr := f.Write(data)
	closeErr := f.Close()
	if writeErr != nil {
		return Output{}, writeErr
	}
	if closeErr != nil {
		return Output{}, closeErr
	}
	actual, err := os.ReadFile(target)
	if err != nil {
		return Output{}, err
	}
	if !bytes.Equal(actual, data) {
		return Output{}, errors.New("output byte verification failed")
	}
	return Output{Path: relative, Size: len(data), SHA256: Hash(data)}, nil
}

func JsonBytes(value any) ([]byte, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	return append(data, '\n'), err
}

type OutputWriter struct {
	Root    string `json:"-"`
	written int64
	files   int
}

func (w *OutputWriter) Write(relative string, data []byte) (Output, error) {
	if int64(len(data)) > maxTotalBytes-w.written || w.files >= 65536 {
		return Output{}, errors.New("aggregate output exceeds512MiB or65536-file defensive limit")
	}
	item, err := WriteNew(w.Root, relative, data)
	if err == nil {
		w.written += int64(len(data))
		w.files++
	}
	return item, err
}

func (m *Manifest) decode(raw []byte, width, height int) ([]byte, DecodeStats, error) {
	pixels := int64(width) * int64(height)
	if pixels <= 0 || pixels > maxPixels {
		return nil, DecodeStats{}, errors.New("dimensions exceed supported output bounds")
	}
	if pixels > maxTotalBytes-m.attemptedPixels {
		return nil, DecodeStats{}, errors.New("aggregate attempted decode exceeds512Mi pixels")
	}
	m.attemptedPixels += pixels
	return decodePicture(raw)
}

func identify(raw []byte, name string) (string, string, bool) {
	if bytes.HasPrefix(raw, []byte("MZ")) {
		return "DOS MZ executable", "MZ signature; no execution or unpacking", true
	}
	if headerlessExecutables[Hash(raw)] {
		return "DOS COM launcher candidate", "exact Step1 headerless executable SHA256", true
	}
	if bytes.HasPrefix(raw, []byte("FORM")) {
		kind := "short"
		if len(raw) >= 12 {
			kind = readable(raw[8:12])
		}
		return "FORM/" + kind, "FORM tag and raw outer type only; no recursive decoding", false
	}
	if bytes.HasPrefix(raw, []byte("M0")) || bytes.HasPrefix(raw, []byte("X0")) {
		return "M0/X0 candidate", "leading block tag; conversion performs full bounds checks", false
	}
	if len(raw) == 768 {
		valid := true
		for _, b := range raw {
			if b > 63 {
				valid = false
				break
			}
		}
		if valid {
			return "raw RGB6 palette candidate", "768 bytes, all0..63; structural candidate, no installed association", false
		}
	}
	if len(raw) > 0 && raw[0] == 0xf5 {
		return "F5 picture candidate", "F5 prefix; legacy mode not implemented", false
	}
	if bytes.HasPrefix(raw, []byte("MADSS1")) {
		return "MADSS1 startup", "MADSS1 signature; conversion support is source-profile and variant specific", false
	}
	if bytes.HasPrefix(raw, []byte{255, 255, 0x5a, 0x14}) {
		return "PAN variant candidate", "ffff5a14 prefix; conversion support is source-profile and variant specific", false
	}
	if name == "FONTS.GS2" && IslandIce469085.SourceMatches(roleBitmapFonts, Hash(raw)) {
		return "FONTS.GS2", "supported source-profile identity; six source-backed bitmap selectors", false
	}
	return "opaque resource", "no supported content signature; no guessed container boundaries", false
}

func PrepareAssetsWithAudio(input, output string, prepareAudio func(string, *Manifest) error) (*Manifest, error) {
	return PrepareAssetsWithProgress(input, output, prepareAudio, nil)
}
func PrepareAssetsWithProgress(input, output string, prepareAudio func(string, *Manifest) error, progress func(PreparationProgress)) (*Manifest, error) {
	return prepareSelected(input, output, prepareAudio, progress, preparationFiles(), preparationMembers)
}
func prepareSelected(input, output string, prepareAudio func(string, *Manifest) error, progress func(PreparationProgress), filesRequired map[string]bool, membersRequired map[string]string) (*Manifest, error) {
	emit := func(stage string, n, total int) {
		if progress != nil {
			progress(PreparationProgress{Stage: stage, Completed: n, Total: total})
		}
	}
	emit("Inventory original installation", 0, 0)

	in, out, err := ValidatePaths(input, output)
	if err != nil {
		return nil, err
	}
	m := &Manifest{SchemaVersion: PreparedManifestSchema, Generator: PreparationGenerator, PreparationVersion: PreparationVersion, InputDirectory: in, Result: "preflight", Policy: []string{
		"Known DAT/CAT directories: LE16 count,24-byte records; raw payloads unchanged. Unknown containers remain opaque.",
		"Source hashes select evidence-backed interpretations, not generic parsing. Unknown identities may extract/decode structurally; unsupported office/font profile operations are reported separately.",
		"%HH escapes name bytes; separators retain relative directories; unsafe/empty/device components escaped. Raw12 bytes authoritative. Case/prefix collisions use generated ~entry-NNNNNN namespaces.",
		"All raw paths reserved before conversion paths; new output tree only. No files overwritten.",
		"X0 decode uses original LE16 refill, dictionary/RLE state; unknown inherited entry256 is a failure. Real adjacent byte may be prefetched, never consumed outside X0.",
		"Opaque indexed PNG; index0 retained. RGB6 expansion (v<<2)|(v>>4) is an export convention, not measured colourimetry.",
		"Exact GS2000.CAT BASE/BASE_AN retain prior full-bank validation. BASE_AN1/BASE_AN2/POINTER use the source-backed office-only BASE.PAL association with regional/constructor witnesses, not full-bank proof. Other pictures use diagnostic grayscale.",
		"Exact FONTS.GS2 exports source-backed bitmap masks/metrics; BASE.SCN office metadata is bounded and source-regenerated. No evidence-directory or Python runtime dependency.",
		"Preparation7 adds source-profiled world/GLB/query/cockpit data and retains GS2 renderer raw carrier; source indices and pre-existing picture/audio outputs remain unchanged.",
		"A25e96 index remap is not baked in: pre-tent interstitial scene5 bank1; SHIP_%02d.SHP loadout-sheet loader scenes4/5 bank3 (not commander). Ranges45h..4fh andc0h..dfh. Step D uses raw commander indices.",
		"Limits:64MiB/source,512MiB total input/raw extraction/output,65536 output files,16384 records/container,256 palette prefixes,1048576 pixels/image,512Mi attempted pixels. Defensive checks, not original compatibility behavior.",
	}}
	m.Summary.UnsupportedFormats = make(map[string]int)
	alloc := &pathAllocator{}
	var total int64
	var planned uint64
	in, err = OriginalDirectory(in)
	if err != nil {
		return nil, err
	}
	files, err := selectedFiles(in, filesRequired)
	if err != nil {
		return nil, err
	}
	m.InputDirectory = in
	process := func(d fs.DirEntry) error {
		p := filepath.Join(in, d.Name())
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing input symlink: %s", p)
		}
		info, e := d.Info()
		if e != nil {
			return e
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("nonregular input: %s", p)
		}
		if info.Size() > maxSourceBytes {
			return fmt.Errorf("source exceeds64MiB: %s", p)
		}
		total += info.Size()
		if total > maxTotalBytes {
			return errors.New("total input exceeds512MiB")
		}
		raw, e := ReadLimitedFile(p, maxSourceBytes)
		if e != nil {
			return e
		}
		if len(raw) > maxSourceBytes || int64(len(raw)) != info.Size() {
			return fmt.Errorf("source changed while reading: %s", p)
		}
		rel, _ := filepath.Rel(in, p)
		rel = filepath.ToSlash(rel)
		name := strings.ToUpper(filepath.Base(p))
		sourceHash := Hash(raw)
		// Executable table offsets and audio command addresses are edition-specific.
		// Reject an unknown binary before a converter uses its fixed layout.
		for _, spec := range IslandIce469085.Sources {
			switch spec.Role {
			case RoleMainExecutable, RoleSimulationExecutable, RoleLogoPlayer, RoleIntroPlayer, roleAudioLOG, roleAudioGS3, RoleAudioGS4, RoleAudioADigital, RoleAudioDDigital:
				if name == spec.Name && sourceHash != spec.SHA256 {
					return fmt.Errorf("%s: unsupported executable/table layout for %s", rel, IslandIce469085.Name)
				}
			}
		}
		if knownHash, ok := catalogHashes[name]; ok {
			entries, e := parseDirectory(raw)
			if e != nil {
				return fmt.Errorf("%s: %w", rel, e)
			}
			members := map[string]bool{}
			for _, member := range strings.Fields(membersRequired[name]) {
				members[member] = false
			}
			for _, record := range entries {
				key := strings.ToUpper(string(record.Name))
				seen, want := members[key]
				if membersRequired != nil && !want {
					continue
				}
				if membersRequired != nil && seen {
					return fmt.Errorf("%s: conflicting member %s", rel, key)
				}
				members[key] = true
				planned += record.Length
			}
			for _, member := range strings.Fields(membersRequired[name]) {
				if !members[member] {
					return fmt.Errorf("%s: missing member %s", rel, member)
				}
			}
			if planned > maxTotalBytes {
				return errors.New("planned raw extraction exceeds512MiB (including shared ranges)")
			}
			m.Containers = append(m.Containers, Container{rel, sourceHash, "LE16 count;24-byte records: name12,opaque4,lengthLE32,absoluteOffsetLE32; Steps2/8/20", sourceHash == knownHash, len(entries), 2 + 24*len(entries)})
			for i := range entries {
				record := entries[i]
				member := strings.ToUpper(string(record.Name))
				if !members[member] {
					continue
				}
				payload := raw[int(record.Offset):int(record.End)]
				format, basis, _ := identify(payload, string(record.Name))
				r := &Resource{Source: SourceLocation{rel, sourceHash, "archive_entry", rel, &record, record.Offset, record.Length, Hash(payload)}, Format: format, Basis: basis, Extraction: "pending", backing: raw, Payload: payload, name: member}
				r.ExportPath = alloc.reserve(name + "/" + escapedPath([]byte(member)))
				m.Resources = append(m.Resources, r)
			}
		} else {
			format, basis, executable := identify(raw, name)
			m.Summary.LooseFiles++
			r := &Resource{Source: SourceLocation{Path: rel, SHA256: sourceHash, Kind: "loose_file", Length: uint64(len(raw)), PayloadSHA256: sourceHash}, Format: format, Basis: basis, backing: raw, Payload: raw, name: name, Extraction: "pending"}
			if executable && !audioCarrierName(name) {
				r.Extraction = "inventoried_no_copy"
				r.Conversion.Status = "not_applicable_executable"
				m.Summary.Executables++
			} else {
				planned += uint64(len(raw))
				if planned > maxTotalBytes {
					return errors.New("planned raw extraction exceeds512MiB")
				}
				r.ExportPath = alloc.reserve("loose/" + escapedPath([]byte(name)))
			}
			m.Resources = append(m.Resources, r)
		}
		return nil
	}
	for _, d := range files {
		if err := process(d); err != nil {
			return nil, fmt.Errorf("%s: %w", d.Name(), err)
		}
	}
	m.Summary.Containers = len(m.Containers)
	if err = os.MkdirAll(filepath.Dir(out), 0755); err != nil {
		return nil, err
	}
	if err = os.Mkdir(out, 0755); err != nil {
		return nil, err
	}
	writer := &OutputWriter{Root: out}
	fail := func(e error) (*Manifest, error) {
		m.Result = "fatal_extraction_failure"
		m.FatalError = e.Error()
		data, _ := JsonBytes(m)
		_, _ = WriteNew(out, "manifest.json", data)
		return m, e
	}
	if membersRequired != nil {
		if _, e := ResolveSourceInstallation(m); e != nil {
			return fail(fmt.Errorf("supported source installation: %w", e))
		}
	}
	for index, r := range m.Resources {
		emit("Extract source resources", index, len(m.Resources))
		if r.Extraction == "inventoried_no_copy" {
			continue
		}
		item, e := writer.Write(r.ExportPath, r.Payload)
		if e != nil {
			r.Extraction = "failed"
			return fail(e)
		}
		item.Kind = "original_payload"
		r.Outputs = append(r.Outputs, item)
		r.Extraction = "extracted_byte_verified"
		if r.Source.Kind == "archive_entry" {
			m.Summary.Entries++
		} else {
			m.Summary.LooseCopied++
		}
	}
	emit("Extract source resources", len(m.Resources), len(m.Resources))
	var officePalette *Palette
	var officeSource string
	for _, r := range m.Resources {
		if r.fromCatalog("GS2000.CAT") && r.name == "BASE.PAL" {
			palettes, _, _, e := pictureHeader(r.Payload)
			if e == nil && len(palettes) == 1 && palettes[0].First == 0 && palettes[0].Last == 255 && Hash(r.Payload) == officePaletteHash {
				officePalette = &palettes[0]
				officeSource = r.ExportPath
			}
		}
	}
	for index, r := range m.Resources {
		emit("Decode pictures and palettes", index, len(m.Resources))
		if r.Extraction == "inventoried_no_copy" {
			continue
		}
		if e := convertResource(writer, r, alloc, m, officePalette, officeSource); e != nil {
			return fail(e)
		}
	}
	emit("Decode pictures and palettes", len(m.Resources), len(m.Resources))
	emit("Office, fonts and menus", 0, 1)
	if e := exportOfficeMetadata(writer, alloc, m); e != nil {
		return fail(e)
	}
	if e := exportMenuMetadata(writer, alloc, m); e != nil {
		return fail(e)
	}
	emit("Startup animation", 0, 1)
	if e := exportStartupMetadata(writer, alloc, m); e != nil {
		return fail(e)
	}
	emit("Mission and model tables", 0, 1)
	if e := exportMissionMetadata(writer, alloc, m); e != nil {
		return fail(e)
	}
	emit("Debrief scenes", 0, 1)
	if e := exportDebriefMetadata(writer, alloc, m); e != nil {
		return fail(e)
	}
	emit("World geometry and maps", 0, 1)
	if e := exportWorldAssetsProgress(writer, alloc, m, func(n, total int) { emit("World geometry and maps", n, total) }, func(n, total int) { emit("Encode world geometry", n, total) }); e != nil {
		return fail(e)
	}
	emit("Replay metadata", 0, 1)
	if e := exportReplayMetadata(writer, alloc, m); e != nil {
		return fail(e)
	}
	emit("Editor metadata", 0, 1)
	if e := exportEditorMetadata(writer, alloc, m); e != nil {
		return fail(e)
	}
	emit("Sound conversion", 0, 1)
	if prepareAudio != nil {
		if e := prepareAudio(out, m); e != nil {
			return fail(e)
		}
	}
	emit("Sound conversion", 1, 1)
	emit("Write manifest", 0, 1)
	m.Result = "complete_success"
	if m.Summary.Unsupported > 0 || m.Summary.ConversionFailures > 0 {
		m.Result = "partial_conversion"
	}
	data, e := JsonBytes(m)
	if e != nil {
		return fail(e)
	}
	if _, e = writer.Write("manifest.json", data); e != nil {
		return nil, e
	}
	return m, nil
}

type Palette struct {
	First  int    `json:"first_index"`
	Last   int    `json:"last_index"`
	Offset int    `json:"component_relative_offset"`
	End    int    `json:"component_end_exclusive"`
	RGB6   []byte `json:"-"`
}

func pictureHeader(raw []byte) ([]Palette, int, int, error) {
	pos := 0
	var palettes []Palette
	for pos+2 <= len(raw) && string(raw[pos:pos+2]) == "M0" {
		if len(palettes) >= 256 {
			return nil, 0, 0, errors.New("more than256 palette prefixes")
		}
		if pos+6 > len(raw) {
			return nil, 0, 0, errors.New("truncated M0 header")
		}
		end := pos + 4 + le16(raw[pos+2:])
		if end < pos+6 || end > len(raw) {
			return nil, 0, 0, errors.New("M0 extent outside payload")
		}
		first, last := int(raw[pos+4]), int(raw[pos+5])
		components := raw[pos+6 : end]
		if last < first || len(components) != 3*(last-first+1) {
			return nil, 0, 0, errors.New("M0 range/component length mismatch")
		}
		for _, v := range components {
			if v > 63 {
				return nil, 0, 0, errors.New("M0 component exceeds RGB6 precision")
			}
		}
		palettes = append(palettes, Palette{first, last, pos + 6, end, components})
		pos = end
	}
	if pos+8 > len(raw) || string(raw[pos:pos+2]) != "X0" {
		return palettes, pos, 0, errors.New("unsupported or truncated image block after palette prefix")
	}
	end := pos + 4 + le16(raw[pos+2:])
	if end < pos+8 || end > len(raw) {
		return palettes, pos, 0, errors.New("X0 extent outside payload")
	}
	return palettes, pos, end, nil
}

func paletteColors(p *Palette) color.Palette {
	answer := make(color.Palette, 256)
	for i := range answer {
		answer[i] = color.RGBA{uint8(i), uint8(i), uint8(i), 255}
	}
	if p != nil {
		for i := p.First; i <= p.Last; i++ {
			j := 3 * (i - p.First)
			v := p.RGB6[j : j+3]
			answer[i] = color.RGBA{v[0]<<2 | v[0]>>4, v[1]<<2 | v[1]>>4, v[2]<<2 | v[2]>>4, 255}
		}
	}
	return answer
}

func PngBytes(img image.Image) ([]byte, error) {
	var b bytes.Buffer
	err := png.Encode(&b, img)
	return b.Bytes(), err
}

func addGenerated(out *OutputWriter, r *Resource, a *pathAllocator, suffix, kind string, data []byte) error {
	item, err := out.Write(a.reserve(r.ExportPath+suffix), data)
	if err != nil {
		return err
	}
	item.Kind = kind
	r.Outputs = append(r.Outputs, item)
	return nil
}

func addPalettedGenerated(out *OutputWriter, r *Resource, a *pathAllocator, suffix, kind string, data []byte, palette PaletteAssociation) error {
	if err := addGenerated(out, r, a, suffix, kind, data); err != nil {
		return err
	}
	// Keep a copy: later screen preparation must not recolour an older output's
	// provenance when it adds another PNG using the same immutable indices.
	r.Outputs[len(r.Outputs)-1].Palette = &palette
	return nil
}

func exportPalette(out *OutputWriter, r *Resource, a *pathAllocator, p Palette, suffix string) error {
	type entry struct {
		Index int     `json:"index"`
		RGB6  [3]byte `json:"rgb6"`
		RGB8  [3]byte `json:"rgb8"`
	}
	var rows []entry
	for i := p.First; i <= p.Last; i++ {
		j := 3 * (i - p.First)
		v := p.RGB6[j : j+3]
		rows = append(rows, entry{i, [3]byte{v[0], v[1], v[2]}, [3]byte{v[0]<<2 | v[0]>>4, v[1]<<2 | v[1]>>4, v[2]<<2 | v[2]>>4}})
	}
	data, err := JsonBytes(struct {
		Range       Palette `json:"source_range"`
		RGB6Hex     string  `json:"rgb6_hex"`
		Entries     []entry `json:"entries"`
		Rule        string  `json:"conversion"`
		Unspecified string  `json:"unspecified_entries"`
		Association string  `json:"display_association"`
	}{p, hex.EncodeToString(p.RGB6), rows, "RGB8=(v<<2)|(v>>4), RGB order, opaque export convention", "Indices outside first..last are unspecified, not black; crossed in swatch", "Source data only; no new runtime palette association"})
	if err != nil {
		return err
	}
	if err = addGenerated(out, r, a, suffix+".json", "palette_json", data); err != nil {
		return err
	}
	if err = addGenerated(out, r, a, suffix+".rgb6", "exact_rgb6_components", p.RGB6); err != nil {
		return err
	}
	swatch := image.NewRGBA(image.Rect(0, 0, 512, 344))
	draw.Draw(swatch, swatch.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	drawLabel(swatch, 4, 5, fmt.Sprintf("RGB6 %02X-%02X X=UNSPECIFIED", p.First, p.Last), color.Black)
	colors := paletteColors(&p)
	for i := 0; i < 256; i++ {
		x, y := (i%16)*32, 24+(i/16)*20
		fill := color.Color(color.RGBA{210, 210, 210, 255})
		specified := i >= p.First && i <= p.Last
		if specified {
			fill = colors[i]
		}
		draw.Draw(swatch, image.Rect(x, y, x+32, y+20), image.NewUniform(fill), image.Point{}, draw.Src)
		if !specified {
			for n := 0; n < 20; n++ {
				swatch.Set(x+n, y+n, color.Black)
				swatch.Set(x+31-n, y+n, color.Black)
			}
		}
		// White label backing makes actual black distinct from unspecified crossing.
		draw.Draw(swatch, image.Rect(x, y, x+11, y+8), image.NewUniform(color.White), image.Point{}, draw.Src)
		drawLabel(swatch, x, y, fmt.Sprintf("%02X", i), color.Black)
	}
	data, err = PngBytes(swatch)
	if err != nil {
		return err
	}
	return addGenerated(out, r, a, suffix+".png", "labelled_palette_swatch", data)
}

// Tiny export-label alphabet only; not a substitute for an original game font.
func drawLabel(dst *image.RGBA, x, y int, text string, c color.Color) {
	glyphs := map[rune]string{'0': "111101101101111", '1': "010110010010111", '2': "111001111100111", '3': "111001111001111", '4': "101101111001001", '5': "111100111001111", '6': "111100111101111", '7': "111001010010010", '8': "111101111101111", '9': "111101111001111", 'A': "010101111101101", 'B': "110101110101110", 'C': "111100100100111", 'D': "110101101101110", 'E': "111100110100111", 'F': "111100110100100", 'G': "111100101101111", 'I': "111010010010111", 'L': "100100100100111", 'N': "101111111111101", 'P': "110101110100100", 'R': "110101110101101", 'S': "111100111001111", 'U': "101101101101111", 'X': "101101010101101", '-': "000000111000000", '=': "000111000111000"}
	for _, letter := range text {
		g := glyphs[letter]
		for i, b := range g {
			if b == '1' {
				dst.Set(x+i%3, y+i/3, c)
			}
		}
		x += 5
	}
}

func convertResource(out *OutputWriter, r *Resource, a *pathAllocator, m *Manifest, office *Palette, officeSource string) error {
	c := &r.Conversion
	c.Status = "unsupported"
	c.RuntimeValidation = "none; offline conversion does not establish runtime use"
	c.Palette = PaletteAssociation{Evidence: "no established association", Confidence: "unresolved", Diagnostic: true}
	if ok, err := exportOfficeFonts(out, r, a); ok {
		if err != nil {
			return err
		}
		c.Status = "bitmap_fonts_exported"
		c.Palette = PaletteAssociation{Evidence: "binary foreground masks; caller palette index recolours set bits", Confidence: "source_layout_and_prior_font5_witness", Diagnostic: false}
		c.RuntimeValidation = "prior complete font source load matched original; font5 office glyph rendering verified, other selectors structural"
		m.Summary.FontSelectors += le16(r.Payload)
		return nil
	}
	if r.name == "FONTS.GS2" {
		c.Errors = append(c.Errors, "bitmap font layout export unsupported: source identity is not recognized by a supported profile")
		m.UnsupportedVersionOperations = append(m.UnsupportedVersionOperations, UnsupportedVersionOperation{"bitmap font layout export", IslandIce469085.Name, c.Errors[len(c.Errors)-1]})
	}
	if r.fromCatalog("GS2000.CAT") && r.name == "BASE.PAL" && office != nil {
		c.Palette = PaletteAssociation{officeSource, officePaletteHash, "Step10 all256 source/installed RGB6 entries at completed office", "prior_runtime_verified", false}
		c.RuntimeValidation = "prior all256 installed RGB6 entries matched this source; Step10"
	}
	var palettes []Palette
	pos, end := 0, 0
	var decodeErr error
	if r.Format == "raw RGB6 palette candidate" {
		palettes = []Palette{{0, 255, 0, 768, r.Payload}}
	} else if bytes.HasPrefix(r.Payload, []byte("X0")) || bytes.HasPrefix(r.Payload, []byte("M0")) {
		palettes, pos, end, decodeErr = pictureHeader(r.Payload)
		if decodeErr == nil {
			c.Width = le16(r.Payload[pos+4:])
			c.Height = le16(r.Payload[pos+6:])
			c.ImageOffset = pos
			c.ImageEnd = end
			c.RemainingBytes = len(r.Payload) - end
			if c.Width == 0 && c.Height == 0 {
				c.Status = "palette_only"
				r.Format = "M0 palette with empty X0"
			} else {
				sourceBegin := int(r.Source.Offset) + pos
				sourceEnd := int(r.Source.Offset) + end
				withByte := sourceEnd
				if withByte < len(r.backing) {
					withByte++
				}
				pixels, stats, err := m.decode(r.backing[sourceBegin:withByte], c.Width, c.Height)
				c.Stats = &stats
				decodeErr = err
				if err == nil {
					c.PixelSHA256 = Hash(pixels)
					r.Format = "X0 byte-per-pixel picture"
					if len(palettes) > 0 {
						r.Format = "M0 palette prefix + X0 byte-per-pixel picture"
					}
					c.Status = "converted_diagnostic_grayscale"
					var selected *Palette
					suffix := ".diagnostic-grayscale.png"
					if expected, ok := baselinePictures[r.name]; ok && office != nil && r.fromCatalog("GS2000.CAT") {
						if c.PixelSHA256 != expected {
							decodeErr = errors.New("decoded indices differ from prior full-runtime-validated hash")

						} else {
							selected = office
							suffix = ".png"
							c.Status = "converted_supported_colour"
							c.RuntimeValidation = "prior full64000-byte native bank match, same source/hash; Step9 BASE_AN or Step11 BASE"
							c.Palette = PaletteAssociation{officeSource, officePaletteHash, "Step10 all256 source/installed RGB6 entries at completed office", "prior_runtime_verified", false}
						}
					}
					if expected, ok := officeAtlasPictures[r.name]; ok && office != nil && r.fromCatalog("GS2000.CAT") {
						if c.PixelSHA256 != expected {
							decodeErr = errors.New("office atlas differs from saved independent decode")

						} else {
							selected = office
							suffix = ".png"
							c.Status = "converted_supported_colour"
							c.RuntimeValidation = "prior used office regions/POINTER templates only; full sheet is offline decoded, not full-bank validated"
							c.Palette = PaletteAssociation{officeSource, officePaletteHash, "Step10/18 completed-office global BASE DAC; Step13 unmodified source-index copies/templates; Step21 office-only association", "prior_display_palette_and_source_operations", false}
						}
					}
					if decodeErr == nil {
						if office != nil && r.fromCatalog("GS2000.CAT") {
							if association, ok := menuPaletteAssociation(r.name, officeSource); ok {
								selected, suffix = office, ".png"
								c.Status, c.Palette = "converted_supported_colour", association
							}
						}
						img := image.NewPaletted(image.Rect(0, 0, c.Width, c.Height), paletteColors(selected))
						copy(img.Pix, pixels)
						pngData, e := PngBytes(img)
						if e != nil {
							return e
						}
						if e = addGenerated(out, r, a, ".indices", "decoded_indices", pixels); e != nil {
							return e
						}
						if e = addPalettedGenerated(out, r, a, suffix, "indexed_picture_png", pngData, c.Palette); e != nil {
							return e
						}
						m.Summary.Pictures++
						if selected == nil {
							m.Summary.DiagnosticPictures++
						} else {
							m.Summary.ColourPictures++
						}
					}
				}
			}
		}
	}
	for i, p := range palettes {
		suffix := ""
		if r.Format != "raw RGB6 palette candidate" && !(c.Status == "palette_only" && len(palettes) == 1) {
			suffix = fmt.Sprintf(".palette-%02d", i)
		}
		if err := exportPalette(out, r, a, p, suffix); err != nil {
			return err
		}
		m.Summary.Palettes++
	}
	if decodeErr != nil {
		c.Status = "conversion_failed"
		c.Errors = append(c.Errors, decodeErr.Error())
		m.Summary.ConversionFailures++
	} else if r.Format == "raw RGB6 palette candidate" {
		c.Status = "palette_exported_structural_candidate"
	} else if c.Status == "unsupported" {
		m.Summary.Unsupported++
		m.Summary.UnsupportedFormats[r.Format]++
	}
	return nil
}

type DecodeStats struct {
	Cursor        int `json:"resource_cursor_after_word_prefetch"`
	CodeBits      int `json:"code_bits_consumed"`
	AvailableBits int `json:"available_bits"`
	CodeWidth     int `json:"code_width"`
	NextCode      int `json:"next_code"`
	Resets        int `json:"dictionary_resets"`
	Prefetches    int `json:"prefetch_events"`
	Output        int `json:"output_produced"`
	UnusedBits    int `json:"unused_in_entry_code_bits"`
	Entry256      int `json:"entry256_references"`
}

type pictureDecoder struct {
	byteTail                                           bool
	raw                                                []byte
	limit, width, height, maxBits                      int
	reservoir                                          uint64
	available, cursor, consumed, codeWidth, next, mask int
	prefix                                             [2048]int
	suffix                                             [2048]byte
	valid                                              [2048]bool
	pending                                            []byte
	previousCode, previousFirst                        int
	repeat                                             byte
	previousRLE                                        byte
	resets, prefetches, entry256                       int
}

func (d *pictureDecoder) reset() {
	for i := range d.prefix {
		d.prefix[i] = -1
		d.valid[i] = i < 256
		if i < 256 {
			d.suffix[i] = byte(i)
		}
	}
	d.next = 256
	d.codeWidth = 9
	d.mask = 511
}

func newPictureDecoder(raw []byte) (*pictureDecoder, error) {
	if len(raw) < 10 || string(raw[:2]) != "X0" {
		return nil, errors.New("truncated or unsupported X0 header")
	}
	limit := 4 + le16(raw[2:])
	if limit < 10 || limit > len(raw) {
		return nil, errors.New("X0 declared extent truncated or lacks code initialization")
	}
	w, h := le16(raw[4:]), le16(raw[6:])
	if w == 0 || h == 0 || uint64(w)*uint64(h) > maxPixels {
		return nil, errors.New("dimensions exceed supported output bounds")
	}
	max := int(raw[8])
	if max > 11 {
		max = 11
	}
	if max < 9 {
		return nil, errors.New("maximum code width below9 unsupported")
	}
	d := &pictureDecoder{raw: raw, limit: limit, width: w, height: h, maxBits: max, reservoir: uint64(raw[9]), available: 8, cursor: 10, previousCode: -2, previousFirst: -1}
	d.reset()
	return d, nil
}

func (d *pictureDecoder) code() (int, error) {
	if d.consumed+d.codeWidth > (d.limit-9)*8 {
		return 0, errors.New("code would consume bits beyond named X0 resource")
	}
	for d.available < d.codeWidth {
		if d.byteTail && d.cursor == d.limit-1 {
			d.reservoir |= uint64(d.raw[d.cursor]) << d.available
			d.cursor++
			d.available += 8
			continue
		}
		if d.cursor+2 > d.limit {
			if d.cursor != d.limit-1 || d.cursor+2 > len(d.raw) {
				return 0, errors.New("odd final word requires an original backing byte; none available")
			}
			d.prefetches++
		}
		d.reservoir |= uint64(binary.LittleEndian.Uint16(d.raw[d.cursor:d.cursor+2])) << d.available
		d.cursor += 2
		d.available += 16
	}
	code := int(d.reservoir) & d.mask
	d.reservoir >>= d.codeWidth
	d.available -= d.codeWidth
	d.consumed += d.codeWidth
	return code, nil
}

func (d *pictureDecoder) expand(code int) ([]byte, error) {
	var backwards []byte
	for n := 0; n < 2048; n++ {
		if code < 0 || code >= 2048 || !d.valid[code] {
			return nil, fmt.Errorf("uninitialized/out-of-bounds dictionary reference %d", code)
		}
		if code == 256 {
			d.entry256++
		}
		p := d.prefix[code]
		if p == -2 {
			return nil, errors.New("dictionary entry256 depends on unknown pre-picture previous-code state")
		}
		backwards = append(backwards, d.suffix[code])
		if p == -1 {
			for l, h := 0, len(backwards)-1; l < h; l, h = l+1, h-1 {
				backwards[l], backwards[h] = backwards[h], backwards[l]
			}
			return backwards, nil
		}
		code = p
	}
	return nil, errors.New("dictionary prefix cycle/excessive chain")
}

func (d *pictureDecoder) dictionaryByte() (byte, error) {
	if len(d.pending) == 0 {
		code, err := d.code()
		if err != nil {
			return 0, err
		}
		if code > d.next {
			return 0, fmt.Errorf("code %d exceeds next dictionary code %d", code, d.next)
		}
		var text []byte
		if code == d.next {
			if d.previousCode == -2 || d.previousFirst < 0 {
				return 0, errors.New("first code requires unknown previous-picture state")
			}
			text, err = d.expand(d.previousCode)
			if err == nil {
				text = append(text, byte(d.previousFirst))
			}
		} else {
			text, err = d.expand(code)
		}
		if err != nil {
			return 0, err
		}
		first := text[0]
		d.prefix[d.next] = d.previousCode
		d.suffix[d.next] = first
		d.valid[d.next] = true
		d.next++
		d.previousFirst = int(first)
		if d.next > d.mask {
			d.codeWidth++
			d.mask = d.mask<<1 | 1
		}
		if d.codeWidth > d.maxBits {
			d.reset()
			d.resets++
			d.previousCode = 0
		} else {
			d.previousCode = code
		}
		d.pending = text
	}
	b := d.pending[0]
	d.pending = d.pending[1:]
	return b, nil
}

func (d *pictureDecoder) pixel() (byte, error) {
	if d.repeat != 0 {
		d.repeat--
		return d.previousRLE, nil
	}
	v, err := d.dictionaryByte()
	if err != nil {
		return 0, err
	}
	if v != 0x90 {
		d.previousRLE = v
		return v, nil
	}
	count, err := d.dictionaryByte()
	if err != nil {
		return 0, err
	}
	if count == 0 {
		d.previousRLE = 0x90
		return 0x90, nil
	}
	d.repeat = count - 2
	return d.previousRLE, nil
}

func decodePicture(raw []byte) ([]byte, DecodeStats, error) {
	d, err := newPictureDecoder(raw)
	if err != nil {
		return nil, DecodeStats{}, err
	}
	result := make([]byte, 0, d.width*d.height)
	for len(result) < d.width*d.height {
		b, e := d.pixel()
		if e != nil {
			err = e
			break
		}
		result = append(result, b)
	}
	stats := DecodeStats{d.cursor, d.consumed, d.available, d.codeWidth, d.next, d.resets, d.prefetches, len(result), (d.limit-9)*8 - d.consumed, d.entry256}
	if err != nil {
		return nil, stats, err
	}
	return result, stats, nil
}
