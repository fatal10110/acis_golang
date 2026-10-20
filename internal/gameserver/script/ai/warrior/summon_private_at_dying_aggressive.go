package warrior

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// SummonPrivateAtDyingAggressive is WarriorSummonPrivateAtDyingAggressive:
// a WarriorAggressive that remembers the attacker it fights, and dying, at
// its SummonPrivateRate chance, calls its privates and names that attacker
// to the NPCs around every 1.5 seconds. Its timer hook answers no other
// timer.
func SummonPrivateAtDyingAggressive() script.Script {
	base := AggressiveHooks()
	return script.Script{Dir: aggressiveDir, Behavior: true, NPCs: []int32{21653, 21655, 21656, 21654, 21657, 21652}, Hooks: base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n := e.NPC
			if topDesireTarget := n.TopDesireTarget(); topDesireTarget != nil && script.Same(topDesireTarget, e.Attacker) {
				n.SetInt(script.SlotIntAI3, e.Attacker.ObjectID())
			}
			base.Attacked(s, e)
		},
		OnMyDying: func(s *script.Script, e script.MyDying) {
			n := e.NPC
			if int32(s.Rnd(100)) < n.IntParam("SummonPrivateRate") {
				defaultnpc.CreatePrivates(s, n)
				s.BroadcastScriptEvent(n, eventTarget, n.Int(script.SlotIntAI3), 500)
				s.StartTimer("5001", n, nil, 1500*time.Millisecond)
			}
			base.MyDying(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			if strings.EqualFold(e.Name, "5001") {
				s.BroadcastScriptEvent(e.NPC, eventTarget, e.NPC.Int(script.SlotIntAI3), 500)
				s.StartTimer("5001", e.NPC, nil, 1500*time.Millisecond)
				return base.Timer(s, e)
			}
			return ""
		},
	})}
}
