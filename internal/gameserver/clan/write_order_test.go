package clan

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	datacache "github.com/fatal10110/acis_golang/internal/gameserver/data/cache"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/rs/zerolog"
)

// storedClan is the clan_data columns the fake store keeps.
type storedClan struct {
	level, reputation int
	newLeaderID       int32
	charPenalty       int64
}

// fakeStore applies the clan writes to rows in memory, as the SQL store
// applies them to clan_data, clan_privs and characters.
type fakeStore struct {
	mu      sync.Mutex
	clans   map[int32]storedClan
	clanIDs map[int32]int32
	grades  map[int32]int
	privs   map[[2]int32]int32
	// crests is each clan's stored crest columns, by clan id and crest
	// type.
	crests map[[2]int32]int32
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		clans: map[int32]storedClan{}, clanIDs: map[int32]int32{},
		grades: map[int32]int{}, privs: map[[2]int32]int32{}, crests: map[[2]int32]int32{},
	}
}

func (f *fakeStore) InsertClan(_ context.Context, r Row) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.clans[r.ID] = storedClan{level: r.Level, newLeaderID: r.NewLeaderID}
	return nil
}

// UpdateClan writes what the SQL UpdateClan writes: every column but the
// level.
func (f *fakeStore) UpdateClan(_ context.Context, r Row) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	row := f.clans[r.ID]
	row.reputation, row.newLeaderID, row.charPenalty = r.Reputation, r.NewLeaderID, r.CharPenaltyExpiry
	f.clans[r.ID] = row
	return nil
}

func (f *fakeStore) UpdateLevel(_ context.Context, clanID int32, level int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	row := f.clans[clanID]
	row.level = level
	f.clans[clanID] = row
	return nil
}

func (f *fakeStore) UpdateReputation(_ context.Context, clanID int32, score int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	row := f.clans[clanID]
	row.reputation = score
	f.clans[clanID] = row
	return nil
}

func (f *fakeStore) SetPrivileges(_ context.Context, clanID int32, rank int, privs int32) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.privs[[2]int32{clanID, int32(rank)}] = privs
	return nil
}

func (f *fakeStore) SaveMembership(_ context.Context, r MembershipRow) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.clanIDs[r.ObjectID] = r.ClanID
	f.grades[r.ObjectID] = r.PowerGrade
	return nil
}

func (f *fakeStore) RemoveMembership(_ context.Context, r RemovalRow) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.clanIDs[r.ObjectID] = 0
	if r.Online {
		f.grades[r.ObjectID] = 0
	}
	return nil
}

func (f *fakeStore) SetPowerGrade(_ context.Context, objectID int32, grade int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.grades[objectID] = grade
	return nil
}

func (f *fakeStore) UpdateCrest(_ context.Context, clanID int32, typ datacache.CrestType, crestID int32) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.crests[[2]int32{clanID, int32(typ)}] = crestID
	return nil
}

// laneWriter keeps each owner's jobs in the order they were queued and runs
// them when drained. The first job queued for holdOwner runs hook before it
// is appended, standing in for the queueing goroutine being preempted just
// before its push.
type laneWriter struct {
	mu        sync.Mutex
	lanes     map[int32][]func()
	holdOwner int32
	hook      func()
}

func (w *laneWriter) Enqueue(ownerID int32, job func()) bool {
	w.mu.Lock()
	var hook func()
	if ownerID == w.holdOwner {
		hook, w.hook = w.hook, nil
	}
	w.mu.Unlock()
	if hook != nil {
		hook()
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.lanes == nil {
		w.lanes = map[int32][]func(){}
	}
	w.lanes[ownerID] = append(w.lanes[ownerID], job)
	return true
}

// drain runs every queued job, each lane in its order.
func (w *laneWriter) drain() {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, jobs := range w.lanes {
		for _, job := range jobs {
			job()
		}
	}
	w.lanes = nil
}

// runCompeting runs op in its own goroutine and waits until it returns or
// is plainly blocked, so the caller's push lands after op's own pushes if
// nothing serializes them.
func runCompeting(op func()) chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		op()
	}()
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
	}
	return done
}

const (
	orderClanID   = 0x10000001
	orderLeaderID = 0x10000002
)

// orderedClan restores a level 5 clan holding reputation with members
// besides its leader, all offline, over a fake store and a lane writer.
func orderedClan(t *testing.T, reputation, members int) (*Service, *Clan, *fakeStore, *laneWriter) {
	t.Helper()
	snap := Snapshot{Clans: []Row{{ID: orderClanID, Name: "Order", Level: 5, Reputation: reputation, LeaderID: orderLeaderID}}}
	snap.Members = append(snap.Members, MemberRow{ClanID: orderClanID, Member: Member{ObjectID: orderLeaderID, Name: "Leader"}})
	for i := range members {
		id := int32(orderLeaderID + 1 + i)
		snap.Members = append(snap.Members, MemberRow{ClanID: orderClanID, Member: Member{ObjectID: id, Name: "Member" + strconv.Itoa(i), PowerGrade: MemberPowerGrade}})
	}
	table := NewTable()
	table.Restore(snap, time.Now(), 1)
	store := newFakeStore()
	store.clans[orderClanID] = storedClan{level: 5, reputation: reputation}
	for _, m := range snap.Members {
		store.clanIDs[m.ObjectID] = orderClanID
	}
	writes := &laneWriter{}
	s := NewService(table, store, writes, nil, DefaultConfig(), nil, zerolog.Nop())
	cl, _ := table.Get(orderClanID)
	return s, cl, store, writes
}

func inClan(id int32, name string, clanID int32) *player.Character {
	c := &player.Character{ID: id, Name: name, CharLevel: 40}
	c.SetClanID(clanID)
	return c
}

// TestOustRacingReputationLevelUpKeepsStoredReputation expels a member
// while the leader buys level 6 with the clan's 10000 reputation, the
// expulsion's clan row being queued just as the level-up runs. The stored
// row ends on the level-up's reputation, as the clan holds it: a stale row
// landing last would hand the price back after a restart.
func TestOustRacingReputationLevelUpKeepsStoredReputation(t *testing.T) {
	s, cl, store, writes := orderedClan(t, 10000, 30)
	leader := inClan(orderLeaderID, "Leader", orderClanID)
	var raised []any
	var done chan struct{}
	writes.holdOwner = orderClanID
	writes.hook = func() {
		done = runCompeting(func() { raised = s.RaiseLevel(leader, nil, time.Now()) })
	}

	if _, _, res := s.Oust(leader, "Member0", func(int32) *player.Character { return nil }, time.Now()); res != Ousted {
		t.Fatalf("oust = %v, want Ousted", res)
	}
	<-done
	if _, ok := raised[len(raised)-1].(LevelRaised); !ok {
		t.Fatalf("level-up notices = %#v, want the clan raised", raised)
	}
	writes.drain()

	info := cl.Info()
	got := store.clans[orderClanID]
	if got.level != info.Level || got.reputation != info.Reputation || got.charPenalty == 0 {
		t.Fatalf("stored clan = %+v, clan holds level %d reputation %d with a recruiting penalty", got, info.Level, info.Reputation)
	}
	if info.Level != 6 || info.Reputation != 0 {
		t.Fatalf("clan = level %d reputation %d, want 6 and 0", info.Level, info.Reputation)
	}
}

// TestJoinRacingOustKeepsExpelledMemberOut expels a recruit just as its
// join row is being queued. The stored row ends clanless, as the clan's
// roster does: a join row landing after the removal would put the expelled
// player back in the clan after a restart.
func TestJoinRacingOustKeepsExpelledMemberOut(t *testing.T) {
	s, cl, store, writes := orderedClan(t, 0, 1)
	leader := inClan(orderLeaderID, "Leader", orderClanID)
	recruit := inClan(orderLeaderID+100, "Recruit", 0)
	var oust OustRefusal
	var done chan struct{}
	writes.holdOwner = recruit.ID
	writes.hook = func() {
		done = runCompeting(func() {
			_, _, oust = s.Oust(leader, "Recruit", func(int32) *player.Character { return nil }, time.Now())
		})
	}

	if res := s.Join(cl, orderLeaderID, recruit, SubunitMain, time.Now()); res != JoinAllowed {
		t.Fatalf("join = %v, want JoinAllowed", res)
	}
	<-done
	if oust != Ousted {
		t.Fatalf("oust = %v, want Ousted", oust)
	}
	writes.drain()

	if cl.IsMember(recruit.ID) {
		t.Fatal("expelled recruit is still on the roster")
	}
	if got := store.clanIDs[recruit.ID]; got != 0 {
		t.Fatalf("stored clan of the expelled recruit = %d, want 0", got)
	}
}

// TestCancelNominationRacingOustKeepsPenalty cancels a leader nomination
// while a member is expelled, the cancellation's clan row being queued
// just as the expulsion runs. The stored row keeps the recruiting penalty
// the expulsion set.
func TestCancelNominationRacingOustKeepsPenalty(t *testing.T) {
	s, cl, store, writes := orderedClan(t, 0, 2)
	leader := inClan(orderLeaderID, "Leader", orderClanID)
	cl.mu.Lock()
	cl.newLeaderID = orderLeaderID + 1
	cl.mu.Unlock()
	var done chan struct{}
	writes.holdOwner = orderClanID
	writes.hook = func() {
		done = runCompeting(func() { s.Oust(leader, "Member1", func(int32) *player.Character { return nil }, time.Now()) })
	}

	if res := s.CancelNomination(leader); res != NominationCancelled {
		t.Fatalf("cancel = %v, want NominationCancelled", res)
	}
	<-done
	writes.drain()

	info := cl.rowSnapshot()
	if got := store.clans[orderClanID]; got.charPenalty != info.CharPenaltyExpiry || got.charPenalty == 0 || got.newLeaderID != 0 {
		t.Fatalf("stored clan = %+v, clan holds penalty %d and no nominee", got, info.CharPenaltyExpiry)
	}
}

func (cl *Clan) rowSnapshot() Row {
	cl.mu.RLock()
	defer cl.mu.RUnlock()
	return cl.rowLocked()
}
