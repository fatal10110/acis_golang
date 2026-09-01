package creature

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

const (
	// RaidCurseHitTime is the MagicSkillUse hitTime the raid-curse
	// animation carries.
	RaidCurseHitTime = 300
	// RaidCurseSkillSeeRadius is the known-Attackable scan used by the
	// beneficial skill-see silence branch.
	RaidCurseSkillSeeRadius = 1000
	raidCurseLevelGap       = 8
	raidCurseSkillLevel     = 1
)

// MagicSkillUse is the domain snapshot a playable broadcasts when a raid
// curse animation fires. Caster is the raid-related Attackable; Target is
// the playable.
type MagicSkillUse struct {
	CasterID, TargetID  int32
	CasterAt, TargetAt  location.Location
	SkillID, Level      int32
	HitTime, ReuseDelay int
}

// RaidCurseSkills looks up loaded curse skill definitions.
type RaidCurseSkills interface {
	Definition(modelskill.Ref) (modelskill.Definition, bool)
}

// RaidCurseTarget is a raid-related Attackable that can apply a curse and
// drop the playable from its aggro list.
type RaidCurseTarget interface {
	attackable.Combatant

	Attackable() bool
	Level() int
	NpcID() int
	Dead() bool
	StopAggroHate(attackable.Combatant)
	Position() (int, int, int)
}

// RaidCurseAttacker is the playable receiving the curse decision.
type RaidCurseAttacker interface {
	attackable.Combatant

	Level() int
	EffectList() *effect.List
	Position() (int, int, int)
	Invul() bool
	Dead() bool
}

// RaidCurseInput is the playable attack-curse decision.
type RaidCurseInput struct {
	Attacker  RaidCurseAttacker
	Target    attackable.Combatant
	NPCID     int
	Mounted   bool
	Disabled  bool
	Skills    RaidCurseSkills
	Broadcast func(MagicSkillUse)
}

// RaidCurseSkillRaid is a known raid-related Attackable the beneficial
// skill-see branch consults for hate.
type RaidCurseSkillRaid interface {
	RaidCurseTarget
	RaidRelated() bool
	AggroHate(attackable.Combatant) float64
}

// RaidCurseSkillSeeTarget is one already-resolved skill target. Raid is
// set for a raid-related Attackable; Helped is set for a Playable.
type RaidCurseSkillSeeTarget struct {
	Raid   RaidCurseTarget
	Helped attackable.Combatant
}

// RaidCurseSkillInput is the playable skill-see curse decision.
type RaidCurseSkillInput struct {
	Caster    RaidCurseAttacker
	Offensive bool
	Debuff    bool
	Targets   []RaidCurseSkillSeeTarget
	Nearby    []RaidCurseSkillRaid
	Disabled  bool
	Skills    RaidCurseSkills
	Broadcast func(MagicSkillUse)
}

// SkillSeeTargetOf classifies t for the skill-see curse decision.
func SkillSeeTargetOf(t any, playable bool) RaidCurseSkillSeeTarget {
	var item RaidCurseSkillSeeTarget
	if raid, ok := t.(RaidCurseTarget); ok && raid.Attackable() {
		if related, ok := t.(interface{ RaidRelated() bool }); ok && related.RaidRelated() {
			item.Raid = raid
		}
	}
	if playable {
		if helped, ok := t.(attackable.Combatant); ok {
			item.Helped = helped
		}
	}
	return item
}

// TestCursesOnAttack applies raid petrification and mounted anti-strider
// curses. It reports true only when petrification freshly lands and must
// cancel the leftover physical hit.
func TestCursesOnAttack(in RaidCurseInput) bool {
	if in.Disabled || in.Attacker == nil {
		return false
	}
	target, ok := in.Target.(RaidCurseTarget)
	if !ok || !target.Attackable() {
		return false
	}

	if in.Attacker.Level()-target.Level() > raidCurseLevelGap {
		if blocked := applyRaidCurse(in, target, modelskill.RaidCurse2SkillID, true); blocked {
			return true
		}
	}

	if target.NpcID() == in.NPCID && in.Mounted {
		applyRaidCurse(in, target, modelskill.RaidAntiStriderSlowSkillID, false)
	}
	return false
}

// TestCursesOnSkillSee applies raid petrification on offensive/debuff
// casts and silence on beneficial casts. True means leftover skill
// effects must be skipped.
func TestCursesOnSkillSee(in RaidCurseSkillInput) bool {
	if in.Disabled || in.Caster == nil {
		return false
	}
	if in.Offensive || in.Debuff {
		for _, t := range in.Targets {
			if t.Raid == nil {
				continue
			}
			if in.Caster.Level()-t.Raid.Level() > raidCurseLevelGap {
				if applyRaidCurse(skillSeeAsAttack(in), t.Raid, modelskill.RaidCurse2SkillID, true) {
					return true
				}
			}
		}
		return false
	}
	if len(in.Nearby) == 0 {
		return false
	}
	for _, t := range in.Targets {
		if t.Helped == nil {
			continue
		}
		for _, raid := range in.Nearby {
			if raid == nil || !raid.Attackable() || !raid.RaidRelated() {
				continue
			}
			if in.Caster.Level()-raid.Level() <= raidCurseLevelGap {
				continue
			}
			if raid.AggroHate(t.Helped) <= 0 {
				continue
			}
			if applyRaidCurse(skillSeeAsAttack(in), raid, modelskill.RaidCurseSkillID, true) {
				return true
			}
		}
	}
	return false
}

func skillSeeAsAttack(in RaidCurseSkillInput) RaidCurseInput {
	return RaidCurseInput{
		Attacker:  in.Caster,
		Disabled:  in.Disabled,
		Skills:    in.Skills,
		Broadcast: in.Broadcast,
	}
}

func applyRaidCurse(in RaidCurseInput, target RaidCurseTarget, skillID modelskill.ID, stopHate bool) bool {
	if hasActiveSkill(in.Attacker.EffectList(), skillID) {
		return false
	}
	def, ok := lookupRaidCurse(in.Skills, skillID)
	if !ok {
		return false
	}
	broadcastRaidCurse(in, target, def)
	applyRaidCurseEffects(in.Attacker, target, def)
	if stopHate {
		target.StopAggroHate(in.Attacker)
		return true
	}
	return false
}

func lookupRaidCurse(skills RaidCurseSkills, id modelskill.ID) (modelskill.Definition, bool) {
	if skills == nil {
		return modelskill.Definition{}, false
	}
	return skills.Definition(modelskill.Ref{ID: id, Level: raidCurseSkillLevel})
}

func hasActiveSkill(list *effect.List, id modelskill.ID) bool {
	if list == nil {
		return false
	}
	_, ok := list.ActiveBySkillID(int(id))
	return ok
}

func broadcastRaidCurse(in RaidCurseInput, target RaidCurseTarget, def modelskill.Definition) {
	if in.Broadcast == nil {
		return
	}
	cx, cy, cz := target.Position()
	tx, ty, tz := in.Attacker.Position()
	in.Broadcast(MagicSkillUse{
		CasterID:   target.ObjectID(),
		TargetID:   in.Attacker.ObjectID(),
		CasterAt:   location.Location{X: cx, Y: cy, Z: cz},
		TargetAt:   location.Location{X: tx, Y: ty, Z: tz},
		SkillID:    int32(def.ID),
		Level:      int32(def.Level),
		HitTime:    RaidCurseHitTime,
		ReuseDelay: 0,
	})
}

func applyRaidCurseEffects(attacker RaidCurseAttacker, target RaidCurseTarget, def modelskill.Definition) {
	if def.Activation == modelskill.ActivationPassive || len(def.Effects) == 0 {
		return
	}
	if attacker.Dead() {
		return
	}
	if def.EffectRange > 0 {
		ax, ay, az := attacker.Position()
		tx, ty, tz := target.Position()
		if !location.In3DRange(tx, ty, tz, ax, ay, az, def.EffectRange) {
			return
		}
	}
	if (def.Offensive || def.Debuff) && attacker.Invul() {
		return
	}
	effector, ok := any(target).(effect.Participant)
	if !ok {
		return
	}
	effected, ok := any(attacker).(effect.Participant)
	if !ok {
		return
	}
	effect.Apply(attacker.EffectList(), effector, effected, effect.SkillFromDefinition(def), def.Effects)
}

// NPCIDOf returns target's NPC id when it exposes one, otherwise 0.
func NPCIDOf(target attackable.Combatant) int {
	if n, ok := target.(interface{ NpcID() int }); ok {
		return n.NpcID()
	}
	return 0
}
