package maker

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

// Benom is the maker of Rune castle's Benom: its first spawn is the
// dungeon gatekeeper, its second Benom himself, database-tracked. At
// start it registers on the siege of the castle its CastleID memo names
// (8 when the memo declares none), places the gatekeeper and, unless the
// siege takes registrations, brings Benom back when his row is saved
// alive. The siege's registrations closing bring Benom in, their opening
// deletes him, and while the siege is fought a tower check runs every 30
// seconds: once fewer than two life control towers
// stand, Benom hears script event 10100 and the gatekeeper 10101, and the
// check stops. Its NPCs never respawn on their own.
func Benom() script.Maker {
	// towerCheck holds the running tower check's ticker, none when no
	// check runs: the siege event hook and the check itself share it.
	towerCheck := &script.MapCell[struct{}, *sim.Ticker]{}
	base := defaultHooks()
	return script.Maker{MakerHooks: base.With(script.MakerHooks{
		OnStart: func(m *script.Maker, e script.MakerStart) {
			castle := m.Castle(int32(memoIntOr(e.Group, "CastleID", 8)))
			if castle != nil {
				castle.AddMakerEvent(m, e.Group)
			}
			if spawns := e.Group.Spawns(); len(spawns) > 1 {
				spawns[0].Spawn()
				if castle != nil && castle.SiegeStatus() != script.SiegeRegistrationOpened {
					spawns[1].LoadDBInfo()
				}
			}
		},
		OnNPCDBInfo: func(_ *script.Maker, e script.MakerSpawn) {
			spawnLiveRow(e.Spawn)
		},
		OnNPCDeleted: func(*script.Maker, script.MakerNPC) {},
		OnSiegeEvent: func(_ *script.Maker, e script.MakerSiege) {
			spawns := e.Group.Spawns()
			switch e.Status {
			case script.SiegeInProgress:
				stopTowerCheck(towerCheck)
				// The first check is due 30 seconds in: one at the start's
				// own instant would run before the start makes the castle's
				// control towers work, so it would find none standing.
				// Once they work, every placed life control tower stands.
				towerCheck.Set(struct{}{}, e.Group.Every(30000*time.Millisecond, func() { benomTowerCheck(e, towerCheck) }))
			case script.SiegeRegistrationOpened:
				if len(spawns) > 1 {
					spawns[1].DeleteNPCs()
				}
			case script.SiegeRegistrationOver:
				if len(spawns) > 1 && spawns[1].Spawned() < 1 {
					spawns[1].Spawn()
				}
			}
		},
	})}
}

// benomTowerCheck is one run of the tower check the siege e started. Once
// the siege is no longer fought the check stops. While fewer than two life
// control towers stand: Benom hears 10100 with 1, the gatekeeper then
// hears 10101 with 1 and the check stops; a Benom the spawn keeps none of
// is brought back from his row instead, and the check runs on.
func benomTowerCheck(e script.MakerSiege, towerCheck *script.MapCell[struct{}, *sim.Ticker]) {
	if e.Castle.SiegeStatus() != script.SiegeInProgress {
		stopTowerCheck(towerCheck)
		return
	}
	spawns := e.Group.Spawns()
	if e.Castle.AliveLifeTowers() >= 2 || len(spawns) <= 1 {
		return
	}
	c0 := spawns[1].FirstNPC()
	if c0 == nil {
		spawns[1].LoadDBInfo()
		return
	}
	c0.SendScriptEvent(10100, 1, 0)
	if c0 = spawns[0].FirstNPC(); c0 != nil {
		c0.SendScriptEvent(10101, 1, 0)
	}
	stopTowerCheck(towerCheck)
}

// stopTowerCheck stops the running tower check, if any.
func stopTowerCheck(towerCheck *script.MapCell[struct{}, *sim.Ticker]) {
	if t, ok := towerCheck.Delete(struct{}{}); ok {
		t.Stop()
	}
}
