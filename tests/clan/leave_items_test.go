package clan

import (
	"context"
	"database/sql"
	"slices"
	"sync"
	"testing"

	"github.com/rs/zerolog"

	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// The castle items a member leaving a castle's clan is checked for.
const (
	circletOfGludio int32 = 6838 // castle="1" pledgeClass="2"
	lordsCrown      int32 = 6841 // castle="-1" pledgeClass="-1": the leader only
	faceSlot              = 14   // the paperdoll slot both are worn in
)

var shippedCastleItems = struct {
	once  sync.Once
	table *item.Table
	err   error
}{}

// castleClanOptions boots the shipped castles and the fixture item
// templates plus the Circlet of Gludio and the Lord's Crown.
func castleClanOptions(t *testing.T) []gameservertest.Option {
	t.Helper()
	datapack.Require(t)
	shippedCastleItems.once.Do(func() {
		shippedCastleItems.table, shippedCastleItems.err = gamexml.LoadItemTemplates(datapack.Path(t, "data", "xml", "items"), zerolog.Nop())
	})
	if shippedCastleItems.err != nil {
		t.Fatalf("load shipped items: %v", shippedCastleItems.err)
	}
	templates := gameservertest.ItemTemplates().All()
	for _, id := range []int32{circletOfGludio, lordsCrown} {
		tmpl, ok := shippedCastleItems.table.Get(id)
		if !ok {
			t.Fatalf("shipped item %d missing", id)
		}
		templates = append(templates, tmpl)
	}
	castles, err := gamexml.LoadCastles(datapack.Path(t, "data", "xml", "castles.xml"))
	if err != nil {
		t.Fatalf("load castles: %v", err)
	}
	return []gameservertest.Option{
		gameservertest.WithItemTemplates(item.NewTable(templates)),
		gameservertest.WithCastles(castles),
	}
}

// bootCastleLeaver boots the founder leading a level 5 clan owning castle
// (0 for none) and the recruit wearing worn, a castle item, in the world;
// the recruit has then joined the clan. It returns the world and the worn
// item's object id.
func bootCastleLeaver(t *testing.T, castle int, worn int32) (*clanWorld, int32) {
	t.Helper()
	opts := castleClanOptions(t)
	srv := gameservertest.Boot(t, append(opts,
		gameservertest.WithCharacter("Founder", 40, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithReuseDelays(0, 0),
		gameservertest.WithClanSeed(func(db *sql.DB) {
			seedStatements(t, db,
				`INSERT INTO clan_data (clan_id, clan_name, clan_level, hasCastle, leader_id)
					SELECT `+itoa(titleClanID)+`, 'Knights', 5, `+itoa(int32(castle))+`, obj_Id FROM characters WHERE char_name = 'Founder'`,
				`UPDATE characters SET clanid = `+itoa(titleClanID)+`, power_grade = 0 WHERE char_name = 'Founder'`)
		}),
	)...)
	w := &clanWorld{srv: srv, leader: srv.Client, leaderID: srv.SoleObjectID(t)}
	w.memberID = srv.SeedCharacterFor(t, "player2", "Recruit", 40, 0).ID
	objectID := srv.GiveItem(t, w.memberID, worn, 1)
	if _, err := srv.DB.ExecContext(context.Background(), `UPDATE items SET loc = 'PAPERDOLL', loc_data = ? WHERE object_id = ?`, faceSlot, objectID); err != nil {
		t.Fatalf("wear item %d: %v", worn, err)
	}
	w.member = srv.DialClient(t, "player2", 1)
	startInWorld(t, w.leader)
	startInWorld(t, w.member)
	drainFrames(t, w.leader)
	drainFrames(t, w.member)
	w.recruit(t)
	return w, objectID
}

// itemLoc reads the stored location of objectID once every queued write
// landed.
func itemLoc(t *testing.T, w *clanWorld, objectID int32) string {
	t.Helper()
	w.srv.FlushItems(t)
	var loc string
	if err := w.srv.DB.QueryRowContext(context.Background(), `SELECT loc FROM items WHERE object_id = ?`, objectID).Scan(&loc); err != nil {
		t.Fatalf("read loc of item %d: %v", objectID, err)
	}
	return loc
}

// leftClanOnly is a withdrawal answer that takes nothing off.
var leftClanOnly = []byte{
	serverpackets.OpcodeSkillList, serverpackets.OpcodeUserInfo, serverpackets.OpcodePledgeShowMemberListDelAll,
	serverpackets.OpcodeSystemMessage, serverpackets.OpcodeSystemMessage,
}

// TestLeavingCastleClanChecksItemsWhileStillInIt has a member in the world
// withdraw from a clan owning Gludio Castle. Its items are checked before
// its clan state clears, as Castle.checkItemsForMember runs first in
// Clan.removeClanMember: the Lord's Crown, which a member that is not the
// leader fails, comes off through the equip toggle, while the Circlet of
// Gludio, which the member still passes in the clan, stays on.
func TestLeavingCastleClanChecksItemsWhileStillInIt(t *testing.T) {
	t.Parallel()
	t.Run("crown comes off", func(t *testing.T) {
		t.Parallel()
		w, crown := bootCastleLeaver(t, 1, lordsCrown)
		if loc := itemLoc(t, w, crown); loc != "PAPERDOLL" {
			t.Fatalf("crown before leaving = %s, want PAPERDOLL", loc)
		}
		left := withdrawRecruit(t, w)
		if string(opcodes(left)) == string(leftClanOnly) {
			t.Fatalf("leaver's answer = %x, want the crown taken off first", opcodes(left))
		}
		skills := 0
		for skills < len(left) && left[skills][0] != serverpackets.OpcodeSkillList {
			skills++
		}
		if skills == 0 || string(opcodes(left[skills:])) != string(leftClanOnly) {
			t.Fatalf("leaver's answer = %x, want the unequip, then %x", opcodes(left), leftClanOnly)
		}
		// The UseItem equip toggle: S1_DISARMED naming the crown, then the
		// refreshed UserInfo.
		if got := opcodes(left[:skills]); string(got) != string([]byte{serverpackets.OpcodeSystemMessage, serverpackets.OpcodeUserInfo}) {
			t.Fatalf("unequip frames = %x, want SystemMessage, UserInfo", got)
		}
		if id, params := sysMsg(t, left[0]); id != serverpackets.SystemMessageS1Disarmed || !slices.Equal(params, []string{itoa(lordsCrown)}) {
			t.Fatalf("unequip message = %d %v, want S1_DISARMED %d", id, params, lordsCrown)
		}
		if loc := itemLoc(t, w, crown); loc != "INVENTORY" {
			t.Fatalf("crown after leaving = %s, want INVENTORY", loc)
		}
	})
	t.Run("circlet stays on", func(t *testing.T) {
		t.Parallel()
		w, circlet := bootCastleLeaver(t, 1, circletOfGludio)
		if got := opcodes(withdrawRecruit(t, w)); string(got) != string(leftClanOnly) {
			t.Fatalf("leaver's answer = %x, want %x", got, leftClanOnly)
		}
		if loc := itemLoc(t, w, circlet); loc != "PAPERDOLL" {
			t.Fatalf("circlet after leaving = %s, want PAPERDOLL", loc)
		}
	})
}

// TestLeavingClanWithoutCastleChecksNothing has a member in the world
// withdraw from a clan owning no castle: nothing is checked, so even the
// Lord's Crown it fails stays on.
func TestLeavingClanWithoutCastleChecksNothing(t *testing.T) {
	t.Parallel()
	w, crown := bootCastleLeaver(t, 0, lordsCrown)
	if got := opcodes(withdrawRecruit(t, w)); string(got) != string(leftClanOnly) {
		t.Fatalf("leaver's answer = %x, want %x", got, leftClanOnly)
	}
	if loc := itemLoc(t, w, crown); loc != "PAPERDOLL" {
		t.Fatalf("crown after leaving = %s, want PAPERDOLL", loc)
	}
}

// TestExpellingOfflineMemberUnequipsCastleCirclets expels a member out of
// the world: from a clan owning Gludio Castle, its worn Circlet of Gludio
// is stored back in its inventory; from a clan owning none, it stays worn.
func TestExpellingOfflineMemberUnequipsCastleCirclets(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		castle int
		want   string
	}{
		{"castle clan", 1, "INVENTORY"},
		{"clan without castle", 0, "PAPERDOLL"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w, circlet := bootCastleLeaver(t, tc.castle, circletOfGludio)
			w.leaveWorld(t, w.member)
			drainFrames(t, w.leader)
			w.leader.Send(encodeRequestOustPledgeMember("Recruit"))
			drainFrames(t, w.leader)
			if loc := itemLoc(t, w, circlet); loc != tc.want {
				t.Fatalf("offline member's circlet = %s, want %s", loc, tc.want)
			}
			if got := queryInt(t, w, `SELECT clanid FROM characters WHERE obj_Id = ?`, w.memberID); got != 0 {
				t.Fatalf("expelled member's clanid = %d, want 0", got)
			}
		})
	}
}
