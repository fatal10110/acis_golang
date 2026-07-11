package conditions

// seedSkillIDs are the three elemental-seed effect ids ElementSeed reads a
// charge level from, in Fire/Water/Wind order.
var seedSkillIDs = [3]int{1285, 1286, 1287}

// SeedActor is what ElementSeed needs: the current charge power of an
// elemental-seed effect (one of seedSkillIDs) active on the effector, or 0
// if that seed isn't charged at all. Tracking seed charges is the not-yet-
// built effect engine's job; an effector that doesn't implement this
// reports every seed as uncharged.
type SeedActor interface {
	SeedPower(effectID int) int
}

// ElementSeed requires the effector to have enough Fire/Water/Wind seed
// charge to pay Required[0:3] (indices matching seedSkillIDs), with two
// further optional requirements once the flat costs are paid: Required[3]
// seeds must each still have at least 1 charge left ("any N of the three"),
// and the three remaining charges must sum to at least Required[4].
type ElementSeed struct{ Required [5]int }

func (c ElementSeed) Test(effector, effected, skill any) bool {
	sa, _ := effector.(SeedActor)

	var seeds [3]int
	for i, id := range seedSkillIDs {
		if sa != nil {
			seeds[i] = sa.SeedPower(id)
		}
		if seeds[i] >= c.Required[i] {
			seeds[i] -= c.Required[i]
		} else {
			return false
		}
	}

	if c.Required[3] > 0 {
		count := 0
		for i := 0; i < len(seeds) && count < c.Required[3]; i++ {
			if seeds[i] > 0 {
				seeds[i]--
				count++
			}
		}
		if count < c.Required[3] {
			return false
		}
	}

	if c.Required[4] > 0 {
		count := 0
		for _, s := range seeds {
			count += s
		}
		if count < c.Required[4] {
			return false
		}
	}

	return true
}

// Battle Force and Spell Force are the two Force skills ForceBuff reads a
// level from.
const (
	battleForceSkillID = 5104
	spellForceSkillID  = 5105
)

// ForceActor is what ForceBuff needs: the level of an active Force effect
// (battle or spell) on the effector, and whether one is active at all.
// Tracking Force levels is the not-yet-built effect engine's job; an
// effector that doesn't implement this reports neither Force as active.
type ForceActor interface {
	ForceLevel(skillID int) (level int, ok bool)
}

// ForceBuff requires the effector to have at least the given Battle Force
// and/or Spell Force level active (0 skips that requirement).
type ForceBuff struct {
	BattleForce int
	SpellForce  int
}

func (c ForceBuff) Test(effector, effected, skill any) bool {
	fa, hasForces := effector.(ForceActor)

	if c.BattleForce > 0 {
		level, ok := 0, false
		if hasForces {
			level, ok = fa.ForceLevel(battleForceSkillID)
		}
		if !ok || level < c.BattleForce {
			return false
		}
	}

	if c.SpellForce > 0 {
		level, ok := 0, false
		if hasForces {
			level, ok = fa.ForceLevel(spellForceSkillID)
		}
		if !ok || level < c.SpellForce {
			return false
		}
	}

	return true
}
