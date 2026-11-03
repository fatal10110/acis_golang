package player

// NPCForm is the NPC look a polymorphed character is shown in: the NPC it
// passes for, that NPC's name, held items and body.
type NPCForm struct {
	NPCID               int32
	Name                string
	RightHand, LeftHand int32
	CollisionRadius     float64
	CollisionHeight     float64
}

// Polymorph reports the NPC form the character is shown in, nil while it
// shows as itself. The form lives in memory only.
func (c *Character) Polymorph() *NPCForm { return c.polymorph.Load() }

// SetPolymorph shows the character in form, or as itself again when form is
// nil. Callers own the client refresh that follows.
func (c *Character) SetPolymorph(form *NPCForm) { c.polymorph.Store(form) }
