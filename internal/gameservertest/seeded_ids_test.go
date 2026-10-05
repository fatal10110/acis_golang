package gameservertest

import (
	"context"
	"database/sql"
	"slices"
	"testing"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// TestAllocatorSkipsSeededObjectIDs pins the harness id source to the
// production allocator's contract: an object id a suite seeds straight into
// the database — before Boot allocates anything, or from a seed hook that
// runs after Boot already allocated some — is never handed out again, so no
// two live objects share an ObjectID.
func TestAllocatorSkipsSeededObjectIDs(t *testing.T) {
	const (
		seededChar = 103
		seededItem = 105
		lateClan   = 130
	)
	srv := Boot(t,
		WithCharacter("Login", 1, 0),
		WithWantChars(1),
		WithSeed(func(chars *gamesql.CharacterStore, items *gamesql.ItemStore) {
			ch, err := player.NewCharacter(seededChar, ClassTemplate(), "others", "Seeded", 1, 0, 0, player.SexMale)
			if err != nil {
				t.Fatalf("seed character: %v", err)
			}
			if err := chars.Create(context.Background(), ch); err != nil {
				t.Fatalf("seed character store: %v", err)
			}
			if err := items.Create(context.Background(), seededChar, item.Instance{
				ObjectID: seededItem, TemplateID: item.AdenaID, OwnerID: seededChar, Count: 1, Location: item.LocationInventory,
			}); err != nil {
				t.Fatalf("seed item: %v", err)
			}
		}),
		WithClanSeed(func(db *sql.DB) {
			if _, err := db.Exec(`INSERT INTO clan_data (clan_id, clan_name, leader_id) VALUES (?, 'Late', ?)`, lateClan, seededChar); err != nil {
				t.Fatalf("seed clan: %v", err)
			}
		}),
	)

	got := []int32{srv.SoleObjectID(t)}
	for range 60 {
		got = append(got, srv.NewObjectID())
	}
	for _, seeded := range []int32{seededChar, seededItem, lateClan} {
		if slices.Contains(got, seeded) {
			t.Errorf("allocated ids %v include seeded id %d", got, seeded)
		}
	}
	sorted := slices.Clone(got)
	slices.Sort(sorted)
	if len(slices.Compact(sorted)) != len(got) {
		t.Errorf("allocated ids %v repeat an id", got)
	}
}

// TestReserveReportsSeededIDsAlreadyAllocated pins the clash check a seed
// hook running after allocation began relies on: a newly stored id the
// sequence already handed out is reported, while one it has not reached
// yet, or one below where it started, is only skipped from then on.
func TestReserveReportsSeededIDsAlreadyAllocated(t *testing.T) {
	ids := newSequentialIDs(100)
	ids.reserve([]int64{102})
	for range 3 {
		ids.nextID() // 101, 103, 104
	}
	if clash := ids.reserve([]int64{50, 101, 102, 104, 110}); !slices.Equal(clash, []int32{101, 104}) {
		t.Fatalf("reserve clash = %v, want [101 104]", clash)
	}
	for _, want := range []int32{105, 106, 107, 108, 109, 111} {
		if got := ids.nextID(); got != want {
			t.Fatalf("nextID() = %d, want %d", got, want)
		}
	}
}
