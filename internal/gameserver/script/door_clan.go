package script

import (
	"slices"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

const (
	// doorClan is the clan every door belongs to: a hit on a door calls
	// the NPCs of this clan around it.
	doorClan = "door_clan"
	// doorClanRange is how far a hit door's call reaches.
	doorClanRange = 600
)

var _ door.ClanCall = (*Registry)(nil)

// doorBody is a door as the range of its call measures it: widened by its
// collision radius, as an NPC's by its own.
type doorBody struct{ *door.Object }

func (doorBody) CollisionRadius() float64 { return door.CollisionRadius }

// DoorAttacked runs, for every NPC within the door clan's range of d (3D,
// inclusive, widened by both collision radii), alive and of the door clan,
// the static-object clan-attacked hook of every script bound to its
// template, in list order: attacker hit d for damage. The NPCs in range are
// collected first; the other gates are checked as each one's turn comes,
// so a hook that changes a later NPC is seen.
func (r *Registry) DoorAttacked(d *door.Object, attacker attackable.Combatant, damage float64) {
	if r == nil || r.world == nil {
		return
	}
	var near []attackable.Combatant
	r.world.ForEachKnownInRadius(doorBody{d}, doorClanRange, func(o world.Tracked) {
		switch n := o.(type) {
		case *npc.Hostile:
			near = append(near, n)
		case *npc.Folk:
			near = append(near, n)
		}
	})
	attackerHandle := creatureOf(attacker)
	for _, called := range near {
		if called.Dead() || !slices.Contains(clansOf(called), doorClan) {
			continue
		}
		list := r.scripts(npcIDOf(called), EventStaticObjectClanAttacked)
		if len(list) == 0 {
			continue
		}
		e := StaticObjectClanAttacked{Called: NPCOf(called), Attacker: attackerHandle, Damage: int32(damage)}
		for _, s := range list {
			r.run(s, hookStaticObjectClanAttacked, func() { s.Hooks.StaticObjectClanAttacked(s, e) })
		}
	}
}

// clansOf returns the clans of the NPC n's template.
func clansOf(n attackable.Combatant) []string {
	switch o := n.(type) {
	case *npc.Hostile:
		return o.Instance.Template.Clans
	case *npc.Folk:
		return o.Instance.Template.Clans
	}
	return nil
}
