package effect

import modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"

// skillDOTTarget is a target whose hits name the skill that dealt them: a
// damage-over-time tick on it names the effect's skill.
type skillDOTTarget interface {
	ReduceHPBySkillDOT(damage float64, effector Actor, sk modelskill.Ref)
}

func damageOverTimeAction(e *Effect) bool {
	target := e.Effected
	result := DamageOverTimeTick(DamageOverTimeInput{
		Dead:      target.Dead(),
		HP:        target.HP(),
		Damage:    e.Template.Value,
		KillByDOT: e.Skill.KillByDOT,
		Toggle:    e.Skill.Toggle,
	})
	if result.RemovedForLackHP {
		if player, ok := asPlayer(target); ok {
			player.NotifyEffectRemovedDueLackHP(e)
		}
	}
	if result.Hit {
		// A skill's own damage-over-time tick is always isDOT=true, unlike
		// drowning's periodic damage, a plain (isDOT=false) HP reduction
		// routed through the same Go method (see taskeffects.go's Drown).
		// A toggle's own upkeep tick on a player is HP consumption, not a
		// hit: it leaves sleep, immobile-until-attacked and sitting alone.
		// Every other creature already skips those for any DOT tick.
		if player, ok := asPlayer(target); ok && e.Skill.Toggle {
			player.ReduceHPByToggleUpkeep(result.Damage, e.Effector)
		} else if named, ok := target.(skillDOTTarget); ok {
			named.ReduceHPBySkillDOT(result.Damage, e.Effector, modelskill.Ref{ID: e.Skill.ID, Level: e.Skill.Level})
		} else {
			target.ReduceHPByDOT(result.Damage, e.Effector, true)
		}
	}
	return result.Continue
}

func manaDamageOverTimeAction(e *Effect) bool {
	target := e.Effected
	result := ManaDamageOverTimeTick(ManaDamageOverTimeInput{
		Dead:   target.Dead(),
		MP:     target.MPValue(),
		Damage: e.Template.Value,
		Toggle: e.Skill.Toggle,
	})
	if result.RemovedForLackMP {
		if player, ok := asPlayer(target); ok {
			player.NotifyEffectRemovedDueLackMP(e)
		}
	}
	// An MP reduction that would apply zero is a no-op and skips the
	// broadcast, so no pointless packet is sent — gate on ReduceMP's returned
	// applied amount, not the requested tick damage, so an already-empty
	// target doesn't get a spurious broadcast.
	if result.Damage > 0 && target.ReduceMP(result.Damage) > 0 {
		broadcastStatus(e.Effected)
	}
	return result.Continue
}

func manaHealOverTimeAction(e *Effect) bool {
	target := e.Effected
	if !target.CanBeHealed() {
		return false
	}
	// Same bypass as the HP tick: a full-MP target applies 0 and the MP
	// setter — and its status update — never runs.
	if target.AddMP(e.Template.Value) > 0 {
		broadcastStatus(e.Effected)
	}
	return true
}

// manaDrainTick runs one ManaDamageOverTimeTick against e's target and
// applies its result, shared by relax, chameleon rest, fake death and silent
// move.
//
// Toggle is forced true regardless of e.Skill.Toggle: the relax and
// chameleon-rest effects check "cost exceeds current MP" unconditionally,
// not only for toggle skills (unlike the mana-damage-over-time effect, whose
// lack-MP check really is toggle-gated). Every skill carrying either effect in the
// current datapack happens to be TOGGLE-typed, so reading e.Skill.Toggle
// would produce the same result today — but that's a data coincidence, not
// a contract; force true here so a future non-toggle skill using these
// effects still gets the unconditional check.
func manaDrainTick(e *Effect) bool {
	target := e.Effected
	result := ManaDamageOverTimeTick(ManaDamageOverTimeInput{
		Dead:   target.Dead(),
		MP:     target.MPValue(),
		Damage: e.Template.Value,
		Toggle: true,
	})
	if result.RemovedForLackMP {
		if player, ok := asPlayer(target); ok {
			player.NotifyEffectRemovedDueLackMP(e)
		}
	}
	// See manaDamageOverTimeAction: gate the broadcast on ReduceMP's applied
	// amount, not the requested tick damage.
	if result.Damage > 0 && target.ReduceMP(result.Damage) > 0 {
		broadcastStatus(e.Effected)
	}
	return result.Continue
}

// DamageOverTimeInput is the state a periodic HP damage tick needs.
type DamageOverTimeInput struct {
	Dead      bool
	HP        float64
	Damage    float64
	KillByDOT bool
	Toggle    bool
}

// DamageOverTimeResult reports the effect of one periodic HP damage tick.
type DamageOverTimeResult struct {
	Damage float64
	// Hit reports that the tick reaches the target's HP reduction, a
	// zero-damage tick included: an NPC still registers the hit for it.
	Hit              bool
	Continue         bool
	RemovedForLackHP bool
}

// DamageOverTimeTick computes one periodic HP damage tick without mutating
// actor state.
func DamageOverTimeTick(in DamageOverTimeInput) DamageOverTimeResult {
	if in.Dead {
		return DamageOverTimeResult{}
	}

	damage := in.Damage
	if damage >= in.HP {
		if in.Toggle {
			return DamageOverTimeResult{RemovedForLackHP: true}
		}
		if !in.KillByDOT {
			if in.HP <= 1 {
				return DamageOverTimeResult{Continue: true}
			}
			damage = in.HP - 1
		}
	}
	return DamageOverTimeResult{Damage: damage, Hit: true, Continue: true}
}

// ManaDamageOverTimeInput is the state a periodic MP upkeep tick needs.
type ManaDamageOverTimeInput struct {
	Dead   bool
	MP     float64
	Damage float64
	Toggle bool
}

// ManaDamageOverTimeResult reports the effect of one periodic MP upkeep
// tick.
type ManaDamageOverTimeResult struct {
	Damage           float64
	Continue         bool
	RemovedForLackMP bool
}

// ManaDamageOverTimeTick computes one periodic MP upkeep tick without
// mutating actor state. A toggle skill whose upkeep strictly exceeds the
// available MP drops instead of draining below zero; every other
// mana-drain effect always pays its cost, however low that leaves MP.
// The strict inequality (not "at least") matters: a toggle whose upkeep
// exactly equals the remaining MP still pays it and keeps running.
func ManaDamageOverTimeTick(in ManaDamageOverTimeInput) ManaDamageOverTimeResult {
	if in.Dead {
		return ManaDamageOverTimeResult{}
	}
	if in.Toggle && in.Damage > in.MP {
		return ManaDamageOverTimeResult{RemovedForLackMP: true}
	}
	return ManaDamageOverTimeResult{Damage: in.Damage, Continue: true}
}
