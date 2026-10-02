package clan

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/bbs"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// raiseToLevel2 founds a clan and raises it to level 2, returning its id
// and the count of bbs_forum rows it had at level 1.
func raiseToLevel2(t *testing.T, w *clanWorld) (int32, int64) {
	t.Helper()
	clanID := w.found(t, "Knights")
	if ids := messages(t, w.masterCommand(t, "increase_clan_level")); !slices.Contains(ids, serverpackets.SystemMessageClanLevelIncreased) {
		t.Fatalf("level 1 messages = %v", ids)
	}
	w.srv.FlushPersistence(t)
	atLevel1 := queryInt(t, w, `SELECT COUNT(*) FROM bbs_forum`)
	if ids := messages(t, w.masterCommand(t, "increase_clan_level")); !slices.Contains(ids, serverpackets.SystemMessageClanLevelIncreased) {
		t.Fatalf("level 2 messages = %v", ids)
	}
	w.srv.FlushPersistence(t)
	return clanID, atLevel1
}

// TestClanLevelCreatesForums pins the clan forums a level-up creates with
// the board on: none at level 1; at level 2 the announcement forum, then
// the bulletin forum, both read-only and owned by the clan.
func TestClanLevelCreatesForums(t *testing.T) {
	w := bootClanWorld(t, 10, 180000, 3150000, gameservertest.WithCommunityBoard(bbs.Config{Enabled: true, Home: "_bbshome"}))
	clanID, atLevel1 := raiseToLevel2(t, w)
	if atLevel1 != 0 {
		t.Fatalf("bbs_forum rows at level 1 = %d, want none", atLevel1)
	}
	for _, f := range []struct {
		id  int64
		typ string
	}{{1, "CLAN_ANN"}, {2, "CLAN_CBB"}} {
		if n := queryInt(t, w, `SELECT COUNT(*) FROM bbs_forum WHERE id = ? AND type = ? AND access = 'READ' AND owner_id = ?`, f.id, f.typ, clanID); n != 1 {
			t.Fatalf("forum %d %s of clan %d stored %d times, want once", f.id, f.typ, clanID, n)
		}
	}
	if n := queryInt(t, w, `SELECT COUNT(*) FROM bbs_forum`); n != 2 {
		t.Fatalf("bbs_forum rows = %d, want 2", n)
	}
}

// TestClanLevelBoardOffCreatesNoForums pins the board off, the shipped
// default: a clan reaching level 2 gets no forum.
func TestClanLevelBoardOffCreatesNoForums(t *testing.T) {
	w := bootClanWorld(t, 10, 180000, 3150000)
	raiseToLevel2(t, w)
	if n := queryInt(t, w, `SELECT COUNT(*) FROM bbs_forum`); n != 0 {
		t.Fatalf("bbs_forum rows = %d, want none", n)
	}
}
