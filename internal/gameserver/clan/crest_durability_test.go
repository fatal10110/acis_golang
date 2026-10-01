package clan

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	datacache "github.com/fatal10110/acis_golang/internal/gameserver/data/cache"
	"github.com/rs/zerolog"
)

// failingCrestStore refuses every crest column write.
type failingCrestStore struct{ *fakeStore }

func (failingCrestStore) UpdateCrest(context.Context, int32, datacache.CrestType, int32) error {
	return errors.New("db down")
}

func crestedClanService(t *testing.T, store Store, writes Writer) (*Service, *Clan, *datacache.Crests) {
	t.Helper()
	const leaderID = 77
	table := NewTable()
	table.Restore(Snapshot{
		Clans:   []Row{{ID: 1, Name: "Crested", Level: 3, LeaderID: leaderID, CrestID: 400}},
		Members: []MemberRow{{ClanID: 1, Member: Member{ObjectID: leaderID, Name: "Lead"}}},
	}, time.Now(), 1)
	files := datacache.NewCrestsIn(t.TempDir())
	if err := files.Save(datacache.PledgeCrest, 400, bytes.Repeat([]byte{0x01}, 256)); err != nil {
		t.Fatal(err)
	}
	s := NewService(table, store, writes, &seqIDs{next: 500}, DefaultConfig(), nil, zerolog.Nop())
	cl, _ := table.Get(1)
	return s, cl, files
}

// A replaced crest image stays on disk until the clan_data column naming
// the new crest has been written, so a crash while the write is still
// queued leaves the stored column pointing at an existing image.
func TestReplacedCrestRemovedAfterColumnWrite(t *testing.T) {
	store := newFakeStore()
	writes := &laneWriter{}
	s, cl, files := crestedClanService(t, store, writes)
	leader := inClan(77, "Lead", 1)

	if _, got := s.SetCrest(leader, datacache.PledgeCrest, bytes.Repeat([]byte{0x02}, 256), files, time.Now()); got != CrestRegistered {
		t.Fatalf("upload = %v, want CrestRegistered", got)
	}
	if id := cl.Info().CrestID; id != 500 {
		t.Fatalf("crest id = %d, want 500", id)
	}
	if !files.Has(datacache.PledgeCrest, 400) {
		t.Fatal("replaced crest 400 removed while its column write is still queued")
	}

	writes.drain()
	if got := store.crests[[2]int32{1, int32(datacache.PledgeCrest)}]; got != 500 {
		t.Fatalf("stored crest_id = %d, want 500", got)
	}
	if files.Has(datacache.PledgeCrest, 400) {
		t.Fatal("replaced crest 400 kept after the column write landed")
	}
	if !files.Has(datacache.PledgeCrest, 500) {
		t.Fatal("new crest 500 missing")
	}
}

// A crest column write that fails keeps the image the stored column still
// names.
func TestReplacedCrestKeptWhenColumnWriteFails(t *testing.T) {
	s, _, files := crestedClanService(t, failingCrestStore{newFakeStore()}, nil)
	leader := inClan(77, "Lead", 1)

	if _, got := s.SetCrest(leader, datacache.PledgeCrest, nil, files, time.Now()); got != CrestDeleted {
		t.Fatalf("deletion = %v, want CrestDeleted", got)
	}
	if !files.Has(datacache.PledgeCrest, 400) {
		t.Fatal("crest 400 removed although its column was never cleared")
	}
}
