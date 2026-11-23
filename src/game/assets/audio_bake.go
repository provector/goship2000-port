package assets

const InitializationStartBakePolicy = "gs2-init-0b-arg1-ramp21-reset0-v1"
const InitializationEarlyBakePolicy = "gs2-init-15-canonical-second-loop-v1"

// GS A1eaec selects these GS4 presentation-music commands. They are not GS2
// gameplay events and are gated by the original music preference domain.
var DebriefMusicEvents = []int{0x19, 0x1a, 0x1b, 0x1c, 0x1e, 0x1f, 0x20}

var AudioSourceRoles = []SourceRole{roleAudioLOG, roleAudioGS3, RoleAudioGS4, RoleAudioADigital, RoleAudioDDigital, RoleAudioTarget}
