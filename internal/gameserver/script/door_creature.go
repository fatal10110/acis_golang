package script

import "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"

// A door is a creature a skill can land on, so a door handle is a Creature:
// a see-spell payload lists the doors a skill was cast on in their place
// among its targets (#3763). The world tracks a door apart from the
// creatures that fight, so the creature operations that act on a fighting
// creature (desires, hate, teleports) find nothing in a door handle; its
// identity, name, level, state and place are its own.

var _ Creature = (*Door)(nil)

// ObjectID returns the door's world object id.
func (d *Door) ObjectID() int32 { return d.obj.ObjectID() }

// Name returns the door's template name.
func (d *Door) Name() string { return d.obj.CharacterName() }

// Level returns the door's template level.
func (d *Door) Level() int32 { return int32(d.obj.Level()) }

// Dead reports whether the door is broken.
func (d *Door) Dead() bool { return d.obj.Dead() }

// HPRatio returns the door's current HP over its max HP.
func (d *Door) HPRatio() float64 { return d.obj.HPRatio() }

// Playable reports false: no player controls a door.
func (d *Door) Playable() bool { return false }

// Summon reports false: a door is no summon.
func (d *Door) Summon() bool { return false }

// SiegeSummon reports false: a door is no summon.
func (d *Door) SiegeSummon() bool { return false }

// InstantTeleportTo panics: no instant teleport moves a door.
func (d *Door) InstantTeleportTo(Place) { panic("script: an instant teleport of a door") }

// combatant is nil: a door is not a creature that fights.
func (d *Door) combatant() attackable.Combatant { return nil }

func (d *Door) loc() Loc {
	x, y, z := d.obj.Position()
	return Loc{X: x, Y: y, Z: z, Heading: d.obj.Heading()}
}
