package skill

import (
	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
)

// regularUnlockKeySkillID is the base unlock-key skill: it starts at a
// lower (60%) success rate than every other deluxe-key skill (100%).
const regularUnlockKeySkillID = 2065

type doorTarget interface {
	Unlockable() bool
	Opened() bool
	Open()
}

type chestTarget interface {
	Dead() bool
	Interacted() bool
	SetInteracted()
	Box() bool
	Level() int
	Die(killer any)
	DeleteMe()
}

// attackDesirable optionally lets a chest that resists opening notify its
// AI to attack instead; a target without an AI wired up simply skips it.
type attackDesirable interface {
	AddAttackDesire(attacker any, weight float64)
}

// hateAdder optionally lets a broken-open chest seed its reward
// distribution with the opener's hate; skipped when unimplemented.
type hateAdder interface {
	AddDamageHate(attacker any, damage, hate float64)
}

type unlockHandler struct{}

func (unlockHandler) Types() []string {
	return []string{"UNLOCK", "UNLOCK_SPECIAL", "DELUXE_KEY_UNLOCK"}
}

// Use opens a door or chest target. The live game additionally requires
// the caster to be a player; no generic "is a player" marker exists yet
// in this duck-typed model, so any caster can trigger an unlock here.
func (unlockHandler) Use(cast Cast) {
	if len(cast.Targets) == 0 {
		return
	}

	switch target := cast.Targets[0].(type) {
	case doorTarget:
		useOnDoor(cast, target)
	case chestTarget:
		useOnChest(cast, target)
	}
}

func useOnDoor(cast Cast, target doorTarget) {
	special := skillTypeKey(cast.Skill.SkillType) == "UNLOCK_SPECIAL"
	if !target.Unlockable() && !special {
		return
	}
	if target.Opened() {
		return
	}

	var opens bool
	if special {
		opens = formulas.DoorUnlockSpecialSucceeds(float64(cast.Skill.Power), rnd.Get(100))
	} else {
		opens = formulas.DoorUnlockSucceeds(cast.Skill.Level, rnd.Get(120))
	}
	if opens {
		target.Open()
	}
}

func useOnChest(cast Cast, target chestTarget) {
	if target.Dead() || target.Interacted() {
		return
	}

	if !target.Box() {
		if d, ok := any(target).(attackDesirable); ok {
			d.AddAttackDesire(cast.Caster, 200)
		}
		return
	}
	target.SetInteracted()

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
		if h, ok := any(target).(hateAdder); ok {
			h.AddDamageHate(cast.Caster, 0, 200)
		}
		target.Die(cast.Caster)
		return
	}
	target.DeleteMe()
}
