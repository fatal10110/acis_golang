package event

// OwnerInfoChanged reports that a summon's owner-only info window is stale.
// EffectPass marks a stat change an effect's add or removal made: the
// effect list's own icon refresh follows it, so the icons the new window
// clears need no separate resend.
type OwnerInfoChanged struct{ EffectPass bool }

// Damaged reports direct damage a summon took from a named attacker.
type Damaged struct {
	AttackerName string
	Damage       int32
}

// ExpGained reports experience a pet earned.
type ExpGained struct{ Exp int64 }

// PetUsedSkill reports a skill a pet decided to use on its own; its owner
// is told the pet uses it.
type PetUsedSkill struct{ SkillID, Level int32 }

// Unsummoning reports that a summon is about to leave the world for good.
// It is emitted once, while the summon and its owner are both still in world
// state, so whatever the summon's departure must settle -- a pet's row and
// inventory -- is settled before anyone is told it is gone.
type Unsummoning struct{}

// SummonRemoved reports that the owner's active summon slot was cleared,
// before the summon leaves the world.
type SummonRemoved struct{}

// PetCorpseDecayed reports that a dead pet's corpse decayed and has left
// the world. The pet is gone for good: its owner loses the collar and the
// pet's saved state with it.
type PetCorpseDecayed struct{}

// CorpseLeftBehind reports that a dead summon's owner left the world while
// the corpse stays in it until it decays (or, for a pet, is revived). Whatever
// the owner's session must settle for the summon is settled now, while the
// owner is still in world state, and the runtime moves the summon's work to a
// queue of the corpse's own.
type CorpseLeftBehind struct{}

// OwnerRelinked reports that a pet its owner left behind as a corpse, dead
// or revived since, now answers to the owner's new session and its work
// runs on that session's queue: the runtime moves the rest of the pet's
// work there and gives up the queue the corpse had of its own.
type OwnerRelinked struct{}

// DecayCanceled reports that a summon's pending corpse decay, if any, is
// dropped: it was revived, or is about to be.
type DecayCanceled struct{}

func (OwnerInfoChanged) event() {}
func (Damaged) event()          {}
func (ExpGained) event()        {}
func (PetUsedSkill) event()     {}
func (Unsummoning) event()      {}
func (SummonRemoved) event()    {}
func (PetCorpseDecayed) event() {}
func (CorpseLeftBehind) event() {}
func (OwnerRelinked) event()    {}
func (DecayCanceled) event()    {}
