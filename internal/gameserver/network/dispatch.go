package network

import (
	"context"
	"crypto/rand"
	"fmt"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	datacache "github.com/fatal10110/acis_golang/internal/gameserver/data/cache"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/manager"
	enchantflow "github.com/fatal10110/acis_golang/internal/gameserver/enchant"
	handlerskill "github.com/fatal10110/acis_golang/internal/gameserver/handler/skill"
	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	invops "github.com/fatal10110/acis_golang/internal/gameserver/inventory"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	petmodel "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/pet"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/entity"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/grounditem"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/restart"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/shortcut"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/petitem"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	tradebook "github.com/fatal10110/acis_golang/internal/gameserver/trade"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

type itemStore interface {
	ListByOwner(ctx context.Context, ownerID int32) ([]*item.Instance, error)
	Save(ctx context.Context, inst *item.Instance) error
	Update(ctx context.Context, inst *item.Instance) error
	Delete(ctx context.Context, objectID int32) error
}

type shortcutStore interface {
	ListByOwner(ctx context.Context, ownerID int32) ([]shortcut.Shortcut, error)
	Save(ctx context.Context, ownerID int32, sc shortcut.Shortcut) error
	Delete(ctx context.Context, ownerID int32, slot, page int32) error
}

type attackStanceTracker interface {
	Add(task.AttackStanceActor)
}

type idAllocator interface {
	NextID() (int32, error)
}

type groundItemDropper interface {
	Drop(*grounditem.Item, task.DropOptions)
	Remove(*grounditem.Item)
}

const (
	crystallizeSkillID              = 248
	dropInteractionDistance         = 150
	groundPickupInteractionDistance = 150
)

// PlayerConfig bundles the primitive players.properties-derived gameplay
// flags GameClientLink needs, so its constructor doesn't grow one
// bool/float parameter per config key.
type PlayerConfig struct {
	// RespawnRestoreHP is the fraction of calculated max HP a non-percent
	// revive restores.
	RespawnRestoreHP float64
	// SkillEnchantSPBookNeeded controls whether enchanting a skill above
	// level 76 also consumes the tree's configured spellbook item.
	SkillEnchantSPBookNeeded bool
	// KarmaPlayerCanTeleport controls whether a karma-carrying player may
	// use a TELEPORT/RECALL-type skill, direct or item-attached.
	KarmaPlayerCanTeleport bool
}

// GameClientLink accepts and drives connections from Interlude game
// clients: the VersionCheck/cipher handshake, session-key validation
// against the login server, character list/create/delete/restore, and
// character select through to world entry.
type GameClientLink struct {
	validator     *SessionValidator
	loginLink     func() *LoginLink
	roster        *manager.Roster
	items         itemStore
	shortcuts     shortcutStore
	templates     *player.TemplateTable
	itemTemplates *item.Table
	html          *datacache.HTML
	crests        *datacache.Crests
	skills        *skillstate.Persistence
	spellbooks    modelskill.BookPolicy
	skillTrees    *modelskill.Trees
	cursedWeapons *entity.CursedWeaponTable
	world         *world.State
	npcs          *npc.Table
	geo           move.Geo
	zones         *zone.Index
	ids           idAllocator
	groundItems   groundItemDropper
	attackStance  attackStanceTracker
	positions     *task.PositionUpdates
	playerClock   *task.PlayerClock
	// inventoryUpdates batches InventoryUpdate packets for inventory
	// changes the server makes on its own, outside a client request.
	inventoryUpdates *task.InventoryUpdates
	restarts         *restart.Table
	levels           *player.LevelTable
	playerConfig     PlayerConfig
	petConfig        petmodel.Config // passed into summon.PetConfig by newPet.
	inventory        *invops.Service
	petItems         *petitem.Service
	trades           *tradebook.Book
	enchantState     *enchantflow.State
	enchant          *enchantflow.Service
	targets          *skilltarget.Registry
	skillHandlers    *handlerskill.Registry
	log              zerolog.Logger

	// newCipherKey supplies each connection's XOR cipher key; overridden in
	// tests for a deterministic handshake.
	newCipherKey func() ([]byte, error)

	// enchantRoll supplies enchant dice rolls; overridden in tests.
	enchantRoll func() float64

	// skillEnchantRoll supplies skill-enchant dice rolls in [0,99];
	// overridden in tests for a deterministic outcome.
	skillEnchantRoll func() int

	// afterFunc schedules fn to run once after d; nil defaults to
	// time.AfterFunc. Overridden in tests for deterministic timing.
	afterFunc func(d time.Duration, fn func())
}

// GameClientLinkConfig contains the collaborators required by GameClientLink.
type GameClientLinkConfig struct {
	Validator     *SessionValidator
	LoginLink     func() *LoginLink
	Roster        *manager.Roster
	Items         itemStore
	Shortcuts     shortcutStore
	Templates     *player.TemplateTable
	ItemTemplates *item.Table
	HTML          *datacache.HTML
	Crests        *datacache.Crests
	Skills        *skillstate.Persistence
	Spellbooks    modelskill.BookPolicy
	SkillTrees    *modelskill.Trees
	CursedWeapons *entity.CursedWeaponTable
	World         *world.State
	NPCs          *npc.Table
	Geo           move.Geo
	Zones         *zone.Index
	IDs           idAllocator
	GroundItems   groundItemDropper
	AttackStance  attackStanceTracker
	Positions     *task.PositionUpdates
	PlayerClock   *task.PlayerClock
	// InventoryUpdates batches InventoryUpdate packets for inventory
	// changes the server makes on its own, outside a client request.
	InventoryUpdates *task.InventoryUpdates
	Restarts         *restart.Table
	Levels           *player.LevelTable
	PlayerConfig     PlayerConfig
	PetConfig        petmodel.Config
	Log              zerolog.Logger
}

// NewGameClientLink builds a GameClientLink from its collaborators.
// loginLink returns the game server's current link to the login server, or
// nil while disconnected/reconnecting: session validation fails clients
// gracefully (AuthLoginFail) rather than panicking while the link is down.
func NewGameClientLink(cfg GameClientLinkConfig) *GameClientLink {
	return &GameClientLink{
		validator:     cfg.Validator,
		loginLink:     cfg.LoginLink,
		roster:        cfg.Roster,
		items:         cfg.Items,
		shortcuts:     cfg.Shortcuts,
		templates:     cfg.Templates,
		itemTemplates: cfg.ItemTemplates,
		html:          cfg.HTML,
		crests:        cfg.Crests,
		skills:        cfg.Skills,
		spellbooks:    cfg.Spellbooks,
		skillTrees:    cfg.SkillTrees,
		cursedWeapons: cfg.CursedWeapons,
		world:         cfg.World,
		npcs:          cfg.NPCs,
		geo:           cfg.Geo,
		zones:         cfg.Zones,
		ids:           cfg.IDs,
		groundItems:   cfg.GroundItems,
		attackStance:  cfg.AttackStance,
		positions:     cfg.Positions,
		playerClock:   cfg.PlayerClock,

		inventoryUpdates: cfg.InventoryUpdates,
		restarts:         cfg.Restarts,
		levels:           cfg.Levels,
		playerConfig:     cfg.PlayerConfig,
		petConfig:        cfg.PetConfig,
		inventory:        invops.NewService(cfg.IDs),
		petItems:         petitem.NewService(cfg.IDs),
		trades:           tradebook.NewBook(time.Now),
		enchantState:     enchantflow.NewState(),
		targets:          skilltarget.NewRegistry(skilltarget.WorldKnown{State: cfg.World}),
		skillHandlers: handlerskill.NewDefaultRegistryWithSignet(cfg.Skills, handlerskill.SignetDeps{
			Templates: cfg.NPCs,
			IDs:       cfg.IDs,
			World:     cfg.World,
		}),
		log:          cfg.Log,
		newCipherKey: randomCipherKey,
	}
}

// newPet builds a pet and, if its owner is a connected client, registers
// its inventory with the batching task once here rather than on every
// lookup — the structural attach point activePet/registerPetInventoryUpdates
// expect.
func (l *GameClientLink) newPet(cfg summon.PetConfig) *summon.Actor {
	cfg.Config = &l.petConfig
	pet := summon.NewPet(cfg)
	if live, ok := cfg.Owner.(*livePlayer); ok {
		l.registerPetInventoryUpdates(pet, live)
	}
	return pet
}

func (l *GameClientLink) rollEnchantSkill() int {
	if l.skillEnchantRoll != nil {
		return l.skillEnchantRoll()
	}
	return rnd.Get(100)
}

// scheduleAfter runs fn once, after d elapses, on its own goroutine outside
// the connection's read loop (and its accept-loop recover). fn is wrapped so
// a panic there is recovered and logged instead of taking down the process.
func (l *GameClientLink) scheduleAfter(d time.Duration, fn func()) {
	wrapped := func() {
		defer func() {
			if r := recover(); r != nil {
				l.log.Error().Interface("panic", r).Msg("scheduled callback panic")
			}
		}()
		fn()
	}
	if l.afterFunc != nil {
		l.afterFunc(d, wrapped)
		return
	}
	time.AfterFunc(d, wrapped)
}

func randomCipherKey() ([]byte, error) {
	key := make([]byte, keySize)
	if _, err := rand.Read(key[:8]); err != nil {
		return nil, fmt.Errorf("generate game cipher key: %w", err)
	}
	copy(key[8:], gameCipherStaticKey[:])
	return key, nil
}

func validProtocolRevision(revision int32) bool {
	switch revision {
	case 737, 740, 744, 746:
		return true
	default:
		return false
	}
}
