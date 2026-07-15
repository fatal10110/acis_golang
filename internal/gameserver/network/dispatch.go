package network

import (
	"context"
	"crypto/rand"
	"fmt"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/data/manager"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/grounditem"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

type itemStore interface {
	ListByOwner(ctx context.Context, ownerID int32) ([]*item.Instance, error)
	Save(ctx context.Context, inst *item.Instance) error
	Update(ctx context.Context, inst *item.Instance) error
	Delete(ctx context.Context, objectID int32) error
}

type attackStanceTracker interface {
	Add(task.AttackStanceActor)
}

type idAllocator interface {
	NextID() (int32, error)
}

type groundItemDropper interface {
	Drop(*grounditem.Item, task.DropOptions)
}

const (
	crystallizeSkillID      = 248
	dropInteractionDistance = 150
)

// GameClientLink accepts and drives connections from Interlude game
// clients: the VersionCheck/cipher handshake, session-key validation
// against the login server, character list/create/delete/restore, and
// character select through to world entry.
type GameClientLink struct {
	validator     *SessionValidator
	loginLink     func() *LoginLink
	roster        *manager.Roster
	items         itemStore
	templates     *player.TemplateTable
	itemTemplates *item.Table
	skills        *SkillPersistence
	world         *world.State
	ids           idAllocator
	groundItems   groundItemDropper
	attackStance  attackStanceTracker
	trades        *tradeCoordinator
	log           zerolog.Logger

	// newCipherKey supplies each connection's XOR cipher key; overridden in
	// tests for a deterministic handshake.
	newCipherKey func() ([]byte, error)

	// enchantRoll supplies enchant dice rolls; overridden in tests.
	enchantRoll func() float64
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
	templates *player.TemplateTable,
	itemTemplates *item.Table,
	skills *SkillPersistence,
	worldState *world.State,
	ids idAllocator,
	groundItems groundItemDropper,
	attackStance attackStanceTracker,
	log zerolog.Logger,
) *GameClientLink {
	return &GameClientLink{
		validator:     validator,
		loginLink:     loginLink,
		roster:        roster,
		items:         items,
		templates:     templates,
		itemTemplates: itemTemplates,
		skills:        skills,
		world:         worldState,
		ids:           ids,
		groundItems:   groundItems,
		attackStance:  attackStance,
		trades:        newTradeCoordinator(time.Now),
		log:           log,
		newCipherKey:  randomCipherKey,
	}
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
