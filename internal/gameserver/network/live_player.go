package network

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attack"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cubic"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/shortcut"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/staticobject"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/rs/zerolog"
)

type livePlayer struct {
	*player.Character
	link *GameClientLink
	// ctx is the owning connection's context, used by event arms that reach
	// persistence.
	ctx context.Context
	// session sends one frame to this player's client; see SendFrame.
	session  func(wire.Frame) bool
	template *player.Template
	npcs     *npc.Table
	items    []*item.Instance
	throne   staticobject.Chair
	attack   *attack.Controller
	move     *move.Controller
	combat   *ai.PlayerAttack
	cast     *actorcast.Controller
	// summonSpawner is the pet/servitor spawner summon-request events reach.
	// useSummonItem creates it on the first pet-collar use, from the
	// connection goroutine; the cast timer goroutine reads it.
	summonSpawner atomic.Pointer[gameSummonSpawner]
	// petRestoreInFlight is set while a summon cast that has already hit is
	// still waiting for its pets-row read, and cleared when that read lands
	// however it ends.
	//
	// The reference needs no equivalent: its read is a synchronous call
	// inside useSkill, so isCastingNow() stays true across it and
	// SummonItems.useItem returns on that alone (SummonItems.java:37-38)
	// before it ever reaches the summon-slot check at :42-46. Go's read
	// leaves the queue, and the hold that stands in for that casting state
	// has a ceiling, so past the ceiling the cast is over while the pet is
	// still inbound. This flag keeps the summon slot closed for the rest of
	// the read.
	//
	// It covers the restore window only, not the whole of what :37-38
	// gates. Java's check sits above the switch at :64-65 and so applies to
	// every summon item; Go's equivalents are per-branch, and outside this
	// window the pet branch still relies on StartItemSkill's already-casting
	// rejection and the decorative branch has no such check at all. Those
	// are pre-existing gaps in the pre-cast gating, tracked by #2369 and
	// #2411 — whichever adds a pre-cast gate has to consult this flag as
	// well as hasActiveSummon, or it will close the :42-46 half and leave
	// the :37-38 half open in exactly this window.
	//
	// It is deliberately not part of hasActiveSummon: the reference answers
	// RequestAutoSoulShot with getSummon(), which is null across its own
	// read (SummonCreature.java:58 vs :64), so that gate must keep seeing an
	// empty slot.
	//
	// Only the owner's queue and its persistence continuation write it;
	// atomic so a gate reached from any other goroutine stays race-free.
	petRestoreInFlight atomic.Bool
	shortcuts          *shortcut.List
	isGM               bool
	// handlerPanicked records that a task this player's connection waited on
	// panicked. Written by onLive and read by the dispatch loop, both on the
	// owning connection goroutine and nowhere else.
	handlerPanicked bool
	log             zerolog.Logger

	known          world.KnownBuffer
	zoneActor      *liveZoneActor
	visibilitySend func(wire.Frame) bool
	// kick closes this player's client connection (ServerClose then close),
	// set once at attach time. It is the server-initiated eviction path a
	// duplicate character selection uses to take the character away from its
	// previous session.
	kick       func()
	stopAttack func(*livePlayer)
	// spawnProtectionGen is owned by p's queue.
	spawnProtectionGen uint64
	// deliveryStopped is set on p's queue when detach begins. Autosave and
	// shadow-item expiry check it on the same queue, so an autosave job either
	// sits ahead of detach's offline persistence write or is never enqueued.
	// Atomic for the readers on other goroutines.
	deliveryStopped atomic.Bool
	// pickupMu guards deferred player intentions and pickup state. Another
	// actor's queue reaches them: an effect it applies stops p's actions and
	// drops them (stopLiveActions → tryToIdle).
	pickupMu       sync.Mutex
	pickup         *pickupIntention
	deferredPickup *pickupIntention
	deferredMagic  *clientpackets.RequestMagicSkillUse
	deferredItem   *itemAICastIntention
	pickupLocked   bool
	pickupLockGen  uint64

	// fusionTargetID is the object id of the target this player's active
	// fusion channel holds, or 0; cleared only by the channel that set it.
	fusionTargetID atomic.Int32

	// petInteractMu is taken from another actor's queue for the same reason
	// as pickupMu.
	petInteractMu sync.Mutex
	petInteract   *summon.Actor

	// cubicsMu is taken from another actor's queue: another player's cubic
	// skill grants p a cubic (syncCubicTargets), and a summon-friend cast
	// teleports p, which stops them (TeleportRequested → Stop).
	cubicsMu sync.Mutex
	cubics   map[cubic.ID]*cubic.Runtime
}

// OnlineCharacter returns the character behind an online player registered
// in the world, or false when p is not one this package registered.
func OnlineCharacter(p world.Player) (*player.Character, bool) {
	live, ok := p.(*livePlayer)
	if !ok {
		return nil, false
	}
	return live.Character, true
}

// AccountName returns the owning account of this in-world player, used to
// report the online roster and per-account entries to the login server.
func (p *livePlayer) AccountName() string { return p.Character.AccountName }

type pickupIntention struct {
	ctx    context.Context
	target world.Tracked
	// shift is only meaningful on deferredPickup: it is the original click's
	// shift-modifier, needed at drain time to decide walk-vs-fail exactly as
	// a fresh click would (CreatureMove.java:438, !isShiftPressed gates the
	// walk). pickup (the in-flight walk-then-collect intention) is only ever
	// set for a non-shift click — a shift click fails outright instead of
	// walking — so it never needs this field.
	shift bool
}

type itemAICastIntention struct {
	inventory *itemcontainer.Inventory
	item      *item.Instance
	skill     modelskill.Definition
	selected  world.Tracked
}

func (p *livePlayer) sendVisibilityFrame(frame wire.Frame) bool {
	if p.visibilitySend == nil {
		frame.Release()
		return false
	}
	return p.visibilitySend(frame)
}

// kickClient closes this player's client connection. A no-op when the
// player was attached without a kick hook (defensive; every production
// attach sets one).
func (p *livePlayer) kickClient() {
	if p.kick != nil {
		p.kick()
	}
}

// after arms fn to run once d has elapsed, as a task on p's queue.
func (p *livePlayer) after(d time.Duration, fn func()) *sim.Timer {
	return p.Queue().After(d, fn)
}

// onQueue runs fn as a task on q and returns once it has run, so work a
// connection reads for an in-world player keeps its read order and runs
// serialized with that player's timers and ticks. When q no longer accepts
// tasks (the player detached, or the pool is stopping), fn runs on the
// calling goroutine as q's owner, after the task q is still running, if any.
// A task must never call it for its own queue: it would wait on itself.
//
// It reports whether fn returned normally. False means fn panicked or called
// runtime.Goexit and the pool's per-task recovery contained it, so whatever
// fn had mutated before it stopped is half-applied and the caller must not
// carry on as if the work had succeeded. Running fn on the calling goroutine
// reports true: there the panic keeps unwinding, to the connection handler's
// own recover.
func onQueue(q *sim.Queue, fn func()) (ok bool) {
	done := make(chan struct{})
	// ok is written before the deferred close and read after <-done, so the
	// waiting goroutine sees the task goroutine's write.
	if !q.Post(func() {
		defer close(done)
		fn()
		ok = true
	}) {
		sim.RunOwned(q, fn)
		return true
	}
	<-done
	return ok
}

// postLive posts fn to live's queue without waiting. It reports whether fn
// will run: a detached player's closed queue drops it, which a caller
// holding state for fn to release has to clean up itself.
func postLive(live *livePlayer, fn func()) bool {
	return live.Queue().Post(fn)
}

// onLive runs fn on live's queue, or on the calling goroutine when live is
// nil; see onQueue. It reports whether fn returned normally and records a
// failure on live, so the dispatch loop drops the session even at the call
// sites that ignore the result. The one site that clears live must consult
// the result instead: see OpcodeRequestRestart.
func onLive(live *livePlayer, fn func()) bool {
	if live == nil {
		fn()
		return true
	}
	if onQueue(live.Queue(), fn) {
		return true
	}
	live.handlerPanicked = true
	return false
}

func (p *livePlayer) Stop() {
	p.takePickup()
	p.takeDeferredPickup()
	p.takeDeferredMagicSkill()
	p.takePetInteract()
	if p.combat != nil {
		p.combat.Stop()
	}
	if p.attack != nil {
		p.attack.Stop()
	}
	if p.stopAttack != nil {
		p.stopAttack(p)
	}
	// Player.cleanup -> abortAll(true) -> _cast.stop() (Creature.java:1298-1302)
	// cancels the pending cast task on logout/deletion (CreatureCast.java:416-426),
	// so an in-flight cast never lands against an already-detached character.
	if p.cast != nil {
		p.cast.Stop()
	}
	// Free the chair for others but keep seated identity so observers still
	// receive the stand-then-delete animation when this player despawns.
	p.freeChair()
	p.stopCubics()
}

// detached reports whether p's session has begun detaching (logout).
func (p *livePlayer) detached() bool {
	return p.deliveryStopped.Load()
}

// markDetaching runs on p's queue, where autosave and shadow-item expiry
// check detached, so neither enqueues after detach's own writes (#1948).
func (p *livePlayer) markDetaching() {
	sim.AssertOwner(p.Queue())
	p.deliveryStopped.Store(true)
}

// stopCubics cancels every live cubic runtime's timers on detach, so a
// recurring action tick never fires against a session that has already
// logged out — the reference instead relies on fireAction's own
// isDead()/isOnline() self-check on its next scheduled tick, but stopping
// immediately here is equivalent and avoids a stale timer outliving the
// session.
func (p *livePlayer) stopCubics() {
	p.cubicsMu.Lock()
	defer p.cubicsMu.Unlock()
	for _, r := range p.cubics {
		r.Stop()
	}
}

func (p *livePlayer) setPickup(ctx context.Context, target world.Tracked) {
	p.pickupMu.Lock()
	defer p.pickupMu.Unlock()
	p.deferredMagic = nil
	p.deferredItem = nil
	p.pickup = &pickupIntention{ctx: ctx, target: target}
}

func (p *livePlayer) takePickup() *pickupIntention {
	p.pickupMu.Lock()
	defer p.pickupMu.Unlock()
	pickup := p.pickup
	p.pickup = nil
	return pickup
}

func (p *livePlayer) deferPickup(ctx context.Context, target world.Tracked, shift bool) {
	p.pickupMu.Lock()
	defer p.pickupMu.Unlock()
	p.deferredMagic = nil
	p.deferredItem = nil
	p.deferredPickup = &pickupIntention{ctx: ctx, target: target, shift: shift}
}

func (p *livePlayer) takeDeferredPickup() *pickupIntention {
	p.pickupMu.Lock()
	defer p.pickupMu.Unlock()
	pickup := p.deferredPickup
	p.deferredPickup = nil
	return pickup
}

func (p *livePlayer) deferMagicSkill(req clientpackets.RequestMagicSkillUse) {
	p.pickupMu.Lock()
	defer p.pickupMu.Unlock()
	p.deferredPickup = nil
	p.deferredItem = nil
	p.deferredMagic = &req
}

func (p *livePlayer) takeDeferredMagicSkill() *clientpackets.RequestMagicSkillUse {
	p.pickupMu.Lock()
	defer p.pickupMu.Unlock()
	req := p.deferredMagic
	p.deferredMagic = nil
	return req
}

func (p *livePlayer) deferItemAICast(inventory *itemcontainer.Inventory, inst *item.Instance, skill modelskill.Definition, selected world.Tracked) {
	p.pickupMu.Lock()
	defer p.pickupMu.Unlock()
	p.deferredPickup = nil
	p.deferredMagic = nil
	p.deferredItem = &itemAICastIntention{inventory: inventory, item: inst, skill: skill, selected: selected}
}

func (p *livePlayer) takeDeferredItemAICast() *itemAICastIntention {
	p.pickupMu.Lock()
	defer p.pickupMu.Unlock()
	itemCast := p.deferredItem
	p.deferredItem = nil
	return itemCast
}

func (p *livePlayer) setPetInteract(pet *summon.Actor) {
	p.petInteractMu.Lock()
	defer p.petInteractMu.Unlock()
	p.petInteract = pet
}

func (p *livePlayer) takePetInteract() *summon.Actor {
	p.petInteractMu.Lock()
	defer p.petInteractMu.Unlock()
	pet := p.petInteract
	p.petInteract = nil
	return pet
}

// tryToIdle drops every intention p holds, active and queued, and stops its
// movement (combat.Stop stops the shared move controller, whatever the walk
// was for). A character that was already unable to act keeps its intentions
// and only answers ActionFailed. One still casting answers ActionFailed too:
// its idle waits on the cast, which every caller stops next.
//
// ponytail: the sit/stand transition also defers the idle in the same way;
// not modeled, since no effect-driven stop reaches a player mid-transition
// with anything queued to drop.
func (p *livePlayer) tryToIdle(denied bool) {
	if denied {
		p.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	casting := p.CastingNow()
	p.takePickup()
	p.takeDeferredPickup()
	p.takeDeferredMagicSkill()
	p.takeDeferredItemAICast()
	p.takePetInteract()
	if p.combat != nil {
		p.combat.Stop()
	}
	if casting {
		p.SendFrame(serverpackets.FrameActionFailed())
	}
}

// clearParkedApproaches drops pickup, pet-interact, and deferred-magic
// approach slots so a later walk or chase cannot inherit them.
func (p *livePlayer) clearParkedApproaches() {
	p.takePickup()
	p.takePetInteract()
	p.takeDeferredMagicSkill()
}

// pickupLockActive has no production caller: livePickupBlockedDeferrable
// reads pickupLocked directly under its own pickupMu section instead. Kept
// for the generation-primitive regression tests, which check lock state
// independently of that section.
func (p *livePlayer) pickupLockActive() bool {
	p.pickupMu.Lock()
	defer p.pickupMu.Unlock()
	return p.pickupLocked
}

// enterPickupLock starts a new pickup-paralysis lock, invalidating any lock
// still owned by an earlier, not-yet-fired unlock, and reports the
// generation the matching exitPickupLock must present to be honored.
func (p *livePlayer) enterPickupLock() uint64 {
	p.pickupMu.Lock()
	defer p.pickupMu.Unlock()
	p.pickupLockGen++
	p.pickupLocked = true
	p.SetParalyzed(true)
	return p.pickupLockGen
}

// exitPickupLock clears the lock started by the matching enterPickupLock and
// reports whether it did. It is a no-op when gen is stale — a later click
// already replaced this lock with its own — so a delayed unlock can never
// clear a fresher lock's state or its own paralysis mid-way through.
//
// Paralyzed is cleared before pickupLocked, not after: liveItemOpsAllowed
// (the pickup gate) reads Paralyzed via a separate mutex (stateMu) than
// pickupLockActive reads pickupLocked (pickupMu), so a concurrent click can
// observe the two writes independently. Clearing pickupLocked first would
// open a window where a click reads Paralyzed()==true (still blocked) and
// then pickupLockActive()==false (not deferrable) — blocked but undeferrable,
// so it gets discarded instead of re-deferred. Clearing Paralyzed first
// means any click that still observes it true also still observes the lock
// active, and any click that observes Paralyzed already false takes the
// normal (non-blocked) path instead of consulting the lock at all.
func (p *livePlayer) exitPickupLock(gen uint64) bool {
	p.pickupMu.Lock()
	defer p.pickupMu.Unlock()
	if p.pickupLockGen != gen {
		return false
	}
	p.SetParalyzed(false)
	p.pickupLocked = false
	return true
}

func (p *livePlayer) setFusionTarget(id int32) {
	p.fusionTargetID.Store(id)
}

func (p *livePlayer) clearFusionTarget(id int32) {
	p.fusionTargetID.CompareAndSwap(id, 0)
}

func (p *livePlayer) fusesTarget(id int32) bool {
	return p.fusionTargetID.Load() == id
}

func (p *livePlayer) attackController() *attack.Controller {
	if p.attack == nil {
		p.attack = attack.NewPlayer(p.Character, nil)
		p.attack.SetQueue(p.Queue())
	}
	return p.attack
}

// castController returns live's cast controller, building it on first use
// with live as the sink its abort, stop and finish events reach.
func (l *GameClientLink) castController(live *livePlayer) *actorcast.Controller {
	if live.cast == nil {
		live.cast = actorcast.NewController(actorcast.PlayerActor{Character: live.Character}, live)
		live.cast.SetQueue(live.Queue())
		live.Character.SetCastController(live.cast)
	}
	return live.cast
}

func (p *livePlayer) inventoryItems() []*item.Instance {
	if p == nil {
		return nil
	}
	if inv := p.Inventory(); inv != nil {
		return inv.Items()
	}
	return p.items
}

// buildItemList snapshots the inventory and drops the pending
// inventory-update queue in one critical section, then builds the full
// snapshot frame unlocked.
//
// The reference's ItemList constructor clears the update list before it
// reads the item set, so a full snapshot supersedes and discards the deltas
// it already describes; the batching task then finds nothing to drain and
// sends no InventoryUpdate behind the snapshot. Taking the snapshot and the
// clear under one lock also keeps a mutation landing between them from
// being lost: it is either inside the snapshot and cleared, or queued after
// it and still delivered by the next tick.
//
// A player without a live inventory (no runtime attached yet) falls back to
// the restored row set, which has no update queue of its own.
func (p *livePlayer) buildItemList(templates *item.Table, showWindow bool) (wire.Frame, error) {
	if p == nil {
		return serverpackets.FrameItemList(nil, templates, showWindow)
	}
	inv := p.Inventory()
	if inv == nil {
		return serverpackets.FrameItemList(p.items, templates, showWindow)
	}
	var frame wire.Frame
	err := inv.BuildAndDrainUpdates(func(items []*item.Instance) error {
		var buildErr error
		frame, buildErr = serverpackets.FrameItemList(items, templates, showWindow)
		return buildErr
	})
	if err != nil {
		return wire.Frame{}, err
	}
	return frame, nil
}

// SendInventoryUpdate delivers one batch of queued inventory changes as an
// InventoryUpdate packet, implementing task.InventoryUpdateOwner for
// changes the server makes outside a client request.
func (p *livePlayer) SendInventoryUpdate(updates []itemcontainer.Update) {
	if p == nil || len(updates) == 0 {
		return
	}
	inv := p.Inventory()
	if inv == nil {
		return
	}
	frame, err := serverpackets.FrameInventoryUpdate(updates, inv.ItemsUnordered(), inv.Templates())
	if err != nil {
		p.log.Error().Err(err).Msg("build InventoryUpdate")
		return
	}
	p.SendFrame(frame)
}

func (p *livePlayer) seated() bool {
	return p != nil && p.throne != nil
}

func (p *livePlayer) freeChair() {
	if p == nil || p.throne == nil {
		return
	}
	p.throne.SetBusy(false)
}

func (p *livePlayer) releaseChair() {
	p.freeChair()
	if p != nil {
		p.throne = nil
	}
}
