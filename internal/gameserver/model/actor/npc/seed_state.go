package npc

import (
	"sync"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/manor"
)

// SeedState is one hostile life's manor seed lifecycle. Sowers and
// harvesters act from their own queues while the killer's queue reads it for
// rewards, so every method takes mu.
type SeedState struct {
	mu        sync.Mutex
	sowerID   int32
	seed      manor.Seed
	harvested bool

	// ownerLevel and cropBase are the owner's template level and its
	// strong-type crop multiplier; both are fixed for the owner's life.
	ownerLevel int
	cropBase   int
}

// HarvestClaim is the outcome of one harvest attempt on a seed state.
type HarvestClaim uint8

const (
	// HarvestClaimed means the caller took the crop.
	HarvestClaimed HarvestClaim = iota
	// HarvestNotSown means this life was never sown.
	HarvestNotSown
	// HarvestAlreadyHarvested means another harvest took the crop first.
	HarvestAlreadyHarvested
	// HarvestNotAuthorized means the caller is neither the sower nor in the
	// sower's party.
	HarvestNotAuthorized
)

// Strong-type passive skills: a monster carrying one yields
// (id - strongCropBaseOffset) crops per harvest instead of one.
const (
	strongCropFirstSkill = 4303
	strongCropLastSkill  = 4310
	strongCropBaseOffset = 4301
)

// initSeedState records the owner data a harvest's crop count reads.
func (h *Hostile) initSeedState() {
	tmpl := h.Instance.Template
	h.seed.ownerLevel = tmpl.Level
	h.seed.cropBase = 1
	for _, p := range tmpl.Passives {
		if id := int(p.ID); id >= strongCropFirstSkill && id <= strongCropLastSkill {
			h.seed.cropBase = id - strongCropBaseOffset
			break
		}
	}
}

// Seedable reports whether this NPC's template takes manor seeds.
func (h *Hostile) Seedable() bool { return h.Instance.Template.Seedable }

// SpawnLocation is where this life was spawned, which decides its manor
// area wherever it stands now; false for an NPC spawned with no home.
func (h *Hostile) SpawnLocation() (location.Location, bool) {
	return h.Instance.Home, h.Instance.HasHome
}

// Seeded reports whether this hostile was sown during its current life.
func (s *SeedState) Seeded() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sowerID != 0
}

// Sow records the sower and seed carried by this hostile unless this life
// was already sown. It reports whether sowerID's seed took.
func (s *SeedState) Sow(sowerID int32, seed manor.Seed) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sowerID != 0 {
		return false
	}
	s.sowerID = sowerID
	s.seed = seed
	s.harvested = false
	return true
}

// ClaimHarvest marks the crop consumed for playerID when this life is sown,
// not yet harvested, and playerID is the sower or inSowerParty reports it
// in the sower's party. At most one caller per life is answered
// HarvestClaimed; every other answer leaves the state unchanged.
//
// inSowerParty runs under the state's lock and must not reach back into it.
func (s *SeedState) ClaimHarvest(playerID int32, inSowerParty func(sowerID int32) bool) HarvestClaim {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case s.sowerID == 0:
		return HarvestNotSown
	case s.harvested:
		return HarvestAlreadyHarvested
	case s.sowerID != playerID && (inSowerParty == nil || !inSowerParty(s.sowerID)):
		return HarvestNotAuthorized
	}
	s.harvested = true
	return HarvestClaimed
}

// HarvestedCrop returns the crop of this life's seed and how many a harvest
// yields at rate: the owner's strong-type multiplier, plus one for each
// level the owner stands more than five above the seed, times rate.
func (s *SeedState) HarvestedCrop(rate int) (cropID int32, count int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sowerID == 0 {
		return 0, 0
	}
	count = max(s.cropBase, 1)
	if diff := s.ownerLevel - s.seed.Level - 5; diff > 0 {
		count += diff
	}
	return int32(s.seed.CropID), count * rate
}

// BlocksDrops reports whether this life's seed suppresses its ordinary item
// drops: any seed but an alternative one does.
func (s *SeedState) BlocksDrops() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sowerID != 0 && !s.seed.Alternative
}
