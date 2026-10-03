package pets

import (
	"database/sql"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/party"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: RaidBoss.doDie resolves the killer through getActingPlayer,
// so a summon's kill counts as its owner's: the raid success message and
// its sound go out, and the owner's party, or the partyless owner, earns
// the raid points.

// petRaidBossID is the npc id of the fixture raid boss.
const petRaidBossID = 25001

// petRaidBoss is a level-40 raid boss: each player credited with its kill
// earns 40/2 - 5 to 40/2 + 5 points.
func petRaidBoss() *npc.Template {
	return &npc.Template{
		ID: petRaidBossID, TemplateID: petRaidBossID, Type: "RaidBoss", Level: 40, HPMax: 2000,
		AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
	}
}

func spawnPetRaidBoss(t *testing.T, srv *gameservertest.Server) *npc.Hostile {
	t.Helper()
	return srv.SpawnHostileNPCTemplateAt(t, petRaidBoss(), location.Location{X: hostileX, Y: hostileY, Z: hostileZ})
}

// requireRaidAnnouncement asserts frames show the boss dying, then the
// raid success message once, then its sound right after it.
func requireRaidAnnouncement(t *testing.T, who string, frames [][]byte) {
	t.Helper()
	die, msg, sound := -1, -1, -1
	for i, f := range frames {
		switch {
		case f[0] == serverpackets.OpcodeDie && die < 0:
			die = i
		case f[0] == serverpackets.OpcodeSystemMessage && int(wire.NewReader(f[1:]).ReadInt32()) == serverpackets.SystemMessageRaidWasSuccessful:
			if msg >= 0 {
				t.Fatalf("%s got RAID_WAS_SUCCESSFUL twice", who)
			}
			msg = i
		case f[0] == serverpackets.OpcodePlaySound:
			r := wire.NewReader(f[1:])
			r.ReadInt32()
			if r.ReadString() == serverpackets.SoundRaidWasSuccessful {
				sound = i
			}
		}
	}
	if die < 0 || msg <= die || sound != msg+1 {
		t.Fatalf("%s: Die at %d, RAID_WAS_SUCCESSFUL at %d, its sound at %d; want them in that order, the sound right after the message", who, die, msg, sound)
	}
}

// storedRaidPoints returns the character_raid_points rows of objectID,
// keyed by boss.
func storedRaidPoints(t *testing.T, db *sql.DB, objectID int32) map[int32]int32 {
	t.Helper()
	rows, err := db.Query("SELECT boss_id, points FROM character_raid_points WHERE char_id = ?", objectID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[int32]int32{}
	for rows.Next() {
		var boss, points int32
		if err := rows.Scan(&boss, &points); err != nil {
			t.Fatal(err)
		}
		out[boss] = points
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// requireOneRaidRow asserts objectID has exactly one stored 15..25 row for
// the fixture boss, matching its live record.
func requireOneRaidRow(t *testing.T, srv *gameservertest.Server, who string, objectID int32) {
	t.Helper()
	stored := storedRaidPoints(t, srv.DB, objectID)
	points, ok := stored[petRaidBossID]
	if !ok || len(stored) != 1 || points < 15 || points > 25 {
		t.Fatalf("%s stored raid points %v, want one row for %d worth 15..25", who, stored, petRaidBossID)
	}
	if rec := srv.RaidPoints.Record(objectID); !rec.Found || rec.Total != points || len(rec.Entries) != 1 || rec.Entries[0].BossID != petRaidBossID {
		t.Fatalf("%s record = %+v, want the stored %d points for %d", who, rec, points, petRaidBossID)
	}
}

// requireNoRaidRow asserts objectID earned no raid points.
func requireNoRaidRow(t *testing.T, srv *gameservertest.Server, who string, objectID int32) {
	t.Helper()
	if stored := storedRaidPoints(t, srv.DB, objectID); len(stored) != 0 {
		t.Fatalf("%s stored raid points %v, want none", who, stored)
	}
	if rec := srv.RaidPoints.Record(objectID); rec.Found {
		t.Fatalf("%s record = %+v, want none", who, rec)
	}
}

// TestPetRaidKillCreditsPartylessOwner: a partyless owner's wolf lands the
// killing blow on a raid boss. The owner and a watching stranger see the
// death, the raid success message and its sound; the owner alone earns
// its 15 to 25 points, and neither the stranger nor the wolf earns any.
func TestPetRaidKillCreditsPartylessOwner(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t)
	wolf, _ := h.spawnWolf(t)
	watcher, watcherID := joinSecondPlayer(t, h.srv)
	boss := spawnPetRaidBoss(t, h.srv)
	drainUntilQuiet(t, h.client)
	drainUntilQuiet(t, watcher)

	if !boss.TakeDamage(5000, wolf) {
		t.Fatal("the wolf's hit did not kill the boss")
	}
	requireRaidAnnouncement(t, "owner", drainFrames(t, h.client))
	requireRaidAnnouncement(t, "watcher", drainFrames(t, watcher))

	h.srv.FlushPersistence(t)
	requireOneRaidRow(t, h.srv, "owner", h.ownerID)
	requireNoRaidRow(t, h.srv, "watcher", watcherID)
	requireNoRaidRow(t, h.srv, "wolf", wolf.ObjectID())
}

// TestPetRaidKillCreditsOwnersParty: a partied owner's wolf lands the
// killing blow. Both members see the announcement, and each earns its own
// 15 to 25 points for the boss.
func TestPetRaidKillCreditsOwnersParty(t *testing.T) {
	t.Parallel()
	h, wolf, mate, mateID := partyPet(t, party.LootFindersKeepers)
	boss := spawnPetRaidBoss(t, h.srv)
	drainUntilQuiet(t, h.client)
	drainUntilQuiet(t, mate)

	if !boss.TakeDamage(5000, wolf) {
		t.Fatal("the wolf's hit did not kill the boss")
	}
	requireRaidAnnouncement(t, "owner", drainFrames(t, h.client))
	requireRaidAnnouncement(t, "Mate", drainFrames(t, mate))

	h.srv.FlushPersistence(t)
	requireOneRaidRow(t, h.srv, "owner", h.ownerID)
	requireOneRaidRow(t, h.srv, "Mate", mateID)
	requireNoRaidRow(t, h.srv, "wolf", wolf.ObjectID())
}
