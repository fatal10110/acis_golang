package network

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attack"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cubic"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/admin"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/macro"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/multisell"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/shortcut"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/staticobject"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/rs/zerolog"
)

// A live player reaches attackable.PlayableRefuses as the attacker behind a
// dynamic type assertion; if it stopped satisfying Playable, every
// playable-attacker rule would silently turn off.
var _ attackable.Playable = (*livePlayer)(nil)

type livePlayer struct {
	*player.Character
	link *GameClientLink
	// ctx is the owning connection's context, used by event arms that reach
	// persistence.
	ctx context.Context
	// session sends one frame to this player's client; see SendFrame.
	session func(wire.Frame) bool
	npcs    *npc.Table
	items   []*item.Instance
	throne  staticobject.Chair
	attack  *attack.Controller
	move    *move.Controller
	combat  *ai.PlayerAttack
	cast    *actorcast.Controller
	// petRestoreInFlight is set while a summon cast that has already hit is
	// still waiting for its pets-row read, and cleared when that read lands
	// however it ends.
	//
	// A synchronous read would need no such flag: the caster would stay
	// casting across it, and a summon item's use refuses a casting player
	// before it ever reaches the summon-slot check. Go's read
	// leaves the queue, and the hold that stands in for that casting state
	// has a ceiling, and crowd control or death can end the cast early, so
	// the cast can be over while the pet is still inbound. This flag keeps
	// the summon slot closed for the rest of the read: every summon item
	// and a servitor cast treat it as the casting state it stands in for.
	//
	// It is deliberately not part of hasActiveSummon: RequestAutoSoulShot
	// answers from the summon slot, which stays empty across the pet-row
	// read, so that gate must keep seeing an empty slot.
	//
	// Only the owner's queue and its persistence continuation write it;
	// atomic so a gate reached from any other goroutine stays race-free.
	petRestoreInFlight atomic.Bool
	// inventoryDisabled is set while a shop or warehouse window keeps item
	// list requests unanswered (see tempInventoryDisable). Set on the
	// owner's queue; atomic for any reader.
	inventoryDisabled atomic.Bool
	// replayingEffects is set while EnterWorld replays the saved effects
	// and then decides the weight penalty band, before the player is in the
	// world. The effects' start hooks change its appearance and the band
	// may move, but it has no observers yet and the EnterWorld frames that
	// follow carry the result, so both refreshes stay silent. Written on the
	// owner's queue; atomic for the Emit readers.
	replayingEffects atomic.Bool
	shortcuts        *shortcut.List
	macros           *macro.List
	// board is the player's community board session; owned by its queue.
	board boardSession
	// access is the character's access level, resolved at login and
	// replaced by setAccessLevel on p's queue; any goroutine reads it
	// through accessLevel.
	access atomic.Pointer[admin.AccessLevel]
	// teleportMode is how p's move clicks travel; owned by p's queue.
	teleportMode teleportMode
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
	// entered is set on p's queue once the login spawned p. The player is
	// registered in the world from its selection on, so registration alone
	// does not mean it is in the world yet. Atomic for readers on other
	// goroutines.
	entered atomic.Bool
	// pickupMu guards deferred player intentions and pickup state. Another
	// actor's queue reaches them: an effect it applies stops p's actions and
	// drops them (stopLiveActions → tryToIdle).
	pickupMu       sync.Mutex
	pickup         *pickupIntention
	deferredPickup *pickupIntention
	deferredMagic  *deferredMagicSkill
	deferredItem   *itemAICastIntention
	deferredFollow *followIntention
	// deferredUseItem is a weapon or shield toggle queued as the next
	// intention; see tryToUseItem.
	deferredUseItem *useItemIntention
	// deferredAction runs a player request held until a swing, cast or posture settles.
	deferredAction func()
	// deferredInteract is an interact queued as the next intention; see
	// tryToInteract.
	deferredInteract *interactIntention
	// held is the current intention when it is one no other slot records:
	// a walk to a point or an equip toggle left current; see heldIntention.
	held          heldIntention
	pickupLocked  bool
	pickupLockGen uint64

	// currentFolk is the civilian NPC this player last selected. Selecting
	// another kind of object keeps it; clearing the selection drops it.
	// Trainer, shop and other service requests act on it while
	// playerCanDoInteract(p, currentFolk) holds.
	currentFolk atomic.Pointer[npc.Folk]

	// bypasses are the links of the last validated HTML page p was sent.
	bypasses bypassWhitelist

	// pendingClassChange is the class change the last bypass started, set
	// on p's queue while the connection goroutine waits in onLive and taken
	// by that goroutine once the wait returns; see finishPendingClassChange.
	pendingClassChange *classChange
	// subclassReuseUntil ends the reuse delay of the subclass actions. Set
	// and read on p's queue.
	subclassReuseUntil time.Time

	// shownMultisell is the multisell list p was last shown, the one its
	// exchanges choose from, or nil. Set and read on p's queue.
	shownMultisell atomic.Pointer[multisell.List]
	// storage is p's warehouse and freight state, set at attach and owned
	// by p's queue from then on.
	storage playerStorage

	// fusionTargetID is the object id of the target this player's active
	// fusion channel holds, or 0; cleared only by the channel that set it.
	fusionTargetID atomic.Int32

	// petSightings counts this player's Discover and Forget of their own
	// summon, which can run on another actor's queue. A PetItemList posted by
	// one Discover is dropped once a later one has run.
	petSightings atomic.Uint32

	// pvpChanges holds the PvP flag changes waiting to run on this
	// player's queue, in arrival order: a PK kill's side effects and its
	// summon's flag requests, both of which can arrive from another
	// actor's queue, and the frames held until they have run.
	pvpChanges pendingPvPChanges

	// teleportMu serializes a teleport from its start through the position
	// update with the Appearing that completes it. A summon-friend cast
	// teleports this player from the caster's queue, while Appearing runs on
	// this player's own.
	teleportMu sync.Mutex

	// interactMu is taken from another actor's queue for the same reason
	// as pickupMu.
	interactMu sync.Mutex
	interact   interactTarget

	// cubicsMu is taken from another actor's queue: another player's cubic
	// skill grants p a cubic, or evicts one, from the caster's queue
	// (syncCubicTargets).
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

// RestoringSummon reports whether a summon cast that already hit is still
// waiting for its pets-row read.
func (p *livePlayer) RestoringSummon() bool { return p.petRestoreInFlight.Load() }

// AccountName returns the owning account of this in-world player, used to
// report the online roster and per-account entries to the login server.
func (p *livePlayer) AccountName() string { return p.Character.AccountName }

type pickupIntention struct {
	ctx    context.Context
	target world.Tracked
	// shift is only meaningful on deferredPickup: it is the original click's
	// shift-modifier, needed at drain time to decide walk-vs-fail exactly as
	// a fresh click would (only a non-shift click walks). pickup (the
	// in-flight walk-then-collect intention) is only ever
	// set for a non-shift click — a shift click fails outright instead of
	// walking — so it never needs this field.
	shift bool
}

// deferredMagicSkill is a skill request waiting as the next CAST intention.
// selected is the target the request was made on: the queued cast resolves
// against it, not against whatever is selected once it runs.
type deferredMagicSkill struct {
	req      clientpackets.RequestMagicSkillUse
	selected world.Tracked
}

type itemAICastIntention struct {
	inventory *itemcontainer.Inventory
	item      *item.Instance
	skill     modelskill.Definition
	selected  world.Tracked
	// ctrl is the UseItem Ctrl modifier, the cast's force-use flag.
	ctrl bool
	// summon marks a pet collar's SUMMON_CREATURE: item is the collar the
	// pet comes from, kept rather than consumed.
	summon bool
}

func (p *livePlayer) sendVisibilityFrame(frame wire.Frame) bool {
	if p.visibilitySend == nil || (p.Character != nil && p.SessionDetached()) {
		frame.Release()
		return false
	}
	if p.pvpChanges.hold(frame, true) {
		return true
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

// Stop aborts everything p is doing, as abortAll does, then ends its attack
// stance with AutoAttackStop to its observers. Only detach uses it: a
// teleport aborts the same actions but keeps the stance (abortAll).
func (p *livePlayer) Stop() {
	p.abortAll(false)
	if p.stopAttack != nil {
		p.stopAttack(p)
	}
}

// abortAll drops p's queued intentions and stops its attack, cast and move.
// The attack stance and in-combat state are left alone: they end only on
// their own expiry, on death or on logout.
//
// A teleport has already left p unable to act. Its attack stop is answered
// with two ActionFailed, the refused idle's and the stop's own, and a cast
// in flight is stopped with the intention queued behind it still in place:
// the stopped cast's end thinks it once, refused, before the teleport drops
// it with everything else.
func (p *livePlayer) abortAll(teleport bool) {
	queuedForCast := teleport && p.cast != nil && p.cast.CastingNow()
	if queuedForCast {
		if p.move != nil {
			p.move.Stop()
		}
	} else {
		p.dropQueuedIntentions()
	}
	if p.attack != nil {
		p.attack.Stop()
	}
	if teleport {
		p.SendFrame(serverpackets.FrameActionFailed())
		p.SendFrame(serverpackets.FrameActionFailed())
	}
	// Stopping the cast cancels its pending task, so an in-flight cast never
	// lands against an already-detached or relocated character.
	if p.cast != nil {
		p.cast.Stop()
	}
	if queuedForCast {
		p.goIdle()
	}
	// Free the chair for others but keep seated identity so observers still
	// receive the stand-then-delete animation when this player despawns.
	p.freeChair()
	// Cubics are left alone: across a teleport they keep acting and ageing.
	// Detach stops them (stopCubics); death removes them (removeAllCubics).
}

// dropQueuedIntentions drops every intention p holds, active and queued,
// and stops its attack intention and any walk. A skill or item cast queued
// behind the cast in flight is left for the stop's own CastFinished, which
// drops it with ActionFailed; one queued behind anything else goes
// silently.
func (p *livePlayer) dropQueuedIntentions() {
	p.dropHeldIntention()
	p.takePickup()
	p.takeDeferredPickup()
	if p.cast == nil || !p.cast.CastingNow() {
		p.takeDeferredMagicSkill()
		p.takeDeferredItemAICast()
	}
	p.takeDeferredFollow()
	p.takeDeferredUseItem()
	p.takeDeferredAction()
	p.takeDeferredInteract()
	p.takeInteract()
	if p.combat != nil {
		p.combat.Stop()
	}
}

// stopCastInFlight stops p's cast as Character.StopCast does and reports
// whether a cast was in flight to stop.
func (p *livePlayer) stopCastInFlight() bool {
	if p.cast == nil {
		p.StopCast()
		return false
	}
	return p.cast.StopInFlight()
}

// detached reports whether p's session has begun detaching (logout).
func (p *livePlayer) detached() bool {
	return p.deliveryStopped.Load()
}

// Departed reports whether p has begun leaving the world, for the party
// registry: detach marks it before taking p out of its party.
func (p *livePlayer) Departed() bool {
	return p.detached()
}

// markDetaching runs on p's queue, where autosave and shadow-item expiry
// check detached, so neither enqueues after detach's own writes (#1948).
func (p *livePlayer) markDetaching() {
	sim.AssertOwner(p.Queue())
	p.deliveryStopped.Store(true)
}

// stopCubics cancels every live cubic runtime's timers on detach, so a
// recurring action tick never fires against a session that has already
// logged out. Stopping immediately here is equivalent to the action tick's
// own dead/online self-check on its next scheduled run, and avoids a stale
// timer outliving the session. Only detach calls it: a teleport keeps every
// cubic running.
func (p *livePlayer) stopCubics() {
	p.cubicsMu.Lock()
	defer p.cubicsMu.Unlock()
	for _, r := range p.cubics {
		r.Stop()
	}
}

// clearNextIntentionLocked drops whatever is queued as the next intention:
// the player has a single next-intention slot, so queuing one replaces the
// others. The caller holds pickupMu.
func (p *livePlayer) clearNextIntentionLocked() {
	p.deferredPickup = nil
	p.deferredMagic = nil
	p.deferredItem = nil
	p.deferredFollow = nil
	p.deferredUseItem = nil
	p.deferredAction = nil
	p.deferredInteract = nil
}

// clearNextIntention drops whatever is queued as the next intention.
func (p *livePlayer) clearNextIntention() {
	p.pickupMu.Lock()
	defer p.pickupMu.Unlock()
	p.clearNextIntentionLocked()
}

func (p *livePlayer) setPickup(ctx context.Context, target world.Tracked) {
	p.pickupMu.Lock()
	defer p.pickupMu.Unlock()
	p.clearNextIntentionLocked()
	p.pickup = &pickupIntention{ctx: ctx, target: target}
}

func (p *livePlayer) takePickup() *pickupIntention {
	p.pickupMu.Lock()
	defer p.pickupMu.Unlock()
	pickup := p.pickup
	p.pickup = nil
	return pickup
}

// hasPickup reports whether a walk toward a ground item holds the pickup
// intention.
func (p *livePlayer) hasPickup() bool {
	p.pickupMu.Lock()
	defer p.pickupMu.Unlock()
	return p.pickup != nil
}

func (p *livePlayer) deferPickup(ctx context.Context, target world.Tracked, shift bool) {
	p.pickupMu.Lock()
	defer p.pickupMu.Unlock()
	p.clearNextIntentionLocked()
	p.deferredPickup = &pickupIntention{ctx: ctx, target: target, shift: shift}
}

func (p *livePlayer) takeDeferredPickup() *pickupIntention {
	p.pickupMu.Lock()
	defer p.pickupMu.Unlock()
	pickup := p.deferredPickup
	p.deferredPickup = nil
	return pickup
}

// deferMagicSkill stores req as the next CAST intention, against the
// selection it was requested on, replacing whatever was queued before.
func (p *livePlayer) deferMagicSkill(req clientpackets.RequestMagicSkillUse, selected world.Tracked) {
	p.pickupMu.Lock()
	defer p.pickupMu.Unlock()
	p.clearNextIntentionLocked()
	p.deferredMagic = &deferredMagicSkill{req: req, selected: selected}
}

func (p *livePlayer) takeDeferredMagicSkill() *deferredMagicSkill {
	p.pickupMu.Lock()
	defer p.pickupMu.Unlock()
	req := p.deferredMagic
	p.deferredMagic = nil
	return req
}

// deferredMagicSkillID reports the skill id of the request queued as the
// next CAST intention, if any.
func (p *livePlayer) deferredMagicSkillID() (int32, bool) {
	p.pickupMu.Lock()
	defer p.pickupMu.Unlock()
	if p.deferredMagic == nil {
		return 0, false
	}
	return p.deferredMagic.req.SkillID, true
}

// hasDeferredMagicSkill reports whether a skill request is queued as the
// next CAST intention.
func (p *livePlayer) hasDeferredMagicSkill() bool {
	p.pickupMu.Lock()
	defer p.pickupMu.Unlock()
	return p.deferredMagic != nil
}

// hasDeferredItemAICast reports whether an item cast is queued as the next
// CAST intention.
func (p *livePlayer) hasDeferredItemAICast() bool {
	p.pickupMu.Lock()
	defer p.pickupMu.Unlock()
	return p.deferredItem != nil
}

// dropDeferredCast drops the skill request or item cast queued as the next
// CAST intention, reporting whether one was queued.
func (p *livePlayer) dropDeferredCast() bool {
	p.pickupMu.Lock()
	defer p.pickupMu.Unlock()
	queued := p.deferredMagic != nil || p.deferredItem != nil
	p.deferredMagic, p.deferredItem = nil, nil
	return queued
}

func (p *livePlayer) deferItemAICast(inventory *itemcontainer.Inventory, inst *item.Instance, skill modelskill.Definition, selected world.Tracked, ctrl bool) {
	p.setDeferredItemAICast(&itemAICastIntention{inventory: inventory, item: inst, skill: skill, selected: selected, ctrl: ctrl})
}

// deferSummonCreatureCast stores the SUMMON_CREATURE cast of the pet collar
// collar as the next CAST intention, cast on the player itself.
func (p *livePlayer) deferSummonCreatureCast(inventory *itemcontainer.Inventory, collar *item.Instance, skill modelskill.Definition) {
	p.setDeferredItemAICast(&itemAICastIntention{inventory: inventory, item: collar, skill: skill, selected: p.Character, summon: true})
}

func (p *livePlayer) setDeferredItemAICast(itemCast *itemAICastIntention) {
	p.pickupMu.Lock()
	defer p.pickupMu.Unlock()
	p.clearNextIntentionLocked()
	p.deferredItem = itemCast
}

func (p *livePlayer) takeDeferredItemAICast() *itemAICastIntention {
	p.pickupMu.Lock()
	defer p.pickupMu.Unlock()
	itemCast := p.deferredItem
	p.deferredItem = nil
	return itemCast
}

// deferFollow stores following target as the next intention, replacing
// whatever was queued before.
func (p *livePlayer) deferFollow(target attackable.Combatant, shift bool) {
	p.pickupMu.Lock()
	defer p.pickupMu.Unlock()
	p.clearNextIntentionLocked()
	p.deferredFollow = &followIntention{target: target, shift: shift}
}

func (p *livePlayer) takeDeferredFollow() *followIntention {
	p.pickupMu.Lock()
	defer p.pickupMu.Unlock()
	follow := p.deferredFollow
	p.deferredFollow = nil
	return follow
}

// hasDeferredFollow reports whether a follow is queued as the next
// intention.
func (p *livePlayer) hasDeferredFollow() bool {
	p.pickupMu.Lock()
	defer p.pickupMu.Unlock()
	return p.deferredFollow != nil
}

// deferUseItem stores toggling the item objectID as the next intention,
// replacing whatever was queued before.
func (p *livePlayer) deferUseItem(objectID int32) {
	p.pickupMu.Lock()
	defer p.pickupMu.Unlock()
	p.clearNextIntentionLocked()
	p.deferredUseItem = &useItemIntention{objectID: objectID}
}

func (p *livePlayer) takeDeferredUseItem() *useItemIntention {
	p.pickupMu.Lock()
	defer p.pickupMu.Unlock()
	useItem := p.deferredUseItem
	p.deferredUseItem = nil
	return useItem
}

// hasDeferredUseItem reports whether an equip toggle is queued as the next
// intention.
func (p *livePlayer) hasDeferredUseItem() bool {
	p.pickupMu.Lock()
	defer p.pickupMu.Unlock()
	return p.deferredUseItem != nil
}

// deferAction replaces the next player intention. It is drained by the
// player's queue when the swing, cast, or posture transition ends.
func (p *livePlayer) deferAction(run func()) {
	p.pickupMu.Lock()
	defer p.pickupMu.Unlock()
	p.clearNextIntentionLocked()
	p.deferredAction = run
}

func (p *livePlayer) takeDeferredAction() func() {
	p.pickupMu.Lock()
	defer p.pickupMu.Unlock()
	run := p.deferredAction
	p.deferredAction = nil
	return run
}

func (p *livePlayer) hasDeferredAction() bool {
	p.pickupMu.Lock()
	defer p.pickupMu.Unlock()
	return p.deferredAction != nil
}

// deferInteract stores an interact with target as the next intention,
// replacing whatever was queued before.
func (p *livePlayer) deferInteract(target interactTarget, shift bool) {
	p.pickupMu.Lock()
	defer p.pickupMu.Unlock()
	p.clearNextIntentionLocked()
	p.deferredInteract = &interactIntention{target: target, shift: shift}
}

func (p *livePlayer) takeDeferredInteract() *interactIntention {
	p.pickupMu.Lock()
	defer p.pickupMu.Unlock()
	interact := p.deferredInteract
	p.deferredInteract = nil
	return interact
}

// hasDeferredInteract reports whether an interact is queued as the next
// intention.
func (p *livePlayer) hasDeferredInteract() bool {
	p.pickupMu.Lock()
	defer p.pickupMu.Unlock()
	return p.deferredInteract != nil
}

func (p *livePlayer) setInteract(target interactTarget) {
	p.interactMu.Lock()
	defer p.interactMu.Unlock()
	p.interact = target
}

// hasInteract reports whether a walk toward an interact target holds the
// interact intention.
func (p *livePlayer) hasInteract() bool {
	p.interactMu.Lock()
	defer p.interactMu.Unlock()
	return p.interact != nil
}

func (p *livePlayer) takeInteract() interactTarget {
	p.interactMu.Lock()
	defer p.interactMu.Unlock()
	target := p.interact
	p.interact = nil
	return target
}

// thinkAttack re-thinks p's attack intention from a movement-arrived,
// swing-finished, cast-finished or wake-up hook, answering ActionFailed when
// the think sent the intention idle or found p still busy.
func (p *livePlayer) thinkAttack() {
	if p.combat != nil && p.combat.Think() {
		p.SendFrame(serverpackets.FrameActionFailed())
	}
}

// finishAttack re-thinks p's attack intention once a swing ends. With
// nothing queued behind the swing, a target p cannot keep attacking sends
// the intention idle silently; otherwise the think answers ActionFailed as
// thinkAttack does.
func (p *livePlayer) finishAttack() {
	if p.combat != nil && p.combat.FinishedAttack() {
		p.SendFrame(serverpackets.FrameActionFailed())
	}
}

// tryToIdle drops every intention p holds, active and queued, and stops its
// movement (combat.Stop stops the shared move controller, whatever the walk
// was for). A character that was already unable to act keeps its intentions
// and only answers ActionFailed. One still attacking, casting, sitting down
// or standing up answers ActionFailed too: its idle waits for the swing,
// cast or posture transition to end. Dropping the intentions now instead of
// after that end resumes nothing: every caller either stops the swing and
// cast next or leaves the character unable to act on the intention it would
// have resumed, and a transition end finds no queued cast to run.
func (p *livePlayer) tryToIdle(denied bool) {
	if denied {
		p.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	busy := p.CastingNow() || (p.attack != nil && p.attack.AttackingNow()) || inPostureTransition(p)
	p.goIdle()
	if busy {
		p.SendFrame(serverpackets.FrameActionFailed())
	}
}

// goIdle drops every intention p holds, active and queued, and stops its
// movement, answering nothing: the idle a refused stand takes, whose
// refusal sends its own ActionFailed.
func (p *livePlayer) goIdle() {
	p.dropHeldIntention()
	p.takePickup()
	p.takeDeferredPickup()
	p.takeDeferredMagicSkill()
	p.takeDeferredItemAICast()
	p.takeDeferredFollow()
	p.takeDeferredUseItem()
	p.takeDeferredAction()
	p.takeDeferredInteract()
	p.takeInteract()
	if p.combat != nil {
		p.combat.Stop()
	}
}

// clearParkedApproaches drops pickup, pet-interact, and deferred-magic and
// item-cast approach slots, the follow intention, current or queued, a
// queued pickup, equip toggle or summon interact, and a held walk or equip
// toggle, so a later walk or chase cannot inherit them.
func (p *livePlayer) clearParkedApproaches() {
	p.dropHeldIntention()
	p.takePickup()
	p.takeInteract()
	p.takeDeferredPickup()
	p.takeDeferredMagicSkill()
	p.takeDeferredItemAICast()
	p.takeDeferredFollow()
	p.takeDeferredUseItem()
	p.takeDeferredAction()
	p.takeDeferredInteract()
	p.endFollow()
}

// replaceIntention drops p's attack and follow intentions, offensive or
// friendly, for a new intention taking their place, leaving a walk under
// way running.
func (p *livePlayer) replaceIntention() {
	if p.combat != nil {
		p.combat.Replace()
	}
	if p.move != nil {
		p.move.CancelFollow()
	}
}

// endFollow drops p's follow intention, leaving a walk under way running.
func (p *livePlayer) endFollow() {
	if p.move != nil {
		p.move.CancelFriendlyFollow()
	}
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
// Paralyzed and pickupLocked are cleared together under pickupMu, the
// section livePickupBlockedDeferrable reads both in, so a click never sees
// one cleared without the other.
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

// castController returns live's cast controller, building it on first use
// with live as the sink its abort, stop and finish events reach.
func (l *GameClientLink) castController(live *livePlayer) *actorcast.Controller {
	if live.cast == nil {
		actor := actorcast.PlayerActor{Character: live.Character}
		if live.attack != nil {
			actor.Attack = live.attack
		}
		live.cast = actorcast.NewController(actor, live)
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
// Building the item list clears the update list before it reads the item
// set, so a full snapshot supersedes and discards the deltas
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
