package warrior

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

const castingHealDir = "ai/individual/Monster/WarriorBase/Warrior/WarriorCastingHeal"

// CastingHeal is WarriorCastingHeal: a Warrior that, on a draw under 33,
// wants its heal on itself when a hit leaves it below 70% of its HP, and
// on a clan member a creature hits while it is not attacking.
func CastingHeal() script.Script {
	return script.Script{Dir: castingHealDir, Behavior: true, NPCs: []int32{20686}, Hooks: castingHealHooks()}
}

// castingHealHooks are WarriorCastingHeal's hooks, which its children
// derive theirs from.
func castingHealHooks() script.Hooks {
	base := WarriorHooks()
	return base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n := e.NPC
			if s.Rnd(100) < 33 && n.HPRatio() < 0.7 {
				addCastByType(n, n, npc.SkillTypeMagicHeal, 1000000)
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called := e.Called
			if s.Rnd(100) < 33 && called.Intention() != script.IntentionAttack {
				addCastByType(called, e.Caller, npc.SkillTypeMagicHeal, 1000000)
			}
			base.ClanAttacked(s, e)
		},
	}).Named("WarriorCastingHeal")
}

// CastingHealAggressive is WarriorCastingHealAggressive: a
// WarriorCastingHeal that attacks the players and summons it sees. It
// binds no NPC of its own.
func CastingHealAggressive() script.Script {
	base := castingHealHooks()
	return script.Script{Dir: castingHealDir, Behavior: true, Hooks: base.With(script.Hooks{
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			if !e.Creature.Playable() {
				return
			}
			defaultnpc.TryToAttack(s, e.NPC, e.Creature)
			base.SeeCreature(s, e)
		},
	})}
}
