package manager

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	castledata "github.com/fatal10110/acis_golang/internal/gameserver/model/residence/castle"
)

// SpawnTower places the control tower t of the castle castleID as a
// standalone spawn of its own, from its tower template (npc.TowerTemplate),
// at its position on the ground height below it, belonging to the castle,
// and returns it. It stands dead until the castle's siege makes it work.
// A tower that cannot be placed is skipped with a warning and reports
// false.
func (n *Npcs) SpawnTower(castleID int, t castledata.ControlTower) (*npc.Folk, bool) {
	tmpl := npc.TowerTemplate(t.Type == castledata.TowerLifeControl, t.HP, t.PDef, t.MDef)
	live, err := n.spawnStandalone(tmpl, t.Position.X, t.Position.Y, t.Position.Z, 0, 0, nil, castleID)
	if err != nil {
		n.log.Warn().Err(err).Str("tower", t.Alias).Int("castle", castleID).Msg("castle control tower: cannot place npc")
		return nil, false
	}
	f, ok := live.(*npc.Folk)
	return f, ok
}

// towerRuntime is what inst works with when it is a control tower: the
// siege of the castle it belongs to, and its working form's template; the
// zero value for any other NPC.
func (n *Npcs) towerRuntime(inst *npc.Instance) npc.TowerRuntime {
	id, ok := npc.TowerWorkingNpcID(inst)
	if !ok {
		return npc.TowerRuntime{}
	}
	var rt npc.TowerRuntime
	rt.Working, _ = n.templates.Get(id)
	if n.guarded != nil {
		rt.Siege, _ = n.guarded(inst).(npc.TowerSiege)
	}
	return rt
}
