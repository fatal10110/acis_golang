package petition

import (
	"slices"
	"testing"
	"time"
)

// countingIDs numbers petitions from 1000 and records what it takes back.
type countingIDs struct {
	next     int32
	released []int32
}

func (c *countingIDs) NextID() (int32, error) { c.next++; return 1000 + c.next, nil }

func (c *countingIDs) ReleaseID(id int32) { c.released = append(c.released, id) }

type nobody struct{}

func (nobody) Online(int32) bool { return false }
func (nobody) GM(int32) bool     { return false }

// TestCancelKeepsTheMostRecentCancelled drives a submit/cancel loop and
// requires the player to keep only its keptCancelled most recent cancelled
// petitions, the older ones dropped with their ids freed, while another
// player's cancelled petition and the player's own closed-count stay put.
func TestCancelKeepsTheMostRecentCancelled(t *testing.T) {
	ids := &countingIDs{}
	clock := time.UnixMilli(1_000_000)
	m := NewManager(DefaultConfig(), ids, func() time.Time { return clock }, nil, nil)
	player, other := Person{ID: 1, Name: "Player"}, Person{ID: 2, Name: "Other"}

	if s, _ := m.Submit(other, int32(TypeOther), "x"); s.Result != Submitted {
		t.Fatalf("other submit = %v", s.Result)
	}
	if c, _ := m.Cancel(other, false, nobody{}); c.Result != CancelDone {
		t.Fatalf("other cancel = %v", c.Result)
	}

	var submitted []int32
	for i := range 3 * keptCancelled {
		clock = clock.Add(time.Millisecond)
		s, err := m.Submit(player, int32(TypeOther), "x")
		if err != nil || s.Result != Submitted {
			t.Fatalf("submit %d = %v, %v", i, s.Result, err)
		}
		submitted = append(submitted, s.ID)
		c, _ := m.Cancel(player, false, nobody{})
		if c.Result != CancelDone || c.Remaining != DefaultConfig().MaxPerPlayer {
			t.Fatalf("cancel %d = %+v, want CancelDone with %d remaining", i, c, DefaultConfig().MaxPerPlayer)
		}
	}

	var kept []int32
	for _, s := range m.List() {
		if s.Petitioner == player.ID {
			kept = append(kept, s.ID)
		}
	}
	if want := submitted[len(submitted)-keptCancelled:]; !slices.Equal(kept, want) {
		t.Fatalf("player's petitions = %v, want the %d most recent %v", kept, keptCancelled, want)
	}
	if want := submitted[:len(submitted)-keptCancelled]; !slices.Equal(ids.released, want) {
		t.Fatalf("released ids = %v, want %v", ids.released, want)
	}
	if len(m.Records()) != keptCancelled+1 {
		t.Fatalf("stored records = %d, want %d", len(m.Records()), keptCancelled+1)
	}
	if _, ok := m.Read(submitted[0]); ok {
		t.Fatal("a dropped petition can still be read")
	}
}

// TestGMNoticesAreBoundedPerPlayer requires one player's submits and
// cancels to tell the game masters at most gmNoticeBurst times per
// gmNoticeWindow, another player keeping its own budget, and the budget
// coming back once the window has run out.
func TestGMNoticesAreBoundedPerPlayer(t *testing.T) {
	clock := time.Unix(1_000_000, 0)
	m := NewManager(DefaultConfig(), &countingIDs{}, func() time.Time { return clock }, nil, nil)
	player, other := Person{ID: 1, Name: "Player"}, Person{ID: 2, Name: "Other"}

	loop := func(p Person, rounds int) (notified int) {
		for range rounds {
			s, err := m.Submit(p, int32(TypeOther), "x")
			if err != nil || s.Result != Submitted {
				t.Fatalf("submit = %v, %v", s.Result, err)
			}
			if s.NotifyGMs {
				notified++
			}
			c, _ := m.Cancel(p, false, nobody{})
			if c.Result != CancelDone {
				t.Fatalf("cancel = %v", c.Result)
			}
			if c.NotifyGMs {
				notified++
			}
		}
		return notified
	}

	if n := loop(player, 10); n != gmNoticeBurst {
		t.Fatalf("notices in one window = %d, want %d", n, gmNoticeBurst)
	}
	if n := loop(other, 1); n != 2 {
		t.Fatalf("other player's notices = %d, want 2", n)
	}
	clock = clock.Add(gmNoticeWindow - time.Second)
	if n := loop(player, 1); n != 0 {
		t.Fatalf("notices before the window ran out = %d, want 0", n)
	}
	clock = clock.Add(time.Second)
	if n := loop(player, 10); n != gmNoticeBurst {
		t.Fatalf("notices in the next window = %d, want %d", n, gmNoticeBurst)
	}
	if len(m.noticeWindows) != 1 {
		t.Fatalf("notice windows = %d, want only the player's current one", len(m.noticeWindows))
	}
}
