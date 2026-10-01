// Package party owns player parties and the command channels that join
// them: membership, leadership, loot rule, level, and the ordered client
// notices each change produces. It holds no persistent state; a party lives
// only while its members are in the world.
package party

import (
	"strings"
	"sync"
	"time"
)

// MaxMembers is the largest party.
const MaxMembers = 9

// InviteTimeout is how long a party's outstanding invitation keeps its
// leader from inviting anyone else.
const InviteTimeout = 15 * time.Second

// Member is a player as a party sees it.
type Member interface {
	ObjectID() int32
	Level() int
	CharacterName() string
	// Departed reports whether the player has begun leaving the world.
	// It must turn true before the player's own Leave(Disconnected), so
	// that Answer, under the registry lock, either runs before that Leave
	// (which then takes the player out again) or sees the player gone.
	Departed() bool
}

// LootRule is a party's item distribution rule, as the client numbers it.
type LootRule int32

// Loot rules.
const (
	LootFindersKeepers LootRule = iota
	LootRandom
	LootRandomIncludingSpoil
	LootByTurn
	LootByTurnIncludingSpoil
	lootRuleCount
)

// Reason is why a member leaves a party.
type Reason uint8

// Leave reasons.
const (
	Expelled Reason = iota
	Left
	Disconnected
)

// ID identifies one party for as long as it exists.
type ID uint64

// Registry owns every party and command channel. mu guards all of its
// state, including every group and channel reachable from it.
type Registry[M Member] struct {
	mu       sync.Mutex
	now      func() time.Time
	nextID   ID
	byMember map[int32]*group[M]
	byID     map[ID]*group[M]
	// lootChoice is the rule a player last chose when inviting without a
	// party of its own; the party its invitation forms takes it.
	lootChoice map[int32]LootRule
}

type group[M Member] struct {
	id     ID
	leader M
	// members is in join order. It is replaced, never changed in place, so
	// returned notices and views may share it.
	members []M
	loot    LootRule
	level   int
	// inviting is set while an invitation the leader sent waits for its
	// answer; inviteUntil ends it.
	inviting    bool
	inviteUntil time.Time
	channel     *channel[M]
}

// View is a copy of one party.
type View[M Member] struct {
	ID        ID
	Leader    M
	Members   []M
	Loot      LootRule
	Level     int
	InChannel bool
}

// NewRegistry returns an empty registry timing invitations on now; nil
// means time.Now.
func NewRegistry[M Member](now func() time.Time) *Registry[M] {
	if now == nil {
		now = time.Now
	}
	return &Registry[M]{
		now:        now,
		byMember:   make(map[int32]*group[M]),
		byID:       make(map[ID]*group[M]),
		lootChoice: make(map[int32]LootRule),
	}
}

// InParty reports whether the player is in a party.
func (r *Registry[M]) InParty(memberID int32) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.byMember[memberID] != nil
}

// View returns a copy of the player's party.
func (r *Registry[M]) View(memberID int32) (View[M], bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	g := r.byMember[memberID]
	if g == nil {
		return View[M]{}, false
	}
	return g.view(), true
}

// Members returns the members of the party id, in join order.
func (r *Registry[M]) Members(id ID) ([]M, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	g := r.byID[id]
	if g == nil {
		return nil, false
	}
	return append([]M(nil), g.members...), true
}

// SameParty reports whether both players are in one party.
func (r *Registry[M]) SameParty(a, b int32) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	g := r.byMember[a]
	return g != nil && g == r.byMember[b]
}

// SameChannel reports whether b is in the command channel a's party belongs
// to.
func (r *Registry[M]) SameChannel(a, b int32) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	g, other := r.byMember[a], r.byMember[b]
	return g != nil && other != nil && g.channel != nil && g.channel == other.channel
}

// RecalculateLevel sets the player's party level to its highest member's.
func (r *Registry[M]) RecalculateLevel(memberID int32) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if g := r.byMember[memberID]; g != nil {
		g.recalculateLevel()
	}
}

// Forget drops what the registry still holds for a player leaving the
// world once its party has let it go.
func (r *Registry[M]) Forget(memberID int32) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.lootChoice, memberID)
}

// InviteStatus is the outcome of BeginInvite.
type InviteStatus uint8

// Invite statuses.
const (
	InviteReady InviteStatus = iota
	// InviteNotLeader: only the party leader invites.
	InviteNotLeader
	// InviteFull: the party has MaxMembers members.
	InviteFull
	// InviteWaiting: another invitation is still waiting for its answer.
	InviteWaiting
	// InviteBadLoot: the loot rule a partyless inviter chose is not one.
	InviteBadLoot
)

// BeginInvite readies an invitation from requesterID. A party leader's
// party is marked as waiting for the answer; a partyless requester's loot
// choice is kept for the party the answer forms. The returned rule is the
// one the invitation offers.
func (r *Registry[M]) BeginInvite(requesterID int32, loot int32) (InviteStatus, LootRule) {
	r.mu.Lock()
	defer r.mu.Unlock()
	g := r.byMember[requesterID]
	if g == nil {
		if loot < 0 || loot >= int32(lootRuleCount) {
			return InviteBadLoot, 0
		}
		r.lootChoice[requesterID] = LootRule(loot)
		return InviteReady, LootRule(loot)
	}
	switch {
	case g.leader.ObjectID() != requesterID:
		return InviteNotLeader, 0
	case len(g.members) >= MaxMembers:
		return InviteFull, 0
	case g.inviting && r.now().Before(g.inviteUntil):
		return InviteWaiting, 0
	}
	g.setInviting(true, r.now())
	return InviteReady, g.loot
}

// CancelInvite withdraws the invitation BeginInvite readied for
// requesterID when it could not be delivered after all: the requester's
// party stops waiting, so its leader may invite again at once.
func (r *Registry[M]) CancelInvite(requesterID int32) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if g := r.byMember[requesterID]; g != nil && g.leader.ObjectID() == requesterID {
		g.setInviting(false, r.now())
	}
}

// Answer applies target's answer to requester's invitation: an acceptance
// forms a party with the requester as leader, or adds target to the
// requester's party. Either way the requester's party stops waiting.
//
// A target already in a party, or a party that filled up meanwhile, takes
// no one: membership stays one party per player and at most MaxMembers.
// Neither does a requester or target that has begun leaving the world, so
// no party forms around a player whose own departure already ran.
func (r *Registry[M]) Answer(requester, target M, accept bool) []Notice {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out notices
	g := r.byMember[requester.ObjectID()]
	if accept && r.byMember[target.ObjectID()] == nil && !requester.Departed() && !target.Departed() {
		switch {
		case g == nil:
			g = r.form(&out, requester, target)
		case len(g.members) < MaxMembers:
			r.add(&out, g, target)
		}
	}
	if g != nil {
		g.setInviting(false, r.now())
	}
	return out
}

// form builds a party of leader and target.
func (r *Registry[M]) form(out *notices, leader, target M) *group[M] {
	r.nextID++
	g := &group[M]{id: r.nextID, leader: leader, members: []M{leader, target}, loot: r.lootChoice[leader.ObjectID()]}
	r.byID[g.id] = g
	r.byMember[leader.ObjectID()] = g
	r.byMember[target.ObjectID()] = g
	g.recalculateLevel()

	out.add(WindowAll[M]{To: target, Leader: leader.ObjectID(), Loot: g.loot, Others: g.others(target)})
	out.add(WindowAdd[M]{To: []M{leader}, Leader: leader.ObjectID(), Loot: g.loot, Member: target})
	out.add(Msg[M]{To: []M{target}, ID: MsgYouJoinedParty, Name: leader.CharacterName()})
	out.add(Msg[M]{To: []M{leader}, ID: MsgJoinedParty, Name: target.CharacterName()})
	for _, m := range g.members {
		out.add(InfoRefresh[M]{Member: m})
	}
	out.add(Formed{ID: g.id})
	return g
}

// add puts player into g.
func (r *Registry[M]) add(out *notices, g *group[M], player M) {
	existing := append([]M(nil), g.members...)
	out.add(WindowAll[M]{To: player, Leader: g.leader.ObjectID(), Loot: g.loot, Others: existing})
	out.add(WindowAdd[M]{To: existing, Leader: g.leader.ObjectID(), Loot: g.loot, Member: player})
	out.add(Msg[M]{To: []M{player}, ID: MsgYouJoinedParty, Name: g.leader.CharacterName()})
	out.add(Msg[M]{To: existing, ID: MsgJoinedParty, Name: player.CharacterName()})

	g.members = append(existing, player)
	r.byMember[player.ObjectID()] = g
	if lvl := player.Level(); lvl > g.level {
		g.level = lvl
	}
	for _, m := range g.members {
		out.add(InfoRefresh[M]{Member: m})
	}
	if g.channel != nil {
		out.add(ChannelOpen[M]{To: []M{player}})
	}
}

// Leave takes member out of its party for reason. The party disperses
// when only two members were left, or when its leader leaves other than by
// disconnecting; a disconnecting leader hands the party to the next member.
func (r *Registry[M]) Leave(member M, reason Reason) []Notice {
	r.mu.Lock()
	defer r.mu.Unlock()
	g := r.byMember[member.ObjectID()]
	if g == nil {
		return nil
	}
	var out notices
	r.remove(&out, g, member, reason)
	return out
}

// Expel takes the member named name out of leaderID's party. Anyone but the
// leader, or a name no member has, expels no one.
func (r *Registry[M]) Expel(leaderID int32, name string) []Notice {
	r.mu.Lock()
	defer r.mu.Unlock()
	g := r.byMember[leaderID]
	if g == nil || g.leader.ObjectID() != leaderID {
		return nil
	}
	target, ok := g.byName(name)
	if !ok {
		return nil
	}
	var out notices
	r.remove(&out, g, target, Expelled)
	return out
}

func (r *Registry[M]) remove(out *notices, g *group[M], player M, reason Reason) {
	isLeader := g.leader.ObjectID() == player.ObjectID()
	if len(g.members) == 2 || (reason != Disconnected && isLeader) {
		r.disband(out, g)
		return
	}
	// Only a disconnecting leader gets here: its client is gone, so the
	// leadership notices go to the members who stay.
	g.members = without(g.members, player.ObjectID())
	delete(r.byMember, player.ObjectID())
	if isLeader {
		r.changeLeader(out, g, g.members[0])
	}
	g.recalculateLevel()
	out.add(FusionStop[M]{Member: player})
	if reason == Expelled {
		out.add(Msg[M]{To: []M{player}, ID: MsgExpelledFromParty})
		out.add(Msg[M]{To: g.members, ID: MsgWasExpelled, Name: player.CharacterName()})
	} else {
		out.add(Msg[M]{To: []M{player}, ID: MsgYouLeftParty})
		out.add(Msg[M]{To: g.members, ID: MsgLeftParty, Name: player.CharacterName()})
	}
	out.add(WindowDeleteAll[M]{To: player})
	out.add(WindowDelete[M]{To: g.members, Member: player})
	if g.channel != nil {
		out.add(ChannelClose[M]{To: []M{player}})
	}
}

// disband dissolves g, taking its channel down with it when its leader
// leads the channel.
func (r *Registry[M]) disband(out *notices, g *group[M]) {
	if c := g.channel; c != nil {
		out.add(ChannelClose[M]{To: g.members})
		if c.leader.ObjectID() == g.leader.ObjectID() {
			c.disband(out)
		} else {
			c.remove(out, g)
		}
	}
	for _, m := range g.members {
		delete(r.byMember, m.ObjectID())
		out.add(WindowDeleteAll[M]{To: m})
		out.add(FusionStop[M]{Member: m})
		out.add(Msg[M]{To: []M{m}, ID: MsgPartyDispersed})
	}
	g.members = nil
	delete(r.byID, g.id)
	out.add(Dispersed{ID: g.id})
}

// ChangeLeader hands requester's party to the member named name. A
// requester leading no party is told only a leader transfers its rights; a
// name no member has changes nothing.
func (r *Registry[M]) ChangeLeader(requester M, name string) []Notice {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out notices
	g := r.byMember[requester.ObjectID()]
	if g == nil || g.leader.ObjectID() != requester.ObjectID() {
		out.add(Msg[M]{To: []M{requester}, ID: MsgOnlyLeaderTransfers})
		return out
	}
	if target, ok := g.byName(name); ok {
		r.changeLeader(&out, g, target)
	}
	return out
}

func (r *Registry[M]) changeLeader(out *notices, g *group[M], player M) {
	if g.leader.ObjectID() == player.ObjectID() {
		out.add(Msg[M]{To: []M{player}, ID: MsgCannotTransferToSelf})
		return
	}
	if c := g.channel; c != nil && c.leader.ObjectID() == g.leader.ObjectID() {
		c.leader = player
		out.add(Msg[M]{To: c.members(), ID: MsgChannelLeaderNow, Name: player.CharacterName()})
	}
	g.leader = player
	for _, m := range g.members {
		out.add(WindowDeleteAll[M]{To: m})
		out.add(WindowAll[M]{To: m, Leader: player.ObjectID(), Loot: g.loot, Others: g.others(m)})
		out.add(InfoRefresh[M]{Member: m})
		out.add(Msg[M]{To: []M{m}, ID: MsgBecameLeader, Name: player.CharacterName()})
	}
}

func (g *group[M]) view() View[M] {
	return View[M]{
		ID:        g.id,
		Leader:    g.leader,
		Members:   append([]M(nil), g.members...),
		Loot:      g.loot,
		Level:     g.level,
		InChannel: g.channel != nil,
	}
}

func (g *group[M]) setInviting(inviting bool, now time.Time) {
	g.inviting = inviting
	g.inviteUntil = now.Add(InviteTimeout)
}

func (g *group[M]) recalculateLevel() {
	level := 0
	for _, m := range g.members {
		if lvl := m.Level(); lvl > level {
			level = lvl
		}
	}
	g.level = level
}

// others returns g's members but self, in join order.
func (g *group[M]) others(self M) []M {
	return without(g.members, self.ObjectID())
}

func (g *group[M]) byName(name string) (M, bool) {
	for _, m := range g.members {
		if strings.EqualFold(m.CharacterName(), name) {
			return m, true
		}
	}
	var zero M
	return zero, false
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
