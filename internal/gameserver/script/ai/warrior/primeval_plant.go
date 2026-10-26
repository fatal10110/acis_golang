package warrior

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// PrimevalPlant is PrimevalPlant: a WarriorHold plant of the primeval isle
// that every 15 seconds, by the chance its AI parameter
// ProbSelfRangeDeBuff1 sets, wants its self range debuff. It neither
// fights back when hit nor heeds script events.
func PrimevalPlant() script.Script {
	base := HoldHooks()
	return script.Script{Dir: "ai/individual/Monster/WarriorBase/Warrior/WarriorHold", Behavior: true, NPCs: []int32{18345, 18346}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			s.StartTimerAtFixedRate("5001", e.NPC, nil, 15000*time.Millisecond, 15000*time.Millisecond)
		},
		OnAttacked:    func(*script.Script, script.Attacked) {},
		OnScriptEvent: func(*script.Script, script.ScriptEvent) {},
		OnTimer: func(s *script.Script, e script.Timer) string {
			n := e.NPC
			if strings.EqualFold(e.Name, "5001") {
				if int32(s.Rnd(100)) <= n.IntParam("ProbSelfRangeDeBuff1") {
					addCastByType(n, n, npc.SkillTypeSelfRangeDebuff1, 1000000)
				}
			}
			return base.Timer(s, e)
		},
	})}
}
