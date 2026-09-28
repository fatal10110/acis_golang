package cast

import (
	"errors"

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
	// ErrSitting means the caster is seated, or playing dead and asked for
	// anything but the fake-death toggle itself.
	ErrSitting = errors.New("cast: sitting")
	// ErrSiegeSummonUnavailable means a siege-summon skill was requested by a
	// caster who is not an attacker of an active siege.
	ErrSiegeSummonUnavailable = errors.New("cast: siege summon outside siege")
)

// CanPlayerAttemptCast is a live player's pre-attempt gate: the checks that
// run on the raw request, before it is queued behind an in-flight swing,
// walks toward its target, or stops for its hit time. An active skill runs
// the shared CanAttemptCast checks and a toggle its own CanCastToggle
// checks; the player-only rules follow, then the unset-signet and
// siege-summon gates. Every skill request path of a player shares it.
func (c *Controller) CanPlayerAttemptCast(caster *player.Character, target Target, def modelskill.Definition) error {
	if caster == nil {
		return ErrInvalidTarget
	}
	var err error
	if def.Activation == modelskill.ActivationToggle {
		err = c.CanCastToggle(def)
	} else {
		err = c.canAttemptShared(target, def)
	}
	if err != nil {
		return err
	}
	if err := playerStateBlocksCast(caster, def); err != nil {
		return err
	}
	if err := c.groundTargetGate(def); err != nil {
		return err
	}
	// A duel side check belongs here once duels exist (#215): a caster in a
	// duel may not target a player from outside it (INVALID_TARGET).
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
// order the reference answers them.
func playerStateBlocksCast(caster *player.Character, def modelskill.Definition) error {
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
	if !caster.Standing() && !fakeDead {
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
