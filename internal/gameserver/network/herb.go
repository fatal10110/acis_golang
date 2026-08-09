package network

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	itemhandler "github.com/fatal10110/acis_golang/internal/gameserver/handler/item"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// consumeHerb applies a received herb's carried skill to live and mirrors it
// onto an active servitor, the way a herb behaves the moment it is picked up
// or auto-looted. A herb never enters an inventory, so nothing is consumed
// from one and no InventoryUpdate follows: the item exists only long enough
// to resolve its skill.
func (l *GameClientLink) consumeHerb(live *livePlayer, itemID int32) {
	if live == nil {
		return
	}
	inv := live.Inventory()
	if inv == nil {
		return
	}
	// Only a herb template may take this path: the transient instance below
	// carries no object id, and the instant-cast path only skips its stack
	// decrement — the destroy that would run against the live inventory —
	// for herbs. Checking here keeps that precondition local to the function
	// that depends on it, for every caller.
	tmpl, ok := inv.Templates().Get(itemID)
	if !ok || tmpl.EtcItem == nil || tmpl.EtcItem.Type != item.EtcItemHerb {
		return
	}
	// A transient off-inventory instance is enough: the instant-cast path
	// reads only its template.
	herb := &item.Instance{TemplateID: itemID, Count: 1, Location: item.LocationVoid}
	beforeVitals := live.Vitals()
	results := itemhandler.UseAll(itemhandler.UseRequest{
		Caster:      live.Character,
		Inventory:   inv,
		Item:        herb,
		Definitions: l.skills,
		Effects:     actorcast.EffectHandlers{Targets: l.targets, Skills: l.skillHandlers},
		Destroyer:   l.inventory,
		Summon:      l.activeServitorTarget(live),
	})
	for _, res := range results {
		if res.Outcome == itemhandler.ReuseRejected {
			// The herb is already gone by the time its skill is dispatched, so a
			// still-cooling reuse only reports the reason — the pickup that
			// consumed it owns the action acknowledgement.
			sendMagicCastFailureReason(live, res.Skill, actorcast.ErrSkillDisabled)
			return
		}
		if res.Outcome != itemhandler.Applied {
			return
		}
		self := skillCastObject(live)
		l.broadcastLiveFrame(live, func() wire.Frame {
			return serverpackets.FrameMagicSkillUse(self, self, int32(res.Skill.ID), int32(res.Skill.Level), 0, 0, false)
		})
		if res.MirroredSummon != nil {
			summonObject := skillCastObject(res.MirroredSummon)
			l.broadcastLiveFrame(live, func() wire.Frame {
				return serverpackets.FrameMagicSkillUse(summonObject, summonObject, int32(res.Skill.ID), int32(res.Skill.Level), 0, 0, false)
			})
		}
		res.Apply()
		sendMagicStatusUpdate(live, beforeVitals)
	}
}
