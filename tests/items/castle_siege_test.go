package items

import (
	"context"
	"database/sql"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// siegeRivalClanID attacks Gludio Castle in the siege castle world.
const (
	siegeRivalClanID   = 0x70000021
	siegeRivalLeaderID = 0x70000022
)

// TestAdminCastleOwnerChangeDuringSiege drives //castle remove and
// //castle set on Gludio Castle while its siege runs (Castle.removeOwner,
// setOwner): the clan losing the castle leaves the siege's registrations,
// and, the siege being under way, keeps the Lord's Crown on instead of
// having the castle items checked; the clan given the castle back owns the
// siege's defending side again (Siege.midVictory), the attacker staying on
// its side.
func TestAdminCastleOwnerChangeDuringSiege(t *testing.T) {
	t.Parallel()
	w := bootCastleWorld(t, 1, func(db *sql.DB) {
		for _, s := range []struct {
			q    string
			args []any
		}{
			{"INSERT INTO characters (account_name, obj_Id, char_name, level, clanid) VALUES ('rival', ?, 'Red', 40, ?)", []any{siegeRivalLeaderID, siegeRivalClanID}},
			{"INSERT INTO clan_data (clan_id, clan_name, clan_level, leader_id) VALUES (?, 'Rivals', 5, ?)", []any{siegeRivalClanID, siegeRivalLeaderID}},
			{"INSERT INTO siege_clans (castle_id, clan_id, type) VALUES (1, ?, 'ATTACKER')", []any{siegeRivalClanID}},
		} {
			if _, err := db.ExecContext(context.Background(), s.q, s.args...); err != nil {
				t.Fatalf("seed rival clan: %v", err)
			}
		}
	}, gameservertest.WithSieges(siege.DefaultConfig()))
	crown := w.srv.GiveItem(t, w.leaderID, lordsCrownID, 1)
	w.enter(t)
	w.leader.Send(encodeUseItem(crown, false))
	drainUntilQuiet(t, w.leader)
	drainUntilQuiet(t, w.gm)

	gludio, ok := w.srv.Sieges.Get(1)
	if !ok {
		t.Fatal("no Gludio siege")
	}
	if gludio.Side(castleClanID) != siege.SideOwner || gludio.Side(siegeRivalClanID) != siege.SideAttacker {
		t.Fatalf("restored sides: owner %s, rival %s", gludio.Side(castleClanID), gludio.Side(siegeRivalClanID))
	}
	gludio.Start()
	drainUntilQuiet(t, w.leader)
	drainUntilQuiet(t, w.gm)
	if !gludio.InProgress() {
		t.Fatal("siege not under way")
	}

	_, leader := w.castleCommand(t, "remove gludio_castle")
	if castle, _ := pledgeCastle(t, leader); castle != 0 {
		t.Fatalf("PledgeShowInfoUpdate castle after remove = %d, want 0", castle)
	}
	w.srv.FlushItems(t)
	if inst := mustFindItem(t, w.srv, w.leaderID, crown); inst.Location != item.LocationPaperdoll {
		t.Fatalf("crown after a removal during the siege = %v, want still worn", inst.Location)
	}
	if got := gludio.Side(castleClanID); got != siege.SideNone {
		t.Fatalf("former owner side after remove = %s, want none", got)
	}

	_, leader = w.castleCommand(t, "set gludio_castle")
	if castle, _ := pledgeCastle(t, leader); castle != 1 {
		t.Fatalf("PledgeShowInfoUpdate castle after set = %d, want 1", castle)
	}
	if got := gludio.Side(castleClanID); got != siege.SideOwner {
		t.Fatalf("new owner side = %s, want OWNER", got)
	}
	if got := gludio.Side(siegeRivalClanID); got != siege.SideAttacker {
		t.Fatalf("rival side = %s, want ATTACKER", got)
	}
	if got := w.srv.PlayerSiegeState(t, w.leaderID); got != 2 {
		t.Fatalf("new owner's leader siege state = %d, want 2", got)
	}
}
