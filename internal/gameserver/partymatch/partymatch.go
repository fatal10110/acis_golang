// Package partymatch owns party-matching: the rooms players open to gather
// a party, and the waiting list of players browsing them. Like parties,
// rooms hold no persistent state; a room lives only while its members are
// in the world.
package partymatch

import (
	"slices"
	"sync"
)

// Member is a player as party matching sees it.
type Member interface {
	ObjectID() int32
	Level() int
	CharacterName() string
	// Departed reports whether the player has begun leaving the world. It
	// must turn true before the player's own Leave, so that an operation
	// adding the player to a room or to the waiting list, under the
	// registry lock, either runs before that Leave (which then takes the
	// player out again) or sees the player gone.
	Departed() bool
	// SetPartyRoom records the room the player is in, 0 for none, for the
	// views other players see of it. It is called under the registry lock.
	SetPartyRoom(id int32)
}

// Location values of room listings and of a room's own location.
const (
	// NearMe lists the rooms whose leader stands in the asker's world
	// region.
	NearMe int32 = -2
	// AnyLocation lists rooms wherever they are.
	AnyLocation int32 = -1
	// NoLocation is the location of a room whose leader stands in no
	// restart region.
	NoLocation int32 = 100
)

// LevelsMine lists only the rooms whose level range holds the asker's
// level; any other level mode lists every room.
const LevelsMine int32 = 0

// Settings are a room's terms, as its leader sets them.
type Settings struct {
	MaxMembers int32
	MinLevel   int32
	MaxLevel   int32
	Loot       int32
	Title      string
}

// Room is a copy of one room. Its leader is Members[0].
type Room[M Member] struct {
	ID       int32
	Settings Settings
	// Location is the region number of the leader's position when the
	// room was opened or last revised.
	Location int32
	Members  []M
}

// Leader returns the room's leader.
func (r Room[M]) Leader() M { return r.Members[0] }

// Registry owns every room and the waiting list. mu guards all of it,
// including every room reachable from it.
type Registry[M Member] struct {
	mu     sync.Mutex
	nextID int32
	rooms  map[int32]*room[M]
	// byMember is the room each player is in.
	byMember map[int32]*room[M]
	// waiting is the waiting list, in the order players joined it.
	waiting []M
}

type room[M Member] struct {
	id       int32
	settings Settings
	location int32
	// members is in join order but for the leader, always first. It is
	// replaced, never changed in place, so returned copies may share it.
	members []M
}

// NewRegistry returns an empty registry.
func NewRegistry[M Member]() *Registry[M] {
	return &Registry[M]{
		rooms:    make(map[int32]*room[M]),
		byMember: make(map[int32]*room[M]),
	}
}

// RoomOf returns a copy of the room the player is in.
func (r *Registry[M]) RoomOf(memberID int32) (Room[M], bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rm := r.byMember[memberID]
	if rm == nil {
		return Room[M]{}, false
	}
	return rm.view(), true
}

// AddWaiting puts player on the waiting list. A player that has begun
// leaving the world, or is in a room, is not listed.
func (r *Registry[M]) AddWaiting(player M) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.addWaiting(player)
}

// RemoveWaiting takes player off the waiting list.
func (r *Registry[M]) RemoveWaiting(player M) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.removeWaiting(player)
}

// Waiting returns the listed players other than player whose level lies
// in [minLevel, maxLevel], in the order they joined the list.
func (r *Registry[M]) Waiting(player M, minLevel, maxLevel int32) []M {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []M
	for _, m := range r.waiting {
		if m.ObjectID() == player.ObjectID() {
			continue
		}
		if lvl := int32(m.Level()); lvl < minLevel || lvl > maxLevel {
			continue
		}
		out = append(out, m)
	}
	return out
}

// Rooms returns the rooms player may browse with the given location and
// level mode, by ascending id: none that is full; with NearMe only those
// whose leader near reports in player's world region, with AnyLocation all
// of them, otherwise only those at location; with LevelsMine only those
// whose level range holds player's level.
func (r *Registry[M]) Rooms(player M, location, levelMode int32, near func(leader M) bool) []Room[M] {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.available(player, location, levelMode, near)
}

func (r *Registry[M]) available(player M, location, levelMode int32, near func(leader M) bool) []Room[M] {
	ids := make([]int32, 0, len(r.rooms))
	for id := range r.rooms {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	var out []Room[M]
	for _, id := range ids {
		rm := r.rooms[id]
		if rm.full() {
			continue
		}
		switch location {
		case NearMe:
			if !near(rm.leader()) {
				continue
			}
		case AnyLocation:
		default:
			if rm.location != location {
				continue
			}
		}
		if levelMode == LevelsMine && !rm.levelFits(player) {
			continue
		}
		out = append(out, rm.view())
	}
	return out
}

// Open opens a room led by leader, which must be on the waiting list; it
// leaves the list. Each of partyMembers but leader enters the room with
// it, unless it is in a room already or has begun leaving the world. A
// leader not on the list opens nothing and hears nothing.
func (r *Registry[M]) Open(leader M, s Settings, location int32, partyMembers []M) []Notice {
	r.mu.Lock()
	defer r.mu.Unlock()
	if leader.Departed() || !r.removeWaiting(leader) {
		return nil
	}
	r.nextID++
	rm := &room[M]{id: r.nextID, settings: s, location: location, members: []M{leader}}
	r.rooms[rm.id] = rm
	r.byMember[leader.ObjectID()] = rm

	var out notices
	for _, m := range partyMembers {
		if m.ObjectID() == leader.ObjectID() || m.Departed() || r.byMember[m.ObjectID()] != nil {
			continue
		}
		r.add(&out, rm, m)
	}
	view := rm.view()
	out.add(Detail[M]{To: leader, Room: view})
	out.add(MemberList[M]{To: leader, Room: view, Mode: ListOpened})
	out.add(Msg[M]{To: []M{leader}, ID: MsgRoomCreated})
	leader.SetPartyRoom(rm.id)
	out.add(InfoRefresh[M]{Member: leader})
	return out
}

// Revise sets the terms of the room roomID that leader leads, and its
// location to location; every member is shown the new terms. No room
// roomID, or one leader does not lead, changes nothing.
func (r *Registry[M]) Revise(leader M, roomID int32, s Settings, location int32) []Notice {
	r.mu.Lock()
	defer r.mu.Unlock()
	rm := r.rooms[roomID]
	if rm == nil || rm.leader().ObjectID() != leader.ObjectID() {
		return nil
	}
	rm.settings = s
	rm.location = location
	view := rm.view()
	var out notices
	for _, m := range rm.members {
		out.add(Detail[M]{To: m, Room: view})
		out.add(MemberList[M]{To: m, Room: view, Mode: ListRevised})
		out.add(Msg[M]{To: []M{m}, ID: MsgRoomRevised})
	}
	return out
}

// JoinStatus is the outcome of Join and Answer.
type JoinStatus uint8

// Join statuses.
const (
	Joined JoinStatus = iota
	// JoinRefused: no such room, or player's level or the room's size
	// keeps it out.
	JoinRefused
	// JoinNotWaiting: player is not on the waiting list; nothing happens.
	JoinNotWaiting
	// JoinNoRoom: the inviter is in no room; nothing happens.
	JoinNoRoom
)

// Join enters player into the room roomID, or with roomID 0 into the
// first room Rooms would list for location and levelMode. player must be
// on the waiting list; it leaves the list.
func (r *Registry[M]) Join(player M, roomID, location, levelMode int32, near func(leader M) bool) ([]Notice, JoinStatus) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var rm *room[M]
	if roomID > 0 {
		rm = r.rooms[roomID]
	} else if rooms := r.available(player, location, levelMode, near); len(rooms) > 0 {
		rm = r.rooms[rooms[0].ID]
	}
	return r.join(rm, player)
}

// Answer enters player into the room inviter is in, as Join.
func (r *Registry[M]) Answer(player, inviter M) ([]Notice, JoinStatus) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rm := r.byMember[inviter.ObjectID()]
	if rm == nil {
		return nil, JoinNoRoom
	}
	return r.join(rm, player)
}

func (r *Registry[M]) join(rm *room[M], player M) ([]Notice, JoinStatus) {
	if rm == nil || !rm.admits(player) {
		return nil, JoinRefused
	}
	if player.Departed() || !r.removeWaiting(player) {
		return nil, JoinNotWaiting
	}
	var out notices
	view := rm.view()
	out.add(Detail[M]{To: player, Room: view})
	out.add(MemberList[M]{To: player, Room: view, Mode: ListEntered})
	for _, m := range rm.members {
		out.add(MemberChange[M]{To: []M{m}, Member: player, Leader: rm.leader(), Mode: ChangeAdded})
		out.add(Msg[M]{To: []M{m}, ID: MsgEnteredRoom, Name: player.CharacterName()})
	}
	r.add(&out, rm, player)
	return out, Joined
}

// OustStatus is the outcome of Oust.
type OustStatus uint8

// Oust statuses.
const (
	Ousted OustStatus = iota
	// OustNotLeader: target is in no room, or leader does not lead it;
	// nothing happens.
	OustNotLeader
	// OustPartyMember: the leader and target share a party.
	OustPartyMember
)

// Oust takes target out of the room leader leads and puts it back on the
// waiting list. sameParty reports whether two players share a party; it is
// called under the registry lock. The returned notices end with the room
// list target now sees.
func (r *Registry[M]) Oust(leader, target M, sameParty func(a, b M) bool) ([]Notice, OustStatus) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rm := r.byMember[target.ObjectID()]
	if rm == nil || rm.leader().ObjectID() != leader.ObjectID() {
		return nil, OustNotLeader
	}
	if sameParty(leader, target) {
		return nil, OustPartyMember
	}
	var out notices
	r.remove(&out, rm, target)
	r.addWaiting(target)
	out.add(RoomList[M]{To: target, Rooms: r.available(target, ousterLocation, ousterLevelMode, nil)})
	return out, Ousted
}

// The room list an ousted player is shown: the rooms at location 1, of
// every level range.
const (
	ousterLocation  int32 = 1
	ousterLevelMode int32 = 1
)

// Dismiss disbands the room roomID that leader leads. Anyone else
// disbands nothing.
func (r *Registry[M]) Dismiss(leader M, roomID int32) []Notice {
	r.mu.Lock()
	defer r.mu.Unlock()
	rm := r.rooms[roomID]
	if rm == nil || rm.leader().ObjectID() != leader.ObjectID() {
		return nil
	}
	var out notices
	r.disband(&out, rm)
	return out
}

// WithdrawStatus is the outcome of Withdraw.
type WithdrawStatus uint8

// Withdraw statuses.
const (
	Withdrew WithdrawStatus = iota
	// WithdrawNoRoom: no room roomID exists; nothing happens.
	WithdrawNoRoom
	// WithdrawPartyOfLeader: player shares a party with the room's leader
	// (or leads it while in a party), and stays.
	WithdrawPartyOfLeader
)

// Withdraw takes player out of the room roomID. sameParty is as for Oust.
// A player not in that room leaves nothing, yet still withdraws.
func (r *Registry[M]) Withdraw(player M, roomID int32, sameParty func(a, b M) bool) ([]Notice, WithdrawStatus) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rm := r.rooms[roomID]
	if rm == nil {
		return nil, WithdrawNoRoom
	}
	if sameParty(player, rm.leader()) {
		return nil, WithdrawPartyOfLeader
	}
	var out notices
	r.remove(&out, rm, player)
	return out, Withdrew
}

// Leave takes a player leaving the world off the waiting list and out of
// its room.
func (r *Registry[M]) Leave(player M) []Notice {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.removeWaiting(player)
	rm := r.byMember[player.ObjectID()]
	if rm == nil {
		return nil
	}
	var out notices
	r.remove(&out, rm, player)
	return out
}

// LeaveWithParty takes player, who just left its party, out of its room:
// it is shown the room's terms and members first.
func (r *Registry[M]) LeaveWithParty(player M) []Notice {
	r.mu.Lock()
	defer r.mu.Unlock()
	rm := r.byMember[player.ObjectID()]
	if rm == nil {
		return nil
	}
	var out notices
	view := rm.view()
	out.add(Detail[M]{To: player, Room: view})
	out.add(MemberList[M]{To: player, Room: view, Mode: ListEntered})
	r.remove(&out, rm, player)
	return out
}

// PartyJoined follows player joining inviter's party: when inviter is in a
// room, player enters it unless it is in a room already, and every member
// of inviter's room is shown player's row again. A player in another room
// changes nothing.
func (r *Registry[M]) PartyJoined(inviter, player M) []Notice {
	r.mu.Lock()
	defer r.mu.Unlock()
	rm := r.byMember[inviter.ObjectID()]
	if rm == nil {
		return nil
	}
	var out notices
	switch own := r.byMember[player.ObjectID()]; {
	case own == rm:
	case own != nil || player.Departed():
		return nil
	default:
		r.removeWaiting(player)
		r.add(&out, rm, player)
	}
	out.add(MemberChange[M]{To: rm.members, Member: player, Leader: rm.leader(), Mode: ChangeUpdated})
	return out
}

// PartyLeaderChanged follows leader becoming its party's leader: when it is
// in a room, it leads the room too.
func (r *Registry[M]) PartyLeaderChanged(leader M) []Notice {
	r.mu.Lock()
	defer r.mu.Unlock()
	rm := r.byMember[leader.ObjectID()]
	if rm == nil {
		return nil
	}
	var out notices
	rm.changeLeader(&out, leader)
	return out
}

// add puts player into rm.
func (r *Registry[M]) add(out *notices, rm *room[M], player M) {
	rm.members = append(slices.Clip(rm.members), player)
	r.byMember[player.ObjectID()] = rm
	player.SetPartyRoom(rm.id)
	out.add(InfoRefresh[M]{Member: player})
}

// remove takes player out of rm: a leader alone disbands it, a leader with
// company hands it to the first other member first.
func (r *Registry[M]) remove(out *notices, rm *room[M], player M) {
	if !rm.contains(player) {
		return
	}
	if rm.leader().ObjectID() == player.ObjectID() {
		if len(rm.members) == 1 {
			r.disband(out, rm)
			return
		}
		rm.changeLeader(out, rm.members[1])
	}
	rm.members = without(rm.members, player.ObjectID())
	delete(r.byMember, player.ObjectID())
	for _, m := range rm.members {
		out.add(Msg[M]{To: []M{m}, ID: MsgLeftRoom, Name: player.CharacterName()})
		out.add(MemberChange[M]{To: []M{m}, Member: player, Leader: rm.leader(), Mode: ChangeRemoved})
	}
	out.add(Close[M]{To: player})
	player.SetPartyRoom(0)
	out.add(InfoRefresh[M]{Member: player})
}

func (r *Registry[M]) disband(out *notices, rm *room[M]) {
	delete(r.rooms, rm.id)
	for _, m := range rm.members {
		out.add(Close[M]{To: m})
		out.add(Msg[M]{To: []M{m}, ID: MsgRoomDisbanded})
		delete(r.byMember, m.ObjectID())
		m.SetPartyRoom(0)
		out.add(InfoRefresh[M]{Member: m})
	}
	rm.members = nil
}

func (r *Registry[M]) addWaiting(player M) {
	if player.Departed() || r.byMember[player.ObjectID()] != nil {
		return
	}
	for _, m := range r.waiting {
		if m.ObjectID() == player.ObjectID() {
			return
		}
	}
	r.waiting = append(r.waiting, player)
}

// removeWaiting reports whether player was on the waiting list.
func (r *Registry[M]) removeWaiting(player M) bool {
	i := slices.IndexFunc(r.waiting, func(m M) bool { return m.ObjectID() == player.ObjectID() })
	if i < 0 {
		return false
	}
	r.waiting = slices.Delete(r.waiting, i, i+1)
	return true
}

// changeLeader hands rm to player, a member other than its leader; the
// two swap places in the member order.
func (rm *room[M]) changeLeader(out *notices, player M) {
	i := slices.IndexFunc(rm.members, func(m M) bool { return m.ObjectID() == player.ObjectID() })
	if i <= 0 {
		return
	}
	old := rm.members[0]
	members := slices.Clone(rm.members)
	members[0], members[i] = members[i], members[0]
	rm.members = members
	for _, m := range rm.members {
		out.add(MemberChange[M]{To: []M{m}, Member: player, Leader: player, Mode: ChangeUpdated})
		out.add(MemberChange[M]{To: []M{m}, Member: old, Leader: player, Mode: ChangeUpdated})
		out.add(Msg[M]{To: []M{m}, ID: MsgRoomLeaderChanged})
	}
}

func (rm *room[M]) leader() M { return rm.members[0] }

func (rm *room[M]) full() bool { return int32(len(rm.members)) >= rm.settings.MaxMembers }

func (rm *room[M]) levelFits(player M) bool {
	lvl := int32(player.Level())
	return lvl >= rm.settings.MinLevel && lvl <= rm.settings.MaxLevel
}

func (rm *room[M]) admits(player M) bool { return rm.levelFits(player) && !rm.full() }

func (rm *room[M]) contains(player M) bool {
	return slices.ContainsFunc(rm.members, func(m M) bool { return m.ObjectID() == player.ObjectID() })
}

func (rm *room[M]) view() Room[M] {
	return Room[M]{ID: rm.id, Settings: rm.settings, Location: rm.location, Members: rm.members}
}

func without[M Member](members []M, id int32) []M {
	out := make([]M, 0, len(members))
	for _, m := range members {
		if m.ObjectID() != id {
			out = append(out, m)
		}
	}
	return out
}
