package warrior

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

const priestDir = "ai/individual/Monster/WarriorBase/Warrior/WarriorPriest"

// Priest is WarriorPriest: a Warrior that, while it does not attack, wants
// its buff on a creature it sees that is neither a player nor a summon and
// does not hold that buff's kind of effect above level 0. Every 20 seconds
// it looks around again.
func Priest() script.Script {
	return script.Script{Dir: priestDir, Behavior: true, NPCs: []int32{20925, 20929, 20931, 21646, 20934}, Hooks: priestHooks()}
}

// priestHooks are WarriorPriest's hooks, which its children derive theirs
// from.
func priestHooks() script.Hooks {
	base := WarriorHooks()
	return base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			s.StartTimerAtFixedRate("5001", e.NPC, nil, 20000*time.Millisecond, 20000*time.Millisecond)
			base.Created(s, e)
		},
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			if !c.Playable() && n.Intention() != script.IntentionAttack {
				if buff, ok := n.SkillByType(npc.SkillTypeBuff); ok && s.AbnormalLevel(c, buff) <= 0 {
					n.AddCastDesire(c, buff, 1000000)
				}
			}
			base.SeeCreature(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			if strings.EqualFold(e.Name, "5001") {
				e.NPC.LookNeighbor()
			}
			return base.Timer(s, e)
		},
	}).Named("WarriorPriest")
}

// PriestAggressive is WarriorPriestAggressive: a WarriorPriest that
// attacks the players and summons it sees.
func PriestAggressive() script.Script {
	base := priestHooks()
	return script.Script{Dir: priestDir, Behavior: true, NPCs: []int32{21306}, Hooks: base.With(script.Hooks{
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			if !e.Creature.Playable() {
				return
			}
			defaultnpc.TryToAttack(s, e.NPC, e.Creature)
			base.SeeCreature(s, e)
		},
	})}
}
