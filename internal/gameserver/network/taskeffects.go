package network

import (
	"context"
	"sync"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/manager"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
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
	_ zone.Swimmer           = (*playerZones)(nil)
	_ zone.Wader             = (*playerZones)(nil)
	_ zone.Endangered        = (*playerZones)(nil)
	_ task.WaterEffects      = (*TaskEffects)(nil)
	_ task.ShadowItemEffects = (*TaskEffects)(nil)
	_ task.AutosaveEffects   = (*TaskEffects)(nil)
)

// autosaveSaveTimeout bounds one periodic full-stat save; unrelated to and
// independent from the disconnect-time save budget.
const autosaveSaveTimeout = 5 * time.Second

// playerZones is a live player's zone membership: the zone.Member every
// creature keeps, whose ledger the character reads its zones from, plus
// what only a player's zones do: the compass zone, the breath countdown,
// the move speed in water, the pvp flag of a player leaving a battlefield
// under siege, and the scripts its zone entries reach.
//
// mu serializes every zone pass on the player (each position report, the
// grid leave and rejoin of a teleport, logout, a zone reload, and a zone
// replaying its rules on the player) with the frames the pass sends, so
// compass updates go out in the order of the zone transitions. The zone
// rules a pass runs write the fields below; they only ever run inside one
// of these passes, so mu guards them too. The Member's own lock, which the
// rules run under, is released before a pass sends its frames.
type playerZones struct {
	zone.Member
	live *livePlayer

	mu          sync.Mutex
	lastCompass int32
	// inWater is the water state the move speed was last set for.
	inWater bool
	// leftBattlefield is set by the exit rule of a battlefield under siege.
	// The pass that ran it applies the pvp flag before its compass update,
	// so the flag's UserInfo goes out first.
	leftBattlefield bool
	// leftWater is set when a water zone's exit reports the player out of
	// the water. The pass that ran it may still enter another water zone,
	// so the breath countdown stops only if the player ends the pass out of
	// the water (keepBreath, #4954).
	leftWater bool
	// scriptZones are the zones a script reacts to that a pass just
	// entered. The pass hands them to scripts once it has released mu
	// (fireScriptZones).
	scriptZones []int32
	// scripts are the scripts zone entries reach; nil reaches none.
	scripts zoneScripts
	// spawnMu guards spawning and spawnRules. While spawning, the zone
	// rules the spawn's revalidation runs (a move out, its notices) are
	// held in spawnRules instead of posted, and that revalidation runs them
	// inline once it has released its locks, so their frames land at the
	// spawn's place in the enter-world burst.
	spawnMu    sync.Mutex
	spawning   bool
	spawnRules []func()
}

// newPlayerZones gives live its zone membership; the character reads its
// zones from the membership's ledger from then on.
func newPlayerZones(live *livePlayer, scripts zoneScripts) *playerZones {
	a := &playerZones{live: live, scripts: scripts}
	live.SetZoneStatus(a.ZoneFlags())
	return a
}

// zoneScripts is what a player's zone entries ask of the script registry.
type zoneScripts interface {
	ZoneEntered(zoneID int32, c attackable.Combatant)
}

// noteScriptZone records that the pass running entered the zone zoneID,
// which a script reacts to.
func (a *playerZones) noteScriptZone(zoneID int32) {
	a.scriptZones = append(a.scriptZones, zoneID)
}

// fireScriptZones hands the script zones the last passes entered to their
// scripts, in entry order, with no zone lock held.
func (a *playerZones) fireScriptZones() {
	a.mu.Lock()
	ids := a.scriptZones
	a.scriptZones = nil
	a.mu.Unlock()
	if a.scripts == nil {
		return
	}
	for _, id := range ids {
		a.scripts.ZoneEntered(id, a.live)
	}
}

// zoneRevalidation is why a player's position changed, which decides when
// its zones are revalidated.
type zoneRevalidation uint8

const (
	// revalidateStep is a movement step (zone.Member.Step).
	revalidateStep zoneRevalidation = iota
	// revalidateForce is a position update that revalidates at once
	// (zone.Member.Moved).
	revalidateForce
	// revalidatePlaced is a position set outside movement, a forced
	// flight's landing (zone.Member.Place).
	revalidatePlaced
	// revalidateStop is a move's stop where the player stands
	// (zone.Member.Settle).
	revalidateStop
)

func (a *playerZones) ObjectID() int32             { return a.live.ObjectID() }
func (a *playerZones) Position() location.Location { return a.live.CurrentLocation() }
func (a *playerZones) Class() zone.Class           { return zone.ClassPlayer }
func (a *playerZones) GM() bool                    { return a.live.accessLevel().IsGM }
func (a *playerZones) Online() bool                { return !a.live.detached() }
func (a *playerZones) Race() player.Race           { return a.live.Character.Race }
func (a *playerZones) ClanID() int32               { return a.live.Character.ClanID() }
func (a *playerZones) SiegeState() int32           { return a.live.Character.SiegeState() }

// Teleporting reports a teleport in progress: it keeps the step count but
// revalidates no zone, so the move it stops does not enter the zones of a
// position the player is leaving.
func (a *playerZones) Teleporting() bool { return a.live.Teleporting() }

// revalidateSpawn enters the player into the zones it spawns in, on its
// own queue, then runs the zone rules that entry asked of it (zoneRule)
// with no zone lock held.
func (a *playerZones) revalidateSpawn(ix *zone.Index) {
	a.spawnMu.Lock()
	a.spawning = true
	a.spawnMu.Unlock()
	a.enter(ix)
	a.spawnMu.Lock()
	rules := a.spawnRules
	a.spawning, a.spawnRules = false, nil
	a.spawnMu.Unlock()
	for _, fn := range rules {
		fn()
	}
}

// zoneRule runs fn, a zone rule's action on live, on live's queue with no
// zone lock held: at the end of live's spawn revalidation while it runs,
// else posted to the queue. It reports false when live's queue is closed.
func zoneRule(live *livePlayer, fn func()) bool {
	if a := live.zoneActor; a != nil {
		a.spawnMu.Lock()
		if a.spawning {
			a.spawnRules = append(a.spawnRules, fn)
			a.spawnMu.Unlock()
			return true
		}
		a.spawnMu.Unlock()
	}
	return postLive(live, fn)
}

// enter puts the player on the grid, at a spawn or once its client has
// appeared after a teleport, entering the zones at its position. A player
// that logged out enters none.
func (a *playerZones) enter(ix *zone.Index) {
	defer a.fireScriptZones()
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.Enter(ix, a) {
		return
	}
	a.settle()
	a.finishPass()
}

// revalidateMove revalidates the player's zones after its position changed
// for reason. A player off the grid enters no zone.
func (a *playerZones) revalidateMove(ix *zone.Index, reason zoneRevalidation) {
	defer a.fireScriptZones()
	a.mu.Lock()
	defer a.mu.Unlock()
	var onGrid bool
	switch reason {
	case revalidateStep:
		onGrid = a.Step(ix, a)
	case revalidateForce:
		onGrid = a.Moved(ix, a)
	case revalidatePlaced:
		onGrid = a.Place(ix, a)
	case revalidateStop:
		onGrid = a.Settle(ix, a)
	}
	if !onGrid {
		return
	}
	a.settle()
	a.finishPass()
}

// remove takes the player off the grid for good as it logs out. A player
// logging out is no longer tracked for the pvp flag, so leaving a
// battlefield on the way out does not flag it.
func (a *playerZones) remove(ix *zone.Index) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.Remove(ix, a)
	a.settle()
	a.leftBattlefield = false
}

// leave takes the player off the grid for a teleport: it exits every zone
// of the region holding it, and enters none until enter.
func (a *playerZones) leave(ix *zone.Index) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.Leave(ix, a)
	a.settle()
	a.flagLeftBattlefield()
}

// reload moves the player off the zones a reload swapped out and into the
// reloaded zones of the region holding it (zone.Member.Reload), sending
// its compass zone when it changed. A player off the grid or mid-teleport
// enters none and is sent nothing.
func (a *playerZones) reload(ix *zone.Index) {
	defer a.fireScriptZones()
	a.mu.Lock()
	defer a.mu.Unlock()
	onGrid := a.Reload(ix, a)
	a.settle()
	if onGrid {
		a.finishPass()
	}
}

// ReplayZoneRules runs fn, a battlefield or stadium switching its rules
// replaying them on the player, as a pass would. The compass is left for
// the player's next pass.
func (a *playerZones) ReplayZoneRules(fn func()) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.Member.ReplayZoneRules(fn)
	a.settle()
}

// settle brings what the player keeps beside its ledger in line with it
// after a pass: the move speed in or out of the water, the swamp move
// bonus outside any swamp, and the breath countdown. mu is held.
func (a *playerZones) settle() {
	a.applyWater()
	if !a.ZoneFlags().Has(zone.FlagSwamp) {
		a.live.SetSwampMoveBonus(0)
	}
	a.keepBreath()
}

// applyWater switches the move speed to the swim speed or back when the
// ledger's water state changed since it was last applied. mu is held.
func (a *playerZones) applyWater() {
	if in := a.ZoneFlags().Has(zone.FlagWater); in != a.inWater {
		a.inWater = in
		a.live.WaterChanged()
	}
}

// finishPass ends a pass that leaves the player on the grid: the pvp flag
// of a battlefield it left, then its compass zone when that changed. mu is
// held.
func (a *playerZones) finishPass() {
	a.flagLeftBattlefield()
	a.sendCompass()
}

// flagLeftBattlefield puts a player whose pass left a battlefield under
// siege on the normal pvp flag timer, flagging it when it is not. mu is
// held.
func (a *playerZones) flagLeftBattlefield() {
	if a.leftBattlefield {
		a.leftBattlefield = false
		a.live.Emit(event.PvPFlagged{})
	}
}

// sendCompass sends the player its compass zone when it changed since the
// last one sent; dropping from a siege zone to a general one updates its
// pvp status first. mu is held.
func (a *playerZones) sendCompass() {
	flags := a.ZoneFlags()
	var code int32
	switch {
	case flags.Has(zone.FlagSiege):
		code = serverpackets.CompassSiegeZone
	case flags.Has(zone.FlagPvP):
		code = serverpackets.CompassPvPZone
	case a.live.Character.In7sDungeon():
		code = serverpackets.CompassSevenSignsZone
	case flags.Has(zone.FlagPeace):
		code = serverpackets.CompassPeaceZone
	default:
		code = serverpackets.CompassGeneralZone
	}
	if code == a.lastCompass {
		return
	}
	flagPvP := a.lastCompass == serverpackets.CompassSiegeZone && code == serverpackets.CompassGeneralZone
	a.lastCompass = code
	if flagPvP {
		a.live.Character.UpdatePvPStatus()
	}
	a.live.SendFrame(serverpackets.FrameExSetCompassZoneCode(code))
}

// sendCurrentCompass sends the player its compass zone when it changed.
func (a *playerZones) sendCurrentCompass() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sendCompass()
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
	damage := drownDamage(live.MaxHPValue(), live.Race, player.ClassMage(live.ClassID()))
	// Drowning damage is a plain HP reduction with isDOT=false, so
	// drowning still allows the 1-in-10 STUN-break
	// roll, unlike a real damage-over-time skill tick.
	live.ReduceHPByDOT(damage, live, false)
	live.SendFrame(serverpackets.FrameSystemMessageNumber(serverpackets.SystemMessageDrownDamage, int32(damage)))
}

// drownDamage is the HP a swimmer past its breath loses each second: max HP
// times the race's breath multiplier times a per-second rate that is higher
// for a mage class.
func drownDamage(maxHP float64, race player.Race, mage bool) float64 {
	rate := 0.001724
	if mage {
		rate = 0.002698
	}
	return maxHP * race.BreathMultiplier() * rate
}

// Save persists actor's full character stats, position, live skill state and
// active pet on each periodic autosave. The values are copied here and written
// on the owner's persistence lane.
//
// A session mid-detach is skipped: detachLivePlayer writes the same columns
// marking the row offline, and Roster.Save marks it online. Save runs
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
	charState := live.saveState()
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
// AllowWater its breath countdown starts, or stops once the pass ends out of
// the water (keepBreath).
func (a *playerZones) SwimStateChanged(swimming bool) {
	live := a.live
	a.applyWater()
	if live.Live != nil {
		live.Move().SetSwimming(swimming)
	}
	l := live.link
	if l == nil {
		return
	}
	l.broadcastCharacterInfo(live)
	if l.water == nil || !l.playerConfig().AllowWater || live.Teleporting() {
		// A teleport stops the breath countdown itself, once the old
		// neighborhood has been forgotten.
		return
	}
	if swimming {
		l.water.Add(live, breathTime(live))
		return
	}
	a.leftWater = true
}

// breathTime is how long live holds its breath under water before drowning
// damage begins.
func breathTime(live *livePlayer) time.Duration {
	return time.Duration(live.Against(live.ConditionActor(), nil).CalcStat(stat.Breath, float64(time.Minute)*live.Race.BreathMultiplier()))
}

// keepBreath settles the breath countdown once a zone pass is over. It
// starts the countdown of a living player the pass finds swimming, with
// AllowWater, when none runs: every position update and zone revalidation
// checks it, not only a water boundary crossing, so a player revived in the
// water starts drowning again. The countdown of a player already tracked
// runs on, and a dead one is never tracked. A player a water exit of the
// pass left out of the water stops it; one the pass handed from one water
// zone to another keeps it running, whichever zone it saw first (#4954). A
// teleport handles the countdown itself. It runs under mu, as the zone
// callbacks do.
func (a *playerZones) keepBreath() {
	left := a.leftWater
	a.leftWater = false
	live := a.live
	l := live.link
	if l == nil || l.water == nil || !l.playerConfig().AllowWater || live.Teleporting() {
		return
	}
	if live.InWater() {
		l.water.Add(live, breathTime(live))
		return
	}
	if left {
		l.water.Remove(live)
	}
}

// DangerStateChanged is the player's reaction to entering a damage or
// effect zone, or leaving the last one: its own status window shows the
// danger flag.
func (a *playerZones) DangerStateChanged() {
	live := a.live
	live.SendFrame(serverpackets.FrameEtcStatusUpdate(etcStatus(live.Character)))
}

// SwampStateChanged is the player's reaction to crossing a swamp boundary:
// the move bonus of the first swamp at its position applies while any swamp
// holds it, and its appearance is rebroadcast with the new speed.
func (a *playerZones) SwampStateChanged(z *zone.Swamp) {
	live := a.live
	bonus := 0
	if a.ZoneFlags().Has(zone.FlagSwamp) {
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
	a, ok := actor.(*playerZones)
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
	// Zone entry may hold both the zone and teleport locks, so the
	// teleport starts on the player's queue after the callback returns.
	zoneRule(live, func() {
		if !live.detached() {
			l.teleportLivePlayer(live, dest, radius)
		}
	})
}

func (l *GameClientLink) revalidateZones(live *livePlayer, reason zoneRevalidation) {
	if live != nil && live.zoneActor != nil {
		live.zoneActor.revalidateMove(l.zones, reason)
	}
}

// leaveZones exits live from every zone of the region holding it as a
// teleport takes it off the grid; until rejoinZones, movement enters none.
func (l *GameClientLink) leaveZones(live *livePlayer) {
	if live.zoneActor == nil {
		return
	}
	live.zoneActor.leave(l.zones)
}

// rejoinZones enters live into the zones at its teleport destination once
// its client has appeared; a water destination starts a fresh breath
// countdown.
func (l *GameClientLink) rejoinZones(live *livePlayer) {
	if live.zoneActor == nil {
		return
	}
	live.zoneActor.enter(l.zones)
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
