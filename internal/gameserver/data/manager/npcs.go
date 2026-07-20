package manager

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attack"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// slotInfo is the static definition of one spawn slot: the entry it was
// declared under, and (when non-empty) the persisted state row backing it.
// A slot with a non-empty dbName is the only kind restored across restarts
// and forced to a single live instance, matching the reference server's
// "a database-tracked spawn ignores its total and only ever has one
// instance" rule.
type slotInfo struct {
	key    string
	maker  *spawn.Maker
	entry  spawn.Entry
	dbName string
}

// KillRewardConfig carries live reward settings loaded at game-server boot.
type KillRewardConfig struct {
	Rates             item.Rates
	AutoLoot          bool
	AutoLootRaid      bool
	AutoLootHerbs     bool
	DeepBlueDropRules bool
	PlayerLevels      *player.LevelTable
}

// Npcs owns every live NPC instantiated from the spawn table at boot,
// indexed by object id, and drives their decay/respawn/AI lifecycle.
//
// Spawn entries with an explicit "pos" attribute are placed at that fixed
// or chance-weighted coordinate. Entries with no declared positions roll a
// random point inside their maker's territory polygon and resolve the Z
// through geodata. Entry.Privates (child minion spawns declared under one
// spawn position) are not instantiated here; minion fan-out has its own
// master/minion linking concerns.
//
// Every instantiated NPC becomes a npc.Hostile: an entry whose template
// resolves to a non-combat instance type (a shop, trainer, gatekeeper,
// village master, and similar service NPCs — confirmed against the full
// shipped spawn list, roughly a quarter of positioned entries) is counted
// and skipped rather than given some other live representation. Those NPCs
// need dialog/HTML/shop interaction, not combat, which is its own later
// system (the dialog pipeline epic) — this type only builds the
// combat-capable half of "every spawn entry becomes a live NPC".
//
// All exported methods are safe for concurrent use; mu guards slots/live.
type Npcs struct {
	templates *npc.Table
	geo       move.Geo
	state     *world.State
	ids       idAllocator
	decay     *task.Decay
	respawn   *task.Respawn
	ai        *task.AI
	positions *task.PositionUpdates
	items     *item.Table
	ground    groundPlacer
	rewards   KillRewardConfig
	spawns    *Spawns
	now       func() time.Time
	log       zerolog.Logger

	mu   sync.Mutex
	slot map[string]slotInfo
	live map[int32]string

	// liveCount is guarded by mu, not atomic: every update pairs it with a
	// live map write/delete that must stay consistent with the count.
	liveCount int

	// deferredCount/restoredDeadCount/skippedNonCombatCount are lone
	// increments with no other state to keep in sync, so they're atomic
	// rather than sharing mu.
	deferredCount         atomic.Int64
	restoredDeadCount     atomic.Int64
	skippedNonCombatCount atomic.Int64
}

// NewNpcs walks spawns' loaded table and instantiates every "on start"
// maker's qualifying entries into state, respecting persisted dead/alive
// data for database-tracked entries.
func NewNpcs(spawns *Spawns, templates *npc.Table, geo move.Geo, state *world.State, ids idAllocator, decay *task.Decay, respawnTask *task.Respawn, ai *task.AI, positions *task.PositionUpdates, items *item.Table, ground groundPlacer, rewards KillRewardConfig, now func() time.Time, log zerolog.Logger) (*Npcs, error) {
	if spawns == nil || spawns.Table() == nil {
		return nil, fmt.Errorf("npcs: nil spawn table")
	}
	if templates == nil {
		return nil, fmt.Errorf("npcs: nil npc template table")
	}
	if geo == nil {
		return nil, fmt.Errorf("npcs: nil geo")
	}
	if state == nil {
		return nil, fmt.Errorf("npcs: nil world state")
	}
	if ids == nil {
		return nil, fmt.Errorf("npcs: nil id allocator")
	}
	if decay == nil {
		return nil, fmt.Errorf("npcs: nil decay task")
	}
	if respawnTask == nil {
		return nil, fmt.Errorf("npcs: nil respawn task")
	}
	if ai == nil {
		return nil, fmt.Errorf("npcs: nil ai task")
	}
	if positions == nil {
		return nil, fmt.Errorf("npcs: nil position updates task")
	}
	if items == nil {
		return nil, fmt.Errorf("npcs: nil item table")
	}
	if ground == nil {
		return nil, fmt.Errorf("npcs: nil ground placer")
	}
	if now == nil {
		now = time.Now
	}

	n := &Npcs{
		templates: templates,
		geo:       geo,
		state:     state,
		ids:       ids,
		decay:     decay,
		respawn:   respawnTask,
		ai:        ai,
		positions: positions,
		items:     items,
		ground:    ground,
		rewards:   rewards,
		spawns:    spawns,
		now:       now,
		log:       log,
		slot:      make(map[string]slotInfo),
		live:      make(map[int32]string),
	}

	for _, maker := range spawns.Table().Makers() {
		if !isOnStartMaker(maker) {
			continue
		}
		remaining := maker.MaximumNPCs
		for entryIndex, entry := range maker.Entries {
			n.bootSpawnEntry(maker, entryIndex, entry, &remaining)
		}
	}

	return n, nil
}

// isOnStartMaker reports whether maker should be populated at boot: it has
// no event gate and its ai params don't disable the initial spawn. Makers
// with an "ai type" that scripts special spawn selection (random pick
// among candidates, exclusive slots, day/night toggles, etc.) are treated
// the same as the default "spawn every entry up to its total" behavior —
// no scripted maker framework exists in this codebase yet.
func isOnStartMaker(maker *spawn.Maker) bool {
	if maker.Event != "" {
		return false
	}
	if v, ok := maker.AIParams["on_start_spawn"]; ok && v == "0" {
		return false
	}
	return true
}

// bootSpawnEntry instantiates one maker entry's slots at boot: a single
// persisted slot for a database-tracked entry, or up to entry.Total fresh
// slots otherwise. remaining is the maker's shared spawn budget, decremented
// per instance placed and left untouched for a skipped/deferred entry.
func (n *Npcs) bootSpawnEntry(maker *spawn.Maker, entryIndex int, entry spawn.Entry, remaining *int) {
	tmpl, ok := n.templates.Get(int(entry.NPCID))
	if !ok {
		n.log.Warn().Int32("npc_id", entry.NPCID).Str("maker", maker.Name).Msg("spawn entry references unknown npc template")
		return
	}

	if entry.DBName != "" {
		if *remaining <= 0 {
			return
		}
		*remaining--
		n.bootSpawnPersisted(maker, entry.DBName, entry, tmpl)
		return
	}

	for i := 0; i < entry.Total; i++ {
		if *remaining <= 0 {
			return
		}
		pos, ok := n.pickSpawnPosition(maker, entry)
		if !ok {
			n.deferredCount.Add(1)
			return
		}
		*remaining--
		key := fmt.Sprintf("%s#%d#%d", maker.Name, entryIndex, i)
		n.registerSlot(key, maker, entry, "")
		n.spawnFresh(key, entry, tmpl, pos)
	}
}

func (n *Npcs) registerSlot(key string, maker *spawn.Maker, entry spawn.Entry, dbName string) {
	n.mu.Lock()
	n.slot[key] = slotInfo{key: key, maker: maker, entry: entry, dbName: dbName}
	n.mu.Unlock()
}

// bootSpawnPersisted restores or freshly spawns a database-tracked entry's
// single slot at boot. A spawn still dead with a pending respawn deadline
// is not instantiated: only its respawn timer is (re)armed, matching the
// persisted-state restore rule.
func (n *Npcs) bootSpawnPersisted(maker *spawn.Maker, dbName string, entry spawn.Entry, tmpl *npc.Template) {
	n.registerSlot(dbName, maker, entry, dbName)

	state, ok := n.spawns.State(dbName)
	if !ok {
		state = spawn.NewState(dbName)
	}

	now := n.now()
	if state.Dead(now) {
		remaining := time.UnixMilli(state.RespawnTime).Sub(now)
		if remaining < 0 {
			remaining = 0
		}
		n.respawn.Add(dbName, now.Add(remaining))
		n.restoredDeadCount.Add(1)
		return
	}

	n.spawnPersisted(dbName, maker, entry, tmpl, state)
}

// spawnPersisted places one instance of a database-tracked entry, reusing
// persisted HP and position when the row was still alive, or a freshly
// rolled position at full HP otherwise (CheckAlive's own restore rule).
func (n *Npcs) spawnPersisted(key string, maker *spawn.Maker, entry spawn.Entry, tmpl *npc.Template, state *spawn.State) {
	now := n.now()
	pos, ok := n.pickSpawnPosition(maker, entry)
	if !ok {
		n.deferredCount.Add(1)
		return
	}

	loc, heading, hp := pos.Location, pos.Heading, int(tmpl.HPMax)
	if state.CheckAlive(pos.Location, pos.Heading, int(tmpl.HPMax), 0, now) {
		loc, heading, hp = state.Location, state.Heading, state.CurrentHP
	}
	n.instantiate(key, entry, tmpl, loc, heading, hp)
}

// spawnFresh places one non-persisted instance of entry at a freshly rolled
// position, always alive at full HP — the reference server never restores
// HP/position across restarts for a spawn without a database name.
func (n *Npcs) spawnFresh(key string, entry spawn.Entry, tmpl *npc.Template, pos spawn.Position) {
	n.instantiate(key, entry, tmpl, pos.Location, pos.Heading, int(tmpl.HPMax))
}

// instantiate builds one live Hostile from tmpl and places it in the world
// at (loc, heading) with hp current HP, registering it for AI ticks and
// corpse decay/respawn.
func (n *Npcs) instantiate(key string, entry spawn.Entry, tmpl *npc.Template, loc location.Location, heading, hp int) {
	id, err := n.ids.NextID()
	if err != nil {
		n.log.Warn().Err(err).Int32("npc_id", entry.NPCID).Msg("spawn: id space exhausted")
		return
	}

	inst, err := npc.NewInstance(id, tmpl)
	if err != nil {
		n.log.Warn().Err(err).Int32("npc_id", entry.NPCID).Msg("spawn: cannot build npc instance")
		return
	}
	inst.Home = loc
	inst.HasHome = true

	if !npc.Attackable(inst) {
		n.skippedNonCombatCount.Add(1)
		return
	}

	hostile, err := newLiveHostile(inst, tmpl.RunSpeed, n.geo, n.positions)
	if err != nil {
		n.log.Warn().Err(err).Int32("npc_id", entry.NPCID).Msg("spawn: cannot build live npc")
		return
	}

	hostile.SetCurrentHP(hp)
	hostile.SetWorld(n.state)
	hostile.SetWeapon(n.items)
	hostile.SetRewarder(n.rewarderFor(hostile, tmpl))

	n.state.Spawn(hostile, loc.X, loc.Y, loc.Z, heading)
	n.ai.Add(hostile)

	n.mu.Lock()
	n.live[id] = key
	n.liveCount++
	n.mu.Unlock()
}

// rewarderFor returns the kill-reward hook for a newly spawned hostile.
func (n *Npcs) rewarderFor(hostile *npc.Hostile, tmpl *npc.Template) *deathRewards {
	return &deathRewards{
		hostile:    hostile,
		tmpl:       tmpl,
		categories: tmpl.Drops,
		config:     n.rewards,
		raid:       tmpl.Type == "RaidBoss",
		decay:      n.decay,
		ids:        n.ids,
		items:      n.items,
		ground:     n.ground,
	}
}

// RespawnHook implements the decay task's per-actor respawn resolution: it
// unregisters actorID from AI ticks and live tracking, and — when its slot
// has a positive respawn delay — returns the closure that arms the next
// respawn. It reports nil when actorID isn't a tracked spawn slot, or when
// the slot's entry has no respawn delay (a permanent, one-shot spawn).
func (n *Npcs) RespawnHook(actorID int32) func() {
	if obj, ok := n.state.Object(actorID); ok {
		if h, ok := obj.(*npc.Hostile); ok {
			n.ai.Remove(h)
		}
	}

	n.mu.Lock()
	key, tracked := n.live[actorID]
	if tracked {
		delete(n.live, actorID)
		n.liveCount--
	}
	n.mu.Unlock()
	if !tracked {
		return nil
	}

	n.mu.Lock()
	slot, ok := n.slot[key]
	n.mu.Unlock()
	if !ok {
		return nil
	}

	delay := spawn.CalculateRespawnDelay(slot.entry)
	if delay <= 0 {
		n.mu.Lock()
		delete(n.slot, key)
		n.mu.Unlock()
		return nil
	}

	return func() { n.scheduleRespawn(slot, delay) }
}

func (n *Npcs) scheduleRespawn(slot slotInfo, delay time.Duration) {
	now := n.now()
	if slot.dbName != "" {
		if state, ok := n.spawns.State(slot.dbName); ok {
			state.SetRespawn(delay, now)
		}
	}
	n.respawn.Add(slot.key, now.Add(delay))
}

// Respawn implements task.RespawnEffects: it re-instantiates the slot key
// identifies, picking a fresh position (and, for a database-tracked slot,
// resuming through the same persisted-state restore rule used at boot).
func (n *Npcs) Respawn(key string) {
	n.mu.Lock()
	slot, ok := n.slot[key]
	n.mu.Unlock()
	if !ok {
		return
	}

	tmpl, ok := n.templates.Get(int(slot.entry.NPCID))
	if !ok {
		return
	}

	if slot.dbName != "" {
		state, ok := n.spawns.State(slot.dbName)
		if !ok {
			state = spawn.NewState(slot.dbName)
		}
		n.spawnPersisted(key, slot.maker, slot.entry, tmpl, state)
		return
	}
	pos, ok := n.pickSpawnPosition(slot.maker, slot.entry)
	if !ok {
		n.deferredCount.Add(1)
		return
	}
	n.spawnFresh(key, slot.entry, tmpl, pos)
}

// SyncPersistedState writes every live database-tracked slot's current HP
// and position back into its spawn.State row, ready for Spawns.Save. Dead
// rows (mid respawn countdown) are left untouched by State.SetStats itself.
func (n *Npcs) SyncPersistedState() {
	n.mu.Lock()
	live := make(map[int32]string, len(n.live))
	for id, key := range n.live {
		live[id] = key
	}
	n.mu.Unlock()

	for id, key := range live {
		n.mu.Lock()
		slot, ok := n.slot[key]
		n.mu.Unlock()
		if !ok || slot.dbName == "" {
			continue
		}
		obj, ok := n.state.Object(id)
		if !ok {
			continue
		}
		hostile, ok := obj.(*npc.Hostile)
		if !ok {
			continue
		}
		state, ok := n.spawns.State(slot.dbName)
		if !ok {
			continue
		}
		x, y, z := hostile.Position()
		state.SetStats(hostile.CurrentHP(), 0, location.Location{X: x, Y: y, Z: z}, hostile.Heading())
	}
}

// LiveCount returns the number of currently spawned (not decayed) NPCs.
func (n *Npcs) LiveCount() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.liveCount
}

// DeferredCount returns the number of spawn entries skipped at boot because
// no usable explicit or territory-random position could be chosen.
func (n *Npcs) DeferredCount() int {
	return int(n.deferredCount.Load())
}

// RestoredDeadCount returns the number of database-tracked entries that
// were still dead with a pending respawn deadline at boot.
func (n *Npcs) RestoredDeadCount() int {
	return int(n.restoredDeadCount.Load())
}

// SkippedNonCombatCount returns the number of spawn entries skipped at boot
// for resolving to a non-combat instance type (shops, trainers, and similar
// service NPCs the dialog pipeline doesn't support yet).
func (n *Npcs) SkippedNonCombatCount() int {
	return int(n.skippedNonCombatCount.Load())
}

// pickPosition selects one spawn position from positions. A single entry
// (the "fixed" declaration) is used exactly as declared, heading included.
// Multiple entries (the "chance-weighted" declaration) are chosen by
// rolling a percentage against each Chance in turn — and, matching the
// reference server's own behavior for this shape, the winning entry's
// declared heading is discarded in favor of a fresh random one. A weight
// table that doesn't sum to 100 falls back to the last entry rather than
// leaving the slot unspawned.
func pickPosition(positions []spawn.Position) spawn.Position {
	if len(positions) == 1 {
		return positions[0]
	}

	chance := rnd.Get(100)
	for _, pos := range positions {
		chance -= pos.Chance
		if chance < 0 {
			pos.Heading = rnd.Get(65536)
			return pos
		}
	}
	last := positions[len(positions)-1]
	last.Heading = rnd.Get(65536)
	return last
}

func (n *Npcs) pickSpawnPosition(maker *spawn.Maker, entry spawn.Entry) (spawn.Position, bool) {
	if len(entry.Positions) > 0 {
		return pickPosition(entry.Positions), true
	}
	return randomTerritoryPosition(maker, n.geo)
}

const territorySpawnAttempts = 10
const territoryPointAttempts = 64

func randomTerritoryPosition(maker *spawn.Maker, geo move.Geo) (spawn.Position, bool) {
	if maker == nil || len(maker.Territories) == 0 {
		return spawn.Position{}, false
	}

	var last spawn.Position
	haveLast := false
	for i := 0; i < territorySpawnAttempts; i++ {
		territory := maker.Territories[rnd.Get(len(maker.Territories))]
		x, y, ok := randomPointInTerritory(territory)
		if !ok {
			continue
		}

		z := int(geo.Height(x, y, averageZ(territory)))
		pos := spawn.Position{
			Location: location.Location{X: x, Y: y, Z: z},
			Heading:  rnd.Get(65536),
		}
		last, haveLast = pos, true

		if z < territory.MinZ || z > territory.MaxZ || insideAnyTerritory(maker.BannedTerritories, pos.Location) {
			continue
		}
		return pos, true
	}
	return last, haveLast
}

func randomPointInTerritory(territory *spawn.Territory) (int, int, bool) {
	if territory == nil || len(territory.Nodes) == 0 {
		return 0, 0, false
	}

	minX, maxX := territory.Nodes[0].X, territory.Nodes[0].X
	minY, maxY := territory.Nodes[0].Y, territory.Nodes[0].Y
	for _, node := range territory.Nodes[1:] {
		minX = min(minX, node.X)
		maxX = max(maxX, node.X)
		minY = min(minY, node.Y)
		maxY = max(maxY, node.Y)
	}

	for i := 0; i < territoryPointAttempts; i++ {
		x := rnd.GetRange(minX, maxX)
		y := rnd.GetRange(minY, maxY)
		if territoryContains2D(territory, x, y) {
			return x, y, true
		}
	}

	x, y := territoryCentroid(territory)
	if territoryContains2D(territory, x, y) {
		return x, y, true
	}
	return 0, 0, false
}

func insideAnyTerritory(territories []*spawn.Territory, loc location.Location) bool {
	for _, territory := range territories {
		if territoryContainsLocation(territory, loc) {
			return true
		}
	}
	return false
}

func territoryContainsLocation(territory *spawn.Territory, loc location.Location) bool {
	return territory != nil &&
		loc.Z >= territory.MinZ &&
		loc.Z <= territory.MaxZ &&
		territoryContains2D(territory, loc.X, loc.Y)
}

func territoryContains2D(territory *spawn.Territory, x, y int) bool {
	nodes := territory.Nodes
	inside := false
	j := len(nodes) - 1
	for i := range nodes {
		a, b := nodes[i], nodes[j]
		if pointOnSegment(x, y, a, b) {
			return true
		}
		if (a.Y > y) != (b.Y > y) {
			crossX := float64(b.X-a.X)*float64(y-a.Y)/float64(b.Y-a.Y) + float64(a.X)
			if float64(x) < crossX {
				inside = !inside
			}
		}
		j = i
	}
	return inside
}

func pointOnSegment(x, y int, a, b spawn.Node) bool {
	cross := (x-a.X)*(b.Y-a.Y) - (y-a.Y)*(b.X-a.X)
	if cross != 0 {
		return false
	}
	return x >= min(a.X, b.X) && x <= max(a.X, b.X) &&
		y >= min(a.Y, b.Y) && y <= max(a.Y, b.Y)
}

func territoryCentroid(territory *spawn.Territory) (int, int) {
	var x, y int
	for _, node := range territory.Nodes {
		x += node.X
		y += node.Y
	}
	return x / len(territory.Nodes), y / len(territory.Nodes)
}

func averageZ(territory *spawn.Territory) int {
	return (territory.MinZ + territory.MaxZ) / 2
}

// locatedRef and creatureActorRef are forward references that break the
// construction cycle between a live NPC and the movement/attack
// controllers it owns: the controllers need the NPC's position/combat
// surface, but the NPC's own constructor needs the controllers already
// built. Each embeds its target interface unset, is handed to the
// controller constructors, and is pointed at the real NPC immediately
// after — before anything can call through it.
type locatedRef struct{ move.Actor }
type creatureActorRef struct{ attack.CreatureActor }

// newLiveHostile builds a live Hostile for inst, wiring a real movement
// controller (over the Hostile's lifetime movement state) and a real attack
// controller, resolving their mutual construction-order dependency on the
// finished Hostile via locatedRef/creatureActorRef.
func newLiveHostile(inst *npc.Instance, speed float64, geo move.Geo, positions *task.PositionUpdates) (*npc.Hostile, error) {
	live, err := creature.NewLive(inst.Home, speed, geo)
	if err != nil {
		return nil, err
	}

	locRef := &locatedRef{}
	moveCtl, err := move.NewController(live.Move(), locRef)
	if err != nil {
		return nil, err
	}
	moveCtl.SetPositionUpdates(positions)

	actorRef := &creatureActorRef{}
	attackCtl := attack.NewAttackable(actorRef)

	hostile, err := npc.NewHostile(inst, live, moveCtl, attackCtl)
	if err != nil {
		return nil, err
	}
	if los, ok := geo.(npc.LineOfSight); ok {
		hostile.SetLineOfSight(los)
	}

	locRef.Actor = hostile
	actorRef.CreatureActor = hostile

	// Re-evaluate the AI loop as soon as a chase leg completes or a swing
	// finishes, rather than waiting for the next fixed AI tick — otherwise
	// a hostile NPC only closes distance on, or re-attacks, its target once
	// per task.AITick. CreatureMove tracks position for its own timing only;
	// the arrived hook must push that position into the world-grid presence
	// range checks actually read before re-thinking, or the AI loop re-runs
	// against a stale position forever.
	moveCtl.SetArrived(func() {
		pos := moveCtl.Position()
		hostile.SyncPosition(pos)
		hostile.Think()
	})
	attackCtl.SetFinished(hostile.Think)

	return hostile, nil
}

// deathRewards applies one victim's live death rewards at its position at
// the moment of death, rather than a position fixed when it spawned —
// hostile NPCs can move (offensive follow) between spawning and dying.
type deathRewards struct {
	hostile    *npc.Hostile
	tmpl       *npc.Template
	categories []item.DropCategory
	config     KillRewardConfig
	raid       bool
	decay      *task.Decay
	ids        idAllocator
	items      *item.Table
	ground     groundPlacer
}

type playerRewardEntry struct {
	actor  *player.Character
	damage float64
}

// CalculateRewards implements creature.Rewarder.
func (d *deathRewards) CalculateRewards(killer creature.DeathActor) {
	d.scheduleDecay()

	entries, totalDamage, maxDealer, highestLevel := d.rewardEntries()
	d.rollDrops(killer, maxDealer, highestLevel)
	d.grantExpAndSp(entries, totalDamage)
}

func (d *deathRewards) scheduleDecay() {
	if d.tmpl.CorpseTime <= 0 {
		return
	}
	interval := time.Duration(d.tmpl.CorpseTime) * time.Second
	if d.hostile.Spoiled() || d.hostile.Seeded() {
		interval *= 2
	}
	deadline := d.decay.Add(d.hostile, interval)
	d.hostile.SetCorpseDeadline(deadline)
}

func (d *deathRewards) rewardEntries() ([]playerRewardEntry, float64, *player.Character, int) {
	var entries []playerRewardEntry
	var totalDamage float64
	var maxDealer *player.Character
	var maxDamage float64
	var highestLevel int

	for _, threat := range d.hostile.AI().Threats().Snapshot() {
		if threat.Damage <= 1 {
			continue
		}
		attacker, ok := threat.Attacker.(*player.Character)
		if !ok || attacker.AlikeDead() || !attacker.Knows(d.hostile) {
			continue
		}
		entries = append(entries, playerRewardEntry{actor: attacker, damage: threat.Damage})
		totalDamage += threat.Damage
		if maxDealer == nil || threat.Damage > maxDamage {
			maxDealer = attacker
			maxDamage = threat.Damage
		}
		if attacker.CharLevel > highestLevel {
			highestLevel = attacker.CharLevel
		}
	}

	return entries, totalDamage, maxDealer, highestLevel
}

func (d *deathRewards) rollDrops(killer creature.DeathActor, maxDealer *player.Character, highestLevel int) {
	if len(d.categories) == 0 {
		return
	}
	x, y, z := d.hostile.Position()
	heading := d.hostile.Heading()

	levelMultiplier := 1.0
	if highestLevel > 0 {
		levelMultiplier = item.LevelPenaltyMultiplier(int32(highestLevel), int32(d.tmpl.Level), d.raid, d.config.DeepBlueDropRules)
	}
	autoLootItems := d.config.AutoLoot
	if d.raid {
		autoLootItems = d.config.AutoLootRaid
	}

	receiver := killer
	if maxDealer != nil {
		receiver = maxDealer
	}
	NewKillReward(d.categories, d.hostile.SpoilPool(), levelMultiplier, d.raid, d.config.Rates, autoLootItems, d.config.AutoLootHerbs, d.ids, d.items, d.ground, x, y, z, heading, d.hostile.ObjectID()).CalculateRewards(receiver)
}

func (d *deathRewards) grantExpAndSp(entries []playerRewardEntry, totalDamage float64) {
	if d.config.PlayerLevels == nil || totalDamage <= 0 {
		return
	}
	for _, entry := range entries {
		exp, sp := player.KillRewardExpAndSp(d.tmpl.RewardExp, d.tmpl.RewardSp, entry.damage, totalDamage, entry.actor.CharLevel-d.tmpl.Level)
		entry.actor.RewardExpAndSp(d.config.PlayerLevels, exp, sp)
	}
}
