package network

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/shortcut"
)

// --- fake character/item stores: Roster's own persistence seam, no DB needed ---

type fakeCharStore struct {
	mu             sync.Mutex
	byID           map[int32]*player.Character
	names          map[string]bool
	savedPositions map[int32]savedPosition
	online         map[int32]int64
	offline        map[int32]int64
	saveCount      map[int32]int
	onlineSeq      map[int32][]string

	// saveHook, when set, runs at the start of Save before the online-status
	// write below is recorded — outside s.mu so a test can block one Save
	// call in flight (simulating a slow write) without deadlocking a
	// concurrent caller on a different id.
	saveHook func(id int32)
}

func newFakeCharStore() *fakeCharStore {
	return &fakeCharStore{byID: map[int32]*player.Character{}, names: map[string]bool{}, savedPositions: map[int32]savedPosition{}, online: map[int32]int64{}, offline: map[int32]int64{}, saveCount: map[int32]int{}, onlineSeq: map[int32][]string{}}
}

type savedPosition struct {
	location    location.Location
	heading     int
	ctxErr      error
	hasDeadline bool
	deadline    time.Time
}

func (s *fakeCharStore) Create(_ context.Context, c *player.Character) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byID[c.ID] = c
	s.names[c.Name] = true
	return nil
}

func (s *fakeCharStore) Save(_ context.Context, st player.SaveState) error {
	if s.saveHook != nil {
		s.saveHook(st.ID)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saveCount[st.ID]++
	// Mirrors character.go's CharacterStore.Save, which marks the row
	// online (`online = 1`) as part of the same UPDATE (#1948).
	s.onlineSeq[st.ID] = append(s.onlineSeq[st.ID], "online")
	return nil
}

func (s *fakeCharStore) Get(_ context.Context, id int32) (*player.Character, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.byID[id]
	if !ok {
		return nil, fmt.Errorf("character %d not found", id)
	}
	return c, nil
}

func (s *fakeCharStore) saves(id int32) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveCount[id]
}

// onlineSequence returns the ordered sequence of online/offline
// online-status writes recorded for id, so a test can assert which write
// landed last.
func (s *fakeCharStore) onlineSequence(id int32) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.onlineSeq[id]...)
}

func (s *fakeCharStore) ListByAccount(_ context.Context, account string) ([]*player.Character, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*player.Character
	for _, c := range s.byID {
		if c.AccountName == account {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *fakeCharStore) CountByAccount(ctx context.Context, account string) (int, error) {
	list, _ := s.ListByAccount(ctx, account)
	return len(list), nil
}

func (s *fakeCharStore) NameTaken(_ context.Context, name string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.names[name], nil
}

func (s *fakeCharStore) SetDeleteAt(_ context.Context, id int32, at int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c, ok := s.byID[id]; ok {
		c.DeleteAt = at
	}
	return nil
}

func (s *fakeCharStore) SetPosition(ctx context.Context, id int32, loc location.Location, heading int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c, ok := s.byID[id]; ok {
		c.Location = loc
		c.LastHeading = heading
	}
	deadline, hasDeadline := ctx.Deadline()
	s.savedPositions[id] = savedPosition{
		location:    loc,
		heading:     heading,
		ctxErr:      ctx.Err(),
		hasDeadline: hasDeadline,
		deadline:    deadline,
	}
	return ctx.Err()
}

func (s *fakeCharStore) SetDeathPenaltyLevel(_ context.Context, id int32, level int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c, ok := s.byID[id]; ok {
		c.SetDeathPenaltyLevel(level)
	}
	return nil
}

func (s *fakeCharStore) SetOnline(_ context.Context, id int32, lastAccess int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.online[id] = lastAccess
	s.onlineSeq[id] = append(s.onlineSeq[id], "online")
	return nil
}

func (s *fakeCharStore) SetOffline(_ context.Context, id int32, lastAccess int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.offline[id] = lastAccess
	s.onlineSeq[id] = append(s.onlineSeq[id], "offline")
	return nil
}

// Purge drops the character row only: this fake owns no item or shortcut
// state to cascade into.
func (s *fakeCharStore) Purge(_ context.Context, id int32) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.byID[id]
	delete(s.byID, id)
	return ok, nil
}

func (s *fakeCharStore) soleObjectID(t *testing.T) int32 {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.byID) != 1 {
		t.Fatalf("fakeCharStore has %d characters, want 1", len(s.byID))
	}
	for id := range s.byID {
		return id
	}
	return 0
}

func (s *fakeCharStore) savedPosition(t *testing.T, id int32) savedPosition {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	pos, ok := s.savedPositions[id]
	if !ok {
		t.Fatalf("character %d position was not saved", id)
	}
	return pos
}

type fakeItemStore struct {
	mu             sync.Mutex
	items          map[int32][]*item.Instance
	listByOwnerErr error
}

// failListByOwner makes every later ListByOwner return err.
func (s *fakeItemStore) failListByOwner(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listByOwnerErr = err
}

func newFakeItemStore() *fakeItemStore {
	return &fakeItemStore{items: map[int32][]*item.Instance{}}
}

func (s *fakeItemStore) Create(_ context.Context, ownerID int32, inst item.Instance) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := inst
	s.items[ownerID] = append(s.items[ownerID], &cp)
	return nil
}

func (s *fakeItemStore) DeleteByOwner(_ context.Context, ownerID int32) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := int64(len(s.items[ownerID]))
	delete(s.items, ownerID)
	return n, nil
}

func (s *fakeItemStore) ListByOwner(_ context.Context, ownerID int32) ([]*item.Instance, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listByOwnerErr != nil {
		return nil, s.listByOwnerErr
	}
	return append([]*item.Instance(nil), s.items[ownerID]...), nil
}

func (s *fakeItemStore) Save(_ context.Context, inst *item.Instance) error {
	return s.SaveState(context.Background(), inst.Snapshot())
}

func (s *fakeItemStore) SaveState(_ context.Context, st item.InstanceState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saveLocked(st)
	return nil
}

func (s *fakeItemStore) saveLocked(st item.InstanceState) {
	cp := st.Instance()
	for ownerID, items := range s.items {
		for i, existing := range items {
			if existing.ObjectID == cp.ObjectID {
				s.items[ownerID][i] = cp
				return
			}
		}
	}
	s.items[cp.OwnerID] = append(s.items[cp.OwnerID], cp)
}

func (s *fakeItemStore) Update(ctx context.Context, inst *item.Instance) error {
	return s.Save(ctx, inst)
}

func (s *fakeItemStore) UpdateState(ctx context.Context, st item.InstanceState) error {
	return s.SaveState(ctx, st)
}

func (s *fakeItemStore) Delete(_ context.Context, objectID int32) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deleteLocked(objectID)
	return nil
}

func (s *fakeItemStore) deleteLocked(objectID int32) {
	for ownerID, items := range s.items {
		for i, existing := range items {
			if existing.ObjectID == objectID {
				s.items[ownerID] = append(items[:i], items[i+1:]...)
				return
			}
		}
	}
}

// WriteBatch applies batch's saves, then its deletes, under one hold of the
// store, so no reader sees part of it.
func (s *fakeItemStore) WriteBatch(_ context.Context, batch item.FlushBatch) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, st := range batch.Saves {
		s.saveLocked(st)
	}
	for _, id := range batch.Deletes {
		s.deleteLocked(id)
	}
	return nil
}

func (s *fakeItemStore) SetEnchantOwned(_ context.Context, ownerID, objectID int32, enchant int) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, items := range s.items {
		for _, existing := range items {
			if existing.ObjectID != objectID {
				continue
			}
			if existing.OwnerID != ownerID {
				return false, nil
			}
			existing.EnchantLevel = enchant
			return true, nil
		}
	}
	return false, nil
}

func (s *fakeItemStore) DeleteOwned(_ context.Context, ownerID, objectID int32) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for bucket, items := range s.items {
		for i, existing := range items {
			if existing.ObjectID != objectID {
				continue
			}
			if existing.OwnerID != ownerID {
				return false, nil
			}
			s.items[bucket] = append(items[:i], items[i+1:]...)
			return true, nil
		}
	}
	return false, nil
}

type fakeShortcutStore struct {
	mu             sync.Mutex
	byOwner        map[int32][]shortcut.Shortcut
	listByOwnerErr error
	saveErr        error
	deleteErr      error
}

func newFakeShortcutStore() *fakeShortcutStore {
	return &fakeShortcutStore{byOwner: map[int32][]shortcut.Shortcut{}}
}

func (s *fakeShortcutStore) ListByOwner(_ context.Context, ownerID int32, _ int) ([]shortcut.Shortcut, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listByOwnerErr != nil {
		return nil, s.listByOwnerErr
	}
	return append([]shortcut.Shortcut(nil), s.byOwner[ownerID]...), nil
}

func (s *fakeShortcutStore) Save(_ context.Context, ownerID int32, _ int, sc shortcut.Shortcut) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.saveErr != nil {
		return s.saveErr
	}
	list := shortcut.NewList(s.byOwner[ownerID])
	list.Register(sc)
	s.byOwner[ownerID] = list.All()
	return nil
}

func (s *fakeShortcutStore) Delete(_ context.Context, ownerID int32, _ int, slot, page int32) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.deleteErr != nil {
		return s.deleteErr
	}
	list := shortcut.NewList(s.byOwner[ownerID])
	list.Delete(slot, page)
	s.byOwner[ownerID] = list.All()
	return nil
}

func (s *fakeShortcutStore) DeleteByOwner(_ context.Context, ownerID int32) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.byOwner, ownerID)
	return nil
}

type sequentialIDs struct{ next int32 }

func (s *sequentialIDs) NextID() (int32, error) {
	s.next++
	return s.next, nil
}

// testGeo is an always-passable move.Geo double for tests that don't
// exercise geodata behavior.
type testGeo struct{}

func (testGeo) CanMove(int, int, int, int, int, int) bool { return true }
func (testGeo) Height(_, _, z int) int16                  { return int16(z) }

func (testGeo) FindPath(_, _ location.Location) ([]location.Location, bool) { return nil, false }
func (testGeo) Walkable(int, int, int) bool                                 { return true }

func (g testGeo) CanFly(ox, oy, oz int, _ float64, tx, ty, tz int) bool {
	return g.CanMove(ox, oy, oz, tx, ty, tz)
}

func (g testGeo) ValidFlyLocation(ox, oy, oz int, _ float64, tx, ty, tz int) location.Location {
	return g.ValidLocation(ox, oy, oz, tx, ty, tz)
}

func (testGeo) ValidLocation(ox, oy, oz, _, _, _ int) location.Location {
	return location.Location{X: ox, Y: oy, Z: oz}
}
