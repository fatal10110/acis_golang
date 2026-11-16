package olympiad

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/admin"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/restart"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestStadiumLoginSendsGameMasterToTown pins the login's own stadium check
// (EnterWorld.java, Olympiad.playerInStadia): a player entering the world
// inside a stadium is sent to town right after the login's SkillCoolTime,
// with no game master exemption. The stadium's entry rule lets a game master
// stay, so this check is the only teleport in its burst.
func TestStadiumLoginSendsGameMasterToTown(t *testing.T) {
	t.Parallel()
	form, err := zone.NewCuboid(-500, 500, -500, 500, -1000, 1000)
	if err != nil {
		t.Fatalf("stadium form: %v", err)
	}
	stadium := zone.NewOlympiad(1, form)
	stadium.AddSpawn(zone.SpawnNormal, location.Location{X: 0, Y: 0, Z: gameservertest.SpawnZ})
	zones := zone.NewIndex()
	zones.Add(stadium)
	town := location.Location{X: 3000, Y: 3000, Z: gameservertest.SpawnZ}
	// The class spawn (10, 20) lies in map region (20, 18).
	restarts := &restart.Table{Points: []restart.Point{{Name: "town", Points: []location.Location{town}, MapRegions: []location.Point{{X: 20, Y: 18}}}}}
	levels, err := admin.NewData([]admin.AccessLevel{{Level: 0, Name: "User"}, {Level: 7, Name: "Master", IsGM: true}}, nil)
	if err != nil {
		t.Fatalf("access levels: %v", err)
	}

	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Master", 40, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithZones(zones),
		gameservertest.WithRestartPoints(restarts),
		gameservertest.WithAdmin(levels),
	)
	id := srv.SoleObjectID(t)
	if _, err := srv.DB.ExecContext(context.Background(), "UPDATE characters SET accesslevel = 7 WHERE obj_Id = ?", id); err != nil {
		t.Fatalf("set access level: %v", err)
	}
	burst := startInWorld(t, srv.Client)

	coolTime, tele := -1, -1
	var landed location.Location
	for i, f := range burst {
		switch f[0] {
		case serverpackets.OpcodeSkillCoolTime:
			if coolTime < 0 {
				coolTime = i
			}
		case serverpackets.OpcodeTeleportToLocation:
			r := wire.NewReader(f[1:])
			if r.ReadInt32() != id {
				continue
			}
			if tele >= 0 {
				t.Fatalf("burst teleports the game master twice (frames %d and %d)", tele, i)
			}
			tele = i
			landed = location.Location{X: int(r.ReadInt32()), Y: int(r.ReadInt32()), Z: int(r.ReadInt32())}
		}
	}
	if coolTime < 0 || tele < coolTime {
		t.Fatalf("SkillCoolTime at frame %d, TeleportToLocation at %d; want the teleport after the SkillCoolTime", coolTime, tele)
	}
	// The teleport's own aborts answer ActionFailed ahead of it; nothing
	// else of the login comes between.
	for i := coolTime + 1; i < tele; i++ {
		if burst[i][0] != serverpackets.OpcodeActionFailed {
			t.Fatalf("frame %d between SkillCoolTime and the teleport = %#x, want only ActionFailed", i, burst[i][0])
		}
	}
	if dx, dy := landed.X-town.X, landed.Y-town.Y; dx < -20 || dx > 20 || dy < -20 || dy > 20 {
		t.Fatalf("game master teleported to %+v, want within 20 of %+v", landed, town)
	}
}
