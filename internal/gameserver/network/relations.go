package network

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// relationBits returns the subset of RelationChanged's bitmask this port
// computes from a Character's own state: pvp-flag and karma. The clan
// leader bit is computable from clan.Service but is not wired in yet
// (#2466); the clan-war bits wait on clan wars (#149) and the siege bits on
// the siege core/engine (#232/#234), so those bits are always zero here.
func relationBits(karma int, pvpFlag task.PvPFlagState) int32 {
	var bits int32
	if pvpFlag != task.PvPFlagNone {
		bits |= serverpackets.RelationPvPFlag
	}
	if karma > 0 {
		bits |= serverpackets.RelationHasKarma
	}
	return bits
}

// relationAutoAttackable mirrors the PvP-zone and terminal branches of
// Playable.isAttackableWithoutForceBy. The party and command-channel
// exemptions read the party registry once #2466 wires them; other earlier
// branches are tracked by their respective subsystems.
func relationAutoAttackable(karma int, pvpFlag task.PvPFlagState, subjectInPvPZone, observerInPvPZone bool) bool {
	return subjectInPvPZone && observerInPvPZone || karma > 0 || pvpFlag != task.PvPFlagNone
}

// broadcastRelations sends live's owned summon a self-view RelationChanged,
// then broadcasts live's relation — and its owned summon's, if any — to
// every nearby observer — the shared tail of a pvp-flag or karma change:
// each nearby player gets one RelationChanged for the player and, if it has
// a summon, one more for the summon, both carrying the same
// relation/auto-attackable values.
// It is the RelationChanged event's arm on livePlayer.Emit.
func (l *GameClientLink) broadcastRelations(live *livePlayer) {
	if l.world == nil {
		return
	}
	karma := live.Karma()
	pvpFlag := live.PvPFlagState()
	relation := relationBits(karma, pvpFlag)

	pet, hasPet := l.world.Summon(live.ObjectID())
	if hasPet {
		// The summon's own RelationChanged reports the owner's
		// karma/pvp-flag: Summon has no independent karma/pvp-flag state
		// of its own, matching Summon.getKarma()/getPvpFlag() delegating
		// to their owner.
		live.BroadcastFrame(serverpackets.FrameRelationChanged(serverpackets.RelationChangedInfo{
			ObjectID: pet.ObjectID(),
			Relation: relation,
			Karma:    int32(karma),
			PvPFlag:  int32(pvpFlag),
		}))
	}

	l.world.ForEachKnown(live, func(o world.Tracked) {
		observer, ok := o.(*livePlayer)
		if !ok {
			return
		}
		autoAttackable := relationAutoAttackable(karma, pvpFlag, live.InPvPZone(), observer.InPvPZone())
		observer.BroadcastFrame(serverpackets.FrameRelationChanged(serverpackets.RelationChangedInfo{
			ObjectID: live.ObjectID(), Relation: relation, IsAutoAttackable: autoAttackable,
			Karma: int32(karma), PvPFlag: int32(pvpFlag),
		}))
		if hasPet {
			observer.BroadcastFrame(serverpackets.FrameRelationChanged(serverpackets.RelationChangedInfo{
				ObjectID: pet.ObjectID(), Relation: relation, IsAutoAttackable: autoAttackable,
				Karma: int32(karma), PvPFlag: int32(pvpFlag),
			}))
		}
	})
}

// broadcastSummonSpawnRelation sends live a self-view RelationChanged for its
// just-spawned pet, then broadcasts that same relation to every nearby
// observer, as a summon's spawn does. Unlike broadcastRelations (the
// pvp-flag/karma tail), a summon's relation broadcast only ever sends the
// summon's own RelationChanged to
// nearby observers — the owner's relation hasn't changed, so it is not
// resent here. Each observer's auto-attackable flag is the summon's own
// AttackableWithoutForceBy, which reads the summon's PvP-zone membership
// rather than its owner's. Call after the pet is registered in world state
// (world.AddSummon), since it must already be resolvable as live's summon.
func (l *GameClientLink) broadcastSummonSpawnRelation(live *livePlayer, pet *summon.Actor) {
	if l.world == nil || pet == nil {
		return
	}
	karma := live.Karma()
	pvpFlag := live.PvPFlagState()
	relation := relationBits(karma, pvpFlag)

	live.BroadcastFrame(serverpackets.FrameRelationChanged(serverpackets.RelationChangedInfo{
		ObjectID: pet.ObjectID(),
		Relation: relation,
		Karma:    int32(karma),
		PvPFlag:  int32(pvpFlag),
	}))

	l.world.ForEachKnown(live, func(o world.Tracked) {
		observer, ok := o.(*livePlayer)
		if !ok {
			return
		}
		observer.BroadcastFrame(serverpackets.FrameRelationChanged(serverpackets.RelationChangedInfo{
			ObjectID: pet.ObjectID(), Relation: relation,
			IsAutoAttackable: pet.AttackableWithoutForceBy(observer.Character),
			Karma:            int32(karma), PvPFlag: int32(pvpFlag),
		}))
	})
}
