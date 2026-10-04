package bbs

import (
	"cmp"
	"context"
	"slices"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

// ForumType is what a forum is for.
type ForumType int

// The forum types, as bbs_forum.type names them.
const (
	ForumRoot ForumType = iota
	ForumNormal
	ForumClanAnnouncements
	ForumClanBulletin
	ForumMemo
	ForumMail
)

var forumTypeNames = [...]string{
	ForumRoot:              "ROOT",
	ForumNormal:            "NORMAL",
	ForumClanAnnouncements: "CLAN_ANN",
	ForumClanBulletin:      "CLAN_CBB",
	ForumMemo:              "MEMO",
	ForumMail:              "MAIL",
}

// String is the type as bbs_forum.type stores it.
func (t ForumType) String() string { return forumTypeNames[t] }

// ParseForumType reads a stored type; the name must match exactly.
func ParseForumType(name string) (ForumType, bool) {
	for t, n := range forumTypeNames {
		if n == name {
			return ForumType(t), true
		}
	}
	return 0, false
}

// ForumAccess is who may use a forum.
type ForumAccess int

// The forum accesses, as bbs_forum.access names them.
const (
	AccessNone ForumAccess = iota
	AccessRead
	AccessWrite
	AccessAll
)

var forumAccesses = [...]struct{ name, description string }{
	AccessNone:  {"NONE", "No access"},
	AccessRead:  {"READ", "Read access"},
	AccessWrite: {"WRITE", "Write access"},
	AccessAll:   {"ALL", "All access"},
}

// String is the access as bbs_forum.access stores it.
func (a ForumAccess) String() string { return forumAccesses[a].name }

// Description is the access as the clan management page shows it.
func (a ForumAccess) Description() string { return forumAccesses[a].description }

// ParseForumAccess reads a stored access; the name must match exactly.
func ParseForumAccess(name string) (ForumAccess, bool) {
	for a, f := range forumAccesses {
		if f.name == name {
			return ForumAccess(a), true
		}
	}
	return 0, false
}

// Forum is a forum: its identity, without its topics.
type Forum struct {
	ID      int32
	Type    ForumType
	Access  ForumAccess
	OwnerID int32
}

// Topic is a topic of a forum. Date is its creation time in Unix
// milliseconds, as stored.
type Topic struct {
	ID        int32
	ForumID   int32
	Name      string
	Date      int64
	OwnerName string
	OwnerID   int32
}

// Post is a post of a topic. A new topic's text is its post 0; Date is in
// Unix milliseconds, as stored.
type Post struct {
	ID        int32
	OwnerName string
	OwnerID   int32
	Date      int64
	TopicID   int32
	ForumID   int32
	Text      string
}

// ForumRow is a bbs_forum row as stored: its type and access are read
// when the forums are restored.
type ForumRow struct {
	ID      int32
	Type    string
	Access  string
	OwnerID int32
}

// ForumStore writes the bbs_forum, bbs_topic and bbs_post rows.
type ForumStore interface {
	InsertForum(ctx context.Context, f Forum) error
	// InsertTopic stores a new topic, then its first post.
	InsertTopic(ctx context.Context, t Topic, first Post) error
	// DeleteTopic removes a topic, then every post of it.
	DeleteTopic(ctx context.Context, forumID, topicID int32) error
	UpdatePostText(ctx context.Context, p Post) error
}

// forumWriteTimeout bounds one forum write.
const forumWriteTimeout = 2 * time.Second

// forum is a forum with its topics. Its fields are guarded by the
// registry's mu.
type forum struct {
	Forum
	topics map[int32]*topic
	// lastTopicID is the highest topic id the forum has held; a new topic
	// takes the next one, and a deleted topic's id is not given out again
	// until a restart reads the stored ones.
	lastTopicID int32
}

// topic is a topic with its posts, in the order they were added.
type topic struct {
	Topic
	posts []Post
}

func (f *forum) addTopic(t *topic) {
	f.topics[t.ID] = t
	if t.ID > f.lastTopicID {
		f.lastTopicID = t.ID
	}
}

func (t *topic) post(id int32) (*Post, bool) {
	for i := range t.posts {
		if t.posts[i].ID == id {
			return &t.posts[i], true
		}
	}
	return nil, false
}

// Forums is the board's forum registry: every forum with its topics and
// posts. mu guards every forum, topic and post: the players' queues read
// and change them concurrently. Rows are written through writes, one lane
// per forum, so a forum's writes land in the order they were made.
type Forums struct {
	mu     sync.Mutex
	forums map[int32]*forum

	store  ForumStore
	writes Writer
	log    zerolog.Logger
}

// NewForums returns an empty registry writing through store on writes.
func NewForums(store ForumStore, writes Writer, log zerolog.Logger) *Forums {
	return &Forums{forums: map[int32]*forum{}, store: store, writes: writes, log: log}
}

// Restore fills the registry once at boot, before any player connects,
// from the stored forums, the topics in descending id order and the posts
// in ascending id order. Loading stops at the first forum whose type or
// access does not read, at the first topic whose forum is not loaded and
// at the first post whose forum or topic is not loaded: nothing after it
// is restored, the topics and posts too for a forum row, the posts too for
// a topic row. It returns how many forums it restored and whether it
// stopped early.
func (f *Forums) Restore(forums []ForumRow, topics []Topic, posts []Post) (int, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range forums {
		typ, okType := ParseForumType(r.Type)
		access, okAccess := ParseForumAccess(r.Access)
		if !okType || !okAccess {
			return len(f.forums), true
		}
		f.forums[r.ID] = &forum{Forum: Forum{ID: r.ID, Type: typ, Access: access, OwnerID: r.OwnerID}, topics: map[int32]*topic{}}
	}
	for _, t := range topics {
		fo, ok := f.forums[t.ForumID]
		if !ok {
			return len(f.forums), true
		}
		fo.addTopic(&topic{Topic: t})
	}
	for _, p := range posts {
		fo, ok := f.forums[p.ForumID]
		if !ok {
			return len(f.forums), true
		}
		t, ok := fo.topics[p.TopicID]
		if !ok {
			return len(f.forums), true
		}
		t.posts = append(t.posts, p)
	}
	return len(f.forums), false
}

// Forum returns the forum id.
func (f *Forums) Forum(id int32) (Forum, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if fo, ok := f.forums[id]; ok {
		return fo.Forum, true
	}
	return Forum{}, false
}

// Owned returns ownerID's forum of type typ: the lowest id one when
// several are stored.
func (f *Forums) Owned(typ ForumType, ownerID int32) (Forum, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if fo := f.ownedLocked(typ, ownerID); fo != nil {
		return fo.Forum, true
	}
	return Forum{}, false
}

func (f *Forums) ownedLocked(typ ForumType, ownerID int32) *forum {
	var found *forum
	for _, fo := range f.forums {
		if fo.Type == typ && fo.OwnerID == ownerID && (found == nil || fo.ID < found.ID) {
			found = fo
		}
	}
	return found
}

// OwnedOrCreate returns ownerID's forum of type typ, first creating and
// storing it with access when there is none. A new forum takes the id
// after the highest one held.
func (f *Forums) OwnedOrCreate(typ ForumType, access ForumAccess, ownerID int32) Forum {
	f.mu.Lock()
	defer f.mu.Unlock()
	if fo := f.ownedLocked(typ, ownerID); fo != nil {
		return fo.Forum
	}
	var last int32
	for id := range f.forums {
		last = max(last, id)
	}
	fo := &forum{Forum: Forum{ID: last + 1, Type: typ, Access: access, OwnerID: ownerID}, topics: map[int32]*topic{}}
	f.forums[fo.ID] = fo
	created := fo.Forum
	f.write(created.ID, "insert forum", func(ctx context.Context, st ForumStore) error { return st.InsertForum(ctx, created) })
	return created
}

// MemoOf reports whether forum id is playerID's memo forum. A memo forum
// answers its owner alone: the reference serves any player who names its
// id, so anyone could read, rewrite or delete another's memos, and open
// topics in a clan's forums (#3262).
func (f *Forums) MemoOf(id, playerID int32) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	fo, ok := f.forums[id]
	return ok && fo.Type == ForumMemo && fo.OwnerID == playerID
}

// Memo returns ownerID's memo forum, creating it, open to all, on first
// use.
func (f *Forums) Memo(ownerID int32) Forum {
	return f.OwnedOrCreate(ForumMemo, AccessAll, ownerID)
}

// clanForumLevel is the clan level from which a clan has its
// announcement and bulletin forums.
const clanForumLevel = 2

// EnsureClanForums creates clan clanID's announcement forum, then its
// bulletin forum, both read-only, once the clan is at level 2 or more and
// has not got them yet.
func (f *Forums) EnsureClanForums(clanID int32, level int) {
	if level < clanForumLevel {
		return
	}
	f.OwnedOrCreate(ForumClanAnnouncements, AccessRead, clanID)
	f.OwnedOrCreate(ForumClanBulletin, AccessRead, clanID)
}

// ForumView is a forum as a page shows it: the forum, its topics in
// descending id order, and the highest topic id it has held.
type ForumView struct {
	Forum
	Topics      []Topic
	LastTopicID int32
}

// View returns the forum id with its topics.
func (f *Forums) View(id int32) (ForumView, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fo, ok := f.forums[id]
	if !ok {
		return ForumView{}, false
	}
	v := ForumView{Forum: fo.Forum, LastTopicID: fo.lastTopicID, Topics: make([]Topic, 0, len(fo.topics))}
	for _, t := range fo.topics {
		v.Topics = append(v.Topics, t.Topic)
	}
	slices.SortFunc(v.Topics, func(a, b Topic) int { return cmp.Compare(b.ID, a.ID) })
	return v, true
}

// TopicPost returns the topic topicID of forum forumID and its post
// postID; found reports which of the forum, the topic and the post exist.
func (f *Forums) TopicPost(forumID, topicID, postID int32) (Forum, Topic, Post, Found) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fo, ok := f.forums[forumID]
	if !ok {
		return Forum{}, Topic{}, Post{}, FoundNothing
	}
	t, ok := fo.topics[topicID]
	if !ok {
		return fo.Forum, Topic{}, Post{}, FoundForum
	}
	p, ok := t.post(postID)
	if !ok {
		return fo.Forum, t.Topic, Post{}, FoundTopic
	}
	return fo.Forum, t.Topic, *p, FoundPost
}

// Found is how far a lookup of a forum's topic's post got.
type Found int

// The lookup outcomes, each finding what the one before found and more.
const (
	FoundNothing Found = iota
	FoundForum
	FoundTopic
	FoundPost
)

// topicLimit is how many topics a forum holds; a forum holding as many
// takes no new one. The reference has no limit, so one player could grow
// the topic and post tables without bound (#3262).
const topicLimit = 100

// AddTopic opens a topic called name in forum forumID, by the character
// ownerName, ownerID, at now, with text as its post 0, and stores both.
// It reports false when the forum does not exist or already holds
// topicLimit topics.
func (f *Forums) AddTopic(forumID int32, name, ownerName string, ownerID int32, text string, now time.Time) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	fo, ok := f.forums[forumID]
	if !ok || len(fo.topics) >= topicLimit {
		return false
	}
	date := now.UnixMilli()
	t := &topic{Topic: Topic{ID: fo.lastTopicID + 1, ForumID: forumID, Name: name, Date: date, OwnerName: ownerName, OwnerID: ownerID}}
	first := Post{ID: 0, OwnerName: ownerName, OwnerID: ownerID, Date: date, TopicID: t.ID, ForumID: forumID, Text: text}
	t.posts = []Post{first}
	fo.addTopic(t)
	stored := t.Topic
	f.write(forumID, "insert topic", func(ctx context.Context, st ForumStore) error { return st.InsertTopic(ctx, stored, first) })
	return true
}

// DeleteTopic removes the topic topicID of forum forumID with its posts,
// and from the store. found reports which of the forum and the topic
// exist; the topic is deleted only when both do.
func (f *Forums) DeleteTopic(forumID, topicID int32) Found {
	f.mu.Lock()
	defer f.mu.Unlock()
	fo, ok := f.forums[forumID]
	if !ok {
		return FoundNothing
	}
	if _, ok := fo.topics[topicID]; !ok {
		return FoundForum
	}
	delete(fo.topics, topicID)
	f.write(forumID, "delete topic", func(ctx context.Context, st ForumStore) error { return st.DeleteTopic(ctx, forumID, topicID) })
	return FoundTopic
}

// EditPost replaces the text of the post postID of the topic topicID of
// forum forumID, and stores it. found reports which of the forum, the
// topic and the post exist; the text changes only when all three do.
func (f *Forums) EditPost(forumID, topicID, postID int32, text string) Found {
	f.mu.Lock()
	defer f.mu.Unlock()
	fo, ok := f.forums[forumID]
	if !ok {
		return FoundNothing
	}
	t, ok := fo.topics[topicID]
	if !ok {
		return FoundForum
	}
	p, ok := t.post(postID)
	if !ok {
		return FoundTopic
	}
	p.Text = text
	stored := *p
	f.write(forumID, "update post text", func(ctx context.Context, st ForumStore) error { return st.UpdatePostText(ctx, stored) })
	return FoundPost
}

// write runs fn against the store on forumID's lane; f.mu is held, so the
// lane takes a forum's writes in the order they were made.
func (f *Forums) write(forumID int32, what string, fn func(context.Context, ForumStore) error) {
	if f.store == nil {
		return
	}
	store, log := f.store, f.log
	job := func() {
		ctx, cancel := context.WithTimeout(context.Background(), forumWriteTimeout)
		defer cancel()
		if err := fn(ctx, store); err != nil {
			log.Error().Err(err).Int32("forum_id", forumID).Msg("bbs: " + what)
		}
	}
	if f.writes == nil {
		job()
		return
	}
	if !f.writes.Enqueue(forumID, job) {
		log.Error().Int32("forum_id", forumID).Msg("bbs: " + what + ": write dropped")
	}
}
