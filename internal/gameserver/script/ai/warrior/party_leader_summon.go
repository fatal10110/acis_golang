package warrior

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// PartyLeaderPhysicalSpecialSummon is PartyLeaderPhysicalSpecialSummon: a
// PartyLeaderPhysicalSpecial that, under half its HP, calls its
// silhouette beside it to attack the creature that hit it, at most twice
// and once per 5 s. Its created hook calls no parent, so it spawns with
// none of its privates.
func PartyLeaderPhysicalSpecialSummon() script.Script {
	return script.Script{Dir: partyLeaderDir, Behavior: true, NPCs: []int32{22029}, Hooks: PartyLeaderPhysicalSpecialSummonHooks()}
}

// PartyLeaderPhysicalSpecialSummonHooks are
// PartyLeaderPhysicalSpecialSummon's hooks, which its children derive
// theirs from.
func PartyLeaderPhysicalSpecialSummonHooks() script.Hooks {
	base := PartyLeaderPhysicalSpecialHooks()
	return base.With(script.Hooks{
		OnCreated: func(_ *script.Script, e script.Created) {
			e.NPC.SetInt(script.SlotIntAI0, 0)
			e.NPC.SetInt(script.SlotIntAI1, 0)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n := e.NPC
			if n.HPRatio() < 0.5 && n.Int(script.SlotIntAI0) < 2 && n.Int(script.SlotIntAI1) == 0 {
				i1 := s.Rnd(50) - 25
				i2 := s.Rnd(50) - 25
				at := script.LocOf(n)
				s.Spawner().CreateOnePrivateEx(n, n.IntParam("silhouette"), script.Loc{X: at.X + i1, Y: at.Y + i2, Z: at.Z}, 0, script.Params{P1: 1000, P2: e.Attacker.ObjectID()})
				n.SetInt(script.SlotIntAI0, n.Int(script.SlotIntAI0)+1)
				n.SetInt(script.SlotIntAI1, 1)
				s.StartTimer("6006", n, nil, 5000*time.Millisecond)
			}
			base.Attacked(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			if strings.EqualFold(e.Name, "6006") {
				e.NPC.SetInt(script.SlotIntAI1, 0)
			}
			return base.Timer(s, script.Timer{Name: e.Name, NPC: e.NPC})
		},
	}).Named("PartyLeaderPhysicalSpecialSummon")
}

// PartyLeaderPhysicalSpecialSummonAggressive is
// PartyLeaderPhysicalSpecialSummonAggressive: a
// PartyLeaderPhysicalSpecialSummon that attacks the players and summons it
// sees.
func PartyLeaderPhysicalSpecialSummonAggressive() script.Script {
	base := PartyLeaderPhysicalSpecialSummonHooks()
	return script.Script{Dir: partyLeaderDir, Behavior: true, NPCs: []int32{22032, 22038, 22039}, Hooks: base.With(script.Hooks{
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			if !e.Creature.Playable() {
				return
			}
			defaultnpc.TryToAttack(s, e.NPC, e.Creature)
			base.SeeCreature(s, e)
		},
	})}
}
