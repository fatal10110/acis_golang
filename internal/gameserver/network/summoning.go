package network

import (
	"context"
	"time"

	petmodel "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/pet"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/petitem"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
)

// SummoningConfig holds the collaborators of the summon feature: the summon
// items, the pet rows and settings, the pets' dice and feeding pace, and the
// corpse decay.
type SummoningConfig struct {
	Items  *item.SummonItemTable
	Store  petStore
	Config petmodel.Config
	// Roll supplies the dice of every pet called out in [0,n), its hunger
	// included; nil falls back to the random source.
	Roll func(n int) int
	// FeedPeriod is how often every pet called out eats; zero is the
	// standard ten seconds. Harnesses lengthen it so a meal cannot land
	// inside a timed exchange.
	FeedPeriod time.Duration
	// Decay removes a dead summon's corpse once its decay delay has passed.
	Decay *task.Decay
}

// summoning is the summon feature: the summon items and pet rows, the pet
// settings, dice and feeding pace every pet called out shares, the pet saves
// still behind their lane, the pet inventory transfers and the corpse decay.
// NewGameClientLink builds it whole and nothing in it is replaced
// afterwards, so every client goroutine reads it without a lock; queued
// guards itself.
type summoning struct {
	items  *item.SummonItemTable
	store  petStore
	config petmodel.Config // passed into summon.PetConfig by newPet.
	// roll is the random source of every pet called out; nil uses the
	// summon default.
	roll func(n int) int
	// feedPeriod is how often every pet called out eats; zero uses the
	// summon default.
	feedPeriod time.Duration
	queued     queuedPets
	petItems   *petitem.Service
	// decay removes a dead summon's corpse at its deadline; nil leaves
	// summon corpses in the world.
	decay *task.Decay
}

// petStore is the narrow persistence surface a pet-collar restore needs:
// the saved row for a collar's pet, if any (data/sql.PetStore.Get).
type petStore interface {
	Get(ctx context.Context, itemObjectID int32) (petmodel.State, bool, error)
	Save(ctx context.Context, itemObjectID int32, state petmodel.State) error
	SaveFed(ctx context.Context, itemObjectID int32, fed int) error
}

// newSummoning builds the summon feature from cfg's tables, store, pet
// settings and dice.
func newSummoning(cfg GameClientLinkConfig) summoning {
	return summoning{
		items:      cfg.Summoning.Items,
		store:      cfg.Summoning.Store,
		config:     cfg.Summoning.Config,
		roll:       cfg.Summoning.Roll,
		feedPeriod: cfg.Summoning.FeedPeriod,
		petItems:   petitem.NewService(cfg.IDs),
		decay:      cfg.Summoning.Decay,
	}
}

// newPet builds a pet with the shared effect, config and skill wiring. A live
// pet inventory must be constructed with
// itemcontainer.NewPetInventoryWithDelivery (delivery and persistence) before
// calling this method; NewPetInventory has neither.
func (l *GameClientLink) newPet(cfg summon.PetConfig) (*summon.Actor, error) {
	cfg.Effects = l.effects
	cfg.Config = &l.summoning.config
	cfg.Settings = l.playerRules
	if cfg.Roll == nil {
		cfg.Roll = l.summoning.roll
	}
	if cfg.FeedPeriod == 0 {
		cfg.FeedPeriod = l.summoning.feedPeriod
	}
	if cfg.SkillDefs == nil {
		cfg.SkillDefs = l.skills
	}
	pet, err := summon.NewPet(cfg)
	if err != nil {
		return nil, err
	}
	pet.SetRaidCursesDisabled(l.disableRaidCurse)
	return pet, nil
}
