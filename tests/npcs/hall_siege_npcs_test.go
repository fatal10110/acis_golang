package npcs

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/clanhall"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
	"github.com/rs/zerolog"
)

// Reference: SiegeGuard.isAttackableBy (SiegeGuard.java:22-44) lets a
// player attack a guard of a siegable hall only while the hall's siege is
// IN_PROGRESS, and only when its clan attacks it. SpawnMaker.
// checkHasSpawnCondition holds an agit_defend_warfare_start(N) maker unless
// hall N's siege is IN_PROGRESS, so the hall scripts' spawn-time start
// (SpawnManager.startSpawnTime) spawns its NPCs only then.

const (
	devastatedHall int32 = 34
	// devastatedGuardID is Dietrich, an NPC Devastated Castle lists, here a
	// fixture siege guard.
	devastatedGuardID = 35408
	// devastatedDefenderID is a fixture defender of the hall's
	// agit_defend_warfare_start(34) maker.
	devastatedDefenderID = 21601
	hallKnightsID        = int32(0x7f320101)
	hallRivalsID         = int32(0x7f320102)
)

const devastatedSpawnlist = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="devastated_field" minZ="0" maxZ="100"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="devastated_defenders" territory="devastated_field" maximumNpcs="1" spawnTime="agit_defend_warfare_start(34)">
		<ai type="default_maker"/>
		<npc id="21601" total="1" pos="160;20;30;400" respawn="60sec"/>
	</npcmaker>
</list>`

// hallNPCWorld is the Talker, leading Knights, on Devastated Castle's
// battlefield, the hall's siege run by script.
type hallNPCWorld struct {
	srv    *gameservertest.Server
	talker *player.Character
	at     location.Location
	siege  *clanhall.HallSiege
}

// bootHallNPCs boots Devastated Castle with Rivals attacking its siege, and
// Knights too when attacks is set.
func bootHallNPCs(t *testing.T, attacks bool, script func(w *hallNPCWorld) clanhall.SiegeScript, extra ...gameservertest.Option) *hallNPCWorld {
	t.Helper()
	datapack.Require(t)
	halls, err := gamexml.LoadClanHalls(datapack.Path(t, "data", "xml", "clanHalls.xml"))
	if err != nil {
		t.Fatal(err)
	}
	zones := zone.NewIndex()
	field := zone.NewSiege(1, battlefieldBox(t))
	field.ResidenceID = int(devastatedHall)
	zones.Add(field)
	w := &hallNPCWorld{}
	sc := clanhall.SiegeScript{}
	if script != nil {
		sc = script(w)
	}
	srv := gameservertest.Boot(t, append([]gameservertest.Option{
		gameservertest.WithCharacter("Talker", playerLevel, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithZones(zones),
		gameservertest.WithClanHalls(halls, nil),
		gameservertest.WithResidences(halls, nil),
		gameservertest.WithHallSiegeScript(devastatedHall, sc),
		gameservertest.WithClanSeed(func(db *sql.DB) {
			stmts := []struct {
				q    string
				args []any
			}{
				{"INSERT INTO clan_data (clan_id, clan_name, clan_level, leader_id) SELECT ?, 'Knights', 5, obj_Id FROM characters WHERE char_name = 'Talker'", []any{hallKnightsID}},
				{"UPDATE characters SET clanid = ? WHERE char_name = 'Talker'", []any{hallKnightsID}},
				{"INSERT INTO clan_data (clan_id, clan_name, clan_level) VALUES (?, 'Rivals', 5)", []any{hallRivalsID}},
				{"INSERT INTO clanhall (id, endDate) VALUES (?, ?)", []any{devastatedHall, time.Now().Add(7 * 24 * time.Hour).UnixMilli()}},
				{"INSERT INTO clanhall_siege_attackers VALUES (?, ?)", []any{devastatedHall, hallRivalsID}},
			}
			if attacks {
				stmts = append(stmts, struct {
					q    string
					args []any
				}{"INSERT INTO clanhall_siege_attackers VALUES (?, ?)", []any{devastatedHall, hallKnightsID}})
			}
			for _, s := range stmts {
				if _, err := db.ExecContext(context.Background(), s.q, s.args...); err != nil {
					t.Fatalf("seed hall siege: %s: %v", s.q, err)
				}
			}
		}),
	}, extra...)...)
	if !srv.DrivesClock() {
		t.Skip("the hall siege steps run on the driven clock")
	}
	w.srv = srv
	startInWorld(t, srv, srv.Client)
	drainUntilQuiet(t, srv.Client)
	objID := srv.SoleObjectID(t)
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("the Talker is not in the world")
	}
	if w.talker, ok = network.OnlineCharacter(obj); !ok {
		t.Fatal("the Talker is no online character")
	}
	x, y, z := srv.PlayerPosition(t, objID)
	w.at = location.Location{X: x, Y: y, Z: z}
	if w.siege, ok = srv.Halls.Siege(devastatedHall); !ok {
		t.Fatal("Devastated Castle has no siege")
	}
	return w
}

// start starts the hall's siege.
func (w *hallNPCWorld) start(t *testing.T) {
	t.Helper()
	w.siege.Instant()
	w.srv.Advance(t, 0)
	if !w.siege.InProgress() {
		t.Fatal("Devastated Castle's siege did not start")
	}
}

// TestHallSiegeGuardAttackableOnlyByAttackersDuringItsSiege spawns a guard
// of Devastated Castle: nobody may attack it before the siege, a player of
// an attacking clan may while the siege is fought, a player of a clan not
// attacking it may not, and nobody may once it ended.
func TestHallSiegeGuardAttackableOnlyByAttackersDuringItsSiege(t *testing.T) {
	t.Parallel()
	guardTemplate := func() *npc.Template {
		return &npc.Template{
			ID: devastatedGuardID, TemplateID: devastatedGuardID, Type: "SiegeGuard", Level: 1, HPMax: 1000,
			AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
		}
	}
	for _, tt := range []struct {
		name    string
		attacks bool
	}{{"attacker", true}, {"not attacking", false}} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := bootHallNPCs(t, tt.attacks, nil)
			guard := w.srv.SpawnHostileNPCTemplateAt(t, guardTemplate(), location.Location{X: w.at.X + 100, Y: w.at.Y, Z: w.at.Z})
			if guard.AttackableBy(w.talker) {
				t.Fatal("the guard is attackable before the siege")
			}
			w.start(t)
			if got := guard.AttackableBy(w.talker); got != tt.attacks {
				t.Fatalf("guard attackable during the siege = %v, want %v", got, tt.attacks)
			}
			w.siege.End()
			if guard.AttackableBy(w.talker) {
				t.Fatal("the guard is attackable after the siege")
			}
		})
	}
}

// TestHallSiegeDefendMakerSpawnsOnlyDuringTheSiege starts Devastated
// Castle's agit_defend_warfare_start(34) maker: before the siege it holds
// and spawns nothing; the siege's start, through the script, spawns its
// defender, and the end, stopping the spawn time, takes it away.
func TestHallSiegeDefendMakerSpawnsOnlyDuringTheSiege(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "devastated.xml"), []byte(devastatedSpawnlist), 0o644); err != nil {
		t.Fatalf("write spawnlist: %v", err)
	}
	table, err := gamexml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatalf("load spawnlist: %v", err)
	}
	const spawnTime = "agit_defend_warfare_start"
	w := bootHallNPCs(t, false, func(w *hallNPCWorld) clanhall.SiegeScript {
		return clanhall.SiegeScript{
			SpawnNPCs:   func() { w.srv.NpcSpawns.StartSpawnTime(spawnTime, "34") },
			UnspawnNPCs: func() { w.srv.NpcSpawns.StopSpawnTime(spawnTime, "34") },
		}
	},
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{spawnMonster(devastatedDefenderID, "Defender")})),
		gameservertest.WithNpcSpawns(table),
	)
	defenders := func() int {
		w.srv.Settle(t)
		n := 0
		for _, obj := range w.srv.State.Objects() {
			if h, ok := obj.(*npc.Hostile); ok && h.NpcID() == devastatedDefenderID {
				n++
			}
		}
		return n
	}
	if got := w.srv.NpcSpawns.StartSpawnTime(spawnTime, "34"); got != 0 || defenders() != 0 {
		t.Fatalf("the maker spawned %d before the siege", got)
	}
	w.start(t)
	if n := defenders(); n != 1 {
		t.Fatalf("%d defenders during the siege, want 1", n)
	}
	w.siege.End()
	if n := defenders(); n != 0 {
		t.Fatalf("%d defenders after the siege, want none", n)
	}
}

// devastatedDoormanID is a Doorman of Hell, Devastated Castle's
// CastleDoorman.
const devastatedDoormanID = 35417

// TestHallSiegeDoormanBusy has Devastated Castle's doorman refuse its
// owner's leader the gates while the hall's siege is fought
// (CastleDoorman.isUnderSiege reads SiegableHall.isInSiege): the busy page,
// then GATES_NOT_OPENED_CLOSED_DURING_SIEGE.
func TestHallSiegeDoormanBusy(t *testing.T) {
	t.Parallel()
	pages := commandPages()
	pages["doormen/35417.htm"] = commandPage("doormen/35417.htm")
	w := bootHallNPCs(t, true, nil, gameservertest.WithHTMLPages(pages))
	w.start(t)
	knights, ok := w.srv.Clans.Table().Get(hallKnightsID)
	if !ok || !w.srv.Halls.SetOwner(devastatedHall, knights) {
		t.Fatal("Knights did not take Devastated Castle")
	}
	fw := &folkWorld{srv: w.srv, c: w.srv.Client, player: w.talker.ObjectID(), at: w.at}
	drainUntilQuiet(t, fw.c)
	f := fw.spawnKeeper(t, "CastleDoorman", devastatedDoormanID, 30)
	frames := fw.command(t, f, "open_doors 1")
	want := []byte{serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeActionFailed, serverpackets.OpcodeSystemMessage, serverpackets.OpcodeActionFailed}
	assertAnswer(t, frames, want, f, asRead("doormen/busy.htm"))
	if got := messages(frames); !slices.Equal(got, []int32{serverpackets.SystemMessageGatesNotOpenedClosedDuringSiege}) {
		t.Fatalf("messages = %v", got)
	}
}
