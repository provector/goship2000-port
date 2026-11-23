package assets

// Bump preparationVersion when required generated front-end outputs change.
// Zero is the legacy Step21 layout, accepted after the same full loader checks.
const (
	PreparedManifestSchema = 1
	officeMetadataSchema   = 1
	AudioMetadataSchema    = 9
	audioMetadataSchema    = AudioMetadataSchema
	PreparationVersion     = 35
	PreparationGenerator   = "gunship2000-port/assets"
)

func compatiblePreparation(m Manifest) bool {
	return m.SchemaVersion == PreparedManifestSchema && m.PreparationVersion >= 0 && m.PreparationVersion <= PreparationVersion
}
