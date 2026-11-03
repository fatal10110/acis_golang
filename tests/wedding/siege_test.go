package wedding

import (
	"context"
	"database/sql"
	"slices"
	"testing"

	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// The castle world: Lords, led by Alice, own Gludio Castle; Rivals attack
// it.
const (
	lordsClanID   int32 = 0x7f320101
	rivalsClanID  int32 = 0x7f320102
	rivalLeaderID int32 = 0x7f320002
	gludioCastle        = 1
)

// TestWeddingGoToLoveRefusedWhileSpouseCastleBesieged: once Alice's clan's
// castle is under siege, Bobby's teleport to Alice is refused with its own
// notice and nobody moves (WeddingManagerNpc.java:98-102).
func TestWeddingGoToLoveRefusedWhileSpouseCastleBesieged(t *testing.T) {
	datapack.Require(t)
	castles, err := gamexml.LoadCastles(datapack.Path(t, "data", "xml", "castles.xml"))
	if err != nil {
		t.Fatalf("load castles: %v", err)
	}
	opts := defaultChapel()
	opts.extra = []gameservertest.Option{
		gameservertest.WithCastles(castles),
		gameservertest.WithSieges(siege.DefaultConfig()),
		gameservertest.WithClanSeed(func(db *sql.DB) {
			for _, s := range []struct {
				q    string
				args []any
			}{
				{"UPDATE characters SET clanid = ? WHERE char_name = 'Alice'", []any{lordsClanID}},
				{"INSERT INTO clan_data (clan_id, clan_name, clan_level, hasCastle, leader_id) SELECT ?, 'Lords', 5, ?, obj_Id FROM characters WHERE char_name = 'Alice'", []any{lordsClanID, gludioCastle}},
				{"INSERT INTO characters (account_name, obj_Id, char_name, level, clanid) VALUES ('rival', ?, 'Red', 40, ?)", []any{rivalLeaderID, rivalsClanID}},
				{"INSERT INTO clan_data (clan_id, clan_name, clan_level, leader_id) VALUES (?, 'Rivals', 5, ?)", []any{rivalsClanID, rivalLeaderID}},
				{"INSERT INTO siege_clans (castle_id, clan_id, type) VALUES (?, ?, 'ATTACKER')", []any{gludioCastle, rivalsClanID}},
			} {
				if _, err := db.ExecContext(context.Background(), s.q, s.args...); err != nil {
					t.Fatalf("seed castle clans: %v", err)
				}
			}
		}),
	}
	c := bootChapel(t, opts)
	c.talk(t, c.alice)
	c.bypass(t, c.alice, "AskWedding Bobby")
	answer(t, c.bobby, true)
	drainFrames(t, c.alice)
	drainFrames(t, c.bobby)
	c.assertPage(t, "married menu", c.talk(t, c.alice), c.page(t, "start2.htm", "", ""))

	gludio, ok := c.srv.Sieges.Get(gludioCastle)
	if !ok {
		t.Fatal("no Gludio siege")
	}
	gludio.Start()
	if !gludio.InProgress() {
		t.Fatal("Gludio's siege did not start")
	}
	drainFrames(t, c.alice)
	drainFrames(t, c.bobby)

	c.assertPage(t, "married menu", c.talk(t, c.bobby), c.page(t, "start2.htm", "", ""))
	frames := c.bypass(t, c.bobby, "GoToLove")
	if got := texts(t, frames); !slices.Equal(got, []string{"As your partner is in siege, you can't go to him/her."}) {
		t.Fatalf("GoToLove to a spouse whose castle is besieged = %q", got)
	}
	if got := only(frames, serverpackets.OpcodeTeleportToLocation); len(got) != 0 {
		t.Fatal("a refused GoToLove teleported")
	}
}
