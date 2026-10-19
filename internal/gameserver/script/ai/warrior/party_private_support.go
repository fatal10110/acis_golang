package warrior

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// PartyPrivateCastEnchant is PartyPrivateCastEnchant: a PartyPrivateWarrior
// that, the first time a draw under 33 passes on a player's hit while it
// is above half its HP, wants to cast its buff on itself.
func PartyPrivateCastEnchant() script.Script {
	base := PartyPrivateHooks()
	return script.Script{Dir: partyPrivateDir, Behavior: true, NPCs: []int32{20976, 20769, 20765, 22099}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			e.NPC.SetInt(script.SlotIntAI0, 0)
			e.NPC.SetInt(script.SlotIntAI1, 0)
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n := e.NPC
			if e.Attacker.Playable() && n.Int(script.SlotIntAI1) == 0 && s.Rnd(100) < 33 && n.HPRatio() > 0.5 {
				castByType(n, n, npc.SkillTypeBuff)
				n.SetInt(script.SlotIntAI1, 1)
			}
			base.Attacked(s, e)
		},
	})}
}

// PartyPrivateCastHeal is PartyPrivateCastHeal: a PartyPrivateWarrior that
// may heal itself, or a party member that was hit, under 70% of its HP.
func PartyPrivateCastHeal() script.Script {
	return script.Script{Dir: partyPrivateDir, Behavior: true, NPCs: []int32{20774, 20990, 20762}, Hooks: PartyPrivateCastHealHooks()}
}

// PartyPrivateCastHealHooks are PartyPrivateCastHeal's hooks, which its
// children derive theirs from.
func PartyPrivateCastHealHooks() script.Hooks {
	base := PartyPrivateHooks()
	return base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			if n := e.NPC; s.Rnd(100) < 33 && n.HPRatio() < 0.7 {
				castByType(n, n, npc.SkillTypeMagicHeal)
			}
			base.Attacked(s, e)
		},
		OnPartyAttacked: func(s *script.Script, e script.PartyAttacked) {
			if s.Rnd(100) < 33 && e.Caller.HPRatio() < 0.7 {
				castByType(e.Called, e.Caller, npc.SkillTypeMagicHeal)
			}
			base.PartyAttacked(s, e)
		},
	}).Named("PartyPrivateCastHeal")
}

// PartyPrivateCastHealCurse is PartyPrivateCastHealCurse: a
// PartyPrivateCastHeal that may also lay its debuff on its most hated
// player, whether that player hit it or its party.
func PartyPrivateCastHealCurse() script.Script {
	base := PartyPrivateCastHealHooks()
	return script.Script{Dir: partyPrivateDir, Behavior: true, NPCs: []int32{21060, 20962, 20968, 20988, 20940, 20768, 20748, 21077, 21083, 21092, 22130}, Hooks: base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if attacker.Playable() {
				if mostHated := n.MostHated(); mostHated != nil {
					debuffMostHated(s, n, attacker, mostHated)
				}
			}
			base.Attacked(s, e)
		},
		OnPartyAttacked: func(s *script.Script, e script.PartyAttacked) {
			called, target := e.Called, e.Target
			if target.Playable() {
				if mostHated := called.MostHated(); mostHated != nil {
					debuffMostHated(s, called, target, mostHated)
				}
			}
			base.PartyAttacked(s, e)
		},
	})}
}

// PartyPrivatePriest is PartyPrivatePriest: a PartyPrivateWarrior that
// looks around every 20 s and, seeing its master while it is not
// attacking, wants to cast its buff on a master that holds none of it.
func PartyPrivatePriest() script.Script {
	base := PartyPrivateHooks()
	return script.Script{Dir: partyPrivateDir, Behavior: true, NPCs: []int32{20970, 20958, 20960, 20945, 20979, 20942, 20936, 20757, 20951}, Hooks: base.With(script.Hooks{
		OnTimer: func(s *script.Script, e script.Timer) string {
			if strings.EqualFold(e.Name, "5001") {
				e.NPC.LookNeighbor()
			}
			return base.Timer(s, e)
		},
		OnCreated: func(s *script.Script, e script.Created) {
			s.StartTimerAtFixedRate("5001", e.NPC, nil, 20000*time.Millisecond, 20000*time.Millisecond)
			base.Created(s, e)
		},
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n := e.NPC
			if master := n.Master(); master != nil && script.Same(e.Creature, master) && n.Intention() != script.IntentionAttack {
				if buff, ok := n.SkillByType(npc.SkillTypeBuff); s.AbnormalLevel(master, buff) <= 0 && ok {
					n.AddCastDesire(master, buff, 1000000)
				}
			}
			base.SeeCreature(s, e)
		},
	})}
}
