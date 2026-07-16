package network

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attack"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/shortcut"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

type livePlayer struct {
	*player.Character
	template  *player.Template
	items     []*item.Instance
	target    world.Tracked
	throne    staticChairObject
	attack    *attack.Controller
	cast      *actorcast.Controller
	shortcuts *shortcut.List

	stopAttack func(*livePlayer)
}

func (p *livePlayer) SendFrame(frame wire.Frame) bool {
	return p.Character.SendFrame(frame)
}

func (p *livePlayer) Stop() {
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

func (p *livePlayer) castController() *actorcast.Controller {
	if p.cast == nil {
		p.cast = actorcast.NewController(liveCastActor{live: p})
	}
	return p.cast
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
