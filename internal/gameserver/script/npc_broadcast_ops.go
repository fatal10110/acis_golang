package script

import "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"

// PlayMusic plays the music file bound to the NPC, starting where it
// stands, for the players that know it, once each and in call order with
// the NPC's other broadcasts. A handle on nothing plays nothing.
func (n *NPC) PlayMusic(file string) {
	switch o := n.combatant().(type) {
	case *npc.Hostile:
		o.PlayMusic(file)
	case *npc.Folk:
		o.PlayMusic(file)
	}
}

// ShoutInRadius shouts text as the NPC's chat line to the players within
// radius of it, bodies included, however far that reaches past the
// players that know it; a negative radius is 600. A handle on nothing
// shouts nothing.
func (n *NPC) ShoutInRadius(text string, radius int) {
	if radius < 0 {
		radius = 600
	}
	switch o := n.combatant().(type) {
	case *npc.Hostile:
		o.ShoutInRadius(text, radius)
	case *npc.Folk:
		o.ShoutInRadius(text, radius)
	}
}
