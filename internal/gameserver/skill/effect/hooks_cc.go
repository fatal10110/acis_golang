package effect

import (
	"math"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

func stunStart(e *Effect) bool {
	e.Effected.AbortAll(false)
	e.Effected.TryToIdle()
	refresh(e.Effected)
	return true
}

func rootStart(e *Effect) bool {
	e.Effected.StopMove()
	refresh(e.Effected)
	return true
}

func sleepStart(e *Effect) bool {
	e.Effected.AbortAll(false)
	refresh(e.Effected)
	return true
}

// fearFleeDistance is how far a feared creature runs from its effector on
// every flee.
const fearFleeDistance = 500

// fearStart keeps the fear once the target has been sent fleeing, whether or
// not that first flee could move it. The already-afraid check reads started
// effects only: this effect is already held, and its own flag must not
// reject it.
func fearStart(e *Effect) bool {
	if e.Effected.FearImmune() {
		return false
	}
	if isPlayable(e.Effected) && fearSkippedPlayableSkillIDs[e.Skill.ID] {
		return false
	}
	if e.Effected.EffectList().StartedAffected(FlagFear) {
		return false
	}

	e.Effected.AbortAll(false)
	refresh(e.Effected)
	fearAction(e)
	return true
}

// fearAction sends the target fleeing again on every tick; the fear lasts its
// full count whatever the flee does.
func fearAction(e *Effect) bool {
	e.Effected.FleeFrom(e.Effector, fearFleeDistance)
	return true
}

func fearExit(e *Effect) {
	e.Effected.StopEffects(TypeFear)
	refresh(e.Effected)
}

func thinkAndRefreshExit(e *Effect) {
	think(e.Effected)
	refresh(e.Effected)
}

// think wakes an NPC or summon target's AI; other kinds have no AI loop to
// wake here.
func think(target Actor) {
	if npc, ok := asNPC(target); ok {
		_ = npc.Think()
	} else if summon, ok := asSummon(target); ok {
		_ = summon.Think()
	}
}

func refreshExit(e *Effect) {
	refresh(e.Effected)
}

func abortCastStart(e *Effect) bool {
	if e.Effected == nil || e.Effected == e.Effector {
		return false
	}
	if e.Effected.RaidRelated() {
		return false
	}
	if target, ok := asCaster(e.Effected); ok && target.CastingNow() {
		target.InterruptCast()
	}
	return true
}

func immobileUntilAttackedStart(e *Effect) bool {
	e.Effected.AbortAll(false)
	refresh(e.Effected)
	return true
}

// immobileUntilAttackedExit wakes the effected's AI whatever its kind:
// unlike root, sleep, paralysis and petrification, a player is not left out.
func immobileUntilAttackedExit(e *Effect) {
	e.Effected.StopSkillEffectsByID(e.Skill.ID)
	if player, ok := asPlayer(e.Effected); ok && isPlayer(e.Effected) {
		player.WakeAI()
	} else {
		think(e.Effected)
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
	if e.Effector != nil {
		e.Effector.SetImmobilized(true)
	}
	return true
}

func immobilizeEffectorExit(e *Effect) {
	if e.Effector != nil {
		e.Effector.SetImmobilized(false)
	}
}

func invincibleStart(e *Effect) bool {
	e.Effected.SetInvul(true)
	return true
}

func invincibleExit(e *Effect) {
	e.Effected.SetInvul(false)
}

func muteStart(e *Effect) bool {
	if target, ok := asCaster(e.Effected); ok && target.CastingNow() && target.CurrentSkillIsMagic() {
		target.StopCast()
	}
	refresh(e.Effected)
	return true
}

func physicalMuteStart(e *Effect) bool {
	if target, ok := asCaster(e.Effected); ok && target.CastingNow() && !target.CurrentSkillIsMagic() {
		target.StopCast()
	}
	refresh(e.Effected)
	return true
}

func paralyzeStart(e *Effect) bool {
	startAbnormalEffect(e.Effected, 0x000400)
	e.Effected.AbortAll(false)
	return true
}

func paralyzeExit(e *Effect) {
	stopAbnormalEffect(e.Effected, 0x000400)
	thinkIfNotPlayer(e.Effected)
}

func petrificationStart(e *Effect) bool {
	startAbnormalEffect(e.Effected, 0x000800)
	e.Effected.AbortAll(false)
	e.Effected.SetInvul(true)
	return true
}

func petrificationExit(e *Effect) {
	stopAbnormalEffect(e.Effected, 0x000800)
	thinkIfNotPlayer(e.Effected)
	e.Effected.SetInvul(false)
}

func thinkIfNotPlayer(target Actor) {
	if isPlayer(target) {
		return
	}
	think(target)
}

func removeTargetStart(e *Effect) bool {
	e.Effected.ClearTarget()
	e.Effected.StopAttack()
	if target, ok := asCaster(e.Effected); ok {
		target.StopCast()
	}
	return true
}

func silenceAllStart(e *Effect) bool {
	if target, ok := asCaster(e.Effected); ok {
		target.StopCast()
	}
	refresh(e.Effected)
	return true
}

func silentMoveAction(e *Effect) bool {
	if e.Skill.SkillType != "CONT" {
		return false
	}
	return manaDrainTick(e)
}

func stunSelfStart(e *Effect) bool {
	if isPlayable(e.Effected) {
		e.Effected.TryToIdle()
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
	summon, ok := asSummon(e.Effected)
	if !ok || summon.OwnerID() != e.Effector.ObjectID() {
		return false
	}
	summon.SetImmobilized(true)
	return true
}

func immobilizePetBuffExit(e *Effect) {
	e.Effected.SetImmobilized(false)
}

// throwUpStart computes a knockback's landing point and starts the
// client-visible flight. The target is always aborted first, even when the
// distance gate below rejects the effect outright — that ordering matches
// the reference behavior, where the abort is unconditional and the range
// check only guards whether the flight itself happens.
//
// The destination pivots on the effector's position, not the effected's:
// the target is pushed further along the effector-to-effected line. Z is
// left at the effected's current height even after the X/Y geo correction
// below — the reference implementation never corrects Z for this effect,
// a known approximation preserved here rather than fixed. Every creature
// kind, summons included, takes the same flight.
func throwUpStart(e *Effect) bool {
	e.Effected.AbortAll(false)

	if e.Effector == nil {
		return false
	}
	sx, sy, sz := e.Effector.Position()
	ox, oy, oz := e.Effected.Position()
	dx := float64(sx - ox)
	dy := float64(sy - oy)
	distance := math.Sqrt(dx*dx + dy*dy)
	if distance < 1 || distance > 2000 {
		return false
	}

	offset := float64(min(int(distance)+e.Skill.FlyRadius, 1400))
	offset += math.Abs(float64(sz - oz))
	if offset < 5 {
		offset = 5
	}

	x := sx - int(offset*(dx/distance))
	y := sy - int(offset*(dy/distance))
	z := oz

	valid := e.Effected.ValidLocation(ox, oy, oz, x, y, z)
	x, y = valid.X, valid.Y

	e.landing = location.Location{X: x, Y: y, Z: z}
	refresh(e.Effected)

	e.Effected.FlyTo(e.landing, modelskill.FlightThrowUp)
	return true
}

// throwUpExit teleports the target to its pre-computed landing point and
// syncs it to observers.
func throwUpExit(e *Effect) {
	refresh(e.Effected)
	e.Effected.SetXYZ(e.landing.X, e.landing.Y, e.landing.Z)
	e.Effected.BroadcastPosition()
}
