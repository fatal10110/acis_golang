package script

import (
	"fmt"
	"time"
)

// RespawnDelay returns the respawn delay, in seconds, the spawn that placed
// n declares, without its random part; 0 for an NPC no spawn placed. Only
// a hostile NPC's is read yet: any other panics.
func (s *Spawner) RespawnDelay(n *NPC) int32 {
	return int32(s.population().RespawnDelay(n.hostile("read the respawn delay")) / time.Second)
}

// ScheduleRespawn asks the spawn of n, a master's private, to bring it back
// once d has passed: around its master, or at its own point when it was
// created at one. It comes back only when its corpse has left the world by
// then and its master is alive. Of two requests for one private, the first
// to come due brings it back. A d that is not positive asks nothing. A
// private whose spawn declares no respawn delay comes back when asked
// before its corpse decayed, no longer linked to its master. A private
// whose spawn is gone asks nothing. Any other NPC panics, as an
// npcmaker's NPCs come back through their maker.
func (s *Spawner) ScheduleRespawn(n *NPC, d time.Duration) {
	if err := s.population().ScheduleRespawn(n.hostile("schedule a respawn"), d); err != nil {
		panic(fmt.Sprintf("script: schedule the respawn of npc %d: %v", n.NpcID(), err))
	}
}

// DecayMe hides n: it leaves the world with no decayed hook and is never
// respawned, and an npcmaker keeps counting it. It takes no more turns and
// its behavior timers stop. A hidden or decayed NPC is left as it is. Only
// a hostile NPC hides yet: any other panics.
func (s *Spawner) DecayMe(n *NPC) {
	s.population().DecayMe(n.hostile("hide"))
}
