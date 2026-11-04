package clan

import (
	"context"

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
// is kept in: the order a clan mail addresses the members in and a roster
// packet lists them in.
func (cl *Clan) MembersInTableOrder() []Member {
	cl.mu.RLock()
	defer cl.mu.RUnlock()
	ids := cl.memberOrder.Order()
	out := make([]Member, 0, len(ids))
	for _, id := range ids {
		out = append(out, *cl.members[id])
	}
	return out
}

// Clans returns every clan in registry order: by bucket of the hash table
// the registry is kept in (an id folded onto its own high half, masked to
// the table size), lowest bucket first, and within a bucket in the order
// the clans came in, as reordered by the table's doublings. The table
// never shrinks after a clan leaves it.
func (t *Table) Clans() []*Clan {
	t.mu.RLock()
	defer t.mu.RUnlock()
	ids := t.order.Order()
	out := make([]*Clan, 0, len(ids))
	for _, id := range ids {
		out = append(out, t.clans[id])
	}
	return out
}
