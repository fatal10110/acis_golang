package network

import (
	"math"
	"time"

	handlerskill "github.com/fatal10110/acis_golang/internal/gameserver/handler/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cubic"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

// cubicMaxMagicRange is Cubic.MAX_MAGIC_RANGE: the 3D range (collision radii
// included) both pickEnemyTarget and pickFriendlyTarget scan within.
const cubicMaxMagicRange = 900

// cubicCastDelay is Cubic.CAST_DELAY: the fixed delay between a cubic's
// visible MagicSkillUse broadcast and its effect actually landing.
const cubicCastDelay = 2 * time.Second

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
		runtime = cubic.NewRuntime(id, def.Level, def.CubicActivationChance, interval, func() {
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

// fireCubic runs one action tick for live's cubic id, matching
// Cubic.fireAction: for a non-Life cubic, going out of combat stance stops
// the action tick outright (StopAction) rather than firing; otherwise it
// rolls the granting skill's activation chance, picks a random skill among
// the cubic's fixed set, and resolves a target. The Life Cubic instead
// always tries its single heal skill against a friendly target and ignores
// the combat-stance gate entirely.
func (l *GameClientLink) fireCubic(live *livePlayer, id cubic.ID, runtime *cubic.Runtime) {
	if live == nil || runtime == nil {
		return
	}

	skillIDs := cubic.SkillIDs(id)
	if len(skillIDs) == 0 {
		return
	}

	var (
		skillID int
		target  actorcast.Target
		ok      bool
	)
	if id == cubic.Life {
		skillID = skillIDs[0]
		target, ok = l.pickCubicFriendlyTarget(live)
	} else {
		if l.attackStance == nil || !l.attackStance.InAttackStance(live) {
			runtime.StopAction()
			return
		}
		if live.Roll(100) >= runtime.ActivationChance {
			return
		}
		skillID = skillIDs[live.Roll(len(skillIDs))]
		target, ok = l.pickCubicEnemyTarget(live)
	}
	if !ok {
		return
	}

	def, ok := l.skills.Definition(modelskill.Ref{ID: modelskill.ID(skillID), Level: runtime.Level})
	if !ok {
		return
	}

	casterObject := skillCastObject(live)
	targetObject := skillCastObject(target)
	l.broadcastLiveFrame(live, func() wire.Frame {
		return serverpackets.FrameMagicSkillUse(
			casterObject, targetObject, int32(def.ID), int32(def.Level),
			int(cubicCastDelay/time.Millisecond), int(cubicCastDelay/time.Millisecond), false)
	})

	beforeVitals := live.Vitals()
	l.scheduleAfter(cubicCastDelay, func() {
		l.skillHandlers.UseResult(handlerskill.Cast{Caster: live.Character, Skill: def, Targets: []any{target}})
		sendMagicStatusUpdate(live, beforeVitals)
	})
}

// pickCubicEnemyTarget mirrors Cubic.pickEnemyTarget: the owner's currently
// selected target, if within range and not already dead. The reference's
// full isAttackableWithoutForceBy alignment/karma matrix is not replicated
// here — deferred, see #1129.
func (l *GameClientLink) pickCubicEnemyTarget(live *livePlayer) (actorcast.Target, bool) {
	selected := live.Target()
	if selected == nil {
		return nil, false
	}
	combatant, ok := selected.(attackable.Combatant)
	if !ok || combatant.AlikeDead() {
		return nil, false
	}
	target, ok := selected.(actorcast.Target)
	if !ok || !cubicWithinRange(live, target) {
		return nil, false
	}
	return target, true
}

// pickCubicFriendlyTarget mirrors Cubic.pickFriendlyTarget's no-party
// fallback: heal the owner if under full HP, gated by the reference's
// HP-ratio-banded probability roll. The party-scan branch needs the
// milestone-M8 party system and isn't reachable yet — deferred, see #1129.
func (l *GameClientLink) pickCubicFriendlyTarget(live *livePlayer) (actorcast.Target, bool) {
	maxHP := live.MaxHPValue()
	if maxHP <= 0 {
		return nil, false
	}
	ratio := float64(live.CurrentHP()) / maxHP
	if ratio >= 1.0 {
		return nil, false
	}

	roll := live.Roll(100)
	var chance int
	switch {
	case ratio > 0.6:
		chance = 13
	case ratio < 0.3:
		chance = 53
	default:
		chance = 33
	}
	if roll > chance {
		return nil, false
	}
	return live, true
}

func cubicWithinRange(a, b actorcast.Target) bool {
	ax, ay, az := a.Position()
	bx, by, bz := b.Position()
	dx := float64(ax - bx)
	dy := float64(ay - by)
	dz := float64(az - bz)
	dist := math.Sqrt(dx*dx + dy*dy + dz*dz)

	total := float64(cubicMaxMagicRange) + cubicCollisionRadius(a) + cubicCollisionRadius(b)
	return dist <= total
}

func cubicCollisionRadius(t actorcast.Target) float64 {
	if cr, ok := t.(interface{ CollisionRadius() float64 }); ok {
		return cr.CollisionRadius()
	}
	return 0
}
