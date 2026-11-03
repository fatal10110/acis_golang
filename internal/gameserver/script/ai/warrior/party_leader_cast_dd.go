package warrior

import (
	"slices"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

const partyLeaderDir = "ai/individual/Monster/WarriorBase/Warrior/PartyLeaderWarrior"

// PartyLeaderCastDD is PartyLeaderCastDD: a PartyLeaderWarrior that may
// cast its DD magic on its most hated attacker standing close, and on a
// clan member's attacker standing away, and once hit, every 5 s while it
// fights, on the creature it hates most. It binds no NPC of its own.
func PartyLeaderCastDD() script.Script {
	return script.Script{Dir: partyLeaderDir, Behavior: true, Hooks: PartyLeaderCastDDHooks()}
}

// PartyLeaderCastDDHooks are PartyLeaderCastDD's hooks, which its
// children derive theirs from.
func PartyLeaderCastDDHooks() script.Hooks {
	base := PartyLeaderHooks()
	return base.With(script.Hooks{
		OnTimer: func(s *script.Script, e script.Timer) string {
			if strings.EqualFold(e.Name, "2001") {
				n := e.NPC
				if !fighting(n.Intention()) {
					n.SetInt(script.SlotIntAI1, 0)
					n.SetInt(script.SlotIntAI2, 0)
					return base.Timer(s, e)
				}
				if n.Int(script.SlotIntAI2) == 0 {
					if ddMagic, ok := n.SkillByType(npc.SkillTypeDDMagic); ok && s.Rnd(100) < 50 {
						if mostHated := n.MostHated(); mostHated != nil {
							n.AddCastDesire(mostHated, ddMagic, 1000000)
						}
					}
				}
				s.StartTimer("2001", n, e.Player, 5000*time.Millisecond)
				n.SetInt(script.SlotIntAI2, 0)
			}
			return base.Timer(s, e)
		},
		OnCreated: func(s *script.Script, e script.Created) {
			e.NPC.SetInt(script.SlotIntAI1, 0)
			e.NPC.SetInt(script.SlotIntAI2, 0)
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			mostHated := n.MostHated()
			armFightTimer(s, n, attacker, mostHated)
			if attacker.Playable() && script.Same(mostHated, attacker) {
				if ddMagic, ok := n.SkillByType(npc.SkillTypeDDMagic); ok && s.Rnd(100) < 33 && n.Distance2D(attacker) < 100 {
					n.AddCastDesire(attacker, ddMagic, 1000000)
				}
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called, attacker := e.Called, e.Attacker
			if attacker.Playable() && called.LifeTime() > 7 {
				if ddMagic, ok := called.SkillByType(npc.SkillTypeDDMagic); ok && s.Rnd(100) < 33 && called.Distance2D(attacker) > 100 {
					called.AddCastDesire(attacker, ddMagic, 1000000)
				}
			}
			base.ClanAttacked(s, e)
		},
	}).Named("PartyLeaderCastDD")
}

// PartyLeaderCastDDAggressive is PartyLeaderCastDDAggressive: a
// PartyLeaderCastDD that attacks the players and summons it sees.
func PartyLeaderCastDDAggressive() script.Script {
	return script.Script{Dir: partyLeaderDir, Behavior: true, NPCs: []int32{20963, 20749, 20969, 20944, 20761, 20747, 20771, 20977, 22028}, Hooks: PartyLeaderCastDDAggressiveHooks()}
}

// PartyLeaderCastDDAggressiveHooks are PartyLeaderCastDDAggressive's
// hooks, which its children derive theirs from.
func PartyLeaderCastDDAggressiveHooks() script.Hooks {
	base := PartyLeaderCastDDHooks()
	return base.With(script.Hooks{
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			if !e.Creature.Playable() {
				return
			}
			defaultnpc.TryToAttack(s, e.NPC, e.Creature)
			base.SeeCreature(s, e)
		},
	}).Named("PartyLeaderCastDDAggressive")
}

// PartyLeaderCastDDMagicStoneAggressive is
// PartyLeaderCastDDMagicStoneAggressive: a Ketra or Varka
// PartyLeaderCastDDAggressive that petrifies an attacker allied with its
// own faction, and a player who helps one, and ignores such a player it
// sees.
func PartyLeaderCastDDMagicStoneAggressive() script.Script {
	base := PartyLeaderCastDDAggressiveHooks()
	return script.Script{Dir: partyLeaderDir, Behavior: true, NPCs: []int32{21345, 21371}, Hooks: base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			if defaultnpc.MaybeCastPetrify(e.NPC, e.Attacker) {
				return
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			if defaultnpc.MaybeCastPetrify(e.Called, e.Attacker) {
				return
			}
			base.ClanAttacked(s, e)
		},
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n := e.NPC
			p := script.ActingPlayer(e.Creature)
			if p == nil {
				return
			}
			if p.AlliedWithVarka() && slices.Contains(n.Clans(), "varka_silenos_clan") {
				return
			}
			if p.AlliedWithKetra() && slices.Contains(n.Clans(), "ketra_orc_clan") {
				return
			}
			base.SeeCreature(s, e)
		},
		OnSeeSpell: func(s *script.Script, e script.SeeSpell) {
			if k := s.Skill(e.Skill); k.AggroPoints() > 0 && !k.Offensive() && defaultnpc.MaybeCastPetrify(e.NPC, e.Caster) {
				return
			}
			base.SeeSpell(s, e)
		},
	})}
}
