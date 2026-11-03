package network

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/castle"
	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	"github.com/fatal10110/acis_golang/internal/gameserver/hero"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
)

// castleSiege resolves castle id's siege.
func (l *GameClientLink) castleSiege(id int32) (*siege.Siege, bool) {
	return l.sieges.Get(int(id))
}

// requestSiegeAttackerList shows the attacking clans of castle req.ID, or
// of siegable hall req.ID; an id naming neither answers nothing, the window
// staying empty.
func (l *GameClientLink) requestSiegeAttackerList(live *livePlayer, req clientpackets.RequestSiegeList) {
	s, ok := l.castleSiege(req.ID)
	if !ok {
		l.requestHallSiegeAttackerList(live, req.ID)
		return
	}
	live.SendFrame(frameSiegeAttackerList(s))
}

// requestSiegeDefenderList shows the defending clans of castle req.ID; an
// id naming no castle answers nothing, the window staying empty.
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
// refused or not, live then sees the siege window again. An id naming a
// siegable hall registers on its siege (requestJoinHallSiege); one naming
// neither answers nothing.
func (l *GameClientLink) requestJoinSiege(live *livePlayer, req clientpackets.RequestJoinSiege) {
	cl, member := l.clanService().ClanOf(live.Character)
	if !member || !cl.HasPrivilege(live.ObjectID(), clan.PrivCastleSiege) {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNotAuthorizedToDoThat))
		return
	}
	s, ok := l.castleSiege(req.ID)
	if !ok {
		l.requestJoinHallSiege(live, cl, req)
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
// castle or clan id naming nothing, gets nothing.
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
// window. A page comes with ActionFailed. A messenger of no castle is a
// siegable hall's (showHallSiegeMessenger); one of neither is left to the
// chat window.
func (l *GameClientLink) showSiegeMessenger(live *livePlayer, f *npc.Folk) bool {
	if f.Instance.Template.Type != siegeNpcKind {
		return false
	}
	c, ok := l.castles.ByNPC(f.NpcID())
	if !ok {
		return l.showHallSiegeMessenger(live, f)
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
// the combat zone notices, the forced dismount of a wyvern rider on a
// battlefield under siege, the pvp flag of a player leaving one, and the
// town teleport of a player thrown off one.
func (l *GameClientLink) wireSiegeZones(ix *zone.Index) {
	for _, field := range zone.OfKind[*zone.Siege](ix) {
		field.CombatNotice = l.siegeCombatNotice
		field.DismountTimer = l.siegeDismountTimer
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
// player thrown off the battlefield as its siege starts hears neither: it
// is teleported out before the battlefield turns on.
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
// that lock is released, ahead of its compass update.
func (l *GameClientLink) siegeFlagPvP(a zone.Actor) {
	za, ok := a.(*liveZoneActor)
	if !ok || l.siegeBanished(za.live) {
		return
	}
	za.leftBattlefield = true
}

// siegeBanisher teleports a player thrown off field to its town restart point,
// on the player's own queue, so after the siege has taken its next step. Its
// outcome is that of a teleport made in place: thrown off as the siege starts,
// before field turns on, the player gets no combat notice or flag; thrown off
// while field is on, at the end of the siege or as the castle changes hands, it
// leaves a battlefield under siege and is flagged, even when field has turned
// off by the time the teleport runs.
func (l *GameClientLink) siegeBanisher(field *zone.Siege) func(zone.Actor) {
	return func(a zone.Actor) {
		za, ok := a.(*liveZoneActor)
		if !ok {
			return
		}
		live := za.live
		l.banishOffBattlefield(live, field, func() {
			if dest, ok := l.restartDestination(live); ok {
				l.teleportLivePlayer(live, dest, restartTeleportOffset)
			}
		})
	}
}

// banishOffBattlefield runs teleport, live's banishment, on live's queue.
// Banished while field is off, as its siege starts, live hears no combat
// notice and takes no flag until the teleport has run; banished while field
// is on, it is flagged once teleported should field have turned off by
// then. A nil field leaves the teleport alone.
func (l *GameClientLink) banishOffBattlefield(live *livePlayer, field *zone.Siege, teleport func()) {
	if field == nil {
		postLive(live, teleport)
		return
	}
	active := field.Active()
	if !active {
		l.siegeBanishing.Store(live, struct{}{})
	}
	posted := postLive(live, func() {
		defer l.siegeBanishing.Delete(live)
		teleport()
		if active && !field.Active() {
			l.startPvPFlag(live, false)
		}
	})
	if !posted {
		// The player's queue is closed, so the job never runs.
		l.siegeBanishing.Delete(live)
	}
}

// siegeNotices tells the world what the sieges do.
type siegeNotices struct{ l *GameClientLink }

// SiegeNotifier returns the notifier telling the world through l.
func SiegeNotifier(l *GameClientLink) siege.Notifier { return siegeNotices{l: l} }

// The notices reach each player on its own queue, behind the siege states
// and teleports the siege posted there before them, so a player sees them
// in the order they were posted.

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

// GuardDoors makes s the siege of each door named in gates (castles.xml
// <gates>), resolved by name now and kept by door id, so a door reload
// guards the new door too. A name naming no door is left out: it is not
// one of the castle's doors.
func (n siegeNotices) GuardDoors(gates []string, s door.Siege) {
	l := n.l
	if l.doors == nil {
		return
	}
	for _, name := range gates {
		if d, ok := l.doors.DoorByName(name); ok {
			l.doors.SetDoorSiege(d.DoorID(), s)
		}
	}
}

// CloseDoors closes each door named in gates that stands open; a door
// that is unknown or broken stays as it is.
func (n siegeNotices) CloseDoors(gates []string) { n.l.setHallDoors(gates, false) }

// RespawnDoors stands each door named in gates again: a broken one
// revives with the configured RespawnRestoreHP, each closes, then its HP
// is set to its maximum, or to half of it when weak.
func (n siegeNotices) RespawnDoors(gates []string, weak bool) {
	l := n.l
	if l.doors == nil {
		return
	}
	restoreHP := l.playerConfig().RespawnRestoreHP
	for _, name := range gates {
		if d, ok := l.doors.DoorByName(name); ok {
			l.doors.RespawnDoor(d.DoorID(), restoreHP, weak)
		}
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
