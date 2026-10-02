package clan

import (
	"context"
	"sort"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
)

// maxWars is how many clans one clan may be at war with.
const maxWars = 30

// minWarLevel is the clan level a clan needs to declare war, or to have
// war declared on it.
const minWarLevel = 3

// WarRow is one clan_wars row: ClanID's war on TargetID, or, with a
// positive Expiry, the epoch millisecond until which ClanID may not declare
// it again.
type WarRow struct {
	ClanID   int32
	TargetID int32
	Expiry   int64
}

// restoreWars files the stored wars as of nowMs; t.mu is held. A row with
// an expiry is a penalty, kept while it lies ahead; any other row is a war.
// A row naming a clan that no longer exists is skipped.
func (t *Table) restoreWars(rows []WarRow, nowMs int64) {
	for _, r := range rows {
		cl, ok := t.clans[r.ClanID]
		if !ok {
			continue
		}
		if r.Expiry > 0 {
			if r.Expiry > nowMs {
				cl.warPenalties[r.TargetID] = r.Expiry
			}
			continue
		}
		target, ok := t.clans[r.TargetID]
		if !ok {
			continue
		}
		cl.wars[r.TargetID] = struct{}{}
		target.attackers[r.ClanID] = struct{}{}
	}
}

// AtWarWith reports whether the clan has declared war on clanID.
func (cl *Clan) AtWarWith(clanID int32) bool {
	cl.mu.RLock()
	defer cl.mu.RUnlock()
	_, ok := cl.wars[clanID]
	return ok
}

// WarList returns the clans this clan declared war on, in the order the
// client is sent them.
func (cl *Clan) WarList() []int32 {
	cl.mu.RLock()
	defer cl.mu.RUnlock()
	return hashSetOrder(cl.wars)
}

// AttackerList returns the clans that declared war on this clan, in the
// order the client is sent them.
func (cl *Clan) AttackerList() []int32 {
	cl.mu.RLock()
	defer cl.mu.RUnlock()
	return hashSetOrder(cl.attackers)
}

// hashSetOrder lists ids in the order the reference's war sets hold them
// right after boot: hash-bucket order of a table of 16 buckets doubled
// whenever it is three quarters full, a bucket being the low bits of the
// id folded with its high half, and ids sharing a bucket in ascending
// order. It drifts from the reference later in two ways, both cosmetic
// (only the order of clans in the war window): the reference table never
// shrinks, so a clan that once held 12 or more wars keeps its larger
// table after they end, where this recomputes the size from the current
// count; and inside a bucket the reference keeps insertion order, which
// is ascending only for the rows restored at boot, while a war declared
// later sorts here by id rather than after its bucket-mates.
func hashSetOrder(ids map[int32]struct{}) []int32 {
	buckets := uint32(16)
	for len(ids) >= int(buckets-buckets/4) {
		buckets *= 2
	}
	bucket := func(id int32) uint32 {
		h := uint32(id)
		return ((h ^ (h >> 16)) & 0x7fffffff) & (buckets - 1)
	}
	out := make([]int32, 0, len(ids))
	for id := range ids {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool {
		bi, bj := bucket(out[i]), bucket(out[j])
		if bi != bj {
			return bi < bj
		}
		return out[i] < out[j]
	})
	return out
}

// lockPair write-locks a and b, in id order so two clans acting on each
// other never deadlock, and returns the unlock.
func lockPair(a, b *Clan) func() {
	if a == b {
		a.mu.Lock()
		return a.mu.Unlock
	}
	first, second := a, b
	if second.id < first.id {
		first, second = second, first
	}
	first.mu.Lock()
	second.mu.Lock()
	return func() {
		second.mu.Unlock()
		first.mu.Unlock()
	}
}

// WarResult is the outcome of a war request.
type WarResult int

// The war outcomes, in the order each request checks them. WarIgnored is
// not answered.
const (
	WarDone WarResult = iota
	WarIgnored
	WarNotAuthorized
	WarNoSuchClan
	WarOwnClan
	WarTooMany
	// WarTooWeak refuses a clan below level 3 or the configured member
	// count.
	WarTooWeak
	// WarTargetTooWeak refuses a target below level 3 or the configured
	// member count, unless the target already declared war on the clan.
	WarTargetTooWeak
	WarAllied
	WarTargetDissolving
	WarAlreadyDeclared
	// WarPenalty refuses a war the clan stopped too recently.
	WarPenalty
	// WarNotInvolved refuses ending a war the clan did not declare.
	WarNotInvolved
	// WarMemberInCombat refuses stopping a war while a member online is in
	// combat.
	WarMemberInCombat
	// WarSurrenderFailed refuses a personal surrender of a war the clan
	// did not declare, or from a member that already wants peace.
	WarSurrenderFailed
)

// War is a war request's two clans: Clan the requester's, Target the
// other.
type War struct {
	Clan   *Clan
	Target *Clan
}

// DeclareWar has c's clan declare war on the clan named targetName. It
// holds allyMu, so no clan joins the alliance of the other between the
// alliance check and the declaration: an acceptance checks the war under
// the same lock.
func (s *Service) DeclareWar(c *player.Character, targetName string, now time.Time) (War, WarResult) {
	cl, ok := s.ClanOf(c)
	if !ok {
		return War{}, WarIgnored
	}
	if !cl.HasPrivilege(c.ID, PrivClanWar) {
		return War{}, WarNotAuthorized
	}
	target, ok := s.table.ByName(targetName)
	switch {
	case !ok:
		return War{}, WarNoSuchClan
	case target == cl:
		return War{}, WarOwnClan
	}
	s.allyMu.Lock()
	defer s.allyMu.Unlock()
	unlock := lockPair(cl, target)
	defer unlock()
	_, attacked := cl.attackers[target.id]
	_, atWar := cl.wars[target.id]
	switch {
	case len(cl.wars) >= maxWars:
		return War{}, WarTooMany
	case cl.level < minWarLevel || len(cl.members) < s.cfg.MembersForWar:
		return War{}, WarTooWeak
	case !attacked && (target.level < minWarLevel || len(target.members) < s.cfg.MembersForWar):
		return War{Clan: cl, Target: target}, WarTargetTooWeak
	case cl.allyID == target.allyID && cl.allyID != 0:
		return War{}, WarAllied
	case target.dissolvingExpiry > 0:
		return War{}, WarTargetDissolving
	case atWar:
		return War{}, WarAlreadyDeclared
	case cl.warPenalties[target.id] > now.UnixMilli():
		return War{Clan: cl, Target: target}, WarPenalty
	}
	cl.wars[target.id] = struct{}{}
	target.attackers[cl.id] = struct{}{}
	clanID, targetID := cl.id, target.id
	s.write(clanID, "store clan war", func(ctx context.Context, st Store) error { return st.InsertWar(ctx, clanID, targetID) })
	return War{Clan: cl, Target: target}, WarDone
}

// StopWar has c's clan stop its war on the clan named targetName.
// inCombat reports whether a member online is in combat.
func (s *Service) StopWar(c *player.Character, targetName string, inCombat func(objectID int32) bool, now time.Time) (War, WarResult) {
	cl, ok := s.ClanOf(c)
	if !ok {
		return War{}, WarIgnored
	}
	target, ok := s.table.ByName(targetName)
	switch {
	case !ok:
		return War{}, WarIgnored
	case !cl.HasPrivilege(c.ID, PrivClanWar):
		return War{}, WarNotAuthorized
	case !cl.AtWarWith(target.id):
		return War{}, WarNotInvolved
	}
	for _, id := range cl.OnlineMemberIDs() {
		if inCombat(id) {
			return War{}, WarMemberInCombat
		}
	}
	if !s.EndWar(cl, target, now) {
		return War{}, WarNotInvolved
	}
	return War{Clan: cl, Target: target}, WarDone
}

// CheckSurrender reports whether c's clan may surrender its war on the clan
// named targetName; EndWar then ends it, once the surrender's cost is
// paid.
func (s *Service) CheckSurrender(c *player.Character, targetName string) (War, WarResult) {
	cl, ok := s.ClanOf(c)
	if !ok {
		return War{}, WarIgnored
	}
	if !cl.HasPrivilege(c.ID, PrivClanWar) {
		return War{}, WarNotAuthorized
	}
	target, ok := s.table.ByName(targetName)
	switch {
	case !ok:
		return War{}, WarIgnored
	case !cl.AtWarWith(target.id):
		return War{}, WarNotInvolved
	}
	return War{Clan: cl, Target: target}, WarDone
}

// EndWar ends cl's war on target as of now, reporting whether there was
// one. cl may not declare it again for WarPenaltyDays; the stored row then
// keeps that penalty, else it is deleted. target's own war on cl, if any,
// goes on.
func (s *Service) EndWar(cl, target *Clan, now time.Time) bool {
	unlock := lockPair(cl, target)
	defer unlock()
	return s.endWarLocked(cl, target, now)
}

// endWarLocked is EndWar with both clans' mu held.
func (s *Service) endWarLocked(cl, target *Clan, now time.Time) bool {
	if _, ok := cl.wars[target.id]; !ok {
		return false
	}
	delete(cl.wars, target.id)
	delete(target.attackers, cl.id)
	var expiry int64
	if s.cfg.WarPenaltyDays > 0 {
		expiry = now.UnixMilli() + int64(s.cfg.WarPenaltyDays)*dayMillis
		cl.warPenalties[target.id] = expiry
	}
	clanID, targetID := cl.id, target.id
	s.write(clanID, "end clan war", func(ctx context.Context, st Store) error { return st.EndWar(ctx, clanID, targetID, expiry) })
	return true
}
