package main

import (
	"bytes"
	"encoding/json"
	assetdata "gunship2000-assets/game/assets"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func audioOriginalTestDirectory(t *testing.T) string {
	t.Helper()
	p := os.Getenv("GS2000_TEST_AUDIO_ORIGINAL")
	if p == "" {
		t.Skip("set GS2000_TEST_AUDIO_ORIGINAL for source-backed preparation safety checks")
	}
	return p
}

func copyAudioTestSources(t *testing.T, base string) string {
	t.Helper()
	d := filepath.Join(t.TempDir(), "original")
	if e := os.Mkdir(d, 0700); e != nil {
		t.Fatal(e)
	}
	roles := append(append([]assetdata.SourceRole{}, assetdata.AudioSourceRoles...), assetdata.RoleMainExecutable, assetdata.RoleLogoPlayer, assetdata.RoleIntroPlayer, assetdata.RoleSimulationExecutable)
	for _, role := range roles {
		source := assetdata.IslandIce469085.Source(role)
		b, e := os.ReadFile(filepath.Join(base, source.Name))
		if e != nil {
			t.Fatal(e)
		}
		if assetdata.Hash(b) != source.SHA256 {
			t.Fatalf("source identity differs %s", source.Name)
		}
		if e = os.WriteFile(filepath.Join(d, source.Name), b, 0600); e != nil {
			t.Fatal(e)
		}
	}
	return d
}

func TestAudioUnknownProfileRetainsRawWithoutInvokingHelper(t *testing.T) {
	input := copyAudioTestSources(t, audioOriginalTestDirectory(t))
	parent := t.TempDir()
	output := filepath.Join(parent, "output")
	name := assetdata.IslandIce469085.Source(assetdata.RoleAudioGS4).Name
	p := filepath.Join(input, name)
	b, e := os.ReadFile(p)
	if e != nil {
		t.Fatal(e)
	}
	b[len(b)-1] ^= 1
	if e = os.WriteFile(p, b, 0600); e != nil {
		t.Fatal(e)
	}
	block := filepath.Join(parent, "blocked-temp")
	os.WriteFile(block, []byte("file"), 0600)
	t.Setenv("TMPDIR", block)
	manifest, e := prepareAssets(input, output)
	if e != nil {
		t.Fatal("generic raw extraction must survive unknown audio identity:", e)
	}
	if manifest.AudioMetadata != "" {
		t.Fatal("unknown version gained audio interpretation")
	}
	out, e := os.ReadFile(filepath.Join(output, "loose", name))
	if e != nil || !bytes.Equal(out, b) {
		t.Fatal("unknown MZ audio payload lost or rewritten", e)
	}
	found := false
	for _, u := range manifest.UnsupportedVersionOperations {
		if u.Operation == "startup/front-end audio" && strings.Contains(u.Reason, name) {
			found = true
		}
	}
	if !found {
		t.Fatal("unsupported audio operation lacks source diagnostic")
	}
}

func TestAudioRealHelperFailurePreservesManagedPreviousTree(t *testing.T) {
	input := copyAudioTestSources(t, audioOriginalTestDirectory(t))
	parent := t.TempDir()
	assets := filepath.Join(parent, "assets")
	if e := os.Mkdir(assets, 0700); e != nil {
		t.Fatal(e)
	}
	marker := []byte("previous usable generated tree")
	os.WriteFile(filepath.Join(assets, "marker"), marker, 0600)
	metadata, _ := json.Marshal(assetdata.Manifest{Generator: assetdata.PreparationGenerator, SchemaVersion: assetdata.PreparedManifestSchema, PreparationVersion: 5})
	os.WriteFile(filepath.Join(assets, "manifest.json"), metadata, 0600)
	blocked := filepath.Join(parent, "blocked-temp")
	os.WriteFile(blocked, []byte("not a directory"), 0600)
	t.Setenv("XDG_CACHE_HOME", blocked)
	t.Setenv("LocalAppData", blocked)
	services := defaultStartupServices()
	services.load = func(p string) (*assetdata.OfficeAssets, error) {
		b, e := os.ReadFile(filepath.Join(p, "marker"))
		if e == nil && bytes.Equal(b, marker) {
			return &assetdata.OfficeAssets{}, nil
		}
		return nil, os.ErrNotExist
	}
	_, e := ensureOfficeAssets(ApplicationPaths{Directory: parent, Original: input, Assets: assets}, true, io.Discard, services)
	if e == nil || !strings.Contains(e.Error(), "create helper cache") {
		t.Fatal("expected real bundled helper creation failure", e)
	}
	now, e := os.ReadFile(filepath.Join(assets, "marker"))
	if e != nil || !bytes.Equal(now, marker) {
		t.Fatal("previous tree altered", e)
	}
	now, e = os.ReadFile(filepath.Join(assets, "manifest.json"))
	if e != nil || !bytes.Equal(now, metadata) {
		t.Fatal("previous manifest altered", e)
	}
	if _, e = services.load(assets); e != nil {
		t.Fatal("previous assets no longer usable")
	}
}

func TestAudioPreparedPlaybackNeedsNoRawExecutablesOrHelper(t *testing.T) {
	source := os.Getenv("GS2000_TEST_AUDIO_PREPARED")
	if source == "" {
		t.Skip("set GS2000_TEST_AUDIO_PREPARED")
	}
	dir := t.TempDir()
	raw, e := os.ReadFile(filepath.Join(source, "manifest.json"))
	if e != nil {
		t.Fatal(e)
	}
	var main assetdata.Manifest
	if e = json.Unmarshal(raw, &main); e != nil {
		t.Fatal(e)
	}
	metaRaw, e := os.ReadFile(filepath.Join(source, filepath.FromSlash(main.AudioMetadata)))
	if e != nil {
		t.Fatal(e)
	}
	var meta assetdata.AudioMetadata
	if e = json.Unmarshal(metaRaw, &meta); e != nil {
		t.Fatal(e)
	}
	copyFile := func(path string, b []byte) {
		p := filepath.Join(dir, filepath.FromSlash(path))
		if e := os.MkdirAll(filepath.Dir(p), 0700); e != nil {
			t.Fatal(e)
		}
		if e := os.WriteFile(p, b, 0600); e != nil {
			t.Fatal(e)
		}
	}
	copyFile("manifest.json", raw)
	copyFile(main.AudioMetadata, metaRaw)
	for _, tr := range meta.Tracks {
		b, e := os.ReadFile(filepath.Join(source, filepath.FromSlash(tr.Output.Path)))
		if e != nil {
			t.Fatal(e)
		}
		copyFile(tr.Output.Path, b)
	}
	// L.2's retained flight owner also consumes the prepared initialized-data
	// carrier. This is data, not MZ code or a dependency on the original tree.
	for _, carrier := range meta.Carriers {
		if carrier.Name == "ASOUND.GS2" && carrier.InitializedData != nil {
			b, e := os.ReadFile(filepath.Join(source, filepath.FromSlash(carrier.InitializedData.Path)))
			if e != nil {
				t.Fatal(e)
			}
			copyFile(carrier.InitializedData.Path, b)
		}
	}
	// No raw MZ/bank/sequence file is copied. Loading prepared assets must not
	// execute the helper; actual continuous flight playback has its own worker.
	block := filepath.Join(t.TempDir(), "blocked")
	os.WriteFile(block, []byte("file"), 0600)
	t.Setenv("TMPDIR", block)
	a, e := assetdata.LoadAudioAssets(dir)
	// Independent family counts: schema5 has13 front-end/startup +14 fixed
	// GS2 +7 debrief; schema6 adds source row2 warnings4/5; schema7
	// independently adds exact chaff0A and flare2A fixed constructors; schema8
	// adds the original stationary hostile gun29; schema9 adds warning06.
	wantClips := 34
	if meta.Schema == 9 {
		wantClips = 40
	} else if meta.Schema == 8 {
		wantClips = 39
	} else if meta.Schema == 7 {
		wantClips = 38
	} else if meta.Schema == 6 {
		wantClips = 36
	} else if meta.Schema != 5 {
		t.Fatal("unsupported test audio schema", meta.Schema)
	}
	if e != nil || len(a.Clips) != wantClips {
		t.Fatal("prepared-only audio load failed", e)
	}
	if meta.Schema >= 6 {
		if a.GS2Commands[4].Channel != 2 || a.GS2Commands[4].Start != 0x8d8 || a.GS2Commands[5].Channel != 2 || a.GS2Commands[5].Start != 0x8d0 {
			t.Fatal("source warning constructors missing")
		}
	} else if _, ok := a.GS2Commands[4]; ok {
		t.Fatal("historical audio family acquired warning4")
	}
	if meta.Schema >= 7 && !a.SupportsDefenseReleaseAudio() {
		t.Fatal("exact successful-release constructors missing from prepared-only load")
	}
	if meta.Schema >= 8 {
		gun, ok := a.GS2Commands[0x29]
		if !ok || gun.Channel != 1 || gun.Start != 0x8e0 || gun.Alternate != 0 || gun.ClearNoise || gun.HandlerCS != 0x910 || gun.HandlerFile != 0xb10 || gun.HandlerHex != "8d0ee008e9f2fd" || gun.TrackKey != "gs2_29" || !a.SupportsStationaryHostileAudio() {
			t.Fatal("exact source gun29 constructor missing from prepared-only load")
		}
	} else if _, ok := a.GS2Commands[0x29]; ok {
		t.Fatal("historical audio family acquired gun29")
	}
	if meta.Schema >= 9 {
		if !a.SupportsThreatWarningAudio() {
			t.Fatal("source threat-warning06 constructor missing from prepared-only load")
		}
	} else if _, ok := a.GS2Commands[0x06]; ok {
		t.Fatal("historical audio family acquired warning06")
	}
	victim := meta.Tracks[0].Output.Path
	filename := filepath.Join(dir, filepath.FromSlash(victim))
	b, e := os.ReadFile(filename)
	if e != nil {
		t.Fatal(e)
	}
	b[len(b)-1] ^= 1
	os.WriteFile(filename, b, 0600)
	if _, e = assetdata.LoadAudioAssets(dir); e == nil {
		t.Fatal("damaged required audio silently accepted")
	}
}

func TestAudioFrontEndReadyWithoutOriginalsOrRewrite(t *testing.T) {
	assets := os.Getenv("GS2000_TEST_AUDIO_PREPARED")
	if assets == "" {
		t.Skip("set GS2000_TEST_AUDIO_PREPARED")
	}
	raw, e := os.ReadFile(filepath.Join(assets, "manifest.json"))
	if e != nil {
		t.Fatal(e)
	}
	calls := 0
	s := frontEndStartupServices()
	s.prepare = func(string, string) (*assetdata.Manifest, error) { calls++; return nil, os.ErrPermission }
	root := t.TempDir()
	p := ApplicationPaths{Directory: root, Original: filepath.Join(root, "missing-original"), Assets: assets, Saves: filepath.Join(root, "saves")}
	a, e := ensureOfficeAssets(p, false, io.Discard, s)
	if e != nil || a.Audio == nil || calls != 0 {
		t.Fatal("complete assets demanded originals or regeneration", e, calls)
	}
	after, e := os.ReadFile(filepath.Join(assets, "manifest.json"))
	if e != nil || !bytes.Equal(raw, after) {
		t.Fatal("valid reuse rewrote manifest")
	}
}
