package skill

import (
	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
)

// regularUnlockKeySkillID is the base unlock-key skill: it starts at a
// lower (60%) success rate than every other deluxe-key skill (100%).
const regularUnlockKeySkillID = 2065

// DoorUnlockUnableMessage reports a door the cast skill can never unlock.
// DoorUnlockFailedMessage reports an unlock attempt on a door that failed
// or found the door already open. UnlockInvalidTargetMessage reports an
// unlock cast whose target is neither a door nor a chest.
type (
	DoorUnlockUnableMessage    struct{}
	DoorUnlockFailedMessage    struct{}
	UnlockInvalidTargetMessage struct{}
)

// doorTarget is a spawned door. Open changes its state through the door's
// owner, which also applies the geodata, status broadcast, linked door and
// auto-close timer.
type doorTarget interface {
	Unlockable() bool
	Opened() bool
	Open() bool
}

// chestTarget is a Chest NPC.
type chestTarget interface {
	Actor
	Unlockable() bool
	Box() bool
	Interacted() bool
	ClaimInteraction() bool
	Level() int
	AddAttackDesire(attacker attackable.Combatant, hate float64)
	AddDamageHate(attacker attackable.Combatant, damage, hate float64)
	Kill(killer attackable.Combatant) bool
	DeleteMe()
}

type unlockHandler struct{}

func (unlockHandler) Types() []string {
	return []string{"UNLOCK", "UNLOCK_SPECIAL", "DELUXE_KEY_UNLOCK"}
}

// Use opens a door or chest target for a player caster. The UNLOCKABLE
// target type admits only an unlockable door or a chest, but an unlock
// skill with a ONE target (the event chest key) can land on any actor;
// such a cast is answered as an invalid target.
func (unlockHandler) Use(cast Cast) {
	if len(cast.Targets) == 0 {
		return
	}
	if _, ok := asPlayer(cast.Caster); !ok {
		return
	}

	target := cast.Targets[0]
	if door, ok := target.(doorTarget); ok && target.Kind() == actor.KindDoor {
		useOnDoor(cast, door)
		return
	}
	if chest, ok := target.(chestTarget); ok && chest.Unlockable() {
		useOnChest(cast, chest)
		return
	}
	cast.record(UnlockInvalidTargetMessage{})
}

func useOnDoor(cast Cast, target doorTarget) {
	special := skillTypeKey(cast.Skill.SkillType) == "UNLOCK_SPECIAL"
	if !target.Unlockable() && !special {
		cast.record(DoorUnlockUnableMessage{})
		return
	}

	opens := false
	if !target.Opened() {
		if special {
			opens = formulas.DoorUnlockSpecialSucceeds(float64(cast.Skill.Power), rnd.Get(100))
		} else {
			opens = formulas.DoorUnlockSucceeds(cast.Skill.Level, rnd.Get(120))
		}
	}
	if !opens {
		cast.record(DoorUnlockFailedMessage{})
		return
	}
	// Another opener can win the race after the check above; the door is
	// open either way, so losing it answers nothing, as an already-open
	// door's state change does.
	target.Open()
}

func useOnChest(cast Cast, target chestTarget) {
	if target.Dead() || target.Interacted() {
		return
	}

	if !target.Box() {
		target.AddAttackDesire(cast.Caster, 200)
		return
	}
	if !target.ClaimInteraction() {
		return
	}

	var opens bool
	if skillTypeKey(cast.Skill.SkillType) == "DELUXE_KEY_UNLOCK" {
		regular := int(cast.Skill.ID) == regularUnlockKeySkillID
		rate := formulas.ChestUnlockDeluxeKeyRate(target.Level(), cast.Skill.Level, regular)
		opens = rnd.Get(100) < rate
	} else {
		rate, definite, succeeds := formulas.ChestUnlockRate(target.Level(), cast.Skill.Level)
		if definite {
			opens = succeeds
		} else {
			opens = rnd.Get(100) < rate
		}
	}

	if opens {
		// The opener's hate lets the kill pay its rewards.
		target.AddDamageHate(cast.Caster, 0, 200)
		target.Kill(cast.Caster)
		return
	}
	target.DeleteMe()
}
