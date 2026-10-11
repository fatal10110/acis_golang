package warrior

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// Warrior is the Warrior behavior: a monster that wanders, plays its
// social animations and shouts its lines, and hates by the damage dealt to
// it.
func Warrior() script.Script {
	return script.Script{Dir: "ai/individual/Monster/WarriorBase/Warrior", Behavior: true, NPCs: warriorIDs(), Hooks: WarriorHooks()}
}

// warriorIDs are Warrior's own NPCs.
func warriorIDs() []int32 {
	return []int32{
		20001, 20009, 20023, 20028, 20032, 20065, 20069, 20073, 20091, 20095, 20097, 20107, 20116, 20119,
		20139, 20142, 20151, 20152, 20163, 20175, 20189, 20255, 20293, 20297, 20298, 20299, 20301, 20303,
		20307, 20309, 20318, 20321, 20326, 20328, 20330, 20334, 20336, 20337, 20348, 20358, 20360, 20381,
		20382, 20391, 20392, 20393, 20401, 20405, 20406, 20407, 20418, 20440, 20441, 20443, 20444, 20450,
		20451, 20465, 20477, 20488, 20492, 20510, 20531, 20533, 20535, 20653, 20742, 20744, 21011, 21094,
		21098, 21119, 21124, 21126, 21127, 21132, 21136, 21265, 27001, 27002, 27006, 27007, 27008, 27009,
		27018, 27019, 27042, 27046, 27047, 27048, 27049, 27050, 27051, 27052, 27053, 27055, 27063, 27066,
		27069, 27078, 27111, 27115, 27199, 27200, 18003, 18231, 18232, 18233, 18234, 18235, 18236, 18237,
		18238, 18239, 18240, 18241, 18242, 27154, 27121, 20074, 20077, 20079, 27182, 27183, 20456, 20388,
		20373, 20370, 27079, 20317, 20480, 20400, 20389, 20390, 20532, 20775, 20568, 27038, 20349, 20236,
		20272, 20316, 20251, 20252, 20203, 27153, 27101, 20013, 20019, 20281, 20536, 20538, 20544, 20539,
		20537, 20442, 27155, 27316, 21017, 20008, 20010, 20014, 20433, 20594, 20007, 20651, 20540, 27098,
		20121, 20250, 20003, 20322, 20319, 20528, 20325, 20327, 20320, 27179, 20226, 20525, 20335, 27214,
		27215, 27216, 20640, 20147, 27058, 20308, 20777, 20286, 20403, 20274, 20004, 20005, 27061, 27067,
		27064, 27059, 20468, 20473, 20470, 20206, 21117, 20476, 20474, 20475, 20481, 27021, 20487, 20030,
		20027, 20357, 27116, 20577, 27122, 27123, 20580, 20919, 20922, 20363, 20232, 27022, 20369, 20365,
		27117, 27016, 20089, 20575, 27130, 27031, 20547, 20031, 20130, 27054, 20096, 20131, 20598, 20106,
		27317, 20778, 20779, 20311, 20312, 20359, 20021, 20534, 27086, 20551, 20054, 27171, 20112, 20114,
		20039, 27090, 20514, 20168, 27097, 27096, 27095, 27100, 20515, 20104, 27003, 27004, 27005, 20509,
		20200, 20040, 20630, 20573, 20570, 20583, 20035, 20560, 20062, 27070, 18342, 20192, 27077, 20446,
		20448, 27020, 20132, 20553, 20120, 20529, 20545, 20530, 18339, 22001, 22089, 22090, 22097, 22098,
		27319, 27320, 27321, 27322, 27201,
	}
}

// damageHate returns the attack desire weight of amount, before its
// multiplier: amount scaled down by the NPC's level and up by its hate
// ratio for the attacker, in percent.
func damageHate(n *script.NPC, amount, hateRatio float64) float64 {
	return ((1.0 * amount) / float64(n.Level()+7)) + ((hateRatio / 100) * ((1.0 * amount) / float64(n.Level()+7)))
}

// WarriorHooks are Warrior's hooks, which its children derive theirs from.
func WarriorHooks() script.Hooks {
	base := BaseHooks()
	return base.With(script.Hooks{
		OnNoDesire: func(_ *script.Script, e script.NoDesire) {
			e.NPC.AddWanderDesire(5, 5)
		},
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			if shoutMsg1 := n.IntParam("ShoutMsg1"); shoutMsg1 > 0 {
				n.Shout(script.NpcString(shoutMsg1))
			}
			if n.IntParam("MoveAroundSocial") > 0 || n.IntParam("ShoutMsg2") > 0 || n.IntParam("ShoutMsg3") > 0 {
				s.StartTimerAtFixedRate("1001", n, nil, 10000*time.Millisecond, 10000*time.Millisecond)
			}
			base.Created(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			if strings.EqualFold(e.Name, "1001") {
				n := e.NPC
				switch n.Intention() {
				case script.IntentionWander, script.IntentionIdle, script.IntentionMoveTo:
					if !n.Dead() && n.HPRatio() > 0.4 {
						moveAroundSocial2 := n.IntParam("MoveAroundSocial2")
						moveAroundSocial1 := n.IntParam("MoveAroundSocial1")
						moveAroundSocial := n.IntParam("MoveAroundSocial")
						if moveAroundSocial2 > 0 && s.Rnd(100) < 20 {
							n.AddSocialDesire(3, int(moveAroundSocial2*1000/30), 50)
						} else if moveAroundSocial1 > 0 && s.Rnd(100) < 20 {
							n.AddSocialDesire(2, int(moveAroundSocial1*1000/30), 50)
						} else if moveAroundSocial > 0 && s.Rnd(100) < 20 {
							n.AddSocialDesire(1, int(moveAroundSocial*1000/30), 50)
						}
						if shoutMsg2 := n.IntParam("ShoutMsg2"); shoutMsg2 > 0 && s.Rnd(1000) < 17 {
							n.Shout(script.NpcString(shoutMsg2))
						}
					}
				case script.IntentionAttack:
					if shoutMsg3 := n.IntParam("ShoutMsg3"); shoutMsg3 > 0 && s.Rnd(100) < 10 {
						n.Shout(script.NpcString(shoutMsg3))
					}
				}
			}
			return base.Timer(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker, damage := e.NPC, e.Attacker, float64(e.Damage)
			initialHateRatio := defaultnpc.HateRatio(n, attacker)
			if attacker.Playable() {
				n.AddAttackDesire(attacker, damageHate(n, damage, initialHateRatio)*100)
			}
			if mostHated := n.MostHated(); mostHated != nil {
				if n.GeoPathFailCount() >= 10 && script.Same(attacker, mostHated) && n.HPRatio() != 1.0 {
					n.AbortAll()
					n.InstantTeleportTo(attacker)
				}
				if i0 := s.AbnormalLevel(n, skill.Ref{ID: 1201, Level: 1}); i0 >= 0 && n.Distance2D(attacker) > 40 {
					if !n.CanAttack(mostHated) {
						n.StopHate(mostHated)
					}
					if attacker.Playable() {
						n.AddAttackDesire(attacker, damageHate(n, damage, initialHateRatio)*100)
					}
				}
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(_ *script.Script, e script.ClanAttacked) {
			called, attacker := e.Called, e.Attacker
			if called.LifeTime() > 7 {
				mostHated := called.MostHated()
				if mostHated != nil && script.Same(attacker, mostHated) && called.HPRatio() != 1.0 && called.GeoPathFailCount() >= 10 {
					called.AbortAll()
					called.InstantTeleportTo(attacker)
				}
				if attacker.Playable() {
					hateRatio := damageHate(called, float64(e.Damage), defaultnpc.HateRatio(called, attacker))
					called.AddAttackDesire(attacker, float64(int(hateRatio*100)))
				}
			}
		},
		OnSeeSpell: func(s *script.Script, e script.SeeSpell) {
			n, caster := e.NPC, e.Caster
			mostHated := n.MostHated()
			k := s.Skill(e.Skill)
			if (k.AggroPoints() > 0 || k.Power(nil) > 0 || k.Offensive()) && n.Intention() == script.IntentionAttack && targets(e.Targets, mostHated) {
				i0 := max(max(float64(k.AggroPoints()), k.Power(e.Targets[0])), 20)
				n.AddAttackDesire(caster, damageHate(n, i0, defaultnpc.HateRatio(n, caster))*150)
			}
			if script.Same(caster, mostHated) && n.HPRatio() != 1.0 && n.GeoPathFailCount() >= 10 {
				n.AbortAll()
				n.InstantTeleportTo(caster)
			}
		},
		OnMyDying: func(s *script.Script, e script.MyDying) {
			if shoutMsg4 := e.NPC.IntParam("ShoutMsg4"); shoutMsg4 > 0 && s.Rnd(100) < 30 {
				e.NPC.Shout(script.NpcString(shoutMsg4))
			}
		},
	})
}

// targets reports whether c is among list; no creature is among none.
func targets(list []script.Creature, c script.Creature) bool {
	for _, t := range list {
		if script.Same(t, c) {
			return true
		}
	}
	return false
}
