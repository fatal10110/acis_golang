package script

import (
	"runtime/debug"

	"github.com/fatal10110/acis_golang/internal/gameserver/castle"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/rs/zerolog"
)

// Maker is a spawn maker built in Go: the hooks an npcmaker of its type
// reacts with. A maker derives from its parent's hooks with
// MakerHooks.With; calling the parent is calling the captured parent
// MakerHooks. Every npcmaker gets a value of its own.
type Maker struct {
	MakerHooks
	world MakerWorld
	// run raises the maker's hooks; set by the registry.
	run *makerRun
}

// IsNight reports whether it is night in the game world (00:00 to 05:59
// game time).
func (m *Maker) IsNight() bool { return m.world.Night() }

// MakerHooks holds a maker's handlers, one func field per hook; a nil
// field does not react. The field list, the invokers and With are kept by
// hand and checked against each other by a test.
type MakerHooks struct {
	OnStart       func(*Maker, MakerStart)
	OnNPCCreated  func(*Maker, MakerNPC)
	OnNPCDeleted  func(*Maker, MakerNPC)
	OnNPCDBInfo   func(*Maker, MakerSpawn)
	OnScriptEvent func(*Maker, MakerEvent)
	// OnDoorEvent runs for each opening or closing of a door the maker
	// registered on with Door.AddMakerEvent.
	OnDoorEvent func(*Maker, MakerDoor)
	// OnSiegeEvent runs for each status change of a siege the maker
	// registered on with Castle.AddMakerEvent.
	OnSiegeEvent func(*Maker, MakerSiege)
	// OnTimer is never raised by the engine: a maker schedules its own
	// timers with the group's After or Every and calls it from there.
	OnTimer func(*Maker, MakerTimer)
}

// MakerStart: the group starts.
type MakerStart struct {
	Group spawn.Group
}

// MakerNPC: NPC of Spawn entered or left the world.
type MakerNPC struct {
	Group spawn.Group
	Spawn spawn.GroupSpawn
	NPC   spawn.GroupNPC
}

// MakerSpawn: the database-tracked Spawn is loaded.
type MakerSpawn struct {
	Group spawn.Group
	Spawn spawn.GroupSpawn
}

// MakerEvent: the maker script event Name, with its two arguments.
type MakerEvent struct {
	Group      spawn.Group
	Name       string
	Int1, Int2 int
}

// MakerSiege: the siege of Castle, which Group's maker registered on,
// changed to Status.
type MakerSiege struct {
	Group  spawn.Group
	Castle *Castle
	Status SiegeStatus
}

// MakerDoor: Door, which Group's maker registered on, opened or closed.
type MakerDoor struct {
	Group spawn.Group
	Door  *Door
}

// MakerTimer: the maker's timer Name came due.
type MakerTimer struct {
	Group spawn.Group
	Name  string
}

// With returns h with every hook o sets replaced by o's: a child maker
// overriding its parent's handlers.
func (h MakerHooks) With(o MakerHooks) MakerHooks {
	if o.OnStart != nil {
		h.OnStart = o.OnStart
	}
	if o.OnNPCCreated != nil {
		h.OnNPCCreated = o.OnNPCCreated
	}
	if o.OnNPCDeleted != nil {
		h.OnNPCDeleted = o.OnNPCDeleted
	}
	if o.OnNPCDBInfo != nil {
		h.OnNPCDBInfo = o.OnNPCDBInfo
	}
	if o.OnScriptEvent != nil {
		h.OnScriptEvent = o.OnScriptEvent
	}
	if o.OnSiegeEvent != nil {
		h.OnSiegeEvent = o.OnSiegeEvent
	}
	if o.OnDoorEvent != nil {
		h.OnDoorEvent = o.OnDoorEvent
	}
	if o.OnTimer != nil {
		h.OnTimer = o.OnTimer
	}
	return h
}

// The maker invokers run a hook when it is set and do nothing otherwise,
// so a parent call ports as one call whether or not an ancestor reacts.

func (h *MakerHooks) Start(m *Maker, e MakerStart) {
	if h.OnStart != nil {
		h.OnStart(m, e)
	}
}

func (h *MakerHooks) NPCCreated(m *Maker, e MakerNPC) {
	if h.OnNPCCreated != nil {
		h.OnNPCCreated(m, e)
	}
}

func (h *MakerHooks) NPCDeleted(m *Maker, e MakerNPC) {
	if h.OnNPCDeleted != nil {
		h.OnNPCDeleted(m, e)
	}
}

func (h *MakerHooks) NPCDBInfo(m *Maker, e MakerSpawn) {
	if h.OnNPCDBInfo != nil {
		h.OnNPCDBInfo(m, e)
	}
}

func (h *MakerHooks) ScriptEvent(m *Maker, e MakerEvent) {
	if h.OnScriptEvent != nil {
		h.OnScriptEvent(m, e)
	}
}

func (h *MakerHooks) SiegeEvent(m *Maker, e MakerSiege) {
	if h.OnSiegeEvent != nil {
		h.OnSiegeEvent(m, e)
	}
}

func (h *MakerHooks) DoorEvent(m *Maker, e MakerDoor) {
	if h.OnDoorEvent != nil {
		h.OnDoorEvent(m, e)
	}
}

func (h *MakerHooks) Timer(m *Maker, e MakerTimer) {
	if h.OnTimer != nil {
		h.OnTimer(m, e)
	}
}

// MakerCatalog maps a spawnlist maker type, the <ai type> of an npcmaker,
// to the constructor of its maker.
type MakerCatalog map[string]func() Maker

// Makers is the maker registry: the catalog and the maker every other
// type gets. It never changes after NewMakers.
type Makers struct {
	catalog  MakerCatalog
	fallback func() Maker
	world    MakerWorld
	log      zerolog.Logger
}

// MakerWorld is what the makers read of the world.
type MakerWorld struct {
	// Night tells whether it is night in the game world.
	Night func() bool
	// Castles finds a castle by id; nil finds none.
	Castles CastlesByID
	// Sieges are the castles' sieges; nil finds none.
	Sieges Sieges
	// Doors are the doors of the world; nil finds none.
	Doors Doors
}

// CastlesByID finds a castle by its id.
type CastlesByID interface {
	Get(id int) (*castle.Castle, bool)
}

// NewMakers returns the registry of catalog, giving every type the catalog
// lacks the maker fallback builds, its makers reading world.
func NewMakers(catalog MakerCatalog, fallback func() Maker, world MakerWorld, log zerolog.Logger) *Makers {
	return &Makers{catalog: catalog, fallback: fallback, world: world, log: log}
}

// Registered reports whether aiType has a maker of its own; any other type
// runs the fallback.
func (r *Makers) Registered(aiType string) bool {
	_, ok := r.catalog[aiType]
	return ok
}

// New builds the maker of one npcmaker of type aiType.
func (r *Makers) New(aiType string) spawn.Behavior {
	ctor, ok := r.catalog[aiType]
	if !ok {
		ctor = r.fallback
	}
	m := ctor()
	m.world = r.world
	run := &makerRun{m: &m, aiType: aiType, log: r.log}
	m.run = run
	return run
}

// makerRun raises one npcmaker's maker hooks, each invocation recovering
// a panic.
type makerRun struct {
	m      *Maker
	aiType string
	log    zerolog.Logger
}

func (r *makerRun) Start(g spawn.Group) {
	r.run(g, "onStart", func() { r.m.MakerHooks.Start(r.m, MakerStart{Group: g}) })
}

func (r *makerRun) NPCCreated(g spawn.Group, s spawn.GroupSpawn, npc spawn.GroupNPC) {
	r.run(g, "onNpcCreated", func() { r.m.MakerHooks.NPCCreated(r.m, MakerNPC{Group: g, Spawn: s, NPC: npc}) })
}

func (r *makerRun) NPCDeleted(g spawn.Group, s spawn.GroupSpawn, npc spawn.GroupNPC) {
	r.run(g, "onNpcDeleted", func() { r.m.MakerHooks.NPCDeleted(r.m, MakerNPC{Group: g, Spawn: s, NPC: npc}) })
}

func (r *makerRun) NPCDBInfo(g spawn.Group, s spawn.GroupSpawn) {
	r.run(g, "onNpcDBInfo", func() { r.m.MakerHooks.NPCDBInfo(r.m, MakerSpawn{Group: g, Spawn: s}) })
}

func (r *makerRun) ScriptEvent(g spawn.Group, name string, int1, int2 int) {
	r.run(g, "onMakerScriptEvent", func() {
		r.m.MakerHooks.ScriptEvent(r.m, MakerEvent{Group: g, Name: name, Int1: int1, Int2: int2})
	})
}

func (r *makerRun) SiegeEvent(g spawn.Group, c *Castle, st SiegeStatus) {
	r.run(g, "onSiegeEvent", func() {
		r.m.MakerHooks.SiegeEvent(r.m, MakerSiege{Group: g, Castle: c, Status: st})
	})
}

func (r *makerRun) DoorEvent(g spawn.Group, d *Door) {
	r.run(g, "onDoorEvent", func() {
		r.m.MakerHooks.DoorEvent(r.m, MakerDoor{Group: g, Door: d})
	})
}

// run calls fn, one hook invocation, recovering and logging a panic with
// its stack; the next invocation runs as usual.
func (r *makerRun) run(g spawn.Group, hook string, fn func()) {
	defer func() {
		if p := recover(); p != nil {
			r.log.Error().Str("maker", g.Maker().Name).Str("type", r.aiType).Str("hook", hook).Interface("panic", p).Str("stack", string(debug.Stack())).Msg("script: maker hook panicked")
		}
	}()
	fn()
}
