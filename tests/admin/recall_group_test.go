package admin

import (
	"context"
	"database/sql"
	"strconv"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// The recall cast: Lead leads a clan with Mate and Third in it, and the
// party Lead forms with them; Loner is in neither. Ids sit far above
// anything the id factory hands out during a test.
const recallClanID int32 = 0x7f000101

var recallCast = []struct {
	id      int32
	account string
	name    string
	inClan  bool
}{
	{0x7f100101, "player2", "Lead", true},
	{0x7f100102, "player3", "Mate", true},
	{0x7f100103, "player4", "Third", true},
	{0x7f100104, "player5", "Loner", false},
}

// seedRecallCast stores the cast and the clan of its first three.
func seedRecallCast(t *testing.T) []gameservertest.Option {
	return []gameservertest.Option{
		gameservertest.WithSeed(func(chars *gamesql.CharacterStore, _ *gamesql.ItemStore) {
			tmpl, ok := gameservertest.Templates(t).Get(0)
			if !ok {
				t.Fatal("missing test class template")
			}
			for _, m := range recallCast {
				ch, err := player.NewCharacter(m.id, tmpl, m.account, m.name, 1, 0, 0, player.SexMale)
				if err != nil {
					t.Fatalf("seed %s: %v", m.name, err)
				}
				if err := chars.Create(context.Background(), ch); err != nil {
					t.Fatalf("seed %s: %v", m.name, err)
				}
			}
		}),
		gameservertest.WithClanSeed(func(db *sql.DB) {
			ctx := context.Background()
			clanID := strconv.Itoa(int(recallClanID))
			stmts := []string{
				`INSERT INTO clan_data (clan_id, clan_name, clan_level, leader_id) VALUES (` + clanID + `, 'Recalled', 0, ` + strconv.Itoa(int(recallCast[0].id)) + `)`,
			}
			for _, m := range recallCast {
				if m.inClan {
					stmts = append(stmts, `UPDATE characters SET clanid = `+clanID+`, power_grade = 5 WHERE obj_Id = `+strconv.Itoa(int(m.id)))
				}
			}
			for _, stmt := range stmts {
				if _, err := db.ExecContext(ctx, stmt); err != nil {
					t.Fatalf("seed clan: %v", err)
				}
			}
		}),
	}
}

func encodeAnswerJoinParty(response int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestAnswerJoinParty)
	w.WriteInt32(response)
	return w.Bytes()
}

// TestAdminRecallGroup pins //recall party and //recall clan
// (AdminTeleport.java:69-122): every member of the named player's party,
// or every online member of its clan, is brought to the GM, and the GM's
// selection is dropped once; a player in no party or clan comes alone.
func TestAdminRecallGroup(t *testing.T) {
	t.Parallel()
	srv, _ := bootAdmin(t, adminLevel, seedRecallCast(t)...)
	gm := srv.Client
	enterWorld(t, gm)
	clients := make([]*testsupport.ScriptedClient, len(recallCast))
	for i, m := range recallCast {
		clients[i] = srv.DialClient(t, m.account, 1)
		enterWorld(t, clients[i])
	}
	drainAll := func() {
		drain(t, gm)
		for _, c := range clients {
			drain(t, c)
		}
	}
	drainAll()

	// Lead invites Mate and Third into its party.
	lead := clients[0]
	for i, name := range []string{"Mate", "Third"} {
		lead.Send(encodeJoinParty(name, 0))
		drain(t, clients[i+1])
		clients[i+1].Send(encodeAnswerJoinParty(1))
		drainAll()
	}

	// recall moves the GM to x, recalls with line, and requires exactly
	// the players at want to come, then completes their teleports.
	recall := func(line string, x int32, want ...int) {
		t.Helper()
		exchange(t, gm, encodeBuildCmd("teleport "+strconv.Itoa(int(x))+" 600 300"))
		appear(t, gm)
		drainAll()

		frames := exchange(t, gm, encodeBuildCmd(line))
		if n := countOpcode(frames, serverpackets.OpcodeActionFailed); n != 1 {
			t.Fatalf("//%s GM frames = %x, want the selection drop's one ActionFailed", line, testsupport.FrameOpcodes(frames))
		}
		comes := make(map[int]bool, len(want))
		for _, i := range want {
			comes[i] = true
			if at := readTeleport(t, clients[i], recallCast[i].id); at != [3]int32{x, 600, 300} {
				t.Fatalf("//%s: %s destination = %v, want the GM's %d 600 300", line, recallCast[i].name, at, x)
			}
			appear(t, clients[i])
		}
		drainAll()
		for i, m := range recallCast {
			px, _, _ := srv.PlayerPosition(t, m.id)
			if comes[i] != (px == int(x)) {
				t.Fatalf("//%s: %s at X %d, recalled %v", line, m.name, px, comes[i])
			}
		}
	}

	recall("recall party Mate", 1000, 0, 1, 2)
	recall("recall clan Third", 2000, 0, 1, 2)
	recall("recall party Loner", 3000, 3)
	recall("recall clan Loner", 4000, 3)
}

func countOpcode(frames [][]byte, op byte) int {
	n := 0
	for _, f := range frames {
		if f[0] == op {
			n++
		}
	}
	return n
}
