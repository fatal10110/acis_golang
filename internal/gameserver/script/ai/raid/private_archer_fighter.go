package raid

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// PrivateArcher is RaidPrivateArcher: a raid private that, hit from closer
// than 100 while no such hit is pending, runs from that attacker 2 s later,
// as far as its drift range, at 10000. One time in 20 it wants its long
// range physical special at 1000000 on its top desire target when that
// target, a playable, hits its party or casts in its sight.
func PrivateArcher() script.Script {
	return script.Script{Dir: "ai/individual/Monster/RaidPrivate/RaidPrivateArcher", Behavior: true, NPCs: privateArcherIDs(), Hooks: privateArcherHooks()}
}

// privateArcherIDs are RaidPrivateArcher's own NPCs.
func privateArcherIDs() []int32 {
	return []int32{
		25005, 25011, 25027, 25036, 25042, 25058, 25068, 25086, 25107, 25116, 25132, 25147, 25150, 25171,
		25186, 25212, 25218, 25257, 25261, 25270, 25300, 25344, 25364, 25389, 25413, 25446, 25477, 25491,
		25502, 29098,
	}
}

// privateArcherHooks are RaidPrivateArcher's hooks: i_ai2 marks a pending
// run, c_ai1 the attacker it runs from.
func privateArcherHooks() script.Hooks {
	base := PrivateStandardHooks()
	return base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			e.NPC.SetInt(script.SlotIntAI2, 0)
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n := e.NPC
			if n.Int(script.SlotIntAI2) == 0 && n.Distance2D(e.Attacker) < 100 {
				s.StartTimer("100002", n, nil, 2000*time.Millisecond)
				n.SetInt(script.SlotIntAI2, 1)
				n.SetSlot(script.SlotCreature1, e.Attacker)
			}
			base.Attacked(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			if strings.EqualFold(e.Name, "100002") {
				n := e.NPC
				n.AddFleeDesire(n.Slot(script.SlotCreature1), n.MaxDriftRange(), 10000)
				n.SetInt(script.SlotIntAI2, 0)
			}
			return base.Timer(s, e)
		},
		OnPartyAttacked: func(s *script.Script, e script.PartyAttacked) {
			if e.Target.Playable() {
				if top := e.Called.TopDesireTarget(); top != nil {
					if script.Same(e.Target, top) && s.Rnd(20) < 1 {
						castByType(e.Called, e.Target, npc.SkillTypeLongRangePhysicalSpecialA)
					}
				}
			}
			base.PartyAttacked(s, e)
		},
		OnSeeSpell: func(s *script.Script, e script.SeeSpell) {
			if top := e.NPC.TopDesireTarget(); top != nil && script.Same(e.Caster, top) && s.Rnd(20) < 1 {
				castByType(e.NPC, e.Caster, npc.SkillTypeLongRangePhysicalSpecialA)
			}
			base.SeeSpell(s, e)
		},
	}).Named("RaidPrivateArcher")
}

// PrivateFighter is RaidPrivateFighter: a raid private that, one time in
// 15, wants its physical special at 1000000 on its top desire target when
// that target, a playable, hits its party or casts in its sight.
func PrivateFighter() script.Script {
	return script.Script{Dir: "ai/individual/Monster/RaidPrivate/RaidPrivateFighter", Behavior: true, NPCs: privateFighterIDs(), Hooks: privateFighterHooks()}
}

// privateFighterIDs are RaidPrivateFighter's own NPCs.
func privateFighterIDs() []int32 {
	return []int32{
		25003, 25014, 25024, 25034, 25039, 25048, 25055, 25062, 25065, 25072, 25077, 25084, 25091, 25093,
		25097, 25104, 25111, 25114, 25121, 25123, 25130, 25139, 25141, 25153, 25157, 25177, 25181, 25191,
		25193, 25201, 25206, 25210, 25216, 25222, 25225, 25231, 25243, 25246, 25251, 25254, 25265, 25268,
		25275, 25285, 25294, 25298, 25304, 25307, 25310, 25317, 25324, 25326, 25348, 25353, 25356, 25358,
		25361, 25367, 25370, 25374, 25376, 25379, 25381, 25384, 25386, 25393, 25397, 25402, 25405, 25406,
		25408, 25409, 25411, 25419, 25425, 25427, 25428, 25430, 25435, 25439, 25443, 25461, 25462, 25464,
		25466, 25469, 25472, 25474, 25482, 25485, 25486, 25489, 25494, 25497, 25499, 29041, 25507, 25505,
		25510, 25516, 25332,
	}
}

// privateFighterHooks are RaidPrivateFighter's hooks.
func privateFighterHooks() script.Hooks {
	base := PrivateStandardHooks()
	return base.With(script.Hooks{
		OnPartyAttacked: func(s *script.Script, e script.PartyAttacked) {
			if e.Target.Playable() {
				if top := e.Called.TopDesireTarget(); top != nil {
					if script.Same(e.Target, top) && s.Rnd(15) < 1 {
						castByType(e.Called, e.Target, npc.SkillTypePhysicalSpecialA)
					}
				}
			}
			base.PartyAttacked(s, e)
		},
		OnSeeSpell: func(s *script.Script, e script.SeeSpell) {
			if top := e.NPC.TopDesireTarget(); top != nil {
				if script.Same(e.Caster, top) && s.Rnd(15) < 1 {
					castByType(e.NPC, e.Caster, npc.SkillTypePhysicalSpecialA)
				}
			}
			base.SeeSpell(s, e)
		},
	}).Named("RaidPrivateFighter")
}
