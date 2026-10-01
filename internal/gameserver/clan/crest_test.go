package clan

import (
	"bytes"
	"testing"
	"time"

	datacache "github.com/fatal10110/acis_golang/internal/gameserver/data/cache"
	"github.com/rs/zerolog"
)

// seqIDs hands out ids from next upwards.
type seqIDs struct{ next int32 }

func (s *seqIDs) NextID() (int32, error) {
	s.next++
	return s.next - 1, nil
}

// TestSetCrestSkipsStoredCrestID skips a fresh id a stored crest of the
// same family already uses, so the upload never overwrites another clan's
// image, then replaces and deletes the clan's crest, removing the replaced
// image and queueing each column.
func TestSetCrestSkipsStoredCrestID(t *testing.T) {
	const leaderID = 77
	table := NewTable()
	table.Restore(Snapshot{
		Clans:   []Row{{ID: 1, Name: "Crested", Level: 3, LeaderID: leaderID}},
		Members: []MemberRow{{ClanID: 1, Member: Member{ObjectID: leaderID, Name: "Lead"}}},
	}, time.Now(), 1)
	store := newFakeStore()
	files := datacache.NewCrestsIn(t.TempDir())
	other := bytes.Repeat([]byte{0x01}, 256)
	if err := files.Save(datacache.PledgeCrest, 500, other); err != nil {
		t.Fatal(err)
	}
	s := NewService(table, store, nil, &seqIDs{next: 500}, DefaultConfig(), nil, zerolog.Nop())
	leader := inClan(leaderID, "Lead", 1)

	image := bytes.Repeat([]byte{0x02}, 256)
	if _, got := s.SetCrest(leader, datacache.PledgeCrest, image, files, time.Now()); got != CrestRegistered {
		t.Fatalf("upload = %v, want CrestRegistered", got)
	}
	cl, _ := table.Get(1)
	if id := cl.Info().CrestID; id != 501 {
		t.Fatalf("crest id = %d, want 501 (500 is another crest's)", id)
	}
	if data, _ := files.Get(datacache.PledgeCrest, 500); !bytes.Equal(data, other) {
		t.Fatal("the upload overwrote the stored crest 500")
	}
	if got := store.crests[[2]int32{1, int32(datacache.PledgeCrest)}]; got != 501 {
		t.Fatalf("stored crest_id = %d, want 501", got)
	}

	if _, got := s.SetCrest(leader, datacache.PledgeCrest, nil, files, time.Now()); got != CrestDeleted {
		t.Fatalf("deletion = %v, want CrestDeleted", got)
	}
	if files.Has(datacache.PledgeCrest, 501) || cl.Info().CrestID != 0 || store.crests[[2]int32{1, int32(datacache.PledgeCrest)}] != 0 {
		t.Fatal("deletion left the crest image, id or column")
	}
	if _, got := s.SetCrest(leader, datacache.PledgeCrest, nil, files, time.Now()); got != CrestIgnored {
		t.Fatalf("deleting no crest = %v, want CrestIgnored", got)
	}
}
