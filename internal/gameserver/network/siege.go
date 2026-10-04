package network

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/castle"
	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	"github.com/fatal10110/acis_golang/internal/gameserver/hero"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
)

// dispatchSiege decodes and runs one siege window packet; see
// dispatchPartyMatch.
func (l *GameClientLink) dispatchSiege(client *Client, live *livePlayer, opcode byte, payload []byte) bool {
	switch opcode {
	case clientpackets.OpcodeRequestSiegeAttackerList:
		return dispatchLive(l, client, live, payload, clientpackets.DecodeRequestSiegeList, l.requestSiegeAttackerList)
	case clientpackets.OpcodeRequestSiegeDefenderList:
		return dispatchLive(l, client, live, payload, clientpackets.DecodeRequestSiegeList, l.requestSiegeDefenderList)
	case clientpackets.OpcodeRequestJoinSiege:
		return dispatchLive(l, client, live, payload, clientpackets.DecodeRequestJoinSiege, l.requestJoinSiege)
	case clientpackets.OpcodeRequestConfirmSiegeWaitingList:
		return dispatchLive(l, client, live, payload, clientpackets.DecodeRequestConfirmSiegeWaitingList, l.requestConfirmSiegeWaitingList)
	}
	return true
}

// castleSiege resolves castle id's siege.
func (l *GameClientLink) castleSiege(id int32) (*siege.Siege, bool) {
	return l.sieges.Get(int(id))
}

// requestSiegeAttackerList shows the attacking clans of castle req.ID.
//
// ponytail: a siegable clan hall's attackers wait on the clan hall sieges
// (#244); for such an id, as for an id naming nothing, the reference sends
// nothing, the window simply staying empty, and so does this.
func (l *GameClientLink) requestSiegeAttackerList(live *livePlayer, req clientpackets.RequestSiegeList) {
	s, ok := l.castleSiege(req.ID)
	if !ok {
		return
	}
	live.SendFrame(frameSiegeAttackerList(s))
}

// requestSiegeDefenderList shows the defending clans of castle req.ID; an
// id naming no castle answers nothing, as the reference does.
func (l *GameClientLink) requestSiegeDefenderList(live *livePlayer, req clientpackets.RequestSiegeList) {
	s, ok := l.castleSiege(req.ID)
	if !ok {
		return
	}
	live.SendFrame(frameSiegeDefenderList(s))
}

// requestJoinSiege registers live's clan on castle req.ID's siege, or drops
// its registration. live needs the siege management privilege. A clan
// whose dissolution is pending may not register. Registering or dropping,
// refused or not, live then sees the siege window again.
//
// ponytail: a siegable clan hall's registration waits on the clan hall
// sieges (#244); for such an id, as for an id naming nothing, the
// reference answers nothing, and so does this.
func (l *GameClientLink) requestJoinSiege(live *livePlayer, req clientpackets.RequestJoinSiege) {
	cl, member := l.clanService().ClanOf(live.Character)
	if !member || !cl.HasPrivilege(live.ObjectID(), clan.PrivCastleSiege) {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNotAuthorizedToDoThat))
		return
	}
	s, ok := l.castleSiege(req.ID)
	if !ok {
		return
	}
	if req.IsJoining == 1 {
		if l.sieges.Now().UnixMilli() < cl.Info().DissolvingExpiry {
			live.SendFrame(serverpackets.FrameSystemMessage(siege.MsgDissolutionInProgress))
			return
		}
		var refusal siege.Message
		var refused bool
		if req.IsAttacker == 1 {
			refusal, refused = s.RegisterAttacker(cl)
		} else {
			refusal, refused = s.RegisterDefender(cl)
		}
		if refused {
			live.SendFrame(frameSiegeMessage(refusal))
		}
	} else {
		s.Unregister(cl)
	}
	live.SendFrame(l.frameSiegeInfo(live, s.Castle()))
}

// requestConfirmSiegeWaitingList is the castle lord's answer to a clan's
// request to defend castle req.CastleID, then the defender list again.
// Only the leader of the clan owning the castle answers; anyone else, a
// castle or clan id naming nothing, gets nothing, as in the reference.
func (l *GameClientLink) requestConfirmSiegeWaitingList(live *livePlayer, req clientpackets.RequestConfirmSiegeWaitingList) {
	own, ok := l.clanService().ClanOf(live.Character)
	if !ok {
		return
	}
	s, ok := l.castleSiege(req.CastleID)
	if !ok || s.Castle().OwnerID() != own.ID() || !own.IsLeader(live.ObjectID()) {
		return
	}
	target, ok := l.clanService().Table().Get(req.ClanID)
	if !ok {
		return
	}
	s.ConfirmWaiting(target, req.Approved == 1)
	live.SendFrame(frameSiegeDefenderList(s))
}

// frameSiegeInfo builds castle c's siege window as live sees it.
func (l *GameClientLink) frameSiegeInfo(live *livePlayer, c *castle.Castle) wire.Frame {
	ownerID := c.OwnerID()
	v := serverpackets.SiegeInfoView{
		CastleID:  int32(c.ID),
		IsLord:    ownerID == live.ClanID() && live.IsClanLeader(),
		OwnerID:   ownerID,
		Now:       int32(l.sieges.Now().UnixMilli() / 1000),
		SiegeDate: int32(c.SiegeDate() / 1000),
	}
	if owner, ok := l.clanService().Table().Get(ownerID); ok && ownerID > 0 {
		info := owner.Info()
		v.Owner = true
		v.OwnerName, v.LeaderName, v.AllyID, v.AllyName = info.Name, info.LeaderName, info.AllyID, info.AllyName
	}
	return serverpackets.FrameSiegeInfo(v)
}

func frameSiegeAttackerList(s *siege.Siege) wire.Frame {
	attackers := s.Attackers()
	rows := make([]serverpackets.SiegeClan, 0, len(attackers))
	for _, cl := range attackers {
		rows = append(rows, siegeClanRow(cl))
	}
	return serverpackets.FrameSiegeAttackerList(int32(s.Castle().ID), rows)
}

func frameSiegeDefenderList(s *siege.Siege) wire.Frame {
	defenders, pending := s.Defenders(), s.Pending()
	rows := make([]serverpackets.SiegeDefender, 0, len(defenders)+len(pending))
	for _, d := range defenders {
		side := serverpackets.SiegeDefenderApproved
		if d.Side == siege.SideOwner {
			side = serverpackets.SiegeDefenderOwner
		}
		rows = append(rows, serverpackets.SiegeDefender{SiegeClan: siegeClanRow(d.Clan), Side: side})
	}
	for _, cl := range pending {
		rows = append(rows, serverpackets.SiegeDefender{SiegeClan: siegeClanRow(cl), Side: serverpackets.SiegeDefenderPending})
	}
	return serverpackets.FrameSiegeDefenderList(int32(s.Castle().ID), rows)
}

func siegeClanRow(cl *clan.Clan) serverpackets.SiegeClan {
	info := cl.Info()
	return serverpackets.SiegeClan{
		ClanID: info.ID, Name: info.Name, LeaderName: info.LeaderName, CrestID: info.CrestID,
		AllyID: info.AllyID, AllyName: info.AllyName, AllyCrestID: info.AllyCrestID,
	}
}

// frameSiegeMessage builds the system message m.
func frameSiegeMessage(m siege.Message) wire.Frame {
	var params []serverpackets.SystemMessageParam
	if m.Text != "" {
		params = append(params, serverpackets.TextParam(m.Text))
	}
	if m.CastleID != 0 {
		params = append(params, serverpackets.CastleNameParam(int32(m.CastleID)))
	}
	if m.HasNumber {
		params = append(params, serverpackets.NumberParam(m.Number))
	}
	return serverpackets.FrameSystemMessageParams(m.ID, params...)
}

// siegeNpcKind is the castle messenger's type.
const siegeNpcKind = "SiegeNpc"

// showSiegeMessenger answers a talk to a castle's siege messenger, and
// reports whether f is one: the leader of the clan owning the castle reads
// siege/01.htm, or siege/03.htm while its siege is under way; anyone else
// reads siege/02.htm while it is under way and otherwise sees the siege
// window. A page comes with ActionFailed. A messenger of no castle is left
// to the chat window (the siegable clan halls' messengers, #244).
func (l *GameClientLink) showSiegeMessenger(live *livePlayer, f *npc.Folk) bool {
	if f.Instance.Template.Type != siegeNpcKind {
		return false
	}
	c, ok := l.castles.ByNPC(f.NpcID())
	if !ok {
		return false
	}
	s, ok := l.castleSiege(int32(c.ID))
	if !ok {
		return false
	}
	lord := live.IsClanLeader() && c.OwnerID() != 0 && c.OwnerID() == live.ClanID()
	underSiege := s.InProgress()
	var page string
	switch {
	case lord && underSiege:
		page = "03"
	case lord:
		page = "01"
	case underSiege:
		page = "02"
	default:
		live.SendFrame(l.frameSiegeInfo(live, c))
		return true
	}
	sendFilledHTML(live, f.ObjectID(), l.setPage("data/html/siege/"+page+".htm"), 0)
	live.SendFrame(serverpackets.FrameActionFailed())
	return true
}

// wireSiegeZones gives the castle battlefields their effects on players:
// the combat zone notices, the pvp flag of a player leaving a battlefield
// under siege, and the town teleport of a player thrown off one.
//
// ponytail: the forced dismount of a wyvern rider on a battlefield under
// siege, as on no-landing ground, is not wired (#3377).
func (l *GameClientLink) wireSiegeZones() {
	if l.zones == nil {
		return
	}
	for _, field := range zone.OfKind[*zone.Siege](l.zones) {
		field.CombatNotice = l.siegeCombatNotice
		field.FlagPvP = l.siegeFlagPvP
		field.Banish = l.siegeBanisher(field)
	}
}

// siegeBanished reports whether live is being thrown off a battlefield as
// its siege starts.
func (l *GameClientLink) siegeBanished(live *livePlayer) bool {
	_, ok := l.siegeBanishing.Load(live)
	return ok
}

// siegeCombatNotice tells a player it entered or left a combat zone. A
// player thrown off the battlefield as its siege starts hears neither: the
// reference teleports it out before the battlefield turns on.
func (l *GameClientLink) siegeCombatNotice(a zone.Actor, entering bool) {
	za, ok := a.(*liveZoneActor)
	if !ok || l.siegeBanished(za.live) {
		return
	}
	id := siege.MsgLeftCombatZone
	if entering {
		id = siege.MsgEnteredCombatZone
	}
	za.live.SendFrame(serverpackets.FrameSystemMessage(id))
}

// siegeFlagPvP marks a player leaving a battlefield under siege for the
// pvp flag; a player thrown off the battlefield as its siege starts is not
// flagged. Zone rules run with the player's zone state locked and the flag
// broadcasts, so the revalidation that ran this rule applies the flag once
// that lock is released, before its compass update, in the reference's
// order (SiegeZone.onExit, then Player.revalidateZone's compass code).
func (l *GameClientLink) siegeFlagPvP(a zone.Actor) {
	za, ok := a.(*liveZoneActor)
	if !ok || l.siegeBanished(za.live) {
		return
	}
	za.leftBattlefield = true
}

// siegeBanisher teleports a player thrown off field to its town restart
// point (SiegeZone.banishForeigners), on the player's own queue, so after
// the siege has taken its next step. Its outcome follows the reference's
// teleport made in place: thrown off as the siege starts, before field
// turns on, the player gets no combat notice or flag; thrown off while
// field is on, at the end of the siege or as the castle changes hands, it
// leaves a battlefield under siege and is flagged, even when field has
// turned off by the time the teleport runs.
func (l *GameClientLink) siegeBanisher(field *zone.Siege) func(zone.Actor) {
	return func(a zone.Actor) {
		za, ok := a.(*liveZoneActor)
		if !ok {
			return
		}
		live := za.live
		active := field.Active()
		if !active {
			l.siegeBanishing.Store(live, struct{}{})
		}
		posted := postLive(live, func() {
			defer l.siegeBanishing.Delete(live)
			if dest, ok := l.restartDestination(live); ok {
				l.teleportLivePlayer(live, dest, restartTeleportOffset)
			}
			if active && !field.Active() {
				l.startPvPFlag(live, false)
			}
		})
		if !posted {
			// The player's queue is closed, so the job never runs.
			l.siegeBanishing.Delete(live)
		}
	}
}

// siegeNotices tells the world what the sieges do.
type siegeNotices struct{ l *GameClientLink }

// SiegeNotifier returns the notifier telling the world through l.
func SiegeNotifier(l *GameClientLink) siege.Notifier { return siegeNotices{l: l} }

// The notices reach each player on its own queue, behind the siege states
// and teleports the siege posted there before them, so a player sees them
// in the reference's order.

// Announce sends m to every player in the world.
func (n siegeNotices) Announce(m siege.Message) {
	n.l.toAllPlayersQueued(func() wire.Frame { return frameSiegeMessage(m) })
}

// PlaySound plays file for every player in the world.
func (n siegeNotices) PlaySound(file string) {
	n.l.toAllPlayersQueued(func() wire.Frame { return serverpackets.FramePlaySound(file) })
}

// TellClans sends m to the members of each clan in turn.
func (n siegeNotices) TellClans(clans []*clan.Clan, m siege.Message) {
	for _, cl := range clans {
		n.l.broadcastToClanQueued(cl, nil, func() wire.Frame { return frameSiegeMessage(m) })
	}
}

// toAllPlayersQueued is toAllPlayers, each player getting the frame on its
// own queue.
func (l *GameClientLink) toAllPlayersQueued(build func() wire.Frame) {
	if l.world == nil {
		return
	}
	broadcastFrame(build, func(send func(frameReceiver)) {
		for _, p := range l.world.Players() {
			if live, ok := p.(*livePlayer); ok {
				send(queuedMember{member: live})
			}
		}
	})
}

// SetSiegeState gives each member in the world of clans the siege state
// s, then resends its UserInfo and its relations, on its own queue.
func (n siegeNotices) SetSiegeState(clans []*clan.Clan, s siege.State) {
	for _, cl := range clans {
		for _, member := range n.l.onlineClanMembers(cl, 0) {
			postLive(member, func() {
				member.SetSiegeState(int32(s))
				member.UpdateUserInfo()
				member.BroadcastRelations()
			})
		}
	}
}

// Reputation moves cl's reputation by points, showing its members the new
// score, then tells them m.
func (n siegeNotices) Reputation(cl *clan.Clan, points int, m siege.Message) {
	var change clan.ReputationChanged
	var changed bool
	if points < 0 {
		change, changed = n.l.clanService().TakeReputation(cl, -points)
	} else {
		change, changed = n.l.clanService().AddReputation(cl, points)
	}
	if changed {
		n.l.sendReputationChange(cl, change, nil)
	}
	n.l.broadcastToClanQueued(cl, nil, func() wire.Frame { return frameSiegeMessage(m) })
}

// CastleTaken has former's members take off the items c's owners wear and
// each noble of owner in the world record c in its hero diary.
func (n siegeNotices) CastleTaken(c *castle.Castle, owner, former *clan.Clan) {
	l := n.l
	l.checkCastleItems(nil, c, former)
	if l.heroes == nil {
		return
	}
	for _, member := range l.onlineClanMembers(owner, 0) {
		if member.IsNoble() {
			l.heroes.AddDiaryEntry(member.ObjectID(), hero.DiaryCastleTaken, c.ID)
		}
	}
}
