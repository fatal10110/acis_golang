package wizard

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// addHate adds weight to c on hateList, 300 more when the list is empty.
func addHate(hateList script.HateList, c script.Creature, weight float64) {
	if hateList.Empty() {
		hateList.Add(c, weight+300)
	} else {
		hateList.Add(c, weight)
	}
}

// refreshHate answers the 1002 timer: it refreshes hateList and drops the
// creatures over 1000 away.
func refreshHate(hateList script.HateList) {
	hateList.Refresh()
	hateList.RemoveIfOutOfRange(1000)
}

// waitOutSilence marks a silenced n as waiting and checks again 10 s
// later, on the 1003 timer.
func waitOutSilence(s *script.Script, n *script.NPC) {
	if n.Muted() {
		n.SetInt(script.SlotIntAI0, 1)
		s.StartTimer("1003", n, nil, 10000*time.Millisecond)
	}
}

// silenceRetry answers the 1003 timer: still silenced, n checks again 10 s
// later with player; free again, it drops its attack desires and answers
// the creature it hates most as a hit of 100.
func silenceRetry(s *script.Script, n *script.NPC, player *script.Player, hateList script.HateList) {
	if n.Muted() {
		s.StartTimer("1003", n, player, 10000*time.Millisecond)
		return
	}
	n.RemoveAllAttackDesire()
	n.SetInt(script.SlotIntAI0, 0)
	if mostHated := hateList.MostHated(); mostHated != nil {
		s.Attacked(s, script.Attacked{NPC: n, Attacker: mostHated, Damage: 100})
	}
}
