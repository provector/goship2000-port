package state

import (
	"fmt"
	assetdata "gunship2000-assets/game/assets"
)

const OwnedSupportChoiceV1 = "owned-support-choice-v1"

type AuthoredSupportEdits struct {
	Profile             string `json:"profile"`
	PreviousRevision    string `json:"previous_revision,omitempty"`
	RetiredReservations string `json:"retired_reservations,omitempty"`
}

// NewNativeAuthoredDocument is the current UI authoring policy. The lower-level
// source constructor remains available for historical document interpretation.
func NewNativeAuthoredDocument(data *assetdata.MissionGenerationData, world, title string, seed uint32) (*AuthoredDocument, error) {
	if data == nil || data.NativeModels == nil {
		return nil, fmt.Errorf("native authoring requires verified prepared model resources")
	}
	d, e := NewAuthoredDocument(data, world, title, seed)
	if e != nil {
		return nil, e
	}
	d.SupportEdits = &AuthoredSupportEdits{Profile: OwnedSupportChoiceV1}
	d.ResourcePolicy = NativeModelResourcesV1
	return d, nil
}

// Only the choice owner participates in popup conflict detection. An unrelated
// ordinary edit must neither be rolled back nor prevent this owned merge.
func AuthoredSupportEditFingerprint(d *AuthoredDocument) string {
	if d == nil {
		return ""
	}
	return authoredDigest(struct {
		ID, World, Dependency string
		Support               *AuthoredSupport
		Memory                *AuthoredSupportMemory
		Edits                 *AuthoredSupportEdits
	}{d.ID, d.World, d.Dependency, d.Support, d.SupportMemory, d.SupportEdits})
}

func CommitAuthoredSupportChoice(data *assetdata.MissionGenerationData, d *AuthoredDocument, o AuthoredOption, expected string) error {
	if d == nil || d.Initial != nil {
		return fmt.Errorf("Support edit requires a complete editable descriptor graph")
	}
	if expected != "" && expected != AuthoredSupportEditFingerprint(d) {
		return fmt.Errorf("Support choice changed while picker was open; accepted mission preserved")
	}
	if e := validateAuthoredShape(d); e != nil {
		return e
	}
	if e := validateAuthoredSupportMemory(data, d); e != nil {
		return e
	}
	q, e := CloneAuthored(d)
	if e != nil {
		return e
	}
	if q.SupportEdits == nil {
		q.SupportEdits = &AuthoredSupportEdits{Profile: OwnedSupportChoiceV1, PreviousRevision: AuthoredRevision(d), RetiredReservations: authoredDigest(d.SupportMemory)}
	}
	q.SupportMemory = nil
	if data.NativeModels != nil {
		q.ResourcePolicy = NativeModelResourcesV1
	}
	if e = setAuthoredSupportSource(data, q, o); e != nil {
		return e
	}
	if q.Generated != nil && q.Generated.Support != nil && (q.Support == nil || *q.Support != q.Generated.Support.Selection) {
		for i, p := range q.Placements {
			if p.ID == q.Generated.SupportOwner {
				q.Placements = append(q.Placements[:i], q.Placements[i+1:]...)
				break
			}
		}
	}
	if d.SupportEdits != nil && authoredDigest(q.Support) == authoredDigest(d.Support) {
		return nil
	}
	if e = validateAuthoredShape(q); e != nil {
		return e
	}
	*d = *q
	return nil
}

func SetAuthoredSupport(data *assetdata.MissionGenerationData, d *AuthoredDocument, o AuthoredOption) error {
	if d != nil && d.SupportEdits != nil {
		return CommitAuthoredSupportChoice(data, d, o, "")
	}
	return setAuthoredSupportSource(data, d, o)
}
