package network

import (
	"context"
	"sync"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attack"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
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
}

type pickupIntention struct {
	ctx    context.Context
	target world.Tracked
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

func (p *livePlayer) deferPickup(ctx context.Context, target world.Tracked) {
	p.pickupMu.Lock()
	defer p.pickupMu.Unlock()
	p.deferredPickup = &pickupIntention{ctx: ctx, target: target}
}

func (p *livePlayer) takeDeferredPickup() *pickupIntention {
	p.pickupMu.Lock()
	defer p.pickupMu.Unlock()
	pickup := p.deferredPickup
	p.deferredPickup = nil
	return pickup
}

func (p *livePlayer) setPickupLocked(locked bool) {
	p.pickupMu.Lock()
	defer p.pickupMu.Unlock()
	p.pickupLocked = locked
}

func (p *livePlayer) pickupLockActive() bool {
	p.pickupMu.Lock()
	defer p.pickupMu.Unlock()
	return p.pickupLocked
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
