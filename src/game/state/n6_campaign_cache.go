package state

import (
	"fmt"
	assetdata "gunship2000-assets/game/assets"
)

type CampaignCachePresence string

const (
	CampaignCachePresent CampaignCachePresence = "present"
	CampaignCacheAbsent  CampaignCachePresence = "absent"
	CampaignCacheUnknown CampaignCachePresence = "unknown"
)

// Version2 records knowledge of the source filename ordinal, not an invented
// creator world. An omitted import entry is unknown, never evidence of absence.
type CampaignCacheKnowledge struct {
	Ordinal  int                   `json:"ordinal"`
	Presence CampaignCachePresence `json:"presence"`
	Evidence string                `json:"evidence"`
}

func freshCampaignCacheInventory() []CampaignCacheKnowledge {
	var out []CampaignCacheKnowledge
	for i := 0; i < 10; i++ {
		out = append(out, CampaignCacheKnowledge{i, CampaignCacheAbsent, "native fresh isolated cache namespace"})
	}
	return out
}

func (s *CampaignState) setCacheKnowledge(ordinal int, presence CampaignCachePresence, evidence string) {
	for i := range s.CacheInventory {
		if s.CacheInventory[i].Ordinal == ordinal {
			s.CacheInventory[i] = CampaignCacheKnowledge{ordinal, presence, evidence}
			return
		}
	}
	s.CacheInventory = append(s.CacheInventory, CampaignCacheKnowledge{ordinal, presence, evidence})
}

func (s *CampaignState) validateCacheInventory() error {
	if s.Version == 1 {
		if len(s.CacheInventory) != 0 {
			return fmt.Errorf("historical Campaign version1 cannot carry version2 cache knowledge")
		}
		return nil
	}
	seen := map[int]CampaignCachePresence{}
	if len(s.CacheInventory) > 256 {
		return fmt.Errorf("Campaign cache inventory exceeds byte ordinal domain")
	}
	for _, entry := range s.CacheInventory {
		if entry.Ordinal < 0 || entry.Ordinal > 255 || entry.Evidence == "" || len(entry.Evidence) > 256 || seen[entry.Ordinal] != "" {
			return fmt.Errorf("Campaign cache knowledge has invalid/duplicate ordinal or provenance")
		}
		if entry.Presence != CampaignCachePresent && entry.Presence != CampaignCacheAbsent && entry.Presence != CampaignCacheUnknown {
			return fmt.Errorf("Campaign cache presence %q is invalid", entry.Presence)
		}
		seen[entry.Ordinal] = entry.Presence
	}
	cacheOrdinals := map[int]bool{}
	for _, cache := range s.Caches {
		if cacheOrdinals[cache.Ordinal] {
			return fmt.Errorf("Campaign has multiple current read contexts for ordinal%d", cache.Ordinal)
		}
		cacheOrdinals[cache.Ordinal] = true
		if presence := seen[cache.Ordinal]; presence != "" && presence != CampaignCachePresent {
			return fmt.Errorf("Campaign cache payload contradicts inventory ordinal%d", cache.Ordinal)
		}
	}
	for ordinal, presence := range seen {
		if presence == CampaignCachePresent && !cacheOrdinals[ordinal] {
			return fmt.Errorf("Campaign known-present ordinal%d lacks supplied bytes", ordinal)
		}
	}
	return nil
}

func upgradeCampaignKnowledge(s *CampaignState) {
	if s.Version != 1 {
		return
	}
	s.Version = campaignSchemaVersion
	if s.ImportSHA256 == "" {
		s.CacheInventory = freshCampaignCacheInventory()
	}
	for _, cache := range s.Caches {
		s.setCacheKnowledge(cache.Ordinal, CampaignCachePresent, "retained exact cache payload")
	}
}

func checkCampaignCacheRead(in *CampaignGenerationInputs, w *assetdata.MissionWorld) error {
	ordinal := int(w.SNHD[0])
	for _, cache := range in.Caches {
		if cache.Ordinal == ordinal {
			if cache.Scenario != w.Scenario || cache.World != w.WorldName {
				return fmt.Errorf("Campaign ordinal%d has bytes bound to a different read context", ordinal)
			}
			if in.GenerationPolicy == CampaignGenerationPolicyPairedV1 {
				if err := cache.Validate(); err != nil {
					return err
				}
			}
			return nil
		}
	}
	if in.CacheKnowledgeVersion < 2 {
		if in.ScenarioCacheAbsent {
			return nil
		}
	} else {
		for _, entry := range in.CacheInventory {
			if entry.Ordinal == ordinal && entry.Presence == CampaignCacheAbsent {
				return nil
			}
		}
	}
	return fmt.Errorf("Campaign ordinal%d cache presence is unknown; next preparation was not published", ordinal)
}

// CampaignCacheReadContext binds exact supplied bytes to the actual consumer.
// It deliberately says nothing about which world originally created the file.
type CampaignCacheReadContext struct {
	Theatre         Theatre
	Scenario, World string
	Ordinal         int
}

func BindCampaignIntermediate(s *CampaignState, a *assetdata.MissionAssets, context CampaignCacheReadContext, raw []byte) (*CampaignState, error) {
	draft, err := CloneCampaign(s)
	if err != nil || draft == nil {
		return nil, fmt.Errorf("Campaign cache association needs a valid owner: %v", err)
	}
	if a == nil || a.Generation == nil || context.Theatre != s.Theatre {
		return nil, fmt.Errorf("Campaign cache read-context theatre/assets disagree")
	}
	matched := false
	for _, w := range a.Generation.Worlds {
		if w.Theatre == context.Theatre.Raw() && w.Scenario == context.Scenario && w.WorldName == context.World && int(w.SNHD[0]) == context.Ordinal {
			matched = true
		}
	}
	if !matched {
		return nil, fmt.Errorf("Campaign cache read context is not an exact prepared source world")
	}
	cache, err := decodeCampaignIntermediate(context, raw)
	if err != nil {
		return nil, err
	}
	upgradeCampaignKnowledge(draft)
	for _, old := range draft.Caches {
		if old.Ordinal == context.Ordinal {
			return nil, fmt.Errorf("Campaign ordinal%d is already bound; replacement needs an explicit changed-input transaction", context.Ordinal)
		}
	}
	draft.Caches = append(draft.Caches, cache)
	draft.setCacheKnowledge(context.Ordinal, CampaignCachePresent, "explicit source read-context import; creator unknown")
	return draft, draft.Validate()
}

func decodeCampaignIntermediate(context CampaignCacheReadContext, raw []byte) (CampaignFormationCache, error) {
	if len(raw) < 2 {
		return CampaignFormationCache{}, fmt.Errorf("Campaign intermediate is truncated")
	}
	n := int(int8(raw[1]))
	if n < 0 || n > 80 || len(raw) != 2+33*n {
		return CampaignFormationCache{}, fmt.Errorf("Campaign intermediate framing is incomplete")
	}
	c := CampaignFormationCache{Scenario: context.Scenario, World: context.World, Ordinal: context.Ordinal, BeforeCount: int(raw[0]), Objects: make([]MissionObject, n), Provenance: append([]byte(nil), raw[2+32*n:]...)}
	for i := range c.Objects {
		copy(c.Objects[i].Raw[:], raw[2+32*i:2+32*(i+1)])
		c.Objects[i].Formation = int(c.Provenance[i])
	}
	return c, c.Validate()
}

// TrainingCacheEffect is the pure GS section2:215d existing-file rb+ effect.
// The caller owns the distinct retained Campaign and actual Training result.
// This API does not admit the still-unresolved full Training preparation epoch.
type TrainingCacheEffect struct {
	RetainedOwner *CampaignState
	SourceProfile string
	Stage         byte
	Mission       MissionType
	LaunchHigh    byte
	Ordinal       byte
	Presence      CampaignCachePresence
	Raw           []byte
	Credits       [40]byte
}

func ApplyTrainingCacheEffect(in TrainingCacheEffect) ([]byte, error) {
	if in.SourceProfile != assetdata.IslandIce469085.Name || in.Mission != MissionTraining || in.LaunchHigh&0xe0 != 0 || in.Stage > 2 || in.Ordinal != 0 {
		return nil, fmt.Errorf("Training cache effect has unsupported launch/stage/source ownership")
	}
	if in.Stage == 0 {
		return append([]byte(nil), in.Raw...), nil
	}
	if in.RetainedOwner == nil || in.RetainedOwner.Validate() != nil || in.RetainedOwner.SourceProfile != in.SourceProfile || (in.Stage == 1 && in.RetainedOwner.Phase != CampaignActive) || (in.Stage == 2 && in.RetainedOwner.Phase != CampaignSuspended) {
		return nil, fmt.Errorf("Training cache effect needs its distinct valid retained Campaign owner snapshot")
	}
	if in.Presence == CampaignCacheAbsent {
		if len(in.Raw) != 0 {
			return nil, fmt.Errorf("absent Training cache has contradictory bytes")
		}
		return nil, nil
	}
	if in.Presence != CampaignCachePresent || len(in.Raw) < 2 {
		return nil, fmt.Errorf("Training cache presence/bytes are unknown")
	}
	k, n := int(int8(in.Raw[0])), int(int8(in.Raw[1]))
	if k < 0 || n < 0 || k+n > 80 || len(in.Raw) != 2+33*n {
		return nil, fmt.Errorf("Training cache signed credit interval/framing is unsupported")
	}
	out := append([]byte(nil), in.Raw...)
	for i := 0; i < n; i++ {
		if campaignCredit(in.Credits, k+i) == 0 {
			continue
		}
		r := out[2+32*i : 2+32*(i+1)]
		r[0] &= 0xc7
		r[10] = 0x1d
		r[12], r[13] = r[14], r[14]
		copy(r[15:27], "CMPGN WRECK\x00")
	}
	return out, nil
}
