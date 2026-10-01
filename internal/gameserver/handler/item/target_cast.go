package item

import (
	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	modelitem "github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// Handler names of the etc items whose skills are cast on the user's
// current target once the item's own target check passes: chest keys, soul
// crystals and beast spices.
const (
	KeysHandler         = "Keys"
	SoulCrystalsHandler = "SoulCrystals"
	BeastSpicesHandler  = "BeastSpices"
)

// soulCrystalSkillID is the one skill a soul crystal casts.
const soulCrystalSkillID modelskill.ID = 2096

// TargetCastRefusal is why a target-cast item casts nothing.
type TargetCastRefusal uint8

const (
	// TargetCastAllowed means the item's skills are cast on the target.
	TargetCastAllowed TargetCastRefusal = iota
	// TargetCastSilent means the item is refused with no answer.
	TargetCastSilent
	// TargetCastSitting means a seated user is refused (CANT_MOVE_SITTING).
	TargetCastSitting
	// TargetCastInvalidTarget means the target is not one the item is used
	// on (INVALID_TARGET).
	TargetCastInvalidTarget
	// TargetCastNoSkills means a key carries no skill at all: a data error
	// that is only logged.
	TargetCastNoSkills
)

// TargetCastUser is the player using a target-cast item.
type TargetCastUser interface {
	Target() world.Tracked
	// Seated reports a finished sit-down.
	Seated() bool
	MovementDisabled() bool
	MoveSpeed() float64
}

// TargetCast is the decision on one target-cast item use.
type TargetCast struct {
	// Handled reports that the item is a key, soul crystal or beast spice;
	// nothing else is decided otherwise.
	Handled bool
	Refusal TargetCastRefusal
	// Skills are cast, in order, on the user's current target as ordinary
	// skill requests when Refusal is TargetCastAllowed. The item itself is
	// not consumed: a skill that costs it names it as its consume item.
	Skills []modelskill.Definition
	// CtrlForces reports that the UseItem Ctrl modifier is the casts'
	// force-use flag. Otherwise they are cast with no modifiers.
	CtrlForces bool
}

// chestTarget is a chest a key may be used on.
type chestTarget interface {
	Chest() bool
	Dead() bool
	Interacted() bool
}

// feedableTarget is an NPC that can be a beast spice's target.
type feedableTarget interface {
	FeedableBeast() bool
}

// ResolveTargetCast decides a key, soul crystal or beast spice use by user
// against the user's current target. Each item checks its target itself
// before any of its skills reaches the cast pipeline.
func ResolveTargetCast(tmpl *modelitem.Template, user TargetCastUser, defs actorcast.Definitions) TargetCast {
	if tmpl == nil || tmpl.Kind != modelitem.KindEtcItem || tmpl.EtcItem == nil || user == nil {
		return TargetCast{}
	}
	switch tmpl.EtcItem.Handler {
	case KeysHandler:
		return resolveKey(tmpl, user, defs)
	case SoulCrystalsHandler:
		return resolveSoulCrystal(tmpl, user, defs)
	case BeastSpicesHandler:
		return resolveBeastSpice(tmpl, user, defs)
	}
	return TargetCast{}
}

// resolveKey refuses a seated or immobile user, then a target that is not a
// living chest no unlock attempt has claimed yet, and casts every skill the
// key carries.
func resolveKey(tmpl *modelitem.Template, user TargetCastUser, defs actorcast.Definitions) TargetCast {
	if user.Seated() {
		return TargetCast{Handled: true, Refusal: TargetCastSitting}
	}
	if user.MovementDisabled() || user.MoveSpeed() == 0 {
		return TargetCast{Handled: true, Refusal: TargetCastSilent}
	}
	chest, ok := user.Target().(chestTarget)
	if !ok || !chest.Chest() || chest.Dead() || chest.Interacted() {
		return TargetCast{Handled: true, Refusal: TargetCastInvalidTarget}
	}
	if len(tmpl.AttachedSkills) == 0 {
		return TargetCast{Handled: true, Refusal: TargetCastNoSkills}
	}
	skills := make([]modelskill.Definition, 0, len(tmpl.AttachedSkills))
	for _, ref := range tmpl.AttachedSkills {
		if def, ok := resolveSkillRef(ref, defs); ok {
			skills = append(skills, def)
		}
	}
	return TargetCast{Handled: true, Skills: skills}
}

// resolveSoulCrystal casts the crystal's first skill, which must be Soul
// Crystal, on any creature target, Ctrl forcing it.
func resolveSoulCrystal(tmpl *modelitem.Template, user TargetCastUser, defs actorcast.Definitions) TargetCast {
	if len(tmpl.AttachedSkills) == 0 {
		return TargetCast{Handled: true, Refusal: TargetCastSilent}
	}
	def, ok := resolveSkillRef(tmpl.AttachedSkills[0], defs)
	if !ok || def.ID != soulCrystalSkillID {
		return TargetCast{Handled: true, Refusal: TargetCastSilent}
	}
	if _, ok := user.Target().(skilltarget.Actor); !ok {
		return TargetCast{Handled: true, Refusal: TargetCastInvalidTarget}
	}
	return TargetCast{Handled: true, Skills: []modelskill.Definition{def}, CtrlForces: true}
}

// resolveBeastSpice casts the spice's first skill on a FeedableBeast
// target.
func resolveBeastSpice(tmpl *modelitem.Template, user TargetCastUser, defs actorcast.Definitions) TargetCast {
	beast, ok := user.Target().(feedableTarget)
	if !ok || !beast.FeedableBeast() {
		return TargetCast{Handled: true, Refusal: TargetCastInvalidTarget}
	}
	if len(tmpl.AttachedSkills) == 0 {
		return TargetCast{Handled: true, Refusal: TargetCastSilent}
	}
	def, ok := resolveSkillRef(tmpl.AttachedSkills[0], defs)
	if !ok {
		return TargetCast{Handled: true, Refusal: TargetCastSilent}
	}
	return TargetCast{Handled: true, Skills: []modelskill.Definition{def}}
}

func resolveSkillRef(ref modelitem.SkillRef, defs actorcast.Definitions) (modelskill.Definition, bool) {
	if defs == nil {
		return modelskill.Definition{}, false
	}
	return defs.Definition(modelskill.Ref{ID: modelskill.ID(ref.ID), Level: int(ref.Level)})
}
