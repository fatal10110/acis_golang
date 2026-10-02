package bbs

import (
	"context"
	"database/sql"
	"strconv"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// favoriteRows reads every bbs_favorite row as "id|player|title|bypass|date",
// in id order, once the queued writes have landed.
func favoriteRows(t *testing.T, srv *gameservertest.Server) []string {
	t.Helper()
	srv.FlushPersistence(t)
	rows, err := srv.DB.QueryContext(context.Background(),
		"SELECT CONCAT_WS('|', id, player_id, title, bypass, DATE_FORMAT(date, '%Y-%m-%d %H:%i:%s')) FROM bbs_favorite ORDER BY id")
	if err != nil {
		t.Fatalf("read bbs_favorite: %v", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var r string
		if err := rows.Scan(&r); err != nil {
			t.Fatalf("scan bbs_favorite: %v", err)
		}
		out = append(out, r)
	}
	return out
}

// TestFavoritesBoard walks the favorites board over stored favorites:
// the list shows the player's own, each through the row template; an add
// stores the home board under the id after the highest stored one; a
// delete removes only the player's own favorite; each then shows the list.
func TestFavoritesBoard(t *testing.T) {
	seed := gameservertest.WithBoardSeed(func(db *sql.DB) {
		if _, err := db.ExecContext(context.Background(), `INSERT INTO bbs_favorite (id, player_id, title, bypass, date)
			SELECT 7, obj_Id, 'Mine', '_bbsclan', '2026-01-05 14:07:09' FROM characters WHERE char_name = 'Alice'
			UNION ALL SELECT 9, 999, 'Theirs', '_bbsmail', '2026-01-06 08:00:00'`); err != nil {
			t.Fatalf("seed favorites: %v", err)
		}
	})
	p := bootPair(t, gameservertest.WithCommunityBoard(boardOn), seed)
	p.enterAll(t)

	mine := "ROW 7|_bbsclan|Mine|2026-01-05 14:07:09;\n"
	assertPage(t, command(t, p.alice, "_bbsgetfav"), "FAVORITES "+mine+"\n")
	assertPage(t, command(t, p.bobby, "_bbsgetfav"), "FAVORITES \n")

	frames := command(t, p.alice, "_bbsgetfav_add")
	rows := favoriteRows(t, p.srv)
	if len(rows) != 3 || len(rows[2]) < len("10|1|Testing favorites|_bbshome|") {
		t.Fatalf("bbs_favorite = %q, want a third row", rows)
	}
	added := rows[2]
	wantPrefix := "10|" + strconv.Itoa(int(p.aliceID)) + "|Testing favorites|_bbshome|"
	if added[:len(wantPrefix)] != wantPrefix {
		t.Fatalf("added favorite = %q, want %q...", added, wantPrefix)
	}
	date := added[len(wantPrefix):]
	assertPage(t, frames, "FAVORITES "+mine+"ROW 10|_bbshome|Testing favorites|"+date+";\n\n")

	// Another character's favorite is not Alice's to delete.
	assertPage(t, command(t, p.alice, "_bbsgetfav_del_9"), "FAVORITES "+mine+"ROW 10|_bbshome|Testing favorites|"+date+";\n\n")
	assertPage(t, command(t, p.alice, "_bbsgetfav_del_7"), "FAVORITES ROW 10|_bbshome|Testing favorites|"+date+";\n\n")
	if got := favoriteRows(t, p.srv); len(got) != 2 || got[0][:2] != "9|" || got[1] != added {
		t.Fatalf("bbs_favorite after deletes = %q, want the other character's 9 and the added 10", got)
	}
	assertPage(t, command(t, p.alice, "_bbsgetfav_del_10"), "FAVORITES \n")

	assertPage(t, command(t, p.alice, "_bbsgetfavorites"), "<html><body><br><br><center>The command: _bbsgetfavorites isn't implemented.</center></body></html>")
	for _, cmd := range []string{"_bbsgetfav_del", "_bbsgetfav_del_x"} {
		if frames := command(t, p.alice, cmd); len(frames) != 0 {
			t.Fatalf("%s answer = %x, want silence", cmd, opcodes(frames))
		}
	}
}
