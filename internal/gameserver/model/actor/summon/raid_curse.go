package summon

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

var _ creature.RaidCurseAttacker = (*Actor)(nil)

// SetRaidCursesDisabled records the npcs.properties DisableRaidCurse gate.
func (a *Actor) SetRaidCursesDisabled(disabled bool) {
	if a == nil {
		return
	}
	a.statusMu.Lock()
	defer a.statusMu.Unlock()
	a.raidCursesDisabled = disabled
}

// TestCursesOnAttack applies this summon's raid petrification curse against
// target. Mounted anti-strider never applies to a summon. True means the
// leftover physical hit must be cancelled.
func (a *Actor) TestCursesOnAttack(target attackable.Combatant) bool {
	if a == nil {
		return false
	}
	a.statusMu.RLock()
	disabled := a.raidCursesDisabled
	a.statusMu.RUnlock()
	return creature.TestCursesOnAttack(creature.RaidCurseInput{
		Attacker:  a,
		Target:    target,
		NPCID:     creature.NPCIDOf(target),
		Mounted:   false,
		Disabled:  disabled,
		Skills:    a.skillDefs,
		Broadcast: a.broadcastMagicSkillUse,
	})
}

// TestCursesOnSkillSee applies this summon's raid petrification or silence
// curses against the resolved skill targets. True means leftover skill
// effects must be skipped.
func (a *Actor) TestCursesOnSkillSee(def modelskill.Definition, targets []skilltarget.Creature) bool {
	if a == nil {
		return false
	}
	a.statusMu.RLock()
	disabled := a.raidCursesDisabled
	a.statusMu.RUnlock()

	converted := make([]creature.RaidCurseSkillSeeTarget, 0, len(targets))
	for _, t := range targets {
		playable := t != nil && t.Category().Has(skilltarget.CategoryPlayable)
		converted = append(converted, creature.SkillSeeTargetOf(t, playable))
	}
	var nearby []creature.RaidCurseSkillRaid
	if !def.Offensive && !def.Debuff {
		a.ForEachKnownCombatantInRadius(creature.RaidCurseSkillSeeRadius, func(candidate attackable.Combatant) {
			raid, ok := candidate.(creature.RaidCurseSkillRaid)
			if !ok || !raid.Attackable() || !raid.RaidRelated() {
				return
			}
			nearby = append(nearby, raid)
		})
	}
	return creature.TestCursesOnSkillSee(creature.RaidCurseSkillInput{
		Caster:    a,
		Offensive: def.Offensive,
		Debuff:    def.Debuff,
		Targets:   converted,
		Nearby:    nearby,
		Disabled:  disabled,
		Skills:    a.skillDefs,
		Broadcast: a.broadcastMagicSkillUse,
	})
}

func (a *Actor) broadcastMagicSkillUse(use creature.MagicSkillUse) {
	a.BroadcastSkillUse(use.CasterID, use.CasterAt, use.TargetID, use.TargetAt, use.SkillID, use.Level, use.HitTime, use.ReuseDelay)
}

// BroadcastSkillUse sends the cast-start animation of skillID from caster to
// target. A nil builder or world is a silent no-op.
func (a *Actor) BroadcastSkillUse(casterID int32, casterAt location.Location, targetID int32, targetAt location.Location, skillID, level int32, hitTime, reuseDelay int) {
	a.broadcast(func() wire.Frame {
		return a.frames.SkillUse(casterID, casterAt, targetID, targetAt, skillID, level, hitTime, reuseDelay, false)
	})
}
