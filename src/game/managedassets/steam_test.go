package managedassets

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestSteamLibrariesAndFallback(t *testing.T) {
	for _, v := range []string{"default", "modern-secondary", "old-secondary", "stale", "wrong-app", "escape", "other-game"} {
		t.Run(v, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "Steam")
			lib := root
			write := func(p, v string) {
				t.Helper()
				if e := os.MkdirAll(filepath.Dir(p), 0700); e != nil {
					t.Fatal(e)
				}
				if e := os.WriteFile(p, []byte(v), 0600); e != nil {
					t.Fatal(e)
				}
			}
			if v == "modern-secondary" || v == "old-secondary" {
				lib = filepath.Join(t.TempDir(), "Gry Żółte")
				d := "steamapps"
				entry := fmt.Sprintf(`"1" { "path" %q "apps" { "2766090" "100" } }`, lib)
				if v == "old-secondary" {
					d = "config"
					entry = fmt.Sprintf(`"1" %q`, lib)
				}
				write(filepath.Join(root, d, "libraryfolders.vdf"), `"LibraryFolders" { `+entry+` }`)
			}
			id := steamApp
			name := "Gunship bundle"
			if v == "wrong-app" {
				id = "1"
			}
			if v == "escape" {
				name = "../elsewhere"
			}
			if v != "stale" {
				write(filepath.Join(lib, "steamapps", "appmanifest_"+steamApp+".acf"), fmt.Sprintf(`"AppState" { "appid" %q "installdir" %q }`, id, name))
			}
			want := filepath.Join(lib, "steamapps", "common", name, "GD", "Gunship2000", "MPS", "GS2000CD")
			got := steamInstallation([]string{root}, func(p string) bool { return p == want && v != "other-game" })
			if v == "stale" || v == "escape" || v == "wrong-app" || v == "other-game" {
				want = ""
			}
			if got != want {
				t.Fatalf("got %q want %q", got, want)
			}
		})
	}
}

func TestConfiguredNestedInstallPrecedesSteam(t *testing.T) {
	src := os.Getenv("GS2000_TEST_ORIGINAL")
	if src == "" {
		t.Skip("requires the supported installation fixture")
	}
	root := t.TempDir()
	nested := filepath.Join(root, "my collection", "custom layout")
	if e := os.MkdirAll(nested, 0700); e != nil {
		t.Fatal(e)
	}
	for _, n := range []string{"GS.GS2", "GS2.GS2", "GS2000.CAT", "GS2000.DAT", "MBUILDER.CAT", "FONTS.GS2"} {
		b, e := os.ReadFile(filepath.Join(src, n))
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(filepath.Join(nested, n), b, 0600); e != nil {
			t.Fatal(e)
		}
	}
	if matched, _ := probeConfiguredSource(root); !matched {
		t.Fatal("configured installation would be bypassed")
	}
	if e := os.Remove(filepath.Join(nested, "MBUILDER.CAT")); e != nil {
		t.Fatal(e)
	}
	if matched, _ := probeConfiguredSource(root); matched || supportedSteamData(nested) {
		t.Fatal("incomplete signature admitted")
	}
}

func TestWindowsSecondaryLibraryMetadata(t *testing.T) {
	path := filepath.Join(t.TempDir(), "libraryfolders.vdf")
	data := `"libraryfolders" { "0" { "path" "C:\\Program Files (x86)\\Steam" } "2" { "path" "D:\\Games Łódź\\Steam Library" "apps" { "2766090" "123" } } }`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := readVDF(path)
	if err != nil {
		t.Fatal(err)
	}
	secondary := result["libraryfolders"].children["2"].children
	if secondary["path"].text != `D:\Games Łódź\Steam Library` || secondary["apps"].children[steamApp].text != "123" {
		t.Fatal(secondary)
	}
}
