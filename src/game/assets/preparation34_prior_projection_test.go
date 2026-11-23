package assets

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestPreparation34PriorProjection(t *testing.T) {
	root := os.Getenv("GS2000_R231_ASSETS")
	if root == "" {
		t.Skip("requires declared owned Preparation34 output")
	}
	// Link immutable files into a private reader tree, replacing only the changed
	// metadata files. No payload tree is duplicated or source link overwritten.
	// Keep the private links on the declared owned fixture's filesystem. /tmp
	// may be another mount; copying the complete asset tree is unnecessary.
	projected, err := os.MkdirTemp(filepath.Dir(root), ".r231-prior-projection-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(projected) })
	if err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		dst := filepath.Join(projected, rel)
		if d.IsDir() {
			return os.MkdirAll(dst, 0700)
		}
		return os.Link(p, dst)
	}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m Manifest
	if err = DecodeObject(raw, &m); err != nil || m.PreparationVersion != 34 {
		t.Fatal(err, m.PreparationVersion)
	}
	audioRaw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(m.AudioMetadata)))
	if err != nil {
		t.Fatal(err)
	}
	var audio AudioMetadata
	if err = json.Unmarshal(audioRaw, &audio); err != nil || audio.Schema != 9 {
		t.Fatal(err, audio.Schema)
	}
	removed := map[string]bool{}
	tracks := audio.Tracks[:0]
	for _, track := range audio.Tracks {
		if track.Key == "gs2_06" {
			removed[track.Output.Path], removed[track.Sequence.Path] = true, true
		} else {
			tracks = append(tracks, track)
		}
	}
	if len(removed) != 2 {
		t.Fatal("ineffective audio projection", removed)
	}
	audio.Tracks = tracks
	commands := audio.GS2Commands[:0]
	for _, command := range audio.GS2Commands {
		if command.EventID != 0x06 {
			commands = append(commands, command)
		}
	}
	audio.GS2Commands = commands
	files := audio.Files[:0]
	for _, file := range audio.Files {
		if !removed[file.Path] {
			files = append(files, file)
		}
	}
	audio.Files, audio.Schema = files, 8
	// The source-module audit table also records the supported fixed-command
	// inventory. Its source bytes/handlers are unchanged; remove precisely06
	// from this declarative support list and update its typed references.
	var tablePath string
	for _, carrier := range audio.Carriers {
		if carrier.Name == "ASOUND.GS2" && carrier.Tables != nil {
			tablePath = carrier.Tables.Path
		}
	}
	if tablePath == "" {
		t.Fatal("missing source GS2 audit table")
	}
	tableRaw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(tablePath)))
	if err != nil {
		t.Fatal(err)
	}
	var table, layout map[string]json.RawMessage
	if err = json.Unmarshal(tableRaw, &table); err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(table["Layout"], &layout); err != nil {
		t.Fatal(err)
	}
	var supported, oldSupported []int
	if err = json.Unmarshal(layout["FixedGameplayEvents"], &supported); err != nil {
		t.Fatal(err)
	}
	for _, event := range supported {
		if event != 0x06 {
			oldSupported = append(oldSupported, event)
		}
	}
	if len(oldSupported) != len(supported)-1 {
		t.Fatal("ineffective audit support-list projection")
	}
	layout["FixedGameplayEvents"], _ = json.Marshal(oldSupported)
	table["Layout"], _ = json.Marshal(layout)
	tableRaw, _ = json.Marshal(table)
	for i := range audio.Carriers {
		if out := audio.Carriers[i].Tables; out != nil && out.Path == tablePath {
			out.SHA256, out.Size = Hash(tableRaw), len(tableRaw)
		}
	}
	for i := range audio.Files {
		if out := &audio.Files[i]; out.Path == tablePath {
			out.SHA256, out.Size = Hash(tableRaw), len(tableRaw)
		}
	}
	audioRaw, _ = json.Marshal(audio)
	for _, r := range m.Resources {
		outputs := r.Outputs[:0]
		for _, output := range r.Outputs {
			if removed[output.Path] {
				continue
			}
			if output.Path == m.AudioMetadata {
				output.SHA256, output.Size = Hash(audioRaw), len(audioRaw)
			}
			if output.Path == tablePath {
				output.SHA256, output.Size = Hash(tableRaw), len(tableRaw)
			}
			outputs = append(outputs, output)
		}
		r.Outputs = outputs
	}
	m.PreparationVersion = 33
	replace := func(name string, b []byte) {
		p := filepath.Join(projected, filepath.FromSlash(name))
		if err := os.Remove(p); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, b, 0600); err != nil {
			t.Fatal(err)
		}
	}
	replace(m.AudioMetadata, audioRaw)
	replace(tablePath, tableRaw)
	raw, _ = JsonBytes(m)
	replace("manifest.json", raw)
	r, err := newPreparedReader(projected)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := canonicalLogicalResourceDataMode(r, &m, true)
	if err != nil || Hash(decoded) != decodedPreparation33 {
		t.Fatal("prior complete semantic profile changed", Hash(decoded), err)
	}
	if profile, err := LogicalResourceProfile(projected); err != nil || profile != LogicalPreparation33 {
		t.Fatal("prior public identity changed", profile, err)
	}
	t.Log("Removing only source warning06 and schema/version changes reproduces the complete Preparation33 decoded profile", Hash(decoded))
}

func TestPreparation34PinnedCompleteProfile(t *testing.T) {
	root := os.Getenv("GS2000_R231_ASSETS")
	if root == "" {
		t.Skip("complete owned Preparation34 required")
	}
	r, e := newPreparedReader(root)
	if e != nil {
		t.Fatal(e)
	}
	raw, e := r.read("manifest.json", 64<<20)
	if e != nil {
		t.Fatal(e)
	}
	var m Manifest
	if e = DecodeObject(raw, &m); e != nil {
		t.Fatal(e)
	}
	logical, e := canonicalLogicalResourceProfile(r, &m)
	if e != nil {
		t.Fatal(e)
	}
	decoded, e := canonicalLogicalResourceDataMode(r, &m, true)
	if e != nil {
		t.Fatal(e)
	}
	if logical != LogicalPreparation34 || Hash(decoded) != decodedPreparation34 {
		t.Fatalf("new complete profile logical=%s decoded=%s", logical, Hash(decoded))
	}
}
