package bbs

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

// laneWriter queues each lane's jobs in the order they were enqueued and
// runs them only when drained, as the persistence worker runs one lane's
// jobs one after another, later than they were made.
type laneWriter struct {
	mu    sync.Mutex
	lanes map[int32][]func()
}

func (w *laneWriter) Enqueue(lane int32, job func()) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.lanes == nil {
		w.lanes = map[int32][]func(){}
	}
	w.lanes[lane] = append(w.lanes[lane], job)
	return true
}

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

// forumWrite is one write a forum store took.
type forumWrite struct {
	op      string
	forumID int32
	topicID int32
	text    string
}

// forumLog records every forum write in the order the store took it.
type forumLog struct {
	mu     sync.Mutex
	writes []forumWrite
}

func (s *forumLog) add(w forumWrite) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.writes = append(s.writes, w)
	return nil
}

func (s *forumLog) InsertForum(_ context.Context, f Forum) error {
	return s.add(forumWrite{op: "forum", forumID: f.ID})
}

func (s *forumLog) InsertTopic(_ context.Context, t Topic, first Post) error {
	return s.add(forumWrite{op: "topic", forumID: t.ForumID, topicID: t.ID, text: first.Text})
}

func (s *forumLog) DeleteTopic(_ context.Context, forumID, topicID int32) error {
	return s.add(forumWrite{op: "delete", forumID: forumID, topicID: topicID})
}

func (s *forumLog) UpdatePostText(_ context.Context, p Post) error {
	return s.add(forumWrite{op: "edit", forumID: p.ForumID, topicID: p.TopicID, text: p.Text})
}

// favoriteLog records the favorite ids the store inserted and deleted,
// in the order it took them.
type favoriteLog struct {
	mu                sync.Mutex
	inserted, deleted []int32
	order             []int32 // each write's id, negated for a delete
}

func (s *favoriteLog) InsertFavorite(_ context.Context, f Favorite) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.inserted = append(s.inserted, f.ID)
	s.order = append(s.order, f.ID)
	return nil
}

func (s *favoriteLog) DeleteFavorite(_ context.Context, id int32) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deleted = append(s.deleted, id)
	s.order = append(s.order, -id)
	return nil
}

// TestForumsConcurrentUse drives many players' queues through one forum
// registry at once: each owner's memo forum is opened from several queues,
// clans gain their forums from several queues, and every queue adds,
// edits, reads and deletes topics in one shared memo forum. Each owner
// gets one forum, forum ids and topic ids are distinct and gap-free, and
// each forum's rows reach the store in the order the forum changed:
// the forum first, then each topic's insert, edit and delete in turn.
func TestForumsConcurrentUse(t *testing.T) {
	const (
		owners       = 8
		clans        = 4
		queues       = 12
		topicsEach   = 10
		sharedOwner  = int32(1)
		firstClanID  = int32(500)
		wantForumIDs = owners + 2*clans
	)
	store := &forumLog{}
	lanes := &laneWriter{}
	forums := NewForums(store, lanes, zerolog.Nop())
	now := time.Now()

	var (
		wg    sync.WaitGroup
		mu    sync.Mutex
		memos = map[int32][]int32{}
	)
	start := make(chan struct{})
	for q := range queues {
		wg.Go(func() {
			<-start
			for o := range owners {
				memo := forums.Memo(int32(o) + 1)
				mu.Lock()
				memos[memo.OwnerID] = append(memos[memo.OwnerID], memo.ID)
				mu.Unlock()
			}
			for c := range clans {
				forums.EnsureClanForums(firstClanID+int32(c), clanForumLevel)
			}
			shared := forums.Memo(sharedOwner)
			for i := range topicsEach {
				if !forums.AddTopic(shared.ID, "t", "owner", int32(q), "first", now) {
					t.Errorf("AddTopic on forum %d refused", shared.ID)
					return
				}
				v, _ := forums.View(shared.ID)
				id := v.LastTopicID
				// The topic this queue just added may already be gone,
				// deleted by another queue reading the same list.
				forums.EditPost(shared.ID, id, 0, "edited")
				forums.TopicPost(shared.ID, id, 0)
				if i%2 == 0 {
					forums.DeleteTopic(shared.ID, id)
				}
			}
		})
	}
	close(start)
	wg.Wait()
	lanes.drain()

	// One memo forum per owner, the same one every queue saw.
	ids := map[int32]bool{}
	for o := range owners {
		owner := int32(o) + 1
		got := memos[owner]
		if len(got) != queues || slices.Min(got) != slices.Max(got) {
			t.Fatalf("owner %d memo forums seen = %v, want one id seen %d times", owner, got, queues)
		}
		owned, ok := forums.Owned(ForumMemo, owner)
		if !ok || owned.ID != got[0] {
			t.Fatalf("owner %d memo = %+v, %t; want forum %d", owner, owned, ok, got[0])
		}
		ids[owned.ID] = true
	}
	for c := range clans {
		clan := firstClanID + int32(c)
		ann, okAnn := forums.Owned(ForumClanAnnouncements, clan)
		cbb, okCbb := forums.Owned(ForumClanBulletin, clan)
		if !okAnn || !okCbb || ann.Access != AccessRead || cbb.Access != AccessRead {
			t.Fatalf("clan %d forums = %+v %t, %+v %t; want both, read-only", clan, ann, okAnn, cbb, okCbb)
		}
		if ann.ID > cbb.ID {
			t.Fatalf("clan %d announcement forum %d after its bulletin forum %d", clan, ann.ID, cbb.ID)
		}
		ids[ann.ID], ids[cbb.ID] = true, true
	}
	if len(ids) != wantForumIDs {
		t.Fatalf("distinct forum ids = %d, want %d", len(ids), wantForumIDs)
	}
	for id := int32(1); id <= wantForumIDs; id++ {
		if !ids[id] {
			t.Fatalf("forum ids %v skip %d", ids, id)
		}
	}

	// Topic ids are handed out once each, from 1 with no gap.
	shared, _ := forums.Owned(ForumMemo, sharedOwner)
	v, _ := forums.View(shared.ID)
	if want := int32(queues * topicsEach); v.LastTopicID != want {
		t.Fatalf("last topic id = %d, want %d", v.LastTopicID, want)
	}

	// The store took each forum's rows in the order the forum changed.
	store.mu.Lock()
	defer store.mu.Unlock()
	byForum := map[int32][]forumWrite{}
	for _, w := range store.writes {
		byForum[w.forumID] = append(byForum[w.forumID], w)
	}
	if len(byForum) != wantForumIDs {
		t.Fatalf("forums written = %d, want %d", len(byForum), wantForumIDs)
	}
	topicIDs := map[int32]bool{}
	for forumID, writes := range byForum {
		if writes[0].op != "forum" {
			t.Fatalf("forum %d first write = %+v, want its insert", forumID, writes[0])
		}
		state := map[int32]string{}
		for _, w := range writes[1:] {
			switch prev := state[w.topicID]; w.op {
			case "forum":
				t.Fatalf("forum %d inserted twice", forumID)
			case "topic":
				if prev != "" {
					t.Fatalf("forum %d topic %d inserted after %q", forumID, w.topicID, prev)
				}
				topicIDs[w.topicID] = true
			case "edit", "delete":
				if prev == "" || prev == "delete" {
					t.Fatalf("forum %d topic %d %s after %q", forumID, w.topicID, w.op, prev)
				}
			}
			state[w.topicID] = w.op
		}
	}
	if len(topicIDs) != queues*topicsEach {
		t.Fatalf("topics inserted = %d, want %d", len(topicIDs), queues*topicsEach)
	}
	for id := int32(1); id <= queues*topicsEach; id++ {
		if !topicIDs[id] {
			t.Fatalf("topic ids skip %d", id)
		}
	}
	// The store holds what the registry holds: every topic not deleted.
	stored := map[int32]bool{}
	for _, w := range byForum[shared.ID] {
		switch w.op {
		case "topic":
			stored[w.topicID] = true
		case "delete":
			delete(stored, w.topicID)
		}
	}
	if len(stored) != len(v.Topics) {
		t.Fatalf("store holds %d topics, registry %d", len(stored), len(v.Topics))
	}
	for _, tp := range v.Topics {
		if !stored[tp.ID] {
			t.Fatalf("registry topic %d is not stored", tp.ID)
		}
	}
}

// TestFavoritesConcurrentUse drives many players' queues adding and
// deleting favorites at once: every favorite id is handed out once, from
// the one after the highest stored, and the store took each insert before
// its delete.
func TestFavoritesConcurrentUse(t *testing.T) {
	const (
		players  = 8
		addsEach = 25
		storedID = int32(40)
	)
	store := &favoriteLog{}
	lanes := &laneWriter{}
	favs := NewFavorites(store, lanes, zerolog.Nop())
	favs.Restore([]Favorite{{ID: storedID, PlayerID: 99, Title: "t", Bypass: "_bbshome", Date: time.Now()}})

	var wg sync.WaitGroup
	start := make(chan struct{})
	for p := range players {
		player := int32(p) + 1
		wg.Go(func() {
			<-start
			for i := range addsEach {
				favs.Add(player, time.Now())
				list := favs.List(player)
				if i%3 == 0 {
					favs.Delete(player, list[len(list)-1].ID)
				}
				// Another player's favorite is not this one's to delete.
				favs.Delete(player, storedID)
			}
		})
	}
	close(start)
	wg.Wait()
	lanes.drain()

	seen := map[int32]bool{}
	kept := 0
	for p := range players {
		list := favs.List(int32(p) + 1)
		kept += len(list)
		for i, f := range list {
			if seen[f.ID] {
				t.Fatalf("favorite id %d held twice", f.ID)
			}
			seen[f.ID] = true
			if i > 0 && list[i-1].ID >= f.ID {
				t.Fatalf("player %d favorites out of id order: %d then %d", p+1, list[i-1].ID, f.ID)
			}
		}
	}
	if other := favs.List(99); len(other) != 1 || other[0].ID != storedID {
		t.Fatalf("player 99 favorites = %+v, want the stored %d kept", other, storedID)
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.inserted) != players*addsEach {
		t.Fatalf("favorites inserted = %d, want %d", len(store.inserted), players*addsEach)
	}
	ids := slices.Sorted(slices.Values(store.inserted))
	for i, id := range ids {
		if want := storedID + 1 + int32(i); id != want {
			t.Fatalf("inserted favorite ids %v: position %d = %d, want %d (distinct, gap-free)", ids, i, id, want)
		}
	}
	if got := len(store.inserted) - len(store.deleted); got != kept {
		t.Fatalf("store holds %d favorites, registry %d", got, kept)
	}
	live := map[int32]bool{}
	for _, id := range store.order {
		if id > 0 {
			live[id] = true
			continue
		}
		if !live[-id] {
			t.Fatalf("favorite %d deleted before it was inserted", -id)
		}
		delete(live, -id)
	}
}
