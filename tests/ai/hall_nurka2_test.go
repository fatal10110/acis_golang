package ai

import (
	"context"
	"database/sql"
	"testing"
	"time"

	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	hallmodel "github.com/fatal10110/acis_golang/internal/gameserver/model/residence/clanhall"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/hall"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// partisanHideaway is the Fortress of Resistance, the siegable hall whose
// siege raises Nurka's second form.
const partisanHideaway = 21

// escortRespawn is how long each private of Nurka's escort waits to come
// back, as the hall's maker declares it.
const escortRespawn = 60 * time.Second

// bootNurka2 boots the hall behaviors with BloodyLordNurka2 bound, beside
// the Partisan Hideaway, whose next siege is untilSiege away, and spawns
// Nurka's second form dx from the character. When serves is set the hall
// lists his id among its NPCs, standing in for the siege maker that names
// it as his residence. His escort is two healers, a swordsman and an archer, each
// coming back after escortRespawn.
func bootNurka2(t *testing.T, untilSiege time.Duration, dx int, serves bool) (*warriorWorld, *npc.Hostile) {
	t.Helper()
	var hallNPCs []int
	if serves {
		hallNPCs = []int{int(nurka2ID)}
	}
	fortress, err := hallmodel.NewHall(hallmodel.HallAttrs{
		ID: partisanHideaway, ParentID: -1, Alias: "partisan_agit001", Name: "Fortress of Resistance",
		Description: "Contestable Clan Hall", Town: "Dion", Grade: 2, SiegeLength: 3600000,
		Siegable: true, ScheduleConfig: []int{14, 0, 0, 12, 0}, NPCs: hallNPCs,
	}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	halls, err := hallmodel.NewTable([]*hallmodel.Hall{fortress})
	if err != nil {
		t.Fatal(err)
	}
	master := agitTemplate(nurka2ID)
	for _, id := range []int32{escortHealerID, escortHealerID, escortSwordID, escortBowID} {
		master.Privates = append(master.Privates, npc.PrivateEntry{NpcID: int(id), Weight: 1, RespawnDelay: escortRespawn})
	}
	templates := []*npc.Template{master, agitTemplate(escortHealerID), agitTemplate(escortSwordID), agitTemplate(escortBowID)}
	list, catalog := hallScripts()
	list = append(list, script.Listing{Path: "script.ai.siegablehall.BloodyLordNurka1.BloodyLordNurka2"})
	catalog["script.ai.siegablehall.BloodyLordNurka1.BloodyLordNurka2"] = hall.Nurka2
	kinds := map[int32]script.NPCKind{}
	for _, tmpl := range templates {
		kinds[int32(tmpl.ID)] = script.KindHostile
	}
	w := bootHalls(t, 0, 0, location.Location{}, raidBoot{opts: []gameservertest.Option{
		gameservertest.WithNPCScripts(kinds, list, catalog),
		gameservertest.WithClanHalls(halls, nil),
		gameservertest.WithResidences(halls, nil),
		gameservertest.WithClanSeed(func(db *sql.DB) {
			if _, err := db.ExecContext(context.Background(), "INSERT INTO clanhall (id, ownerId, paidUntil, paid, endDate) VALUES (?, 0, 0, 1, ?)",
				partisanHideaway, time.Now().Add(untilSiege).UnixMilli()); err != nil {
				t.Fatalf("seed the Fortress of Resistance: %v", err)
			}
		}),
	}}, templates...)
	return w, w.spawn(t, nurka2ID, dx, 0)
}

// TestNurka2SendsPlayersInHisTerritoryAway ports the teleport of
// BloodyLordNurka2's created hook: appearing 100 from the character, he
// sends it to 51952, 111060, -1970, up to 200 away on each axis.
func TestNurka2SendsPlayersInHisTerritoryAway(t *testing.T) {
	t.Parallel()
	w, _ := bootNurka2(t, 7*24*time.Hour, 100, true)
	x, y, z := w.srv.PlayerPosition(t, w.player.ObjectID())
	if abs(x-nurkaLanding.X) > 200 || abs(y-nurkaLanding.Y) > 200 {
		t.Fatalf("the character stands at %d,%d,%d, want within 200 of %v", x, y, z, nurkaLanding)
	}
}

// TestNurka2WalksByTheSiegeDate ports BloodyLordNurka2's created and timer
// hooks. He appears with his flag 0 and his four privates, and every 10
// seconds reads how far his hall's siege date is: an hour or more away he
// walks once to 44525, 108867, -2020, setting his flag to 2; from half an
// hour to an hour he walks to 45802, 109981, -1970, setting it to 1; under
// half an hour he stays and keeps it 0. The next check adds no walk. A
// Nurka serving no hall has no date to read and never walks.
func TestNurka2WalksByTheSiegeDate(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name       string
		untilSiege time.Duration
		flag       int32
		walk       location.Location
		serves     bool
	}{
		{"a week away", 7 * 24 * time.Hour, 2, location.Location{X: 44525, Y: 108867, Z: -2020}, true},
		{"45 minutes away", 45 * time.Minute, 1, location.Location{X: 45802, Y: 109981, Z: -1970}, true},
		{"20 minutes away", 20 * time.Minute, 0, location.Location{}, true},
		{"no hall", 7 * 24 * time.Hour, 0, location.Location{}, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			w, nurka := bootNurka2(t, c.untilSiege, 300, c.serves)
			privatesOf(t, nurka, 4)
			if got := script.NPCOf(nurka).Int(script.SlotFlag); got != 0 {
				t.Fatalf("flag as he appears = %d, want 0", got)
			}
			if d, ok := desire(nurka, actorai.IntentionMoveTo); ok {
				t.Fatalf("move desire as he appears = %+v, want none", d)
			}

			for range 2 {
				w.srv.Advance(t, 10000*time.Millisecond)
				if got := script.NPCOf(nurka).Int(script.SlotFlag); got != c.flag {
					t.Fatalf("flag = %d, want %d", got, c.flag)
				}
				var walks []actorai.Desire
				for _, d := range nurka.AI().Desires().Snapshot() {
					if d.Kind == actorai.IntentionMoveTo {
						walks = append(walks, d)
					}
				}
				if c.flag == 0 {
					if len(walks) != 0 {
						t.Fatalf("move desires = %+v, want none", walks)
					}
					continue
				}
				if len(walks) != 1 {
					t.Fatalf("move desires = %+v, want one", walks)
				}
				wantWalk(t, "Nurka", nurka, c.walk)
			}
		})
	}
}

// TestNurka2BringsBackAFallenPrivate ports BloodyLordNurka2's party-died
// hook: a dead private of his escort is asked back after its own respawn
// delay, 60 s; once its corpse has decayed, that respawn brings a new
// private back as his. His own death, where he is both the caller and the
// called, asks nothing more.
func TestNurka2BringsBackAFallenPrivate(t *testing.T) {
	t.Parallel()
	w, nurka := bootNurka2(t, 7*24*time.Hour, 300, true)
	fallen := privatesOf(t, nurka, 4)[0]

	before := time.Now()
	w.kill(t, fallen)
	after := time.Now()
	key := w.armedPrivateRespawn(t)
	deadline, ok := w.srv.NpcRespawns.Deadline(key)
	if !ok {
		t.Fatalf("no deadline for the armed respawn %s", key)
	}
	if deadline.Before(before.Add(escortRespawn)) || deadline.After(after.Add(escortRespawn)) {
		t.Fatalf("respawn due %s after the death, want %s", deadline.Sub(before), escortRespawn)
	}

	w.onQueue(t, fallen, func() { fallen.Decay(w.srv.State, w.srv.NpcSpawns.RespawnHook(fallen.ObjectID())) })
	w.srv.NpcSpawns.Respawn(key)
	w.srv.Settle(t)
	w.srv.ReadQueued(t, w.srv.Client)
	var back *npc.Hostile
	for _, h := range privatesOf(t, nurka, 4) {
		if h.NpcID() == fallen.NpcID() && h.ObjectID() != fallen.ObjectID() {
			back = h
		}
	}
	if back == nil || back.Dead() || !w.inWorld(back) || back.Master() != nurka {
		t.Fatalf("no new %d in the world following Nurka after the respawn", fallen.NpcID())
	}

	w.kill(t, nurka)
	if got := w.armedPrivateRespawn(t); got != key {
		t.Fatalf("respawn armed for %s after Nurka died, want only the private's %s", got, key)
	}
}
