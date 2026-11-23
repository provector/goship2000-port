package replay

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testFilm(id string) *Film {
	return &Film{Header: Header{Version: Version, ID: id, Title: "Last Mission", Simulation: Compatibility, InputSchema: InputCompatibility, InitializationSchema: "accepted-native-launch-v1", Resources: strings.Repeat("a", 64), Complete: true}, Launch: json.RawMessage(`{"typed_launch":true}`), End: End{Checkpoint: map[string]string{"flight": strings.Repeat("b", 64)}}}
}
func TestRejectRetiredAndUnknownStorage(t *testing.T) {
	for _, v := range []uint16{1, 4, 65535} {
		b := append([]byte(nil), blockPrefix...)
		order.PutUint16(b[8:10], v)
		if _, e := Unmarshal(b); e == nil || !strings.Contains(e.Error(), "unsupported G2R storage version") {
			t.Fatal(v, e)
		}
		root := t.TempDir()
		id := strings.Repeat("a", 32)
		path := filepath.Join(root, id+".g2r")
		if e := os.WriteFile(path, b, 0600); e != nil {
			t.Fatal(e)
		}
		rows, e := (Store{root}).List()
		if e != nil || len(rows) != 1 || !strings.Contains(rows[0].Error, "unsupported G2R storage version") {
			t.Fatal(rows, e)
		}
		if _, e := OpenFile(path); e == nil || !strings.Contains(e.Error(), "unsupported G2R storage version") {
			t.Fatal(v, e)
		}
	}
}
func TestFilmManagedPublicationAndRetainedNames(t *testing.T) {
	s := Store{Root: t.TempDir()}
	f := testFilm(strings.Repeat("1", 32))
	if e := s.PublishLast(f); e != nil {
		t.Fatal(e)
	}
	if e := s.Rename(f.Header.ID, "Retained"); e != nil {
		t.Fatal(e)
	}
	if e := s.PublishLast(f); e != nil {
		t.Fatal(e)
	}
	g, _ := s.Load(f.Header.ID)
	if g.Header.Title != "Retained" {
		t.Fatal("repeated publication overwrote retained title")
	}
	h := testFilm(strings.Repeat("2", 32))
	if e := s.PublishLast(h); e != nil {
		t.Fatal(e)
	}
	list, e := s.List()
	if e != nil || len(list) != 2 {
		t.Fatal("named film lost", list, e)
	}
	j := testFilm(strings.Repeat("3", 32))
	if e = s.PublishLast(j); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Load(h.Header.ID); !os.IsNotExist(e) {
		t.Fatal("unretained Last not replaced", e)
	}
	list, _ = s.List()
	if len(list) != 2 {
		t.Fatal("unretained orphan appears")
	}
	if e = s.Delete("../escape"); e == nil {
		t.Fatal("path traversal")
	}
	bad := strings.Repeat("4", 32)
	os.WriteFile(filepath.Join(s.Root, bad+".g2r"), []byte("bad"), 0600)
	if e = s.Delete(bad); e != nil {
		t.Fatal("confirmed corrupt managed film not deletable", e)
	}
	target := filepath.Join(t.TempDir(), "private")
	os.WriteFile(target, []byte("private"), 0600)
	linked := strings.Repeat("5", 32)
	os.Symlink(target, filepath.Join(s.Root, linked+".g2r"))
	if _, e = s.Load(linked); e == nil {
		t.Fatal("symlink read")
	}
	if e = s.Delete(linked); e == nil {
		t.Fatal("symlink deletion")
	}
	data, _ := os.ReadFile(target)
	if string(data) != "private" {
		t.Fatal("unowned file changed")
	}
}
