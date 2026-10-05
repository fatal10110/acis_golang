package network

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/boat"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/privatestore"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// Boat deck and shore constants.
const (
	// boatShoreZ is the height of a dock's shore line: where a walk onto or
	// off a boat ends.
	boatShoreZ = -3624
	// boatEntranceReach is how close to a boarding point a player must stand
	// to board without walking there first.
	boatEntranceReach = 50
	// deckFarEdgeY is the deck's far edge: a walk on the deck past it is
	// dropped.
	deckFarEdgeY = 470
	// deckFloorZ is the deck's lowest height: a deck walk aimed below it is
	// dropped.
	deckFloorZ = -48
	// passengerDriftLimit is how far a passenger's reported deck position
	// may drift from the server's before it is put back.
	passengerDriftLimit = 500
)

// deckCenter is the middle of a boat's deck, in the boat's coordinates.
var deckCenter = boat.Point{X: 0, Y: -100}

// boatByID returns the boat with object id id, or nil.
func (l *GameClientLink) boatByID(id int32) *boat.Boat {
	if l.world == nil {
		return nil
	}
	obj, ok := l.world.Object(id)
	if !ok {
		return nil
	}
	b, _ := obj.(*boat.Boat)
	return b
}

// riddenBoat returns the boat live rides, or nil.
func riddenBoat(live *livePlayer) *boat.Boat {
	b, _ := live.Boat().(*boat.Boat)
	return b
}

func livePoint(live *livePlayer) boat.Point {
	at := live.CurrentLocation()
	return boat.Point{X: at.X, Y: at.Y}
}

func boatLocation(b *boat.Boat) location.Location {
	x, y, z := b.Position()
	return location.Location{X: x, Y: y, Z: z}
}

// requestMoveInVehicle answers RequestMoveToLocationInVehicle: a click on a
// boat's deck. Ashore it walks the player to the boat's entrance, or onto
// the deck when it already stands there; aboard it walks the player across
// the deck. Either way it grants the player leave to board.
func (l *GameClientLink) requestMoveInVehicle(live *livePlayer, req clientpackets.RequestMoveToLocationInVehicle) {
	if live.SittingNow() || !live.Standing() || live.StandingNow() {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	live.SetBoatMovement(true)
	live.SetCanBoard(true)
	target := location.Location{X: int(req.TargetX), Y: int(req.TargetY), Z: int(req.TargetZ)}
	origin := location.Location{X: int(req.OriginX), Y: int(req.OriginY), Z: int(req.OriginZ)}
	if target == origin {
		// Answered by the stop alone, as specified: no ActionFailed.
		l.sendStopMoveInVehicle(live, req.BoatID)
		return
	}
	if b := riddenBoat(live); b != nil {
		if b.ObjectID() != req.BoatID {
			live.SendFrame(serverpackets.FrameActionFailed())
			return
		}
		if target.Z > deckFloorZ {
			l.walkOnDeck(live, b, target, origin)
		}
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	b := l.boatByID(req.BoatID)
	if b == nil || b.Moving() {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	dock := b.Dock()
	point := dock.AdjustedBoardingPoint(livePoint(live), dock.BoatToWorld(target.X, target.Y), false)
	if livePoint(live).Distance(point) < boatEntranceReach {
		l.walkOnDeck(live, b, target, origin)
	} else {
		l.moveToBoatEntrance(live, point, b)
	}
	live.SendFrame(serverpackets.FrameActionFailed())
}

// walkOnDeck shows live walking across b's deck from origin to target, deck
// coordinates both, and keeps target as live's deck position. A target
// past the deck's far edge is dropped.
func (l *GameClientLink) walkOnDeck(live *livePlayer, b *boat.Boat, target, origin location.Location) {
	if target.Y > deckFarEdgeY {
		return
	}
	live.SetBoatPosition(target)
	id, boatID := live.ObjectID(), b.ObjectID()
	l.broadcastLiveFrame(live, func() wire.Frame {
		return serverpackets.FrameMoveToLocationInVehicle(id, boatID, target, origin)
	})
}

// moveToBoatEntrance walks live to point on the shore line of b's dock,
// a walk whose arrival grants leave to board; standing there already, live
// has that leave at once and is answered ActionFailed.
func (l *GameClientLink) moveToBoatEntrance(live *livePlayer, point boat.Point, b *boat.Boat) {
	dest := location.Location{X: point.X, Y: point.Y, Z: boatShoreZ}
	if live.CurrentLocation().Distance2D(dest) > boatEntranceReach {
		l.tryLiveMoveTo(live, dest, b.ObjectID())
		return
	}
	live.SetCanBoard(true)
	live.SendFrame(serverpackets.FrameActionFailed())
}

// probeBoatEntrance walks live, ashore, to a boat's entrance when its walk
// toward target crosses the entrance of the dock a boat it knows serves,
// and reports whether it did. Otherwise, with no boat known or no entrance
// crossed, live is answered ActionFailed: a ground click's own walk then
// follows that answer, as the reference's MoveBackwardToLocation does.
func (l *GameClientLink) probeBoatEntrance(live *livePlayer, target location.Location) bool {
	b := l.knownBoat(live)
	if b == nil {
		live.SendFrame(serverpackets.FrameActionFailed())
		return false
	}
	point, ok := b.Dock().BoardingPoint(livePoint(live), boat.Point{X: target.X, Y: target.Y}, false)
	if !ok {
		live.SendFrame(serverpackets.FrameActionFailed())
		return false
	}
	l.moveToBoatEntrance(live, point, b)
	return true
}

// knownBoat returns a boat obj knows, or nil.
func (l *GameClientLink) knownBoat(obj world.Tracked) *boat.Boat {
	if l.world == nil {
		return nil
	}
	var found *boat.Boat
	l.world.ForEachKnown(obj, func(obj world.Tracked) {
		if b, ok := obj.(*boat.Boat); ok && found == nil {
			found = b
		}
	})
	return found
}

// requestGetOnVehicle answers RequestGetOnVehicle: live, granted leave to
// board, steps aboard the boat it asks for, or the one it rides.
func (l *GameClientLink) requestGetOnVehicle(live *livePlayer, req clientpackets.RequestGetOnVehicle) {
	if !live.CanBoard() {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	b := riddenBoat(live)
	if b == nil {
		b = l.boatByID(req.BoatID)
		// Only a boat in sight can be boarded: the reference takes any boat
		// in the world, which a forged request would turn into a jump
		// across it.
		// Nor one under way: a real client is only walked to the entrance
		// of a boat tied up, and boarding at sea after the fare is taken
		// would ride for free.
		if b != nil && (!world.Knows(live, b) || b.Moving()) {
			b = nil
		}
	} else if b.ObjectID() != req.BoatID {
		b = nil
	}
	if b == nil {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	// No summon boards: it is dismissed.
	if s := l.liveSummon(live); s != nil {
		s.Unsummon()
	}
	live.Board(b)
	l.updateLivePlayerPosition(live, boatLocation(b), live.CurrentHeading())
	if b.AddPassenger(live.ObjectID()) {
		live.zoneActor.holdBoatPeace(true)
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageEnterPeacefulZone))
	}
	id, boatID := live.ObjectID(), b.ObjectID()
	at := location.Location{X: int(req.X), Y: int(req.Y), Z: int(req.Z)}
	l.broadcastLiveFrame(live, func() wire.Frame { return serverpackets.FrameGetOnVehicle(id, boatID, at) })
}

// maxStepOffReach is how far from its boat a passenger may step off.
const maxStepOffReach = 600

// requestGetOffVehicle answers RequestGetOffVehicle: live steps off the
// boat it rides onto the shore line of the boat's dock, toward the point
// it asks for, and walks there.
func (l *GameClientLink) requestGetOffVehicle(live *livePlayer, req clientpackets.RequestGetOffVehicle) {
	b := riddenBoat(live)
	if b == nil || b.ObjectID() != req.BoatID || (live.BoatMovement() && live.CanBoard()) {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	origin := livePoint(live)
	point := b.Dock().AdjustedBoardingPoint(origin, boat.Point{X: int(req.X), Y: int(req.Y)}, true)
	// The step-off point stays beside the boat: past the entrance it lies
	// at most about 420 away, and a real client asks for a point over the
	// side. The reference takes any point, which a forged request would
	// turn into a jump across the world.
	if origin.Distance(point) > maxStepOffReach {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	dest := location.Location{X: point.X, Y: point.Y, Z: boatShoreZ}
	l.leaveBoat(live)
	l.sendStopMoveInVehicle(live, req.BoatID)
	live.SetBoatMovement(false)
	id := live.ObjectID()
	at := location.Location{X: int(req.X), Y: int(req.Y), Z: int(req.Z)}
	l.broadcastLiveFrame(live, func() wire.Frame { return serverpackets.FrameGetOffVehicle(id, req.BoatID, at) })
	l.updateLivePlayerPosition(live, dest, live.CurrentHeading())
	l.tryLiveMoveTo(live, dest, 0)
}

// cannotMoveInVehicle answers CannotMoveAnymoreInVehicle: a passenger's
// deck walk ended where its client says. A report about another boat, or
// from a player ashore, is ignored without an answer, as specified: the
// report is no action the client waits on.
func (l *GameClientLink) cannotMoveInVehicle(live *livePlayer, req clientpackets.CannotMoveAnymoreInVehicle) {
	b := riddenBoat(live)
	if b == nil || b.ObjectID() != req.BoatID {
		return
	}
	live.SetBoatPositionHeading(location.Location{X: int(req.X), Y: int(req.Y), Z: int(req.Z)}, int(req.Heading))
	l.sendStopMoveInVehicle(live, req.BoatID)
	live.SendFrame(serverpackets.FrameActionFailed())
}

// sendStopMoveInVehicle stops live's deck walk on boat boatID where its deck
// position stands.
func (l *GameClientLink) sendStopMoveInVehicle(live *livePlayer, boatID int32) {
	at, heading := live.BoatPosition()
	live.SendFrame(serverpackets.FrameStopMoveInVehicle(live.ObjectID(), boatID, at, heading))
}

// moveOnBoat answers a passenger's ground click, target already raised to
// head height: toward the dock's entrance or exit line it walks off the
// deck, while the boat sails it walks freely near the deck's middle, and
// otherwise it walks to the line's deck point. A click no line answers on
// a boat tied up, or too far from the deck's middle, does nothing.
func (l *GameClientLink) moveOnBoat(live *livePlayer, target, packetOrigin location.Location) {
	live.SetHeading(packetOrigin.HeadingTo(target))
	b := riddenBoat(live)
	dock := b.Dock()
	moving := b.Moving()
	origin := boat.Point{X: packetOrigin.X, Y: packetOrigin.Y}
	to := boat.Point{X: target.X, Y: target.Y}
	point, crosses := dock.BoardingPoint(origin, to, true)
	if !crosses {
		point, crosses = dock.ExitPoint(origin, to, true)
	}
	if !crosses && !moving {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	deck, _ := live.BoatPosition()
	live.SetBoatMovement(true)
	toBorder := 400.0
	if !moving {
		toBorder = origin.Distance(point)
	}
	if crosses && toBorder < 90 {
		// The client walks off the deck by itself.
		l.broadcastLiveWalk(live, location.Location{X: point.X, Y: point.Y, Z: boatShoreZ})
		live.SetBoatMovement(false)
		live.SetCanBoard(false)
		return
	}
	toCenter := deckCenter.Distance(boat.Point{X: deck.X, Y: deck.Y})
	if toCenter > 350 || (!moving && toBorder > 200 && toCenter > 250) {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	if moving && toCenter < 250 {
		l.broadcastLiveWalk(live, target)
		live.SetBoatMovement(false)
		live.SetCanBoard(false)
		return
	}
	if crosses {
		onDeck := dock.WorldToBoat(point.X, point.Y)
		id, boatID := live.ObjectID(), b.ObjectID()
		to := location.Location{X: onDeck.X, Y: onDeck.Y, Z: deck.Z}
		from := location.Location{X: deck.X, Y: deck.Y, Z: deck.Z}
		l.broadcastLiveFrame(live, func() wire.Frame {
			return serverpackets.FrameMoveToLocationInVehicle(id, boatID, to, from)
		})
		live.SetBoatMovement(false)
		live.SetCanBoard(false)
	}
	live.SendFrame(serverpackets.FrameActionFailed())
}

// broadcastLiveWalk shows live walking from where it stands to dest, with
// no walk simulated: a passenger's client walks it on the boat by itself.
func (l *GameClientLink) broadcastLiveWalk(live *livePlayer, dest location.Location) {
	id, from := live.ObjectID(), live.CurrentLocation()
	l.broadcastLiveFrame(live, func() wire.Frame { return serverpackets.FrameMoveToLocation(id, dest, from) })
}

// validatePassengerPosition answers a passenger's ValidatePosition: a deck
// position drifted more than passengerDriftLimit from the server's is put
// back with GetOnVehicle, naming the boat the report names.
func (l *GameClientLink) validatePassengerPosition(live *livePlayer, boatID int32, reported location.Location) {
	deck, _ := live.BoatPosition()
	if deck.Distance2D(reported) > passengerDriftLimit {
		live.SendFrame(serverpackets.FrameGetOnVehicle(live.ObjectID(), boatID, deck))
	}
}

// leaveBoat takes live off the boat it rides, if any: it leaves the boat's
// peace and the ticket collection due is called off.
func (l *GameClientLink) leaveBoat(live *livePlayer) {
	b := riddenBoat(live)
	if b == nil {
		return
	}
	b.RemovePassenger(live.ObjectID())
	live.Board(nil)
	live.zoneActor.holdBoatPeace(false)
	live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageExitPeacefulZone))
}

// dropBoat forgets that live, leaving the world, rides a boat, telling no
// one: its saved position is already the dock's shore.
func dropBoat(live *livePlayer) {
	if b := riddenBoat(live); b != nil {
		b.DropPassenger(live.ObjectID())
		live.Board(nil)
	}
}

// liveSummon returns live's summon, or nil.
func (l *GameClientLink) liveSummon(live *livePlayer) *summon.Actor {
	if l.world == nil {
		return nil
	}
	obj, ok := l.world.Summon(live.ObjectID())
	if !ok {
		return nil
	}
	s, _ := obj.(*summon.Actor)
	return s
}

// carryPassenger puts live, aboard boatID, where the boat now stands and
// tells its client where the boat is.
func (l *GameClientLink) carryPassenger(live *livePlayer, boatID int32, at location.Location, heading int) {
	if live.BoatObjectID() != boatID {
		return
	}
	previous := live.CurrentLocation()
	facing := live.CurrentHeading()
	live.Character.SetLastKnownPosition(at, facing)
	if live.move != nil {
		live.move.SetPosition(at)
	}
	if l.world != nil {
		if err := l.world.Move(live, at.X, at.Y, at.Z); err != nil {
			l.log.Debug().Err(err).Int32("object_id", live.ObjectID()).Msg("carry passenger")
		}
	}
	l.revalidateZones(live, previous, revalidateStep)
	live.SendFrame(serverpackets.FrameOnVehicleCheckLocation(boatID, at, heading))
}

// collectFare takes one itemID ticket from live, aboard boatID, or puts it
// ashore at oust when it holds none.
func (l *GameClientLink) collectFare(live *livePlayer, boatID int32, itemID int, oust location.Location) {
	if live.BoatObjectID() != boatID {
		return
	}
	if inv := live.Inventory(); inv != nil && inv.ItemCount(int32(itemID), -1, false) >= 1 && inv.DestroyByTemplateID(int32(itemID), 1) != nil {
		live.SendFrame(serverpackets.FrameSystemMessageItemName(serverpackets.SystemMessageS1Disappeared, int32(itemID)))
		return
	}
	l.oustPassenger(live, oust)
	live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNotCorrectBoatTicket))
}

// oustPassenger puts live ashore at oust: a store it keeps open is closed
// first, and the teleport takes it off the boat.
func (l *GameClientLink) oustPassenger(live *livePlayer, oust location.Location) {
	if live.InStoreMode() {
		live.SetOperateType(privatestore.OperateNone)
		l.broadcastCharacterInfo(live)
	}
	l.teleportLivePlayer(live, oust, 0)
}

// holdBoatPeace raises (on) or releases the peace a boat keeps its
// passengers in: a peace zone's hold, and summoning friends barred, kept
// apart from the zones' own holds. The compass follows on the next zone
// revalidation, as specified.
func (a *liveZoneActor) holdBoatPeace(on bool) {
	if a == nil {
		return
	}
	a.deliveryMu.Lock()
	defer a.deliveryMu.Unlock()
	a.mu.Lock()
	defer a.mu.Unlock()
	a.flags.Set(zone.FlagPeace, on)
	a.flags.Set(zone.FlagNoSummonFriend, on)
	a.syncFlags()
}

// arriveAtBoatEntrance ends live's walk: a walk on or toward a boat is
// over, and a walk to a boat's entrance grants leave to board, warning a
// player with a summon that it cannot come aboard.
func (l *GameClientLink) arriveAtBoatEntrance(live *livePlayer) {
	live.SetBoatMovement(false)
	if held := live.heldIntention(); held.kind != heldMoveTo || held.boatID == 0 {
		return
	}
	live.SetCanBoard(true)
	if l.liveSummon(live) != nil {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageReleasePetOnBoat))
	}
}

// aboardBoat reports whether obj is a player riding a boat.
func aboardBoat(obj world.Tracked) bool {
	p, ok := obj.(*livePlayer)
	return ok && p.InBoat()
}
