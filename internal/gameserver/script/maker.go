package script

import (
	"runtime/debug"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/rs/zerolog"
)

// Maker is a spawn maker built in Go: the hooks an npcmaker of its type
// reacts with. A maker derives from its parent's hooks with
// MakerHooks.With; calling the parent is calling the captured parent
// MakerHooks. Every npcmaker gets a value of its own.
type Maker struct {
	MakerHooks
}

// MakerHooks holds a maker's handlers, one func field per hook; a nil
// field does not react. The field list, the invokers and With are kept by
// hand and checked against each other by a test.
type MakerHooks struct {
	OnStart       func(*Maker, MakerStart)
	OnNPCCreated  func(*Maker, MakerNPC)
	OnNPCDeleted  func(*Maker, MakerNPC)
	OnNPCDBInfo   func(*Maker, MakerSpawn)
	OnScriptEvent func(*Maker, MakerEvent)
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
	log      zerolog.Logger
}

// NewMakers returns the registry of catalog, giving every type the catalog
// lacks the maker fallback builds.
func NewMakers(catalog MakerCatalog, fallback func() Maker, log zerolog.Logger) *Makers {
	return &Makers{catalog: catalog, fallback: fallback, log: log}
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
	return &makerRun{m: &m, aiType: aiType, log: r.log}
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
