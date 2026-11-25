package petition

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
)

// TestPetitionSaveIsOneTransaction pins the parity exception of #5010: the
// petition save cuts a feedback to the 512 characters its column holds,
// and a save that fails part way, here on a petition stored twice, leaves
// the previous rows in place rather than losing every petition.
func TestPetitionSaveIsOneTransaction(t *testing.T) {
	t.Parallel()
	r := boot(t)
	r.enterAll(t)
	r.submit(t)
	ctx := context.Background()
	store := gamesql.NewPetitionStore(r.srv.DB)

	records := slices.Clone(r.srv.Petitions.Records())
	if len(records) != 1 {
		t.Fatalf("petitions = %+v, want the one submitted", records)
	}
	records[0].Feedback = strings.Repeat("é", 600)
	if err := store.Save(ctx, records); err != nil {
		t.Fatalf("save with a long feedback: %v", err)
	}
	want := slices.Clone(records)
	want[0].Feedback = strings.Repeat("é", 512)
	if got, err := store.Load(ctx); err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("stored petitions = %+v (%v), want %+v", got, err, want)
	}

	// The failing batch differs from the stored rows, so a save that cleared
	// the tables and inserted the first copy before failing would show it.
	changed := records[0]
	changed.Content = "changed before the failing save"
	changed.Feedback = "changed feedback"
	if err := store.Save(ctx, append(records[:0:0], changed, changed)); err == nil {
		t.Fatal("save of a petition stored twice succeeded")
	}
	if got, err := store.Load(ctx); err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("stored petitions after the failed save = %+v (%v), want %+v", got, err, want)
	}
}
