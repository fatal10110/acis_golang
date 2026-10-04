package bbs

import (
	"testing"
	"time"

	"github.com/rs/zerolog"
)

// TestMemoOf pins a memo forum belonging to its owner alone: another
// player's memo forum, a clan forum owned by an id equal to the player's
// and a missing forum are not the player's memo forum (#3262).
func TestMemoOf(t *testing.T) {
	forums := NewForums(nil, nil, zerolog.Nop())
	memo := forums.Memo(7)
	forums.EnsureClanForums(7, 2)
	clan, _ := forums.Owned(ForumClanAnnouncements, 7)
	for _, tc := range []struct {
		forum, player int32
		want          bool
	}{
		{memo.ID, 7, true},
		{memo.ID, 8, false},
		{clan.ID, 7, false},
		{99, 7, false},
	} {
		if got := forums.MemoOf(tc.forum, tc.player); got != tc.want {
			t.Errorf("MemoOf(%d, %d) = %t, want %t", tc.forum, tc.player, got, tc.want)
		}
	}
}

// TestForumTopicLimit pins a forum taking topicLimit topics and then no
// more, until one is deleted (#3262).
func TestForumTopicLimit(t *testing.T) {
	forums := NewForums(nil, nil, zerolog.Nop())
	memo := forums.Memo(7)
	now := time.Now()
	for i := range topicLimit {
		if !forums.AddTopic(memo.ID, "t", "Alice", 7, "x", now) {
			t.Fatalf("topic %d refused, want it added", i+1)
		}
	}
	if forums.AddTopic(memo.ID, "t", "Alice", 7, "x", now) {
		t.Fatalf("topic %d added, want the full forum to refuse it", topicLimit+1)
	}
	if v, _ := forums.View(memo.ID); len(v.Topics) != topicLimit {
		t.Fatalf("forum holds %d topics, want %d", len(v.Topics), topicLimit)
	}
	if forums.DeleteTopic(memo.ID, 1) != FoundTopic {
		t.Fatal("delete of topic 1 failed")
	}
	if !forums.AddTopic(memo.ID, "t", "Alice", 7, "x", now) {
		t.Fatal("topic after a delete refused, want it added")
	}
}

// TestFavoriteLimit pins a character keeping favoriteLimit favorites and
// no more, while another character still adds its own (#3262).
func TestFavoriteLimit(t *testing.T) {
	favs := NewFavorites(nil, nil, zerolog.Nop())
	now := time.Now()
	for range favoriteLimit + 3 {
		favs.Add(7, now)
	}
	list := favs.List(7)
	if len(list) != favoriteLimit {
		t.Fatalf("favorites = %d, want %d", len(list), favoriteLimit)
	}
	favs.Delete(7, list[0].ID)
	favs.Add(7, now)
	favs.Add(8, now)
	if got := len(favs.List(7)); got != favoriteLimit {
		t.Fatalf("favorites after a delete and an add = %d, want %d", got, favoriteLimit)
	}
	if got := len(favs.List(8)); got != 1 {
		t.Fatalf("other character's favorites = %d, want 1", got)
	}
}
