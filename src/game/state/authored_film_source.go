package state

import (
	"errors"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	"reflect"
)

var ErrAuthoredSourceReadOnly = errors.New("retained Support process memory has no independent editable inverse; its complete launch can be retained read-only")

// ValidateAuthoredFilmSource proves that an optional native authoring carrier
// actually produced this immutable launch. A label, ending snapshot or guessed
// inverse placement is insufficient. It neither draws RNG nor mutates owners.
func ValidateAuthoredFilmSource(data *assetdata.MissionGenerationData, source *AuthoredDocument, m *MissionData) error {
	if source == nil || source.Initial != nil || source.Compiled == nil || m == nil || m.Inputs.MissionType != MissionSimulated || m.SourcePreparation != nil {
		return fmt.Errorf("editable Export needs a complete native authored launch and its original document")
	}
	if source.Compiler != AuthoredCompiler && source.Compiler != AuthoredCompilerSupport && source.Compiler != AuthoredCompilerP5 && source.Compiler != AuthoredCompilerSupportP5 {
		return fmt.Errorf("editable Export source has an unsupported authored interpretation")
	}
	q, err := CloneAuthored(source)
	if err != nil {
		return err
	}
	expected, err := CompileAuthoredMission(data, q, m.Inputs)
	if err != nil {
		return fmt.Errorf("editable Export source: %w", err)
	}
	if expected.SoloBuilder != m.SoloBuilder || expected.FlightTemplate != m.FlightTemplate || source.World != m.World || !reflect.DeepEqual(expected.Objects, m.Objects) || !reflect.DeepEqual(expected.Objectives, m.Objectives) || !reflect.DeepEqual(expected.Targets, m.Targets) || !reflect.DeepEqual(expected.Paths, m.Paths) || !reflect.DeepEqual(expected.TargetCountdown, m.TargetCountdown) || expected.Conditions != m.Conditions || expected.RawE278 != m.RawE278 || expected.Scene != m.Scene || !reflect.DeepEqual(expected.Support, m.Support) {
		return fmt.Errorf("editable Export source differs from the recorded initial mission")
	}
	return nil
}

// NewAuthoredFromFilmSource copies a proven native descriptor graph into a new
// independent document. Its stable placement IDs remain internal references;
// its document ID and saved revision are new. No compilation or Fill is repeated.
func NewAuthoredFromFilmSource(data *assetdata.MissionGenerationData, source *AuthoredDocument, m *MissionData, title string) (*AuthoredDocument, error) {
	if err := ValidateAuthoredFilmSource(data, source, m); err != nil {
		return nil, err
	}
	if source.SupportMemory != nil {
		return nil, ErrAuthoredSourceReadOnly
	}
	if err := ValidateAuthoredTitle(title); err != nil {
		return nil, err
	}
	q, err := CloneAuthored(source)
	if err != nil {
		return nil, err
	}
	q.ID, err = authoredID()
	if err != nil {
		return nil, err
	}
	q.Title = title
	q.Compiled.Revision = AuthoredRevision(q)
	return q, nil
}
