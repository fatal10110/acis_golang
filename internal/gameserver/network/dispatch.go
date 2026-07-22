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
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/entity"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/grounditem"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/restart"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/shortcut"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
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

// GameClientLink accepts and drives connections from Interlude game
// clients: the VersionCheck/cipher handshake, session-key validation
// against the login server, character list/create/delete/restore, and
// character select through to world entry.
type GameClientLink struct {
	validator                *SessionValidator
	loginLink                func() *LoginLink
	roster                   *manager.Roster
	items                    itemStore
	shortcuts                shortcutStore
	templates                *player.TemplateTable
	itemTemplates            *item.Table
	html                     *datacache.HTML
	crests                   *datacache.Crests
	skills                   *skillstate.Persistence
	spellbooks               modelskill.BookPolicy
	skillTrees               *modelskill.Trees
	cursedWeapons            *entity.CursedWeaponTable
	world                    *world.State
	geo                      move.Geo
	ids                      idAllocator
	groundItems              groundItemDropper
	attackStance             attackStanceTracker
	positions                *task.PositionUpdates
	playerClock              *task.PlayerClock
	restarts                 *restart.Table
	respawnRestoreHP         float64
	levels                   *player.LevelTable
	skillEnchantSPBookNeeded bool
	inventory                *invops.Service
	petItems                 *petitem.Service
	trades                   *tradebook.Book
	enchantState             *enchantflow.State
	enchant                  *enchantflow.Service
	targets                  *skilltarget.Registry
	skillHandlers            *handlerskill.Registry
	log                      zerolog.Logger

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

// NewGameClientLink builds a GameClientLink from its collaborators.
// loginLink returns the game server's current link to the login server, or
// nil while disconnected/reconnecting: session validation fails clients
// gracefully (AuthLoginFail) rather than panicking while the link is down.
func NewGameClientLink(
	validator *SessionValidator,
	loginLink func() *LoginLink,
	roster *manager.Roster,
	items itemStore,
	shortcuts shortcutStore,
	templates *player.TemplateTable,
	itemTemplates *item.Table,
	html *datacache.HTML,
	crests *datacache.Crests,
	skills *skillstate.Persistence,
	spellbooks modelskill.BookPolicy,
	skillTrees *modelskill.Trees,
	cursedWeapons *entity.CursedWeaponTable,
	worldState *world.State,
	geo move.Geo,
	ids idAllocator,
	groundItems groundItemDropper,
	attackStance attackStanceTracker,
	positions *task.PositionUpdates,
	playerClock *task.PlayerClock,
	restarts *restart.Table,
	respawnRestoreHP float64,
	levels *player.LevelTable,
	skillEnchantSPBookNeeded bool,
	log zerolog.Logger,
) *GameClientLink {
	return &GameClientLink{
		validator:                validator,
		loginLink:                loginLink,
		roster:                   roster,
		items:                    items,
		shortcuts:                shortcuts,
		templates:                templates,
		itemTemplates:            itemTemplates,
		html:                     html,
		crests:                   crests,
		skills:                   skills,
		spellbooks:               spellbooks,
		skillTrees:               skillTrees,
		cursedWeapons:            cursedWeapons,
		world:                    worldState,
		geo:                      geo,
		ids:                      ids,
		groundItems:              groundItems,
		attackStance:             attackStance,
		positions:                positions,
		playerClock:              playerClock,
		restarts:                 restarts,
		respawnRestoreHP:         respawnRestoreHP,
		levels:                   levels,
		skillEnchantSPBookNeeded: skillEnchantSPBookNeeded,
		inventory:                invops.NewService(ids),
		petItems:                 petitem.NewService(ids),
		trades:                   tradebook.NewBook(time.Now),
		enchantState:             enchantflow.NewState(),
		targets:                  skilltarget.NewRegistry(skilltarget.WorldKnown{State: worldState}),
		skillHandlers:            handlerskill.NewDefaultRegistry(),
		log:                      log,
		newCipherKey:             randomCipherKey,
	}
}

func (l *GameClientLink) inventoryService() *invops.Service {
	if l.inventory == nil {
		l.inventory = invops.NewService(l.ids)
	}
	return l.inventory
}

func (l *GameClientLink) petItemService() *petitem.Service {
	if l.petItems == nil {
		l.petItems = petitem.NewService(l.ids)
	}
	return l.petItems
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
