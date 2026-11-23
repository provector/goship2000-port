package assets

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Source identities select interpretations backed by this release. Generic
// directory parsing and X0 decompression deliberately do not depend on them.
type SourceRole string

const (
	rolePictureCatalog       SourceRole = "picture catalog"
	roleScreenCatalog        SourceRole = "screen/theatre catalog"
	roleBuilderCatalog       SourceRole = "mission-builder catalog"
	RoleMainExecutable       SourceRole = "office code and initialized data"
	RoleSimulationExecutable SourceRole = "simulation renderer code and initialized data"
	RoleModelGeometry        SourceRole = "model geometry catalogue"
	RoleModelTypes           SourceRole = "model type labels"
	roleBitmapFonts          SourceRole = "bitmap font bank"
	roleLauncher             SourceRole = "normal COM launcher"
	roleCDLauncher           SourceRole = "CD COM launcher"
	roleSetupLauncher        SourceRole = "setup COM launcher"
	roleOfficeScreen         SourceRole = "BASE screen definition payload"
	roleOfficePalette        SourceRole = "office palette payload"
	roleStartupLogo          SourceRole = "MicroProse logo animation"
	roleStartupIntro         SourceRole = "intro and credits animation"
	RoleLogoPlayer           SourceRole = "logo player code"
	RoleIntroPlayer          SourceRole = "intro player code"
	roleAudioLOG             SourceRole = "AdLib logo driver/data"
	roleAudioGS3             SourceRole = "AdLib title driver/data"
	RoleAudioGS4             SourceRole = "AdLib front-end driver/data"
	RoleAudioADigital        SourceRole = "AdLib/SB sample descriptor driver"
	RoleAudioDDigital        SourceRole = "alternate digital descriptor driver"
	RoleAudioTarget          SourceRole = "digital source sample bank"
)

type KnownSourceIdentity struct {
	Name   string
	Role   SourceRole
	SHA256 string
}

type KnownPictureIdentity struct {
	Name              string
	PixelSHA256       string
	FullBankValidated bool
}

type OfficeSourceLayout struct {
	DataFileAnchor int
	DataFileEnd    int
}

type MissionSourceLayout struct {
	LeaderDS                                                                           int
	CampaignStatusPointersDS                                                           int
	VisibilityDS                                                                       [4]int
	TemplateStrings                                                                    []int
	PoseDS, WindDS, TemperatureDS, ConsonantsDS, CallsignsDS, CompassDS, RankPointerDS int
	TemplatesDS, PrimaryDS, SecondaryDS, MasksDS, DisplayKindsDS, WindMultiplierDS     int
}

type PreflightSourceLayout struct {
	DifficultyDS, DifficultyBytes                     int
	StringsDS, StringsEndDS                           int
	SquadronNamesDS                                   int
	OutfitRankPointerDS, AwardPointerDS               int
	AdditionalStringsDS                               []int
	PointerInitializerFile, StationMarkerConsumerFile int
	OutfitHeaderDS                                    [2]int
	ObjectiveVerbsDS, TargetNameFormatDS              int
	ObjectiveAbbreviationDS                           [2]int
}

type AssetVersionProfile struct {
	Name                 string
	Sources              []KnownSourceIdentity
	Pictures             []KnownPictureIdentity
	OfficeLayout         OfficeSourceLayout
	MissionLayout        MissionSourceLayout
	PreflightLayout      PreflightSourceLayout
	AudioLayout          *AudioSourceLayout
	WorldLayout          *WorldSourceLayout
	CockpitLayout        *CockpitSourceLayout
	InitializationLayout *InitializationSourceLayout
	DebriefLayout        *DebriefSourceLayout
}

// This is the single production authority for known hashes. Adding an older
// edition means adding a profile and its supported layouts, not weakening gates.
var IslandIce469085 = AssetVersionProfile{
	Name: "Gunship 2000 469.085 Islands & Ice (supplied installation)",
	Sources: []KnownSourceIdentity{
		{"GS2.GS2", RoleSimulationExecutable, "1a1db96070adb8c23e1eb499b449cc147f9fb0fd7b47a65e88c4c5603a40e209"},
		{"OBJFILE", RoleModelGeometry, "b5e99bcddfb408bc66363864ab134f2b97e56d6723ac8fb157b71f76cd7364ff"},
		{"OBJTYPES.DAT", RoleModelTypes, "a85143130d48e80b24d8a96f99c6583385f7e85c481df30a772417754224e24a"},
		{"ASOUND.LOG", roleAudioLOG, "06db9ef02010a3e6850328f57633244b88ef18e527de8354287c2c66c5a05bba"},
		{"ASOUND.GS3", roleAudioGS3, "8d23008483b33c4141342ef2cd059b84785e0308792bf48c5bc809f19962b74d"},
		{"ASOUND.GS4", RoleAudioGS4, "d02c0b2ab09b6bda2babb181b1d3cb557c160092475514ff849524a7dcc2501e"},
		{"ASOUND.GS2", RoleAudioADigital, "ab3f9e2a0d083c73ec7bc0aa7a92c54e569d8176b4a1d5b1e5a6ce2e42554c71"},
		{"DSOUND.GS2", RoleAudioDDigital, "afe19b790677770ae7e39df8961ff9f02c9ead5ac0e5bc43e1b44b6109357ad4"},
		{"TARGET.BIN", RoleAudioTarget, "b7f19b9feff24babadd5d05bffe4da5e01b6c1249fbe246de7716a45c5c1a9c5"},
		{"GS2000.CAT", rolePictureCatalog, "d8704a146f2d002be170ae7fe905421679755516f969fd501bef097f7006ca92"},
		{"GS2000.DAT", roleScreenCatalog, "79b11a4654600742fa5a2ec8a9935e31c521e5603af232de80bf759d70d20a04"},
		{"MBUILDER.CAT", roleBuilderCatalog, "7d0f969734a38832bb6ad00cb9642b03b4f00634b0150e0a73074d024b761855"},
		{"GS.GS2", RoleMainExecutable, "46ce1a52b67e50ebee7d5aacafa063021bfd7e39303b5a67da61a5cd5aed90e9"},
		{"FONTS.GS2", roleBitmapFonts, "fab69f3e5325dc0e419f66840e30b73c8d1225c6184893a570608638a0c3164f"},
		{"GS2000.COM", roleLauncher, "df56479feaa71bf6522755118943fe181849facbd44e07508f7a658545099e53"},
		{"PACK2.CD", roleCDLauncher, "4f967df52fc548f85da3bc1bcbb6d20294679513f354beb1f890c8b8efbf1e24"},
		{"SETUP.EXE", roleSetupLauncher, "d2c826cabf1967373109cebb7f4b97f73dfaca6a29a2f8b4fdeb27f0531c60e3"},
		{"BASE.SCN", roleOfficeScreen, "9b2195142f5685ee95e94762bfacd3e3dc971e8c3dadbf9e53f0e55d0b67e102"},
		{"BASE.PAL", roleOfficePalette, "ed985dab3a363cc6301a6031863e08a7f251181407b7c6908038262ad8e7caab"},
		{"LABSLOGO.SS", roleStartupLogo, "af7f42ee44b2bdac702765b26ff20e6a61767cf90ef0219d247081e72de888eb"},
		{"GST.PAN", roleStartupIntro, "0b817bd4cbe8b1f83d0e7caad43889f806e24920c2adb4b75ea33a4aeae7a7d4"},
		{"LABS.GS2", RoleLogoPlayer, "2c4af14c1447f0ae4d04356de9f7c0b4691cc7cb5cc0431d11885cffd016b1bd"},
		{"PLAYER.GS2", RoleIntroPlayer, "7e299763e9efc5064e0038a45b5f727cb8b0e25b6cff4ef0149819b517d7b45a"},
	},
	Pictures: []KnownPictureIdentity{
		{"BASE.PIC", "06e44068ad15fb0108ca79515a04d5c2148c2565bf3a7b3b63726c6d251a7071", true},
		{"BASE_AN.PIC", "7c9becf5a94e301056b9a800b6cf5a7f925569ef1f0e166fd98d5ef7fad49a41", true},
		{"BASE_AN1.PIC", "024a09d382b42217f64edd7a2686f8b2db1c5256400d780b260f22e22f8d9654", false},
		{"BASE_AN2.PIC", "3d2c8bd7ecc297cbc141cbddd3335a756de97621697be46a666a279ad59db5e6", false},
		{"POINTER.PIC", "f14c1abfd38433a2f9712978b611144a259f0bc5134cae4b7e0ebd8ee258cf09", false},
	},
	AudioLayout:          &islandIceAudioLayout,
	WorldLayout:          &islandIceWorldLayout,
	InitializationLayout: &islandIceInitializationLayout,
	CockpitLayout:        &islandIceCockpitLayout,
	DebriefLayout:        &islandIceDebriefLayout,
	OfficeLayout:         OfficeSourceLayout{DataFileAnchor: 0x3d2f0, DataFileEnd: 0x4471e},
	PreflightLayout:      PreflightSourceLayout{DifficultyDS: 0x4cfc, DifficultyBytes: 66, StringsDS: 0x3ddd, StringsEndDS: 0x4112, SquadronNamesDS: 0x5aee, OutfitRankPointerDS: 0x1a90, AwardPointerDS: 0x1a8c, AdditionalStringsDS: []int{0x3d34, 0x3d3b, 0x3d57, 0x3d6b, 0x3d7b, 0x3d82}, PointerInitializerFile: 0xf53c, StationMarkerConsumerFile: 0x356b2, OutfitHeaderDS: [2]int{0x10ee, 0x10ff}, ObjectiveVerbsDS: 0x1b82, TargetNameFormatDS: 0x233, ObjectiveAbbreviationDS: [2]int{0x222, 0x22a}},
	MissionLayout: MissionSourceLayout{
		LeaderDS: 0x1ca, CampaignStatusPointersDS: 0x35ea, VisibilityDS: [4]int{0x3572, 0x3563, 0x356d, 0x355b},
		PoseDS: 0x3c28, WindDS: 0x28a, TemperatureDS: 0x2a6, ConsonantsDS: 0x3bf3, CallsignsDS: 0x980, CompassDS: 0xcde, RankPointerDS: 0x1a92,
		TemplatesDS: 0x11a0, PrimaryDS: 0x244, SecondaryDS: 0x260, MasksDS: 0x1ba6, DisplayKindsDS: 0x1b94, WindMultiplierDS: 0x723c,
		TemplateStrings: []int{0x1b3a, 0x1b4e, 0x1b62, 0x30c2, 0x30cb, 0x30d1, 0x30d7, 0x30e0, 0x30f2, 0x30f9, 0x3106, 0x3344, 0x335f, 0x3370, 0x3390, 0x3395, 0x33a8, 0x33b8, 0x33cb, 0x344f, 0x34a2, 0x34a9, 0x34bc, 0x34d1, 0x34db, 0x34eb, 0x34f5, 0x3504, 0x3509, 0x350f, 0x3513, 0x352f, 0x355b, 0x3563, 0x356d, 0x3572, 0x3577, 0x3591, 0x3597, 0x35b3, 0x35f8, 0x35fe, 0x3601, 0x3667, 0x368d, 0x3694, 0x369f, 0x36a6, 0x36cb, 0x36d0, 0x36d6, 0x36db, 0x36e1, 0x36eb, 0x372e, 0x3732, 0x3762, 0x3772, 0x3776, 0x37be, 0x3817, 0x3857, 0x385b, 0x388f, 0x3893, 0x38ba, 0x38be, 0x38e2, 0x38f8, 0x3970, 0x3b3f, 0x3b88, 0x3bb9, 0x3bc9, 0x3bda, 0x3bf0, 0x3bf3, 0x3c08, 0x3c0e, 0x3c14, 0x3c1b, 0x3c22, 0x3c54, 0x4cc9},
	},
}

func (p *AssetVersionProfile) Source(role SourceRole) KnownSourceIdentity {
	for _, s := range p.Sources {
		if s.Role == role {
			return s
		}
	}
	panic("missing source role in built-in asset profile: " + string(role))
}

func (p *AssetVersionProfile) SourceMatches(role SourceRole, sha string) bool {
	return sha != "" && p.Source(role).SHA256 == sha
}

func (p *AssetVersionProfile) pictureHashes(fullBank bool) map[string]string {
	result := make(map[string]string)
	for _, picture := range p.Pictures {
		if picture.FullBankValidated == fullBank {
			result[picture.Name] = picture.PixelSHA256
		}
	}
	return result
}

// Compatibility names are derived views; callers do not own separate identities.
var baseCatalogHash = IslandIce469085.Source(rolePictureCatalog).SHA256

var originalFontHash = IslandIce469085.Source(roleBitmapFonts).SHA256

var officeScreenHash = IslandIce469085.Source(roleOfficeScreen).SHA256

var officeCodeHash = IslandIce469085.Source(RoleMainExecutable).SHA256

var officePaletteHash = IslandIce469085.Source(roleOfficePalette).SHA256

var officeDataAnchor = IslandIce469085.OfficeLayout.DataFileAnchor

var baselinePictures = IslandIce469085.pictureHashes(true)

var officeAtlasPictures = IslandIce469085.pictureHashes(false)

var catalogHashes = func() map[string]string {
	result := make(map[string]string)
	for _, role := range []SourceRole{rolePictureCatalog, roleScreenCatalog, roleBuilderCatalog} {
		s := IslandIce469085.Source(role)
		result[s.Name] = s.SHA256
	}
	return result
}()

var headerlessExecutables = func() map[string]bool {
	result := make(map[string]bool)
	for _, role := range []SourceRole{roleLauncher, roleCDLauncher, roleSetupLauncher} {
		result[IslandIce469085.Source(role).SHA256] = true
	}
	return result
}()

type UnsupportedVersionOperation struct {
	Operation string `json:"operation"`
	Profile   string `json:"supported_profile"`
	Reason    string `json:"reason"`
}

// Each operation declares its complete source requirements. Matching BASE alone
// never authorizes executable offsets or gives a mixed installation provenance.
func recognizeOfficeProfile(resources []*Resource) (*AssetVersionProfile, error) {
	p := &IslandIce469085
	var reasons []string
	for _, role := range []SourceRole{rolePictureCatalog, roleScreenCatalog, RoleMainExecutable, roleBitmapFonts} {
		source := p.Source(role)
		found, matched, conflicting := false, false, false
		for _, r := range resources {
			if filepath.Base(r.Source.Path) != source.Name {
				continue
			}
			if role == rolePictureCatalog || role == roleScreenCatalog {
				if r.Source.Kind != "archive_entry" {
					continue
				}
			} else if r.Source.Kind != "loose_file" {
				continue
			}
			found = true
			if p.SourceMatches(role, r.Source.SHA256) {
				matched = true
			} else {
				conflicting = true
			}
		}
		if conflicting {
			reasons = append(reasons, source.Name+" has an unsupported or conflicting source hash")
			continue
		}
		if !matched {
			if found {
				reasons = append(reasons, source.Name+" source hash is not supported")
			} else {
				reasons = append(reasons, source.Name+" source is missing")
			}
		}
	}
	for _, role := range []SourceRole{roleOfficeScreen, roleOfficePalette} {
		s := p.Source(role)
		parentRole := roleScreenCatalog
		if role == roleOfficePalette {
			parentRole = rolePictureCatalog
		}
		matched := false
		for _, r := range resources {
			if r.name == s.Name && r.Source.Kind == "archive_entry" && p.SourceMatches(parentRole, r.Source.SHA256) && r.Source.PayloadSHA256 == s.SHA256 {
				matched = true
			}
		}
		if !matched {
			reasons = append(reasons, s.Name+" payload from its supported catalog is missing or changed")
		}
	}
	if len(reasons) != 0 {
		return nil, fmt.Errorf("%s", strings.Join(reasons, "; "))
	}
	return p, nil
}
