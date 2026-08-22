package network

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cubic"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// cubicCastDelay is Cubic.CAST_DELAY: the fixed delay between a cubic's
// visible MagicSkillUse broadcast and its effect actually landing.
const cubicCastDelay = 2 * time.Second

func (l *GameClientLink) syncCubicTargets(caster *livePlayer, result actorcast.EffectResult, def modelskill.Definition) {
	if result.CubicTouched {
		l.syncCubicRuntime(caster, result.CubicID, def)
	}
	if result.CubicAdded {
		l.broadcastCharacterInfo(caster)
	}
	for _, target := range result.CubicTargets {
		if live, ok := target.(*livePlayer); ok {
			l.syncCubicRuntime(live, result.CubicID, def)
		}
	}
	for _, target := range result.CubicAddedTargets {
		if live, ok := target.(*livePlayer); ok {
			l.broadcastCharacterInfo(live)
		}
	}
}

// syncCubicRuntime (re)synchronizes live's live cubic runtime for id after a
// SUMMON cubic cast touched its cubic list, matching
// CubicList.addOrRefreshCubic: a brand new id gets a fresh Runtime (self-
// starting immediately for the Life Cubic, matching Cubic's constructor;
// every other kind waits for the owner's next combat-stance entry via
// Cubics()/AttackStance.Add), while an already-active id only has its
// disappear timer reset, its original granting level and fire behavior left
// untouched.
func (l *GameClientLink) syncCubicRuntime(live *livePlayer, id cubic.ID, def modelskill.Definition) {
	if live == nil {
		return
	}
	lifetime := time.Duration(def.SummonTotalLifeTime) * time.Millisecond

	live.cubicsMu.Lock()
	if live.cubics == nil {
		live.cubics = make(map[cubic.ID]*cubic.Runtime)
	}
	runtime, exists := live.cubics[id]
	if !exists {
		interval := time.Duration(def.CubicActivationTime) * time.Second
		runtime = cubic.NewRuntime(id, actorcast.CubicGrantedLevel(def), def.CubicActivationChance, interval, func() {
			l.fireCubic(live, id, runtime)
		}, func() {
			l.expireCubic(live, id)
		}, l.cubicAfterFunc)
		live.cubics[id] = runtime
	}
	live.cubicsMu.Unlock()

	runtime.RefreshDisappear(lifetime)
	if id == cubic.Life {
		runtime.Action()
	}
}

// expireCubic runs when a cubic's granted lifetime elapses: it drops the id
// from the owner's cubic list and live runtime map and refreshes the
// character info the client sees, matching Cubic.stop() removing itself
// from CubicList and broadcasting.
func (l *GameClientLink) expireCubic(live *livePlayer, id cubic.ID) {
	if live == nil {
		return
	}
	live.Character.RemoveCubic(id)
	live.cubicsMu.Lock()
	runtime := live.cubics[id]
	delete(live.cubics, id)
	live.cubicsMu.Unlock()
	if runtime != nil {
		runtime.Stop()
	}
	l.broadcastCharacterInfo(live)
}

// Cubics satisfies task.attackStanceCubics, letting AttackStance.Add restart
// every non-Life cubic's action tick when live enters combat stance.
func (live *livePlayer) Cubics() []task.AttackStanceCubic {
	live.cubicsMu.Lock()
	defer live.cubicsMu.Unlock()
	out := make([]task.AttackStanceCubic, 0, len(live.cubics))
	for _, r := range live.cubics {
		out = append(out, r)
	}
	return out
}

// cubicStillActive reports whether id is still one of live's active
// cubics, so a deferred effect scheduled cubicCastDelay ago can tell its
// cubic wasn't stopped (owner death, logout, natural expiry) in the
// meantime.
func (live *livePlayer) cubicStillActive(id cubic.ID) bool {
	live.cubicsMu.Lock()
	defer live.cubicsMu.Unlock()
	_, ok := live.cubics[id]
	return ok
}

// fireCubic runs one action tick for live's cubic id, matching
// Cubic.fireAction: a dead owner stops the cubic outright rather than
// firing; for a non-Life cubic, going out of combat stance stops the action
// tick outright (StopAction) rather than firing, otherwise the granting
// skill's activation chance, a random skill among the cubic's fixed set,
// and the target are resolved by cast.DecideCubicFire. The Life Cubic
// instead always tries its single heal skill against cast.DecideLifeCubicTarget
// and ignores the combat-stance gate entirely. Target selection, activation
// rolls, and the heal formula are domain decisions (model/actor/cast); this
// function only resolves session/world context, calls those decisions, and
// translates the outcome into packets.
func (l *GameClientLink) fireCubic(live *livePlayer, id cubic.ID, runtime *cubic.Runtime) {
	if live == nil || runtime == nil {
		return
	}
	if live.Character.Dead() {
		// Matches Cubic.fireAction's own isDead()/isOnline() self-check:
		// a dead owner stops the cubic outright rather than firing.
		l.expireCubic(live, id)
		return
	}

	skillIDs := cubic.SkillIDs(id)
	if len(skillIDs) == 0 {
		return
	}

	owner := cubicFireOwner{live}

	var (
		skillID int
		target  actorcast.Target
		ok      bool
	)
	if id == cubic.Life {
		skillID = skillIDs[0]
		target, ok = actorcast.DecideLifeCubicTarget(owner)
	} else {
		if l.attackStance == nil || !l.attackStance.InAttackStance(live) {
			runtime.StopAction()
			return
		}
		skillID, target, ok = actorcast.DecideCubicFire(owner, skillIDs, runtime.ActivationChance)
	}
	if !ok {
		return
	}

	def, ok := l.skills.Definition(modelskill.Ref{ID: modelskill.ID(skillID), Level: runtime.Level})
	if !ok {
		return
	}

	broadcastLevel := def.Level
	if id == cubic.Life && broadcastLevel == 8 {
		// Cubic.fireAction's Life-branch broadcast quirk: the granted
		// level-8 heal skill displays as level 20 on the client.
		broadcastLevel = 20
	}

	casterObject := skillCastObject(live)
	targetObject := skillCastObject(target)
	l.broadcastLiveFrame(live, func() wire.Frame {
		return serverpackets.FrameMagicSkillUse(
			casterObject, targetObject, int32(def.ID), int32(broadcastLevel),
			int(cubicCastDelay/time.Millisecond), int(cubicCastDelay/time.Millisecond), false)
	})

	beforeVitals := live.Vitals()
	if id == cubic.Life {
		l.scheduleAfter(cubicCastDelay, func() {
			// The reference's stop() cancels the in-flight cast task on a
			// dead/stopped cubic; re-check here since fireCubic only gated
			// on Dead() at the start of the tick, before this delay.
			// detached() also covers a logout inside the delay window:
			// stopCubics() stops each runtime's timers but never removes
			// it from live.cubics, so cubicStillActive alone would still
			// report true after the session detached.
			if live.Character.Dead() || live.detached() || !live.cubicStillActive(id) {
				return
			}
			if healed := actorcast.ApplyCubicHeal(def.Power, target); healed {
				if recipient, ok := target.(cubicHealMessageTarget); ok {
					recipient.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageRejuvenatingHP))
				}
			}
			sendMagicStatusUpdate(live, beforeVitals)
		})
		return
	}
	l.scheduleAfter(cubicCastDelay, func() {
		if live.Character.Dead() || live.detached() || !live.cubicStillActive(id) {
			return
		}
		result := actorcast.ApplyCubicEffect(l.skillHandlers, live.Character, def, target)
		l.sendSkillHandlerResult(live, result)
		sendMagicStatusUpdate(live, beforeVitals)
	})
}

// cubicHealMessageTarget is the narrow surface a Life Cubic heal target
// sends the REJUVENATING_HP feedback message to; only a player-shaped
// target gets it, matching Cubic.useHealSkill's
// `if (target instanceof Player)` guard.
type cubicHealMessageTarget interface {
	SendFrame(wire.Frame) bool
}

// cubicFireOwner adapts *livePlayer to actorcast.CubicFireOwner: Roll,
// CurrentHP, MaxHPValue, ObjectID and Position already match through
// promotion from the embedded Character. Target() below is a plain
// passthrough, not a boxing conversion — Character.Target() already returns
// world.Tracked, the same type CubicFireOwner's Target() wants.
type cubicFireOwner struct{ *livePlayer }

func (o cubicFireOwner) Target() world.Tracked { return o.livePlayer.Target() }
