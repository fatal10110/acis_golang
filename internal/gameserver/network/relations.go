package network

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/worldobject"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// relationBits returns the subset of RelationChanged's bitmask this port
// can compute from a Character's own state: pvp-flag and karma. Clan
// leadership and clan-war bits (Player.java:816-838) require the clan/pledge
// subsystem tracked in #149 (Clan/pledge: creation, wars, subpledges, clan
// skills, privileges); the siege bits require #232/#234 (siege core/engine).
// Neither subsystem exists in this port yet, so those bits are always zero
// here.
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

// relationAutoAttackable mirrors the terminal branch of
// Playable.isAttackableWithoutForceBy (Playable.java:526-528): "CTRL is
// not needed if the target is flagged/PK". The party, clan, ally,
// Olympiad, duel, siege-side, and PVP-zone exemptions earlier in that
// method depend on subsystems this port doesn't have yet, so this always
// reduces to the flagged/karma check — every relation observer sees the
// same value for a given subject.
func relationAutoAttackable(karma int, pvpFlag task.PvPFlagState) bool {
	return karma > 0 || pvpFlag != task.PvPFlagNone
}

// broadcastRelations sends live's owned summon a self-view RelationChanged,
// then broadcasts live's relation — and its owned summon's, if any — to
// every nearby observer, mirroring Player.updatePvPFlag/setKarma's shared
// tail (Player.java:800-803, 1083-1086) followed by
// broadcastRelationsChanges() (Player.java:6827-6839): each nearby player
// gets one RelationChanged for `this` and, if `_summon != null`, one more
// for `_summon`, both carrying the same relation/auto-attackable values.
// Wired as live.Character's SetRelationBroadcaster hook.
func (l *GameClientLink) broadcastRelations(live *livePlayer) {
	if l.world == nil {
		return
	}
	karma := live.KarmaPoints
	pvpFlag := live.PvPFlagState()
	relation := relationBits(karma, pvpFlag)
	autoAttackable := relationAutoAttackable(karma, pvpFlag)

	pet, hasPet := l.world.Summon(live.ObjectID())
	if hasPet {
		// The summon's own RelationChanged reports the owner's
		// karma/pvp-flag: Summon has no independent karma/pvp-flag state
		// of its own, matching Summon.getKarma()/getPvpFlag() delegating
		// to their owner.
		live.SendFrame(serverpackets.FrameRelationChanged(serverpackets.RelationChangedInfo{
			ObjectID: pet.ObjectID(),
			Relation: relation,
			Karma:    int32(karma),
			PvPFlag:  int32(pvpFlag),
		}))
	}

	nearby := func(send func(frameReceiver)) {
		l.world.ForEachKnown(live, func(o world.Tracked) {
			if receiver, ok := o.(frameReceiver); ok {
				send(receiver)
			}
		})
	}
	ownerInfo := serverpackets.RelationChangedInfo{
		ObjectID: live.ObjectID(), Relation: relation, IsAutoAttackable: autoAttackable,
		Karma: int32(karma), PvPFlag: int32(pvpFlag),
	}
	broadcastFrame(func() wire.Frame { return serverpackets.FrameRelationChanged(ownerInfo) }, nearby)

	if hasPet {
		petInfo := serverpackets.RelationChangedInfo{
			ObjectID: pet.ObjectID(), Relation: relation, IsAutoAttackable: autoAttackable,
			Karma: int32(karma), PvPFlag: int32(pvpFlag),
		}
		broadcastFrame(func() wire.Frame { return serverpackets.FrameRelationChanged(petInfo) }, nearby)
	}
}

// broadcastSummonSpawnRelation sends live a self-view RelationChanged for its
// just-spawned pet, then broadcasts that same relation to every nearby
// observer, mirroring Summon.onSpawn (Summon.java:336-349) together with
// Summon's own broadcastRelationsChanges override (Summon.java:351-355).
// Unlike broadcastRelations (Player.updatePvPFlag/setKarma's shared tail),
// Summon's override only ever sends the summon's own RelationChanged to
// nearby observers — the owner's relation hasn't changed, so it is not
// resent here. Call after the pet is registered in world state (world.AddSummon),
// since it must already be resolvable as live's summon.
func (l *GameClientLink) broadcastSummonSpawnRelation(live *livePlayer, pet worldobject.Object) {
	if l.world == nil || pet == nil {
		return
	}
	karma := live.KarmaPoints
	pvpFlag := live.PvPFlagState()
	relation := relationBits(karma, pvpFlag)
	autoAttackable := relationAutoAttackable(karma, pvpFlag)

	live.SendFrame(serverpackets.FrameRelationChanged(serverpackets.RelationChangedInfo{
		ObjectID: pet.ObjectID(),
		Relation: relation,
		Karma:    int32(karma),
		PvPFlag:  int32(pvpFlag),
	}))

	petInfo := serverpackets.RelationChangedInfo{
		ObjectID: pet.ObjectID(), Relation: relation, IsAutoAttackable: autoAttackable,
		Karma: int32(karma), PvPFlag: int32(pvpFlag),
	}
	broadcastFrame(func() wire.Frame { return serverpackets.FrameRelationChanged(petInfo) }, func(send func(frameReceiver)) {
		l.world.ForEachKnown(live, func(o world.Tracked) {
			if receiver, ok := o.(frameReceiver); ok {
				send(receiver)
			}
		})
	})
}
