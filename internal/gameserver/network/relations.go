package network

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// relationTo returns the RelationChanged bitmask subject shows observer:
// its PvP flag and karma, its clan leadership, and the clan wars between
// their two clans, which a clan academy member on either side neither
// shows nor sees. The siege bits wait on the siege engine (#232/#234), so
// they are always zero here.
func (l *GameClientLink) relationTo(subject, observer *player.Character) int32 {
	var bits int32
	if subject.PvPFlagState() != task.PvPFlagNone {
		bits |= serverpackets.RelationPvPFlag
	}
	if subject.Karma() > 0 {
		bits |= serverpackets.RelationHasKarma
	}
	if subject.IsClanLeader() {
		bits |= serverpackets.RelationLeader
	}
	if l.clans == nil {
		return bits
	}
	own, ok := l.clans.Table().Get(subject.ClanID())
	if !ok {
		return bits
	}
	other, ok := l.clans.Table().Get(observer.ClanID())
	if !ok || academyMember(own, subject.ID) || academyMember(other, observer.ID) {
		return bits
	}
	if other.AtWarWith(own.ID()) {
		bits |= serverpackets.RelationOneSidedWar
		if own.AtWarWith(other.ID()) {
			bits |= serverpackets.RelationMutualWar
		}
	}
	return bits
}

// academyMember reports whether objectID is in cl's academy.
func academyMember(cl *clan.Clan, objectID int32) bool {
	m, ok := cl.Member(objectID)
	return ok && m.PledgeType == clan.SubunitAcademy
}

// sendRelations sends, through send, the RelationChanged observer gets for
// subject and then, when subject has one, for its summon pet: both carry
// subject's relation to observer and whether observer may attack subject
// without force.
func (l *GameClientLink) sendRelations(subject *livePlayer, pet world.Tracked, observer *player.Character, send func(wire.Frame) bool) {
	info := serverpackets.RelationChangedInfo{
		ObjectID:         subject.ObjectID(),
		Relation:         l.relationTo(subject.Character, observer),
		IsAutoAttackable: subject.AttackableWithoutForceBy(observer),
		Karma:            int32(subject.Karma()),
		PvPFlag:          int32(subject.PvPFlagState()),
	}
	send(serverpackets.FrameRelationChanged(info))
	if pet != nil {
		info.ObjectID = pet.ObjectID()
		send(serverpackets.FrameRelationChanged(info))
	}
}

// summonOf returns live's summon in the world, or nil.
func (l *GameClientLink) summonOf(live *livePlayer) world.Tracked {
	if l.world == nil {
		return nil
	}
	pet, ok := l.world.Summon(live.ObjectID())
	if !ok {
		return nil
	}
	return pet
}

// broadcastRelations sends live's owned summon a self-view RelationChanged,
// then sends every nearby player live's relation — and its owned summon's,
// if any — as that player sees it: the shared tail of a pvp-flag or karma
// change. The summon's own RelationChanged reports the owner's karma and
// PvP flag: a summon has none of its own.
// It is the RelationChanged event's arm on livePlayer.Emit.
func (l *GameClientLink) broadcastRelations(live *livePlayer) {
	if l.world == nil {
		return
	}
	pet := l.summonOf(live)
	if pet != nil {
		live.BroadcastFrame(serverpackets.FrameRelationChanged(serverpackets.RelationChangedInfo{
			ObjectID: pet.ObjectID(),
			Relation: l.relationTo(live.Character, live.Character),
			Karma:    int32(live.Karma()),
			PvPFlag:  int32(live.PvPFlagState()),
		}))
	}
	l.world.ForEachKnown(live, func(o world.Tracked) {
		if observer, ok := o.(*livePlayer); ok {
			l.sendRelations(live, pet, observer.Character, observer.BroadcastFrame)
		}
	})
}

// broadcastSummonSpawnRelation sends live a self-view RelationChanged for its
// just-spawned pet, then sends every nearby player the pet's relation as
// that player sees it, as a summon's spawn does. Unlike broadcastRelations
// (the pvp-flag/karma tail), a summon's spawn only sends the summon's own
// RelationChanged — the owner's relation hasn't changed, so it is not
// resent here. Each observer's auto-attackable flag is the summon's own
// AttackableWithoutForceBy, which reads the summon's own zone membership
// rather than its owner's. Call after the pet is registered in world state
// (world.AddSummon), since it must already be resolvable as live's summon.
func (l *GameClientLink) broadcastSummonSpawnRelation(live *livePlayer, pet *summon.Actor) {
	if l.world == nil || pet == nil {
		return
	}
	karma := int32(live.Karma())
	pvpFlag := int32(live.PvPFlagState())
	live.BroadcastFrame(serverpackets.FrameRelationChanged(serverpackets.RelationChangedInfo{
		ObjectID: pet.ObjectID(),
		Relation: l.relationTo(live.Character, live.Character),
		Karma:    karma,
		PvPFlag:  pvpFlag,
	}))

	l.world.ForEachKnown(live, func(o world.Tracked) {
		observer, ok := o.(*livePlayer)
		if !ok {
			return
		}
		observer.BroadcastFrame(serverpackets.FrameRelationChanged(serverpackets.RelationChangedInfo{
			ObjectID: pet.ObjectID(), Relation: l.relationTo(live.Character, observer.Character),
			IsAutoAttackable: pet.AttackableWithoutForceBy(observer.Character),
			Karma:            karma, PvPFlag: pvpFlag,
		}))
	})
}
