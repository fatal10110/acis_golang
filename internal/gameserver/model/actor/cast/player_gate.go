package cast

import (
	"errors"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// Player-only pre-attempt rejections. Each is answered before the request
// is queued behind a swing, starts a walk, or pays any cost.
var (
	// ErrFormalWear means the caster wears a full-body formal dress.
	ErrFormalWear = errors.New("cast: wearing formal wear")
	// ErrFishingSkillsOnly means a fishing caster asked for a non-fishing skill.
	ErrFishingSkillsOnly = errors.New("cast: only fishing skills while fishing")
	// ErrObserverMode means the caster is spectating.
	ErrObserverMode = errors.New("cast: observer mode")
	// ErrSitting means the caster is seated, or playing dead (the stand-up
	// out of fake death included) and asked for anything but the fake-death
	// toggle itself.
	ErrSitting = errors.New("cast: sitting")
	// ErrSiegeSummonUnavailable means a siege-summon skill was requested by a
	// caster who is not an attacker of an active siege.
	ErrSiegeSummonUnavailable = errors.New("cast: siege summon outside siege")
)

// CanPlayerAttemptCast is a live player's pre-attempt gate: the checks that
// run on the raw request, before it is queued behind an in-flight swing or
// cast, walks toward its target, or stops for its hit time. An active skill
// runs the shared skill checks and a toggle its own CanCastToggle checks;
// the player-only rules follow, then the unset-signet, duel-side and
// siege-summon gates. An in-flight cast is not a failure here: the caller queues a
// request that passes behind it. A caster still sitting down is not seated
// yet: the request waits out the sit-down and is refused once it ends.
// Every skill request path of a player shares it.
func (c *Controller) CanPlayerAttemptCast(caster *player.Character, target Target, def modelskill.Definition) error {
	if caster == nil || c.actor == nil || target == nil {
		return ErrInvalidTarget
	}
	var err error
	if def.Activation == modelskill.ActivationToggle {
		err = c.CanCastToggle(def)
	} else {
		err = c.canAttemptSkill(def)
	}
	if err != nil {
		return err
	}
	return c.playerAttemptRules(caster, target, def, caster.Seated())
}

// CanPlayerAttemptItemCast is the pre-attempt gate of an item-carried
// skill. It differs from CanPlayerAttemptCast only in that a toggle runs
// the shared skill checks rather than CanCastToggle's.
func (c *Controller) CanPlayerAttemptItemCast(caster *player.Character, target Target, def modelskill.Definition) error {
	if caster == nil || c.actor == nil || target == nil {
		return ErrInvalidTarget
	}
	if err := c.canAttemptSkill(def); err != nil {
		return err
	}
	return c.playerAttemptRules(caster, target, def, caster.Seated())
}

// playerAttemptRules is the player-only part of the pre-attempt gate;
// sitting is whether the caster counts as seated.
func (c *Controller) playerAttemptRules(caster *player.Character, target Target, def modelskill.Definition, sitting bool) error {
	if err := playerStateBlocksCast(caster, def, sitting); err != nil {
		return err
	}
	if err := c.groundTargetGate(def); err != nil {
		return err
	}
	if outsideCasterDuel(caster, target) {
		return ErrInvalidTarget
	}
	if def.SiegeSummonSkill && !caster.ActiveSiegeAttacker() {
		return ErrSiegeSummonUnavailable
	}
	// With sieges (#234), a siege attacker's summon is next refused inside a
	// castle zone (NOT_CALL_PET_FROM_THIS_LOCATION) and by a Dawn-held Seal
	// of Strife for a Dusk member (SEAL_OF_STRIFE_FORBIDS_SUMMONING). Neither
	// is reachable while no siege is ever active.
	return nil
}

// playerStateBlocksCast rejects a cast the caster's own state forbids, in the
// specified order.
func playerStateBlocksCast(caster *player.Character, def modelskill.Definition, sitting bool) error {
	if caster.WearingFormalWear() {
		return ErrFormalWear
	}
	if caster.Fishing() && !fishingSkill(def) {
		return ErrFishingSkillsOnly
	}
	if caster.ObserverMode() {
		return ErrObserverMode
	}
	fakeDead := caster.FakeDead()
	if sitting && !fakeDead {
		return ErrSitting
	}
	if fakeDead && def.ID != fakeDeathSkillID {
		return ErrSitting
	}
	return nil
}

func fishingSkill(def modelskill.Definition) bool {
	switch def.SkillType {
	case "FISHING", "PUMPING", "REELING":
		return true
	}
	return false
}

// outsideCasterDuel reports whether caster, in a duel, aims at a player, or
// a player's summon, from outside that duel.
func outsideCasterDuel(caster *player.Character, target Target) bool {
	if !caster.InDuel() {
		return false
	}
	t, ok := target.(interface {
		Kind() actor.Kind
		Owner() (attackable.Combatant, bool)
	})
	if !ok {
		return false
	}
	var acting any = t
	switch t.Kind() {
	case actor.KindPlayer:
	case actor.KindSummon:
		owner, ok := t.Owner()
		if !ok || owner == nil {
			return false
		}
		acting = owner
	default:
		return false
	}
	p, ok := acting.(interface{ DuelID() int32 })
	return ok && p.DuelID() != caster.DuelID()
}
