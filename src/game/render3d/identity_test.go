package render3d

import (
	"encoding/json"
	"gunship2000-assets/game/world"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// Explicit offline inventory export, not part of normal startup/preparation.
func TestExportFillIdentities(t *testing.T) {
	out := os.Getenv("GS2000_R81_IDENTITIES")
	if out == "" {
		t.Skip("explicit inventory export only")
	}
	b, e := world.Load(filepath.Join(os.Getenv("GS2000_TEST_MISSIONS"), "world"))
	if e != nil {
		t.Fatal(e)
	}
	var ids []materialIdentity
	for _, g := range b.Geometries {
		found := false
		var visit func([]world.Group)
		visit = func(groups []world.Group) {
			for _, x := range groups {
				if p := x.Primitive; p != nil && (p.Opcode == 1 || p.Opcode >= 3 && p.Opcode <= 6) && p.Colour > 0 && p.Colour < 256 {
					found = true
				}
				visit(x.Children)
			}
		}
		visit(g.PlaneGroups)
		visit(g.DirectGroups)
		if !found {
			continue
		}
		source := "OBJFILE"
		if strings.HasPrefix(g.ID, "effects/") {
			source = "GS2.GS2"
		}
		if strings.HasPrefix(g.ID, "world/") {
			source = strings.Split(g.ID, "/")[1]
		}
		if b.Sources[source] == "" || g.SourceHash == "" {
			t.Fatalf("unbound source %s", g.ID)
		}
		ids = append(ids, materialIdentity{g.ID, source, b.Sources[source], g.SourceHash, g.SourceOffset, g.SourceLength, materialLayoutHash(g)})
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i].Geometry < ids[j].Geometry })
	raw, e := json.MarshalIndent(ids, "", "  ")
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(out, append(raw, '\n'), 0600); e != nil {
		t.Fatal(e)
	}
}
