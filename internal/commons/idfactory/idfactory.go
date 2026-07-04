package idfactory

import (
	"context"
	"database/sql"
	"fmt"
	"sync"

	"github.com/sirupsen/logrus"
)

// FirstObjectID and LastObjectID bound the range of ids Allocator hands out.
const (
	FirstObjectID = 0x10000000
	LastObjectID  = 0x7FFFFFFF
)

// usedObjectIDQueries lists, for every table that persists an object id
// handed out by this allocator, the query that reads those ids back.
var usedObjectIDQueries = [...]string{
	"SELECT obj_Id FROM characters",
	"SELECT object_id FROM items",
	"SELECT clan_id FROM clan_data",
	"SELECT object_id FROM items_on_ground",
	"SELECT id FROM mods_wedding",
	"SELECT oid FROM petition",
}

// Allocator hands out unique object ids, reusing ids released back to it.
//
// An id released mid-session only becomes available again once allocation
// naturally reaches it (ids are handed out in increasing order and the
// search cursor never moves backward) or the Allocator is rebuilt via New,
// which reclaims every id no longer present in the database. This trades
// perfect same-session reuse for O(1) amortized allocation.
//
// mu guards used and next.
type Allocator struct {
	mu   sync.Mutex
	used map[int32]struct{}
	next int32

	first, last int32 // id range; always FirstObjectID/LastObjectID outside tests
	log         *logrus.Logger
}

// New scans db for object ids already in use and returns an Allocator seeded
// with them, ready to hand out ids that don't collide with existing rows. It
// fails loudly on a query error rather than booting with a partial id set.
func New(ctx context.Context, db *sql.DB, log *logrus.Logger) (*Allocator, error) {
	if log == nil {
		log = logrus.StandardLogger()
	}

	a := &Allocator{
		used:  make(map[int32]struct{}),
		first: FirstObjectID,
		last:  LastObjectID,
		log:   log,
	}

	for _, query := range usedObjectIDQueries {
		if err := a.loadUsedIDs(ctx, db, query); err != nil {
			return nil, fmt.Errorf("idfactory: %w", err)
		}
	}

	a.next = a.nextFreeFrom(a.first)
	log.Infof("idfactory: initialized with %d used object ids", len(a.used))
	return a, nil
}

func (a *Allocator) loadUsedIDs(ctx context.Context, db *sql.DB, query string) error {
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("query used object ids (%s): %w", query, err)
	}
	defer rows.Close()

	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return fmt.Errorf("scan used object id (%s): %w", query, err)
		}
		if id < int64(a.first) {
			a.log.Warnf("idfactory: found object id %d below minimum %d, skipping", id, a.first)
			continue
		}
		a.used[int32(id)] = struct{}{}
	}
	return rows.Err()
}

// NextID returns the next available object id and marks it used.
func (a *Allocator) NextID() int32 {
	a.mu.Lock()
	defer a.mu.Unlock()

	id := a.next
	a.used[id] = struct{}{}
	a.next = a.nextFreeFrom(id + 1)
	return id
}

// ReleaseID returns id to the pool so a later NextID call can hand it out
// again. Ids below FirstObjectID never came from this allocator; releasing
// one is logged and ignored rather than corrupting allocator state.
func (a *Allocator) ReleaseID(id int32) {
	if id < a.first {
		a.log.Warnf("idfactory: release of invalid object id %d (< %d) ignored", id, a.first)
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.used, id)
}

// nextFreeFrom returns the first id >= from that isn't marked used. Callers
// hold mu. Panics if the id space is exhausted, mirroring how the caller
// would otherwise silently corrupt or duplicate an object id.
func (a *Allocator) nextFreeFrom(from int32) int32 {
	for id := from; id <= a.last; id++ {
		if _, used := a.used[id]; !used {
			return id
		}
	}
	panic(fmt.Sprintf("idfactory: ran out of object ids in [%d, %d]", a.first, a.last))
}
