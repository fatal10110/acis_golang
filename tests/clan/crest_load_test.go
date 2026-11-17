package clan

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	datacache "github.com/fatal10110/acis_golang/internal/gameserver/data/cache"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// bootWithCrestDir boots the founder's seeded clan pointing at crests
// 700/701/702 (pledge/large/alliance) with the crests dir loads, and
// returns the stored ids once the boot's writes have landed.
func bootWithCrestDir(t *testing.T, dir string) (crest, large, ally int64) {
	t.Helper()
	crests, _, _ := datacache.LoadCrests(dir)
	if crests == nil {
		t.Fatalf("LoadCrests(%q) returned no cache", dir)
	}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Founder", 40, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithCrests(crests),
		gameservertest.WithClanSeed(func(db *sql.DB) {
			for _, q := range []string{
				`UPDATE characters SET clanid = 268435456, power_grade = 0 WHERE char_name = 'Founder'`,
				`INSERT INTO clan_data (clan_id, clan_name, clan_level, leader_id, crest_id, crest_large_id, ally_crest_id)
					SELECT 268435456, 'Seeded', 3, obj_Id, 700, 701, 702 FROM characters WHERE char_name = 'Founder'`,
			} {
				if _, err := db.ExecContext(context.Background(), q); err != nil {
					t.Fatalf("%s: %v", q, err)
				}
			}
		}),
	)
	srv.FlushPersistence(t)
	if err := srv.DB.QueryRowContext(context.Background(),
		`SELECT crest_id, crest_large_id, ally_crest_id FROM clan_data WHERE clan_id = ?`, seededClanID).Scan(&crest, &large, &ally); err != nil {
		t.Fatal(err)
	}
	return crest, large, ally
}

// A stray crest file does not end the load: the large crest read after it
// keeps its clan row, and only the crest the directory lacks is cleared.
func TestBootStrayCrestFileKeepsLaterCrests(t *testing.T) {
	dir := t.TempDir()
	for name, size := range map[string]int{
		"Crest_700.dds":        pledgeCrestSize,
		"Crest_123 - Copy.dds": pledgeCrestSize,
		"LargeCrest_701.dds":   largeCrestSize,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), bytes.Repeat([]byte{9}, size), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if crest, large, ally := bootWithCrestDir(t, dir); crest != 700 || large != 701 || ally != 0 {
		t.Fatalf("stored crests = %d/%d/%d, want 700/701/0", crest, large, ally)
	}
}

// A crest file that cannot be read makes the load partial: no clan row is
// cleared, since the load cannot tell which crests are gone.
func TestBootUnreadableCrestFileClearsNoCrest(t *testing.T) {
	dir := t.TempDir()
	if err := os.Symlink(filepath.Join(dir, "gone"), filepath.Join(dir, "Crest_700.dds")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "LargeCrest_701.dds"), bytes.Repeat([]byte{9}, largeCrestSize), 0o644); err != nil {
		t.Fatal(err)
	}
	if crest, large, ally := bootWithCrestDir(t, dir); crest != 700 || large != 701 || ally != 702 {
		t.Fatalf("stored crests = %d/%d/%d, want 700/701/702", crest, large, ally)
	}
}
