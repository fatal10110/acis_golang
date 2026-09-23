package cast

import (
	"fmt"

	handlerskill "github.com/fatal10110/acis_golang/internal/gameserver/handler/skill"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// CanCastToggle validates the pre-cast checks a toggle skill keeps: the
// blanket skill lock and its reuse-delay cooldown. A toggle skips every
// other check CanCast applies to an ordinary cast — MP/HP, mute state,
// required items — because those only ever apply when CastToggle activates
// it, never when it deactivates an already-running instance.
//
// Java routes toggle activation through the same entry point as any other
// skill (RequestMagicSkillUse.java:24-68 has no toggle branch; it always
// calls player.getAI().tryToCast), so a CC'd actor is rejected by
// PlayableAI.tryToCast's denyAiAction() check (PlayableAI.java:299-303)
// before the toggle skill's own logic runs — toggles get no exemption from
// the blanket lock in the reference.
func (c *Controller) CanCastToggle(def modelskill.Definition) error {
	if c.actor == nil {
		return ErrInvalidTarget
	}
	if def.Activation != modelskill.ActivationToggle {
		return fmt.Errorf("cast: skill %d level %d is not a toggle skill", def.ID, def.Level)
	}
	if c.actor.AllSkillsDisabled() {
		return ErrAllSkillsDisabled
	}
	if c.actor.SkillDisabled(ReuseKey(def)) {
		return ErrSkillDisabled
	}
	return nil
}

// CastToggle applies casting a toggle skill. alreadyActive reports whether
// the caller's live effect state currently holds a running instance of
// def's skill — resolving that lookup is the caller's job, matching a
// toggle skill's on/off rule: recasting an active toggle turns it off at
// no cost, while casting an inactive one activates it by paying its MP/HP
// cost up front. activated reports which branch ran; when true, the caller
// still owns applying the skill's actual effects, exactly as it would for
// any other successfully started cast.
//
// Unlike Start, activating a toggle never installs a reuse delay and has
// no separate Hit/Finish phase — its whole cost applies immediately.
//
// It does claim the Controller's casting state across those costs, because
// the abort funnel keys off that claim. The reference's doToggleCast opens
// with setCastTask, which sets _isCastingNow (PlayerCast.java:125,
// CreatureCast.java:636-645), so a cost that kills the caster reaches
// doDie -> abortAll(true) -> stop() while the cast still counts as in
// flight, and the client is sent MagicSkillCanceled ahead of its Die.
// Without the claim the funnel finds no cast and sends only the
// acknowledgement, leaving a toggle that kills you silent where one you
// cannot afford cancels cleanly.
//
// The claim is released as soon as the costs are paid — a toggle has no
// hit or cool phase to hold it open, matching onMagicFinalizer being
// scheduled at zero delay (PlayerCast.java:173) — and released silently,
// so a toggle that does not kill its caster emits exactly the events it
// did before. A cast already in flight keeps its own claim; the funnel
// reports for that one.
//
// Deactivating a running toggle deliberately does not claim, though
// setCastTask is the first statement of doToggleCast and so claims on both
// branches: that branch only stops the effect, which cannot kill the
// caster, so there is no abort for the funnel to report and the narrower
// claim is unobservable.
//
// MP is checked and paid before HP is checked at all: a toggle that has
// enough MP but not enough HP still loses the MP, uncredited, when
// activation then fails on the HP check. This mirrors the exact order the
// two costs are validated in and is not a transactional all-or-nothing
// charge.
func (c *Controller) CastToggle(alreadyActive bool, def modelskill.Definition) (activated bool, err error) {
	if err := c.CanCastToggle(def); err != nil {
		return false, err
	}
	if alreadyActive {
		return false, nil
	}

	seq, claimed := c.claimToggle(def)
	if claimed {
		defer c.releaseToggle(seq)
	}

	if mp := c.actor.MPCost(def); mp > 0 {
		if mp > c.actor.MP() {
			return false, ErrNotEnoughMP
		}
		c.actor.ReduceMP(mp)
	}
	if hp := def.HPConsume; hp > 0 {
		if hp > c.actor.HP() {
			return false, ErrNotEnoughHP
		}
		c.actor.ReduceHP(hp)
	}
	return true, nil
}

// ApplyToggle resolves req's toggle skill and decides whether it activates
// or deactivates based on the caster's current effect state, driving that
// decision through controller. It does not apply or remove the skill's
// effects on either branch — matching the reference's send-then-apply cast
// sequencing (PlayerCast.doToggleCast broadcasts MagicSkillUse at
// PlayerCast.java:127 before either callSkill's effect application or
// effect.exit() at PlayerCast.java:135-137, on both the activation and
// deactivation paths), the caller sends its cast-acknowledgment packet
// first and only then calls ApplyEffects with the returned def and target
// when activated is true, or StopEffect(req.Caster, def.ID) when it is
// false and err is nil. The on/off rule CastToggle documents lives
// entirely inside this package, not in whatever is decoding the request.
//
// ack, when non-nil, sends the cast acknowledgment and runs before the
// costs rather than after them, matching doToggleCast broadcasting
// MagicSkillUse at PlayerCast.java:127 ahead of the MP/HP consume at
// :139-165. The order only becomes observable once a cost can kill: the
// death packets it produces would otherwise reach the client ahead of the
// acknowledgment for the cast that caused them.
//
// stopMovement, when non-nil, runs only after target resolution and
// CanCastToggle both pass — matching PlayerAI.thinkCast, which reaches its
// unconditional getMove().stop() (PlayerAI.java:273-276) only after
// denyAiAction()/isCastingNow() (PlayableAI.java:299-303, PlayerAI.java:219-241)
// and the reuse-delay gate in canAttemptCast. A rejected toggle — blanket
// lock, on cooldown, dead, unknown skill — must never stop a walk that
// Java leaves running. It reports nothing, matching Java's stop, which
// cannot fail at all.
func ApplyToggle(handlers EffectHandlers, controller *Controller, req PlayerToggleRequest, stopMovement func(), ack func(modelskill.Definition)) (def modelskill.Definition, target Target, activated bool, err error) {
	def, target, err = ResolvePlayerToggle(req)
	if err != nil {
		return def, target, false, err
	}
	if err := controller.CanCastToggle(def); err != nil {
		return def, target, false, err
	}

	if stopMovement != nil {
		stopMovement()
	}

	if ack != nil {
		ack(def)
	}

	alreadyActive := handlerskill.ActiveEffect(req.Caster, def.ID)
	activated, err = controller.CastToggle(alreadyActive, def)
	if err != nil {
		return def, target, false, err
	}

	return def, target, activated, nil
}
