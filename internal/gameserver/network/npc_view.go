package network

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npcinfo"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// npcView is an NPC with both client views: NpcInfo, and the stationary
// ServerObjectInfo.
type npcView interface {
	world.Tracked
	NPCInfoSnapshot() npcinfo.Snapshot
	ServerObjectInfoSnapshot() npcinfo.Snapshot
}

// npcAttackRules are the attack rules of an NPC whose views tell each
// viewer whether it may attack the NPC. An NPC without them shows every
// viewer the flag its snapshot carries.
type npcAttackRules interface {
	AttackableBy(caster skilltarget.Actor) bool
	AttackableWithoutForceBy(caster skilltarget.Actor) bool
}

// npcSnapshot is v's stationary view when serverObject is set, its full
// view otherwise, with the flag it shows no particular viewer.
func npcSnapshot(v npcView, serverObject bool) npcinfo.Snapshot {
	if serverObject {
		return v.ServerObjectInfoSnapshot()
	}
	return v.NPCInfoSnapshot()
}

// npcShownAttackable reports the attackable flag viewer sees in v's view,
// whose flag for no particular viewer is fallback. NpcInfo shows whether
// viewer may attack v without Ctrl; ServerObjectInfo whether it may attack v
// at all, which also decides whether that view shows v's HP. Only a player
// is shown either.
func npcShownAttackable(v npcView, viewer *livePlayer, serverObject, fallback bool) bool {
	rules, ok := v.(npcAttackRules)
	switch {
	case !ok:
		return fallback
	case viewer == nil:
		return false
	case serverObject:
		return rules.AttackableBy(viewer.Character)
	}
	return rules.AttackableWithoutForceBy(viewer.Character)
}

// npcViewFrame builds v's view as viewer sees it: the stationary one when
// serverObject is set, the full one otherwise.
func npcViewFrame(v npcView, viewer *livePlayer, serverObject bool) wire.Frame {
	s := npcSnapshot(v, serverObject)
	s.Attackable = npcShownAttackable(v, viewer, serverObject, s.Attackable)
	return npcSnapshotFrame(s, serverObject)
}

func npcSnapshotFrame(s npcinfo.Snapshot, serverObject bool) wire.Frame {
	if serverObject {
		return serverpackets.FrameServerObjectInfo(s)
	}
	return serverpackets.FrameNPCInfo(s)
}

// broadcastNPCView shows every player that knows v, from a detached
// snapshot of its known list taken through buf, v's view again: the
// stationary one when serverObject is set, the full one otherwise. The view
// tells each viewer whether it may attack v, so a viewer gets one of two
// frames, each built once on first use from one snapshot of v.
func broadcastNPCView(buf *world.KnownBuffer, state *world.State, v npcView, serverObject bool) {
	known := buf.SnapshotCopy(state, v)
	defer known.Release()
	var snapshot npcinfo.Snapshot
	taken := false
	view := func() npcinfo.Snapshot {
		if !taken {
			snapshot, taken = npcSnapshot(v, serverObject), true
		}
		return snapshot
	}
	variant := func(attackable bool) func() wire.Frame {
		return func() wire.Frame {
			s := view()
			s.Attackable = attackable
			return npcSnapshotFrame(s, serverObject)
		}
	}
	frames := [2]fanout{newFanout(variant(false)), newFanout(variant(true))}
	defer frames[0].release()
	defer frames[1].release()
	for _, o := range known.Tracked() {
		receiver, ok := o.(frameReceiver)
		if !ok {
			continue
		}
		viewer, _ := o.(*livePlayer)
		i := 0
		if npcShownAttackable(v, viewer, serverObject, view().Attackable) {
			i = 1
		}
		frames[i].send(receiver)
	}
}
