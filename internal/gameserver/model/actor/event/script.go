package event

import modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"

// ItemsTaken reports units of ItemID a script took from the character, with
// the chat line naming them; Short reports a take that found too few units
// and removed nothing.
type ItemsTaken struct {
	ItemID int32
	Count  int
	Short  bool
}

func (ItemsTaken) event() {}

// UnequipRequested asks for the worn item ObjectID to be taken off before a
// script takes it: its slot is cleared with no chat line, the equipment's
// effects are recomputed and the character's look is resent.
type UnequipRequested struct{ ObjectID int32 }

func (UnequipRequested) event() {}

// SoundPlayed reports a sound File played for the character alone, bound to
// nothing and at no location.
type SoundPlayed struct{ File string }

func (SoundPlayed) event() {}

// SkillCalled reports a script making the character use Skill on the
// creature TargetID at once: the cast animation shows, and the skill's
// effects land with no cast time, cost, reuse or target check.
type SkillCalled struct {
	TargetID int32
	Skill    modelskill.Ref
}

func (SkillCalled) event() {}
