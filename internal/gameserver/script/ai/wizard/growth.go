package wizard

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// growthDir is the directory of the growing spellcasting beasts a player
// feeds.
const growthDir = "ai/individual/Monster/WizardBase/Wizard/WizardDDMagic2/WizardGrowth"

// growthSocialDelay is the delay, in TakeSocial frames of 30 a second,
// after which a fed beast grows.
func growthSocialDelay(n *script.NPC) time.Duration {
	return time.Duration(n.IntParamOr("TakeSocial", 5)*1000/30) * time.Millisecond
}

// growthFed is the attacked hook of a beast grown from a fed one, hit by
// the food it grew from: unless TakeSocial is 0 it plays its eating
// animation, for MoveAroundSocial frames of 30 a second, and it may say
// one of its five lines from firstLine. Fed again by its feeder, each feed
// counts; still unfed since it grew (i_ai4 1), it grows after its eating
// animation with a chance of chance percent per feed counted (i_ai4 3).
// Fed by its feeder after it was fed by another, it marks itself spoiled
// (i_ai4 2) and, fed by a playable, wants to attack it by the damage.
func growthFed(s *script.Script, e script.Attacked, firstLine, chance int32) {
	n, attacker := e.NPC, e.Attacker
	if n.IntParamOr("TakeSocial", 5) != 0 {
		n.AddSocialDesire(1, int(n.IntParam("MoveAroundSocial")*1000/30), 200)
	}
	if s.Rnd(100) < 5 {
		n.Say(script.NpcString(int32(s.Rnd(5)) + firstLine))
	}
	c0 := n.Slot(script.SlotCreature0)
	if c0 == nil || !script.Same(c0, attacker) {
		return
	}
	n.SetInt(script.SlotIntAI2, n.Int(script.SlotIntAI2)+1)
	if n.Int(script.SlotIntAI4) == 1 {
		if int32(s.Rnd(100)) <= n.Int(script.SlotIntAI2)*chance {
			s.StartTimer("2001", n, nil, growthSocialDelay(n))
			n.SetInt(script.SlotIntAI4, 3)
		}
		return
	}
	if n.Int(script.SlotIntAI4) != 3 {
		n.SetInt(script.SlotIntAI4, 2)
		if attacker.Playable() {
			n.AddAttackDesire(attacker, damageHate(n, float64(e.Damage), defaultnpc.HateRatio(n, attacker))*100)
		}
	}
}

// growthSpell makes n want to cast at target, weight 1000000, the skill
// its template lists under t, when n has the MP and HP its own cast of it
// takes; otherwise n gives up its spells (int slot 0 set to 1) and wants
// to attack target, weight 1000.
func growthSpell(n *script.NPC, target script.Creature, t npc.SkillType) {
	ref, _ := n.SkillByType(t)
	if !n.MeetsHPMPConditions(n, ref) {
		n.SetInt(script.SlotIntAI0, 1)
		n.AddAttackDesire(target, 1000)
		return
	}
	n.AddCastDesire(target, ref, 1000000)
}

// growthStrikeFeeder makes n, grown from a fed beast, strike its feeder
// (c_ai0) with its long range DD magic beyond 100, else its short range
// one.
func growthStrikeFeeder(n *script.NPC) {
	c0 := n.Slot(script.SlotCreature0)
	if c0 == nil {
		return
	}
	if n.Distance2D(c0) > 100 {
		growthSpell(n, c0, npc.SkillTypeWLongRangeDDMagic)
	} else {
		growthSpell(n, c0, npc.SkillTypeWShortRangeDDMagic)
	}
}

// growthBlast makes n, until it gave up its spells, blast around itself at
// the playable attacker when it hates most someone else.
func growthBlast(n *script.NPC, attacker script.Creature) {
	if !attacker.Playable() || n.Int(script.SlotIntAI0) != 0 {
		return
	}
	if mostHatedHI := n.HateList().MostHated(); mostHatedHI != nil && !script.Same(mostHatedHI, attacker) {
		growthSpell(n, attacker, npc.SkillTypeSelfRangeDDMagic)
	}
}

// growInto calls the NPC of id where n stands, facing its way, as n's
// private, with feeder's object id and the food's skill id (i_ai3) as its
// spawn parameters.
func growInto(s *script.Script, n *script.NPC, id int32, feeder script.Creature) {
	s.Spawner().CreateOnePrivateEx(n, id, script.LocOf(n), 0, script.Params{P1: feeder.ObjectID(), P2: n.Int(script.SlotIntAI3)})
}

// growIntoByOccupation calls one of n's two silhouettes at even odds, or,
// for a draw of 50 or less, the one for a fighter feeder or the other,
// and leaves the world. A feeder gone, or one that is not a player when
// its occupation chooses, grows nothing and keeps n.
func growIntoByOccupation(s *script.Script, n *script.NPC) bool {
	c0 := n.Slot(script.SlotCreature0)
	var id int32
	if s.Rnd(100) > 50 {
		if s.Rnd(100) < 50 {
			id = n.IntParam("silhouette1")
		} else {
			id = n.IntParam("silhouette2")
		}
	} else {
		p, ok := c0.(*script.Player)
		if !ok {
			return false
		}
		if p.SameOccupation("@fighter") {
			id = n.IntParam("silhouette_ex")
		} else {
			id = n.IntParam("silhouette_ex2")
		}
	}
	if c0 == nil {
		return false
	}
	growInto(s, n, id, c0)
	n.DeleteMe()
	return true
}

// GrowthStep3Aggressive is WizardGrowthStep3Aggressive: a spellcasting
// beast grown twice, hating its feeder as it appears and striking it by
// distance, and striking the players and summons it sees. Fed by its
// feeder it may grow into one of its silhouettes, likelier with each feed,
// the feeder's occupation choosing for half the draws. Hit otherwise by a
// player or summon while it hates most someone else, it blasts around
// itself. Told a grown beast appeared near it (event 10018), it grows into
// its last silhouette one time in five.
func GrowthStep3Aggressive() script.Script {
	base := ddMagic2AggressiveHooks()
	return script.Script{Dir: growthDir, Behavior: true, NPCs: []int32{21461, 21463, 21465, 21467, 21480, 21482, 21484, 21486, 21499, 21501, 21503, 21505}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			n.SetInt(script.SlotIntAI4, 1)
			n.SetSlot(script.SlotCreature0, s.Object(n.Int(script.SlotParam1)))
			n.SetInt(script.SlotIntAI3, n.Int(script.SlotParam2))
			if c0 := n.Slot(script.SlotCreature0); c0 != nil {
				n.HateList().Add(c0, 100)
			}
			n.SetInt(script.SlotIntAI2, 0)
			if s.Rnd(100) < 33 {
				castByType(n, n, npc.SkillTypeSelfBuff, 1000000)
			}
			growthStrikeFeeder(n)
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			if e.Skill.ID != 0 && int32(e.Skill.ID) == e.NPC.Int(script.SlotIntAI3) {
				growthFed(s, e, 2014, 20)
				return
			}
			base.Attacked(s, e)
			growthBlast(e.NPC, e.Attacker)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			n := e.NPC
			if strings.EqualFold(e.Name, "2001") && (n.Int(script.SlotIntAI4) == 1 || n.Int(script.SlotIntAI4) == 3) && !n.Dead() {
				if !growIntoByOccupation(s, n) {
					return ""
				}
			}
			return base.Timer(s, e)
		},
		OnScriptEvent: func(s *script.Script, e script.ScriptEvent) {
			n := e.NPC
			if e.EventID == 10018 && s.Rnd(100) < 20 {
				c0 := n.Slot(script.SlotCreature0)
				if c0 == nil {
					return
				}
				growInto(s, n, n.IntParam("silhouette_ex2"), c0)
				n.DeleteMe()
			}
			base.ScriptEvent(s, e)
		},
	})}
}

// GrowthStep4Aggressive is WizardGrowthStep4Aggressive: a fully grown
// spellcasting beast, attacking its feeder as it appears and striking it
// by distance, and striking the players and summons it sees, that tells
// the beasts within 700 it appeared (event 10018). Its first hit starts a
// 5 s fight timer; each tick, unless its top desire target hit it since
// the last, it may hold that target when the target holds none of the
// hold's effect, and out of a fight it stands down until hit again. Hit by
// a player or summon while it hates most someone else, it blasts around
// itself.
func GrowthStep4Aggressive() script.Script {
	base := ddMagic2AggressiveHooks()
	return script.Script{Dir: growthDir, Behavior: true, NPCs: []int32{21469, 21488, 21507, 21825, 21827, 21829}, Hooks: base.With(script.Hooks{
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
				castByType(n, n, npc.SkillTypeSelfBuff, 1000000)
			}
			growthStrikeFeeder(n)
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			base.Attacked(s, e)
			n, attacker := e.NPC, e.Attacker
			if script.Same(n.TopDesireTarget(), attacker) && n.Int(script.SlotIntAI1) == 1 {
				n.SetInt(script.SlotIntAI2, 1)
			} else {
				s.StartTimer("2001", n, nil, 5000*time.Millisecond)
				n.SetInt(script.SlotIntAI1, 1)
			}
			growthBlast(n, attacker)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			n := e.NPC
			if strings.EqualFold(e.Name, "2001") {
				if !fighting(n.Intention()) {
					n.SetInt(script.SlotIntAI1, 0)
					n.SetInt(script.SlotIntAI2, 0)
				} else if n.Int(script.SlotIntAI2) == 0 {
					if top := n.TopDesireTarget(); top != nil && s.Rnd(100) < 50 {
						if holdMagic, ok := n.SkillByType(npc.SkillTypeHoldMagic); ok && s.AbnormalLevel(top, holdMagic) <= 0 {
							n.AddCastDesire(top, holdMagic, 1000000)
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
