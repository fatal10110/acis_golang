package frintezza

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// FollowerDummy (29053), the stand-in Frintezza's follower leaves as it
// transforms. Appearing it wants to cast skill 5004 level 1 on itself,
// weight 1000000, and it leaves the world 10.2 s later.
func FollowerDummy() script.Script {
	base := defaultnpc.New().Hooks
	return script.Script{Dir: dir, Behavior: true, NPCs: []int32{29053}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			e.NPC.AddCastDesire(e.NPC, skill.Ref{ID: 5004, Level: 1}, 1000000)
			s.StartTimer("1001", e.NPC, nil, 10200*time.Millisecond)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			if strings.EqualFold(e.Name, "1001") {
				e.NPC.DeleteMe()
			}
			return base.Timer(s, e)
		},
	})}
}

// FrintezzaSeeker (29059), the tomb's watcher. Placed from a live
// spawn_data row, it leaves the world unless Frintezza (global memo key 4)
// is alive at db value 1 or less. While he is, the first player or summon
// it sees makes it tell him event 0, arg1 2, and leave the world.
func FrintezzaSeeker() script.Script {
	base := defaultnpc.New().Hooks
	return script.Script{Dir: dir, Behavior: true, NPCs: []int32{29059}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			if !n.DBLoaded() {
				return
			}
			c0 := memoNPC(s, "4")
			if c0 != nil && !c0.Dead() {
				if c0.DBValue() > 1 {
					n.DeleteMe()
				}
			} else {
				n.DeleteMe()
			}
		},
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			if !e.Creature.Playable() {
				return
			}
			c0 := memoNPC(s, "4")
			if c0 != nil && !c0.Dead() && c0.DBValue() <= 1 {
				s.SendScriptEvent(c0, 0, 2, 0)
				e.NPC.DeleteMe()
			}
		},
	})}
}
