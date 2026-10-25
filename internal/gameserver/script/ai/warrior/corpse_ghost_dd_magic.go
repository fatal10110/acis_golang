package warrior

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

const corpseGhostDDMagicDir = "ai/individual/Monster/WarriorBase/Warrior/WarriorCorpseGhostDDMagic"

// CorpseGhostDDMagic is WarriorCorpseGhostDDMagic: a Warrior that casts
// its self buff at its creation and, every 10 seconds while it fights no
// one, again when it lost it, and that, on a draw under 33, wants its
// damage magic on the player it hates most when hit by that one from
// farther than 100, or on the attacker of its clan farther than 100 while
// it does not fight.
func CorpseGhostDDMagic() script.Script {
	return script.Script{Dir: corpseGhostDDMagicDir, Behavior: true, NPCs: []int32{21556, 21579}, Hooks: corpseGhostDDMagicHooks()}
}

// corpseGhostDDMagicHooks are WarriorCorpseGhostDDMagic's hooks, which its
// child derives its own from.
func corpseGhostDDMagicHooks() script.Hooks {
	base := WarriorHooks()
	return base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			s.StartTimerAtFixedRate("2001", n, nil, 10000*time.Millisecond, 10000*time.Millisecond)
			castByType(n, n, npc.SkillTypeSelfBuff)
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if attacker.Playable() {
				if script.Same(n.MostHated(), attacker) && n.Distance2D(attacker) > 100 && s.Rnd(100) < 33 {
					castByType(n, attacker, npc.SkillTypeDDMagic)
				}
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called, attacker := e.Called, e.Attacker
			if attacker.Playable() && called.LifeTime() > 7 && called.Intention() != script.IntentionAttack && called.Distance2D(attacker) > 100 && s.Rnd(100) < 33 {
				castByType(called, attacker, npc.SkillTypeDDMagic)
			}
			base.ClanAttacked(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			if strings.EqualFold(e.Name, "2001") {
				n := e.NPC
				if n.Intention() != script.IntentionAttack {
					selfBuff, _ := n.SkillByType(npc.SkillTypeSelfBuff)
					if s.AbnormalLevel(n, selfBuff) <= 0 {
						castByType(n, n, npc.SkillTypeSelfBuff)
					}
				}
			}
			return base.Timer(s, e)
		},
	}).Named("WarriorCorpseGhostDDMagic")
}

// CorpseGhostDDMagicAggressive is WarriorCorpseGhostDDMagicAggressive: a
// WarriorCorpseGhostDDMagic that attacks the players and summons it sees
// and, settled in its territory and fighting no one, may want its damage
// magic on one it sees farther than 100.
func CorpseGhostDDMagicAggressive() script.Script {
	base := corpseGhostDDMagicHooks()
	hooks := base.With(script.Hooks{
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			if !c.Playable() {
				return
			}
			if n.LifeTime() > 7 && n.Intention() != script.IntentionAttack && n.InMyTerritory() {
				if s.Rnd(100) < 33 && n.Distance2D(c) > 100 {
					castByType(n, c, npc.SkillTypeDDMagic)
				}
			}
			defaultnpc.TryToAttack(s, n, c)
			base.SeeCreature(s, e)
		},
	})
	return script.Script{Dir: corpseGhostDDMagicDir, Behavior: true, NPCs: []int32{21562}, Hooks: hooks}
}
