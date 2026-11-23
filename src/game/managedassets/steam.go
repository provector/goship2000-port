package managedassets

import (
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/game/configuration"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/scanner"
)

const steamApp = "2766090"

// Steam KeyValues files contain nested key/value pairs. Read only named
// metadata files and reject malformed, ambiguous, or excessively nested trees.
type vdfValue struct {
	text     string
	children map[string]vdfValue
}

func readVDF(path string) (map[string]vdfValue, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("expected regular Steam metadata")
	}
	data, err := assetdata.ReadLimitedFile(path, 2<<20)
	if err != nil {
		return nil, err
	}
	var s scanner.Scanner
	s.Init(strings.NewReader(strings.TrimPrefix(string(data), "\ufeff")))
	s.Mode = scanner.ScanStrings | scanner.ScanIdents | scanner.ScanComments | scanner.SkipComments
	s.IsIdentRune = func(r rune, _ int) bool { return r > ' ' && !strings.ContainsRune("{}\"/", r) }
	s.Error = func(_ *scanner.Scanner, _ string) {}
	token := func() (string, rune, error) {
		t := s.Scan()
		v := s.TokenText()
		if t == scanner.String { // VDF escapes quotes and backslashes; leave other path characters literal.
			if len(v) < 2 || v[len(v)-1] != '"' {
				return "", 0, fmt.Errorf("unterminated VDF string")
			}
			v = strings.NewReplacer(`\\`, `\`, `\"`, `"`).Replace(v[1 : len(v)-1])
		}
		return v, t, nil
	}
	count := 0
	var object func(int) (map[string]vdfValue, error)
	object = func(depth int) (map[string]vdfValue, error) {
		if depth > 12 {
			return nil, fmt.Errorf("Steam metadata nesting limit")
		}
		m := make(map[string]vdfValue)
		for {
			key, t, e := token()
			if e != nil {
				return nil, e
			}
			if t == scanner.EOF && depth == 0 || t == '}' && depth > 0 {
				return m, nil
			}
			if t != scanner.String && t != scanner.Ident {
				return nil, fmt.Errorf("invalid Steam metadata key")
			}
			key = strings.ToLower(key)
			if _, exists := m[key]; exists {
				return nil, fmt.Errorf("duplicate Steam metadata key")
			}
			count++
			if count > 50000 {
				return nil, fmt.Errorf("Steam metadata too large")
			}
			v, t, e := token()
			if e != nil {
				return nil, e
			}
			switch t {
			case '{':
				children, e := object(depth + 1)
				if e != nil {
					return nil, e
				}
				m[key] = vdfValue{children: children}
			case scanner.String, scanner.Ident:
				m[key] = vdfValue{text: v}
			default:
				return nil, fmt.Errorf("invalid Steam metadata value")
			}
		}
	}
	return object(0)
}

func supportedSteamData(dir string) bool {
	// Bind the candidate to one supported simulation, frontend and catalogue.
	// Full preparation subsequently validates every required resource in that tree.
	files, e := os.ReadDir(dir)
	if e != nil {
		return false
	}
	names := map[string]bool{}
	for _, f := range files {
		if f.Type().IsRegular() {
			names[f.Name()] = true
		}
	}
	for _, s := range assetdata.IslandIce469085.Sources {
		switch s.Name {
		case "GS.GS2", "GS2.GS2", "GS2000.CAT", "GS2000.DAT", "MBUILDER.CAT", "FONTS.GS2":
			if !names[s.Name] {
				return false
			}
			data, e := assetdata.ReadLimitedFile(filepath.Join(dir, s.Name), 32<<20)
			if e != nil || assetdata.Hash(data) != s.SHA256 {
				return false
			}
		}
	}
	return true
}

// Probe only the selected tree. An unusually large tree keeps its precedence
// and is left for the full preparer to validate, as in T4.
func probeConfiguredSource(root string) (matched, incomplete bool) {
	root, err := filepath.EvalSymlinks(root)
	if err != nil || filepath.Dir(root) == root {
		return false, false
	}
	count := 0
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		count++
		if count > 4096 {
			incomplete = true
			return filepath.SkipAll
		}
		if d.Name() == "GS.GS2" && d.Type().IsRegular() && supportedSteamData(filepath.Dir(p)) {
			matched = true
			return filepath.SkipAll
		}
		return nil
	})
	return matched, incomplete
}

func discoverSteam() string { return steamInstallation(steamRoots(), supportedSteamData) }

// OriginalForSetup resolves the draft once on entry. Keep configured relative
// names portable. Steam supplies the absolute install it actually validated.
func OriginalForSetup(directory, configured string) string {
	selected, _ := selectOriginal(directory, configured, discoverSteam)
	return selected
}

func selectOriginal(directory, configured string, discover func() string) (string, bool) {
	path := configuration.Relative(directory, configured)
	if matched, incomplete := probeConfiguredSource(path); matched || incomplete {
		return configured, true
	}
	local := configuration.DefaultOriginal
	if filepath.Clean(path) != filepath.Join(directory, local) {
		if matched, _ := probeConfiguredSource(filepath.Join(directory, local)); matched {
			return local, true
		}
	}
	if candidate := discover(); candidate != "" {
		return candidate, true
	}
	if filepath.Clean(configured) == filepath.Join("..", local) {
		return local, false
	}
	// Leave an unavailable custom location visible for correction or remount.
	return configured, false
}

func steamInstallation(roots []string, valid func(string) bool) string {
	seen := map[string]bool{}
	for _, root := range roots {
		if !filepath.IsAbs(root) {
			continue
		}
		libraries := []string{root}
		for _, name := range []string{"steamapps", "config"} {
			kv, e := readVDF(filepath.Join(root, name, "libraryfolders.vdf"))
			if e != nil {
				continue
			}
			entries := kv["libraryfolders"].children
			// VDF indices can have gaps; retain Steam's numeric order.
			for i := 0; i < 256; i++ {
				v, ok := entries[strconv.Itoa(i)]
				if !ok {
					continue
				}
				p := v.text
				if v.children != nil {
					p = v.children["path"].text
				}
				if filepath.IsAbs(p) {
					libraries = append(libraries, p)
				}
			}
		}
		for _, lib := range libraries {
			lib = filepath.Clean(lib)
			if seen[lib] {
				continue
			}
			seen[lib] = true
			kv, e := readVDF(filepath.Join(lib, "steamapps", "appmanifest_"+steamApp+".acf"))
			if e != nil {
				continue
			}
			app := kv["appstate"].children
			name := app["installdir"].text
			if app["appid"].text != steamApp || name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\:\x00") {
				continue
			}
			candidate := filepath.Join(lib, "steamapps", "common", name, "GD", "Gunship2000", "MPS", "GS2000CD")
			if valid(candidate) {
				return candidate
			}
		}
	}
	return ""
}
