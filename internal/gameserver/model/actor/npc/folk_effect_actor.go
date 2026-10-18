package npc

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/conditions"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// EffectList returns the buffs and debuffs the NPC holds.
func (f *Folk) EffectList() *effect.List { return f.effects }

// Paralyzed reports whether an effect paralyzes the NPC.
func (f *Folk) Paralyzed() bool { return f.effects.IsAffected(effect.FlagParalyzed) }

// Afraid reports whether an effect fears the NPC.
func (f *Folk) Afraid() bool { return f.effects.IsAffected(effect.FlagFear) }

// SetImmobilized sets the movement lock and reports whether it changed.
func (f *Folk) SetImmobilized(v bool) bool { return f.immobilized.Swap(v) != v }

// CancelVulnerability returns the cancel vulnerability multiplier.
func (f *Folk) CancelVulnerability(string) float64 { return f.CalcStat(stat.CancelVuln, 1) }

// StopEffects removes every effect of type t the NPC holds.
func (f *Folk) StopEffects(t effect.Type) { f.effects.StopByType(t) }

// StopSkillEffectsByID removes every effect skill id applied to the NPC.
func (f *Folk) StopSkillEffectsByID(id modelskill.ID) { f.effects.StopBySkillID(id) }

// AddChanceTrigger registers a started chance-skill trigger.
func (f *Folk) AddChanceTrigger(e *effect.Effect) { f.effects.AddChanceTrigger(e) }

// RemoveChanceTrigger drops an exiting chance-skill trigger.
func (f *Folk) RemoveChanceTrigger(e *effect.Effect) { f.effects.RemoveChanceTrigger(e) }

// AbortAll stops the NPC's walk, its swing in flight and its cast in
// flight. The NPC keeps no selected target to reset.
func (f *Folk) AbortAll(bool) { f.abortAll() }

// StopAttack aborts the NPC's swing in flight, if any.
func (f *Folk) StopAttack() { f.stopAttack() }

// The NPC keeps no selected target, and the effects that stop, redirect,
// frighten, bluff or throw a creature never land on it, so the hooks below
// that only those reach do nothing. FearImmune and BluffExempt report the
// civilian NPC's own immunity.
func (f *Folk) StopMove()                                  {}
func (f *Folk) TryToIdle()                                 {}
func (f *Folk) ClearTarget()                               {}
func (f *Folk) FearImmune() bool                           { return true }
func (f *Folk) FleeFrom(effect.Actor, int)                 {}
func (f *Folk) BluffExempt() bool                          { return true }
func (f *Folk) FlyTo(location.Location, modelskill.Flight) {}
func (f *Folk) BroadcastPosition()                         {}
func (f *Folk) CurrentTarget() world.Tracked               { return nil }
func (f *Folk) SetTarget(world.Tracked)                    {}
func (f *Folk) AttackTarget(world.Tracked)                 {}

// ValidLocation returns the destination unchanged: the NPC is never
// thrown.
func (f *Folk) ValidLocation(_, _, _, tx, ty, tz int) location.Location {
	return location.Location{X: tx, Y: ty, Z: tz}
}

// SetXYZ moves the NPC's world presence to (x, y, z), a position set
// outside movement.
func (f *Folk) SetXYZ(x, y, z int) {
	if f.world != nil {
		_ = f.world.Move(f, x, y, z)
		f.zones.place()
	}
}

// The skills and effects that reach these aggro and overhit controls act
// only on an attackable NPC's lists, so they do nothing here: a civilian
// NPC's aggro list is its AI's alone (see folk_fight.go).
func (f *Folk) NotifyAggression(attackable.Combatant, int) {}
func (f *Folk) ReduceAllAggroHate(float64)                 {}
func (f *Folk) StopAggroHate(attackable.Combatant)         {}
func (f *Folk) StopHateList(attackable.Combatant)          {}
func (f *Folk) ClearAggroTables()                          {}
func (f *Folk) EnableOverhit()                             {}

// SkillSuccessInput resolves an effect-landing roll of caster's skill
// against the NPC.
func (f *Folk) SkillSuccessInput(caster creature.FormulaActor, def modelskill.Definition, bss bool, shield formulas.ShieldDefense) (formulas.SkillSuccessInput, bool) {
	return creature.ResolveSkillSuccessInput(caster, f, def, bss, shield)
}

// EffectSuccessInput resolves the landing roll of one effect template of
// caster's skill against the NPC.
func (f *Folk) EffectSuccessInput(caster creature.FormulaActor, def modelskill.Definition, tmpl modelskill.EffectTemplate, bss bool, shield formulas.ShieldDefense) (formulas.SkillSuccessInput, bool) {
	return creature.ResolveEffectSuccessInput(caster, f, def, tmpl, bss, shield)
}

// SkillReflectInput resolves the NPC's chance to reflect def.
func (f *Folk) SkillReflectInput(def modelskill.Definition) formulas.SkillReflectInput {
	reflectStat := stat.ReflectSkillPhysic
	if def.Magic {
		reflectStat = stat.ReflectSkillMagic
	}
	return formulas.SkillReflectInput{
		IgnoreResists:  def.IgnoreResists,
		CanBeReflected: def.CanBeReflected,
		Magic:          def.Magic,
		CastRange:      def.CastRange,
		ReflectChance:  f.calcFor(reflectStat, 0, nil, conditions.SkillOf(def.Stat)),
	}
}

// ShieldDefense reports ShieldFailed: NPCs carry no shield.
func (f *Folk) ShieldDefense(creature.FormulaActor, modelskill.Definition, bool) formulas.ShieldDefense {
	return formulas.ShieldFailed
}

// RaceMultiplier returns the attacker's race attack over the NPC's race
// resistance, or 1 for a race without that pair.
func (f *Folk) RaceMultiplier(attacker creature.FormulaActor) float64 {
	if attacker == nil {
		return 1
	}
	atk, res, ok := raceStats(f.Instance.Template.Race)
	if !ok {
		return 1
	}
	self := f.ConditionActor()
	return 1 + ((attacker.Against(self, nil).CalcStat(atk, 1) - f.calcFor(res, 1, self, nil)) / 100)
}

// PhysicalSkillInput resolves a physical skill's damage against the NPC.
func (f *Folk) PhysicalSkillInput(caster creature.FormulaActor, def modelskill.Definition) (formulas.PhysicalSkillInput, bool) {
	return creature.ResolvePhysicalSkillInput(caster, f, def, false, f.RaceMultiplier(caster))
}

// MagicDamageInput resolves a magic skill's damage against the NPC.
func (f *Folk) MagicDamageInput(caster creature.FormulaActor, def modelskill.Definition, magicFailures bool) (formulas.MagicDamageInput, bool) {
	return creature.ResolveMagicDamageInput(caster, f, def, false, magicFailures)
}

// BlowInput resolves a blow skill's damage against the NPC.
func (f *Folk) BlowInput(caster creature.FormulaActor, def modelskill.Definition) (formulas.BlowInput, bool) {
	return creature.ResolveBlowInput(caster, f, def, false)
}

// ManaDamageInput resolves a mana-burn skill against the NPC.
func (f *Folk) ManaDamageInput(caster creature.FormulaActor, def modelskill.Definition) (formulas.ManaDamageInput, bool) {
	return creature.ResolveManaDamageInput(caster, f, f.MaxMPValue(), def)
}

// CounterSkillPhysical returns the NPC's physical skill counter rate.
func (f *Folk) CounterSkillPhysical() float64 {
	return f.calcFor(stat.CounterSkillPhysical, 0, f.ConditionActor(), nil)
}

// LethalInput resolves a lethal-strike roll against the NPC.
func (f *Folk) LethalInput(caster creature.FormulaActor, def modelskill.Definition) (formulas.LethalInput, bool) {
	if caster == nil || f.Invul() || !creature.CanDealDamage(caster) {
		return formulas.LethalInput{}, false
	}
	return formulas.LethalInput{
		Chance1:       def.LethalChance1,
		Chance2:       def.LethalChance2,
		MagicLevel:    def.MagicLevel,
		AttackerLevel: caster.Level(),
		TargetLevel:   f.Level(),
		LethalMul:     caster.Against(f.ConditionActor(), nil).CalcStat(stat.LethalRate, 1),
	}, true
}

// ApplyLethalOutcome applies a lethal-strike tier to the NPC.
func (f *Folk) ApplyLethalOutcome(outcome formulas.LethalOutcome, caster attackable.Combatant, def modelskill.Definition) {
	switch outcome {
	case formulas.LethalFull:
		f.reduceHP(f.HP()-1, caster, skillRef(def))
	case formulas.LethalHalf:
		f.reduceHP(f.HP()/2, caster, skillRef(def))
	}
}

// Lethalable reports whether a lethal strike can land on the NPC: on any
// civilian NPC but a siege headquarters.
func (f *Folk) Lethalable() bool { return !f.SiegeFlag() }

// SpoilPool reports none: a civilian NPC has nothing to sweep.
func (f *Folk) SpoilPool() *item.SpoilPool { return nil }

// SeedState reports none: a civilian NPC is never sown.
func (f *Folk) SeedState() *SeedState { return nil }
