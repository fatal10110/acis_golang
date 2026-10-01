package clan

import (
	"sort"
	"strings"
	"time"

	datacache "github.com/fatal10110/acis_golang/internal/gameserver/data/cache"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
)

// The alliance penalty kinds a clan_data row stores beside the penalty's
// expiry.
const (
	// AllyPenaltyClanLeft keeps a clan that left its alliance out of any
	// alliance.
	AllyPenaltyClanLeft = 1
	// AllyPenaltyClanDismissed keeps a clan dismissed from its alliance out
	// of any alliance.
	AllyPenaltyClanDismissed = 2
	// AllyPenaltyDismissClan keeps an alliance leader that dismissed a clan
	// from inviting another.
	AllyPenaltyDismissClan = 3
	// AllyPenaltyDissolveAlly keeps a clan that dissolved its alliance from
	// founding another.
	AllyPenaltyDissolveAlly = 4
)

// minAllyLevel is the clan level an alliance founder needs.
const minAllyLevel = 5

// Allies returns the clans of the alliance allyID, in ascending clan id
// order; none for 0.
func (t *Table) Allies(allyID int32) []*Clan {
	if t == nil || allyID == 0 {
		return nil
	}
	var out []*Clan
	for _, cl := range t.allClans() {
		if cl.AllyID() == allyID {
			out = append(out, cl)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].id < out[j].id })
	return out
}

// AllyID is the id of the clan's alliance, its leading clan's id; 0 when
// it is in none.
func (cl *Clan) AllyID() int32 {
	cl.mu.RLock()
	defer cl.mu.RUnlock()
	return cl.allyID
}

// allyNameTaken reports whether a clan's alliance is named name, ignoring
// case.
func (t *Table) allyNameTaken(name string) bool {
	for _, cl := range t.allClans() {
		if n := cl.Info().AllyName; n != "" && strings.EqualFold(n, name) {
			return true
		}
	}
	return false
}

// setAllyLocked points cl at the alliance allyID named name, with the
// alliance penalty given, and queues its clan_data row; cl.mu is held.
func (s *Service) setAllyLocked(cl *Clan, allyID int32, name string, penaltyExpiry int64, penaltyType int) {
	cl.allyID, cl.allyName = allyID, name
	cl.allyPenaltyExpiry, cl.allyPenaltyType = penaltyExpiry, penaltyType
	s.updateClanLocked(cl)
}

// setAllyCrestLocked points cl's alliance crest at id and queues the
// column; cl.mu is held. The image the clan pointed at is left in files.
func (s *Service) setAllyCrestLocked(cl *Clan, id int32) {
	cl.allyCrestID = id
	s.queueCrestLocked(cl, datacache.AllyCrest, id, 0, nil)
}

// AllyCreateResult is the outcome of founding an alliance.
type AllyCreateResult int

// The founding outcomes, in the order they are checked. A founding while a
// siege involving the clan is in progress is refused once sieges exist
// (#3150).
const (
	AllyCreated AllyCreateResult = iota
	// AllyCreateNotLeader refuses a clanless player or a member that does
	// not lead its clan.
	AllyCreateNotLeader
	AllyCreateAlreadyAllied
	AllyCreateLevelTooLow
	// AllyCreatePenalty refuses a clan that dissolved an alliance less than
	// CreateAllyDaysWhenDissolved ago.
	AllyCreatePenalty
	AllyCreateDissolving
	AllyCreateNameInvalid
	AllyCreateNameLength
	AllyCreateNameTaken
)

// CreateAlly founds the alliance name with c's clan as its leading clan.
func (s *Service) CreateAlly(c *player.Character, name string, now time.Time) AllyCreateResult {
	cl, ok := s.ClanOf(c)
	if !ok || !cl.IsLeader(c.ID) {
		return AllyCreateNotLeader
	}
	s.allyMu.Lock()
	defer s.allyMu.Unlock()
	nowMs := now.UnixMilli()
	info := cl.Info()
	switch {
	case info.AllyID != 0:
		return AllyCreateAlreadyAllied
	case info.Level < minAllyLevel:
		return AllyCreateLevelTooLow
	case info.AllyPenaltyType == AllyPenaltyDissolveAlly && info.AllyPenaltyExpiry > nowMs:
		return AllyCreatePenalty
	case info.DissolvingExpiry > nowMs:
		return AllyCreateDissolving
	case !validName(name):
		return AllyCreateNameInvalid
	case nameLength(name) < minNameLength || nameLength(name) > maxNameLength:
		return AllyCreateNameLength
	case s.table.allyNameTaken(name):
		return AllyCreateNameTaken
	}
	cl.mu.Lock()
	s.setAllyLocked(cl, cl.id, name, 0, 0)
	cl.mu.Unlock()
	return AllyCreated
}

// AllyDissolveResult is the outcome of dissolving an alliance.
type AllyDissolveResult int

// The dissolution outcomes, in the order they are checked. A dissolution
// while any of the alliance's clans takes part in a siege in progress is
// refused once sieges exist (#3150).
const (
	AllyDissolved AllyDissolveResult = iota
	// AllyDissolveNoClan is a clanless player; it is not answered.
	AllyDissolveNoClan
	AllyDissolveNoAlly
	AllyDissolveNotAllyLeader
)

// AllyDissolution is a dissolved alliance.
type AllyDissolution struct {
	// Clans are the clans the alliance held, in ascending id order.
	Clans []*Clan
	// Cleared are the clans the dissolution took an alliance crest from:
	// the alliance's clans and any clan in no alliance still showing one,
	// in ascending id order. Their members show the change.
	Cleared []*Clan
}

// DissolveAlly dissolves the alliance c's clan leads: every other clan
// leaves it without a penalty; the leading clan may not found another for
// CreateAllyDaysWhenDissolved. The alliance crest is removed from files
// and cleared on every clan in no alliance, as the reference clears it by
// the now-zero alliance id; with consistent rows that is only the
// dissolved alliance's clans.
func (s *Service) DissolveAlly(c *player.Character, files *datacache.Crests, now time.Time) (AllyDissolution, AllyDissolveResult) {
	cl, ok := s.ClanOf(c)
	if !ok {
		return AllyDissolution{}, AllyDissolveNoClan
	}
	s.allyMu.Lock()
	defer s.allyMu.Unlock()
	allyID := cl.AllyID()
	switch {
	case allyID == 0:
		return AllyDissolution{}, AllyDissolveNoAlly
	case !cl.IsLeader(c.ID) || cl.id != allyID:
		return AllyDissolution{}, AllyDissolveNotAllyLeader
	}
	out := AllyDissolution{Clans: s.table.Allies(allyID)}
	for _, member := range out.Clans {
		if member == cl {
			continue
		}
		member.mu.Lock()
		s.setAllyCrestLocked(member, 0)
		s.setAllyLocked(member, 0, "", 0, 0)
		member.mu.Unlock()
	}
	cl.mu.Lock()
	replaced := cl.allyCrestID
	cl.allyCrestID = 0
	s.queueCrestLocked(cl, datacache.AllyCrest, 0, replaced, files)
	s.setAllyLocked(cl, 0, "", now.UnixMilli()+int64(s.cfg.CreateAllyDaysWhenDissolved)*dayMillis, AllyPenaltyDissolveAlly)
	cl.mu.Unlock()
	out.Cleared = append(out.Cleared, out.Clans...)
	for _, other := range s.table.allClans() {
		other.mu.Lock()
		if other.allyID == 0 && other.allyCrestID != 0 {
			s.setAllyCrestLocked(other, 0)
			out.Cleared = append(out.Cleared, other)
		}
		other.mu.Unlock()
	}
	sort.Slice(out.Cleared, func(i, j int) bool { return out.Cleared[i].id < out.Cleared[j].id })
	return out, AllyDissolved
}

// AllyJoinRefusal is why an alliance invitation, or its acceptance, is
// refused. Every refusal is answered to the inviter.
type AllyJoinRefusal int

// The invitation refusals, in the order they are checked. A target clan
// registered against the leading clan in a siege is refused once sieges
// exist (#3150).
const (
	AllyJoinAllowed AllyJoinRefusal = iota
	AllyJoinNotAllyLeader
	// AllyJoinInvitePenalty refuses a leading clan that dismissed a clan
	// less than AcceptClanDaysWhenDismissed ago.
	AllyJoinInvitePenalty
	AllyJoinSelf
	AllyJoinTargetNoClan
	AllyJoinTargetNotLeader
	AllyJoinTargetAllied
	// AllyJoinTargetLeftPenalty refuses a clan that left an alliance less
	// than AllyJoinDaysWhenLeft ago.
	AllyJoinTargetLeftPenalty
	// AllyJoinTargetDismissedPenalty refuses a clan dismissed from an
	// alliance less than AllyJoinDaysWhenDismissed ago.
	AllyJoinTargetDismissedPenalty
	AllyJoinAtWar
	AllyJoinFull
)

// AllyJoinCheck is the outcome of an alliance invitation check: the
// refusal and the target's clan the refusal names.
type AllyJoinCheck struct {
	Refusal AllyJoinRefusal
	// Target is the invited player's clan, nil when it has none.
	Target *Clan
}

// CheckAllyJoin reports whether requesterID may invite target's clan into
// the alliance its own clan leads now.
func (s *Service) CheckAllyJoin(requesterID int32, target *player.Character, now time.Time) AllyJoinCheck {
	s.allyMu.Lock()
	defer s.allyMu.Unlock()
	check, _ := s.checkAllyJoinLocked(requesterID, target, now)
	return check
}

// checkAllyJoinLocked runs the invitation rules under allyMu, which keeps
// every clan's alliance fixed; it also returns the leading clan.
func (s *Service) checkAllyJoinLocked(requesterID int32, target *player.Character, now time.Time) (AllyJoinCheck, *Clan) {
	nowMs := now.UnixMilli()
	leader, ok := s.table.MemberClan(requesterID)
	if !ok {
		return AllyJoinCheck{Refusal: AllyJoinNotAllyLeader}, nil
	}
	info := leader.Info()
	if info.AllyID == 0 || info.LeaderID != requesterID || info.ID != info.AllyID {
		return AllyJoinCheck{Refusal: AllyJoinNotAllyLeader}, nil
	}
	if info.AllyPenaltyType == AllyPenaltyDismissClan && info.AllyPenaltyExpiry > nowMs {
		return AllyJoinCheck{Refusal: AllyJoinInvitePenalty}, nil
	}
	if requesterID == target.ID {
		return AllyJoinCheck{Refusal: AllyJoinSelf}, nil
	}
	targetClan, ok := s.ClanOf(target)
	if !ok {
		return AllyJoinCheck{Refusal: AllyJoinTargetNoClan}, nil
	}
	check := AllyJoinCheck{Target: targetClan}
	ti := targetClan.Info()
	switch {
	case ti.LeaderID != target.ID:
		check.Refusal = AllyJoinTargetNotLeader
	case ti.AllyID != 0:
		check.Refusal = AllyJoinTargetAllied
	case ti.AllyPenaltyExpiry > nowMs && ti.AllyPenaltyType == AllyPenaltyClanLeft:
		check.Refusal = AllyJoinTargetLeftPenalty
	case ti.AllyPenaltyExpiry > nowMs && ti.AllyPenaltyType == AllyPenaltyClanDismissed:
		check.Refusal = AllyJoinTargetDismissedPenalty
	case leader.AtWarWith(targetClan.id):
		check.Refusal = AllyJoinAtWar
	case len(s.table.Allies(info.AllyID)) >= s.cfg.MaxClansInAlly:
		check.Refusal = AllyJoinFull
	}
	return check, leader
}

// JoinAlly puts target's clan into the alliance requesterID's clan leads,
// re-checking the invitation rules first. The clan takes the alliance's
// name and crest and drops any alliance penalty it had.
func (s *Service) JoinAlly(requesterID int32, target *player.Character, now time.Time) AllyJoinCheck {
	s.allyMu.Lock()
	defer s.allyMu.Unlock()
	check, leader := s.checkAllyJoinLocked(requesterID, target, now)
	if check.Refusal != AllyJoinAllowed {
		return check
	}
	info := leader.Info()
	joined := check.Target
	joined.mu.Lock()
	s.setAllyCrestLocked(joined, info.AllyCrestID)
	s.setAllyLocked(joined, info.AllyID, info.AllyName, 0, 0)
	joined.mu.Unlock()
	return check
}

// AllyLeaveResult is the outcome of a clan leaving its alliance.
type AllyLeaveResult int

// The withdrawal outcomes, in the order they are checked.
const (
	AllyLeft AllyLeaveResult = iota
	AllyLeaveNoClan
	AllyLeaveNotLeader
	AllyLeaveNoAlly
	AllyLeaveIsAllyLeader
)

// LeaveAlly takes c's clan out of its alliance; it may not join one for
// AllyJoinDaysWhenLeft.
func (s *Service) LeaveAlly(c *player.Character, now time.Time) (*Clan, AllyLeaveResult) {
	cl, ok := s.ClanOf(c)
	if !ok {
		return nil, AllyLeaveNoClan
	}
	if !cl.IsLeader(c.ID) {
		return nil, AllyLeaveNotLeader
	}
	s.allyMu.Lock()
	defer s.allyMu.Unlock()
	switch allyID := cl.AllyID(); {
	case allyID == 0:
		return nil, AllyLeaveNoAlly
	case allyID == cl.id:
		return nil, AllyLeaveIsAllyLeader
	}
	cl.mu.Lock()
	s.setAllyCrestLocked(cl, 0)
	s.setAllyLocked(cl, 0, "", now.UnixMilli()+int64(s.cfg.AllyJoinDaysWhenLeft)*dayMillis, AllyPenaltyClanLeft)
	cl.mu.Unlock()
	return cl, AllyLeft
}

// AllyDismissResult is the outcome of an alliance leader dismissing a
// clan.
type AllyDismissResult int

// The dismissal outcomes, in the order they are checked.
const (
	AllyDismissed AllyDismissResult = iota
	AllyDismissNoClan
	AllyDismissNoAlly
	AllyDismissNotAllyLeader
	AllyDismissNoSuchClan
	AllyDismissOwnClan
	AllyDismissDifferentAlly
)

// DismissAllyClan has the alliance leader c dismiss the clan named
// clanName, ignoring case. The dismissed clan may not join an alliance for
// AllyJoinDaysWhenDismissed; the leading clan may not invite another for
// AcceptClanDaysWhenDismissed.
func (s *Service) DismissAllyClan(c *player.Character, clanName string, now time.Time) (*Clan, AllyDismissResult) {
	leader, ok := s.ClanOf(c)
	if !ok {
		return nil, AllyDismissNoClan
	}
	s.allyMu.Lock()
	defer s.allyMu.Unlock()
	allyID := leader.AllyID()
	switch {
	case allyID == 0:
		return nil, AllyDismissNoAlly
	case !leader.IsLeader(c.ID) || leader.id != allyID:
		return nil, AllyDismissNotAllyLeader
	}
	target, ok := s.table.ByName(clanName)
	switch {
	case !ok:
		return nil, AllyDismissNoSuchClan
	case target == leader:
		return nil, AllyDismissOwnClan
	case target.AllyID() != allyID:
		return nil, AllyDismissDifferentAlly
	}
	nowMs := now.UnixMilli()
	leader.mu.Lock()
	leader.allyPenaltyExpiry = nowMs + int64(s.cfg.AcceptClanDaysWhenDismissed)*dayMillis
	leader.allyPenaltyType = AllyPenaltyDismissClan
	s.updateClanLocked(leader)
	leader.mu.Unlock()
	target.mu.Lock()
	s.setAllyCrestLocked(target, 0)
	s.setAllyLocked(target, 0, "", nowMs+int64(s.cfg.AllyJoinDaysWhenDismissed)*dayMillis, AllyPenaltyClanDismissed)
	target.mu.Unlock()
	return target, AllyDismissed
}

// SetAllyCrest replaces the crest of the alliance c's clan leads with
// data, or deletes it when data is empty, on every clan of the alliance.
// Only the leader of the leading clan may; anyone else's upload, a delete
// of a crest that is not set, and an image not stored are ignored. The new
// image is stored in files under a fresh id before the clans point at it;
// the replaced image is removed from files once the leading clan's column
// write has landed. It returns the alliance's clans.
func (s *Service) SetAllyCrest(c *player.Character, data []byte, files *datacache.Crests) ([]*Clan, CrestResult) {
	cl, ok := s.ClanOf(c)
	if !ok {
		return nil, CrestIgnored
	}
	s.allyMu.Lock()
	defer s.allyMu.Unlock()
	allyID := cl.AllyID()
	if allyID == 0 || cl.id != allyID || !cl.IsLeader(c.ID) {
		return nil, CrestIgnored
	}
	result := CrestRegistered
	var id int32
	if len(data) == 0 {
		if cl.Info().AllyCrestID == 0 {
			return nil, CrestIgnored
		}
		result = CrestDeleted
	} else {
		var ok bool
		if id, ok = s.newCrestID(datacache.AllyCrest, files); !ok {
			return nil, CrestIgnored
		}
		if err := files.Save(datacache.AllyCrest, int(id), data); err != nil {
			s.log.Warn().Err(err).Int32("clan_id", cl.id).Msg("clan: alliance crest not saved")
			return nil, CrestIgnored
		}
	}
	allies := s.table.Allies(allyID)
	for _, member := range allies {
		member.mu.Lock()
		if member == cl {
			replaced := cl.allyCrestID
			cl.allyCrestID = id
			s.queueCrestLocked(cl, datacache.AllyCrest, id, replaced, files)
		} else {
			s.setAllyCrestLocked(member, id)
		}
		member.mu.Unlock()
	}
	return allies, result
}

// AllyInfo is an alliance as its information window shows it.
type AllyInfo struct {
	Name       string
	LeaderClan string
	LeaderName string
	// Total and Online count the members of every clan of the alliance.
	Total, Online int
	Clans         []AllyClanInfo
}

// AllyClanInfo is one clan of an alliance's information window.
type AllyClanInfo struct {
	Name       string
	Level      int
	LeaderName string
	Total      int
	Online     int
}

// AllianceInfo returns the alliance allyID's information, its clans in
// ascending id order; false when no clan leads it.
func (t *Table) AllianceInfo(allyID int32) (AllyInfo, bool) {
	leader, ok := t.Get(allyID)
	if !ok {
		return AllyInfo{}, false
	}
	li := leader.Info()
	out := AllyInfo{Name: li.AllyName, LeaderClan: li.Name, LeaderName: li.LeaderName}
	for _, cl := range t.Allies(allyID) {
		info := cl.Info()
		entry := AllyClanInfo{
			Name: info.Name, Level: info.Level, LeaderName: info.LeaderName,
			Total: cl.MembersCount(), Online: len(cl.OnlineMemberIDs()),
		}
		out.Total += entry.Total
		out.Online += entry.Online
		out.Clans = append(out.Clans, entry)
	}
	return out, true
}

// DropDanglingAlliances takes out of its alliance every clan whose
// alliance's leading clan no longer exists, as the clans are restored at
// boot before any player connects. Its alliance name and crest are cleared
// and its row written; the crest image stays.
func (s *Service) DropDanglingAlliances() {
	for _, cl := range s.table.allClans() {
		allyID := cl.AllyID()
		if allyID == 0 || allyID == cl.id {
			continue
		}
		if _, ok := s.table.Get(allyID); ok {
			continue
		}
		cl.mu.Lock()
		s.log.Warn().Int32("clan_id", cl.id).Int32("ally_id", allyID).Msg("clan: removing non-existent alliance")
		s.setAllyCrestLocked(cl, 0)
		s.setAllyLocked(cl, 0, "", cl.allyPenaltyExpiry, cl.allyPenaltyType)
		cl.mu.Unlock()
	}
}
