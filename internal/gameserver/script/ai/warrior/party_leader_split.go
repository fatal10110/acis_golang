package warrior

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// PartyLeaderSplit is a Warrior that splits: the first physical skill that
// hits it puts six of its silhouettes around where it stands, each turning
// on the attacker, and takes it out of the world.
func PartyLeaderSplit() script.Script {
	return script.Script{Dir: "ai/individual/Monster/WarriorBase/Warrior", Behavior: true, NPCs: []int32{22088}, Hooks: partyLeaderSplitHooks()}
}

// partyLeaderSplitHooks are PartyLeaderSplit's hooks, which its child
// derives its own from.
func partyLeaderSplitHooks() script.Hooks {
	base := WarriorHooks()
	return base.With(script.Hooks{
		OnCreated: func(_ *script.Script, e script.Created) {
			e.NPC.SetInt(script.SlotIntAI1, 0)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker, k := e.NPC, e.Attacker, e.Skill
			if k != (skill.Ref{}) && !s.Skill(k).Magic() && n.Int(script.SlotIntAI1) == 0 {
				silhouette := n.IntParam("silhouette")
				at := script.LocOf(n)
				x, y, z := at.X, at.Y, at.Z
				params := script.Params{P1: 1000, P2: attacker.ObjectID(), P3: 0}
				spawner := s.Spawner()
				spawner.CreateOnePrivateEx(n, silhouette, script.Loc{X: x, Y: y, Z: z}, 0, params)
				spawner.CreateOnePrivateEx(n, silhouette, script.Loc{X: x + 20, Y: y, Z: z}, 0, params)
				spawner.CreateOnePrivateEx(n, silhouette, script.Loc{X: x + 40, Y: y, Z: z}, 0, params)
				spawner.CreateOnePrivateEx(n, silhouette, script.Loc{X: x, Y: y + 20, Z: z}, 0, params)
				spawner.CreateOnePrivateEx(n, silhouette, script.Loc{X: x, Y: y + 40, Z: z}, 0, params)
				spawner.CreateOnePrivateEx(n, silhouette, script.Loc{X: x, Y: y + 60, Z: z}, 0, params)
				n.SetInt(script.SlotIntAI1, 1)
				n.DeleteMe()
			}
			base.Attacked(s, e)
		},
	}).Named("PartyLeaderSplit")
}

// PartyLeaderSplitAggressive is a PartyLeaderSplit that attacks the
// players and summons it sees.
func PartyLeaderSplitAggressive() script.Script {
	base := partyLeaderSplitHooks()
	hooks := base.With(script.Hooks{
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			if !e.Creature.Playable() {
				return
			}
			defaultnpc.TryToAttack(s, e.NPC, e.Creature)
			base.SeeCreature(s, e)
		},
	})
	return script.Script{Dir: "ai/individual/Monster/WarriorBase/Warrior", Behavior: true, NPCs: []int32{22094}, Hooks: hooks}
}
