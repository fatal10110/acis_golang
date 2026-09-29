package summon

import (
	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
)

// Karma reports the owner's karma: a summon carries none of its own.
func (a *Actor) Karma() int {
	owner := a.currentOwner()
	if owner == nil {
		return 0
	}
	return owner.Karma()
}

// PvPFlagState reports the owner's PvP flag: a summon carries none of its
// own.
func (a *Actor) PvPFlagState() task.PvPFlagState {
	owner := a.currentOwner()
	if owner == nil {
		return task.PvPFlagNone
	}
	return owner.PvPFlagState()
}

// FakeDeath reports false: summons never feign death.
func (a *Actor) FakeDeath() bool { return false }

// RecentFakeDeath reports false: summons never feign death.
func (a *Actor) RecentFakeDeath() bool { return false }

// SilentMoving reports false: summons never move silently.
func (a *Actor) SilentMoving() bool { return false }

// SpawnProtected reports false: spawn protection is a player state.
func (a *Actor) SpawnProtected() bool { return false }

// RaidRelated reports false: summons are never raid bosses or minions.
func (a *Actor) RaidRelated() bool { return false }

// Guard reports false: summons are never town guards.
func (a *Actor) Guard() bool { return false }

// Owner returns the summon's controlling player.
func (a *Actor) Owner() (attackable.Combatant, bool) {
	owner := a.currentOwner()
	if owner == nil {
		return nil, false
	}
	return owner, true
}

// CanSeeTarget reports whether t is visible to this summon for the cast
// pipeline's line-of-sight gates: the same geodata query as
// CanSee, keyed to t's own eye height, or permissive when no query is
// attached.
func (a *Actor) CanSeeTarget(t skilltarget.Actor) bool {
	tx, ty, tz := t.Position()
	return a.canSeeObject(t, tx, ty, tz, t.CollisionHeight())
}

// ShieldDefense reports ShieldFailed: summons carry no shield.
func (a *Actor) ShieldDefense(creature.FormulaActor, modelskill.Definition, bool) formulas.ShieldDefense {
	return formulas.ShieldFailed
}

// RaceMultiplier reports 1: only NPC races scale damage.
func (a *Actor) RaceMultiplier(creature.FormulaActor) float64 { return 1 }

// NotePvPSkillTargets does nothing: PvP flagging tracks the owning player.
func (a *Actor) NotePvPSkillTargets([]attackable.Combatant, bool, string) {}

// Flying reports false: summons never fly.
func (a *Actor) Flying() bool { return false }
