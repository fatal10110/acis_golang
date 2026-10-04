// Package hero owns the heroes: the ones the end of an Olympiad elects,
// one per third-class profession, and every character that was ever one.
// It keeps them in the heroes table and writes the hero diary entries to
// heroes_diary.
//
// An elected hero stays inactive until it claims the status at a Monument
// of Heroes; only an active hero is a hero in game. The diary and fight
// history pages and the hero's message are not read back yet (#3361).
package hero

import (
	"cmp"
	"context"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/rs/zerolog"
)

// taskTimeout bounds one queued database write.
const taskTimeout = 10 * time.Second

// writeLane is the persistence owner every hero write is queued under. It
// is the Olympiad's lane too, so an election reads the nobles' records the
// Olympiad saved just before it, and a later activation lands after the
// election's own writes. No character, item or clan has object id 0.
const writeLane int32 = 0

// The heroes_diary actions.
const (
	DiaryRaidKilled  = 1
	DiaryHeroGained  = 2
	DiaryCastleTaken = 3
)

// lastClassID is the highest profession id.
const lastClassID = 118

// Hero is one heroes row, with the clan and alliance its entry shows.
type Hero struct {
	Name    string
	ClassID int
	// Count is how many times the character was elected.
	Count int
	// Played marks a hero of the running era.
	Played bool
	// Active marks a hero that claimed its status.
	Active    bool
	ClanName  string
	ClanCrest int32
	AllyName  string
	AllyCrest int32
}

// Row is one stored heroes row, with the character's current name and
// clan.
type Row struct {
	ObjectID int32
	Name     string
	ClassID  int
	Count    int
	Played   bool
	Active   bool
	ClanID   int32
}

// Store persists the heroes.
type Store interface {
	// LoadHeroes returns every heroes row whose character still exists.
	LoadHeroes(ctx context.Context) ([]Row, error)
	// ClanID returns the clan objectID's character belongs to, 0 for none;
	// found is false when the character does not exist.
	ClanID(ctx context.Context, objectID int32) (clanID int32, found bool, err error)
	// ResetPlayed marks every stored hero as no longer of the running era.
	ResetPlayed(ctx context.Context) error
	// BestNoble returns the noble of classID with the most points, then
	// matches, then wins, among those with at least minMatches matches and
	// one win; found is false when there is none.
	BestNoble(ctx context.Context, classID, minMatches int) (objectID int32, name string, found bool, err error)
	// DeleteHeroItems deletes every hero item no game master owns.
	DeleteHeroItems(ctx context.Context) error
	// SaveHeroes inserts each hero, or updates its count and flags.
	SaveHeroes(ctx context.Context, heroes map[int32]Hero) error
	// AddDiaryEntry stores one diary entry made at at, in Unix milliseconds.
	AddDiaryEntry(ctx context.Context, objectID int32, at int64, action, param int) error
}

// Writer runs a database write later, on ownerID's persistence lane.
type Writer interface {
	Enqueue(ownerID int32, job func()) bool
}

// Clans finds a clan by id.
type Clans interface {
	Get(id int32) (*clan.Clan, bool)
}

// Online reaches the players online.
type Online interface {
	// Dethrone takes hero status and every hero item from objectID's
	// player when it is online.
	Dethrone(objectID int32)
}

// Manager holds the heroes. It is safe for concurrent use.
type Manager struct {
	store      Store
	clans      Clans
	writes     Writer
	minMatches int
	now        func() time.Time
	log        zerolog.Logger

	// mu guards the fields below it; never held across I/O.
	mu     sync.Mutex
	online Online
	// heroes are the running era's heroes, allTime every hero ever
	// elected, both by character object id.
	heroes  map[int32]Hero
	allTime map[int32]Hero
}

// New returns a Manager persisting through store, its writes queued on
// writes (run inline when nil), and reading the clan names and crests the
// heroes show from clans. An election picks only nobles with at least
// minMatches matches. Restore then Start bring it up.
func New(store Store, clans Clans, writes Writer, minMatches int, now func() time.Time, log zerolog.Logger) *Manager {
	return &Manager{
		store: store, clans: clans, writes: writes, minMatches: minMatches, now: now, log: log,
		heroes: map[int32]Hero{}, allTime: map[int32]Hero{},
	}
}

// Restore loads the heroes: every stored one whose character exists, the
// running era's among them.
func (m *Manager) Restore(ctx context.Context) error {
	rows, err := m.store.LoadHeroes(ctx)
	if err != nil {
		return err
	}
	heroes, allTime := map[int32]Hero{}, map[int32]Hero{}
	for _, r := range rows {
		h := Hero{Name: r.Name, ClassID: r.ClassID, Count: r.Count, Played: r.Played, Active: r.Active}
		m.setClan(&h, r.ClanID)
		allTime[r.ObjectID] = h
		if r.Played {
			heroes[r.ObjectID] = h
		}
	}
	m.mu.Lock()
	m.heroes, m.allTime = heroes, allTime
	m.mu.Unlock()
	m.log.Info().Int("heroes", len(heroes)).Int("all_time", len(allTime)).Msg("hero: restored")
	return nil
}

// Start lets an election reach the heroes online through online.
func (m *Manager) Start(online Online) {
	m.mu.Lock()
	m.online = online
	m.mu.Unlock()
}

// setClan fills h's clan and alliance from clanID's clan; a character
// without a clan, or whose clan is gone, shows none.
func (m *Manager) setClan(h *Hero, clanID int32) {
	if clanID <= 0 || m.clans == nil {
		return
	}
	cl, ok := m.clans.Get(clanID)
	if !ok {
		return
	}
	info := cl.Info()
	h.ClanName, h.ClanCrest = info.Name, info.CrestID
	if info.AllyID > 0 {
		h.AllyName, h.AllyCrest = info.AllyName, info.AllyCrestID
	}
}

// IsActive reports whether objectID is a hero of the running era that
// claimed its status.
func (m *Manager) IsActive(objectID int32) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	h, ok := m.heroes[objectID]
	return ok && h.Active
}

// IsInactive reports whether objectID is a hero of the running era that
// has not claimed its status yet.
func (m *Manager) IsInactive(objectID int32) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	h, ok := m.heroes[objectID]
	return ok && !h.Active
}

// Heroes returns the running era's heroes ordered by class, then object
// id.
func (m *Manager) Heroes() []Hero {
	m.mu.Lock()
	ids := slices.Collect(maps.Keys(m.heroes))
	heroes := maps.Clone(m.heroes)
	m.mu.Unlock()
	slices.SortFunc(ids, func(a, b int32) int {
		return cmp.Or(cmp.Compare(heroes[a].ClassID, heroes[b].ClassID), cmp.Compare(a, b))
	})
	out := make([]Hero, len(ids))
	for i, id := range ids {
		out[i] = heroes[id]
	}
	return out
}

// Activate has objectID claim the hero status it was elected to, and
// returns its entry. It reports false, changing nothing, unless objectID
// is an inactive hero of the running era, so two claims never both
// succeed. The claim's diary entry and the heroes are stored behind it.
func (m *Manager) Activate(objectID int32) (Hero, bool) {
	m.mu.Lock()
	h, ok := m.heroes[objectID]
	if !ok || h.Active {
		m.mu.Unlock()
		return Hero{}, false
	}
	h.Active = true
	m.heroes[objectID] = h
	heroes := maps.Clone(m.heroes)
	m.mu.Unlock()
	m.AddDiaryEntry(objectID, DiaryHeroGained, 0)
	m.write("save heroes", func(ctx context.Context) error { return m.store.SaveHeroes(ctx, heroes) })
	return h, true
}

// AddDiaryEntry queues a diary entry for objectID, dated now.
func (m *Manager) AddDiaryEntry(objectID int32, action, param int) {
	at := m.now().UnixMilli()
	m.write("add diary entry", func(ctx context.Context) error {
		return m.store.AddDiaryEntry(ctx, objectID, at, action, param)
	})
}

// Elect replaces the running era's heroes with the best noble of each
// third-class profession, as the stored records rank them; it is run on
// the persistence lane, right behind the save of those records. Every
// stored hero leaves the running era, and the outgoing heroes online lose
// their status and hero items. A new hero is inactive. With nobody to
// elect the era is left without heroes; otherwise every hero item no game
// master owns is deleted and the heroes are stored. A re-elected hero
// keeps the class and clan of its first election and counts one more.
//
// The outgoing heroes leave as the election starts, so none can claim its
// status meanwhile and store the ending era's heroes behind the new ones.
func (m *Manager) Elect(ctx context.Context) {
	if err := m.store.ResetPlayed(ctx); err != nil {
		m.log.Error().Err(err).Msg("hero: reset the running era's heroes")
	}
	m.mu.Lock()
	outgoing := slices.Sorted(maps.Keys(m.heroes))
	clear(m.heroes)
	online := m.online
	m.mu.Unlock()
	if online != nil {
		for _, id := range outgoing {
			online.Dethrone(id)
		}
	}

	type pick struct {
		objectID int32
		name     string
		classID  int
	}
	var picks []pick
	for classID := range lastClassID + 1 {
		if level, ok := player.ClassLevel(classID); !ok || level != 3 {
			continue
		}
		id, name, found, err := m.store.BestNoble(ctx, classID, m.minMatches)
		if err != nil {
			// One failed read ends the election's reads; the heroes
			// picked so far stand.
			m.log.Error().Err(err).Int("class_id", classID).Msg("hero: pick the heroes to be")
			break
		}
		if found {
			picks = append(picks, pick{objectID: id, name: name, classID: classID})
		}
	}
	if len(picks) == 0 {
		return
	}

	m.mu.Lock()
	elected := make(map[int32]Hero, len(picks))
	for _, p := range picks {
		h, ok := m.allTime[p.objectID]
		if ok {
			h.Count++
			h.Played, h.Active = true, false
			m.allTime[p.objectID] = h
		} else {
			h = Hero{Name: p.name, ClassID: p.classID, Count: 1, Played: true}
		}
		elected[p.objectID] = h
	}
	m.mu.Unlock()

	if err := m.store.DeleteHeroItems(ctx); err != nil {
		m.log.Error().Err(err).Msg("hero: delete hero items")
	}
	for id, h := range elected {
		m.mu.Lock()
		_, known := m.allTime[id]
		m.mu.Unlock()
		if known {
			continue
		}
		clanID, found, err := m.store.ClanID(ctx, id)
		if err != nil {
			m.log.Error().Err(err).Int32("object_id", id).Msg("hero: read the new hero's clan")
		}
		if found {
			m.setClan(&h, clanID)
		}
		elected[id] = h
		m.mu.Lock()
		m.allTime[id] = h
		m.mu.Unlock()
	}
	m.mu.Lock()
	m.heroes = elected
	heroes := maps.Clone(elected)
	m.mu.Unlock()
	if err := m.store.SaveHeroes(ctx, heroes); err != nil {
		m.log.Error().Err(err).Msg("hero: save heroes")
	}
}

// write queues fn on the heroes' persistence lane, or runs it at once
// without a writer. Each write gets taskTimeout.
func (m *Manager) write(what string, fn func(context.Context) error) {
	log := m.log
	job := func() {
		ctx, cancel := context.WithTimeout(context.Background(), taskTimeout)
		defer cancel()
		if err := fn(ctx); err != nil {
			log.Error().Err(err).Msg("hero: " + what)
		}
	}
	if m.writes == nil {
		job()
		return
	}
	if !m.writes.Enqueue(writeLane, job) {
		log.Error().Msg("hero: " + what + ": write dropped")
	}
}
