package warrior

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// partyPrivatePhysicalSpecialDir is the directory of
// PartyPrivatePhysicalSpecial's children.
const partyPrivatePhysicalSpecialDir = "ai/individual/Monster/WarriorBase/Warrior/PartyPrivateWarrior/PartyPrivatePhysicalSpecial"

// PartyPrivatePhysicalSpecial is PartyPrivatePhysicalSpecial: a
// PartyPrivateWarrior that may strike a player or summon with its physical
// special skill when it hits it while being the creature it most wants to
// act on, and a party member's attacker while it is settled and not
// attacking.
func PartyPrivatePhysicalSpecial() script.Script {
	return script.Script{Dir: partyPrivateDir, Behavior: true, NPCs: partyPrivatePhysicalSpecialIDs(), Hooks: PartyPrivatePhysicalSpecialHooks()}
}

// partyPrivatePhysicalSpecialIDs are PartyPrivatePhysicalSpecial's own
// NPCs.
func partyPrivatePhysicalSpecialIDs() []int32 {
	return []int32{
		20971, 20772, 20961, 20965, 20759, 21518, 20955, 20995, 20954, 20949, 20957, 20967, 21436, 20946,
		20937, 20746, 20938, 20984, 20992, 20993, 21073, 21076, 21079, 21082, 21091, 21519, 20952, 22027,
		22083,
	}
}

// PartyPrivatePhysicalSpecialHooks are PartyPrivatePhysicalSpecial's hooks,
// which its children derive theirs from.
func PartyPrivatePhysicalSpecialHooks() script.Hooks {
	base := PartyPrivateHooks()
	return base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if attacker.Playable() {
				if script.Same(n.TopDesireTarget(), attacker) && s.Rnd(100) < 33 {
					castByType(n, attacker, npc.SkillTypePhysicalSpecial)
				}
			}
			base.Attacked(s, e)
		},
		OnPartyAttacked: func(s *script.Script, e script.PartyAttacked) {
			called, target := e.Called, e.Target
			if target.Playable() && called.LifeTime() > 7 && called.Intention() != script.IntentionAttack && s.Rnd(100) < 33 {
				castByType(called, target, npc.SkillTypePhysicalSpecial)
			}
			base.PartyAttacked(s, e)
		},
	}).Named("PartyPrivatePhysicalSpecial")
}

// PartyPrivatePhysicalSpecialHeal is PartyPrivatePhysicalSpecialHeal: a
// PartyPrivatePhysicalSpecial that may heal itself when hit at half its HP
// or below, and, told of a hit on its party, the member hit or its master
// at half their HP or below.
func PartyPrivatePhysicalSpecialHeal() script.Script {
	base := PartyPrivatePhysicalSpecialHooks()
	return script.Script{Dir: partyPrivatePhysicalSpecialDir, Behavior: true, NPCs: []int32{22122}, Hooks: base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n := e.NPC
			if n.HPRatio() <= 0.5 && s.Rnd(100) < 33 {
				castByType(n, n, npc.SkillTypeMagicHeal)
			}
			base.Attacked(s, e)
		},
		OnPartyAttacked: func(s *script.Script, e script.PartyAttacked) {
			caller, called := e.Caller, e.Called
			if caller.HPRatio() <= 0.5 && !script.Same(called, caller) && s.Rnd(100) < 33 {
				castByType(called, caller, npc.SkillTypeMagicHeal)
			}
			if master := called.Master(); master != nil && master.HPRatio() <= 0.5 && s.Rnd(100) < 33 {
				castByType(called, master, npc.SkillTypeMagicHeal)
			}
			base.PartyAttacked(s, e)
		},
	})}
}

// PartyPrivatePhysicalSpecialPowerShot is
// PartyPrivatePhysicalSpecialPowerShot: a PartyPrivatePhysicalSpecial
// that, hit from closer than 100, runs from that attacker 2 s later; it
// takes up no other close attacker until it has run.
func PartyPrivatePhysicalSpecialPowerShot() script.Script {
	base := PartyPrivatePhysicalSpecialHooks()
	return script.Script{Dir: partyPrivatePhysicalSpecialDir, Behavior: true, NPCs: []int32{20985, 20760, 20978, 20770, 20750, 20756}, Hooks: base.With(script.Hooks{
		OnTimer: func(s *script.Script, e script.Timer) string {
			n := e.NPC
			if strings.EqualFold(e.Name, "100002") {
				n.AddFleeDesire(n.Slot(script.SlotCreature1), n.MaxDriftRange(), 10000)
				n.SetInt(script.SlotIntAI2, 0)
			}
			return base.Timer(s, e)
		},
		OnCreated: func(s *script.Script, e script.Created) {
			e.NPC.SetInt(script.SlotIntAI2, 0)
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if n.Int(script.SlotIntAI2) == 0 && n.Distance2D(attacker) < 100 {
				s.StartTimer("100002", n, nil, 2000*time.Millisecond)
				n.SetInt(script.SlotIntAI2, 1)
				n.SetSlot(script.SlotCreature1, attacker)
			}
			base.Attacked(s, e)
		},
	})}
}

// PartyPrivatePrimeval is PartyPrivatePrimeval: a
// PartyPrivatePhysicalSpecial of the primeval isle. It turns on the player
// a lair shout names when it listens to shouts, follows its live master
// or else walks back to its spawn point when it wants nothing, and, when
// aggressive, attacks the players and summons it sees.
func PartyPrivatePrimeval() script.Script {
	base := PartyPrivatePhysicalSpecialHooks()
	return script.Script{Dir: partyPrivatePhysicalSpecialDir, Behavior: true, NPCs: []int32{22204, 22201, 22209, 22197, 22212}, Hooks: base.With(script.Hooks{
		OnScriptEvent: func(s *script.Script, e script.ScriptEvent) {
			n := e.NPC
			if e.EventID == eventShoutTarget {
				if c0 := s.Object(e.Arg1); isPlayer(c0) && n.IntParam("BroadCastReception") == 1 {
					n.RemoveAllAttackDesire()
					n.AddAttackDesire(c0, 100)
				}
			}
			base.ScriptEvent(s, e)
		},
		OnNoDesire: func(_ *script.Script, e script.NoDesire) {
			n := e.NPC
			if master := n.Master(); master != nil && !master.Dead() {
				n.AddFollowDesire(master, 5)
			} else {
				n.AddMoveToDesire(n.SpawnLocation(), 30)
			}
		},
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			if n.IntParam("ag_type") == 1 {
				if !c.Playable() {
					return
				}
				defaultnpc.TryToAttack(s, n, c)
			}
		},
	})}
}

// hereticZealotPartyDied is the party-died hook of the heretic zealots up
// to its parent call: its master's death marks it (i_ai0) and has it want
// its second physical special skill on the creature it most wants to act
// on.
func hereticZealotPartyDied(e script.PartyDied) {
	caller, called := e.Caller, e.Called
	if script.Same(caller, called.Master()) {
		called.SetInt(script.SlotIntAI0, 1)
		if topDesireTarget := called.TopDesireTarget(); topDesireTarget != nil {
			castByType(called, topDesireTarget, npc.SkillTypePhysicalSpecial2)
		}
	}
}

// PartyPrivateHereticZealot is PartyPrivateHereticZealot: a
// PartyPrivatePhysicalSpecial that keeps to its territory and follows its
// master's calls as the heretic privates do. Its master's death has it
// strike with its second physical special skill, and that strike landing
// kills it.
func PartyPrivateHereticZealot() script.Script {
	base := PartyPrivatePhysicalSpecialHooks()
	return script.Script{Dir: partyPrivatePhysicalSpecialDir, Behavior: true, NPCs: []int32{22161, 22169, 22172, 22190}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			e.NPC.SetInt(script.SlotIntAI4, 0)
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			if n := e.NPC; hereticIdle(n) && n.Int(script.SlotIntAI4) == 0 {
				n.SetInt(script.SlotIntAI4, 1)
			}
			base.Attacked(s, e)
		},
		OnPartyAttacked: func(s *script.Script, e script.PartyAttacked) {
			if called := e.Called; hereticIdle(called) && called.Int(script.SlotIntAI4) == 0 {
				called.SetInt(script.SlotIntAI4, 1)
			}
			base.PartyAttacked(s, e)
		},
		OnOutOfTerritory: hereticOutOfTerritory,
		OnScriptEvent:    hereticScriptEvent,
		OnPartyDied: func(s *script.Script, e script.PartyDied) {
			hereticZealotPartyDied(e)
			base.PartyDied(s, e)
		},
		OnUseSkillFinished: func(_ *script.Script, e script.UseSkillFinished) {
			if n := e.NPC; isSkillOfType(n, e.Skill, npc.SkillTypePhysicalSpecial2) && e.Success {
				n.DoDie(n)
			}
		},
	})}
}

// PartyPrivateHereticZealot2 is PartyPrivateHereticZealot2: a
// PartyPrivatePhysicalSpecial that keeps to its territory and follows its
// master's calls as the heretic privates do. Once its master is dead it
// answers every hit by a player or summon on itself, and a third of those
// on its party, with its second physical special skill.
func PartyPrivateHereticZealot2() script.Script {
	base := PartyPrivatePhysicalSpecialHooks()
	return script.Script{Dir: partyPrivatePhysicalSpecialDir, Behavior: true, NPCs: []int32{22156, 22160, 22164, 22170, 22189}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			e.NPC.SetInt(script.SlotIntAI0, 0)
			e.NPC.SetInt(script.SlotIntAI4, 0)
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if n.Int(script.SlotIntAI4) == 0 && hereticIdle(n) {
				n.SetInt(script.SlotIntAI4, 1)
			}
			if attacker.Playable() && n.Int(script.SlotIntAI0) == 1 {
				if topDesireTarget := n.TopDesireTarget(); topDesireTarget != nil {
					castByType(n, topDesireTarget, npc.SkillTypePhysicalSpecial2)
				} else {
					castByType(n, attacker, npc.SkillTypePhysicalSpecial2)
				}
			}
			base.Attacked(s, e)
		},
		OnPartyAttacked: func(s *script.Script, e script.PartyAttacked) {
			called, target := e.Called, e.Target
			if called.Int(script.SlotIntAI4) == 0 && hereticIdle(called) {
				called.SetInt(script.SlotIntAI4, 1)
			}
			if target.Playable() && called.Int(script.SlotIntAI0) == 1 {
				if topDesireTarget := called.TopDesireTarget(); topDesireTarget != nil {
					if s.Rnd(100) < 33 {
						castByType(called, topDesireTarget, npc.SkillTypePhysicalSpecial2)
					}
				} else if s.Rnd(100) < 33 {
					castByType(called, target, npc.SkillTypePhysicalSpecial2)
				}
			}
			base.PartyAttacked(s, e)
		},
		OnOutOfTerritory: hereticOutOfTerritory,
		OnScriptEvent:    hereticScriptEvent,
		OnPartyDied: func(s *script.Script, e script.PartyDied) {
			hereticZealotPartyDied(e)
			base.PartyDied(s, e)
		},
	})}
}
