package script

import (
	"fmt"
	"slices"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/rs/zerolog"
)

// NPCKind is the kind of live NPC a template spawns as, which decides
// whether the engine raises a hook on it.
type NPCKind uint8

// The NPC kinds.
const (
	// KindOther is a template the world never spawns as a scripted NPC.
	KindOther NPCKind = iota
	KindFolk
	KindHostile
)

func (k NPCKind) String() string {
	switch k {
	case KindFolk:
		return "folk"
	case KindHostile:
		return "hostile"
	default:
		return "other"
	}
}

// kindSet is a set of NPC kinds, one bit per kind.
type kindSet uint8

func kinds(ks ...NPCKind) kindSet {
	var s kindSet
	for _, k := range ks {
		s |= 1 << k
	}
	return s
}

// raisedHooks lists, per hook, the NPC kinds the engine raises it on; a
// hook that is not about an NPC (event, timer) is raised when its entry is
// non-zero. The change that adds a hook's raising site adds the hook here.
// Until then no script subscribing to it registers, so no script waits on a
// hook that never fires.
var raisedHooks = map[hook]kindSet{
	hookAttacked:      kinds(KindHostile),
	hookPartyAttacked: kinds(KindHostile),
	// The spawner and the NPC's death and decay (lifecycle.go).
	hookCreated: kinds(KindFolk, KindHostile),
	hookDecayed: kinds(KindFolk, KindHostile),
	hookMyDying: kinds(KindFolk, KindHostile),
	// The AI's idle and arrival (idle.go). A civilian NPC walks no point
	// and keeps no territory yet (#3492).
	hookNoDesire:       kinds(KindFolk, KindHostile),
	hookMoveToFinished: kinds(KindHostile),
	hookOutOfTerritory: kinds(KindHostile),
	// The schedule runner (StartSchedule).
	hookStart: kinds(KindOther),
	// The tutorial events (TutorialEvent) and the quest event links
	// (QuestEvent).
	hookEvent: kinds(KindOther),
	// The timer registry, on the kinds whose decay stops a behavior's
	// timers (HostileDecayed, FolkDecayed).
	hookTimer: kinds(KindOther, KindFolk, KindHostile),
	// The quest windows (QuestWindow), for both NPC kinds a dialog command
	// reaches.
	hookTalk: kinds(KindFolk, KindHostile),
	// The use of an item (ItemUsed) and the entry into a zone (ZoneEntered).
	hookItemUse:   kinds(KindOther),
	hookZoneEnter: kinds(KindOther),
	// The script-event sends (SendScriptEvent, BroadcastScriptEvent), to an
	// NPC of either kind.
	hookScriptEvent: kinds(KindFolk, KindHostile),
	// The interact of a civilian NPC and of a talking hostile one
	// (Interact).
	hookFirstTalk: kinds(KindFolk, KindHostile),
}

// Config is what Build needs besides the list and the catalog.
type Config struct {
	// KindOf returns the kind of the NPC template with id npcID, false when
	// no template has that id. A binding to such an id is skipped.
	KindOf func(npcID int32) (NPCKind, bool)
	Log    zerolog.Logger
	// Env is what the registered scripts' helpers act through.
	Env *Env
	// Queue is the engine queue. The dying hooks run on it, as they come
	// after the NPC's own queue may have closed at its decay, and so does a
	// script timer whose NPC and player have no queue to run it on.
	Queue *sim.Queue

	// raises overrides raisedHooks; tests use it.
	raises func(hook, NPCKind) bool
}

func (c *Config) raised(h hook, k NPCKind) bool {
	if c.raises != nil {
		return c.raises(h, k)
	}
	return raisedHooks[h]&kinds(k) != 0
}

// Registry is the set of registered scripts and their per-(NPC, event)
// lists. Build returns it complete; nothing changes it afterwards, so any
// goroutine reads it without locking.
type Registry struct {
	log zerolog.Logger
	// entries are the listed scripts, in list order.
	entries []entry
	// npc holds, per (NPC id, event), the scripts that answer it, in
	// dispatch order.
	npc map[npcKey][]*Script
	// byName holds, per lower-cased name, the first registered script of
	// that name in list order.
	byName map[string]*Script
	// behaves holds the NPC ids a behavior is bound to.
	behaves map[int32]bool
	// behaviors holds, per NPC id, the behaviors left on any of its event
	// lists.
	behaviors map[int32]map[*Script]bool
	// items holds, per item id, the scripts its use reaches, in list order.
	items map[int32][]*Script
	// zones holds, per zone id, the scripts its entry reaches, in list
	// order.
	zones map[int32][]*Script
	// queue is the engine queue (Config.Queue).
	queue *sim.Queue
	// timers are the scripts' timers. They are not part of the registry:
	// a leaf-locked container of their own.
	timers *timers
}

type npcKey struct {
	npc   int32
	event NPCEvent
}

// entry is one scripts.xml entry and what became of it.
type entry struct {
	path string
	// script is nil when the entry was not registered; why is in state.
	script *Script
	state  entryState
	// bound are the bindings the script asked for, with ids that have no
	// NPC template left out.
	bound Bindings
	// sched is the schedule of a scheduled task, nil for any other script
	// and for a task its entry does not schedule.
	sched *schedule
}

type entryState uint8

const (
	entryRegistered entryState = iota
	entryMissing
	entryRefused
)

// Build registers, in list order, the script of every listed path the
// catalog has. A listed path with no script, or whose constructor panics,
// is logged at error level and skipped; a catalog entry that is not listed
// is never built. A script whose hooks the engine does not raise for the
// NPC kinds it is bound to is refused: logged and skipped.
//
// For each (NPC, event) a behavior first removes any other behavior and a
// script removes an earlier registration of itself; it is then appended,
// except that first talk keeps a single script. So the last listed behavior wins and other
// scripts accumulate in list order.
func Build(list []Listing, catalog Catalog, cfg Config) *Registry {
	r := &Registry{log: cfg.Log, npc: map[npcKey][]*Script{}, byName: map[string]*Script{}, behaves: map[int32]bool{}, behaviors: map[int32]map[*Script]bool{}, items: map[int32][]*Script{}, zones: map[int32][]*Script{}, queue: cfg.Queue}
	r.timers = newTimers(r, cfg.Queue)
	registered := 0
	for _, l := range list {
		e := entry{path: l.Path}
		ctor, ok := catalog[l.Path]
		var s Script
		if ok {
			s, ok = construct(ctor, r.log, l.Path)
		} else {
			r.log.Error().Str("script", l.Path).Msg("script: listed script has no Go script; skipped")
		}
		if !ok {
			e.state = entryMissing
			r.entries = append(r.entries, e)
			continue
		}
		s.path = l.Path
		s.env = cfg.Env
		s.timers = r.timers
		s.registry = r
		s.Name = l.Path[strings.LastIndexByte(l.Path, '.')+1:]
		e.bound = boundOf(&s, cfg.KindOf)
		if err := gate(&s, e.bound, &cfg); err != nil {
			r.log.Error().Err(err).Str("script", l.Path).Msg("script: refused; a hook it needs is not raised")
			e.state = entryRefused
			r.entries = append(r.entries, e)
			continue
		}
		e.script = &s
		e.sched = scheduleOf(&s, l, r.log)
		r.entries = append(r.entries, e)
		r.register(e.script, e.bound)
		if key := strings.ToLower(s.Name); r.byName[key] == nil {
			r.byName[key] = e.script
		}
		registered++
	}
	for k, list := range r.npc {
		for _, s := range list {
			if s.Behavior {
				if r.behaviors[k.npc] == nil {
					r.behaviors[k.npc] = map[*Script]bool{}
				}
				r.behaviors[k.npc][s] = true
			}
		}
	}
	r.log.Info().Int("listed", len(list)).Int("registered", registered).Msg("script: registry built")
	return r
}

// construct runs ctor, recovering a panic: the script is then missing.
func construct(ctor func() Script, log zerolog.Logger, path string) (s Script, ok bool) {
	defer func() {
		if p := recover(); p != nil {
			log.Error().Str("script", path).Interface("panic", p).Msg("script: constructor panicked; skipped")
			ok = false
		}
	}()
	return ctor(), true
}

// boundOf returns the bindings s asks for: its explicit ones, and for a
// behavior its own NPCs on every event its hooks bind it to. Ids with no
// NPC template are left out silently.
func boundOf(s *Script, kindOf func(int32) (NPCKind, bool)) Bindings {
	out := Bindings{}
	add := func(ev NPCEvent, ids []int32) {
		for _, id := range ids {
			if _, ok := kindOf(id); ok && !slices.Contains(out[ev], id) {
				out[ev] = append(out[ev], id)
			}
		}
	}
	for ev, ids := range s.Bind {
		add(ev, ids)
	}
	if s.Behavior {
		set := s.Hooks.set()
		for ev := range npcEventCount {
			if d := npcEvents[ev]; d.byHook && set.has(d.hook) {
				add(ev, s.NPCs)
			}
		}
	}
	for ev := range out {
		slices.Sort(out[ev])
	}
	return out
}

// gate checks that the engine raises every hook s subscribes to: for each
// binding, the event's hook on that NPC's kind, and the event and timer
// hooks when s sets them. A behavior's timers stop when its NPC decays, so
// a behavior that sets the timer hook needs it raised on the kind of every
// NPC it is bound to.
func gate(s *Script, bound Bindings, cfg *Config) error {
	for ev := range npcEventCount {
		h := npcEvents[ev].hook
		for _, id := range bound[ev] {
			k, _ := cfg.KindOf(id)
			if !cfg.raised(h, k) {
				return fmt.Errorf("%s on npc %d: hook %s is not raised for %s npcs", ev, id, h, k)
			}
		}
	}
	set := s.Hooks.set()
	if s.Behavior && set.has(hookTimer) {
		for ev := range npcEventCount {
			for _, id := range bound[ev] {
				if k, _ := cfg.KindOf(id); !cfg.raised(hookTimer, k) {
					return fmt.Errorf("behavior timers on npc %d: their removal at decay is not raised for %s npcs", id, k)
				}
			}
		}
	}
	for _, h := range []hook{hookEvent, hookStart, hookTimer, hookItemUse, hookZoneEnter} {
		if set.has(h) && !cfg.raised(h, KindOther) && !cfg.raised(h, KindFolk) && !cfg.raised(h, KindHostile) {
			return fmt.Errorf("hook %s is not raised", h)
		}
	}
	return nil
}

// scheduleOf returns the schedule l gives s: none unless s is a scheduled
// task (it has a start hook) and l names a schedule. A task whose entry has
// no start, a schedule kind the runner does not build or a start stamp
// that does not parse is logged and not scheduled; so is one whose end
// differs from its start, as no task reacts to its end. A schedule on any
// other script is ignored.
func scheduleOf(s *Script, l Listing, log zerolog.Logger) *schedule {
	if s.Hooks.OnStart == nil || l.Schedule == "" {
		return nil
	}
	if l.Start == "" {
		log.Warn().Str("script", l.Path).Msg("script: scheduled task has no start; not scheduled")
		return nil
	}
	sc, unknownDay, err := parseSchedule(l.Schedule, l.Start)
	if err == nil && l.End != "" {
		var end schedule
		if end, _, err = parseSchedule(l.Schedule, l.End); err == nil && end != sc {
			err = fmt.Errorf("end %q differs from start %q: no task reacts to its end", l.End, l.Start)
		}
	}
	if err != nil {
		log.Error().Err(err).Str("script", l.Path).Msg("script: bad schedule; not scheduled")
		return nil
	}
	if unknownDay != "" {
		log.Error().Str("script", l.Path).Str("day", unknownDay).Msg("script: unknown day of week in schedule; Monday is used")
	}
	return &sc
}

// register adds s to the list of every (NPC, event) it is bound to, of
// every item whose use it reacts to and of every zone whose entry it reacts
// to. An item keeps every registration, a zone keeps one per script.
func (r *Registry) register(s *Script, bound Bindings) {
	for _, id := range s.UsedItems {
		r.items[id] = append(r.items[id], s)
	}
	for _, id := range s.EnteredZones {
		list := slices.DeleteFunc(r.zones[id], func(o *Script) bool { return o.path == s.path })
		r.zones[id] = append(list, s)
	}
	for ev, ids := range bound {
		for _, id := range ids {
			k := npcKey{id, ev}
			r.npc[k] = registerOn(r.npc[k], s, ev)
			if s.Behavior {
				r.behaves[id] = true
			}
		}
	}
}

// registerOn returns list with s registered on it: a behavior removes any
// behavior, any other script an earlier copy of itself; then s is appended
// unless the event holds a single script and the list is still occupied.
func registerOn(list []*Script, s *Script, ev NPCEvent) []*Script {
	if i := slices.IndexFunc(list, func(o *Script) bool { return same(o, s) }); i >= 0 {
		list = slices.Delete(list, i, i+1)
	}
	if ev.single() && len(list) > 0 {
		return list
	}
	return append(list, s)
}

// same reports whether a and b count as one script for registration: two
// behaviors always, otherwise the same path.
func same(a, b *Script) bool {
	if a.Behavior && b.Behavior {
		return true
	}
	return a.path == b.path
}

// scripts returns the scripts that answer ev on the NPC with id npcID, in
// dispatch order; a nil registry has none. The slice is shared: callers
// never modify it.
func (r *Registry) scripts(npcID int32, ev NPCEvent) []*Script {
	if r == nil {
		return nil
	}
	return r.npc[npcKey{npcID, ev}]
}

// Behaves reports whether a behavior is bound to the NPC with id npcID, on
// any event; a nil registry binds none. Until one is, the NPC keeps its
// built-in stand-in reactions.
func (r *Registry) Behaves(npcID int32) bool {
	return r != nil && r.behaves[npcID]
}

// behaviorOn reports whether s is a behavior left on any event list of the
// NPC id npcID.
func (r *Registry) behaviorOn(s *Script, npcID int32) bool {
	return r.behaviors[npcID][s]
}

// JournalQuest returns the quest a journal row's quest name belongs to: the
// first registered script, in list order, whose name matches name ignoring
// case.
func (r *Registry) JournalQuest(name string) (questlog.Quest, bool) {
	s := r.byName[strings.ToLower(name)]
	if s == nil {
		return questlog.Quest{}, false
	}
	return questlog.Quest{Name: s.Name, ID: s.QuestID, Items: s.Items}, true
}
