package clan

import (
	"cmp"
	"context"
	"slices"

	"github.com/fatal10110/acis_golang/internal/commons"
)

// The longest community board texts a clan keeps, in UTF-16 code units.
const (
	noticeLength       = 8192
	introductionLength = 300
)

// board is a clan's community board texts: the notice its members may be
// shown at login and the introduction its home page shows. The clan's mu
// guards it.
type board struct {
	notice        string
	noticeEnabled bool
	introduction  string
}

func (b *board) restore(r Row) {
	b.notice = commons.UTF16Cut(r.Notice, noticeLength)
	b.noticeEnabled = r.NoticeEnabled
	b.introduction = commons.UTF16Cut(r.Introduction, introductionLength)
}

// Notice is the clan's notice and whether it shows at login.
func (cl *Clan) Notice() (string, bool) {
	cl.mu.RLock()
	defer cl.mu.RUnlock()
	return cl.board.notice, cl.board.noticeEnabled
}

// Introduction is the clan's introduction.
func (cl *Clan) Introduction() string {
	cl.mu.RLock()
	defer cl.mu.RUnlock()
	return cl.board.introduction
}

// SetNotice replaces cl's notice text, keeping whether it shows, and
// stores both.
func (s *Service) SetNotice(cl *Clan, notice string) {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	s.setNoticeLocked(cl, notice, cl.board.noticeEnabled)
}

// EnableNotice sets whether cl's notice shows at login, keeping its text,
// and stores both.
func (s *Service) EnableNotice(cl *Clan, enabled bool) {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	s.setNoticeLocked(cl, cl.board.notice, enabled)
}

func (s *Service) setNoticeLocked(cl *Clan, notice string, enabled bool) {
	cl.board.notice = commons.UTF16Cut(notice, noticeLength)
	cl.board.noticeEnabled = enabled
	id, text := cl.id, cl.board.notice
	s.write(id, "update clan notice", func(ctx context.Context, st Store) error {
		return st.UpdateNotice(ctx, id, enabled, text)
	})
}

// SetIntroduction replaces cl's introduction and stores it.
func (s *Service) SetIntroduction(cl *Clan, introduction string) {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	cl.board.introduction = commons.UTF16Cut(introduction, introductionLength)
	id, text := cl.id, cl.board.introduction
	s.write(id, "update clan introduction", func(ctx context.Context, st Store) error {
		return st.UpdateIntroduction(ctx, id, text)
	})
}

// MembersInTableOrder returns the roster in the order of the hash table it
// is kept in, as Clans orders the registry: the order a clan mail
// addresses the members in and a roster packet lists them in. Like Clans,
// it approximates that order.
func (cl *Clan) MembersInTableOrder() []Member {
	members := cl.Members()
	buckets := registryBuckets(len(members))
	slices.SortStableFunc(members, func(a, b Member) int {
		return registryBucket(a.ObjectID, buckets) - registryBucket(b.ObjectID, buckets)
	})
	return members
}

// Clans returns every clan in registry order: by bucket of the hash table
// the registry is kept in (an id folded onto its own high half, masked to
// the table size), lowest bucket first, and by ascending id within a
// bucket. The order is approximate: the table size is recomputed from the
// current count, where the client-facing order comes from a table that
// never shrinks after a clan leaves it, and whose resize can reverse the
// order within a bucket.
func (t *Table) Clans() []*Clan {
	clans := t.allClans()
	buckets := registryBuckets(len(clans))
	slices.SortFunc(clans, func(a, b *Clan) int {
		if ba, bb := registryBucket(a.id, buckets), registryBucket(b.id, buckets); ba != bb {
			return ba - bb
		}
		return cmp.Compare(a.id, b.id)
	})
	return clans
}

// registryBuckets is the size of the registry's hash table holding n
// clans: 16 to start, doubled while n reaches three quarters of it.
func registryBuckets(n int) int {
	buckets := 16
	for n >= buckets-buckets/4 {
		buckets *= 2
	}
	return buckets
}

func registryBucket(id int32, buckets int) int {
	h := uint32(id)
	return int((h ^ h>>16) & uint32(buckets-1))
}
