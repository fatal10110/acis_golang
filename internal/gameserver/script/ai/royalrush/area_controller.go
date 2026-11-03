package royalrush

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// AreaController is RoyalRushAreaController, a charm of a sepulcher room
// that keeps casting its status effect skill on itself. Its "type" AI
// parameter (0 to 3) picks its lines and its delay: types 0, 1 and 3 shout
// as they appear and start casting after 2, 1 or 3 minutes; type 2 casts
// at once. Once casting, it casts again every 30 s and each time a cast
// ends; dying, it shouts its last line.
func AreaController() script.Script {
	base := DefaultNpcHooks()
	return script.Script{
		Dir: dir, Behavior: true,
		NPCs: []int32{18196, 18197, 18198, 18199, 18200, 18201, 18202, 18203, 18204, 18205, 18206, 18207, 18208, 18209, 18210, 18211},
		Hooks: base.With(script.Hooks{
			OnCreated: func(s *script.Script, e script.Created) {
				n := e.NPC
				switch n.IntParam("type") {
				case 0:
					s.StartTimer("3001", n, nil, 2*60*1000*time.Millisecond)
					n.Shout(script.NpcString(1010474))
				case 1:
					s.StartTimer("3001", n, nil, 60*1000*time.Millisecond)
					n.Shout(script.NpcString(1010473))
				case 2:
					castOnSelf(n, npc.SkillTypeStatusEffect)
					n.Shout(script.NpcString(1010472))
					s.StartTimer("3002", n, nil, 30*1000*time.Millisecond)
				case 3:
					s.StartTimer("3001", n, nil, 3*60*1000*time.Millisecond)
					n.Shout(script.NpcString(1010475))
				}

				base.Created(s, e)
			},
			OnTimer: func(s *script.Script, e script.Timer) string {
				n := e.NPC
				if strings.EqualFold(e.Name, "3001") {
					castOnSelf(n, npc.SkillTypeStatusEffect)
					switch n.IntParam("type") {
					case 0:
						n.Shout(script.NpcString(1010477))
						s.StartTimer("3002", n, nil, 1000*30*time.Millisecond)
					case 1:
						n.Shout(script.NpcString(1010476))
						s.StartTimer("3002", n, nil, 1000*30*time.Millisecond)
					case 2:
						n.Shout(script.NpcString(1010472))
					case 3:
						n.Shout(script.NpcString(1010478))
						s.StartTimer("3002", n, nil, 1000*30*time.Millisecond)
					}
				}
				if strings.EqualFold(e.Name, "3002") {
					castOnSelf(n, npc.SkillTypeStatusEffect)
					s.StartTimer("3002", n, nil, 1000*30*time.Millisecond)
				}

				return base.Timer(s, script.Timer{Name: e.Name, NPC: n})
			},
			OnMyDying: func(_ *script.Script, e script.MyDying) {
				n := e.NPC
				switch n.IntParam("type") {
				case 0:
					n.Shout(script.NpcString(1010481))
				case 1:
					n.Shout(script.NpcString(1010480))
				case 2:
					n.Shout(script.NpcString(1010479))
				case 3:
					n.Shout(script.NpcString(1010482))
				}
			},
			OnUseSkillFinished: func(_ *script.Script, e script.UseSkillFinished) {
				n := e.NPC
				if ref, ok := n.SkillByType(npc.SkillTypeStatusEffect); ok {
					n.AddCastDesireUnchecked(n, ref, 1000000)
				}
			},
		}),
	}
}
