package network

import (
	"context"
	"sync"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/manager"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/rs/zerolog"
)

var (
	_ zone.Swimmer           = (*liveZoneActor)(nil)
	_ zone.Wader             = (*liveZoneActor)(nil)
	_ zone.Endangered        = (*liveZoneActor)(nil)
	_ task.WaterEffects      = (*TaskEffects)(nil)
	_ task.ShadowItemEffects = (*TaskEffects)(nil)
	_ task.AutosaveEffects   = (*TaskEffects)(nil)
)

// autosaveSaveTimeout bounds one periodic full-stat save; unrelated to and
// independent from the disconnect-time save budget.
const autosaveSaveTimeout = 5 * time.Second

// liveZoneActor adapts a live player to the zone package without changing
// Character's world.Position method signature.
//
// mu serializes zone revalidation for this player. Movement revalidates on
// the player's own queue, but a summon-friend cast teleports the player from
// the caster's queue (TeleportRequested → teleportLivePlayer), and logout
// removes it from the zones on the detach path.
//
// offGrid is set from a teleport's grid leave until the Appearing rejoin.
// The player holds no zone in between, so position updates must not enter
// any.
//
// steps counts the movement steps since the zones were last revalidated: a
// step revalidates them only every zoneStepsPerRevalidation steps, while
// spawning, a teleport landing, a move's end or stop and a region change
// revalidate them at once and restart the count. While the player is
// teleporting the count still runs but no zone is revalidated, as a
// teleport stops the player's move before it leaves the grid.
type liveZoneActor struct {
	// deliveryMu keeps compass sends in the same order as zone transitions.
	// mu protects the zone state and is released before sending a frame.
	deliveryMu  sync.Mutex
	mu          sync.Mutex
	live        *livePlayer
	flags       zone.Flags
	offGrid     bool
	steps       int
	lastCompass int32
	// leftBattlefield is set, under mu, by the exit rule of a battlefield
	// under siege. The revalidation that ran it applies the pvp flag once
	// mu is released, still under deliveryMu, so the flag's UserInfo goes
	// out before that revalidation's compass update.
	leftBattlefield bool
}

// zoneStepsPerRevalidation is how many movement steps pass per zone
// revalidation.
const zoneStepsPerRevalidation = 5

// zoneRevalidation is why a player's position changed, which decides when
// its zones are revalidated.
type zoneRevalidation uint8

const (
	// revalidateStep is a movement step: it revalidates on every
	// zoneStepsPerRevalidation-th step.
	revalidateStep zoneRevalidation = iota
	// revalidateForce is a move's end: it revalidates at once.
	revalidateForce
	// revalidatePlaced is a position set outside movement (a forced
	// flight's landing): it revalidates only when it changes region, and
	// does not count as a step.
	revalidatePlaced
)

func (a *liveZoneActor) ObjectID() int32             { return a.live.ObjectID() }
func (a *liveZoneActor) Position() location.Location { return a.live.CurrentLocation() }
func (a *liveZoneActor) ZoneFlags() *zone.Flags      { return &a.flags }
func (a *liveZoneActor) Class() zone.Class           { return zone.ClassPlayer }
func (a *liveZoneActor) GM() bool                    { return a.live.accessLevel().IsGM }
func (a *liveZoneActor) Online() bool                { return !a.live.detached() }
func (a *liveZoneActor) Race() player.Race           { return a.live.Character.Race }
func (a *liveZoneActor) ClanID() int32               { return a.live.Character.ClanID() }

func (a *liveZoneActor) revalidate(ix *zone.Index) {
	a.deliveryMu.Lock()
	defer a.deliveryMu.Unlock()
	a.mu.Lock()
	if a.offGrid {
		a.mu.Unlock()
		return
	}
	a.steps = 0
	if ix != nil {
		ix.Revalidate(a)
	}
	a.syncFlags()
	code, changed, flagPvP := a.compassUpdate()
	leftBattlefield := a.takeLeftBattlefield()
	a.mu.Unlock()
	a.flagLeftBattlefield(leftBattlefield)
	a.sendCompass(code, changed, flagPvP)
}

// revalidateMove revalidates the player's zones after its position changed
// from previous for reason. A region change revalidates the zones of both
// regions at once and restarts the step count; a step then counts toward
// the next revalidation.
func (a *liveZoneActor) revalidateMove(ix *zone.Index, previous location.Location, reason zoneRevalidation) {
	a.deliveryMu.Lock()
	defer a.deliveryMu.Unlock()
	a.mu.Lock()
	if a.offGrid {
		a.mu.Unlock()
		return
	}
	// A teleport in progress keeps the count but revalidates no zone: the
	// move it stops must not enter the zones of a position it is leaving.
	teleporting := a.live.Teleporting()
	pos := a.Position()
	if reason == revalidateForce || world.RegionKey(previous.X, previous.Y) != world.RegionKey(pos.X, pos.Y) {
		a.steps = 0
		if ix != nil && !teleporting {
			ix.RevalidateMove(a, previous)
		}
	}
	if reason == revalidateStep {
		a.steps++
		if a.steps >= zoneStepsPerRevalidation {
			a.steps = 0
			if ix != nil && !teleporting {
				ix.Revalidate(a)
			}
		}
	}
	a.syncFlags()
	code, changed, flagPvP := a.compassUpdate()
	leftBattlefield := a.takeLeftBattlefield()
	a.mu.Unlock()
	a.flagLeftBattlefield(leftBattlefield)
	a.sendCompass(code, changed, flagPvP)
}

func (a *liveZoneActor) removeFrom(ix *zone.Index, x, y int) {
	a.deliveryMu.Lock()
	defer a.deliveryMu.Unlock()
	a.mu.Lock()
	defer a.mu.Unlock()
	ix.RemoveFrom(a, x, y)
	a.syncFlags()
	// A player logging out is no longer tracked for the pvp flag, so
	// leaving a battlefield on the way out does not flag it.
	a.leftBattlefield = false
}

// leave takes the player off the grid for a teleport: it exits every zone
// of the region containing (x, y), its position before the jump, and
// enters none until rejoin.
func (a *liveZoneActor) leave(ix *zone.Index, x, y int) {
	a.deliveryMu.Lock()
	defer a.deliveryMu.Unlock()
	a.mu.Lock()
	a.offGrid = true
	if ix != nil {
		ix.RemoveFrom(a, x, y)
	}
	a.syncFlags()
	leftBattlefield := a.takeLeftBattlefield()
	a.mu.Unlock()
	a.flagLeftBattlefield(leftBattlefield)
}

// rejoin puts the player back on the grid once its client has appeared,
// entering the zones at its current position.
func (a *liveZoneActor) rejoin(ix *zone.Index) {
	a.deliveryMu.Lock()
	defer a.deliveryMu.Unlock()
	a.mu.Lock()
	a.offGrid = false
	a.steps = 0
	if ix != nil {
		ix.Revalidate(a)
	}
	a.syncFlags()
	code, changed, flagPvP := a.compassUpdate()
	leftBattlefield := a.takeLeftBattlefield()
	a.mu.Unlock()
	a.flagLeftBattlefield(leftBattlefield)
	a.sendCompass(code, changed, flagPvP)
}

// takeLeftBattlefield reports and clears whether the revalidation just run
// left a battlefield under siege. mu must be held.
func (a *liveZoneActor) takeLeftBattlefield() bool {
	left := a.leftBattlefield
	a.leftBattlefield = false
	return left
}

// flagLeftBattlefield puts a player that left a battlefield under siege on
// the normal pvp flag timer, flagging it when it is not
// (SiegeZone.onExit). It runs with deliveryMu held and mu released,
// before the compass update of the same revalidation.
func (a *liveZoneActor) flagLeftBattlefield(left bool) {
	if left {
		a.live.Emit(event.PvPFlagged{})
	}
}

func (a *liveZoneActor) syncFlags() {
	a.live.SetInPvPZone(a.flags.Has(zone.FlagPvP))
	a.live.SetInPeaceZone(a.flags.Has(zone.FlagPeace))
	a.live.SetInSiegeZone(a.flags.Has(zone.FlagSiege))
	a.live.SetInNoSummonFriendZone(a.flags.Has(zone.FlagNoSummonFriend))
	a.live.SetInBossZone(a.flags.Has(zone.FlagBoss))
	a.live.SetInClanHallZone(a.flags.Has(zone.FlagClanHall))
	a.live.SetInWater(a.flags.Has(zone.FlagWater))
	a.live.SetInDangerArea(a.flags.Has(zone.FlagDanger))
	if !a.flags.Has(zone.FlagSwamp) {
		a.live.SetSwampMoveBonus(0)
	}
}

func (a *liveZoneActor) compassUpdate() (code int32, changed, flagPvP bool) {
	switch {
	case a.flags.Has(zone.FlagSiege):
		code = serverpackets.CompassSiegeZone
	case a.flags.Has(zone.FlagPvP):
		code = serverpackets.CompassPvPZone
	case a.live.Character.In7sDungeon():
		code = serverpackets.CompassSevenSignsZone
	case a.flags.Has(zone.FlagPeace):
		code = serverpackets.CompassPeaceZone
	default:
		code = serverpackets.CompassGeneralZone
	}
	if code == a.lastCompass {
		return code, false, false
	}
	flagPvP = a.lastCompass == serverpackets.CompassSiegeZone && code == serverpackets.CompassGeneralZone
	a.lastCompass = code
	return code, true, flagPvP
}

func (a *liveZoneActor) sendCompass(code int32, changed, flagPvP bool) {
	if !changed {
		return
	}
	if flagPvP {
		a.live.Character.UpdatePvPStatus()
	}
	a.live.SendFrame(serverpackets.FrameExSetCompassZoneCode(code))
}

func (a *liveZoneActor) sendCurrentCompass() {
	a.deliveryMu.Lock()
	defer a.deliveryMu.Unlock()
	a.mu.Lock()
	code, changed, flagPvP := a.compassUpdate()
	a.mu.Unlock()
	a.sendCompass(code, changed, flagPvP)
}

// TaskEffects routes periodic task effects to their current live player.
//
// The Set* wiring runs once during boot, before any task or listener starts,
// so the fields it writes are read-only by the time a task reads them.
type TaskEffects struct {
	state *world.State
	log   zerolog.Logger

	expire  func(*livePlayer, *item.Instance)
	roster  *manager.Roster
	skills  *skillstate.Persistence
	pets    petStore
	persist *persist.Worker
}

func NewTaskEffects(state *world.State) *TaskEffects {
	return &TaskEffects{state: state}
}

// SetShadowItemExpiry connects expiry to the live inventory owner.
func (e *TaskEffects) SetShadowItemExpiry(expire func(*livePlayer, *item.Instance)) {
	e.expire = expire
}

// SetAutosave connects the periodic autosave task's Save effect to the
// character and pet persistence, skill-state persistence, the persistence
// worker its writes run on, and error logger. Autosave is wired after
// construction (like SetShadowItemExpiry above) since TaskEffects itself is
// what task.Autosave needs to be built.
func (e *TaskEffects) SetAutosave(roster *manager.Roster, skills *skillstate.Persistence, pets petStore, worker *persist.Worker, log zerolog.Logger) {
	e.roster = roster
	e.skills = skills
	e.pets = pets
	e.persist = worker
	e.log = log
}

func (e *TaskEffects) GaugeSet(actor task.WaterActor, remaining time.Duration) {
	if actor == nil || e.state == nil {
		return
	}
	obj, ok := e.state.Player(actor.ObjectID())
	if !ok {
		return
	}
	live, ok := obj.(*livePlayer)
	if !ok {
		return
	}
	ms := int(remaining.Milliseconds())
	live.SendFrame(serverpackets.FrameSetupGauge(serverpackets.GaugeCyan, ms, ms))
}

func (e *TaskEffects) Drown(actor task.WaterActor) {
	if actor == nil || e.state == nil {
		return
	}
	obj, ok := e.state.Player(actor.ObjectID())
	if !ok {
		return
	}
	live, ok := obj.(*livePlayer)
	if !ok || live.Dead() {
		return
	}
	coefficient := 0.001724
	if player.ClassMage(live.ClassID()) {
		coefficient = 0.002698
	}
	damage := live.MaxHPValue() * live.Race.BreathMultiplier() * coefficient
	// Drowning damage is a plain HP reduction with isDOT=false, so
	// drowning still allows the 1-in-10 STUN-break
	// roll, unlike a real damage-over-time skill tick.
	live.ReduceHPByDOT(damage, live, false)
	live.SendFrame(serverpackets.FrameSystemMessageNumber(serverpackets.SystemMessageDrownDamage, int32(damage)))
}

// Save persists actor's full character stats, position, live skill state and
// active pet on each periodic autosave. The values are copied here and written
// on the owner's persistence lane.
//
// A session mid-detach is skipped: detachLivePlayer writes the same columns
// and then marks the row offline, and Roster.Save marks it online. Save runs
// on the player's queue, as detach does, so an autosave job either sits ahead
// of detach's jobs on the lane or is never enqueued. Its online write
// therefore cannot land after detach's offline write (#1948).
func (e *TaskEffects) Save(actor task.AutosaveActor) {
	roster, skills, pets, worker, log := e.roster, e.skills, e.pets, e.persist, e.log
	if actor == nil || e.state == nil || roster == nil {
		return
	}
	obj, ok := e.state.Player(actor.ObjectID())
	if !ok {
		return
	}
	live, ok := obj.(*livePlayer)
	// A relogged character's newer live is not this queue's to touch.
	if !ok || live.Queue() != actor.Queue() || live.detached() {
		return
	}
	sim.AssertOwner(live.Queue())
	charState := live.Character.SaveState()
	skillState := skills.SaveState(live.Character)
	var petItemID int32
	var savePetRow func(context.Context)
	if obj, ok := e.state.Summon(live.ObjectID()); ok {
		if actor, ok := obj.(*summon.Actor); ok {
			petItemID, _, savePetRow = savePet(pets, actor, live.Inventory(), log)
		}
	}

	worker.Enqueue(live.ObjectID(), func() {
		ctx, cancel := context.WithTimeout(context.Background(), autosaveSaveTimeout)
		defer cancel()
		if err := roster.Save(ctx, charState); err != nil {
			log.Error().Err(err).Int32("object_id", charState.ID).Msg("autosave player stats")
		}
		if err := roster.SavePosition(ctx, charState); err != nil {
			log.Error().Err(err).Int32("object_id", charState.ID).Msg("autosave player position")
		}
		if err := skills.Save(ctx, skillState); err != nil {
			log.Error().Err(err).Int32("object_id", charState.ID).Msg("autosave player skill state")
		}
	})
	if savePetRow != nil {
		// The pets row is ordered on its control item's lane, with the
		// unsummon, logout and rename writes of the same row.
		worker.Enqueue(petItemID, func() {
			ctx, cancel := context.WithTimeout(context.Background(), autosaveSaveTimeout)
			defer cancel()
			savePetRow(ctx)
		})
	}
}

func (e *TaskEffects) ManaThreshold(actorID int32, inst *item.Instance, secondsLeft int) {
	if inst == nil {
		return
	}
	message := serverpackets.SystemMessageRemainingMana10Minutes
	switch secondsLeft {
	case 300:
		message = serverpackets.SystemMessageRemainingMana5Minutes
	case 60:
		message = serverpackets.SystemMessageRemainingMana1Minute
	case 600:
	default:
		return
	}
	e.deliver(actorID, serverpackets.FrameSystemMessageItemName(message, inst.TemplateID))
}

func (e *TaskEffects) Expire(actorID int32, inst *item.Instance) {
	if inst == nil || e.state == nil {
		return
	}
	obj, ok := e.state.Player(actorID)
	if !ok {
		return
	}
	live, ok := obj.(*livePlayer)
	if !ok {
		return
	}
	postLive(live, func() {
		sim.AssertOwner(live.Queue())
		if live.detached() {
			return
		}
		if current, ok := e.state.Player(actorID); !ok || current != live {
			return
		}
		if e.expire != nil {
			e.expire(live, inst)
		}
	})
}

// SwimStateChanged is the player's reaction to crossing a water zone
// boundary: its appearance is rebroadcast (swim stance and speed), and with
// AllowWater its breath countdown starts or stops.
func (a *liveZoneActor) SwimStateChanged(swimming bool) {
	live := a.live
	live.SetInWater(a.flags.Has(zone.FlagWater))
	if live.Live != nil {
		live.Move().SetSwimming(swimming)
	}
	l := live.link
	if l == nil {
		return
	}
	l.broadcastCharacterInfo(live)
	if l.water == nil || !l.playerConfig.AllowWater || live.Teleporting() {
		// A teleport stops the breath countdown itself, once the old
		// neighborhood has been forgotten.
		return
	}
	if swimming {
		breath := time.Duration(live.CalcStat(stat.Breath, float64(time.Minute)*live.Race.BreathMultiplier()))
		l.water.Add(live, breath)
		return
	}
	l.water.Remove(live)
}

// DangerStateChanged is the player's reaction to entering a damage or
// effect zone, or leaving the last one: its own status window shows the
// danger flag.
func (a *liveZoneActor) DangerStateChanged() {
	live := a.live
	live.SetInDangerArea(a.flags.Has(zone.FlagDanger))
	live.SendFrame(serverpackets.FrameEtcStatusUpdate(etcStatus(live.Character)))
}

// SwampStateChanged is the player's reaction to crossing a swamp boundary:
// the move bonus of the first swamp at its position applies while any swamp
// holds it, and its appearance is rebroadcast with the new speed.
func (a *liveZoneActor) SwampStateChanged(z *zone.Swamp) {
	live := a.live
	bonus := 0
	if a.flags.Has(zone.FlagSwamp) {
		bonus = z.MoveBonus
		if l := live.link; l != nil && l.zones != nil {
			pos := a.Position()
			if first, ok := zone.FindAt[*zone.Swamp](l.zones, pos.X, pos.Y, pos.Z); ok {
				bonus = first.MoveBonus
			}
		}
	}
	live.SetSwampMoveBonus(bonus)
	if live.link != nil {
		live.link.broadcastCharacterInfo(live)
	}
}

func (l *GameClientLink) ejectBossPlayer(boss *zone.Boss, actor zone.Actor) {
	a, ok := actor.(*liveZoneActor)
	if !ok {
		return
	}
	live := a.live
	dest, radius := boss.OustLoc, 0
	if dest.X == 0 || dest.Y == 0 || dest.Z == 0 {
		var found bool
		dest, found = l.restartDestination(live)
		if !found {
			l.log.Warn().Int32("object_id", live.ObjectID()).Msg("boss zone: no town restart point resolved")
			return
		}
		radius = restartTeleportOffset
	}
	// Zone entry may hold both the zone and teleport locks. Post to the
	// player's queue so the teleport starts after the callback returns.
	postLive(live, func() {
		if !live.detached() {
			live.Emit(event.TeleportRequested{X: dest.X, Y: dest.Y, Z: dest.Z, Radius: radius})
		}
	})
}

func (l *GameClientLink) revalidateZones(live *livePlayer, previous location.Location, reason zoneRevalidation) {
	if live != nil && live.zoneActor != nil {
		live.zoneActor.revalidateMove(l.zones, previous, reason)
	}
}

// leaveZones exits live from every zone around its current position as a
// teleport takes it off the grid; until rejoinZones, movement enters none.
func (l *GameClientLink) leaveZones(live *livePlayer) {
	if live.zoneActor == nil {
		return
	}
	position := live.CurrentLocation()
	live.zoneActor.leave(l.zones, position.X, position.Y)
}

// rejoinZones enters live into the zones at its teleport destination once
// its client has appeared; a water destination starts a fresh breath
// countdown.
func (l *GameClientLink) rejoinZones(live *livePlayer) {
	if live.zoneActor == nil {
		return
	}
	live.zoneActor.rejoin(l.zones)
}

func (e *TaskEffects) deliver(actorID int32, frame wire.Frame) {
	if e.state == nil {
		return
	}
	obj, ok := e.state.Player(actorID)
	if !ok {
		return
	}
	if live, ok := obj.(*livePlayer); ok {
		live.SendFrame(frame)
	}
}
