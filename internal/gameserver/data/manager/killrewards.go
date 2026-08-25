package manager

import (
	"math/rand/v2"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/grounditem"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
)

// dropScatterOffset is the +/- range applied to each stack's drop point
// around the corpse, matching Npc.dropItem's item.dropMe(this, 70) call
// (aCis Npc.java:2258).
const dropScatterOffset = 70

// Loot-protection windows mirror ItemInstance's REGULAR_LOOT_PROTECTION_TIME
// / RAID_LOOT_PROTECTION_TIME constants (aCis ItemInstance.java:53-54).
const (
	regularLootProtection = 15 * time.Second
	raidLootProtection    = 300 * time.Second
)

// groundPlacer drops a rolled item into the visible world. Satisfied by
// *task.GroundItems.
type groundPlacer interface {
	Drop(ground *grounditem.Item, opts task.DropOptions)
}

type rewardItemReceiver interface {
	AddRewardItem(itemID int32, count int, objectID int32) bool
}

// herbReceiver consumes an auto-looted herb on the spot. A herb never
// reaches an inventory: it applies its carried skill to the receiver and is
// discarded. The result reports whether a consumer was wired to take it — a
// detached character has none — so a refused herb can still be delivered
// another way.
type herbReceiver interface {
	ConsumeHerb(itemID int32) bool
}

// KillReward rolls and places the item, spoil, and manually-picked-up herb
// rewards for one NPC template's death, at a fixed drop location.
//
// Experience and SP are granted by the higher-level death rewarder, which
// owns damage attribution and victim level.
type KillReward struct {
	categories      []item.DropCategory
	pool            *item.SpoilPool
	levelMultiplier float64
	raid            bool
	rates           item.Rates
	autoLootItems   bool
	autoLootHerbs   bool

	ids    idAllocator
	items  *item.Table
	ground groundPlacer
	geo    move.Geo

	x, y, z, heading int
	dropperID        int32
	protectOwnerID   int32
}

// NewKillReward returns a Rewarder that rolls categories against pool and
// rates, then places the results on the ground at (x, y, z, heading).
// levelMultiplier is the caller-resolved drop-rate penalty for the
// killer/monster level gap (see item.LevelPenaltyMultiplier); pool may be
// nil for an unspoiled monster. dropperID is the dying NPC's object id, so
// nearby observers see the loot fall from its corpse. geo scatters each
// stack around (x, y, z) and validates the result against geodata,
// matching ItemInstance.dropMe(Creature, int) (aCis ItemInstance.java:768-
// 789); a nil geo drops every stack at the exact corpse position instead.
func NewKillReward(categories []item.DropCategory, pool *item.SpoilPool, levelMultiplier float64, raid bool, rates item.Rates, autoLootItems, autoLootHerbs bool, ids idAllocator, items *item.Table, ground groundPlacer, geo move.Geo, x, y, z, heading int, dropperID int32) *KillReward {
	return &KillReward{
		categories:      categories,
		pool:            pool,
		levelMultiplier: levelMultiplier,
		raid:            raid,
		rates:           rates,
		autoLootItems:   autoLootItems,
		autoLootHerbs:   autoLootHerbs,
		ids:             ids,
		items:           items,
		ground:          ground,
		geo:             geo,
		x:               x,
		y:               y,
		z:               z,
		heading:         heading,
		dropperID:       dropperID,
	}
}

// CalculateRewards rolls this death's item/spoil/herb drops and either
// places them on the ground or, when configured and supported, adds them
// directly to the killer's inventory. An auto-looted herb is consumed
// instantly instead: herbs never occupy an inventory slot.
func (k *KillReward) CalculateRewards(killer creature.DeathActor) {
	receiver, isPlayer := killer.(rewardItemReceiver)
	if isPlayer {
		// Only a Playable kill gets its drop reserved, matching Npc.dropItem's
		// `creature.getActingPlayer() != null` guard before setDropProtection
		// (aCis Npc.java:2255-2256).
		k.protectOwnerID = killer.ObjectID()
	}
	rolled, herbs := item.RollKillReward(k.categories, k.pool, k.levelMultiplier, k.raid, k.rates, k.autoLootHerbs)
	for id, qty := range rolled {
		if k.autoLootItems && k.addToInventory(receiver, id, int(qty)) {
			continue
		}
		k.drop(id, int(qty))
	}
	for _, herb := range herbs {
		if herb.AutoLoot {
			// A herb is consumed or it is left on the ground for another
			// attempt; it never occupies an inventory slot, since it carries
			// no icon there. Only an ordinary item that a category mislabelled
			// HERB happens to hold takes the auto-loot inventory path.
			if k.isHerb(herb.ItemID) {
				if k.consumeHerb(killer, herb.ItemID) {
					continue
				}
			} else if k.addToInventory(receiver, herb.ItemID, int(herb.Amount)) {
				continue
			}
		}
		k.drop(herb.ItemID, int(herb.Amount))
	}
}

// isHerb reports whether itemID's template is a herb. Herb handling follows
// the template's etc type, not the drop category the item was rolled from.
func (k *KillReward) isHerb(itemID int32) bool {
	tmpl, ok := k.items.Get(itemID)
	return ok && tmpl.EtcItem != nil && tmpl.EtcItem.Type == item.EtcItemHerb
}

// consumeHerb hands itemID to killer for instant consumption and reports
// whether a consumer was there to take it.
func (k *KillReward) consumeHerb(killer creature.DeathActor, itemID int32) bool {
	consumer, ok := killer.(herbReceiver)
	if !ok {
		return false
	}
	return consumer.ConsumeHerb(itemID)
}

func (k *KillReward) addToInventory(receiver rewardItemReceiver, itemID int32, count int) bool {
	if receiver == nil || count <= 0 {
		return false
	}
	if _, ok := k.items.Get(itemID); !ok {
		return false
	}
	id, err := k.ids.NextID()
	if err != nil {
		return false
	}
	return receiver.AddRewardItem(itemID, count, id)
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

	x, y, z := k.x, k.y, k.z
	if k.geo != nil {
		nx := k.x + rand.IntN(2*dropScatterOffset+1) - dropScatterOffset
		ny := k.y + rand.IntN(2*dropScatterOffset+1) - dropScatterOffset
		loc := k.geo.ValidLocation(k.x, k.y, k.z, nx, ny, k.z)
		x, y, z = loc.X, loc.Y, loc.Z
	}

	protectFor := regularLootProtection
	if k.raid {
		protectFor = raidLootProtection
	}

	k.ground.Drop(ground, task.DropOptions{
		X: x, Y: y, Z: z, Heading: k.heading,
		DropperID:      k.dropperID,
		ProtectOwnerID: k.protectOwnerID,
		ProtectFor:     protectFor,
	})
}
