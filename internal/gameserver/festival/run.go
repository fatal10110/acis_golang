package festival

import (
	"strconv"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
)

// run is the state of the festivals' runs, guarded by Manager.mu.
type run struct {
	// participants are the parties signed up for the next festival, and
	// previous the parties of the festival that ended last waiting to
	// register their scores, each once, until the next signup period
	// ends; both by side.
	participants, previous [2]roster
	// started is set once a festival schedule has started.
	started bool
	// initialized is set from the moment the parties are taken into their
	// arenas until the festival ends; inProgress from its first monsters
	// until it ends.
	initialized, inProgress bool
	// arenas are the festival's open arenas, by side and festival id.
	arenas [2][Count]*arena
	// lastRun is set when the competition ended while a cycle was under
	// way: it keeps opening signup periods while nobody signs up and runs
	// the festival a party signs up for to its end, then the schedule
	// stops.
	lastRun bool
	// current is the festival under way, nil between festivals.
	current *festivalRun
}

// festivalRun is one festival under way, from its parties' admission to
// its end. It keeps a timer of its own: a schedule started in place of the
// one that began it lets it run to its end.
type festivalRun struct {
	// generation is the schedule's that began the festival, and due when
	// its cycle was due.
	generation uint64
	due        int64
	timer      *time.Timer
}

func newRun() run {
	return run{participants: [2]roster{{}, {}}, previous: [2]roster{{}, {}}}
}

// arena is one open arena: the festival of one oracle that a party signed
// up for.
type arena struct {
	cabal sevensigns.Cabal
	id    int
	// members are the party signed up as the arena opened; it keeps them
	// whatever the roster later says.
	members []Member
	// origins are where each member stood when taken in.
	origins    map[int32]Spot
	witch      NPC
	monsters   []NPC
	challenged bool
}

// The texts of a festival's run.
const (
	guideName          = "Festival Guide"
	witchName          = "Festival Witch"
	twoMinutesShout    = "The main event will start in 2 minutes. Please register now."
	startingText       = "The main event is now starting."
	beginsSoonText     = "The festival will begin in 2 minutes."
	chestsText         = "The chests have spawned! Be quick, the festival will end soon."
	endedText          = "The festival has ended. Your party leader must now register your score before the next festival takes place."
	outsideSoonShout   = "That will do! I'll move you to the outside soon."
	maxArenaOffset     = 230
	challengeBonus     = 2
	chestBonus         = 5
	offeringScoreValue = 5
)

// openArenasLocked returns the open arenas in the order each step of a
// festival goes through them: Dawn's festivals, then Dusk's.
func (m *Manager) openArenasLocked() []*arena {
	var out []*arena
	for _, s := range [...]int{side(sevensigns.Dawn), side(sevensigns.Dusk)} {
		for _, a := range m.arenas[s] {
			if a != nil {
				out = append(out, a)
			}
		}
	}
	return out
}

// scatter returns a point up to maxArenaOffset from (x, y) on each axis,
// both offsets of the same sign.
func scatter(x, y int) (int, int) {
	if rnd.Get(2) == 1 {
		return x + rnd.Get(maxArenaOffset), y + rnd.Get(maxArenaOffset)
	}
	return x - rnd.Get(maxArenaOffset), y - rnd.Get(maxArenaOffset)
}

// beginFestivalLocked opens an arena for each festival taken, Dusk's then Dawn's
// of each level range in turn, and arms the festival's first monsters.
func (m *Manager) beginFestivalLocked(generation uint64, due int64) []*arena {
	var opened []*arena
	for id := range Count {
		for _, cabal := range [...]sevensigns.Cabal{sevensigns.Dusk, sevensigns.Dawn} {
			if members := m.participants[side(cabal)][id]; members != nil {
				a := &arena{cabal: cabal, id: id, members: members, origins: map[int32]Spot{}}
				m.arenas[side(cabal)][id] = a
				opened = append(opened, a)
			}
		}
	}
	m.initialized = true
	m.nextFestivalStart = m.nowMillis() + m.cfg.CycleLength.Milliseconds()
	fest := &festivalRun{generation: generation, due: due}
	m.current = fest
	m.armFestivalLocked(fest, m.cfg.FirstSpawn.Milliseconds(), func() { m.festivalStarted(fest) })
	return opened
}

// openArena takes a's party in, each member online scattered around the
// arena's start, then brings in its witch.
func (m *Manager) openArena(world World, a *arena) {
	start := playerSpawns[side(a.cabal)][a.id]
	for _, p := range a.members {
		at, ok := world.Locate(p.ObjectID)
		if !ok {
			continue
		}
		m.mu.Lock()
		a.origins[p.ObjectID] = at
		m.mu.Unlock()
		x, y := scatter(start.X, start.Y)
		world.Admit(p.ObjectID, x, y, start.Z)
	}
	w := witchSpawns[side(a.cabal)][a.id]
	witch, ok := world.SpawnWitch(w.npcID, Spot{X: w.x, Y: w.y, Z: w.z, Heading: w.heading})
	if !ok {
		m.log.Error().Int("npc_id", w.npcID).Msg("festival: the witch could not be placed")
		return
	}
	m.mu.Lock()
	a.witch = witch
	m.mu.Unlock()
	m.tellParty(world, a, beginsSoonText)
}

// tellParty has a's witch say text, when a holds a party.
func (m *Manager) tellParty(world World, a *arena, text string) {
	m.mu.Lock()
	witch := a.witch
	m.mu.Unlock()
	if witch != nil && len(a.members) > 0 {
		world.Say(witch, text)
	}
}

// spawnMonsters places a's monsters of spawns, each worth bonus blood
// offerings; without archers drops the archers. A monster placed once a has
// closed is deleted again.
func (m *Manager) spawnMonsters(world World, a *arena, spawns []monsterSpawn, bonus int, withoutArchers bool) {
	for _, s := range spawns {
		if withoutArchers && archer(s.npcID) {
			continue
		}
		n, ok := world.SpawnMonster(s.npcID, s.x, s.y, s.z, bonus)
		if !ok {
			m.log.Error().Int("npc_id", s.npcID).Msg("festival: a monster could not be placed")
			continue
		}
		m.mu.Lock()
		open := m.initialized && m.arenas[side(a.cabal)][a.id] == a
		if open {
			a.monsters = append(a.monsters, n)
		}
		m.mu.Unlock()
		if !open {
			// The festival ended while the wave was coming: the arena
			// that would clear it is gone.
			world.Unspawn(n)
		}
	}
}

// archer reports whether npcID is one of the festival's archers.
func archer(npcID int) bool {
	if npcID < 18009 || npcID > 18108 {
		return false
	}
	d := npcID % 10
	return d == 4 || d == 9
}

// step runs fn, holding the lock, unless the schedule armed it under
// another generation; fn returns what to do once the lock is released.
func (m *Manager) step(generation uint64, fn func() func(World)) {
	m.stepIf(func() bool { return generation == m.generation }, fn)
}

// festivalStep runs fn as step does, unless fest is no longer the festival
// under way.
func (m *Manager) festivalStep(fest *festivalRun, fn func() func(World)) {
	m.stepIf(func() bool { return m.current == fest }, fn)
}

// stepIf runs fn, holding the lock, if live reports true under it.
func (m *Manager) stepIf(live func() bool, fn func() func(World)) {
	m.mu.Lock()
	if !live() {
		m.mu.Unlock()
		return
	}
	after := fn()
	world := m.world
	m.mu.Unlock()
	if world != nil && after != nil {
		after(world)
	}
}

// festivalStarted spawns each arena's first monsters.
func (m *Manager) festivalStarted(fest *festivalRun) {
	m.festivalStep(fest, func() func(World) {
		m.inProgress = true
		arenas := m.openArenasLocked()
		m.armFestivalLocked(fest, (m.cfg.FirstSwarm - m.cfg.FirstSpawn).Milliseconds(), func() { m.firstSwarm(fest) })
		return func(world World) {
			for _, a := range arenas {
				m.spawnMonsters(world, a, primarySpawns[side(a.cabal)][a.id], 1, false)
				m.tellParty(world, a, startingText)
			}
		}
	})
}

func (m *Manager) firstSwarm(fest *festivalRun) {
	m.festivalStep(fest, func() func(World) {
		arenas := m.openArenasLocked()
		m.armFestivalLocked(fest, (m.cfg.SecondSpawn - m.cfg.FirstSwarm).Milliseconds(), func() { m.secondSpawn(fest) })
		return func(world World) { m.swarm(world, arenas) }
	})
}

// secondSpawn brings the archers in and tells each party how long is
// left.
func (m *Manager) secondSpawn(fest *festivalRun) {
	m.festivalStep(fest, func() func(World) {
		arenas := m.openArenasLocked()
		m.armFestivalLocked(fest, (m.cfg.SecondSwarm - m.cfg.SecondSpawn).Milliseconds(), func() { m.secondSwarm(fest) })
		left := (m.cfg.Length - m.cfg.SecondSpawn).Milliseconds() / 60000
		return func(world World) {
			for _, a := range arenas {
				m.spawnMonsters(world, a, secondarySpawns[side(a.cabal)][a.id], 1, false)
				m.tellParty(world, a, "The Festival of Darkness will end in "+strconv.FormatInt(left, 10)+" minute(s).")
			}
		}
	})
}

func (m *Manager) secondSwarm(fest *festivalRun) {
	m.festivalStep(fest, func() func(World) {
		arenas := m.openArenasLocked()
		m.armFestivalLocked(fest, (m.cfg.ChestSpawn - m.cfg.SecondSwarm).Milliseconds(), func() { m.chestsSpawn(fest) })
		return func(world World) { m.swarm(world, arenas) }
	})
}

// chestsSpawn brings the chests in, each worth five blood offerings.
func (m *Manager) chestsSpawn(fest *festivalRun) {
	m.festivalStep(fest, func() func(World) {
		arenas := m.openArenasLocked()
		m.armFestivalLocked(fest, (m.cfg.Length - m.cfg.ChestSpawn).Milliseconds(), func() { m.festivalEnded(fest) })
		return func(world World) {
			for _, a := range arenas {
				m.spawnMonsters(world, a, chestSpawns[side(a.cabal)][a.id], chestBonus, false)
				m.tellParty(world, a, chestsText)
			}
		}
	})
}

// swarm sends each arena's idle monsters running at its center.
func (m *Manager) swarm(world World, arenas []*arena) {
	for _, a := range arenas {
		start := playerSpawns[side(a.cabal)][a.id]
		m.mu.Lock()
		monsters := append([]NPC(nil), a.monsters...)
		m.mu.Unlock()
		for _, n := range monsters {
			x, y := scatter(start.X, start.Y)
			world.Swarm(n, x, y, start.Z)
		}
	}
}

// festivalEnded ends the festival: each party is taken back where it came
// from and waits for its leader to register its score, the arenas close
// and a new signup period opens. The cycle that follows is due a cycle
// length after this one was, unless the competition ended meanwhile or a
// new schedule has started since the festival began.
func (m *Manager) festivalEnded(fest *festivalRun) {
	m.festivalStep(fest, func() func(World) {
		m.inProgress = false
		arenas := m.openArenasLocked()
		for _, a := range arenas {
			if len(a.members) > 0 {
				m.previous[side(a.cabal)][a.id] = a.members
			}
		}
		m.arenas = [2][Count]*arena{}
		m.participants = [2]roster{{}, {}}
		m.initialized = false
		m.current = nil
		// A schedule started since the festival began runs on its own.
		if fest.generation == m.generation {
			m.running = false
			if m.lastRun {
				m.lastRun = false
				m.timer = nil
			} else {
				generation, next := fest.generation, fest.due+m.cfg.CycleLength.Milliseconds()
				m.armLocked(max(0, next-m.nowMillis()), func() { m.runCycle(generation, next) })
			}
		}
		return func(world World) {
			for _, a := range arenas {
				for _, p := range a.members {
					if _, online := world.Locate(p.ObjectID); !online {
						continue
					}
					m.relocate(world, []relocation{{arena: a, objectID: p.ObjectID}})
					world.Tell(p.ObjectID, endedText)
				}
				m.mu.Lock()
				npcs := append([]NPC(nil), a.monsters...)
				if a.witch != nil {
					npcs = append([]NPC{a.witch}, npcs...)
				}
				m.mu.Unlock()
				for _, n := range npcs {
					world.Unspawn(n)
				}
			}
			world.Settle(memberIDs(arenas))
			world.Shout(witchName, outsideSoonShout)
		}
	})
}

// memberIDs are the object ids of arenas' parties.
func memberIDs(arenas []*arena) []int32 {
	var ids []int32
	for _, a := range arenas {
		for _, p := range a.members {
			ids = append(ids, p.ObjectID)
		}
	}
	return ids
}

// relocation takes one participant out of an arena; removing forgets
// where it came from.
type relocation struct {
	arena    *arena
	objectID int32
	removing bool
}

// relocate carries out moves, skipping the participants not in the world:
// each goes back where it came from, or to the nearest town when that is
// not known.
func (m *Manager) relocate(world World, moves []relocation) {
	if world == nil {
		return
	}
	for _, mv := range moves {
		if _, online := world.Locate(mv.objectID); !online {
			continue
		}
		m.mu.Lock()
		at, ok := mv.arena.origins[mv.objectID]
		if mv.removing {
			delete(mv.arena.origins, mv.objectID)
		}
		m.mu.Unlock()
		if ok {
			world.Relocate(mv.objectID, &at)
		} else {
			world.Relocate(mv.objectID, nil)
		}
	}
}

// IncreaseChallenge answers a party's wish to face more monsters in the
// arena of festival id of cabal: once per festival, every monster of the
// arena's first wave but the archers comes again, each worth two blood
// offerings. ok is false when that arena is not open.
func (m *Manager) IncreaseChallenge(cabal sevensigns.Cabal, id int) (increased, ok bool) {
	m.mu.Lock()
	var a *arena
	if m.initialized {
		a = m.arenas[side(cabal)][id]
	}
	if a == nil {
		m.mu.Unlock()
		return false, false
	}
	if a.challenged {
		m.mu.Unlock()
		return false, true
	}
	a.challenged = true
	world := m.world
	m.mu.Unlock()
	if world != nil {
		m.spawnMonsters(world, a, primarySpawns[side(cabal)][id], challengeBonus, true)
	}
	return true, true
}

// minutesToNextFestivalLocked counts the minutes left until the next
// festival begins, as NextFestivalNotice shows them.
func (m *Manager) minutesToNextFestivalLocked() int {
	return int(float32((m.nextFestivalStart-m.nowMillis())/60000)) + 1
}
