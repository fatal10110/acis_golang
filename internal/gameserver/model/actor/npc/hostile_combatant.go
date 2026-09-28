package npc

import (
	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
)

// Karma reports 0: NPCs carry no PK karma.
func (h *Hostile) Karma() int { return 0 }

// FakeDeath reports false: NPCs never feign death.
func (h *Hostile) FakeDeath() bool { return false }

// RecentFakeDeath reports false: NPCs never feign death.
func (h *Hostile) RecentFakeDeath() bool { return false }

// InPeaceZone reports whether the NPC's current position lies in a zone that
// raises the peace flag for an NPC. The NPC is not a zone occupant yet, so
// the zones are probed by position instead of read from a flag ledger.
func (h *Hostile) InPeaceZone() bool {
	if h.inPeace == nil {
		return false
	}
	return h.inPeace(h.location())
}

// SpawnProtected reports false: spawn protection is a player state.
func (h *Hostile) SpawnProtected() bool { return false }

// CanGiveDamage reports true: only access-level restrictions revoke damage,
// and NPCs have none.
func (h *Hostile) CanGiveDamage() bool { return true }

// Guard reports whether this NPC is a town Guard or a SiegeGuard. A helpful
// skill cast on a guard does not PvP-flag the caster.
func (h *Hostile) Guard() bool {
	kind := hostileKind(h.Instance)
	return kind == "Guard" || kind == "SiegeGuard"
}

// Owner reports no owner: NPCs are not summons.
func (h *Hostile) Owner() (attackable.Combatant, bool) { return nil, false }

// EffectRangeInPeaceZone reports false: NPC effects are never suppressed by
// peace zones.
func (h *Hostile) EffectRangeInPeaceZone(x, y, z, effectRange int) bool { return false }

// ShieldDefense reports ShieldFailed: NPCs carry no shield.
func (h *Hostile) ShieldDefense(creature.FormulaActor, modelskill.Definition, bool) formulas.ShieldDefense {
	return formulas.ShieldFailed
}

// TestCursesOnSkillSee reports false: raid skill-see curses target playable
// casters only.
func (h *Hostile) TestCursesOnSkillSee(modelskill.Definition, []skilltarget.Actor) bool { return false }

// NotePvPSkillTargets does nothing: NPCs take no part in PvP flagging.
func (h *Hostile) NotePvPSkillTargets([]attackable.Combatant, bool, string) {}

// OwnsOffensiveFollowTicker reports false: a hostile NPC's offensive follow
// is rechecked by its move controller.
func (h *Hostile) OwnsOffensiveFollowTicker() bool { return false }
