package ai

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strings"
	"testing"

	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/boss/benom"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptcontract"
)

// Rune castle's Dungeon Gatekeeper and Rune castle.
const (
	benomGatekeeper = int32(35506)
	runeCastleID    = 8
)

// bootGatekeeper boots the gatekeeper's behavior beside the character, with
// the director script events are sent through, its pages each naming
// itself (the greeting with its link), and extra. The gatekeeper scenario
// of testdata/scenarios covers its greeting and teleport out of a siege;
// this test covers what the scenario format cannot hold: a siege being
// fought and script events.
func bootGatekeeper(t *testing.T, extra ...gameservertest.Option) *warriorWorld {
	t.Helper()
	paths := []string{"script.ai.individual.DefaultNpc", "script.ai.boss.benom.DungeonTeleporter", "test.Ind1Director"}
	catalog := script.Catalog{
		paths[0]: defaultnpc.New, paths[1]: benom.DungeonTeleporter,
		paths[2]: func() script.Script { return script.Script{} },
	}
	var list []script.Listing
	for _, p := range paths {
		list = append(list, script.Listing{Path: p})
	}
	kinds := map[int32]script.NPCKind{benomGatekeeper: script.KindFolk}
	return bootRaid(t, raidBoot{opts: append([]gameservertest.Option{
		gameservertest.WithNPCScripts(kinds, list, catalog),
		gameservertest.WithHTMLPages(map[string]string{
			"doormen/35506.htm":   `<html><body>35506.htm <a action="bypass -h npc_%objectId%_Quest DungeonTeleporter">down</a></body></html>`,
			"doormen/35506-2.htm": "<html><body>35506-2.htm</body></html>",
		}),
	}, extra...)}, gameservertest.FolkTemplate("CastleDoorman", int(benomGatekeeper)))
}

// gatekeeperSays renders, in order, the NpcSay frames among frames.
func gatekeeperSays(t *testing.T, frames [][]byte, roles map[int32]string) []string {
	t.Helper()
	var out []string
	for _, f := range frames {
		if f[0] != serverpackets.OpcodeNpcSay {
			continue
		}
		line, err := scriptcontract.Packet(f, roles)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, line)
	}
	return out
}

// TestDungeonGatekeeperDuringASiege covers the gatekeeper (phases 2 and 5)
// while Rune castle's siege is fought: its created hook sets its first
// memory slot; it greets with its page; asked to teleport, it answers its refusal page and sends no
// one down; script event 10101 makes it shout, and 10100 does nothing.
func TestDungeonGatekeeperDuringASiege(t *testing.T) {
	t.Parallel()
	datapack.Require(t)
	castles, err := gamexml.LoadCastles(datapack.Path(t, "data", "xml", "castles.xml"))
	if err != nil {
		t.Fatalf("load castles: %v", err)
	}
	const clanID = 0x70000041
	w := bootGatekeeper(t,
		gameservertest.WithCastles(castles),
		gameservertest.WithSieges(siege.DefaultConfig()),
		gameservertest.WithClanSeed(func(db *sql.DB) {
			for _, q := range []struct {
				q    string
				args []any
			}{
				{"UPDATE characters SET clanid = ? WHERE char_name = 'Hunter'", []any{clanID}},
				{`INSERT INTO clan_data (clan_id, clan_name, clan_level, leader_id)
					SELECT ?, 'Raiders', 5, obj_Id FROM characters WHERE char_name = 'Hunter'`, []any{clanID}},
				{"INSERT INTO siege_clans (castle_id, clan_id, type) VALUES (?, ?, 'ATTACKER')", []any{runeCastleID, clanID}},
			} {
				if _, err := db.ExecContext(context.Background(), q.q, q.args...); err != nil {
					t.Fatalf("seed clans: %s: %v", q.q, err)
				}
			}
		}),
	)
	live, err := w.srv.NpcSpawns.AddSpawn(benomGatekeeper, w.at.X+60, w.at.Y, w.at.Z, 0, false, 0)
	if err != nil {
		t.Fatalf("spawn the gatekeeper: %v", err)
	}
	gk := live.(*npc.Folk)
	w.srv.Settle(t)
	w.srv.ReadQueued(t, w.srv.Client)
	if got := script.NPCOf(gk).Int(script.SlotIntAI0); got != 1 {
		t.Fatalf("gatekeeper's first memory slot = %d, want 1", got)
	}
	rune, ok := w.srv.Sieges.Get(runeCastleID)
	if !ok {
		t.Fatal("no Rune castle siege")
	}
	rune.Start()
	w.srv.Settle(t)
	w.srv.ReadQueued(t, w.srv.Client)

	var talk [][]byte
	for range 2 {
		w.srv.Client.Send(encodeFolkAction(gk.ObjectID(), w.at))
		w.srv.Settle(t)
		talk = append(talk, w.srv.ReadQueued(t, w.srv.Client)...)
	}
	if got := managerHTML(talk); len(got) != 1 || !strings.Contains(got[0], "35506.htm") {
		t.Fatalf("pages on talking to the gatekeeper during the siege = %q, want its greeting", got)
	}
	x, y, _ := w.srv.PlayerPosition(t, w.player.ObjectID())
	w.srv.Client.Send(encodeManagerBypass(fmt.Sprintf("npc_%d_Quest DungeonTeleporter", gk.ObjectID())))
	w.srv.Settle(t)
	frames := w.srv.ReadQueued(t, w.srv.Client)
	if got := managerHTML(frames); !slices.Equal(got, []string{"<html><body>35506-2.htm</body></html>\n"}) {
		t.Fatalf("pages asked to teleport during the siege = %q, want the refusal", got)
	}
	if got := ind8Pick(frames, serverpackets.OpcodeTeleportToLocation); len(got) != 0 {
		t.Fatalf("TeleportToLocation frames during the siege = %x, want none", got)
	}
	if nx, ny, _ := w.srv.PlayerPosition(t, w.player.ObjectID()); nx != x || ny != y {
		t.Fatalf("the character moved to %d,%d during the siege, want %d,%d", nx, ny, x, y)
	}

	roles := map[int32]string{gk.ObjectID(): "gatekeeper"}
	for _, row := range []struct {
		event int32
		want  []string
	}{
		{10100, nil},
		{10101, []string{fmt.Sprintf("S NpcSay obj=gatekeeper type=1 npc=%d text=%s", 1000000+benomGatekeeper, scriptcontract.Quote(script.NpcString(1010632)))}},
	} {
		if !w.srv.RunScript(t, w.player.ObjectID(), "Ind1Director", func(s *script.Script, _ *script.Player) {
			s.SendScriptEvent(script.NPCOf(gk), row.event, 1, 0)
		}) {
			t.Fatal("the script event panicked")
		}
		w.srv.Settle(t)
		if got := gatekeeperSays(t, w.srv.ReadQueued(t, w.srv.Client), roles); !slices.Equal(got, row.want) {
			t.Fatalf("NpcSay frames on script event %d = %q, want %q", row.event, got, row.want)
		}
	}
}
