package bbs

import (
	"testing"

	"github.com/rs/zerolog"
)

// restoredState is what a restore left in the registry: each forum's
// identity, its topic ids in descending order, the highest topic id it
// holds, and how many posts each topic has.
type restoredState struct {
	forum   Forum
	topics  []int32
	last    int32
	posts   map[int32]int
	present bool
}

func restored(f *Forums, id int32) restoredState {
	v, ok := f.View(id)
	if !ok {
		return restoredState{}
	}
	s := restoredState{forum: v.Forum, last: v.LastTopicID, posts: map[int32]int{}, present: true}
	for _, t := range v.Topics {
		s.topics = append(s.topics, t.ID)
		f.mu.Lock()
		s.posts[t.ID] = len(f.forums[id].topics[t.ID].posts)
		f.mu.Unlock()
	}
	return s
}

// TestForumsRestoreStopsAtUnreadableForum pins the boot load of the
// forums: the first row whose type or access is not one of the stored
// names stops the load there. The forums before it are kept, the forums
// after it are not, and no topic or post of any forum is loaded, since
// the topic and post loads never run.
func TestForumsRestoreStopsAtUnreadableForum(t *testing.T) {
	for _, bad := range []ForumRow{
		{ID: 2, Type: "CLAN", Access: "ALL", OwnerID: 20},
		{ID: 2, Type: "MEMO", Access: "EVERYONE", OwnerID: 20},
		{ID: 2, Type: "memo", Access: "ALL", OwnerID: 20},
	} {
		t.Run(bad.Type+"/"+bad.Access, func(t *testing.T) {
			f := NewForums(nil, nil, zerolog.Nop())
			n, stopped := f.Restore(
				[]ForumRow{
					{ID: 1, Type: "MEMO", Access: "ALL", OwnerID: 10},
					bad,
					{ID: 3, Type: "CLAN_ANN", Access: "READ", OwnerID: 30},
				},
				[]Topic{{ID: 1, ForumID: 1, Name: "kept forum's topic"}},
				[]Post{{ID: 0, TopicID: 1, ForumID: 1, Text: "text"}},
			)
			if n != 1 || !stopped {
				t.Fatalf("Restore = %d forums, stopped %t; want 1, true", n, stopped)
			}
			got := restored(f, 1)
			want := Forum{ID: 1, Type: ForumMemo, Access: AccessAll, OwnerID: 10}
			if !got.present || got.forum != want || len(got.topics) != 0 || got.last != 0 {
				t.Fatalf("forum 1 = %+v, want %+v with no topics", got, want)
			}
			for _, id := range []int32{2, 3} {
				if restored(f, id).present {
					t.Fatalf("forum %d restored after the stop", id)
				}
			}
			// The ids continue from the forums loaded, not the stored ones.
			if memo := f.Memo(30); memo.ID != 2 {
				t.Fatalf("next forum id = %d, want 2 (after the one forum loaded)", memo.ID)
			}
		})
	}
}

// TestForumsRestoreStopsAtOrphanTopic pins the boot load of the topics:
// the first topic, in descending id order, whose forum is not loaded
// stops the load there. The topics before it are kept, the topics after
// it are not, and no post is loaded, since the post load never runs.
func TestForumsRestoreStopsAtOrphanTopic(t *testing.T) {
	f := NewForums(nil, nil, zerolog.Nop())
	n, stopped := f.Restore(
		[]ForumRow{
			{ID: 1, Type: "MEMO", Access: "ALL", OwnerID: 10},
			{ID: 2, Type: "CLAN_CBB", Access: "READ", OwnerID: 20},
		},
		[]Topic{
			{ID: 7, ForumID: 1, Name: "kept"},
			{ID: 6, ForumID: 2, Name: "kept too"},
			{ID: 5, ForumID: 9, Name: "orphan"},
			{ID: 4, ForumID: 1, Name: "after the stop"},
			{ID: 3, ForumID: 2, Name: "after the stop"},
		},
		[]Post{
			{ID: 0, TopicID: 7, ForumID: 1, Text: "text"},
			{ID: 0, TopicID: 6, ForumID: 2, Text: "text"},
		},
	)
	if n != 2 || !stopped {
		t.Fatalf("Restore = %d forums, stopped %t; want 2, true", n, stopped)
	}
	for id, want := range map[int32]struct {
		topic int32
	}{1: {7}, 2: {6}} {
		got := restored(f, id)
		if !got.present || len(got.topics) != 1 || got.topics[0] != want.topic || got.last != want.topic || got.posts[want.topic] != 0 {
			t.Fatalf("forum %d = %+v, want only topic %d, holding no post", id, got, want.topic)
		}
	}
	if restored(f, 9).present {
		t.Fatal("the orphan topic's forum 9 exists")
	}
	if _, _, _, found := f.TopicPost(1, 7, 0); found != FoundTopic {
		t.Fatalf("TopicPost(1, 7, 0) found %d, want the topic without its post (%d)", found, FoundTopic)
	}
}

// TestForumsRestoreLoadsAll is the load without a stop: every forum, topic
// and post, and a forum's next topic id after its highest stored one.
func TestForumsRestoreLoadsAll(t *testing.T) {
	f := NewForums(nil, nil, zerolog.Nop())
	n, stopped := f.Restore(
		[]ForumRow{{ID: 4, Type: "MEMO", Access: "ALL", OwnerID: 10}},
		[]Topic{{ID: 9, ForumID: 4}, {ID: 2, ForumID: 4}},
		[]Post{{ID: 0, TopicID: 2, ForumID: 4}, {ID: 0, TopicID: 9, ForumID: 4}, {ID: 1, TopicID: 9, ForumID: 4}},
	)
	if n != 1 || stopped {
		t.Fatalf("Restore = %d forums, stopped %t; want 1, false", n, stopped)
	}
	got := restored(f, 4)
	if len(got.topics) != 2 || got.topics[0] != 9 || got.topics[1] != 2 || got.posts[9] != 2 || got.posts[2] != 1 || got.last != 9 {
		t.Fatalf("forum 4 = %+v, want topics 9 (2 posts) and 2 (1 post), last 9", got)
	}
}
