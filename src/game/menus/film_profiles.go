package menus

import (
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/game/replay"
)

// Exact verified resource-profile pairs. Preparation25 only adds source text
// metadata; no arbitrary manifest/hash bypass is permitted for historical films.
const legacyOResources = "b1cc3f3185ce46ee6798eadd293cbcf7448cfaf0e202f707a62d76e4aef5088a"
const compatiblePreparation25Resources = "bd62e0f4d916662f0cf1f9aea060ad4179e80ebf1a4585cfd459b2043dbc7771" // Bound after reproducible preparation/media comparison.
const compatiblePreparation26Resources = "7b875d78e1671bddd7e526c6647e2c3f5e1cdfec14918de1ded771db510f60ea" // Exact source/media comparison: evidence/stepP/startup/preparation26-final-identities.json.
func filmResourceCompatible(f *replay.Film, current string) bool {
	// Old Resources strings remain immutable. Only complete, independently
	// validated semantic bundles reach this function; no path-derived hashes
	// are added for individual source folders.
	if f == nil {
		return false
	}
	// Preparation34 adds only source threat sound06. A complete prior33
	// projection is verified; historical controllers do not request06.
	if current == assetdata.LogicalPreparation34 {
		if f.Header.Resources == current {
			return true
		}
		current = assetdata.LogicalPreparation33
	}
	// Preparation33 changes one identified 2D map atlas only. The complete
	// Preparation32 projection is independently verified; simulation carriers,
	// source world geometry and historical checkpoints are unchanged.
	if current == assetdata.LogicalPreparation33 {
		if f.Header.Resources == current {
			return true
		}
		current = assetdata.LogicalPreparation32
	}
	if f.StationaryEnemyMechanics() {
		return current == assetdata.LogicalPreparation32 && (f.Header.Resources == assetdata.LogicalPreparation32 || f.Header.Resources == assetdata.LogicalPreparation32Candidate) || current == assetdata.LogicalPreparation32Candidate && f.Header.Resources == current
	}
	// Preparation32's complete verified prior projection is Preparation31;
	// ENEM is absent from all historical simulation interpretations.
	if current == assetdata.LogicalPreparation32 || current == assetdata.LogicalPreparation32Candidate {
		if f.Header.Resources == current || f.Header.Resources == assetdata.LogicalPreparation32Candidate {
			return true
		}
		current = assetdata.LogicalPreparation31
	}
	// The entire Preparation31 inventory is validated before this mapping.
	// Added release feedback remains launch-gated for historical films.
	if current == assetdata.LogicalPreparation31 {
		if f.Header.Resources == current {
			return true
		}
		current = assetdata.LogicalPreparation30
	}
	// Exact complete-content validation precedes this UI-only mapping. The
	// original Support captions cannot change old mechanics/checkpoints.
	if current == assetdata.LogicalPreparation30 {
		if f.Header.Resources == current {
			return true
		}
		current = assetdata.LogicalPreparation29
	}
	// Preparation29's complete verified profile retains every prior media byte
	// and semantic field. New presentation/feedback leaves are selected only by
	// their launch interpretation; historical checkpoints never use them.
	if current == assetdata.LogicalPreparation29 {
		if f.Header.Resources == current || f.Header.Resources == assetdata.LogicalPreparation29Candidate {
			return true
		}
		current = assetdata.LogicalPreparation28
	}
	if current == assetdata.LogicalPreparation29Candidate {
		if f.Header.Resources == current {
			return true
		}
		current = assetdata.LogicalPreparation28
	}
	// Preparation28 adds REPLACE/editor UI metadata only; the full retained
	// inventory/media comparison is bound in evidence/stepP3/preparation-final-comparison.json.
	if current == assetdata.LogicalPreparation28 || current == assetdata.LogicalPreparation28Candidate {
		if f.Header.Resources == current || f.Header.Resources == assetdata.LogicalPreparation28Candidate {
			return true
		}
		current = assetdata.LogicalPreparation27
	}
	// Preparation27 adds editor-only typed tables/help. Complete-source media
	// comparison is required before this explicit historical mapping is admitted.
	if current == assetdata.LogicalPreparation27 {
		if f.Header.Resources == current {
			return true
		}
		current = assetdata.LogicalPreparation26
	}
	if current != assetdata.LogicalPreparation24 && current != assetdata.LogicalPreparation25 && current != assetdata.LogicalPreparation26 {
		return false
	}
	if f.Header.Resources == current {
		return true
	}
	switch current {
	case assetdata.LogicalPreparation26:
		if f.Header.Resources == compatiblePreparation26Resources {
			return true
		}
		if f.Header.Resources == compatiblePreparation25Resources && f.Header.Simulation == replay.Compatibility {
			return true
		}
		return f.Legacy() && f.Header.Resources == legacyOResources
	case assetdata.LogicalPreparation25:
		return f.Header.Resources == compatiblePreparation25Resources && f.Header.Simulation == replay.Compatibility || f.Legacy() && f.Header.Resources == legacyOResources
	case assetdata.LogicalPreparation24:
		return f.Legacy() && f.Header.Resources == legacyOResources
	}
	return false
}

func filmPresentationReady(a *assetdata.OfficeAssets) bool {
	if a == nil || a.Replay == nil {
		return false
	}
	for key, want := range map[string]string{"mission_score": "MISSION SCORE: ", "time_taken": "TIME TAKEN: ", "total_kills": "TOTAL KILLS: ", "demo_title": "GUNSHIP 2000: DEMO MODE", "demo_copyright": "Copyright 1991 MicroProse Software Inc."} {
		if a.Replay.Text[key] != want {
			return false
		}
	}
	return true
}
