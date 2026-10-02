package sql

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
)

// TestFavoriteStoreLoadUnreadable covers stored favorites missing their
// title, bypass or date: each loads marked unreadable, and a full row
// loads readable with its fields.
func TestFavoriteStoreLoadUnreadable(t *testing.T) {
	ctx := context.Background()
	db := sqltest.SharedDB(t)
	if _, err := db.ExecContext(ctx, `INSERT INTO bbs_favorite (id, player_id, title, bypass, date) VALUES
		(1, 10, 'Mine', '_bbsclan', '2026-01-05 14:07:09'),
		(2, 10, NULL, '_bbsclan', '2026-01-05 14:07:09'),
		(3, 11, 'Mine', NULL, '2026-01-05 14:07:09'),
		(4, 12, 'Mine', '_bbsclan', NULL)`); err != nil {
		t.Fatalf("seed favorites: %v", err)
	}
	favs, err := NewFavoriteStore(db).Load(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(favs) != 4 {
		t.Fatalf("Load = %d favorites, want 4", len(favs))
	}
	first := favs[0]
	if first.ID != 1 || first.PlayerID != 10 || first.Title != "Mine" || first.Bypass != "_bbsclan" || first.Unreadable ||
		first.Date.Format("2006-01-02 15:04:05") != "2026-01-05 14:07:09" {
		t.Fatalf("favorite 1 = %+v, want the full row, readable", first)
	}
	for _, f := range favs[1:] {
		if !f.Unreadable {
			t.Fatalf("favorite %d = %+v, want unreadable", f.ID, f)
		}
	}
}
