package royalrush

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// TreasureBox is RoyalRushTrasureBox, Halisha's treasure box: a
// RoyalRushAfflict that leaves the world 5 minutes after it spawns and,
// hit, wants to flee 150 from its attacker.
func TreasureBox() script.Script {
	base := AfflictHooks()
	return script.Script{Dir: dir, Behavior: true, NPCs: []int32{18256}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			s.Spawner().ScheduleDespawn(e.NPC, 300000*time.Millisecond)

			base.Created(s, e)
		},
		OnAttacked: func(_ *script.Script, e script.Attacked) {
			e.NPC.AddFleeDesire(e.Attacker, 150, 200)
		},
	})}
}
