package skills

import (
	"slices"
	"testing"

	"github.com/rs/zerolog"

	xmldata "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// The fixtures below mirror shipped chance skills: Mirage (445) arms an
// ON_ATTACKED trigger for 5144, "Item Skill: Heal" (3207) is a passive
// ON_HIT skill triggering 5146, and "Special Ability: Infinity Scepter"
// (3595) is a passive ON_MAGIC_GOOD skill triggering 3596, "Special
// Ability: Infinity Blade" (3578) is a passive ON_CRIT skill triggering
// 3579 and "Item Skill: Slow" (3096) is a passive ON_MAGIC_OFFENSIVE skill
// triggering 5166. Each triggered
// skill carries an icon effect here so its landing is observable, and each
// triggered debuff lands at a fixed rate so the test does not depend on the
// landing roll.
const (
	mirageSkill          = 445
	mirageTriggered      = 5144
	onHitPassive         = 3207
	onHitTriggered       = 5146
	onGoodPassive        = 3595
	onGoodTriggered      = 3596
	selfBuffSkill        = 1204
	npcShieldSkill       = 4493
	npcShieldTrigger     = 5520
	onHitTriggerReuse    = 30_000
	onCritPassive        = 3578
	onCritTriggered      = 3579
	onOffensivePassive   = 3096
	onOffensiveTriggered = 5166
	mdamSkill            = 1177
	toggleSkill          = 312
	unlandableDebuff     = 1164
)

func chanceSkills() []modelskill.Definition {
	return []modelskill.Definition{
		{
			ID: mirageSkill, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "BUFF", HitTime: 500, StaticHitTime: true,
			Effects: []modelskill.EffectTemplate{{
				Name: "ChanceSkillTrigger", Time: 60, Icon: true, StackType: "mirage", StackOrder: 1,
				TriggeredID: mirageTriggered, TriggeredLevel: 1, ChanceType: "ON_ATTACKED", ActivationChance: 80,
			}},
		},
		{
			ID: mirageTriggered, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
			SkillType: "DEBUFF", EffectType: "DEBUFF", Debuff: true, Offensive: true, CastRange: 900, EffectRange: 1400,
			EffectPower: 100, IgnoreResists: true,
			Effects: []modelskill.EffectTemplate{{Name: "Debuff", Time: 30, Icon: true, StackType: "mirage_debuff", StackOrder: 1}},
		},
		{
			ID: onHitPassive, Level: 1, Activation: modelskill.ActivationPassive, Target: modelskill.TargetSelf,
			SkillType: "BUFF", ChanceType: "ON_HIT", ActivationChance: 1,
			TriggeredID: onHitTriggered, TriggeredLevel: 1,
		},
		{
			ID: onHitTriggered, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "BUFF", ReuseDelay: onHitTriggerReuse,
			Effects: []modelskill.EffectTemplate{{Name: "Buff", Time: 60, Icon: true, StackType: "item_heal", StackOrder: 1}},
		},
		{
			ID: onGoodPassive, Level: 1, Activation: modelskill.ActivationPassive, Target: modelskill.TargetSelf,
			SkillType: "BUFF", ChanceType: "ON_MAGIC_GOOD", ActivationChance: 3,
			TriggeredID: onGoodTriggered, TriggeredLevel: 1,
		},
		{
			ID: onGoodTriggered, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "BUFF",
			Effects:   []modelskill.EffectTemplate{{Name: "Buff", Time: 60, Icon: true, StackType: "full_recover", StackOrder: 1}},
		},
		{
			ID: selfBuffSkill, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "BUFF", HitTime: 500, StaticHitTime: true,
			Effects: []modelskill.EffectTemplate{{Name: "Buff", Time: 60, Icon: true, StackType: "speed_up", StackOrder: 1}},
		},
		{
			ID: onCritPassive, Level: 1, Activation: modelskill.ActivationPassive, Target: modelskill.TargetSelf,
			SkillType: "BUFF", ChanceType: "ON_CRIT", ActivationChance: 5,
			TriggeredID: onCritTriggered, TriggeredLevel: 1,
		},
		{
			ID: onCritTriggered, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
			SkillType: "DEBUFF", EffectType: "DEBUFF", Debuff: true, Offensive: true,
			EffectPower: 100, IgnoreResists: true,
			Effects: []modelskill.EffectTemplate{{Name: "Debuff", Time: 120, Icon: true, StackType: "pd_down", StackOrder: 3}},
		},
		{
			ID: onOffensivePassive, Level: 1, Activation: modelskill.ActivationPassive, Target: modelskill.TargetSelf,
			SkillType: "BUFF", ChanceType: "ON_MAGIC_OFFENSIVE", ActivationChance: 2,
			TriggeredID: onOffensiveTriggered, TriggeredLevel: 1,
		},
		{
			ID: onOffensiveTriggered, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
			SkillType: "DEBUFF", EffectType: "DEBUFF", Debuff: true, Offensive: true,
			EffectPower: 100, IgnoreResists: true,
			Effects: []modelskill.EffectTemplate{{Name: "Debuff", Time: 30, Icon: true, StackType: "speed_down", StackOrder: 1}},
		},
		{
			// unlandableDebuff lands at its base chance of 20, so a landing
			// roll of 20 or more loses.
			ID: unlandableDebuff, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
			SkillType: "DEBUFF", EffectType: "DEBUFF", Debuff: true, Offensive: true,
			IgnoreResists: true,
			Effects:       []modelskill.EffectTemplate{{Name: "Debuff", Time: 30, Icon: true, StackType: "weakness", StackOrder: 1}},
		},
		{
			ID: mdamSkill, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
			SkillType: "MDAM", Offensive: true, Power: 1, CastRange: 900, HitTime: 500, StaticHitTime: true,
		},
		{
			ID: toggleSkill, Level: 1, Activation: modelskill.ActivationToggle, Target: modelskill.TargetSelf,
			SkillType: "BUFF",
			Effects:   []modelskill.EffectTemplate{{Name: "Buff", Time: 60, Icon: true, StackType: "vicious_stance", StackOrder: 1}},
		},
		{
			ID: npcShieldSkill, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "BUFF", HitTime: 500, StaticHitTime: true,
			Effects: []modelskill.EffectTemplate{{
				Name: "ChanceSkillTrigger", Time: 300, StackType: "debuff_shield", StackOrder: 1,
				TriggeredID: npcShieldTrigger, TriggeredLevel: 1, ChanceType: "ON_ATTACKED", ActivationChance: -1,
			}},
		},
		{
			ID: npcShieldTrigger, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
			SkillType: "DEBUFF", EffectType: "DEBUFF", Debuff: true, Offensive: true,
			EffectPower: 100, IgnoreResists: true,
			Effects: []modelskill.EffectTemplate{{Name: "Debuff", Time: 30, Icon: true, StackType: "shield_debuff", StackOrder: 1}},
		},
	}
}

// bootChanceHolder boots a player knowing skills, in world, on a fixed roll.
func bootChanceHolder(t *testing.T, roll int, skills ...int) (*gameservertest.Server, *testsupport.ScriptedClient, int32) {
	t.Helper()
	return bootChanceHolderWith(t, chanceSkills(), roll, skills...)
}

// bootChanceHolderWith is bootChanceHolder over the skill table defs.
func bootChanceHolderWith(t *testing.T, defs []modelskill.Definition, roll int, skills ...int) (*gameservertest.Server, *testsupport.ScriptedClient, int32) {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, defs)),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	for _, id := range skills {
		seedKnownSkill(t, srv, objID, id, 1)
	}
	startInWorld(t, c)
	setPlayerRoll(t, srv, objID, roll)
	return srv, c, objID
}

func setPlayerRoll(t *testing.T, srv *gameservertest.Server, objID int32, roll int) {
	t.Helper()
	setPlayerRollSource(t, srv, objID, func(n int) int { return min(roll, n-1) })
}

func setPlayerRollSource(t *testing.T, srv *gameservertest.Server, objID int32, source func(int) int) {
	t.Helper()
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatalf("world.Player(%d) missing", objID)
	}
	roller, ok := obj.(interface{ SetRollSource(func(int) int) })
	if !ok {
		t.Fatalf("world.Player(%d) = %T, want SetRollSource", objID, obj)
	}
	roller.SetRollSource(source)
}

func worldCombatant(t *testing.T, srv *gameservertest.Server, objID int32) attackable.Combatant {
	t.Helper()
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatalf("world.Player(%d) missing", objID)
	}
	victim, ok := obj.(attackable.Combatant)
	if !ok {
		t.Fatalf("world.Player(%d) = %T is no combatant", objID, obj)
	}
	return victim
}

// castSelf casts skillID on the player itself and waits for its effect.
func castSelf(t *testing.T, srv *gameservertest.Server, c *testsupport.ScriptedClient, objID int32, skillID int32) {
	t.Helper()
	c.Send(encodeRequestMagicSkillUse(skillID, false, false))
	srv.AdvanceUntil(t, "self cast lands", func() bool { return slices.Contains(liveHeldSkillIDs(t, srv, objID), skillID) })
}

// startMelee clicks hostile twice: the first click targets it, the second
// starts the auto-attack.
func startMelee(t *testing.T, srv *gameservertest.Server, objID, hostileID int32) {
	t.Helper()
	px, py, pz := srv.PlayerPosition(t, objID)
	srv.Client.Send(encodeAction(hostileID, int32(px), int32(py), int32(pz), false))
	drainUntilQuiet(t, srv.Client)
	srv.Client.Send(encodeAction(hostileID, int32(px), int32(py), int32(pz), false))
}

type procFrames struct {
	launched, used int
}

// procSeen reports the index of the triggered cast's MagicSkillLaunched and
// MagicSkillUse among frames (-1 when absent), asserting each one names the
// expected caster and target, a zero cast time and no reuse.
func procSeen(t *testing.T, frames [][]byte, casterID, skillID, skillLevel, targetID int32) procFrames {
	t.Helper()
	seen := procFrames{launched: -1, used: -1}
	for i, frame := range frames {
		r := wireReader(frame[1:])
		switch frame[0] {
		case serverpackets.OpcodeMagicSkillLaunched:
			caster, id, level, count := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
			if id != skillID {
				continue
			}
			if seen.launched >= 0 {
				t.Fatalf("MagicSkillLaunched for skill %d sent twice", skillID)
			}
			if caster != casterID || level != skillLevel || count != 1 || r.ReadInt32() != targetID {
				t.Fatalf("MagicSkillLaunched(skill %d) = caster %d level %d count %d, want caster %d level %d target %d",
					skillID, caster, level, count, casterID, skillLevel, targetID)
			}
			seen.launched = i
		case serverpackets.OpcodeMagicSkillUse:
			caster, target, id, level := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
			if id != skillID {
				continue
			}
			if seen.used >= 0 {
				t.Fatalf("MagicSkillUse for skill %d sent twice", skillID)
			}
			hitTime, reuse := r.ReadInt32(), r.ReadInt32()
			if caster != casterID || target != targetID || level != skillLevel || hitTime != 0 || reuse != 0 {
				t.Fatalf("MagicSkillUse(skill %d) = %d->%d level %d hit %d reuse %d, want %d->%d level %d hit 0 reuse 0",
					skillID, caster, target, level, hitTime, reuse, casterID, targetID, skillLevel)
			}
			seen.used = i
		}
	}
	return seen
}

func assertProc(t *testing.T, frames [][]byte, casterID, skillID, skillLevel, targetID int32) {
	t.Helper()
	seen := procSeen(t, frames, casterID, skillID, skillLevel, targetID)
	if seen.launched < 0 || seen.used < 0 {
		t.Fatalf("triggered skill %d frames: MagicSkillLaunched at %d, MagicSkillUse at %d, want both", skillID, seen.launched, seen.used)
	}
	if seen.launched > seen.used {
		t.Fatalf("triggered skill %d sent MagicSkillUse (%d) before MagicSkillLaunched (%d)", skillID, seen.used, seen.launched)
	}
}

func holds(effects []*effect.Effect, skillID int) bool {
	return slices.ContainsFunc(effects, func(e *effect.Effect) bool { return int(e.Skill.ID) == skillID })
}

// TestChanceTriggerEffectProcsWhenHolderIsHit: a player under Mirage who
// takes a landed melee hit rolls its 80% ON_ATTACKED trigger; on a win it
// casts 5144 at the attacker, shown as the player's own launch and
// animation, and the debuff lands on the attacker. A roll of 80 loses.
func TestChanceTriggerEffectProcsWhenHolderIsHit(t *testing.T) {
	for _, tc := range []struct {
		name  string
		roll  int
		procs bool
	}{
		{"roll wins", 79, true},
		{"roll loses", 80, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv, c, objID := bootChanceHolder(t, tc.roll, mirageSkill)
			castSelf(t, srv, c, objID, mirageSkill)
			drainUntilQuiet(t, c)

			px, py, pz := srv.PlayerPosition(t, objID)
			attacker := srv.SpawnAttackingHostileNPCAt(t, location.Location{X: px + 20, Y: py, Z: pz})
			drainUntilQuiet(t, c)
			attacker.DoAttack(t, worldCombatant(t, srv, objID))
			frames := queueFrames(t, c)

			if !tc.procs {
				if seen := procSeen(t, frames, objID, mirageTriggered, 1, attacker.ObjectID()); seen.launched >= 0 || seen.used >= 0 {
					t.Fatalf("lost roll still cast %d: %+v", mirageTriggered, seen)
				}
				if holds(attacker.EffectList().All(), mirageTriggered) {
					t.Fatal("lost roll still debuffed the attacker")
				}
				return
			}
			assertProc(t, frames, objID, mirageTriggered, 1, attacker.ObjectID())
			if !holds(attacker.EffectList().All(), mirageTriggered) {
				t.Fatalf("attacker effects = %v, want the triggered %d debuff", attacker.EffectList().All(), mirageTriggered)
			}
		})
	}
}

// TestChanceTriggerStopsWithItsEffect: once Mirage's effect leaves the
// player, a landed hit no longer procs it.
func TestChanceTriggerStopsWithItsEffect(t *testing.T) {
	t.Parallel()
	srv, c, objID := bootChanceHolder(t, 0, mirageSkill)
	castSelf(t, srv, c, objID, mirageSkill)
	liveEffectList(t, srv, objID).StopBySkillID(mirageSkill)
	srv.Settle(t)
	drainUntilQuiet(t, c)

	px, py, pz := srv.PlayerPosition(t, objID)
	attacker := srv.SpawnAttackingHostileNPCAt(t, location.Location{X: px + 20, Y: py, Z: pz})
	drainUntilQuiet(t, c)
	attacker.DoAttack(t, worldCombatant(t, srv, objID))
	if seen := procSeen(t, queueFrames(t, c), objID, mirageTriggered, 1, attacker.ObjectID()); seen.launched >= 0 || seen.used >= 0 {
		t.Fatalf("hit after Mirage ended still cast %d: %+v", mirageTriggered, seen)
	}
}

// TestPassiveChanceSkillProcsOnOwnHit: a player knowing a passive ON_HIT
// chance skill triggers its self buff when its own melee hit lands, and the
// triggered skill's reuse starts, so the next hits do not cast it again.
func TestPassiveChanceSkillProcsOnOwnHit(t *testing.T) {
	t.Parallel()
	srv, c, objID := bootChanceHolder(t, 0, onHitPassive)
	px, py, pz := srv.PlayerPosition(t, objID)
	hostile := srv.SpawnHostileNPCAt(t, location.Location{X: px + 20, Y: py, Z: pz})
	drainUntilQuiet(t, c)

	startMelee(t, srv, objID, hostile.ObjectID())
	srv.AdvanceUntil(t, "on-hit proc", func() bool { return slices.Contains(liveHeldSkillIDs(t, srv, objID), onHitTriggered) })
	assertProc(t, queueFrames(t, c), objID, onHitTriggered, 1, objID)

	liveEffectList(t, srv, objID).StopBySkillID(onHitTriggered)
	hp := hostile.CurrentHP()
	srv.AdvanceUntil(t, "another landed hit", func() bool { return hostile.CurrentHP() < hp || hostile.Dead() })
	srv.Settle(t)
	if slices.Contains(liveHeldSkillIDs(t, srv, objID), onHitTriggered) {
		t.Fatal("triggered skill cast again inside its reuse delay")
	}
}

// TestPassiveChanceSkillProcsOnGoodMagic: a player knowing a passive
// ON_MAGIC_GOOD chance skill triggers it when its own self buff lands.
func TestPassiveChanceSkillProcsOnGoodMagic(t *testing.T) {
	t.Parallel()
	srv, c, objID := bootChanceHolder(t, 0, onGoodPassive, selfBuffSkill)
	castSelf(t, srv, c, objID, selfBuffSkill)
	srv.AdvanceUntil(t, "on-good-magic proc", func() bool { return slices.Contains(liveHeldSkillIDs(t, srv, objID), onGoodTriggered) })
	assertProc(t, queueFrames(t, c), objID, onGoodTriggered, 1, objID)
}

// TestNPCChanceTriggerProcsWhenNPCIsHit: a monster holding an ON_ATTACKED
// chance trigger answers the player's landed hit by casting the triggered
// debuff on the player, shown as the monster's launch and animation.
func TestNPCChanceTriggerProcsWhenNPCIsHit(t *testing.T) {
	t.Parallel()
	defs := skillPersistence(t, chanceSkills())
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(defs),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	setPlayerRoll(t, srv, objID, 0)
	hostile := spawnShieldedHostile(t, srv, defs)
	drainUntilQuiet(t, c)

	startMelee(t, srv, objID, hostile.ObjectID())
	srv.AdvanceUntil(t, "npc proc", func() bool { return slices.Contains(liveHeldSkillIDs(t, srv, objID), npcShieldTrigger) })
	assertProc(t, queueFrames(t, c), hostile.ObjectID(), npcShieldTrigger, 1, objID)
}

// spawnShieldedHostile spawns a monster on a zero roll and has it cast
// npcShieldSkill on itself, arming its ON_ATTACKED trigger for
// npcShieldTrigger.
func spawnShieldedHostile(t *testing.T, srv *gameservertest.Server, defs actorcast.Definitions) *npc.Hostile {
	t.Helper()
	hostile, aiCtl := srv.SpawnCastingHostileNPC(t, &npc.Template{
		ID: 100, TemplateID: 100, Type: "Monster", Level: 1, HPMax: 100_000,
		AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
	}, defs)
	hostile.SetRollSource(func(int) int { return 0 })
	// A template M.Def of 0 truncates to 0, which an MDAM turns into a
	// killing blow; give the monster a real one.
	hostile.AddStatFuncs([]effect.Mod{{Stat: stat.MagicDefence, Op: effect.OpSet, Value: 100}})
	if !hostile.Queue().Post(func() { aiCtl.Cast(hostile, modelskill.Ref{ID: npcShieldSkill, Level: 1}) }) {
		t.Fatal("post npc cast: queue closed")
	}
	srv.AdvanceUntil(t, "npc shield", func() bool { return holds(hostile.EffectList().All(), npcShieldSkill) })
	return hostile
}

// TestDamageSkillFiresCasterAndTargetProcs: a player knowing a passive
// ON_MAGIC_OFFENSIVE chance skill lands an MDAM on a monster holding an
// ON_ATTACKED trigger. Both procs fire: the player's debuff lands on the
// monster and the monster's debuff lands on the player.
func TestDamageSkillFiresCasterAndTargetProcs(t *testing.T) {
	t.Parallel()
	defs := skillPersistence(t, chanceSkills())
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(defs),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, onOffensivePassive, 1)
	seedKnownSkill(t, srv, objID, mdamSkill, 1)
	startInWorld(t, c)
	// The proc roll (out of 100) wins; the magic resist and magic critical
	// rolls lose, so the MDAM lands as a plain hit.
	setPlayerRollSource(t, srv, objID, func(n int) int {
		if n == 100 {
			return 0
		}
		return n - 1
	})
	hostile := spawnShieldedHostile(t, srv, defs)
	drainUntilQuiet(t, c)

	px, py, pz := srv.PlayerPosition(t, objID)
	c.Send(encodeAction(hostile.ObjectID(), int32(px), int32(py), int32(pz), false))
	drainUntilQuiet(t, c)
	c.Send(encodeRequestMagicSkillUse(mdamSkill, false, false))
	srv.AdvanceUntil(t, "both procs land", func() bool {
		return holds(hostile.EffectList().All(), onOffensiveTriggered) &&
			slices.Contains(liveHeldSkillIDs(t, srv, objID), npcShieldTrigger)
	})
	frames := queueFrames(t, c)
	assertProc(t, frames, objID, onOffensiveTriggered, 1, hostile.ObjectID())
	assertProc(t, frames, hostile.ObjectID(), npcShieldTrigger, 1, objID)
}

// TestToggleRaisesNoChanceEvent: a toggle reaches its effects without the
// cast-hit steps, so turning one on does not proc a passive ON_MAGIC_GOOD
// skill the way a regular self buff does.
func TestToggleRaisesNoChanceEvent(t *testing.T) {
	t.Parallel()
	srv, c, objID := bootChanceHolder(t, 0, onGoodPassive, toggleSkill)
	c.Send(encodeRequestMagicSkillUse(toggleSkill, false, false))
	srv.AdvanceUntil(t, "toggle on", func() bool { return slices.Contains(liveHeldSkillIDs(t, srv, objID), toggleSkill) })
	srv.Settle(t)
	if seen := procSeen(t, queueFrames(t, c), objID, onGoodTriggered, 1, objID); seen.launched >= 0 || seen.used >= 0 {
		t.Fatalf("toggle cast %d: %+v", onGoodTriggered, seen)
	}
	if slices.Contains(liveHeldSkillIDs(t, srv, objID), onGoodTriggered) {
		t.Fatal("toggle procced the ON_MAGIC_GOOD skill")
	}
}

// TestOnCritChanceSkillProcsOnlyOnCriticalHit: a passive ON_CRIT chance
// skill debuffs the monster when the player's landed melee hit is a
// critical one, and stays silent on a landed hit that is not.
func TestOnCritChanceSkillProcsOnlyOnCriticalHit(t *testing.T) {
	for _, tc := range []struct {
		name  string
		crit  bool
		procs bool
	}{
		{"critical hit", true, true},
		{"normal hit", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv, c, objID := bootChanceHolder(t, 0, onCritPassive)
			// Every roll is 0 but the critical roll (out of 1000) of a normal
			// hit: 299 still lands against the 300 hit-rate floor and loses
			// to the character's critical rate.
			setPlayerRollSource(t, srv, objID, func(n int) int {
				if n == 1000 && !tc.crit {
					return 299
				}
				return 0
			})
			px, py, pz := srv.PlayerPosition(t, objID)
			hostile := srv.SpawnHostileNPCAt(t, location.Location{X: px + 20, Y: py, Z: pz})
			drainUntilQuiet(t, c)

			hp := hostile.CurrentHP()
			startMelee(t, srv, objID, hostile.ObjectID())
			srv.AdvanceUntil(t, "a landed hit", func() bool { return hostile.CurrentHP() < hp || hostile.Dead() })
			srv.Settle(t)
			frames := queueFrames(t, c)
			if !tc.procs {
				if seen := procSeen(t, frames, objID, onCritTriggered, 1, hostile.ObjectID()); seen.launched >= 0 || seen.used >= 0 {
					t.Fatalf("normal hit cast %d: %+v", onCritTriggered, seen)
				}
				if holds(hostile.EffectList().All(), onCritTriggered) {
					t.Fatal("normal hit procced the ON_CRIT skill")
				}
				return
			}
			assertProc(t, frames, objID, onCritTriggered, 1, hostile.ObjectID())
			if !holds(hostile.EffectList().All(), onCritTriggered) {
				t.Fatalf("hostile effects = %v, want the triggered %d debuff", hostile.EffectList().All(), onCritTriggered)
			}
		})
	}
}

// TestChanceSkillConditionGatesProc: a passive ON_HIT chance skill whose
// <cond> caps the owner's HP at 25% casts nothing for a player at full HP,
// and the clause's message, naming the passive skill, reaches the player.
// With the cap at 100% the same skill procs.
func TestChanceSkillConditionGatesProc(t *testing.T) {
	const condPassive = 3208
	for _, tc := range []struct {
		name  string
		hp    string
		procs bool
	}{
		{"clause fails", "25", false},
		{"clause holds", "100", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			defs := append(chanceSkills(), modelskill.Definition{
				ID: condPassive, Level: 1, Activation: modelskill.ActivationPassive, Target: modelskill.TargetSelf,
				SkillType: "BUFF", ChanceType: "ON_HIT", ActivationChance: 5,
				TriggeredID: onHitTriggered, TriggeredLevel: 1,
				Conditions: []modelskill.ConditionClause{{
					Root:      modelskill.Condition{Kind: "player", Attrs: map[string]string{"hp": tc.hp}},
					MessageID: serverpackets.SystemMessageS1CannotBeUsed, AddName: true,
				}},
			})
			srv, c, objID := bootChanceHolderWith(t, defs, 0, condPassive)
			px, py, pz := srv.PlayerPosition(t, objID)
			hostile := srv.SpawnHostileNPCAt(t, location.Location{X: px + 20, Y: py, Z: pz})
			drainUntilQuiet(t, c)

			hp := hostile.CurrentHP()
			startMelee(t, srv, objID, hostile.ObjectID())
			srv.AdvanceUntil(t, "a landed hit", func() bool { return hostile.CurrentHP() < hp || hostile.Dead() })
			srv.Settle(t)
			frames := queueFrames(t, c)
			if tc.procs {
				assertProc(t, frames, objID, onHitTriggered, 1, objID)
				assertNoSystemMessage(t, frames, serverpackets.SystemMessageS1CannotBeUsed)
				return
			}
			if seen := procSeen(t, frames, objID, onHitTriggered, 1, objID); seen.launched >= 0 || seen.used >= 0 {
				t.Fatalf("failed clause still cast %d: %+v", onHitTriggered, seen)
			}
			if slices.Contains(liveHeldSkillIDs(t, srv, objID), onHitTriggered) {
				t.Fatal("failed clause still applied the triggered buff")
			}
			i := slices.IndexFunc(frames, func(f []byte) bool {
				return f[0] == serverpackets.OpcodeSystemMessage &&
					wireReader(f[1:]).ReadInt32() == serverpackets.SystemMessageS1CannotBeUsed
			})
			if i < 0 {
				t.Fatalf("no clause message %d among %v", serverpackets.SystemMessageS1CannotBeUsed, systemMessageIDs(frames))
			}
			assertSystemMessageSkillFrame(t, frames[i], serverpackets.SystemMessageS1CannotBeUsed, condPassive, 1)
		})
	}
}

// shippedSkillDefinitions loads the shared datapack's skill definitions,
// skipping the calling test when no parent directory of the checkout holds
// aCis_datapack (it fails instead when ACIS_REQUIRE_DATAPACK is set).
func shippedSkillDefinitions(t *testing.T) []modelskill.Definition {
	t.Helper()
	dir := datapack.Path(t, "data", "xml", "skills")
	table, err := xmldata.LoadSkillDefinitions(dir, zerolog.Nop())
	if err != nil {
		t.Fatalf("LoadSkillDefinitions: %v", err)
	}
	return table.All()
}

// TestShippedChanceSkillsProc drives the shipped data: Mirage (445) arms
// an 80% ON_ATTACKED trigger for 5144 level 1 (triggeredLevel defaults to
// 1), and "Item Skill: Heal" level 10 (3207) is a passive 5% ON_HIT skill
// triggering 5146 level 10. A won roll casts each one. The won 5144 lands
// its RemoveTarget on the attacker (power 100 against a roll of 0), which
// stops the attacker's swing before it finishes.
func TestShippedChanceSkillsProc(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, shippedSkillDefinitions(t))),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, mirageSkill, 1)
	seedKnownSkill(t, srv, objID, onHitPassive, 10)
	startInWorld(t, c)
	setPlayerRoll(t, srv, objID, 0)
	// Mirage costs 63 MP, more than the fixture character's pool.
	onPlayerQueue(t, srv, objID, func(pc *player.Character) {
		pc.AddStatFuncs([]effect.Mod{{Stat: stat.MaxMP, Op: effect.OpAdd, Value: 100}})
		pc.AddMP(pc.MaxMPValue())
	})
	castSelf(t, srv, c, objID, mirageSkill)
	drainUntilQuiet(t, c)

	px, py, pz := srv.PlayerPosition(t, objID)
	attacker := srv.SpawnAttackingHostileNPCAt(t, location.Location{X: px + 20, Y: py, Z: pz})
	drainUntilQuiet(t, c)
	victim := worldCombatant(t, srv, objID)
	hp, ok := victim.(interface{ HP() float64 })
	if !ok {
		t.Fatalf("world.Player(%d) = %T has no HP", objID, victim)
	}
	full := hp.HP()
	attacker.StartAttack(victim)
	srv.AdvanceUntil(t, "the attacker's hit", func() bool { return hp.HP() < full })
	srv.Settle(t)
	assertProc(t, queueFrames(t, c), objID, mirageTriggered, 1, attacker.ObjectID())

	// The triggered heal leaves no effect behind; its reuse, started by the
	// proc, shows it ran.
	obj, _ := srv.State.Player(objID)
	reuse, ok := obj.(interface{ SkillDisabled(int32) bool })
	if !ok {
		t.Fatalf("world.Player(%d) = %T has no SkillDisabled", objID, obj)
	}
	startMelee(t, srv, objID, attacker.ObjectID())
	srv.AdvanceUntil(t, "Item Skill: Heal proc", func() bool { return reuse.SkillDisabled(onHitTriggered*256 + 10) })
	assertProc(t, queueFrames(t, c), objID, onHitTriggered, 10, objID)
}
