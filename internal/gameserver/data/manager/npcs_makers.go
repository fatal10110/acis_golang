package manager

import (
	"fmt"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

// MakerBehaviors builds the maker each npcmaker runs, by its maker type.
type MakerBehaviors interface {
	New(aiType string) spawn.Behavior
}

// SevenSigns is the Seven Signs state the ssq_* spawn groups follow.
type SevenSigns interface {
	CurrentPeriod() sevensigns.Period
	WinningCabal() sevensigns.Cabal
	SealOwners() [3]sevensigns.Cabal
}

// sevenSignsGroups are the Seven Signs spawn events, in the order a period
// change deletes them.
var sevenSignsGroups = [...]string{
	"ssq_seal1_none", "ssq_seal1_dawn", "ssq_seal1_twilight",
	"ssq_seal2_none", "ssq_seal2_dawn", "ssq_seal2_twilight",
	"ssq_event",
}

// makerGroup is one npcmaker of the spawn list in use, at run time
// (spawn.Group).
type makerGroup struct {
	n        *Npcs
	def      *spawn.Maker
	gen      int
	behavior spawn.Behavior
	// keys are, per entry, the slot keys the entry keeps, in creation
	// order; seq numbers, per entry, the slots it has created. Both are
	// guarded by n.mu.
	keys [][]string
	seq  []int
}

// newGroups builds the run-time npcmakers of spawns for RespawnAll run gen
// and puts them in place of the earlier ones.
func (n *Npcs) newGroups(spawns *Spawns, gen int) []*makerGroup {
	makers := spawns.Table().Makers()
	groups := make([]*makerGroup, 0, len(makers))
	for _, def := range makers {
		groups = append(groups, &makerGroup{
			n: n, def: def, gen: gen, behavior: n.makers.New(def.AIType),
			keys: make([][]string, len(def.Entries)), seq: make([]int, len(def.Entries)),
		})
	}
	n.mu.Lock()
	n.groups = groups
	n.mu.Unlock()
	return groups
}

// currentGroups returns the run-time npcmakers of the spawn list in use.
func (n *Npcs) currentGroups() []*makerGroup {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.groups
}

func (g *makerGroup) Maker() *spawn.Maker { return g.def }

func (g *makerGroup) Alive() int {
	g.n.mu.Lock()
	defer g.n.mu.Unlock()
	alive := 0
	for _, keys := range g.keys {
		for _, key := range keys {
			if g.n.liveLocked(key) {
				alive++
			}
		}
	}
	return alive
}

func (g *makerGroup) Held() bool { return g.n.held(g.def) }

func (g *makerGroup) Listed(event string) bool { return g.n.events.has(event) }

func (g *makerGroup) Spawns() []spawn.GroupSpawn {
	out := make([]spawn.GroupSpawn, len(g.def.Entries))
	for i := range out {
		out[i] = groupSpawn{g: g, i: i}
	}
	return out
}

func (g *makerGroup) SendEvent(maker, name string, int1, int2 int) {
	g.n.MakerEvent(maker, name, int1, int2)
}

// After runs fn on the makers' queue once d has passed. Each spawn fn
// makes read-holds the population gate on its own, so fn runs unlocked.
func (g *makerGroup) After(d time.Duration, fn func()) {
	g.n.makerQueue.After(d, fn)
}

// Every is After every d.
func (g *makerGroup) Every(d time.Duration, fn func()) *sim.Ticker {
	return g.n.makerQueue.Every(d, fn)
}

// DeleteAll deletes every NPC of g, and its database-tracked rows go back
// to uninitialized. Its slots are dropped first, so no maker hook runs for
// the deleted NPCs and no respawn comes back; a master's privates leave
// with it. Each NPC is out of the world, its decayed hooks run, when it
// returns.
func (g *makerGroup) DeleteAll() {
	n := g.n
	var ids []int32
	n.mu.Lock()
	for i, keys := range g.keys {
		for _, key := range keys {
			slot := n.slot[key]
			n.respawn.Cancel(key)
			if n.liveLocked(key) {
				ids = append(ids, slot.liveID)
				delete(n.live, slot.liveID)
				n.liveCount--
			}
			delete(n.slot, key)
			if slot.dbName != "" {
				if state, ok := n.spawns.State(slot.dbName); ok {
					state.Status = spawn.StatusUninitialized
				}
			}
		}
		g.keys[i] = nil
	}
	n.mu.Unlock()
	for _, id := range ids {
		obj, ok := n.state.Object(id)
		if !ok {
			continue
		}
		switch o := obj.(type) {
		case *npc.Hostile:
			n.despawnMinions(o)
			n.forget(o, id)
			o.Decay(n.state, nil)
		case *npc.Folk:
			n.forget(o, id)
			o.Decay(n.state, nil)
		}
	}
}

// groupSpawn is one spawn entry of a makerGroup (spawn.GroupSpawn). A slot
// whose first NPC is still being placed is none of its NPCs yet.
type groupSpawn struct {
	g *makerGroup
	i int
}

func (s groupSpawn) Entry() spawn.Entry { return s.g.def.Entries[s.i] }

func (s groupSpawn) Total() int {
	if s.Persisted() {
		return 1
	}
	return s.Entry().Total
}

func (s groupSpawn) Spawned() int {
	spawned, _ := s.counts()
	return spawned
}

func (s groupSpawn) Decayed() int {
	_, decayed := s.counts()
	return decayed
}

func (s groupSpawn) counts() (spawned, decayed int) {
	n := s.g.n
	n.mu.Lock()
	defer n.mu.Unlock()
	for _, key := range s.g.keys[s.i] {
		switch {
		case n.liveLocked(key):
			spawned++
		case !n.slot[key].spawning:
			decayed++
		}
	}
	return spawned, decayed
}

func (s groupSpawn) Persisted() bool { return s.Entry().DBName != "" }

func (s groupSpawn) NPCs() []spawn.GroupNPC {
	n := s.g.n
	n.mu.Lock()
	defer n.mu.Unlock()
	out := make([]spawn.GroupNPC, 0, len(s.g.keys[s.i]))
	for _, key := range s.g.keys[s.i] {
		if !n.slot[key].spawning {
			out = append(out, groupNPC{n: n, key: key})
		}
	}
	return out
}

func (s groupSpawn) Spawn() { s.g.n.spawnGroupEntry(s.g, s.i) }

func (s groupSpawn) Respawn() {
	n := s.g.n
	n.mu.Lock()
	var first string
	for _, key := range s.g.keys[s.i] {
		if !n.liveLocked(key) && !n.slot[key].spawning {
			first = key
			break
		}
	}
	n.mu.Unlock()
	if first != "" {
		n.respawn.Cancel(first)
		n.respawnSlot(first)
	}
}

func (s groupSpawn) LoadDBInfo() { s.g.behavior.NPCDBInfo(s.g, s) }

// groupNPC is the NPC one spawn slot keeps (spawn.GroupNPC): the slot
// lives on across the NPC's respawns.
type groupNPC struct {
	n   *Npcs
	key string
}

func (c groupNPC) Decayed() bool {
	c.n.mu.Lock()
	defer c.n.mu.Unlock()
	return !c.n.liveLocked(c.key)
}

func (c groupNPC) Delete() {
	c.n.mu.Lock()
	slot := c.n.slot[c.key]
	live := c.n.liveLocked(c.key)
	c.n.mu.Unlock()
	if live {
		c.n.deleteLive(slot.liveID)
	}
}

func (c groupNPC) ScheduleRespawn(d time.Duration) {
	if d <= 0 {
		return
	}
	now := c.n.now()
	c.n.mu.Lock()
	defer c.n.mu.Unlock()
	if _, ok := c.n.slot[c.key]; ok {
		c.n.respawn.AddEarliest(c.key, now.Add(d))
	}
}

// liveLocked reports whether the slot key has its NPC in the world. The
// caller holds n.mu.
func (n *Npcs) liveLocked(key string) bool {
	slot, ok := n.slot[key]
	return ok && slot.liveID != 0 && n.live[slot.liveID] == key
}

// spawnGroupEntry places one more NPC of entry i of g. A database-tracked
// entry keeps a single slot: once it has one, it spawns nothing more.
func (n *Npcs) spawnGroupEntry(g *makerGroup, i int) {
	entry := g.def.Entries[i]
	tmpl, ok := n.templates.Get(int(entry.NPCID))
	if !ok {
		n.log.Warn().Int32("npc_id", entry.NPCID).Str("maker", g.def.Name).Msg("spawn entry references unknown npc template")
		return
	}

	if entry.DBName != "" {
		key := slotKey(entry.DBName, g.gen)
		n.mu.Lock()
		_, exists := n.slot[key]
		n.mu.Unlock()
		if !exists {
			n.spawnPersistedSlot(g, i, key, tmpl)
			n.endSpawn(key)
		}
		return
	}

	pos, ok := n.pickSpawnPosition(g.def, entry)
	if !ok {
		n.deferredCount.Add(1)
		return
	}
	n.mu.Lock()
	seq := g.seq[i]
	g.seq[i]++
	n.mu.Unlock()
	key := slotKey(fmt.Sprintf("%s#%d#%d", g.def.Name, i, seq), g.gen)
	n.registerSlot(key, g, i, "", tmpl)
	n.spawnFresh(key, entry, tmpl, pos)
	n.endSpawn(key)
}

// created runs the maker hook of the npcmaker g for the NPC its entry i
// placed under the slot key, once that NPC's created hooks have run. A
// created hook may have deleted the NPC already: the maker still hears of
// it, and finds it decayed.
func (n *Npcs) created(g *makerGroup, i int, key string) {
	g.behavior.NPCCreated(g, groupSpawn{g: g, i: i}, groupNPC{n: n, key: key})
}

// held reports whether the spawn condition of m holds, keeping its NPCs out
// of the world: for a maker with an event, the Seven Signs state decides
// the ssq_* groups and the SpawnEvents list any other one (listed, it does
// not hold); a maker with a spawn time holds, and any other maker does not.
func (n *Npcs) held(m *spawn.Maker) bool {
	if event := m.EventName(); event != "" {
		if held, ok := sevenSignsHeld(n.sevenSignsState(), event); ok {
			return held
		}
		return !n.events.has(event)
	}
	kind, _ := m.SpawnTimeOf()
	return kind != spawn.SpawnTimeNone
}

// sevenSignsHeld reports whether the Seven Signs state ss keeps the NPCs of
// event out of the world; ok is false for an event that is no Seven Signs
// group. Before the state is known (nil ss) every group is held.
func sevenSignsHeld(ss SevenSigns, event string) (held, ok bool) {
	var seal int
	var cabal sevensigns.Cabal
	switch event {
	case "ssq_event":
	case "ssq_seal1_none", "ssq_seal2_none":
		cabal = sevensigns.NoCabal
	case "ssq_seal1_dawn", "ssq_seal2_dawn":
		cabal = sevensigns.Dawn
	case "ssq_seal1_twilight", "ssq_seal2_twilight":
		cabal = sevensigns.Dusk
	default:
		return false, false
	}
	if strings.HasPrefix(event, "ssq_seal2") {
		seal = 1
	}
	if ss == nil {
		return true, true
	}
	period := ss.CurrentPeriod()
	contest := period == sevensigns.Recruiting || period == sevensigns.Competition
	if event == "ssq_event" {
		return !contest, true
	}
	if contest {
		return true, true
	}
	owner, won := ss.SealOwners()[seal], ss.WinningCabal()
	if cabal == sevensigns.NoCabal {
		return !(owner == sevensigns.NoCabal || owner != won), true
	}
	return !(owner == cabal && owner == won), true
}

// sevenSignsState returns the Seven Signs state, nil before it is known.
func (n *Npcs) sevenSignsState() SevenSigns {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.sevenSigns
}

// StartSevenSigns hands the population the Seven Signs state its ssq_*
// groups follow and spawns them as the state says.
func (n *Npcs) StartSevenSigns(ss SevenSigns) {
	n.mu.Lock()
	n.sevenSigns = ss
	n.mu.Unlock()
	n.SevenSignsChanged()
}

// SevenSignsChanged deletes every Seven Signs group and starts the ones
// the period, the winner and the seal owners call for: the event NPCs
// while recruiting and competing, then a group of each seal's NPCs.
func (n *Npcs) SevenSignsChanged() {
	n.sevenSignsPass()
}

func (n *Npcs) sevenSignsPass() {
	ss := n.sevenSignsState()
	if ss == nil {
		return
	}
	for _, event := range sevenSignsGroups {
		n.deleteEvent(event)
	}
	switch ss.CurrentPeriod() {
	case sevensigns.Recruiting, sevensigns.Competition:
		n.startEventLogged("ssq_event")
	case sevensigns.Results, sevensigns.SealValidation:
		won, owners := ss.WinningCabal(), ss.SealOwners()
		n.startEventLogged(sealGroup("ssq_seal1", owners[0], won))
		n.startEventLogged(sealGroup("ssq_seal2", owners[1], won))
	}
}

// sealGroup is the spawn event of a seal's NPCs: the owner's when it owns
// the seal as the winning cabal, the ownerless one otherwise.
func sealGroup(prefix string, owner, won sevensigns.Cabal) string {
	switch {
	case owner == sevensigns.Dusk && won == sevensigns.Dusk:
		return prefix + "_twilight"
	case owner == sevensigns.Dawn && won == sevensigns.Dawn:
		return prefix + "_dawn"
	}
	return prefix + "_none"
}

// startEvent starts every npcmaker of event, in list order.
func (n *Npcs) startEvent(event string) {
	if event == "" {
		return
	}
	for _, g := range n.currentGroups() {
		if g.def.EventName() == event {
			g.behavior.Start(g)
		}
	}
}

func (n *Npcs) startEventLogged(event string) {
	before := n.LiveCount()
	n.startEvent(event)
	n.log.Info().Str("event", event).Int("npcs", n.LiveCount()-before).Msg("spawned event npcs")
}

// deleteEvent deletes the NPCs of every npcmaker of event.
func (n *Npcs) deleteEvent(event string) {
	for _, g := range n.currentGroups() {
		if g.def.EventName() == event {
			g.DeleteAll()
		}
	}
}

// MakerEvent runs the maker script event name, with its two arguments, of
// the first npcmaker named maker (case-insensitive) and reports whether
// one was found.
func (n *Npcs) MakerEvent(maker, name string, int1, int2 int) bool {
	for _, g := range n.currentGroups() {
		if strings.EqualFold(g.def.Name, maker) {
			g.behavior.ScriptEvent(g, name, int1, int2)
			return true
		}
	}
	return false
}

// deleteLive deletes the live NPC id at once, with no corpse, on the
// calling goroutine: its spawn slot answers as for a decayed corpse.
func (n *Npcs) deleteLive(id int32) {
	obj, ok := n.state.Object(id)
	if !ok {
		return
	}
	switch a := obj.(type) {
	case *npc.Hostile:
		a.DeleteNow()
	case *npc.Folk:
		a.DeleteNow()
	}
}
