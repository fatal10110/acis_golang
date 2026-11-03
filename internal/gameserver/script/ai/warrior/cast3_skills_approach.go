package warrior

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// cast3ApproachDir is the directory of WarriorCast3SkillsApproach and its
// children.
const cast3ApproachDir = "ai/individual/Monster/WarriorBase/Warrior/WarriorCast3SkillsApproach"

// selfBuffCreated arms, as n appears, the 10 second self buff check
// (timer "2001"), and casts n's self buff at once; a transformed NPC
// instead casts it 3 seconds later (timer "2002") and, called by the NPC
// it replaced, stuns and attacks the creature that one named.
func selfBuffCreated(s *script.Script, n *script.NPC) {
	s.StartTimerAtFixedRate("2001", n, nil, 10000*time.Millisecond, 10000*time.Millisecond)
	if n.IntParam("IsTransform") == 0 {
		castByType(n, n, npc.SkillTypeSelfBuff)
		return
	}
	s.StartTimer("2002", n, nil, 3000*time.Millisecond)
	if n.Int(script.SlotParam1) == 1000 {
		if c0 := s.Object(n.Int(script.SlotParam2)); c0 != nil {
			n.AddCastDesire(c0, skill.Ref{ID: 4663, Level: 1}, 10000)
			n.AddAttackDesire(c0, 500)
		}
	}
}

// selfBuffTimer answers the self buff timers: "2001" recasts the self buff
// out of a fight when the NPC lost it, "2002" casts it.
func selfBuffTimer(s *script.Script, e script.Timer) {
	n := e.NPC
	if strings.EqualFold(e.Name, "2001") {
		if n.Intention() != script.IntentionAttack {
			if selfBuff, _ := n.SkillByType(npc.SkillTypeSelfBuff); s.AbnormalLevel(n, selfBuff) <= 0 {
				castByType(n, n, npc.SkillTypeSelfBuff)
			}
		}
	} else if strings.EqualFold(e.Name, "2002") {
		castByType(n, n, npc.SkillTypeSelfBuff)
	}
}

// Cast3SkillsApproach is WarriorCast3SkillsApproach: a Warrior that keeps
// its self buff up and, against the player it hates most or one that
// attacks its clan, blasts around itself when that player is close and
// strikes it with its physical special skill.
func Cast3SkillsApproach() script.Script {
	return script.Script{Dir: cast3ApproachDir, Behavior: true, NPCs: cast3ApproachIDs(), Hooks: Cast3SkillsApproachHooks()}
}

// cast3ApproachIDs are WarriorCast3SkillsApproach's own NPCs.
func cast3ApproachIDs() []int32 {
	return []int32{21177, 21180, 21183, 21186, 21198, 21201, 21204, 21207, 21323, 21009, 21799, 21318, 21315, 21320}
}

// Cast3SkillsApproachHooks are WarriorCast3SkillsApproach's hooks, which
// its children derive theirs from.
func Cast3SkillsApproachHooks() script.Hooks {
	base := WarriorHooks()
	return base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			selfBuffCreated(s, e.NPC)
			base.Created(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			selfBuffTimer(s, e)
			return base.Timer(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if attacker.Playable() {
				if script.Same(n.MostHated(), attacker) {
					if n.Distance2D(attacker) < 200 && s.Rnd(100) < 33 {
						castByType(n, n, npc.SkillTypeSelfRangeDDMagic)
					}
					if s.Rnd(100) < 33 {
						castPhysicalSpecial(n, attacker)
					}
				}
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called, attacker := e.Called, e.Attacker
			if attacker.Playable() && called.LifeTime() > 7 {
				if script.Same(called.MostHated(), attacker) && called.Distance2D(attacker) < 200 && s.Rnd(100) < 33 {
					castByType(called, called, npc.SkillTypeSelfRangeDDMagic)
				}
				if s.Rnd(100) < 33 {
					castPhysicalSpecial(called, attacker)
				}
			}
			base.ClanAttacked(s, e)
		},
	}).Named("WarriorCast3SkillsApproach")
}

// Cast3SkillsApproachAggressive is WarriorCast3SkillsApproachAggressive: a
// WarriorCast3SkillsApproach that attacks the players and summons it sees,
// out of a fight in its territory most likely striking first with its
// physical special skill.
func Cast3SkillsApproachAggressive() script.Script {
	return script.Script{Dir: cast3ApproachDir, Behavior: true, NPCs: cast3ApproachAggressiveIDs(), Hooks: Cast3SkillsApproachAggressiveHooks()}
}

// Cast3SkillsApproachAggressiveHooks are
// WarriorCast3SkillsApproachAggressive's hooks, which its children derive
// theirs from.
func Cast3SkillsApproachAggressiveHooks() script.Hooks {
	base := Cast3SkillsApproachHooks()
	return base.With(script.Hooks{
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			if !c.Playable() {
				return
			}
			if n.LifeTime() > 7 && n.Intention() != script.IntentionAttack && n.InMyTerritory() {
				if s.Rnd(100) < 33 {
					castPhysicalSpecial(n, c)
				}
			}
			defaultnpc.TryToAttack(s, n, c)
			base.SeeCreature(s, e)
		},
	}).Named("WarriorCast3SkillsApproachAggressive")
}

// cast3ApproachAggressiveIDs are WarriorCast3SkillsApproachAggressive's own
// NPCs.
func cast3ApproachAggressiveIDs() []int32 {
	return []int32{
		21309, 20814, 21273, 20674, 20811, 21061, 20820, 20816, 21116, 20629, 21402, 21034, 21641, 20829,
		20827, 21649, 20853, 20854, 20861, 21087, 21089, 22010, 22110, 22114,
	}
}
