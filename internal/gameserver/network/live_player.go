package network

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attack"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/shortcut"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/staticobject"
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

	known      world.KnownBuffer
	stopAttack func(*livePlayer)
}

func (p *livePlayer) SendFrame(frame wire.Frame) bool {
	return p.Character.SendFrame(frame)
}

func (p *livePlayer) Stop() {
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
		live.cast.SetOnAbort(func() { l.broadcastCastAborted(live) })
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

func (p *livePlayer) releaseChair() {
	if p == nil || p.throne == nil {
		return
	}
	p.throne.SetBusy(false)
	p.throne = nil
}
