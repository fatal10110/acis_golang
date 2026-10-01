package network

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// adminHide answers //hide: gm turns invisible, or visible again.
//
// Turning invisible takes gm off the grid and puts it back, so everyone
// around forgets gm and is shown it again, drawn invisible to all but game
// masters; gm's UserInfo goes out in between. Turning visible only resends
// gm's UserInfo and CharInfo. Either way gm's summon is taken off the grid,
// its status resent, and put back, so the players around see it or stop
// seeing it.
func (l *GameClientLink) adminHide(gm *livePlayer, _ string) {
	if !gm.Invisible() {
		gm.SetInvisible(true)
		l.leaveGrid(gm)
		l.broadcastCharacterInfo(gm)
		l.rejoinGrid(gm)
	} else {
		gm.SetInvisible(false)
		l.broadcastCharacterInfo(gm)
	}

	if l.world == nil {
		return
	}
	obj, ok := l.world.Summon(gm.ObjectID())
	if !ok {
		return
	}
	actor, ok := obj.(*summon.Actor)
	if !ok {
		return
	}
	l.world.Leave(actor)
	l.broadcastSummonStatus(actor)
	l.world.Rejoin(actor)
	l.broadcastSummonSpawnRelation(gm, actor)
}

// leaveGrid takes live off the grid where it stands: it leaves its zones,
// drops a selection it no longer sees while its neighbors still see it, and
// everything around forgets it as it forgets them.
func (l *GameClientLink) leaveGrid(live *livePlayer) {
	l.leaveZones(live)
	if l.world == nil {
		return
	}
	if selected := live.Target(); selected != nil && selected.ObjectID() != live.ObjectID() && world.Knows(live, selected) {
		live.forgetTarget(selected)
	}
	l.world.Leave(live)
}

// rejoinGrid puts live, taken off the grid by leaveGrid, back on it where it
// stands: everything around discovers it as it discovers them, and it
// enters its zones again. A submerged live leaves and re-enters its water
// zone too, which restarts its breath countdown (#3235).
func (l *GameClientLink) rejoinGrid(live *livePlayer) {
	if l.world != nil {
		l.world.Rejoin(live)
	}
	l.rejoinZones(live)
}
