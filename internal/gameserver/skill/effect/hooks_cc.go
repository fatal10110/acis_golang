package effect

import (
	"math"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

func stunStart(e *Effect) bool {
	abortAll(e.Effected)
	if target, ok := e.Effected.(idleTarget); ok {
		target.TryToIdle()
	}
	refresh(e.Effected)
	return true
}

func rootStart(e *Effect) bool {
	if target, ok := e.Effected.(moveStopper); ok {
		target.StopMove()
	}
	refresh(e.Effected)
	return true
}

func sleepStart(e *Effect) bool {
	abortAll(e.Effected)
	refresh(e.Effected)
	return true
}

func fearStart(e *Effect) bool {
	if isPlayable(e.Effected) && fearHalvedDurationPlayableSkillIDs[e.Skill.ID] {
		e.Template.Count /= 2
	}
	if fearImmune(e.Effected) || isAfraid(e.Effected) {
		return false
	}
	if isPlayable(e.Effected) && fearSkippedPlayableSkillIDs[e.Skill.ID] {
		return false
	}

	abortAll(e.Effected)
	refresh(e.Effected)
	return fearAction(e)
}

func fearAction(e *Effect) bool {
	target, ok := e.Effected.(fleeTarget)
	if !ok {
		return false
	}
	target.FleeFrom(e.Effector, 500)
	return true
}

func fearExit(e *Effect) {
	if target, ok := e.Effected.(effectStopper); ok {
		target.StopEffects(TypeFear)
	}
	refresh(e.Effected)
}

func thinkAndRefreshExit(e *Effect) {
	if target, ok := e.Effected.(thinkTarget); ok {
		_ = target.Think()
	}
	refresh(e.Effected)
}

func refreshExit(e *Effect) {
	refresh(e.Effected)
}

func abortCastStart(e *Effect) bool {
	if e.Effected == nil || e.Effected == e.Effector {
		return false
	}
	if rt, ok := e.Effected.(raidTarget); ok && rt.RaidRelated() {
		return false
	}
	if target, ok := e.Effected.(castInterrupter); ok && target.CastingNow() {
		target.InterruptCast()
	}
	return true
}

func immobileUntilAttackedStart(e *Effect) bool {
	abortAll(e.Effected)
	refresh(e.Effected)
	return true
}

func immobileUntilAttackedExit(e *Effect) {
	if target, ok := e.Effected.(skillIDEffectStopper); ok {
		target.StopSkillEffectsByID(e.Skill.ID)
	}
	if target, ok := e.Effected.(thinkTarget); ok {
		_ = target.Think()
	}
	refresh(e.Effected)
}

// immobileUntilAttackedAction always ends the effect on its first tick; an
// early trigger (e.g. the target taking damage) is expected to reschedule
// this tick sooner, not something this hook decides on its own.
func immobileUntilAttackedAction(e *Effect) bool {
	immobileUntilAttackedExit(e)
	return false
}

func immobilizeEffectorStart(e *Effect) bool {
	if target, ok := e.Effector.(immobilizeTarget); ok {
		target.SetImmobilized(true)
	}
	return true
}

func immobilizeEffectorExit(e *Effect) {
	if target, ok := e.Effector.(immobilizeTarget); ok {
		target.SetImmobilized(false)
	}
}

func invincibleStart(e *Effect) bool {
	if target, ok := e.Effected.(invulnerabilityTarget); ok {
		target.SetInvul(true)
	}
	return true
}

func invincibleExit(e *Effect) {
	if target, ok := e.Effected.(invulnerabilityTarget); ok {
		target.SetInvul(false)
	}
}

func muteStart(e *Effect) bool {
	if target, ok := e.Effected.(magicCastTarget); ok && target.CastingNow() && target.CurrentSkillIsMagic() {
		target.StopCast()
	}
	refresh(e.Effected)
	return true
}

func physicalMuteStart(e *Effect) bool {
	if target, ok := e.Effected.(magicCastTarget); ok && target.CastingNow() && !target.CurrentSkillIsMagic() {
		target.StopCast()
	}
	refresh(e.Effected)
	return true
}

func paralyzeStart(e *Effect) bool {
	startAbnormalEffect(e.Effected, 0x000400)
	abortAll(e.Effected)
	return true
}

func paralyzeExit(e *Effect) {
	stopAbnormalEffect(e.Effected, 0x000400)
	if target, ok := e.Effected.(thinkTarget); ok {
		_ = target.Think()
	}
}

func petrificationStart(e *Effect) bool {
	startAbnormalEffect(e.Effected, 0x000800)
	abortAll(e.Effected)
	if target, ok := e.Effected.(invulnerabilityTarget); ok {
		target.SetInvul(true)
	}
	return true
}

func petrificationExit(e *Effect) {
	stopAbnormalEffect(e.Effected, 0x000800)
	if target, ok := e.Effected.(thinkTarget); ok {
		_ = target.Think()
	}
	if target, ok := e.Effected.(invulnerabilityTarget); ok {
		target.SetInvul(false)
	}
}

func removeTargetStart(e *Effect) bool {
	if target, ok := e.Effected.(targetClearer); ok {
		target.ClearTarget()
		target.StopAttack()
	}
	if target, ok := e.Effected.(castStopper); ok {
		target.StopCast()
	}
	return true
}

func silenceAllStart(e *Effect) bool {
	if target, ok := e.Effected.(castStopper); ok {
		target.StopCast()
	}
	refresh(e.Effected)
	return true
}

func silentMoveAction(e *Effect) bool {
	if e.Skill.SkillType != "CONT" {
		return false
	}
	target, ok := e.Effected.(mpDotTarget)
	if !ok {
		return false
	}
	result := ManaDamageOverTimeTick(ManaDamageOverTimeInput{
		Dead:   target.Dead(),
		MP:     target.MPValue(),
		Damage: e.Template.Value,
		Toggle: true,
	})
	if result.RemovedForLackMP {
		if notifier, ok := e.Effected.(lackMPNotifier); ok {
			notifier.NotifyEffectRemovedDueLackMP(e)
		}
	}
	// See manaDamageOverTimeAction: gate the broadcast on ReduceMP's applied
	// amount, not the requested tick damage.
	if result.Damage > 0 && target.ReduceMP(result.Damage) > 0 {
		broadcastMPStatus(e.Effected)
	}
	return result.Continue
}

func stunSelfStart(e *Effect) bool {
	if p, ok := e.Effected.(playableTarget); ok && p.Playable() {
		if target, ok := e.Effected.(idleTarget); ok {
			target.TryToIdle()
		}
	}
	refresh(e.Effector)
	return true
}

func stunSelfExit(e *Effect) {
	refresh(e.Effector)
}

func immobilizePetBuffStart(e *Effect) bool {
	if !isPlayer(e.Effector) {
		return false
	}
	player, ok := e.Effector.(objectIDTarget)
	if !ok {
		return false
	}
	summon, ok := e.Effected.(summonOwnerTarget)
	if !ok || summon.OwnerID() != player.ObjectID() {
		return false
	}
	target, ok := e.Effected.(immobilizeTarget)
	if !ok {
		return false
	}
	target.SetImmobilized(true)
	return true
}

func immobilizePetBuffExit(e *Effect) {
	if target, ok := e.Effected.(immobilizeTarget); ok {
		target.SetImmobilized(false)
	}
}

// fakeDeathStart puts the target in the seated transition Fake Death
// reuses for its lie-down animation; it always reports success.

func throwUpStart(e *Effect) bool {
	abortAll(e.Effected)

	source, ok := e.Effector.(flightPosition)
	if !ok {
		return false
	}
	target, ok := e.Effected.(flightPosition)
	if !ok {
		return false
	}
	mover, ok := e.Effected.(flightMover)
	if !ok {
		return false
	}

	ox, oy, oz := target.X(), target.Y(), target.Z()
	dx := float64(source.X() - ox)
	dy := float64(source.Y() - oy)
	distance := math.Sqrt(dx*dx + dy*dy)
	if distance < 1 || distance > 2000 {
		return false
	}

	offset := float64(min(int(distance)+e.Skill.FlyRadius, 1400))
	offset += math.Abs(float64(source.Z() - oz))
	if offset < 5 {
		offset = 5
	}

	x := source.X() - int(offset*(dx/distance))
	y := source.Y() - int(offset*(dy/distance))
	z := oz

	if resolver, ok := e.Effected.(flightResolver); ok {
		valid := resolver.ValidLocation(ox, oy, oz, x, y, z)
		x, y = valid.X, valid.Y
	}

	e.landing = location.Location{X: x, Y: y, Z: z}
	refresh(e.Effected)

	mover.FlyTo(e.landing, modelskill.FlightThrowUp)
	return true
}

// throwUpExit teleports the target to its pre-computed landing point and
// syncs it to observers.
func throwUpExit(e *Effect) {
	refresh(e.Effected)
	if mover, ok := e.Effected.(flightMover); ok {
		mover.SetXYZ(e.landing.X, e.landing.Y, e.landing.Z)
		mover.BroadcastPosition()
	}
}
