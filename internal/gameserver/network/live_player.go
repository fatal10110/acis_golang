package network

import (
	"context"
	"sync"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attack"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cubic"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/shortcut"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/staticobject"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/rs/zerolog"
)

type livePlayer struct {
	*player.Character
	template  *player.Template
	items     []*item.Instance
	throne    staticobject.Chair
	attack    *attack.Controller
	move      *move.Controller
	combat    *ai.PlayerAttack
	cast      *actorcast.Controller
	shortcuts *shortcut.List
	isGM      bool
	log       zerolog.Logger

	known          world.KnownBuffer
	zoneActor      *liveZoneActor
	visibilitySend func(wire.Frame) bool
	stopAttack     func(*livePlayer)
	shadowExpiryMu sync.RWMutex
	detaching      bool
	pickupMu       sync.Mutex // guards pickup, deferredPickup, and pickupLocked
	fusionMu       sync.Mutex // guards fusionTargetID
	fusionTargetID int32
	pickup         *pickupIntention
	deferredPickup *pickupIntention
	pickupLocked   bool
	pickupLockGen  uint64

	cubicsMu sync.Mutex
	cubics   map[cubic.ID]*cubic.Runtime
}

type pickupIntention struct {
	ctx    context.Context
	target world.Tracked
	// shift is only meaningful on deferredPickup: it is the original click's
	// shift-modifier, needed at drain time to decide walk-vs-fail exactly as
	// a fresh click would (CreatureMove.java:438, !isShiftPressed gates the
	// walk). pickup (the in-flight walk-then-collect intention) is only ever
	// set for a non-shift click — a shift click fails outright instead of
	// walking — so it never needs this field.
	shift bool
}

func (p *livePlayer) SendFrame(frame wire.Frame) bool {
	return p.Character.SendFrame(frame)
}

func (p *livePlayer) sendVisibilityFrame(frame wire.Frame) bool {
	if p.visibilitySend == nil {
		frame.Release()
		return false
	}
	return p.visibilitySend(frame)
}

func (p *livePlayer) Stop() {
	p.takePickup()
	p.takeDeferredPickup()
	if p.combat != nil {
		p.combat.Stop()
	}
	if p.attack != nil {
		p.attack.Stop()
	}
	if p.stopAttack != nil {
		p.stopAttack(p)
	}
	p.releaseChair()
	p.stopCubics()
}

// detached reports whether p's session has begun detaching (logout), the
// same shadowExpiryMu-guarded flag taskeffects.go checks before applying a
// deferred effect against an already-detached session.
func (p *livePlayer) detached() bool {
	p.shadowExpiryMu.RLock()
	defer p.shadowExpiryMu.RUnlock()
	return p.detaching
}

// stopCubics cancels every live cubic runtime's timers on detach, so a
// recurring action tick never fires against a session that has already
// logged out — the reference instead relies on fireAction's own
// isDead()/isOnline() self-check on its next scheduled tick, but stopping
// immediately here is equivalent and avoids a stale timer outliving the
// session.
func (p *livePlayer) stopCubics() {
	p.cubicsMu.Lock()
	defer p.cubicsMu.Unlock()
	for _, r := range p.cubics {
		r.Stop()
	}
}

func (p *livePlayer) setPickup(ctx context.Context, target world.Tracked) {
	p.pickupMu.Lock()
	defer p.pickupMu.Unlock()
	p.pickup = &pickupIntention{ctx: ctx, target: target}
}

func (p *livePlayer) takePickup() *pickupIntention {
	p.pickupMu.Lock()
	defer p.pickupMu.Unlock()
	pickup := p.pickup
	p.pickup = nil
	return pickup
}

func (p *livePlayer) deferPickup(ctx context.Context, target world.Tracked, shift bool) {
	p.pickupMu.Lock()
	defer p.pickupMu.Unlock()
	p.deferredPickup = &pickupIntention{ctx: ctx, target: target, shift: shift}
}

func (p *livePlayer) takeDeferredPickup() *pickupIntention {
	p.pickupMu.Lock()
	defer p.pickupMu.Unlock()
	pickup := p.deferredPickup
	p.deferredPickup = nil
	return pickup
}

// pickupLockActive has no production caller: livePickupBlockedDeferrable
// reads pickupLocked directly under its own pickupMu section instead. Kept
// for the generation-primitive regression tests, which check lock state
// independently of that section.
func (p *livePlayer) pickupLockActive() bool {
	p.pickupMu.Lock()
	defer p.pickupMu.Unlock()
	return p.pickupLocked
}

// enterPickupLock starts a new pickup-paralysis lock, invalidating any lock
// still owned by an earlier, not-yet-fired unlock, and reports the
// generation the matching exitPickupLock must present to be honored.
func (p *livePlayer) enterPickupLock() uint64 {
	p.pickupMu.Lock()
	defer p.pickupMu.Unlock()
	p.pickupLockGen++
	p.pickupLocked = true
	p.SetParalyzed(true)
	return p.pickupLockGen
}

// exitPickupLock clears the lock started by the matching enterPickupLock and
// reports whether it did. It is a no-op when gen is stale — a later click
// already replaced this lock with its own — so a delayed unlock can never
// clear a fresher lock's state or its own paralysis mid-way through.
//
// Paralyzed is cleared before pickupLocked, not after: liveItemOpsAllowed
// (the pickup gate) reads Paralyzed via a separate mutex (stateMu) than
// pickupLockActive reads pickupLocked (pickupMu), so a concurrent click can
// observe the two writes independently. Clearing pickupLocked first would
// open a window where a click reads Paralyzed()==true (still blocked) and
// then pickupLockActive()==false (not deferrable) — blocked but undeferrable,
// so it gets discarded instead of re-deferred. Clearing Paralyzed first
// means any click that still observes it true also still observes the lock
// active, and any click that observes Paralyzed already false takes the
// normal (non-blocked) path instead of consulting the lock at all.
func (p *livePlayer) exitPickupLock(gen uint64) bool {
	p.pickupMu.Lock()
	defer p.pickupMu.Unlock()
	if p.pickupLockGen != gen {
		return false
	}
	p.SetParalyzed(false)
	p.pickupLocked = false
	return true
}

func (p *livePlayer) setFusionTarget(id int32) {
	p.fusionMu.Lock()
	p.fusionTargetID = id
	p.fusionMu.Unlock()
}

func (p *livePlayer) clearFusionTarget(id int32) {
	p.fusionMu.Lock()
	if p.fusionTargetID == id {
		p.fusionTargetID = 0
	}
	p.fusionMu.Unlock()
}

func (p *livePlayer) fusesTarget(id int32) bool {
	p.fusionMu.Lock()
	defer p.fusionMu.Unlock()
	return p.fusionTargetID == id
}

func (p *livePlayer) attackController() *attack.Controller {
	if p.attack == nil {
		p.attack = attack.NewPlayer(p.Character)
	}
	return p.attack
}

// castController returns live's cast controller, building it on first use
// with the abort observer that turns an aborted in-flight cast into its
// client-visible cancel packets.
func (l *GameClientLink) castController(live *livePlayer) *actorcast.Controller {
	if live.cast == nil {
		live.cast = actorcast.NewController(actorcast.PlayerActor{Character: live.Character})
		live.cast.SetLogger(live.log)
		live.cast.SetOnAbort(func(interrupted bool) { l.broadcastCastAborted(live, interrupted) })
		live.cast.SetOnFinish(func(bool) {
			if live.combat != nil {
				live.combat.Think()
			}
		})
		live.Character.SetCastController(live.cast)
	}
	return live.cast
}

func (p *livePlayer) inventoryItems() []*item.Instance {
	if p == nil {
		return nil
	}
	if inv := p.Inventory(); inv != nil {
		return inv.Items()
	}
	return p.items
}

// SendInventoryUpdate delivers one batch of queued inventory changes as an
// InventoryUpdate packet, implementing task.InventoryUpdateOwner for
// changes the server makes outside a client request.
func (p *livePlayer) SendInventoryUpdate(updates []itemcontainer.Update) {
	if p == nil || len(updates) == 0 {
		return
	}
	inv := p.Inventory()
	if inv == nil {
		return
	}
	frame, err := serverpackets.FrameInventoryUpdate(updates, inv.Items(), inv.Templates())
	if err != nil {
		p.log.Error().Err(err).Msg("build InventoryUpdate")
		return
	}
	p.SendFrame(frame)
}

func (p *livePlayer) releaseChair() {
	if p == nil || p.throne == nil {
		return
	}
	p.throne.SetBusy(false)
	p.throne = nil
}
