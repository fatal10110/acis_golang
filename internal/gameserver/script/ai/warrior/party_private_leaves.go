package warrior

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

const partyPrivateDir = "ai/individual/Monster/WarriorBase/Warrior/PartyPrivateWarrior"

// FreyaBear is FreyaBear: a PartyPrivateWarrior that answers its master's
// shouted target by dropping all its hate and attacking it with weight
// 1000000, heals its master on event 10034 and leaves the world on event
// 11039.
func FreyaBear() script.Script {
	base := PartyPrivateHooks()
	return script.Script{Dir: partyPrivateDir, Behavior: true, NPCs: []int32{22103}, Hooks: base.With(script.Hooks{
		OnScriptEvent: func(s *script.Script, e script.ScriptEvent) {
			n := e.NPC
			if n.Dead() {
				return
			}
			switch e.EventID {
			case 10002:
				master := n.Master()
				if master == nil {
					return
				}
				if !script.Same(s.Object(e.Arg1), master) {
					return
				}
				flagged := s.Object(master.Int(script.SlotFlag))
				if top := n.TopDesireTarget(); top != nil && script.Same(flagged, top) {
					return
				}
				shouts := privateShouts()
				n.Say(script.NpcString(shouts[s.Rnd(len(shouts))]))
				if flagged != nil {
					n.CleanAllHate()
					n.AddAttackDesire(flagged, 1000000)
				}
			case 10034:
				if master := n.Master(); master != nil {
					castByType(n, master, npc.SkillTypeMagicHeal)
				}
			case 11039:
				n.DeleteMe()
			}
		},
	})}
}

// OrfenPrivateRaikelLeos is OrfenPrivateRaikelLeos: a PartyPrivateWarrior
// that, every 10 s while its live master has not died, rejoins it when
// the master's flag changed or it strays more than 3000 away. It curses
// an attacker more than 8 levels above it and mutes such a caster, and may
// strike its party's attacker with a mortal blow.
func OrfenPrivateRaikelLeos() script.Script {
	base := PartyPrivateHooks()
	return script.Script{Dir: partyPrivateDir, Behavior: true, NPCs: []int32{29016}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			s.StartTimerAtFixedRate("2001", n, nil, 10000*time.Millisecond, 10000*time.Millisecond)
			if master := n.Master(); master != nil {
				n.SetInt(script.SlotIntAI0, master.Int(script.SlotFlag))
			}
			n.SetInt(script.SlotIntAI1, 1)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			if strings.EqualFold(e.Name, "2001") {
				n := e.NPC
				if master := n.Master(); master != nil && !master.Dead() && n.Int(script.SlotIntAI1) == 1 {
					orfenFlag := master.Int(script.SlotFlag)
					if orfenFlag != n.Int(script.SlotIntAI0) || n.Distance2D(master) > 3000 {
						n.SetInt(script.SlotIntAI0, orfenFlag)
						n.TeleportTo(master)
						n.RemoveAllAttackDesire()
					}
				}
			}
			return base.Timer(s, e)
		},
		OnAttacked: func(_ *script.Script, e script.Attacked) {
			if e.Attacker.Level() > e.NPC.Level()+8 {
				e.NPC.AddCastDesire(e.Attacker, skill.Ref{ID: 4515, Level: 1}, 1000000)
			}
		},
		OnSeeSpell: func(s *script.Script, e script.SeeSpell) {
			if e.Caster.Level() > e.NPC.Level()+8 {
				e.NPC.AddCastDesire(e.Caster, skill.Ref{ID: 4215, Level: 1}, 1000000)
				return
			}
			base.SeeSpell(s, e)
		},
		OnPartyAttacked: func(s *script.Script, e script.PartyAttacked) {
			if s.Rnd(100) < 5 {
				e.Called.AddCastDesire(e.Target, skill.Ref{ID: 4067, Level: 4}, 1000000)
			}
			base.PartyAttacked(s, e)
		},
		OnPartyDied: func(_ *script.Script, e script.PartyDied) {
			if master := e.Called.Master(); master != nil && script.Same(e.Caller, master) {
				e.Called.SetInt(script.SlotIntAI1, 0)
			}
		},
	})}
}

// PartyPrivateCannibaliAggressive is PartyPrivateCannibaliAggressive: a
// PartyPrivateWarrior that spawns with no timers, attacks the players and
// summons it sees, walks back toward its live master when hit more than
// 200 away from it, may strike with either physical special skill on a
// hit, and leaves the world on event 10028.
func PartyPrivateCannibaliAggressive() script.Script {
	base := PartyPrivateHooks()
	return script.Script{Dir: partyPrivateDir, Behavior: true, NPCs: []int32{22112}, Hooks: base.With(script.Hooks{
		// It spawns with none of its parents' timers.
		OnCreated: func(*script.Script, script.Created) {},
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			if !e.Creature.Playable() {
				return
			}
			defaultnpc.TryToAttack(s, e.NPC, e.Creature)
			base.SeeCreature(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if master := n.Master(); master != nil && !master.Dead() && n.Distance2D(master) > 200 {
				n.AddMoveToDesire(script.LocOf(master), 1000)
			}
			if s.Rnd(100) < 33 {
				castByType(n, attacker, npc.SkillTypePhysicalSpecial1)
			}
			if s.Rnd(100) < 33 {
				castByType(n, attacker, npc.SkillTypePhysicalSpecial2)
			}
			base.Attacked(s, e)
		},
		OnScriptEvent: func(_ *script.Script, e script.ScriptEvent) {
			if e.EventID == 10028 {
				e.NPC.DeleteMe()
			}
		},
	})}
}
