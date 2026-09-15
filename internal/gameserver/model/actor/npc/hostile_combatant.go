package npc

import (
	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
)

// Karma reports 0: NPCs carry no PK karma.
func (h *Hostile) Karma() int { return 0 }

// FakeDeath reports false: NPCs never feign death.
func (h *Hostile) FakeDeath() bool { return false }

// RecentFakeDeath reports false: NPCs never feign death.
func (h *Hostile) RecentFakeDeath() bool { return false }

// InPeaceZone reports false: NPC peace-zone membership is not tracked, so an
// NPC is never shielded by one.
func (h *Hostile) InPeaceZone() bool { return false }

// SilentMoving reports false: NPCs never move silently.
func (h *Hostile) SilentMoving() bool { return false }

// SpawnProtected reports false: spawn protection is a player state.
func (h *Hostile) SpawnProtected() bool { return false }

// CanGiveDamage reports true: only access-level restrictions revoke damage,
// and NPCs have none.
func (h *Hostile) CanGiveDamage() bool { return true }

// Guard reports false: town-guard classification is not modeled yet.
func (h *Hostile) Guard() bool { return false }

// Owner reports no owner: NPCs are not summons.
func (h *Hostile) Owner() (attackable.Combatant, bool) { return nil, false }

// EffectRangeInPeaceZone reports false: NPC effects are never suppressed by
// peace zones.
func (h *Hostile) EffectRangeInPeaceZone(x, y, z, effectRange int) bool { return false }

// ShieldDefense reports ShieldFailed: NPCs carry no shield.
func (h *Hostile) ShieldDefense(attackable.Combatant, modelskill.Definition, bool) formulas.ShieldDefense {
	return formulas.ShieldFailed
}

// TestCursesOnSkillSee reports false: raid skill-see curses target playable
// casters only.
func (h *Hostile) TestCursesOnSkillSee(modelskill.Definition, []skilltarget.Actor) bool { return false }

// NotePvPSkillTargets does nothing: NPCs take no part in PvP flagging.
func (h *Hostile) NotePvPSkillTargets([]attackable.Combatant, bool, string) {}
