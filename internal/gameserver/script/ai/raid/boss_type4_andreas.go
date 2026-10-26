package raid

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// BossType4AndreasAggressive is Andreas van Halter, the high priestess of
// the altar: a RaidBossType4 that runs its own fight. Its memory:
//   - i_ai2: it has seen a character since its last reset;
//   - i_ai3: the altar's stage, set by the script events below 9;
//   - i_ai4: how many of its three teleports (below 60%, 30% and 10% of
//     its HP) it has made;
//   - i_quest0: 1 while it has dropped its fight after a teleport;
//   - i_quest1: the game tick of its last hit or stage change;
//   - c_ai0: itself while it has no foe, else its foe.
//
// Appearing, it tells the NPCs around it (event 0), places its four
// privates around its start, and arms a 5 hour clock that has it leave,
// resetting the altar's six combat makers, if it has never fought by then
// (else it checks again every 30 minutes). Hit, it tells its foe to the
// NPCs within its seek range (event 10002) and, at each HP step, casts its
// teleport effect, which jumps it to one of its three teleport points and
// starts the altar's next combat makers. It reports its death (event 5).
func BossType4AndreasAggressive() script.Script {
	return script.Script{Dir: "ai/individual/Monster/RaidBoss/RaidBossAlone/RaidBossParty/RaidBossType4", Behavior: true, NPCs: []int32{29062}, Hooks: bossType4AndreasAggressiveHooks()}
}

// andreasStart is where Andreas starts and goes back to.
func andreasStart(n *script.NPC) script.Loc {
	return script.Loc{
		X: int(n.IntParamOr("Start_x", -16385)),
		Y: int(n.IntParamOr("Start_y", -53268)),
		Z: int(n.IntParamOr("Start_z", -10439)),
	}
}

// andreasMaker returns the npcmaker name Andreas reads from param, def
// when its spawn names none.
func andreasMaker(n *script.NPC, param, def string) string {
	if v, ok := n.StringParam(param); ok {
		return v
	}
	return def
}

// andreasMakerEvent runs maker script event name on each of the npcmakers
// Andreas names by params, with their defaults, in order.
func andreasMakerEvent(s *script.Script, n *script.NPC, name string, params ...[2]string) {
	for _, p := range params {
		s.Spawner().SendMakerEvent(andreasMaker(n, p[0], p[1]), name, 0, 0)
	}
}

// andreasCombat1 is the altar's first set of combat makers, by spawn
// parameter and default.
func andreasCombat1() [][2]string {
	return [][2]string{{"MidSpawnMakerName11", "altar_combat1_01"}, {"MidSpawnMakerName12", "altar_combat1_02"}, {"MidSpawnMakerName13", "altar_combat1_03"}}
}

// andreasCombat2 is the altar's second set of combat makers.
func andreasCombat2() [][2]string {
	return [][2]string{{"MidSpawnMakerName21", "altar_combat2_01"}, {"MidSpawnMakerName22", "altar_combat2_02"}, {"MidSpawnMakerName23", "altar_combat2_03"}}
}

// andreasReset forgets Andreas's fight: no sight, no stage, no teleport,
// no foe.
func andreasReset(n *script.NPC) {
	n.SetInt(script.SlotIntAI2, 0)
	n.SetInt(script.SlotIntAI3, 0)
	n.SetInt(script.SlotIntAI4, 0)
	n.SetSlot(script.SlotCreature0, n)
}

// andreasTeleportStep has Andreas, at the teleport count step, drop its
// attack desires and want its teleport effect on itself at 1000000.
func andreasTeleportStep(n *script.NPC, step int32) {
	if n.Int(script.SlotIntAI4) == step {
		n.RemoveAllAttackDesire()
		castByType(n, n, npc.SkillTypeTeleportEffect)
	}
}

// andreasBroadcastRange is how far Andreas tells the NPCs around it of its
// own state.
func andreasBroadcastRange(n *script.NPC) int {
	return int(n.IntParamOr("BroadcastRange", 4000))
}

// andreasSeekRange is how far Andreas tells the NPCs around it of its foe.
func andreasSeekRange(n *script.NPC) int {
	return int(n.IntParamOr("SeekRange", 400))
}

// bossType4AndreasAggressiveHooks are Andreas's hooks. Its created,
// no-desire, see-creature and script event hooks call no parent; its
// attacked hook calls it both first and last.
func bossType4AndreasAggressiveHooks() script.Hooks {
	base := bossType4Hooks()
	return base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			n.SetInt(script.SlotIntAI2, 0)
			n.SetInt(script.SlotIntAI3, 0)
			n.SetInt(script.SlotIntAI4, 0)
			n.SetInt(npc.IntQuest0, 0)
			n.SetInt(npc.IntQuest1, 0)
			n.SetSlot(script.SlotCreature0, n)
			s.BroadcastScriptEvent(n, 0, n.ObjectID(), andreasBroadcastRange(n)*2)
			s.StartTimer("6008", n, nil, 5*60*60000*time.Millisecond)
			silhouettes := [4]int32{n.IntParam("private1_silhouette"), n.IntParam("private2_silhouette"), n.IntParam("private3_silhouette"), n.IntParam("private4_silhouette")}
			xs := [4]int32{n.IntParamOr("private1_pos_x", -91), n.IntParamOr("private2_pos_x", -48), n.IntParamOr("private3_pos_x", -91), n.IntParamOr("private4_pos_x", -48)}
			ys := [4]int32{n.IntParamOr("private1_pos_y", -95), n.IntParamOr("private2_pos_y", -130), n.IntParamOr("private3_pos_y", 95), n.IntParamOr("private4_pos_y", 130)}
			start := andreasStart(n)
			for i := range silhouettes {
				at := script.Loc{X: start.X + int(xs[i]), Y: start.Y + int(ys[i]), Z: start.Z}
				s.Spawner().CreateOnePrivateEx(n, silhouettes[i], at, 0, script.Params{})
			}
		},
		OnNoDesire: func(s *script.Script, e script.NoDesire) {
			n := e.NPC
			if n.Int(script.SlotIntAI3) != 0 || n.Int(script.SlotIntAI4) != 0 {
				n.SetInt(script.SlotIntAI2, 0)
				if n.Slot(script.SlotCreature0) != nil {
					s.StartTimer("6005", n, nil, 5000*time.Millisecond)
				}
			} else if n.Int(script.SlotIntAI3) < 3 || n.Int(script.SlotIntAI3) >= 7 {
				n.AddMoveToDesire(andreasStart(n), 30)
			}
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n := e.NPC
			base.Attacked(s, e)
			if top := n.TopDesireTarget(); top != nil {
				n.SetSlot(script.SlotCreature0, top)
			} else {
				n.SetSlot(script.SlotCreature0, e.Attacker)
			}
			n.SetInt(npc.IntQuest1, s.CurrentTick())
			if int32(s.Rnd(100)) < n.IntParamOr("AttackPartyPointRate", 33) {
				s.BroadcastScriptEvent(n, 10002, n.Slot(script.SlotCreature0).ObjectID(), andreasSeekRange(n))
			}
			switch hpRatio := n.HPRatio(); {
			case hpRatio < 0.1:
				andreasTeleportStep(n, 2)
			case hpRatio < 0.3:
				andreasTeleportStep(n, 1)
			case hpRatio < 0.6:
				andreasTeleportStep(n, 0)
			}
			base.Attacked(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			n := e.NPC
			switch {
			case strings.EqualFold(e.Name, "6008"):
				if n.Int(script.SlotIntAI2) == 0 && n.Int(script.SlotIntAI3) == 0 && n.Int(script.SlotIntAI4) == 0 && script.Same(n.Slot(script.SlotCreature0), n) &&
					n.Intention() != script.IntentionAttack && n.Intention() != script.IntentionCast {
					andreasMakerEvent(s, n, "1000", andreasCombat1()...)
					andreasMakerEvent(s, n, "1000", andreasCombat2()...)
					andreasReset(n)
					s.BroadcastScriptEvent(n, 6, n.ObjectID(), andreasBroadcastRange(n)*2)
					n.DeleteMe()
				} else {
					s.StartTimer("6008", n, e.Player, 30*60*1000*time.Millisecond)
				}
			case strings.EqualFold(e.Name, "6007"):
				n.RemoveAllDesire()
				n.HateList().CleanAll()
				n.SetInt(npc.IntQuest0, 1)
			case strings.EqualFold(e.Name, "6006"):
				if i0 := s.ElapsedTicks(n.Int(npc.IntQuest1)); i0 > (30 * 60) {
					n.RemoveAllAttackDesire()
					s.BroadcastScriptEvent(n, 7, 0, andreasBroadcastRange(n))
				} else {
					s.StartTimer("6006", n, e.Player, 60000*time.Millisecond)
				}
			case strings.EqualFold(e.Name, "6005"):
				if foe := n.Slot(script.SlotCreature0); foe != nil {
					n.SetInt(npc.IntQuest0, 0)
					if foe.Playable() {
						n.AddAttackDesire(foe, 700)
					}
					s.BroadcastScriptEvent(n, 10002, foe.ObjectID(), andreasSeekRange(n))
				}
			case strings.EqualFold(e.Name, "6004") || strings.EqualFold(e.Name, "6003"):
				n.SetInt(npc.IntQuest0, 0)
				n.LookNeighbor()
			case strings.EqualFold(e.Name, "6099"):
				n.AddCastDesire(n, skill.Ref{ID: 4183, Level: 1}, 1000000)
			}
			return base.Timer(s, e)
		},
		OnSeeCreature: func(_ *script.Script, e script.SeeCreature) {
			n := e.NPC
			n.SetInt(script.SlotIntAI2, 1)
			if _, ok := e.Creature.(*script.Player); ok {
				if foe := n.Slot(script.SlotCreature0); script.Same(foe, n) {
					n.AddAttackDesire(e.Creature, 700)
				} else if foe != nil && foe.Playable() {
					n.AddAttackDesire(foe, 700)
				}
			}
		},
		OnUseSkillFinished: func(s *script.Script, e script.UseSkillFinished) {
			n := e.NPC
			if teleport, ok := n.SkillByType(npc.SkillTypeTeleportEffect); ok && e.Skill == teleport {
				tel1 := script.Loc{X: int(n.IntParamOr("TelPosX1", -16393)), Y: int(n.IntParamOr("TelPosY1", -53433)), Z: int(n.IntParamOr("TelPosZ1", -10439))}
				tel2 := script.Loc{X: int(n.IntParamOr("TelPosX2", -17150)), Y: int(n.IntParamOr("TelPosY2", -54064)), Z: int(n.IntParamOr("TelPosZ2", -10439))}
				tel3 := script.Loc{X: int(n.IntParamOr("TelPosX3", -15690)), Y: int(n.IntParamOr("TelPosY3", -54030)), Z: int(n.IntParamOr("TelPosZ3", -10439))}
				switch n.Int(script.SlotIntAI4) {
				case 2:
					if e.Success {
						n.TeleportTo(tel1)
					}
					n.SetInt(script.SlotIntAI4, 3)
					s.StartTimer("6007", n, nil, 500*time.Millisecond)
					s.StartTimer("6005", n, nil, 10000*time.Millisecond)
				case 1:
					if e.Success {
						switch i0 := s.Rnd(3); i0 {
						case 0:
							n.TeleportTo(tel1)
						case 1:
							n.TeleportTo(tel2)
						default:
							n.TeleportTo(tel3)
						}
					}
					n.SetInt(script.SlotIntAI4, 2)
					andreasMakerEvent(s, n, "1001", andreasCombat2()...)
					s.StartTimer("6007", n, nil, 500*time.Millisecond)
					s.StartTimer("6004", n, nil, 10000*time.Millisecond)
				case 0:
					if e.Success {
						if i0 := s.Rnd(2); i0 == 0 {
							n.TeleportTo(tel3)
						} else {
							n.TeleportTo(tel2)
						}
					}
					n.SetInt(script.SlotIntAI4, 1)
					andreasMakerEvent(s, n, "1001", andreasCombat1()...)
					s.StartTimer("6007", n, nil, 500*time.Millisecond)
					s.StartTimer("6003", n, nil, 10000*time.Millisecond)
				}
			}
			base.UseSkillFinished(s, e)
		},
		OnScriptEvent: func(s *script.Script, e script.ScriptEvent) {
			n := e.NPC
			if e.EventID < 9 {
				n.SetInt(script.SlotIntAI3, e.EventID)
				if e.EventID == 1 {
					s.StartTimer("6006", n, nil, 60000*time.Millisecond)
				}
				if e.EventID > 0 {
					n.SetInt(npc.IntQuest1, s.CurrentTick())
				}
			}
			switch e.EventID {
			case 4:
				if e.Arg1 != 0 {
					s.BroadcastScriptEvent(n, 10020, e.Arg1, andreasSeekRange(n))
					if c0 := s.Object(e.Arg1); c0 != nil && c0.Playable() {
						n.AddAttackDesire(c0, 700)
					}
				}
			case 7:
				andreasReset(n)
				n.AddMoveToDesire(andreasStart(n), 30)
			case 3:
				s.StartTimer("6099", n, nil, 6700*time.Millisecond)
			}
		},
		OnMyDying: func(s *script.Script, e script.MyDying) {
			n := e.NPC
			andreasMakerEvent(s, n, "1000", andreasCombat1()...)
			andreasMakerEvent(s, n, "1000", andreasCombat2()...)
			andreasReset(n)
			s.BroadcastScriptEvent(n, 5, n.ObjectID(), andreasBroadcastRange(n)*2)
			base.MyDying(s, e)
		},
	}).Named("RaidBossType4AndreasAggressive")
}
