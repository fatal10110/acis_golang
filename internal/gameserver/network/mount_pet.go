package network

import (
	"context"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/pet"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// actionMountDismount is the action-bar Mount/Dismount command.
const actionMountDismount = 38

// petMountRange is how close, past both bodies' collision radii, a rider
// must stand to its pet to mount it.
const petMountRange = 200

// petMountRefusals answers each refused pet mount.
var petMountRefusals = map[player.PetMountRefusal]int{
	player.PetMountRiderDead:     serverpackets.SystemMessageStriderCantBeRiddenWhileDead,
	player.PetMountPetDead:       serverpackets.SystemMessageDeadStriderCantBeRidden,
	player.PetMountPetInBattle:   serverpackets.SystemMessageStriderInBattleCantBeRidden,
	player.PetMountRiderInBattle: serverpackets.SystemMessageStriderCantBeRiddenWhileInBattle,
	player.PetMountSitting:       serverpackets.SystemMessageStriderCanBeRiddenOnlyWhileStanding,
	player.PetMountFishing:       serverpackets.SystemMessageCannotDoWhileFishing2,
	player.PetMountTooFar:        serverpackets.SystemMessageTooFarAwayFromStriderToMount,
	player.PetMountHungry:        serverpackets.SystemMessageHungryStriderNotMount,
}

// actionUseRefused runs the checks every action-bar command but sit/stand
// passes first, answering the first that refuses: a dead, fake-dead or
// out-of-control player is released with ActionFailed, an observer told it
// cannot take part.
func actionUseRefused(live *livePlayer) bool {
	if live.Dead() || live.FakeDead() || liveOutOfControl(live) {
		live.SendFrame(serverpackets.FrameActionFailed())
		return true
	}
	if live.ObserverMode() {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageObserversCannotParticipate))
		return true
	}
	return false
}

// actionMountDismount runs the action-bar Mount/Dismount command. Where
// mountPlayer does nothing the specified answer is silence too, but the
// action bar waits on an answer, so the client is released with
// ActionFailed.
func (l *GameClientLink) actionMountDismount(live *livePlayer) {
	if actionUseRefused(live) {
		return
	}
	if !l.mountPlayer(live) {
		live.SendFrame(serverpackets.FrameActionFailed())
	}
}

// mountPlayer mounts live on its rideable pet, or takes it off its mount,
// for /mount and the action-bar Mount/Dismount command. It reports whether
// live was answered: neither on a mount nor with a pet to ride, nothing
// happens.
func (l *GameClientLink) mountPlayer(live *livePlayer) bool {
	if p := l.rideablePet(live); p != nil && !live.Mounted() && !live.EffectList().IsAffected(effect.FlagBetrayed) {
		l.mountPet(live, p)
		return true
	}
	if !live.Mounted() {
		return false
	}
	if live.MountType() == player.MountTypeWyvern {
		if live.zoneActor != nil && live.zoneActor.ZoneFlags().Has(zone.FlagNoLanding) {
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNoDismountHere))
			return true
		}
		if l.unsafeDismountHeight(live) {
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCannotDismountFromElevation))
			return true
		}
	}
	if live.MountHungry() {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageHungryStriderNotMount))
		return true
	}
	live.Character.Dismount()
	return true
}

// rideablePet returns live's summon when it is a pet live can ride.
func (l *GameClientLink) rideablePet(live *livePlayer) *summon.Actor {
	if l.world == nil {
		return nil
	}
	obj, ok := l.world.Summon(live.ObjectID())
	if !ok {
		return nil
	}
	s, ok := obj.(*summon.Actor)
	if !ok || !s.IsPet() || !pet.IsMountable(s.NPCID()) {
		return nil
	}
	return s
}

// mountPet puts live on its pet p once every mount check passes; the first
// that refuses names itself. Both hands are emptied, the rider is forced
// to run and loses its toggles; then its skill list, the feed gauge
// started from the pet's own meal, the Ride and its new appearance go out,
// and the pet leaves.
func (l *GameClientLink) mountPet(live *livePlayer, p *summon.Actor) {
	refusal := live.Character.PetMountRefusal(player.RideablePet{
		Dead: p.Dead(), InCombat: p.InCombat(), Rooted: p.Rooted(),
		InReach: petInMountReach(live, p), Hungry: p.Hungry(),
	})
	if refusal != player.PetMountAllowed {
		live.SendFrame(serverpackets.FrameSystemMessage(petMountRefusals[refusal]))
		return
	}
	// A cursed weapon, the one hand disarm keeps, was refused above.
	if !l.disarm(live, true) {
		return
	}
	l.changeLiveMoveType(live, true)
	live.Character.EffectList().StopAllToggles()
	npcID, collar := int32(p.NPCID()), p.ControlItemID()
	if !live.Character.MountPet(npcID, collar, p.Level()) {
		return
	}
	live.SendFrame(serverpackets.FrameSkillList(skillListEntries(live.Character, l.skills)))
	live.Character.StartPetMountFeed(p.Fed(), collar)
	l.broadcastLiveFrame(live, func() wire.Frame {
		return serverpackets.FrameRide(live.ObjectID(), npcID)
	})
	l.broadcastCharacterInfo(live)
	p.Unsummon()
}

// petInMountReach reports whether p stands within petMountRange of live,
// counting both bodies' collision radii and the height difference.
func petInMountReach(live *livePlayer, p *summon.Actor) bool {
	ax, ay, az := live.Position()
	bx, by, bz := p.Position()
	dx, dy, dz := int64(ax-bx), int64(ay-by), int64(az-bz)
	reach := petMountRange + live.CollisionRadius() + p.CollisionRadius()
	return float64(dx*dx+dy*dy+dz*dz) <= reach*reach
}

// storePetFood writes the meal a rider's mount had left back into the row
// of the pet it was called from, on the collar's persistence lane. A save
// of that row still queued restores the pet with this meal.
func (l *GameClientLink) storePetFood(itemObjectID int32, fed int) {
	if l.petStore == nil {
		return
	}
	seq, queued := l.queuedPets.setFed(itemObjectID, fed)
	store, log := l.petStore, l.log
	l.persist.Enqueue(itemObjectID, func() {
		ctx, cancel := context.WithTimeout(context.Background(), livePlayerDetachSaveTimeout)
		defer cancel()
		if err := store.SaveFed(ctx, itemObjectID, fed); err != nil {
			log.Error().Err(err).Int32("item_obj_id", itemObjectID).Msg("store pet food")
		}
		if queued {
			l.queuedPets.written(itemObjectID, seq)
		}
	})
}

// setFed sets the meal of itemObjectID's queued pet state, if a save of it
// is still queued, and returns the new sequence that save now ends with.
func (q *queuedPets) setFed(itemObjectID int32, fed int) (uint64, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	p, ok := q.pending[itemObjectID]
	if !ok {
		return 0, false
	}
	q.seq++
	p.seq = q.seq
	p.state.Fed = fed
	q.pending[itemObjectID] = p
	return p.seq, true
}
