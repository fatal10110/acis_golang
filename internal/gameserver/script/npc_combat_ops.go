package script

import "github.com/fatal10110/acis_golang/internal/gameserver/model/zone"

// The NPC operations a raid private's behavior reads its fight by: the top
// desire target, its hit points, the zone it stands in, and the reshuffle
// of its attack hate. Each is a hostile NPC's; any other NPC panics.

// TopDesireTarget returns the final target of the desire the NPC's AI last
// took up: the creature it attacks, casts on, flees from or follows. It
// outlives that desire; the AI's periodic idle with nothing queued clears
// it. Nil for none.
func (n *NPC) TopDesireTarget() Creature {
	c := n.hostile("read the top desire target").AI().TopDesireTarget()
	if c == nil {
		return nil
	}
	return creatureOf(c)
}

// SetTopDesireTarget replaces the NPC's top desire target with c, nil for
// none, until its AI next takes up a desire or idles.
func (n *NPC) SetTopDesireTarget(c Creature) {
	n.hostile("set the top desire target").AI().SetTopDesireTarget(combatantOf(c))
}

// RandomizeAttack reshuffles the NPC's attack hate when it hates two
// creatures or more: one creature it hates, other than its most hated and
// one it may attack within party range, is raised to 200 over the most
// hated, and its attack desires are queued again from the hate.
func (n *NPC) RandomizeAttack() { n.hostile("randomize its attack").RandomizeHate() }

// ZoneID names a kind of zone a creature can stand in.
type ZoneID = zone.Flag

// ZonePeace is a peace zone.
const ZonePeace = zone.FlagPeace

// InsideZone reports whether the NPC stands in a zone of kind id.
func (n *NPC) InsideZone(id ZoneID) bool { return n.hostile("check its zone").InsideZone(id) }

// HP returns the NPC's current hit points.
func (n *NPC) HP() float64 { return n.hostile("read its hit points").HP() }

// MaxHP returns the NPC's maximum hit points.
func (n *NPC) MaxHP() int32 { return int32(n.hostile("read its hit points").MaxHP()) }
