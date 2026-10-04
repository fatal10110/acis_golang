package manager

import (
	"math/rand/v2"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/grounditem"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/party"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
)

// dropScatterOffset is the +/- range applied to each ground item's drop
// point around the corpse.
const dropScatterOffset = 70

// Loot-protection windows for a regular kill's drop and a raid kill's drop.
const (
	regularLootProtection = 15 * time.Second
	raidLootProtection    = 300 * time.Second
)

// groundPlacer drops a rolled item into the visible world. Satisfied by
// *task.GroundItems.
type groundPlacer interface {
	Drop(ground *grounditem.Item, opts task.DropOptions)
}

// rewardItemReceiver takes auto-looted items straight into its inventory.
// RewardItemFits is the slot check an auto-loot must pass first; an item
// that does not fit falls to the ground instead.
type rewardItemReceiver interface {
	RewardItemFits(itemID int32, count int) bool
	AddRewardItem(itemID int32, count int, objectID int32) bool
}

// partyLooter hands an auto-looted item to its receiver's party, by the
// party's loot rule; LootForParty reports false when the receiver is in no
// party.
type partyLooter interface {
	LootForParty(itemID int32, count int, spoil bool, origin party.LootOrigin) bool
}

// herbReceiver consumes an auto-looted herb on the spot. A herb never
// reaches an inventory: it applies its carried skill to the receiver and is
// discarded. The result reports whether a consumer was wired to take it — a
// detached character has none — so a refused herb can still be delivered
// another way.
type herbReceiver interface {
	ConsumeHerb(itemID int32) bool
}

// raidDropAnnouncer tells a raid or grand boss's observers about each item
// its kill drops or auto-loots.
type raidDropAnnouncer interface {
	AnnounceRaidDrop(itemID int32, count int)
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

	// origin is where the drop comes from: a partied receiver's
	// auto-looted items go to the members in party range of it. Nil keeps
	// them with the receiver.
	origin party.LootOrigin

	// multipleItemDrop is server.properties MultipleItemDrop: a
	// non-stackable item rolled N times falls as N ground items instead of
	// one.
	multipleItemDrop bool

	// announcer names a raid kill's every dropped or auto-looted item to
	// the boss's observers; nil announces nothing.
	announcer raidDropAnnouncer
}

// NewKillReward returns a Rewarder that rolls categories against pool and
// rates, then places the results on the ground at (x, y, z, heading).
// levelMultiplier is the caller-resolved drop-rate penalty for the
// killer/monster level gap (see item.LevelPenaltyMultiplier); pool may be
// nil for an unspoiled monster. dropperID is the dying NPC's object id, so
// nearby observers see the loot fall from its corpse. geo scatters each
// stack around (x, y, z) and validates the result against geodata; a nil
// geo drops every stack at the exact corpse position instead.
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

// From sets where the drop comes from, so a partied receiver's auto-looted
// items follow its party's loot rule among the members in party range of
// origin. It returns k.
func (k *KillReward) From(origin party.LootOrigin) *KillReward {
	k.origin = origin
	return k
}

// MultipleItemDrop sets server.properties MultipleItemDrop: whether a
// non-stackable item rolled with a count above one falls as that many
// ground items, or as a single one. It returns k.
func (k *KillReward) MultipleItemDrop(enabled bool) *KillReward {
	k.multipleItemDrop = enabled
	return k
}

// AnnounceTo sets who names each item a raid kill drops or auto-loots to
// the boss's observers; it has no effect on a non-raid kill. It returns k.
func (k *KillReward) AnnounceTo(announcer raidDropAnnouncer) *KillReward {
	k.announcer = announcer
	return k
}

// CalculateRewards rolls this death's item/spoil/herb drops and either
// places them on the ground or, when configured and supported, adds them
// directly to the killer's inventory, one category roll at a time in the
// reference's order. An auto-looted herb is consumed instantly instead:
// herbs never occupy an inventory slot.
func (k *KillReward) CalculateRewards(killer attackable.Combatant) {
	receiver, isPlayer := killer.(rewardItemReceiver)
	if isPlayer {
		// Only a Playable kill gets its drop reserved: the killer must
		// resolve to an acting player before drop protection is set.
		k.protectOwnerID = killer.ObjectID()
	}
	for _, d := range item.RollKillReward(k.categories, k.pool, k.levelMultiplier, k.raid, k.rates, k.autoLootHerbs) {
		if d.Herb {
			k.herb(killer, receiver, d)
			continue
		}
		if !k.autoLootItems || !k.autoLoot(receiver, d.ItemID, int(d.Count)) {
			k.drop(d.ItemID, int(d.Count))
		}
		if k.raid && k.announcer != nil {
			k.announcer.AnnounceRaidDrop(d.ItemID, int(d.Count))
		}
	}
}

// herb hands a herb category's pickup to killer, or leaves it on the
// ground.
func (k *KillReward) herb(killer attackable.Combatant, receiver rewardItemReceiver, d item.KillDrop) {
	if d.AutoLoot {
		// A herb is consumed or it is left on the ground for another
		// attempt; it never occupies an inventory slot, since it carries
		// no icon there. Only an ordinary item that a category mislabelled
		// HERB happens to hold takes the auto-loot inventory path.
		if k.isHerb(d.ItemID) {
			if k.consumeHerb(killer, d.ItemID) {
				return
			}
		} else if k.addToInventory(receiver, d.ItemID, int(d.Count)) {
			return
		}
	}
	k.drop(d.ItemID, int(d.Count))
}

// isHerb reports whether itemID's template is a herb. Herb handling follows
// the template's etc type, not the drop category the item was rolled from.
func (k *KillReward) isHerb(itemID int32) bool {
	tmpl, ok := k.items.Get(itemID)
	return ok && tmpl.EtcItem != nil && tmpl.EtcItem.Type == item.EtcItemHerb
}

// consumeHerb hands itemID to killer for instant consumption and reports
// whether a consumer was there to take it.
func (k *KillReward) consumeHerb(killer attackable.Combatant, itemID int32) bool {
	consumer, ok := killer.(herbReceiver)
	if !ok {
		return false
	}
	return consumer.ConsumeHerb(itemID)
}

// autoLoot takes an auto-looted item into the receiver's inventory, or
// through its party's loot rule when it is in one. The receiver's own room
// for it decides either way.
func (k *KillReward) autoLoot(receiver rewardItemReceiver, itemID int32, count int) bool {
	if receiver == nil || count <= 0 || !receiver.RewardItemFits(itemID, count) {
		return false
	}
	if _, ok := k.items.Get(itemID); !ok {
		return false
	}
	if sharer, ok := receiver.(partyLooter); ok && k.origin != nil && sharer.LootForParty(itemID, count, false, k.origin) {
		return true
	}
	return k.addToInventory(receiver, itemID, count)
}

func (k *KillReward) addToInventory(receiver rewardItemReceiver, itemID int32, count int) bool {
	if receiver == nil || count <= 0 {
		return false
	}
	if _, ok := k.items.Get(itemID); !ok {
		return false
	}
	if !receiver.RewardItemFits(itemID, count) {
		return false
	}
	id, err := k.ids.NextID()
	if err != nil {
		return false
	}
	return receiver.AddRewardItem(itemID, count, id)
}

// drop places one rolled item on the ground: a stackable one as a single
// stack of count, a non-stackable one as count separate single items under
// MultipleItemDrop, or else as just one. It is a best-effort placement:
// running out of allocatable object ids or an unknown item id skips the
// rest of that roll rather than failing the whole reward, since
// CalculateRewards has no error return to report a partial failure
// through.
func (k *KillReward) drop(itemID int32, count int) {
	if count <= 0 {
		return
	}
	tmpl, ok := k.items.Get(itemID)
	if !ok {
		return
	}
	if tmpl.Stackable {
		k.place(tmpl, count)
		return
	}
	if !k.multipleItemDrop {
		count = 1
	}
	for range count {
		if !k.place(tmpl, 1) {
			return
		}
	}
}

// place drops one ground item of tmpl holding count units, scattered
// around the corpse on its own, and reports whether it was placed.
func (k *KillReward) place(tmpl *item.Template, count int) bool {
	id, err := k.ids.NextID()
	if err != nil {
		return false
	}
	inst := item.Instance{ObjectID: id, TemplateID: tmpl.ID, Count: count, Location: item.LocationVoid}
	ground, err := grounditem.New(inst, tmpl)
	if err != nil {
		return false
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
	return true
}
