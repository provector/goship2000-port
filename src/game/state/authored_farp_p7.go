package state

import (
	"encoding/binary"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
)

// CheckAuthoredFARPArm reproduces1DA4:11CA's first flag40 constructor and
// 6C4C/1584(-1) checks BEFORE205C refunds an existing FARP. Merely arming a
// new site does not spend its component budget; the accepted placement does.
func CheckAuthoredFARPArm(data *assetdata.MissionGenerationData, d *AuthoredDocument) (AuthoredOption, error) {
	if err := validateAuthoredShape(d); err != nil {
		return AuthoredOption{}, err
	}
	w, err := AuthoredWorld(data, d.World)
	if err != nil {
		return AuthoredOption{}, err
	}
	if authoredDependency(w) != d.Dependency {
		return AuthoredOption{}, fmt.Errorf("mission source world dependency changed")
	}
	opts, err := AuthoredOptions(data, d, "FARP", 0)
	if err != nil {
		return AuthoredOption{}, err
	}
	if len(opts) == 0 {
		return AuthoredOption{}, fmt.Errorf("source FARP descriptor unavailable")
	}
	if d.Initial != nil {
		return AuthoredOption{}, fmt.Errorf("retained film FARP geometry requires a verified inverse compiler")
	}
	if authoredOrdinaryDescriptorCount(d) >= AuthoredEditorPlacementLimit {
		return AuthoredOption{}, fmt.Errorf("source editor ordinary placement list is full (75 descriptors)")
	}
	o := opts[0]
	p := AuthoredPlacement{Role: "FARP", Formation: o.Formation, Component: o.Component, Template: o.Template, SelectionProfile: o.SelectionProfile}
	proposal := append(append([]AuthoredPlacement(nil), d.Placements...), p)
	cost, err := authoredModelAdmission(data, d, w, proposal, d.Support)
	if err != nil {
		return AuthoredOption{}, err
	}
	if !d.NativeModelResources() && cost >= 0x9d42 {
		return AuthoredOption{}, fmt.Errorf("source FARP model resource budget is exhausted")
	}
	if _, err = AuthoredBudget(data, d); err != nil {
		return AuthoredOption{}, err
	}
	return o, nil
}

// LiftAuthoredFARP is20F4:205C's special-record transition. It removes the
// placed FARP immediately and returns a detached held descriptor. Cancelling
// that command leaves the FARP absent. No full-SAVE validation is appropriate:
// a player may lift a FARP before creating either objective or even a base.
func LiftAuthoredFARP(data *assetdata.MissionGenerationData, d *AuthoredDocument) (*AuthoredPlacement, error) {
	if err := validateAuthoredShape(d); err != nil {
		return nil, err
	}
	if d.Initial != nil {
		return nil, fmt.Errorf("retained film FARP geometry requires a verified inverse compiler")
	}
	w, err := AuthoredWorld(data, d.World)
	if err != nil {
		return nil, err
	}
	if authoredDependency(w) != d.Dependency {
		return nil, fmt.Errorf("mission source world dependency changed")
	}
	q, err := CloneAuthored(d)
	if err != nil {
		return nil, err
	}
	at := -1
	for i, p := range q.Placements {
		if p.Role == "FARP" {
			if at >= 0 {
				return nil, fmt.Errorf("multiple placed FARP descriptors")
			}
			at = i
		}
	}
	if at < 0 {
		return nil, nil
	}
	p := q.Placements[at]
	if p.Formation < 0 || p.Formation >= len(w.FMHD)/39 || binary.LittleEndian.Uint32(w.FMHD[p.Formation*39+35:])&0x40 == 0 {
		return nil, fmt.Errorf("placed FARP has no source flag40 descriptor")
	}
	ti, err := authoredTemplate(w, p.Component)
	if err != nil || ti != p.Template {
		return nil, fmt.Errorf("placed FARP template/component identity changed")
	}
	if _, err = authoredPlacementComponents(w, p); err != nil {
		return nil, err
	}
	if err = RemoveAuthored(q, p.ID); err != nil {
		return nil, err
	}
	if _, err = AuthoredBudget(data, q); err != nil {
		return nil, err
	}
	*d = *q
	return &p, nil
}
