package setupui

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"gunship2000-assets/game/configuration"
)

func TestOriginalInstallationDiscoveryAndSave(t *testing.T) {
	src := os.Getenv("GS2000_TEST_ORIGINAL")
	if src == "" {
		t.Skip("requires supported installation fixture for six source signatures")
	}
	for _, root := range []string{".steam/steam", "xdg/Steam", ".var/app/com.valvesoftware.Steam/data/Steam"} {
		t.Run(root, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("XDG_DATA_HOME", filepath.Join(home, "xdg"))
			app := filepath.Join(home, "game")
			if err := os.Mkdir(app, 0700); err != nil {
				t.Fatal(err)
			}
			config := filepath.Join(app, configuration.Filename)
			lib := filepath.Join(home, "My Steam Library")
			source := filepath.Join(lib, "steamapps/common/Gunship Bundle/GD/Gunship2000/MPS/GS2000CD")
			write := func(path string, b []byte) {
				t.Helper()
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, b, 0600); err != nil {
					t.Fatal(err)
				}
			}
			for _, n := range []string{"GS.GS2", "GS2.GS2", "GS2000.CAT", "GS2000.DAT", "MBUILDER.CAT", "FONTS.GS2"} {
				data, err := os.ReadFile(filepath.Join(src, n))
				if err != nil {
					t.Fatal(err)
				}
				write(filepath.Join(source, n), data)
			}
			vdf := filepath.Join(home, root, "steamapps/libraryfolders.vdf")
			write(vdf, []byte(fmt.Sprintf(`"libraryfolders" { "3" { "path" %q } }`, lib)))
			manifest := filepath.Join(lib, "steamapps/appmanifest_2766090.acf")
			metadata := []byte(`"AppState" { "appid" "2766090" "installdir" "Gunship Bundle" }`)
			write(manifest, metadata)
			doc, _, err := configuration.Initialize(config)
			if err != nil {
				t.Fatal(err)
			}
			if doc.Config.Original != "original" {
				t.Fatal(doc.Config.Original)
			}
			read := func() []byte {
				b, err := os.ReadFile(config)
				if err != nil {
					t.Fatal(err)
				}
				return b
			}
			before := read()
			open := func() *UI {
				d, err := configuration.Load(config)
				if err != nil {
					t.Fatal(err)
				}
				u := New(d)
				u.open("settings")
				return u
			}
			display := func(u *UI, want string) {
				t.Helper()
				for i, s := range u.settings {
					if s.ID == "original" {
						if u.Table.GetCell(i, 0).Text != "Original Installation" || u.Table.GetCell(i, 1).Text != want {
							t.Fatalf("row %q / %q want %q", u.Table.GetCell(i, 0).Text, u.Table.GetCell(i, 1).Text, want)
						}
						return
					}
				}
				t.Fatal("missing Original Installation in PORT SETTINGS")
			}
			action := func(u *UI, id string) {
				u.home()
				u.Menu.SetCurrentItem(slices.Index(u.mainIDs, id))
				u.Menu.InputHandler()(tcell.NewEventKey(tcell.KeyEnter, 0, 0), func(p tview.Primitive) { u.App.SetFocus(p) })
			}
			u := open()
			display(u, "Choose Steam, GOG or Custom")
			u.activate(1)
			press := func(k tcell.Key) {
				u.App.GetFocus().InputHandler()(tcell.NewEventKey(k, 0, 0), func(p tview.Primitive) { u.App.SetFocus(p) })
			}
			if list, ok := u.App.GetFocus().(*tview.List); !ok || list.GetItemCount() != 2 {
				t.Fatal("expected Steam and Custom choices")
			}
			press(tcell.KeyEnter)
			press(tcell.KeyRight)
			press(tcell.KeyEnter) // Cancel
			if u.popup || u.Doc.Config.OriginalSource != configuration.OriginalUnselected || !bytes.Equal(before, read()) {
				t.Fatal("cancel changed preference")
			}
			u.activate(1)
			press(tcell.KeyEnter)
			press(tcell.KeyEnter) // Apply, including focus on the default Steam choice
			if u.popup {
				t.Fatal("did not apply")
			}
			display(u, "Steam — "+source)
			action(u, "save")
			if !u.Saved {
				t.Fatal("Save & Exit failed")
			}
			if err := os.Remove(manifest); err != nil {
				t.Fatal(err)
			}
			display(open(), "Steam — "+source)
			g, err := configuration.LoadApplication(app, "")
			if err != nil || g.Original != source || g.Config.OriginalSource != configuration.OriginalSteam {
				t.Fatal(g.Config.Original, err)
			}
			u = open()
			u.Doc.Config.Original = "../missing custom"
			u.Doc.Config.OriginalSource = configuration.OriginalCustom
			action(u, "save")
			u = open()
			if got := u.Doc.Config.Original; got != "../missing custom" {
				t.Fatal(got)
			}
		})
	}
}
