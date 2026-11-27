package managedassets

import (
	"fmt"
	assetdata "gunship2000-assets/game/assets"

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
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	names := map[string]bool{}
	for _, d := range entries {
		if d.Type().IsRegular() {
			names[strings.ToUpper(d.Name())] = true
		}
	}
	for _, name := range []string{"GS.GS2", "GS2.GS2", "GS2000.CAT", "GS2000.DAT", "MBUILDER.CAT", "FONTS.GS2"} {
		if !names[name] {
			return false
		}
	}
	return true
}

func steamInstallation(roots []string, valid func(string) bool) string {
	results := steamInstallations(roots, valid)
	if len(results) > 0 {
		return results[0]
	}
	return ""
}

func steamInstallations(roots []string, valid func(string) bool) []string {
	var results []string
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
				results = append(results, candidate)
				if len(results) >= 32 {
					return results
				}
			}
		}
	}
	return results
}
