package menus

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// HistoricalOperand describes an observed primitive for test-only preparation
// replay. It does not introduce a second normal initialization path.
type HistoricalOperand struct {
	Kind        string
	Asset       string
	Source      [4]int
	Destination [2]int
	End         [2]int
	Colour      byte
	Transparent bool
}

func (o *Office) applyHistoricalOperand(c HistoricalOperand) error {
	switch c.Kind {
	case "copy":
		src := o.Scene
		if c.Asset != "" {
			src = o.Assets.Images[c.Asset]
		}
		return assetdata.OfficeBlit(src, o.Scene, c.Source, c.Destination[0], c.Destination[1], c.Transparent)
	case "line":
		return assetdata.OfficeLine(o.Scene, c.Destination[0], c.Destination[1], c.End[0], c.End[1], c.Colour)
	case "fill":
		if !assetdata.OfficeRectFits([4]int{c.Destination[0], c.Destination[1], c.Source[2], c.Source[3]}) {
			return fmt.Errorf("historical fill outside bank")
		}
		assetdata.OfficeFill(o.Scene, c.Destination[0], c.Destination[1], c.Source[2], c.Source[3], c.Colour)
		return nil
	default:
		return fmt.Errorf("unsupported historical operand %q", c.Kind)
	}
}

// This replay consumes recorded primitive operands, not captured image pixels or
// reconstructed native RNG state. It is deliberately separate from NewOffice's
// normal initialization, whose return precedes the historical group0 checkpoint.
type historicalAssociation struct {
	Banks []int `json:"banks"`
	Slots []struct {
		Name        string `json:"name"`
		BankSegment int    `json:"bank_segment"`
	} `json:"slots"`
}

type historicalCheckpoint struct {
	Registers map[string]int `json:"registers"`
}

type historicalBank struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type historicalEvent struct {
	Kind                  string                `json:"kind"`
	ID                    int                   `json:"id"`
	EntryOffset           int                   `json:"entry_offset"`
	Checkpoint            historicalCheckpoint  `json:"checkpoint"`
	Stack                 []int                 `json:"stack_words"`
	Graphics              string                `json:"graphics_state_hex"`
	Association           historicalAssociation `json:"association"`
	Descriptor            string                `json:"descriptor_raw_hex"`
	SourceDescriptor      string                `json:"source_descriptor_raw_hex"`
	DestinationDescriptor string                `json:"destination_descriptor_raw_hex"`
	SourceRectangle       string                `json:"source_rectangle_hex"`
	Temporary             string                `json:"temporary_raw_hex"`
	SpanEdges             string                `json:"span_edges_raw_hex"`
	Text                  string                `json:"text_raw_hex"`
	Name                  string                `json:"name"`
	ProducerArguments     []int                 `json:"producer_arguments"`
	BeforeBank            historicalBank        `json:"before_bank"`
	AfterBank             historicalBank        `json:"after_bank"`
	Bank                  historicalBank        `json:"bank"`
}

type historicalTrace struct {
	Context struct {
		Segment int `json:"segment"`
	} `json:"context"`
	Events   []historicalEvent `json:"events"`
	Complete bool              `json:"complete"`
}

type historicalPrelude struct {
	Calls []struct {
		Arguments       []int  `json:"arguments"`
		Asset           string `json:"logical_source"`
		Segment         int    `json:"temporary_segment"`
		SourceRectangle string `json:"source_rectangle_hex"`
		Temporary       string `json:"temporary_hex"`
	} `json:"calls"`
}

type historicalTemporary struct {
	Width, Height int
	Pixels        []byte
}

type historicalSnapshot struct {
	ID        int
	Phase     string
	Pixels    []byte
	Reference historicalBank
}

type historicalCheck struct {
	ID               int    `json:"id"`
	Kind             string `json:"kind"`
	Actual, Expected string
}

type historicalMismatch struct {
	GeneratedSHA string `json:"generated_sha256"`
	ReferenceSHA string `json:"reference_sha256"`
	Differing    int    `json:"differing_pixels"`
	PerRow       []int  `json:"per_row_mismatch_counts"`
	First        *struct {
		X, Y                 int
		Generated, Reference byte
	} `json:"first_difference,omitempty"`
}

func historicalReadJSON(t *testing.T, path string, destination any) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, destination); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
}

func historicalHex(s string) ([]byte, error) { return hex.DecodeString(s) }

func historicalWords(s string) ([]int, error) {
	b, err := historicalHex(s)
	if err != nil {
		return nil, err
	}
	if len(b)%2 != 0 {
		return nil, fmt.Errorf("odd word-buffer length")
	}
	out := make([]int, len(b)/2)
	for i := range out {
		out[i] = int(binary.LittleEndian.Uint16(b[i*2:]))
	}
	return out, nil
}

func historicalExtract(source []byte, r [4]int) ([]byte, error) {
	if len(source) != 64000 || !assetdata.OfficeRectFits(r) {
		return nil, fmt.Errorf("historical source rectangle %v outside bank", r)
	}
	out := make([]byte, 0, r[2]*r[3])
	for y := 0; y < r[3]; y++ {
		i := (r[1]+y)*320 + r[0]
		out = append(out, source[i:i+r[2]]...)
	}
	return out, nil
}

func historicalEncoded(temp historicalTemporary) ([]byte, error) {
	if temp.Width <= 0 || temp.Height <= 0 || len(temp.Pixels) != temp.Width*temp.Height {
		return nil, fmt.Errorf("incomplete historical temporary %dx%d with%d pixels", temp.Width, temp.Height, len(temp.Pixels))
	}
	var out bytes.Buffer
	binary.Write(&out, binary.LittleEndian, uint16(temp.Width))
	binary.Write(&out, binary.LittleEndian, uint16(temp.Height))
	for y := 0; y < temp.Height; y++ {
		row := temp.Pixels[y*temp.Width : (y+1)*temp.Width]
		first := 0
		for first < len(row) && row[first] == 0 {
			first++
		}
		end := len(row)
		for end > first && row[end-1] == 0 {
			end--
		}
		binary.Write(&out, binary.LittleEndian, uint16(end-first))
		binary.Write(&out, binary.LittleEndian, uint16(first))
		out.Write(row[first:end])
	}
	return out.Bytes(), nil
}

func historicalPlace(destination []byte, temp historicalTemporary, x, y int) error {
	if !assetdata.OfficeRectFits([4]int{x, y, temp.Width, temp.Height}) || len(temp.Pixels) != temp.Width*temp.Height {
		return fmt.Errorf("historical temporary destination outside bank or incomplete")
	}
	for row := 0; row < temp.Height; row++ {
		for col := 0; col < temp.Width; col++ {
			v := temp.Pixels[row*temp.Width+col]
			if v != 0 {
				destination[(y+row)*320+x+col] = v
			}
		}
	}
	return nil
}

// Native 07ee loads/stores one complete word at a time, then the odd byte.
// This differs from extract-first blitting for some overlapping copies; the
// historical replay retains that behavior without imposing it on CMPX extraction.
func historicalCopy(source, destination []byte, r [4]int, dx, dy int) error {
	if r[2] == 0 || r[3] == 0 {
		return nil
	}
	if len(source) != 64000 || len(destination) != 64000 || !assetdata.OfficeRectFits(r) || !assetdata.OfficeRectFits([4]int{dx, dy, r[2], r[3]}) {
		return fmt.Errorf("unsupported historical copy bounds")
	}
	for y := 0; y < r[3]; y++ {
		si, di := (r[1]+y)*320+r[0], (dy+y)*320+dx
		for x := 0; x+1 < r[2]; x += 2 {
			lo, hi := source[si+x], source[si+x+1]
			destination[di+x], destination[di+x+1] = lo, hi
		}
		if r[2]&1 != 0 {
			destination[di+r[2]-1] = source[si+r[2]-1]
		}
	}
	return nil
}

func historicalSpans(destination []byte, start, end int, edges []int, colour byte, sentinel int) error {
	start, end = int(int16(start)), int(int16(end))
	if start < 0 || start > end {
		return nil
	}
	if len(destination) != 64000 || len(edges) != 400 || end >= 200 {
		return fmt.Errorf("unsupported historical spans")
	}
	for y := start; y <= end; y++ {
		lo, hi := edges[y], edges[200+y]
		if hi < lo || (hi == lo && (lo == 0 || lo == sentinel)) {
			continue
		}
		if lo < 0 || hi >= 320 {
			return fmt.Errorf("span outside bank")
		}
		for x := lo; x <= hi; x++ {
			destination[y*320+x] = colour
		}
	}
	return nil
}

func historicalReplay(a *assetdata.OfficeAssets, trace historicalTrace, prelude historicalPrelude) ([]byte, []historicalSnapshot, []historicalCheck, error) {
	if !trace.Complete {
		return nil, nil, nil, fmt.Errorf("historical trace is incomplete")
	}
	canvas := append([]byte(nil), a.Images["BASE.PIC"]...)
	physical := map[int][]byte{trace.Context.Segment: canvas}
	temporaries := map[int]historicalTemporary{}
	enters := map[int]historicalEvent{}
	var snapshots []historicalSnapshot
	var checks []historicalCheck
	snap := func(id int, phase string, bank historicalBank) {
		snapshots = append(snapshots, historicalSnapshot{id, phase, append([]byte(nil), canvas...), bank})
	}
	witness := func(id int, kind string, generated []byte, expectedHex string) {
		checks = append(checks, historicalCheck{id, kind, hex.EncodeToString(generated), expectedHex})
	}
	imageFor := func(association historicalAssociation, bank int) ([]byte, error) {
		if bank < 0 || bank >= len(association.Banks) {
			return nil, fmt.Errorf("bank%d outside association", bank)
		}
		segment := association.Banks[bank]
		if image, ok := physical[segment]; ok {
			return image, nil
		}
		for _, slot := range association.Slots {
			if slot.BankSegment != segment || slot.Name == "" {
				continue
			}
			source, ok := a.Images[slot.Name+".PIC"]
			if !ok {
				return nil, fmt.Errorf("no prepared source for%s", slot.Name)
			}
			image := append([]byte(nil), source...)
			physical[segment] = image
			return image, nil
		}
		return nil, fmt.Errorf("bank%d segment%x has no original-source provenance", bank, segment)
	}
	for i, c := range prelude.Calls {
		if len(c.Arguments) != 5 {
			return nil, nil, nil, fmt.Errorf("prelude%d arguments", i)
		}
		r := [4]int{c.Arguments[1], c.Arguments[2], c.Arguments[3], c.Arguments[4]}
		pixels, err := historicalExtract(a.Images[c.Asset], r)
		if err != nil {
			return nil, nil, nil, err
		}
		temp := historicalTemporary{r[2], r[3], pixels}
		temporaries[c.Segment] = temp
		witness(i, "prelude-source", pixels, c.SourceRectangle)
		encoded, err := historicalEncoded(temp)
		if err != nil {
			return nil, nil, nil, err
		}
		witness(i, "prelude-temporary", encoded, c.Temporary)
	}
	var pending [2]int
	latest := 0
	var textSetup *historicalEvent
	for _, event := range trace.Events {
		e := event
		fail := func(err error) ([]byte, []historicalSnapshot, []historicalCheck, error) {
			return nil, nil, nil, fmt.Errorf("event%d %s MGRAPHIC+%04x: %w", e.ID, e.Kind, e.EntryOffset, err)
		}
		registers := e.Checkpoint.Registers
		switch e.Kind {
		case "picture_population":
			snap(e.ID, "picture-before", e.BeforeBank)
			if stringsHasPictureSuffix(e.Name) {
				if len(e.ProducerArguments) < 1 {
					return fail(fmt.Errorf("missing producer arguments"))
				}
				bank := e.ProducerArguments[0]
				if bank < 0 || bank >= len(e.Association.Banks) {
					return fail(fmt.Errorf("producer bank"))
				}
				source, ok := a.Images[e.Name]
				if !ok {
					return fail(fmt.Errorf("picture%s not prepared", e.Name))
				}
				physical[e.Association.Banks[bank]] = append([]byte(nil), source...)
			}
			snap(e.ID, "picture-after", e.AfterBank)
		case "text_body":
			snap(e.ID, "text-body", e.Bank)
			if textSetup == nil {
				return fail(fmt.Errorf("text has no setup"))
			}
			d, err := historicalWords(textSetup.Descriptor)
			if err != nil || len(d) < 9 {
				return fail(fmt.Errorf("text descriptor malformed"))
			}
			if d[1] != 0 || d[2] != 0 || d[3] != 319 || d[4] != 199 {
				return fail(fmt.Errorf("unmodeled descriptor clipping/origin"))
			}
			dest, err := imageFor(textSetup.Association, d[0])
			if err != nil {
				return fail(err)
			}
			raw, err := historicalHex(textSetup.Text)
			if err != nil {
				return fail(err)
			}
			font := a.Fonts[d[8]]
			if font == nil {
				return fail(fmt.Errorf("missing font%d", d[8]))
			}
			r := textSetup.Checkpoint.Registers
			if err = font.Draw(dest, raw, r["CX"], r["DX"], d[6], d[7], d[5]); err != nil {
				return fail(err)
			}
		case "enter":
			enters[e.ID] = e
			snap(e.ID, "before", e.BeforeBank)
			stack := e.Stack
			if len(stack) < 10 {
				return fail(fmt.Errorf("short stack capture"))
			}
			graphics, err := historicalHex(e.Graphics)
			if err != nil || len(graphics) < 29 {
				return fail(fmt.Errorf("invalid graphics state"))
			}
			words, err := historicalWords(e.Graphics[:52])
			if err != nil {
				return fail(err)
			}
			destSegment, ox, oy := words[8], words[9], words[10]
			colour, opcode := graphics[26], graphics[28]
			switch e.EntryOffset {
			case 0xa90:
				pending = [2]int{stack[2], stack[3]}
			case 0xaf8:
				temp, ok := temporaries[latest]
				if !ok {
					return fail(fmt.Errorf("row encoder has no initialized temporary"))
				}
				source, ok := physical[registers["DS"]]
				if !ok {
					return fail(fmt.Errorf("row segment lacks source provenance"))
				}
				begin := stack[2]
				if begin < 0 || begin+temp.Width > len(source) {
					return fail(fmt.Errorf("row outside source"))
				}
				temp.Pixels = append(temp.Pixels, source[begin:begin+temp.Width]...)
				temporaries[latest] = temp
			case 0xd5e:
				d, err := historicalWords(e.DestinationDescriptor)
				if err != nil || len(d) < 3 {
					return fail(fmt.Errorf("invalid placement descriptor"))
				}
				temp, ok := temporaries[stack[5]]
				if !ok {
					return fail(fmt.Errorf("temporary%x lacks source provenance", stack[5]))
				}
				dest, err := imageFor(e.Association, d[0])
				if err != nil {
					return fail(err)
				}
				if err = historicalPlace(dest, temp, (stack[3]+d[1])&65535, (stack[4]+d[2])&65535); err != nil {
					return fail(err)
				}
			case 0xb9a:
				source, err := imageFor(e.Association, stack[2])
				if err != nil {
					return fail(err)
				}
				pixels, err := historicalExtract(source, [4]int{stack[3], stack[4], stack[5], stack[6]})
				if err != nil {
					return fail(err)
				}
				witness(e.ID, "extract-source", pixels, e.SourceRectangle)
			case 0x7ee:
				sd, err := historicalWords(e.SourceDescriptor)
				if err != nil || len(sd) < 3 {
					return fail(fmt.Errorf("invalid copy source descriptor"))
				}
				dd, err := historicalWords(e.DestinationDescriptor)
				if err != nil || len(dd) < 3 {
					return fail(fmt.Errorf("invalid copy destination descriptor"))
				}
				// Display-only bank0 never becomes an input to this bounded trace.
				if dd[0] == 0 {
					break
				}
				source, err := imageFor(e.Association, sd[0])
				if err != nil {
					return fail(err)
				}
				destination, err := imageFor(e.Association, dd[0])
				if err != nil {
					return fail(err)
				}
				r := [4]int{(stack[3] + sd[1]) & 65535, (stack[4] + sd[2]) & 65535, stack[5], stack[6]}
				pixels, err := historicalExtract(source, r)
				if err != nil {
					return fail(err)
				}
				witness(e.ID, "copy-source", pixels, e.SourceRectangle)
				if err = historicalCopy(source, destination, r, (stack[8]+dd[1])&65535, (stack[9]+dd[2])&65535); err != nil {
					return fail(err)
				}
			case 0x5af, 0x42c, 0x56c:
				if opcode != 0x88 {
					return fail(fmt.Errorf("unmodeled graphics write opcode%x", opcode))
				}
				destination, ok := physical[destSegment]
				if !ok {
					return fail(fmt.Errorf("primitive destination lacks provenance"))
				}
				switch e.EntryOffset {
				case 0x5af:
					err = assetdata.OfficeLine(destination, (registers["AX"]+ox)&65535, (registers["BX"]+oy)&65535, (registers["CX"]+ox)&65535, (registers["DX"]+oy)&65535, colour)
				case 0x56c:
					x, y := (registers["AX"]+ox)&65535, (registers["BX"]+oy)&65535
					if x >= 320 || y >= 200 {
						return fail(fmt.Errorf("pixel outside bank"))
					}
					destination[y*320+x] = colour
				case 0x42c:
					var edges []int
					edges, err = historicalWords(e.SpanEdges)
					if err == nil {
						err = historicalSpans(destination, registers["AX"], registers["CX"], edges, colour, words[11])
					}
				}
				if err != nil {
					return fail(err)
				}
			case 0x133e:
				textSetup = &e
			case 0x4bc, 0x4a4, 0x48d, 0x244: // bank/context changes are explicit in subsequent captures.
			default:
				return fail(fmt.Errorf("unmodeled entry"))
			}
		case "exit":
			if _, ok := enters[e.ID]; !ok {
				return fail(fmt.Errorf("exit has no matching entry"))
			}
			if e.EntryOffset == 0xa90 {
				latest = registers["AX"]
				temporaries[latest] = historicalTemporary{Width: pending[0], Height: pending[1]}
			}
			if e.EntryOffset == 0xb9a {
				temp, ok := temporaries[registers["AX"]]
				if !ok {
					return fail(fmt.Errorf("extraction returned unknown temporary"))
				}
				encoded, err := historicalEncoded(temp)
				if err != nil {
					return fail(err)
				}
				witness(e.ID, "temporary-encoding", encoded, e.Temporary)
			}
			snap(e.ID, "after", e.Bank)
		case "inactive_return_stop":
		default:
			return fail(fmt.Errorf("unknown event kind"))
		}
	}
	return canvas, snapshots, checks, nil
}

func stringsHasPictureSuffix(name string) bool { return len(name) >= 4 && name[len(name)-4:] == ".PIC" }

func historicalCompare(generated, reference []byte) (historicalMismatch, error) {
	result := historicalMismatch{GeneratedSHA: assetdata.Hash(generated), ReferenceSHA: assetdata.Hash(reference), PerRow: make([]int, 200)}
	if len(generated) != 64000 || len(reference) != 64000 {
		return result, fmt.Errorf("historical image size mismatch")
	}
	for i, g := range generated {
		if g != reference[i] {
			result.Differing++
			result.PerRow[i/320]++
			if result.First == nil {
				result.First = &struct {
					X, Y                 int
					Generated, Reference byte
				}{i % 320, i / 320, g, reference[i]}
			}
		}
	}
	return result, nil
}

func TestHistoricalOfficePreparationAndGroup0(t *testing.T) {
	assetPath, root := os.Getenv("GS2000_OFFICE_ASSETS"), os.Getenv("GS2000_FIXTURE_ROOT")
	if assetPath == "" || root == "" {
		t.Skip("set GS2000_OFFICE_ASSETS and GS2000_FIXTURE_ROOT for historical operand replay")
	}
	a, err := assetdata.LoadOfficeAssets(assetPath)
	if err != nil {
		t.Fatal(err)
	}
	var trace historicalTrace
	var prelude historicalPrelude
	historicalReadJSON(t, filepath.Join(root, "office-trace/ordered-preparation-trace.json"), &trace)
	historicalReadJSON(t, filepath.Join(root, "office-prelude/pointer-prelude.json"), &prelude)
	prepared, snapshots, checks, err := historicalReplay(a, trace, prelude)
	if err != nil {
		t.Fatal(err)
	}
	prepared = append([]byte(nil), prepared...)
	o := &Office{Assets: a, Scene: append([]byte(nil), prepared...)}
	if err = o.ComposeGroup(0, 0, 0); err != nil {
		t.Fatal(err)
	}
	composed := append([]byte(nil), o.Scene...)
	// All independent outputs now exist. Captured banks are first opened here,
	// solely as expected results, including every intermediate checkpoint.
	var checkpointReports []map[string]any
	for _, snapshot := range snapshots {
		relative, ok := strings.CutPrefix(snapshot.Reference.Path, "evidence/step13/run03/")
		if !ok || !filepath.IsLocal(filepath.FromSlash(relative)) {
			t.Fatalf("unexpected reference path %q", snapshot.Reference.Path)
		}
		reference, err := os.ReadFile(filepath.Join(root, "office-trace", filepath.FromSlash(relative)))
		if err != nil {
			t.Fatal(err)
		}
		comparison, err := historicalCompare(snapshot.Pixels, reference)
		if err != nil {
			t.Fatal(err)
		}
		if comparison.ReferenceSHA != snapshot.Reference.SHA256 {
			t.Fatalf("captured checkpoint hash changed: %s", snapshot.Reference.Path)
		}
		checkpointReports = append(checkpointReports, map[string]any{"id": snapshot.ID, "phase": snapshot.Phase, "comparison": comparison})
		if comparison.Differing != 0 {
			t.Errorf("event%d %s: %d differing pixels; first %+v", snapshot.ID, snapshot.Phase, comparison.Differing, comparison.First)
		}
	}
	for _, check := range checks {
		if check.Actual != check.Expected {
			t.Errorf("%s event%d source witness differs", check.Kind, check.ID)
		}
	}
	preReference, err := os.ReadFile(filepath.Join(root, "office-trace/banks/pre-group.bin"))
	if err != nil {
		t.Fatal(err)
	}
	postReference, err := os.ReadFile(filepath.Join(root, "office-trace/banks/post-group.bin"))
	if err != nil {
		t.Fatal(err)
	}
	preComparison, err := historicalCompare(prepared, preReference)
	if err != nil {
		t.Fatal(err)
	}
	postComparison, err := historicalCompare(composed, postReference)
	if err != nil {
		t.Fatal(err)
	}
	if preComparison.GeneratedSHA != "ca134510802877ba7e8adc1edc1799e3526c602d0ff703b85e70de75102dd201" || preComparison.Differing != 0 {
		t.Errorf("pre-group comparison %+v", preComparison)
	}
	if postComparison.GeneratedSHA != "888fa9e05fcbac7191b5724b6bb185c0e31b606cdcd1499a77ace8d8737810af" || postComparison.Differing != 0 {
		t.Errorf("post-group comparison %+v", postComparison)
	}
	if output := os.Getenv("GS2000_CHECKPOINT_OUTPUT_DIR"); output != "" {
		absolute, pathErr := filepath.Abs(output)
		if pathErr != nil {
			t.Fatal(pathErr)
		}
		base := os.Getenv("GS2000_CHECKPOINT_OUTPUT_ROOT")
		if base == "" {
			t.Fatal("set a disposable GS2000_CHECKPOINT_OUTPUT_ROOT for generated checkpoints")
		}
		allowed, pathErr := filepath.Abs(base)
		if pathErr != nil {
			t.Fatal(pathErr)
		}
		relative, pathErr := filepath.Rel(allowed, absolute)
		if pathErr != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			t.Fatal("historical checkpoint outputs must use a new directory under GS2000_CHECKPOINT_OUTPUT_ROOT")
		}
		if _, err = os.Stat(output); !os.IsNotExist(err) {
			t.Fatal("choose a new GS2000_CHECKPOINT_OUTPUT_DIR to preserve earlier generated outputs")
		}
		if err = os.MkdirAll(output, 0755); err != nil {
			t.Fatal(err)
		}
		report := map[string]any{"scope": "Replay of recorded drawing operands, not animation scheduling or normal initialization-return equality", "preparation": preComparison, "group0": postComparison, "checkpoint_count": len(snapshots), "source_witness_count": len(checks), "checkpoints": checkpointReports, "source_checks": checks, "all_match": !t.Failed()}
		raw, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		for name, data := range map[string][]byte{"prepared.indices": prepared, "group0.indices": composed, "comparison.json": append(raw, '\n')} {
			if err = os.WriteFile(filepath.Join(output, name), data, 0644); err != nil {
				t.Fatal(err)
			}
		}
	}
	t.Logf("historical preparation/group0: %d exact bank checkpoints, %d source/temporary witnesses, pre=%s post=%s", len(snapshots), len(checks), preComparison.GeneratedSHA, postComparison.GeneratedSHA)
}

func TestHistoricalCopyRetainsForwardWordAliasing(t *testing.T) {
	image := make([]byte, 64000)
	copy(image, []byte{1, 2, 3, 4, 5, 6})
	if err := historicalCopy(image, image, [4]int{0, 0, 5, 1}, 1, 0); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(image[:6], []byte{1, 1, 2, 2, 4, 4}) {
		t.Fatalf("forward word alias result %v", image[:6])
	}
}

// The old empty-roster pre-group0 state has no officer group drawn yet. Its
// computer pixels depend on earlier callbacks/native randomness, so compare the
// other 63,865 pixels to validate normal high-level preparation independently of
// the operand replay. The full computer-inclusive check remains the replay above.
func TestOfficeStaticPreparationMatchesHistoricalFixedRegions(t *testing.T) {
	root := os.Getenv("GS2000_FIXTURE_ROOT")
	directory := os.Getenv("GS2000_OFFICE_ASSETS")
	if root == "" || directory == "" {
		t.Skip("set project/assets for historical static-state comparison")
	}
	assets, err := assetdata.LoadOfficeAssets(directory)
	if err != nil {
		t.Fatal(err)
	}
	state := DefaultOfficeState()
	state.PilotName = ""
	state.Squadron = ""
	state.Emblem = 0
	state.Active = false
	state.Theatre = 0
	o, err := NewOffice(assets, state, 7)
	if err != nil {
		t.Fatal(err)
	}
	generated := o.Frame(false)
	expected, err := os.ReadFile(filepath.Join(root, "office-trace/banks/pre-group.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if len(expected) != 64000 {
		t.Fatal("bad reference length")
	}
	compared := 0
	for y := 0; y < 200; y++ {
		for x := 0; x < 320; x++ {
			if x >= 136 && x < 151 && y >= 85 && y < 94 {
				continue
			}
			compared++
			i := y*320 + x
			if generated[i] != expected[i] {
				t.Fatalf("static pixel%d,%d got%02x want%02x", x, y, generated[i], expected[i])
			}
		}
	}
	if compared != 63865 {
		t.Fatal("wrong comparison extent", compared)
	}
	t.Log("63,865 static/caption pixels match; 135 history-dependent computer pixels excluded explicitly")
}
