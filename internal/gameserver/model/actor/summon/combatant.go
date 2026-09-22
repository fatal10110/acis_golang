package summon

import (
	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
)

// Karma reports 0: a summon carries no karma of its own.
func (a *Actor) Karma() int { return 0 }

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
	if a.owner == nil {
		return nil, false
	}
	return a.owner, true
}

// CanSeeTarget reports true: the launch-phase line-of-sight gate is not wired
// for summon casts yet, so it never aborts one.
func (a *Actor) CanSeeTarget(skilltarget.Actor) bool { return true }

// ShieldDefense reports ShieldFailed: summons carry no shield.
func (a *Actor) ShieldDefense(creature.FormulaActor, modelskill.Definition, bool) formulas.ShieldDefense {
	return formulas.ShieldFailed
}

// RaceMultiplier reports 1: only NPC races scale damage.
func (a *Actor) RaceMultiplier(creature.FormulaActor) float64 { return 1 }

// TakeDamage reports false and applies nothing: auto-attack damage against
// summons is not wired yet.
func (a *Actor) TakeDamage(int, attackable.Combatant) bool { return false }

// BroadcastSkillUse does nothing: summon AI casts do not announce themselves
// to observers yet.
func (a *Actor) BroadcastSkillUse(int32, int, int, int, int32, int32, int, int) {}

// BroadcastSkillLaunched does nothing: summon AI casts do not announce
// themselves to observers yet.
func (a *Actor) BroadcastSkillLaunched(int32, int32, []int32) {}

// NotePvPSkillTargets does nothing: PvP flagging tracks the owning player.
func (a *Actor) NotePvPSkillTargets([]attackable.Combatant, bool, string) {}

// Flying reports false: summons never fly.
func (a *Actor) Flying() bool { return false }
