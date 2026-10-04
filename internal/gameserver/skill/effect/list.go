package effect

import (
	"sync"
	"sync/atomic"
	"time"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/conditions"
)

// StatOwner is what a List is attached to: the holder its stat modifiers
// land on, the client icons its changes refresh and the recipient of its
// expiry messages. Kinds without client icons or messages implement those as
// no-ops.
type StatOwner interface {
	// AttachStatFuncs attaches fns to the owner's stat calculators without
	// reporting the change. The list calls it under its own lock, so it must
	// not reach back into the list; StatFuncsAttached follows once the lock
	// is released.
	AttachStatFuncs([]Mod)
	// StatFuncsAttached reports that fns were attached: the owner refreshes
	// whatever view follows those stats (move speed, appearance, status).
	// The list calls it without holding its lock, so the refresh may read
	// the list back.
	StatFuncsAttached([]Mod)
	RemoveStatsByOwner(owner ModOwner)
	// MaxBuffCount is the number of non-toggle, non-seven-signs buffs the
	// owner can hold at once (base slot count plus any bonus the owner
	// grants, e.g. from a known passive).
	MaxBuffCount() int

	// UpdateEffectIcons refreshes the owner's effect icons after an add or
	// remove attempt.
	UpdateEffectIcons()
	// NotifyEffectWornOff, NotifyEffectDisappeared and NotifyEffectAborted
	// send the system message that accompanies an icon effect leaving the
	// list.
	NotifyEffectWornOff(skillID modelskill.ID, level int)
	NotifyEffectDisappeared(skillID modelskill.ID, level int)
	NotifyEffectAborted(skillID modelskill.ID, level int)
	// NotifyEffectFelt sends the message that accompanies an icon effect
	// newly taking over its stack group when it is added.
	NotifyEffectFelt(skillID modelskill.ID, level int)
}

// Option changes List behavior.
type Option func(*List)

// ActivityRegistry records whether a list has effects to tick.
type ActivityRegistry interface {
	SetActive(*List, bool)
}

// Env is one server's context for its effect lists: the ticker that drives
// them and the gameplay settings they read. The composition root builds it
// once from config. Apart from Activity, the zero value is the shipped
// default: CancelLesserEffect on, always day.
type Env struct {
	// Activity is the ticker that runs periodic actions and expiry. A list
	// built without one is never ticked, so its effects never expire, never
	// send their removal packets, and never recalculate stats; only tests
	// that drive List.Tick themselves may leave it nil. Production
	// constructors reject a nil Activity.
	Activity ActivityRegistry
	// KeepLesser is CancelLesserEffect=false: a newly stacked non-herb
	// effect leaves the lower-priority effect it displaces queued instead
	// of removing it.
	KeepLesser bool
	// Night reports the in-game time of day; nil is always day.
	Night conditions.NightSource
}

// WithEnv attaches a list to one server's Env.
func WithEnv(env Env) Option {
	return func(l *List) {
		l.activity = env.Activity
		l.cancelLesser = !env.KeepLesser
		l.night = env.Night
	}
}

// WithAdmission makes the list hold only the effects admit accepts. Any
// other effect added to it is dropped before it starts: no start hook, no
// stat change, no icon or message.
func WithAdmission(admit func(*Effect) bool) Option {
	return func(l *List) { l.admit = admit }
}

// IsNight reports whether it is night on the server that owns l.
func (l *List) IsNight() bool {
	return l != nil && l.night != nil && l.night.IsNight()
}

// Untrack unconditionally deregisters l from the process-wide activity
// registry, regardless of whether it currently holds an effect. Call it
// when l's owner leaves the world for good (logout, NPC decay, unsummon,
// signet expiry) so a list that still holds an effect doesn't keep ticking
// a detached actor forever. This mirrors the pre-registry behavior, where
// an actor leaving world.State silently dropped out of the tick scan: it
// does not run any effect's exit hook or otherwise touch buffs/debuffs,
// only stops future Tick calls from reaching this list.
//
// Untrack is terminal: a later Add or Remove never registers l again. A
// tick or skill task accepted onto the owner's queue before it closed still
// runs after Untrack, and without this a straggler that leaves l holding an
// effect would re-register a departed actor's list for the rest of uptime.
//
// Like notifyActivityTransition, it decides and applies under one hold of
// l.mu so it can't race a concurrent Add/Remove on the same list into
// re-registering it right after Untrack deregisters it, or vice versa.
func (l *List) Untrack() {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	l.untracked = true
	if !l.tracked {
		return
	}
	l.tracked = false
	if l.activity != nil {
		l.activity.SetActive(l, false)
	}
}

// HasHeld reports whether any effect has ever been placed in l, even one
// since removed. An owner whose list never held one has no icon list to
// refresh: an explicit icon refresh (a death, a class change, a party
// join, a summon's info window) sends nothing for it.
func (l *List) HasHeld() bool {
	if l == nil {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.held
}

// quietLocked reports whether the owner-message helpers leave their
// messages out: during an AddRestored's insertion (silent) or a restored
// effect's catch-up (catchingUp). Caller must hold l.mu.
func (l *List) quietLocked() bool {
	return l.silent || l.catchingUp.Load() > 0
}

// emptyLocked reports whether l currently holds no buff or debuff. Caller
// must hold l.mu.
func (l *List) emptyLocked() bool {
	return len(l.buffs) == 0 && len(l.debuffs) == 0
}

// List owns one creature's active buffs and debuffs. All methods are safe for
// concurrent use; mu guards buffs, debuffs, stacks, held, tracked, untracked, silent, exiting,
// dropHeld, and callbacks
// into owner. Other actors add and dispel effects synchronously from their own
// queues while the owner's effect tick runs.
type List struct {
	mu sync.Mutex

	owner        StatOwner
	activity     ActivityRegistry
	cancelLesser bool
	night        conditions.NightSource
	// admit, when set, decides which effects the list holds at all; see
	// WithAdmission.
	admit func(*Effect) bool

	buffs   []*Effect
	debuffs []*Effect
	stacks  map[string][]*Effect
	// held records that an effect has reached insertion at least once; see
	// HasHeld.
	held bool

	// visibleFlags mirrors flagsLocked for readers that must not take mu:
	// the info packets' abnormal-effect field is built on paths that can
	// already run under mu (a stat change broadcast from an activation).
	// Written under mu whenever buffs or debuffs change.
	visibleFlags atomic.Uint32

	// tracked records whether l is currently registered with the
	// activity registry, so notifyActivityTransition can
	// reconcile against l's own last-known state instead of a value a
	// caller captured before releasing mu — see notifyActivityTransition.
	tracked bool
	// untracked records that Untrack ran; l never registers again.
	untracked bool
	// silent is set only for the duration of an AddRestored's l.mu holds.
	// The owner-message helpers read it while queueing, so the messages
	// that insertion would queue are left out.
	silent bool
	// catchingUp counts the restored effects whose due ticks catchUp is
	// running. While it is set, removals send no system messages, as
	// under silent, and no icon refresh: the replay sends one when it is
	// done. Set and cleared on the owner's queue; atomic for any reader.
	catchingUp atomic.Int32

	// exiting collects, during one add's l.mu hold, the held effects whose
	// exit hook that insertion queued (a replaced, evicted or displaced
	// buff). dropHeld maps each of them, until that Add's hooks have run, to
	// whether its exit hook asked Drop for it: such a drop waits for the
	// insertion to finish.
	exiting  []*Effect
	dropHeld map[*Effect]bool

	// queue is the owner's queue: periodic effect actions run on it and
	// effect periods are measured on its clock. Set before the owner is
	// published, and moved only with the owner's own work (a pet corpse
	// relinked to its returning owner's session); the effects sweep reads it
	// from the scheduler goroutine, hence atomic.
	queue atomic.Pointer[sim.Queue]

	// triggers are the started chance-skill-trigger effects, in start order.
	// triggersMu guards them apart from mu: effect start and exit hooks
	// register them, and another actor's queue reads them when it hits or
	// casts on the owner.
	triggersMu sync.Mutex
	triggers   []*Effect
}

func (l *List) now() time.Time { return l.queue.Load().Now() }

// SetQueue makes q, the owner's queue, the queue this list's periodic
// actions run on and the clock its effect periods are measured on. Every
// list needs one before it holds an effect. Move a published list only
// while none of its work can run on the queue it leaves (that queue is
// owned by the mover, or closed), so no tick of it runs beside the move.
func (l *List) SetQueue(q *sim.Queue) { l.queue.Store(q) }

// Queue returns the queue SetQueue installed last.
func (l *List) Queue() *sim.Queue { return l.queue.Load() }

// NewList returns an empty effect list.
func NewList(owner StatOwner, opts ...Option) *List {
	l := &List{owner: owner, cancelLesser: true}
	for _, opt := range opts {
		opt(l)
	}
	return l
}

// Flags returns the union of every visible held effect's flag bits — the
// active member of each stack group. Stacked-but-inactive members live only
// in the stacks map and contribute nothing here, although the specified
// flag union ORs every held buff and debuff, stacked-but-not-in-use ones
// included. The difference is
// unreachable for the crowd-control flags that consume this surface: the
// kinds carrying them either reject same-flag re-application
// (rejectsIfAffected) or activate the next stack member the instant the
// active one leaves, so the union stays continuous either way. It is
// recomputed from the current buffs and debuffs on every call rather than
// cached, matching how rarely a caller needs it compared to how often the
// list itself changes.
func (l *List) Flags() Flag {
	if l == nil {
		return 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.flagsLocked()
}

// flagsLocked is Flags' body for callers that already hold l.mu (e.g. add,
// which cannot call the exported Flags/IsAffected without self-deadlocking
// on the non-reentrant mutex).
func (l *List) flagsLocked() Flag {
	var flags Flag
	for _, e := range l.buffs {
		if e != nil {
			flags |= e.Flag
		}
	}
	for _, e := range l.debuffs {
		if e != nil {
			flags |= e.Flag
		}
	}
	return flags
}

// publishFlagsLocked refreshes visibleFlags after buffs or debuffs
// changed. Caller must hold l.mu.
func (l *List) publishFlagsLocked() { l.visibleFlags.Store(uint32(l.flagsLocked())) }

// IsAffected reports whether any bit of flag is set in l.Flags().
func (l *List) IsAffected(flag Flag) bool {
	return l.Flags()&flag != 0
}

// crowdControlVisuals pairs crowd-control flags with the client
// abnormal-visual bit (the datapack abnormal name in the comment) a creature
// shows while any of them is set.
var crowdControlVisuals = [...]struct {
	flags Flag
	mask  int
}{
	{FlagStunned, 0x000040},                   // stun
	{FlagRooted, 0x000200},                    // root
	{FlagSleep, 0x000080},                     // sleep
	{FlagConfused | FlagFear, 0x000020},       // fear
	{FlagMuted | FlagPhysicalMuted, 0x000100}, // mute
	{FlagMeditating, 0x020000},                // floatroot
}

// CrowdControlAbnormalEffect returns the abnormal-visual bits derived from
// the crowd-control state l currently imposes. Info packets OR them into the
// actor's stored visual bitmask; they are never stored, so clearing a
// stored bit cannot clear them, and they end with the effect that set them.
// It reads visibleFlags, never mu, so it is safe on any path, including one
// that runs while l's own lock is held.
func (l *List) CrowdControlAbnormalEffect() int {
	if l == nil {
		return 0
	}
	flags := Flag(l.visibleFlags.Load())
	mask := 0
	for _, v := range crowdControlVisuals {
		if flags&v.flags != 0 {
			mask |= v.mask
		}
	}
	return mask
}

// AIDenyFlags are the effect flags that keep an actor from taking AI actions.
const AIDenyFlags = FlagStunned | FlagMeditating | FlagSleep | FlagParalyzed | FlagFear

// StartedAffected is IsAffected limited to effects whose on-start hook has
// completed. Called from inside an on-start hook, it answers for the state
// the actor was in before that effect landed: a hook must not see its own
// flag when deciding how an actor that was already disabled reacts.
func (l *List) StartedAffected(flag Flag) bool {
	if l == nil {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, group := range [][]*Effect{l.buffs, l.debuffs} {
		for _, e := range group {
			if e != nil && e.inUse && e.Flag&flag != 0 {
				return true
			}
		}
	}
	return false
}

// All returns a snapshot of effects ordered as buffs followed by debuffs.
func (l *List) All() []*Effect {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	effects := make([]*Effect, 0, len(l.buffs)+len(l.debuffs))
	effects = append(effects, l.buffs...)
	effects = append(effects, l.debuffs...)
	return effects
}

func (l *List) active() []*Effect {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	effects := make([]*Effect, 0, len(l.buffs)+len(l.debuffs))
	for _, e := range l.buffs {
		if e != nil && e.inUse {
			effects = append(effects, e)
		}
	}
	for _, e := range l.debuffs {
		if e != nil && e.inUse {
			effects = append(effects, e)
		}
	}
	return effects
}

// ActiveBySkillID returns the applied Level of the first effect owned by
// skill id that condition and seed/force lookups read, and whether any such
// effect exists. Despite the name it is not active-only: an in-use match
// wins, otherwise a held, stacked-out match is returned. Buffs
// are searched first and debuffs only when no buff matched at all, and
// among several held matches in one list the last one wins.
func (l *List) ActiveBySkillID(id int) (level int, ok bool) {
	if l == nil {
		return 0, false
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	var held *Effect
	for _, effects := range [2][]*Effect{l.buffs, l.debuffs} {
		for _, e := range effects {
			if e == nil || int(e.Skill.ID) != id {
				continue
			}
			if e.inUse {
				return e.Level, true
			}
			held = e
		}
		if held != nil {
			return held.Level, true
		}
	}
	return 0, false
}

// DanceCount returns the number of active dance/song effects. It drives the
// per-cast MP surcharge a
// dance/song skill pays for each already-running dance/song: casting
// another one gets more expensive as more stay active simultaneously.
func (l *List) DanceCount() int {
	count := 0
	for _, e := range l.active() {
		if e.Skill.Dance {
			count++
		}
	}
	return count
}

// Tick runs periodic actions due at the current time and removes effects
// whose action hook stops or whose configured tick count is exhausted.
