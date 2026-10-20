package royalrush

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// Afflict is RoyalRushAfflict: a watchman of the graves that follows the
// players it sees and flees whoever hits it. Created, it draws one of
// three plans, kept in its first int slot: plan 0 casts its first afflict
// skill on itself after 5 s and deletes itself once the cast ends; plans 1
// and 2 cast the second or third afflict skill after 5 s and again each
// time a cast ends, and look around 5 s after the first cast.
func Afflict() script.Script {
	return script.Script{
		Dir: dir, Behavior: true,
		NPCs:  []int32{18244, 18245, 18246, 18247, 18248, 18249, 18250, 18251, 18252, 18253, 18254, 18255},
		Hooks: AfflictHooks(),
	}
}

// AfflictHooks are RoyalRushAfflict's hooks, which its children derive
// theirs from.
func AfflictHooks() script.Hooks {
	base := DefaultNpcHooks()
	return base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			switch s.Rnd(3) {
			case 0:
				n.SetInt(script.SlotIntAI0, 0)
				s.StartTimer("3001", n, nil, 5000*time.Millisecond)
			case 1:
				n.SetInt(script.SlotIntAI0, 1)
				s.StartTimer("3002", n, nil, 5000*time.Millisecond)
			case 2:
				n.SetInt(script.SlotIntAI0, 2)
				s.StartTimer("3002", n, nil, 5000*time.Millisecond)
			}

			n.LookNeighbor()

			base.Created(s, e)
		},
		OnNoDesire: func(_ *script.Script, e script.NoDesire) {
			e.NPC.AddWanderDesire(5, 5)
		},
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			if _, ok := e.Creature.(*script.Player); ok && s.Rnd(100) < 50 {
				e.NPC.AddFollowDesire(e.Creature, 100)
			}
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			e.NPC.AddFleeDesire(e.Attacker, e.NPC.MaxDriftRange(), 100)

			base.Attacked(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			n := e.NPC
			if strings.EqualFold(e.Name, "3001") {
				castOnSelf(n, npc.SkillTypeAfflictSkill1)
			}
			if strings.EqualFold(e.Name, "3002") {
				switch n.Int(script.SlotIntAI0) {
				case 1:
					castOnSelf(n, npc.SkillTypeAfflictSkill2)
				case 2:
					castOnSelf(n, npc.SkillTypeAfflictSkill3)
				}

				s.StartTimer("3003", n, nil, 5000*time.Millisecond)
			}
			if strings.EqualFold(e.Name, "3003") {
				n.LookNeighbor()
			}

			return base.Timer(s, e)
		},
		OnUseSkillFinished: func(_ *script.Script, e script.UseSkillFinished) {
			n := e.NPC
			switch n.Int(script.SlotIntAI0) {
			case 0:
				n.DeleteMe()
			case 1:
				castOnSelf(n, npc.SkillTypeAfflictSkill2)
			case 2:
				castOnSelf(n, npc.SkillTypeAfflictSkill3)
			}
		},
	}).Named("RoyalRushAfflict")
}

// castOnSelf makes n want to cast on itself the skill its template lists
// under t; a template listing none casts nothing.
func castOnSelf(n *script.NPC, t npc.SkillType) {
	if ref, ok := n.SkillByType(t); ok {
		n.AddCastDesire(n, ref, 1000000)
	}
}
