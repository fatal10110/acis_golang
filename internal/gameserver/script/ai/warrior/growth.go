package warrior

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// growthDir is the directory of the growing beasts a player feeds.
const growthDir = "ai/individual/Monster/WarriorBase/Warrior/WarriorGrowth"

// growthSocialDelay is the delay, in TakeSocial frames of 30 a second,
// after which a fed beast grows.
func growthSocialDelay(n *script.NPC) time.Duration {
	return time.Duration(n.IntParamOr("TakeSocial", 5)*1000/30) * time.Millisecond
}

// growthEat makes n play its eating animation, for MoveAroundSocial frames
// of 30 a second, unless TakeSocial is 0.
func growthEat(n *script.NPC) {
	if n.IntParamOr("TakeSocial", 5) != 0 {
		n.AddSocialDesire(1, int(n.IntParam("MoveAroundSocial")*1000/30), 200)
	}
}

// growthHate makes n hate the playable attacker by the damage it dealt,
// scaled down by n's level and up by n's hate ratio for it.
func growthHate(n *script.NPC, attacker script.Creature, damage int32) {
	if attacker.Playable() {
		n.AddAttackDesire(attacker, damageHate(n, float64(damage), defaultnpc.HateRatio(n, attacker)))
	}
}

// growInto calls the NPC of id where n stands, facing its way, as n's
// private, with the creature that fed n (c_ai0) and the food's skill id
// (i_ai3) as its spawn parameters.
func growInto(s *script.Script, n *script.NPC, id int32) {
	s.Spawner().CreateOnePrivateEx(n, id, script.LocOf(n), 0, script.Params{P1: n.Slot(script.SlotCreature0).ObjectID(), P2: n.Int(script.SlotIntAI3)})
}

// growthStrike makes n, fighting the playable attacker as its top desire
// target, maybe blast around itself when the attacker stands closer than
// 200 and maybe strike the attacker with its physical special.
func growthStrike(s *script.Script, n *script.NPC, attacker script.Creature) {
	if n.Distance2D(attacker) < 200 && s.Rnd(100) < 33 {
		castByType(n, n, npc.SkillTypeSelfRangeDDMagic)
	}
	if s.Rnd(100) < 33 {
		castByType(n, attacker, npc.SkillTypePhysicalSpecial)
	}
}

// fedWith reports whether skill is the food n's AI parameter name names.
func fedWith(n *script.NPC, used script.Attacked, name string) bool {
	ref, ok := n.SkillParam(name)
	return ok && used.Skill.ID != 0 && used.Skill == ref
}

// growthCreated is the created hook of a beast grown from a fed one: it
// keeps the feeder (spawn parameter 1) in c_ai0 and attacks it, keeps the
// food (spawn parameter 2) in i_ai3, and maybe buffs itself.
func growthCreated(s *script.Script, n *script.NPC) {
	n.SetInt(script.SlotIntAI4, 1)
	n.SetSlot(script.SlotCreature0, s.Object(n.Int(script.SlotParam1)))
	n.SetInt(script.SlotIntAI3, n.Int(script.SlotParam2))
	if c0 := n.Slot(script.SlotCreature0); c0 != nil {
		n.AddAttackDesire(c0, 100)
	}
	n.SetInt(script.SlotIntAI2, 0)
	if s.Rnd(100) < 33 {
		castByType(n, n, npc.SkillTypeSelfBuff)
	}
}

// growthFed is the attacked hook of a beast grown from a fed one, hit by
// the food it grew from: it eats and may say one of its five lines from
// firstLine. Fed again by its feeder, each feed counts; still unfed since
// it grew (i_ai4 1), it grows after its eating animation with a chance of
// chance percent per feed counted (i_ai4 3). Fed by its feeder after it
// was fed by another, it marks itself spoiled (i_ai4 2) and hates its
// feeder by the damage.
func growthFed(s *script.Script, e script.Attacked, firstLine, chance int32) {
	n, attacker := e.NPC, e.Attacker
	growthEat(n)
	if s.Rnd(100) < 5 {
		n.Say(script.NpcString(int32(s.Rnd(5)) + firstLine))
	}
	c0 := n.Slot(script.SlotCreature0)
	if c0 == nil || !script.Same(c0, attacker) {
		return
	}
	n.SetInt(script.SlotIntAI2, n.Int(script.SlotIntAI2)+1)
	spoiled := false
	if n.Int(script.SlotIntAI4) == 1 {
		if int32(s.Rnd(100)) <= n.Int(script.SlotIntAI2)*chance {
			s.StartTimer("2001", n, nil, growthSocialDelay(n))
			n.SetInt(script.SlotIntAI4, 3)
		}
	} else if n.Int(script.SlotIntAI4) != 3 {
		n.SetInt(script.SlotIntAI4, 2)
		spoiled = true
	}
	if spoiled {
		growthHate(n, attacker, e.Damage)
	}
}

// fedOnce reports whether n's growth timer may grow it: fed once (i_ai4
// 1) or grown by a count of feeds (i_ai4 3), and alive.
func fedOnce(n *script.NPC) bool {
	i4 := n.Int(script.SlotIntAI4)
	return (i4 == 1 || i4 == 3) && !n.Dead()
}

// growIntoByOccupation calls one of n's two silhouettes at even odds, or,
// for a draw of 50 or less, the one for a fighter feeder or the other.
// A feeder that is not a player grows nothing and keeps n.
func growIntoByOccupation(s *script.Script, n *script.NPC) bool {
	silhouette1 := n.IntParam("silhouette1")
	silhouette2 := n.IntParam("silhouette2")
	silhouetteEx := n.IntParam("silhouette_ex")
	silhouetteEx2 := n.IntParam("silhouette_ex2")
	switch {
	case s.Rnd(100) > 50:
		if s.Rnd(100) < 50 {
			growInto(s, n, silhouette1)
		} else {
			growInto(s, n, silhouette2)
		}
	default:
		p, ok := n.Slot(script.SlotCreature0).(*script.Player)
		if !ok {
			return false
		}
		if p.SameOccupation("@fighter") {
			growInto(s, n, silhouetteEx)
		} else {
			growInto(s, n, silhouetteEx2)
		}
	}
	n.DeleteMe()
	return true
}

// FarmGrowthStep is FarmGrowthStep: a farm calf that, fed its food, eats,
// may say a line, and after its eating animation grows into one of its
// silhouettes, the feeder's occupation choosing for half the draws.
// Fighting a player or summon as its top desire target, it may blast
// around itself and strike with its physical special, for itself and
// for its clan.
func FarmGrowthStep() script.Script {
	base := WarriorHooks()
	return script.Script{Dir: growthDir, Behavior: true, NPCs: []int32{16019, 16021, 16023}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			n.SetInt(script.SlotIntAI4, 0)
			if s.Rnd(100) < 33 {
				castByType(n, n, npc.SkillTypeSelfBuff)
			}
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if fedWith(n, e, "FeedID1") {
				n.SetInt(script.SlotIntAI3, int32(e.Skill.ID))
				if n.Int(script.SlotIntAI4) == 0 {
					s.StartTimer("2001", n, nil, growthSocialDelay(n))
					n.SetInt(script.SlotIntAI4, 1)
					n.SetSlot(script.SlotCreature0, attacker)
					growthEat(n)
					if s.Rnd(100) < 5 {
						n.Say(script.NpcString(int32(s.Rnd(10)) + 2004))
					}
				}
			} else {
				growthHate(n, attacker, e.Damage)
			}
			if attacker.Playable() && script.Same(n.TopDesireTarget(), attacker) {
				growthStrike(s, n, attacker)
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called, attacker := e.Called, e.Attacker
			if called.LifeTime() > 7 && attacker.Playable() && script.Same(called.TopDesireTarget(), attacker) {
				growthStrike(s, called, attacker)
			}
			base.ClanAttacked(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			if strings.EqualFold(e.Name, "2001") && fedOnce(e.NPC) {
				if !growIntoByOccupation(s, e.NPC) {
					return ""
				}
			}
			return base.Timer(s, e)
		},
	})}
}

// WarriorGrowthStep1 is WarriorGrowthStep1: a young beast that, fed one of
// its two foods, eats, may say a line, and after its eating animation
// grows into a warrior or a wizard silhouette of that food. Fed by another
// before it grows, it hates that feeder. Its other hits reach its parent;
// its timers stop at itself.
func WarriorGrowthStep1() script.Script {
	base := WarriorHooks()
	return script.Script{Dir: growthDir, Behavior: true, NPCs: []int32{21451, 21470, 21489}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			e.NPC.SetInt(script.SlotIntAI4, 0)
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if fedWith(n, e, "FeedID1") || fedWith(n, e, "FeedID2") {
				n.SetInt(script.SlotIntAI3, int32(e.Skill.ID))
				if n.Int(script.SlotIntAI4) == 0 {
					s.StartTimer("2001", n, nil, growthSocialDelay(n))
					n.SetInt(script.SlotIntAI4, 1)
					n.SetSlot(script.SlotCreature0, attacker)
					growthEat(n)
					if s.Rnd(100) < 5 {
						n.Say(script.NpcString(int32(s.Rnd(10)) + 2004))
					}
				} else if n.Int(script.SlotIntAI4) != 1 || !script.Same(n.Slot(script.SlotCreature0), attacker) {
					n.SetInt(script.SlotIntAI4, 2)
					growthHate(n, attacker, e.Damage)
				}
			} else {
				base.Attacked(s, e)
			}
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			n := e.NPC
			if strings.EqualFold(e.Name, "2001") && n.Int(script.SlotIntAI4) == 1 && !n.Dead() {
				feed1, ok := n.SkillParam("FeedID1")
				if !ok {
					return ""
				}
				var warrior1, warrior2, wizard1, wizard2 int32
				if n.Int(script.SlotIntAI3) == int32(feed1.ID) {
					warrior1, warrior2 = n.IntParam("FeedID1_warrior_silhouette1"), n.IntParam("FeedID1_warrior_silhouette2")
					wizard1, wizard2 = n.IntParam("FeedID1_wizard_silhouette1"), n.IntParam("FeedID1_wizard_silhouette2")
				} else {
					warrior1, warrior2 = n.IntParam("FeedID2_warrior_silhouette1"), n.IntParam("FeedID2_warrior_silhouette2")
					wizard1, wizard2 = n.IntParam("FeedID2_wizard_silhouette1"), n.IntParam("FeedID2_wizard_silhouette2")
				}
				switch {
				case s.Rnd(100) < 50:
					if s.Rnd(100) < 50 {
						growInto(s, n, warrior1)
					} else {
						growInto(s, n, warrior2)
					}
				case s.Rnd(100) < 50:
					growInto(s, n, wizard1)
				default:
					growInto(s, n, wizard2)
				}
				n.DeleteMe()
			}
			return ""
		},
	})}
}

// WarriorGrowthStep2 is WarriorGrowthStep2: a beast grown once, attacking
// its feeder as it appears. Fed by its feeder it may grow into one of its
// two silhouettes, likelier with each feed; hit otherwise by a player or
// summon it fights, it may strike with its physical special, as it may
// for its clan. Told a grown beast appeared near it (event 10018), it
// grows into its last silhouette one time in five.
func WarriorGrowthStep2() script.Script {
	base := WarriorHooks()
	return script.Script{Dir: growthDir, Behavior: true, NPCs: []int32{21452, 21454, 21456, 21458, 21471, 21473, 21475, 21477, 21490, 21492, 21494, 21496}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			growthCreated(s, e.NPC)
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if e.Skill.ID != 0 && int32(e.Skill.ID) == n.Int(script.SlotIntAI3) {
				growthFed(s, e, 2014, 33)
			} else if attacker.Playable() && script.Same(n.TopDesireTarget(), attacker) && s.Rnd(100) < 33 {
				castByType(n, attacker, npc.SkillTypePhysicalSpecial)
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called, attacker := e.Called, e.Attacker
			if attacker.Playable() && called.LifeTime() > 7 && script.Same(called.TopDesireTarget(), attacker) && s.Rnd(100) < 33 {
				castByType(called, attacker, npc.SkillTypePhysicalSpecial)
			}
			base.ClanAttacked(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			n := e.NPC
			if strings.EqualFold(e.Name, "2001") && fedOnce(n) {
				silhouette1 := n.IntParam("silhouette1")
				silhouette2 := n.IntParam("silhouette2")
				if s.Rnd(100) < 50 {
					growInto(s, n, silhouette1)
				} else {
					growInto(s, n, silhouette2)
				}
				n.DeleteMe()
			}
			return base.Timer(s, e)
		},
		OnScriptEvent: func(s *script.Script, e script.ScriptEvent) {
			n := e.NPC
			if e.EventID == 10018 && s.Rnd(100) < 20 {
				if n.Slot(script.SlotCreature0) == nil {
					return
				}
				growInto(s, n, n.IntParam("silhouette_ex2"))
				n.DeleteMe()
			}
			base.ScriptEvent(s, e)
		},
	})}
}

// WarriorGrowthStep3Aggressive is WarriorGrowthStep3Aggressive: a beast
// grown twice, attacking its feeder as it appears and the players and
// summons it sees. Fed by its feeder it may grow into one of its
// silhouettes, likelier with each feed, the feeder's occupation choosing
// for half the draws; hit otherwise by a player or summon it fights, it
// may blast around itself and strike with its physical special, as it
// may for its clan.
func WarriorGrowthStep3Aggressive() script.Script {
	base := WarriorHooks()
	return script.Script{Dir: growthDir, Behavior: true, NPCs: []int32{21460, 21462, 21464, 21466, 21479, 21481, 21483, 21485, 21498, 21500, 21502, 21504}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			growthCreated(s, e.NPC)
			base.Created(s, e)
		},
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			if !e.Creature.Playable() {
				return
			}
			defaultnpc.TryToAttack(s, e.NPC, e.Creature)
			base.SeeCreature(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if e.Skill.ID != 0 && int32(e.Skill.ID) == n.Int(script.SlotIntAI3) {
				growthFed(s, e, 2019, 20)
			} else if attacker.Playable() && script.Same(n.TopDesireTarget(), attacker) {
				growthStrike(s, n, attacker)
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called, attacker := e.Called, e.Attacker
			if called.LifeTime() > 7 && attacker.Playable() && script.Same(called.TopDesireTarget(), attacker) {
				growthStrike(s, called, attacker)
			}
			base.ClanAttacked(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			if strings.EqualFold(e.Name, "2001") && fedOnce(e.NPC) {
				if !growIntoByOccupation(s, e.NPC) {
					return ""
				}
			}
			return base.Timer(s, e)
		},
	})}
}

// WarriorGrowthStep4Aggressive is WarriorGrowthStep4Aggressive: a fully
// grown beast, attacking its feeder as it appears and the players and
// summons it sees, that tells the beasts within 700 it appeared (event
// 10018). Its first hit starts a 5 s fight timer; each tick, unless its
// top desire target hit it since the last, it may blast that target with
// its DD magic when the target holds none of its effect, and out of a
// fight it stands down until hit again. Hit by a player or summon it
// fights, it may strike with its physical special, as it may for its clan.
func WarriorGrowthStep4Aggressive() script.Script {
	base := WarriorHooks()
	return script.Script{Dir: growthDir, Behavior: true, NPCs: []int32{21468, 21487, 21506, 21824, 21826, 21828}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			n.SetInt(script.SlotIntAI1, 0)
			n.SetInt(script.SlotIntAI2, 0)
			n.SetInt(script.SlotIntAI4, 1)
			n.SetSlot(script.SlotCreature0, s.Object(n.Int(script.SlotParam1)))
			n.SetInt(script.SlotIntAI3, n.Int(script.SlotParam2))
			if c0 := n.Slot(script.SlotCreature0); c0 != nil {
				n.AddAttackDesire(c0, 100)
			}
			n.SetInt(script.SlotIntAI2, 0)
			s.BroadcastScriptEvent(n, 10018, n.ObjectID(), 700)
			if s.Rnd(100) < 33 {
				castByType(n, n, npc.SkillTypeSelfBuff)
			}
			base.Created(s, e)
		},
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			if !e.Creature.Playable() {
				return
			}
			defaultnpc.TryToAttack(s, e.NPC, e.Creature)
			base.SeeCreature(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			top := n.TopDesireTarget()
			if n.Int(script.SlotIntAI1) == 1 && script.Same(top, attacker) {
				n.SetInt(script.SlotIntAI2, 1)
			} else {
				s.StartTimer("2001", n, nil, 5000*time.Millisecond)
				n.SetInt(script.SlotIntAI1, 1)
			}
			if attacker.Playable() && script.Same(top, attacker) && s.Rnd(100) < 33 {
				castByType(n, attacker, npc.SkillTypePhysicalSpecial)
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called, attacker := e.Called, e.Attacker
			if called.LifeTime() > 7 && attacker.Playable() && script.Same(called.TopDesireTarget(), attacker) && s.Rnd(100) < 33 {
				castByType(called, attacker, npc.SkillTypePhysicalSpecial)
			}
			base.ClanAttacked(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			n := e.NPC
			if strings.EqualFold(e.Name, "2001") {
				if i := n.Intention(); i != script.IntentionAttack && i != script.IntentionCast {
					n.SetInt(script.SlotIntAI1, 0)
					n.SetInt(script.SlotIntAI2, 0)
				} else if n.Int(script.SlotIntAI2) == 0 {
					if top := n.TopDesireTarget(); top != nil && s.Rnd(100) < 50 {
						if ddMagic, ok := n.SkillByType(npc.SkillTypeDDMagic); ok && s.AbnormalLevel(top, ddMagic) <= 0 {
							n.AddCastDesire(top, ddMagic, 1000000)
						}
					}
				}
				s.StartTimer("2001", n, nil, 5000*time.Millisecond)
				n.SetInt(script.SlotIntAI2, 0)
			}
			return base.Timer(s, e)
		},
	})}
}
