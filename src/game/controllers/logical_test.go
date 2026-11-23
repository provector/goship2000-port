package controllers

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLegacyReadAndUnrelatedSavePreserveAssignments(t *testing.T) {
	c := testConfig()
	p := c.Profiles[c.Selected]
	p.Bindings["menu.selector1"] = Binding{Kind: "control", Control: &Control{Kind: "button", Index: 1}}
	p.Bindings["weapon.fire"] = Binding{Kind: "control", Control: &Control{Kind: "button", Index: 2}}
	p.Bindings["flight.pitch"] = Binding{Invert: true}
	c.Profiles[p.ID] = p
	d, e := Load(filepath.Join(t.TempDir(), Filename))
	if e != nil {
		t.Fatal(e)
	}
	d.Config = c
	if e = d.Save(); e != nil {
		t.Fatal(e)
	}
	before, _ := os.ReadFile(d.Path)
	loaded, e := Load(d.Path)
	if e != nil {
		t.Fatal(e)
	}
	if _, conflict := EffectiveBinding(loaded.Config.Profiles[p.ID], "menu.selector1"); !conflict {
		t.Fatal("different legacy bindings not surfaced")
	}
	if e = loaded.Save(); e != nil {
		t.Fatal(e)
	}
	after, _ := os.ReadFile(d.Path)
	if string(before) != string(after) {
		t.Fatal("unrelated save rewrote legacy choices")
	}
	if !reflect.DeepEqual(loaded.Config.Profiles[p.ID].Bindings, p.Bindings) {
		t.Fatal("legacy values changed")
	}
	if b, _ := EffectiveBinding(p, "flight.pitch"); !b.Invert {
		t.Fatal("unbound inversion lost")
	}
	SetBinding(&p, "menu.selector1", p.Bindings["weapon.fire"])
	if b, conflict := EffectiveBinding(p, "weapon.fire"); conflict || b.Control.Index != 2 {
		t.Fatal("explicit alias resolution")
	}
	SetBinding(&p, "menu.selector1", Binding{})
	if b, _ := EffectiveBinding(p, "weapon.fire"); b.Kind != "" {
		t.Fatal("clear resurrects legacy alias")
	}
}
func TestSwitchWeaponIsDiscreteSeventhStep(t *testing.T) {
	if len(Basic) != 7 || Basic[6] != "weapon.weapon" {
		t.Fatal(Basic)
	}
	s := neutral()
	c := NewCapture(Basic[6], s)
	s.Axes[0] = 1
	if _, done := c.Update(s); done {
		t.Fatal("Switch weapon accepted analog input")
	}
	s.Hats[0] = 9
	b, done := c.Update(s)
	if !done || b.Kind != "control" || b.Control.Direction != 9 {
		t.Fatal("weapon capture")
	}
}
