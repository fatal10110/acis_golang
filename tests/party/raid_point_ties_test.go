package party

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

// TestBossRecordTieRanksInHashTableOrder pins ExGetBossRecord's rank for a
// player tied with another: equal totals keep the order of the 16-bucket
// hash table the ranking is built in, a bucket being the object id's low
// bits folded with its high half, not the object ids' order. The rival is
// picked so that the two orders disagree.
func TestBossRecordTieRanksInHashTableOrder(t *testing.T) {
	t.Parallel()
	g := bootGroup(t, []seat{{"First", 40}})
	me := g.players[0].id
	bucket := func(id int32) uint32 {
		h := uint32(id)
		return (h ^ h>>16) & 15
	}
	var rival, want int32
	for d := int32(1); d <= 16 && rival == 0; d++ {
		for _, id := range []int32{me + d, me - d} {
			byBucket, byID := bucket(id) < bucket(me), id < me
			if bucket(id) != bucket(me) && byBucket != byID {
				rival, want = id, 1
				if byBucket {
					want = 2
				}
				break
			}
		}
	}
	points := g.srv.RaidPoints
	points.Add(me, 25001, 10)
	points.Add(rival, 25002, 10)

	r := wire.NewReader(g.bossRecord(t, 0)[3:])
	if rank, total := r.ReadInt32(), r.ReadInt32(); rank != want || total != 10 {
		t.Fatalf("player %#x tied with %#x: rank %d total %d, want rank %d total 10", me, rival, rank, total, want)
	}
}
