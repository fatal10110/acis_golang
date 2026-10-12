package npc

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/grounditem"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// ScriptHooks is the script engine as an NPC and its spawner raise its
// hooks. Each method runs the hooks synchronously on the caller's
// goroutine, with no NPC lock held, except the dying ones, which only
// schedule theirs.
type ScriptHooks interface {
	// Behaves reports whether a behavior is bound to the NPC template id.
	// Such an NPC leaves its reactions to the behavior: the built-in
	// attacked hate, party assist, shot recharge and idle follow and wander
	// stay off.
	Behaves(npcID int32) bool
	// ReactsToCreated reports whether any script, a behavior or not,
	// reacts to the creation of an NPC with template id npcID. Such an NPC
	// walks only the routes its scripts ask for: the built-in walker route
	// stays off.
	ReactsToCreated(npcID int32) bool
	// HostileAttacked runs h's attacked hooks: attacker attacked it for
	// damage, with sk when a skill did it (the zero Ref otherwise).
	HostileAttacked(h *Hostile, attacker attackable.Combatant, damage int32, sk skill.Ref)
	// HostilePartyAttacked runs called's party-attacked hooks: caller,
	// attacked by target for damage, called its party member called.
	HostilePartyAttacked(caller PartyMember, called *Hostile, target attackable.Combatant, damage int32)
	// FolkAttacked runs f's attacked hooks: attacker attacked it for
	// damage, with sk when a skill did it (the zero Ref otherwise).
	FolkAttacked(f *Folk, attacker attackable.Combatant, damage int32, sk skill.Ref)
	// ClanAttacked runs called's clan-attacked hooks: caller, attacked by
	// attacker for damage, with sk when a skill did it, called its clan
	// member called. caller and called are each a *Hostile or a *Folk, and
	// are the same NPC for the caller's call to itself.
	ClanAttacked(caller, called, attacker attackable.Combatant, damage int32, sk skill.Ref)
	// HostilePartyDied runs called's party-died hooks: caller, a member of
	// called's party or called itself, has just died.
	HostilePartyDied(caller PartyMember, called *Hostile)
	// ClanDied runs called's clan-died hooks: caller, killed by killer,
	// told its clan member called. caller and called are each a *Hostile
	// or a *Folk.
	ClanDied(caller, called, killer attackable.Combatant)
	// SeesCreatures reports whether any script reacts to an NPC with
	// template id npcID seeing a creature. Only such an NPC scans its
	// surroundings.
	SeesCreatures(npcID int32) bool
	// HostileSeeCreature runs h's see-creature hooks: h noticed c.
	HostileSeeCreature(h *Hostile, c attackable.Combatant)
	// FolkSeeCreature runs f's see-creature hooks: f noticed c.
	FolkSeeCreature(f *Folk, c attackable.Combatant)
	// HostileNoDesire runs h's no-desire hooks: h has nothing left to do.
	HostileNoDesire(h *Hostile)
	// FolkNoDesire runs f's no-desire hooks: f has nothing left to do.
	FolkNoDesire(f *Folk)
	// HostileMoveToFinished runs h's move-finished hooks: a walk to a
	// point, or a flight, ended with h at x, y, z.
	HostileMoveToFinished(h *Hostile, x, y, z int32)
	// FolkMoveToFinished runs f's move-finished hooks: a walk to a point
	// ended with f at x, y, z.
	FolkMoveToFinished(f *Folk, x, y, z int32)
	// HostileOutOfTerritory runs h's out-of-territory hooks: h arrived
	// outside its territory, the first time since it was last inside.
	HostileOutOfTerritory(h *Hostile)
	// FolkOutOfTerritory runs f's out-of-territory hooks: f arrived
	// outside its territory, the first time since it was last inside.
	FolkOutOfTerritory(f *Folk)
	// HostileCreated runs h's created hooks: h has just entered the world.
	HostileCreated(h *Hostile)
	// FolkCreated runs f's created hooks: f has just entered the world.
	FolkCreated(f *Folk)
	// HostileDecayed runs h's decayed hooks once h has decayed, before it
	// leaves the world, then stops the behavior timers bound to h.
	HostileDecayed(h *Hostile)
	// FolkDecayed runs f's decayed hooks once f has decayed, before it
	// leaves the world, then stops the behavior timers bound to f.
	FolkDecayed(f *Folk)
	// HostileHidden stops the behavior timers bound to h, which a script
	// hid: it leaves the world with no decay of its own, and no hook runs.
	HostileHidden(h *Hostile)
	// HostileDying schedules h's dying hooks: killer has just killed it.
	HostileDying(h *Hostile, killer attackable.Combatant)
	// FolkDying schedules f's dying hooks: killer has just killed it.
	FolkDying(f *Folk, killer attackable.Combatant)
	// UseSkillFinished runs n's skill-finished hooks: n's cast of sk on
	// target ended, completed when success, aborted otherwise. n is a
	// *Hostile or a *Folk.
	UseSkillFinished(n, target attackable.Combatant, sk skill.Ref, success bool)
	// HostileAttackFinished runs h's attack-finished hooks: h's attack on
	// target ended.
	HostileAttackFinished(h *Hostile, target attackable.Combatant)
	// SeeSpell runs n's see-spell hooks: caster, a player, cast sk on
	// targets near n, through its summon when isPet. n is a *Hostile or a
	// *Folk.
	SeeSpell(n, caster attackable.Combatant, sk skill.Ref, targets []attackable.Combatant, isPet bool)
	// Spelled runs n's spelled hooks: sk, not offensive, landed on n from
	// caster, a player. n is a *Hostile or a *Folk.
	Spelled(n, caster attackable.Combatant, sk skill.Ref)
	// AbnormalStatusChanged runs the abnormal-status hooks of the scripts
	// that see spells on n: an effect of caster's sk is about to land on n.
	// caster is nil for a sourceless effect. n is a *Hostile or a *Folk.
	AbnormalStatusChanged(n, caster attackable.Combatant, sk skill.Ref)
	// SeeItem runs n's see-item hooks: n looked for ground items and found
	// items, with quantity. n is a *Hostile or a *Folk.
	SeeItem(n attackable.Combatant, quantity int32, items []*grounditem.Item)
	// PickedItem runs n's picked-item hooks: n picked up ground, which is
	// still in the world. n is a *Hostile or a *Folk.
	PickedItem(n attackable.Combatant, ground *grounditem.Item)
}

// behaves reports whether a behavior is bound to h's template id.
func (h *Hostile) behaves() bool {
	return h.scripts != nil && h.scripts.Behaves(int32(h.NpcID()))
}

// raiseAttacked runs h's attacked hooks.
func (h *Hostile) raiseAttacked(attacker attackable.Combatant, damage int32, sk skill.Ref) {
	if h.scripts != nil {
		h.scripts.HostileAttacked(h, attacker, damage, sk)
	}
}

// partyAttacked tells h that caller, its party member, was attacked by
// target for damage: h's party-attacked hooks run, then, when no behavior
// is bound to h, its built-in party assist. assist is false where the
// attack has no built-in assist at all (a skill's attacked call).
func (h *Hostile) partyAttacked(caller PartyMember, target attackable.Combatant, damage int, assist bool) {
	if h.scripts != nil {
		h.scripts.HostilePartyAttacked(caller, h, target, int32(damage))
	}
	if assist && !h.behaves() {
		h.reactPartyAttacked(caller, target, damage)
	}
}

// AtHookPoint is where h's AI loop gives the behavior bound to h's template
// its turn, with the AI loop unlocked: the see-creature scan, and the
// no-desire, move-finished and out-of-territory hooks run there.
func (h *Hostile) AtHookPoint(p ai.HookPoint) {
	if h.scripts == nil {
		return
	}
	switch p {
	case ai.HookSeeCreature:
		scanSight(h)
	case ai.HookNoDesire:
		h.scripts.HostileNoDesire(h)
	case ai.HookMoveFinished:
		x, y, z := h.Position()
		h.scripts.HostileMoveToFinished(h, int32(x), int32(y), int32(z))
	case ai.HookOutOfTerritory:
		h.scripts.HostileOutOfTerritory(h)
	}
}

// AtHookPoint is where f's AI pass, or its arrival, gives the behavior bound
// to f's template its turn, with the AI's brain released: the see-creature
// scan, and the no-desire, move-finished and out-of-territory hooks run
// there.
func (f *Folk) AtHookPoint(p ai.HookPoint) {
	if f.scripts == nil {
		return
	}
	switch p {
	case ai.HookSeeCreature:
		scanSight(f)
	case ai.HookNoDesire:
		f.scripts.FolkNoDesire(f)
	case ai.HookMoveFinished:
		x, y, z := f.Position()
		f.scripts.FolkMoveToFinished(f, int32(x), int32(y), int32(z))
	case ai.HookOutOfTerritory:
		f.scripts.FolkOutOfTerritory(f)
	}
}

// raiseSkillFinished runs n's skill-finished hooks for the cast end e: the
// target is the first creature the launch resolved, else the cast's own
// target.
func raiseSkillFinished(hooks ScriptHooks, n attackable.Combatant, e event.CastFinished) {
	if hooks == nil {
		return
	}
	target := e.LaunchTarget
	if target == nil {
		target = e.Target
	}
	hooks.UseSkillFinished(n, target, skill.Ref{ID: e.Skill.ID, Level: e.Skill.Level}, !e.Interrupted)
}

// AttackEnded runs h's attack-finished hooks: h's attack on target ended.
// The attack controller reports it ahead of the AI re-run that end causes.
func (h *Hostile) AttackEnded(target attackable.Combatant) {
	if h.scripts != nil {
		h.scripts.HostileAttackFinished(h, target)
	}
}

// SeeSpell runs h's see-spell hooks: caster, a player, cast sk on targets
// near h, through its summon when isPet.
func (h *Hostile) SeeSpell(caster attackable.Combatant, sk skill.Ref, targets []attackable.Combatant, isPet bool) {
	if h.scripts != nil {
		h.scripts.SeeSpell(h, caster, sk, targets, isPet)
	}
}

// Spelled runs h's spelled hooks: sk, not offensive, landed on h from
// caster, a player.
func (h *Hostile) Spelled(caster attackable.Combatant, sk skill.Ref) {
	if h.scripts != nil {
		h.scripts.Spelled(h, caster, sk)
	}
}

// AbnormalStatusChanged runs the abnormal-status hooks of the scripts that
// see spells on h: an effect of caster's sk is about to land on h.
func (h *Hostile) AbnormalStatusChanged(caster attackable.Combatant, sk skill.Ref) {
	if h.scripts != nil {
		h.scripts.AbnormalStatusChanged(h, caster, sk)
	}
}

// SeeSpell runs f's see-spell hooks: caster, a player, cast sk on targets
// near f, through its summon when isPet.
func (f *Folk) SeeSpell(caster attackable.Combatant, sk skill.Ref, targets []attackable.Combatant, isPet bool) {
	if f.scripts != nil {
		f.scripts.SeeSpell(f, caster, sk, targets, isPet)
	}
}

// Spelled runs f's spelled hooks: sk, not offensive, landed on f from
// caster, a player.
func (f *Folk) Spelled(caster attackable.Combatant, sk skill.Ref) {
	if f.scripts != nil {
		f.scripts.Spelled(f, caster, sk)
	}
}

// AbnormalStatusChanged runs the abnormal-status hooks of the scripts that
// see spells on f: an effect of caster's sk is about to land on f.
func (f *Folk) AbnormalStatusChanged(caster attackable.Combatant, sk skill.Ref) {
	if f.scripts != nil {
		f.scripts.AbnormalStatusChanged(f, caster, sk)
	}
}

// raiseDecayed runs h's decayed hooks.
func (h *Hostile) raiseDecayed() {
	if h.scripts != nil {
		h.scripts.HostileDecayed(h)
	}
}

// raiseDying schedules h's dying hooks.
func (h *Hostile) raiseDying(killer attackable.Combatant) {
	if h.scripts != nil {
		h.scripts.HostileDying(h, killer)
	}
}

// raiseDecayed runs f's decayed hooks.
func (f *Folk) raiseDecayed() {
	if f.scripts != nil {
		f.scripts.FolkDecayed(f)
	}
}

// raiseDying schedules f's dying hooks.
func (f *Folk) raiseDying(killer attackable.Combatant) {
	if f.scripts != nil {
		f.scripts.FolkDying(f, killer)
	}
}
