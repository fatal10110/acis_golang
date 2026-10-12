package script

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/grounditem"
)

// GroundItem is one item lying on the ground.
type GroundItem struct {
	ObjectID int32
	ItemID   int32
}

func groundItemOf(g *grounditem.Item) GroundItem {
	return GroundItem{ObjectID: g.ObjectID(), ItemID: g.ItemID()}
}

// SeeItem runs the see-item hook of every script bound to n's template, in
// list order: n looked for quantity items and found items.
func (r *Registry) SeeItem(n attackable.Combatant, quantity int32, items []*grounditem.Item) {
	h := NPCOf(n)
	if h == nil {
		return
	}
	list := r.scripts(h.NpcID(), EventSeeItem)
	if len(list) == 0 {
		return
	}
	seen := make([]GroundItem, len(items))
	for i, g := range items {
		seen[i] = groundItemOf(g)
	}
	e := SeeItem{NPC: h, Quantity: quantity, Items: seen}
	for _, s := range list {
		r.run(s, hookSeeItem, func() { s.Hooks.SeeItem(s, e) })
	}
}

// PickedItem runs the picked-item hook of every script bound to n's
// template, in list order: n picked up ground.
func (r *Registry) PickedItem(n attackable.Combatant, ground *grounditem.Item) {
	h := NPCOf(n)
	if h == nil {
		return
	}
	list := r.scripts(h.NpcID(), EventPickedItem)
	if len(list) == 0 {
		return
	}
	e := PickedItem{NPC: h, Item: groundItemOf(ground)}
	for _, s := range list {
		r.run(s, hookPickedItem, func() { s.Hooks.PickedItem(s, e) })
	}
}

// LookItem makes the NPC tell its see-item scripts, with quantity, about
// the ground items it knows within radius whose item id is one of ids, any
// item when ids is empty, before it returns; nothing is told when no item
// matches. A handle on nothing does nothing.
func (n *NPC) LookItem(radius int, quantity int32, ids ...int32) {
	switch o := n.combatant().(type) {
	case *npc.Hostile:
		o.LookItem(radius, quantity, ids...)
	case *npc.Folk:
		o.LookItem(radius, quantity, ids...)
	}
}

// AddPickUpDesire asks the NPC to pick up the ground item itemObjectID: it
// walks to within 36 of it, its picked-item hooks run, then the item leaves
// the world, a cursed weapon by ending its life. Object id 0 is ignored.
// The desire names the item by id only: one whose item is gone, picked up
// by someone else or no longer known to the NPC stays queued and does
// nothing.
func (n *NPC) AddPickUpDesire(itemObjectID int32, weight float64) {
	n.mind().AddPickUpDesire(itemObjectID, weight)
}
