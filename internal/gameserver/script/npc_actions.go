package script

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
)

// instance returns the NPC's instance; a handle on nothing panics.
func (n *NPC) instance() *npc.Instance {
	switch o := n.combatant().(type) {
	case *npc.Hostile:
		return o.Instance
	case *npc.Folk:
		return o.Instance
	}
	panic("script: the instance of a handle on nothing")
}

// Race returns the race of the NPC's template.
func (n *NPC) Race() npc.Race { return n.instance().Template.Race }

// SpawnLocation returns where the NPC was spawned this life.
func (n *NPC) SpawnLocation() Loc {
	inst := n.instance()
	return Loc{X: inst.Home.X, Y: inst.Home.Y, Z: inst.Home.Z, Heading: inst.SpawnHeading, standing: true}
}

// ShowChatWindow opens on p the NPC's own chat window: the one it opens
// when talked to with no script answering.
func (n *NPC) ShowChatWindow(p *Player) {
	p.character().ShowNPCChat(n.ObjectID())
}

// DropItem drops amount units of itemID on the ground around the NPC, as
// its death drops fall, reserved for to while to acts for a player. Only a
// hostile NPC drops anything; a handle on any other NPC panics.
func (n *NPC) DropItem(to Creature, itemID, amount int32) {
	n.hostile("drop items").DropItem(combatantOf(to), itemID, int(amount))
}
