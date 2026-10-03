package character

import (
	"context"
	"database/sql"
	"slices"
	"testing"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// grandbossList returns the stored grandboss_list rows as zone to players.
func grandbossList(t *testing.T, db *sql.DB) map[int][]int32 {
	t.Helper()
	got, err := gamesql.NewBossZoneStore(db).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// TestBossZoneRestoredPermissionLapsesAtEntry pins grandboss_list across a
// restart: a stored player gets its boss zone permission back, which the
// next save writes again and which lets no one back in. The reference
// reads the stored players before the zone's InvadeTime, so the restored
// re-entry deadline is the restore itself: logging in inside the zone
// ejects the player, revokes the permission, and the next save stores
// nothing. A row for an unknown zone is dropped at that save.
func TestBossZoneRestoredPermissionLapsesAtEntry(t *testing.T) {
	t.Parallel()
	oust := location.Location{X: -5000, Y: 20, Z: 30}
	zones, boss := bossEjectZone(t, oust, -100)
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithZones(zones),
		gameservertest.WithBossSeed(func(db *sql.DB) {
			if _, err := db.Exec("INSERT INTO grandboss_list (player_id, zone) SELECT obj_Id, 1 FROM characters WHERE char_name = 'Newbie'"); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec("INSERT INTO grandboss_list (player_id, zone) VALUES (12345, 999)"); err != nil {
				t.Fatal(err)
			}
		}),
	)
	objID := srv.SoleObjectID(t)
	if got := boss.AllowedPlayers(); !slices.Equal(got, []int32{objID}) {
		t.Fatalf("restored allowed players = %v, want [%d]", got, objID)
	}

	store := gamesql.NewBossZoneStore(srv.DB)
	bosses := zone.OfKind[*zone.Boss](zones)
	if err := store.SaveZones(context.Background(), bosses); err != nil {
		t.Fatal(err)
	}
	if got := grandbossList(t, srv.DB); len(got) != 1 || !slices.Equal(got[1], []int32{objID}) {
		t.Fatalf("saved grandboss_list = %v, want zone 1: [%d]", got, objID)
	}

	c := srv.Client
	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	frames := append(readEnterWorldBurst(t, c), readUntilQuiet(c)...)
	waitForWorldPosition(t, srv, objID, oust)
	assertEjectFrame(t, frames, objID, oust.X, oust.X)
	if got := boss.AllowedPlayers(); len(got) != 0 {
		t.Fatalf("allowed players after the lapsed entry = %v, want none", got)
	}

	if err := store.SaveZones(context.Background(), bosses); err != nil {
		t.Fatal(err)
	}
	if got := grandbossList(t, srv.DB); len(got) != 0 {
		t.Fatalf("saved grandboss_list = %v, want empty", got)
	}
}
