package manager

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/grounditem"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
)

// groundPlacer drops a rolled item into the visible world. Satisfied by
// *task.GroundItems.
type groundPlacer interface {
	Drop(ground *grounditem.Item, opts task.DropOptions)
}

// KillReward rolls and places the item, spoil, and manually-picked-up herb
// rewards for one NPC template's death, at a fixed drop location.
//
// Experience and SP are not granted here. The per-kill formula (damage-share
// split across attackers, then a level-difference penalty) exists as
// player.KillRewardExpAndSp, but there is no live player actor or
// killer/victim resolution to drive it with yet — see the
// kill-reward-distribution follow-up.
type KillReward struct {
	categories      []item.DropCategory
	pool            *item.SpoilPool
	levelMultiplier float64
	raid            bool
	rates           item.Rates
	autoLootHerbs   bool

	ids    idAllocator
	items  *item.Table
	ground groundPlacer

	x, y, z, heading int
}

// NewKillReward returns a Rewarder that rolls categories against pool and
// rates, then places the results on the ground at (x, y, z, heading).
// levelMultiplier is the caller-resolved drop-rate penalty for the
// killer/monster level gap (see item.LevelPenaltyMultiplier); pool may be
// nil for an unspoiled monster.
func NewKillReward(categories []item.DropCategory, pool *item.SpoilPool, levelMultiplier float64, raid bool, rates item.Rates, autoLootHerbs bool, ids idAllocator, items *item.Table, ground groundPlacer, x, y, z, heading int) *KillReward {
	return &KillReward{
		categories:      categories,
		pool:            pool,
		levelMultiplier: levelMultiplier,
		raid:            raid,
		rates:           rates,
		autoLootHerbs:   autoLootHerbs,
		ids:             ids,
		items:           items,
		ground:          ground,
		x:               x,
		y:               y,
		z:               z,
		heading:         heading,
	}
}

// CalculateRewards rolls this death's item/spoil/herb drops and places them
// on the ground. killer is unused: nothing here depends on the killer's
// identity, only on the victim's own drop table.
func (k *KillReward) CalculateRewards(creature.DeathActor) {
	rolled, herbs := item.RollKillReward(k.categories, k.pool, k.levelMultiplier, k.raid, k.rates, k.autoLootHerbs)
	for id, qty := range rolled {
		k.drop(id, int(qty))
	}
	for _, herb := range herbs {
		// An auto-looted herb belongs in the killer's inventory, never on
		// the ground; there is no live inventory to place it in yet, so it
		// is dropped rather than silently fabricated as a ground item.
		if herb.AutoLoot {
			continue
		}
		k.drop(herb.ItemID, int(herb.Amount))
	}
}

// drop places one item stack on the ground. It is a best-effort placement:
// running out of allocatable object ids or an unknown item id skips that
// stack rather than failing the whole reward, since CalculateRewards has no
// error return to report a partial failure through.
func (k *KillReward) drop(itemID int32, count int) {
	if count <= 0 {
		return
	}
	tmpl, ok := k.items.Get(itemID)
	if !ok {
		return
	}
	id, err := k.ids.NextID()
	if err != nil {
		return
	}
	inst := item.Instance{ObjectID: id, TemplateID: itemID, Count: count, Location: item.LocationVoid}
	ground, err := grounditem.New(inst, tmpl)
	if err != nil {
		return
	}
	k.ground.Drop(ground, task.DropOptions{X: k.x, Y: k.y, Z: k.z, Heading: k.heading})
}
