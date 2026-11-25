// Package schemebuffer is the scheme buffer NPC: the buffs it offers, the
// named buff schemes each player keeps with it, and its dialog.
package schemebuffer

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/rs/zerolog"
)

// Config is the npcs.properties scheme buffer section.
type Config struct {
	// MaxSchemes is BufferMaxSchemesPerChar: the schemes one player may
	// keep.
	MaxSchemes int
	// StaticCost is BufferStaticCostPerBuff: when above 0, every buff of a
	// scheme costs this much instead of its own price.
	StaticCost int
}

// DefaultConfig is the shipped scheme buffer section.
func DefaultConfig() Config { return Config{MaxSchemes: 4, StaticCost: -1} }

// Row is one buffer_schemes row: a player's scheme and its skill ids as
// stored, comma separated.
type Row struct {
	OwnerID int32
	Name    string
	Skills  string
}

// scheme is one named list of buff skill ids.
type scheme struct {
	name   string
	skills []int32
}

// owner is one player's schemes, kept in name order ignoring case. A
// player gets one the first time a scheme is set for them, kept even once
// emptied.
type owner struct {
	schemes []*scheme
}

// Store replaces one player's stored schemes.
type Store interface {
	// SaveOwner replaces every stored scheme of ownerID with rows.
	SaveOwner(ctx context.Context, ownerID int32, rows []Row) error
}

// Writer runs a database job later, on ownerID's persistence lane, so a
// dialog never waits on the database.
type Writer interface {
	Enqueue(ownerID int32, job func()) bool
}

// writeTimeout bounds one player's schemes write.
const writeTimeout = 10 * time.Second

// Manager holds the buffs the scheme buffer offers and every player's
// schemes. Its methods are safe for concurrent use.
type Manager struct {
	cfg    Config
	buffs  *skill.BufferTable
	skills *skill.Table

	store  Store
	writes Writer
	log    zerolog.Logger

	mu     sync.Mutex
	owners map[int32]*owner
}

// Option configures a Manager.
type Option func(*Manager)

// WithStore has every change to a player's schemes written to store on
// that player's lane of writes, or at once without a writer. Without it
// the schemes are kept in memory only.
func WithStore(store Store, writes Writer, log zerolog.Logger) Option {
	return func(m *Manager) { m.store, m.writes, m.log = store, writes, log }
}

// New returns a Manager offering buffs, whose level-1 names it reads in
// skills. Either may be nil: nothing is then offered, or named.
func New(cfg Config, buffs *skill.BufferTable, skills *skill.Table, opts ...Option) *Manager {
	m := &Manager{cfg: cfg, buffs: buffs, skills: skills, log: zerolog.Nop(), owners: make(map[int32]*owner)}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// Restore loads the stored schemes, in row order. A stored skill id the
// buffer no longer offers is dropped; an empty id ends its scheme's list.
// A player's schemes past the configured maximum are dropped. A skill
// list that does not parse stops the load: the schemes restored before it
// stay, and the error is logged.
func (m *Manager) Restore(rows []Row, log zerolog.Logger) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, row := range rows {
		var ids []int32
		for _, text := range splitSkillList(row.Skills) {
			if text == "" {
				break
			}
			id, err := strconv.ParseInt(text, 10, 32)
			if err != nil {
				log.Error().Err(err).Int32("object_id", row.OwnerID).Str("scheme", row.Name).Msg("scheme buffer: failed to load schemes data")
				return
			}
			if m.offers(int32(id)) {
				ids = append(ids, int32(id))
			}
		}
		m.setLocked(row.OwnerID, row.Name, ids)
	}
}

// Rows returns every scheme as buffer_schemes stores it, players in
// object id order.
func (m *Manager) Rows() []Row {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := make([]int32, 0, len(m.owners))
	for id := range m.owners {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	var out []Row
	for _, id := range ids {
		out = append(out, m.rowsLocked(id)...)
	}
	return out
}

// rowsLocked returns ownerID's schemes as buffer_schemes stores them.
func (m *Manager) rowsLocked(ownerID int32) []Row {
	o := m.owners[ownerID]
	if o == nil {
		return nil
	}
	out := make([]Row, 0, len(o.schemes))
	for _, s := range o.schemes {
		parts := make([]string, len(s.skills))
		for i, sk := range s.skills {
			parts[i] = strconv.Itoa(int(sk))
		}
		out = append(out, Row{OwnerID: ownerID, Name: s.name, Skills: strings.Join(parts, ",")})
	}
	return out
}

// Forget drops every scheme of the player ownerID, a deleted character,
// so the shutdown save does not store them again (#4676). The emptied list
// is written too, behind any write of that player still queued, so a write
// queued before the character was purged cannot store its schemes again.
func (m *Manager) Forget(ownerID int32) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.owners, ownerID)
	m.saveLocked(ownerID)
}

// saveLocked writes ownerID's schemes as they stand, replacing its stored
// ones, so a change outlives a server stopped before its shutdown save
// (#4699). Every write of one player goes to that player's lane with the
// rows taken when it was queued, so the last change lands last.
func (m *Manager) saveLocked(ownerID int32) {
	if m.store == nil {
		return
	}
	rows, store, log := m.rowsLocked(ownerID), m.store, m.log
	job := func() {
		ctx, cancel := context.WithTimeout(context.Background(), writeTimeout)
		defer cancel()
		if err := store.SaveOwner(ctx, ownerID, rows); err != nil {
			log.Error().Err(err).Int32("object_id", ownerID).Msg("scheme buffer: failed to save schemes")
		}
	}
	if m.writes == nil {
		job()
		return
	}
	if !m.writes.Enqueue(ownerID, job) {
		log.Error().Int32("object_id", ownerID).Msg("scheme buffer: failed to save schemes: write dropped")
	}
}

// offers reports whether skill id is one of the buffer's buffs.
func (m *Manager) offers(id int32) bool {
	_, ok := m.buff(id)
	return ok
}

// buff returns the buffer's entry for skill id.
func (m *Manager) buff(id int32) (skill.BufferSkill, bool) {
	if m.buffs == nil {
		return skill.BufferSkill{}, false
	}
	return m.buffs.Skill(id)
}

// setLocked gives ownerID's scheme name the skill list ids, creating the
// player's schemes on first use. A name already kept, in any case, keeps
// its spelling and takes the new list. Nothing is set once the player
// keeps the configured maximum, even a name already kept.
func (m *Manager) setLocked(ownerID int32, name string, ids []int32) {
	o := m.owners[ownerID]
	if o == nil {
		o = &owner{}
		m.owners[ownerID] = o
	}
	if len(o.schemes) >= m.cfg.MaxSchemes {
		return
	}
	i, found := o.find(name)
	if found {
		o.schemes[i].skills = ids
		return
	}
	o.schemes = append(o.schemes, nil)
	copy(o.schemes[i+1:], o.schemes[i:])
	o.schemes[i] = &scheme{name: name, skills: ids}
}

// find returns the index of name in o's schemes, compared ignoring case,
// or where it would go.
func (o *owner) find(name string) (int, bool) {
	i := sort.Search(len(o.schemes), func(i int) bool { return compareIgnoreCase(o.schemes[i].name, name) >= 0 })
	return i, i < len(o.schemes) && compareIgnoreCase(o.schemes[i].name, name) == 0
}

// lookup returns ownerID's scheme name, compared ignoring case.
func (m *Manager) lookup(ownerID int32, name string) (*scheme, bool) {
	o := m.owners[ownerID]
	if o == nil {
		return nil, false
	}
	i, ok := o.find(name)
	if !ok {
		return nil, false
	}
	return o.schemes[i], true
}

// splitSkillList splits s on commas as the stored skill list is read:
// trailing empty fields are dropped, and an empty s is one empty field.
func splitSkillList(s string) []string {
	parts := strings.Split(s, ",")
	if s == "" {
		return parts
	}
	for len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	return parts
}

// compareIgnoreCase orders a and b by UTF-16 code unit, two units being
// equal when they match as is, upper-cased or lower-cased; a string that
// is a prefix of the other sorts first.
func compareIgnoreCase(a, b string) int {
	ua, ub := commons.UTF16Units(a), commons.UTF16Units(b)
	for i := 0; i < len(ua) && i < len(ub); i++ {
		c1, c2 := rune(ua[i]), rune(ub[i])
		if c1 == c2 {
			continue
		}
		c1, c2 = toUpper16(c1), toUpper16(c2)
		if c1 == c2 {
			continue
		}
		c1, c2 = toLower16(c1), toLower16(c2)
		if c1 != c2 {
			return int(c1) - int(c2)
		}
	}
	return len(ua) - len(ub)
}

// toUpper16 and toLower16 map one UTF-16 code unit; a mapping leaving the
// Basic Multilingual Plane keeps the unit.
func toUpper16(r rune) rune {
	if u := unicode.ToUpper(r); u <= 0xFFFF {
		return u
	}
	return r
}

func toLower16(r rune) rune {
	if u := unicode.ToLower(r); u <= 0xFFFF {
		return u
	}
	return r
}
