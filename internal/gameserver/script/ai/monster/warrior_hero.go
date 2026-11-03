package monster

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// WarriorHero is a monster that comes for the creature its third spawn
// parameter names: it casts its hero skill at it and attacks it, says one
// of its seven lines after 8 s and leaves the world after 15 s.
func WarriorHero() script.Script {
	base := Hooks()
	return script.Script{Dir: "ai/individual/Monster", Behavior: true, NPCs: []int32{21260}, Hooks: base.With(script.Hooks{
		OnTimer: func(s *script.Script, e script.Timer) string {
			n := e.NPC
			if strings.EqualFold(e.Name, "3001") {
				n.DeleteMe()
			} else if strings.EqualFold(e.Name, "3002") {
				n.Say(script.NpcString(int32(1000434 + s.Rnd(7))))
			}
			return base.Timer(s, e)
		},
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			if c0 := s.Object(n.Int(script.SlotParam3)); c0 != nil {
				if heroSkill, ok := n.SkillByType(npc.SkillTypeHeroSkill); ok {
					n.AddCastDesire(c0, heroSkill, 1000000)
				}
				n.AddAttackDesire(c0, 1000)
			}
			s.StartTimer("3001", n, nil, 15000*time.Millisecond)
			s.StartTimer("3002", n, nil, 8000*time.Millisecond)
			base.Created(s, e)
		},
	})}
}
