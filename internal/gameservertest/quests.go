package gameservertest

import (
	"context"
	"database/sql"
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

type scriptOptions struct {
	list    []script.Listing
	catalog script.Catalog
	rates   *script.Rates
	rand    func(n int) int
	// singleItemDrop turns MultipleItemDrop off.
	singleItemDrop bool
	// kinds are the NPC ids that have a template, with its kind.
	kinds map[int32]script.NPCKind
}

// WithScripts boots the script registry from list and catalog, as the
// production boot builds it from scripts.xml and its catalogs; without it
// the registry is empty. Every NPC id reads as having no template, so a
// script's NPC bindings are dropped: the scripts serve the quest journal.
func WithScripts(list []script.Listing, catalog script.Catalog) Option {
	return func(o *options) {
		so := o.scriptHelpers()
		so.list, so.catalog = list, catalog
	}
}

// WithScriptRates sets the quest drop and reward rates the script helpers
// scale by; without it every rate is 1.
func WithScriptRates(r script.Rates) Option {
	return func(o *options) { o.scriptHelpers().rates = &r }
}

// WithScriptRand makes rand the random source of every script draw;
// without it the draws are random.
func WithScriptRand(rand func(n int) int) Option {
	return func(o *options) { o.scriptHelpers().rand = rand }
}

// WithScriptSingleItemDrop turns MultipleItemDrop off, so a script give of
// a non-stackable creates one instance whatever the count; without it the
// setting is on, as shipped.
func WithScriptSingleItemDrop() Option {
	return func(o *options) { o.scriptHelpers().singleItemDrop = true }
}

func (o *options) scriptHelpers() *scriptOptions {
	if o.scripts == nil {
		o.scripts = &scriptOptions{}
	}
	return o.scripts
}

// WithNPCScripts is WithScripts where each NPC id in kinds has a template
// of that kind, so the scripts' bindings to those ids hold and the hooks
// they subscribe to must be raised for that kind.
func WithNPCScripts(kinds map[int32]script.NPCKind, list []script.Listing, catalog script.Catalog) Option {
	return func(o *options) {
		so := o.scriptHelpers()
		so.list, so.catalog, so.kinds = list, catalog, kinds
	}
}

// WithQuestLoadFault makes every quest journal read at a character
// selection fail with err.
func WithQuestLoadFault(err error) Option {
	return func(o *options) { o.questLoadErr = err }
}

// WithMemoLoadFault makes every memo read at a character selection fail
// with err.
func WithMemoLoadFault(err error) Option {
	return func(o *options) { o.memoLoadErr = err }
}

// WithRespawnRestoreHP sets the players.properties RespawnRestoreHP: the
// share of max HP a revive restores (default 0.7).
func WithRespawnRestoreHP(share float64) Option {
	return func(o *options) { o.respawnRestoreHP = share }
}

// memoStore is the real memo store, with the read fault a suite sets.
type memoStore struct {
	*gamesql.MemoStore
	loadErr error
}

func (s memoStore) ListMemos(ctx context.Context, ownerID int32) (map[string]string, error) {
	if s.loadErr != nil {
		return nil, s.loadErr
	}
	return s.MemoStore.ListMemos(ctx, ownerID)
}

// questBoot is the quest journal wiring Boot hands the link.
type questBoot struct {
	store    *journalStore
	registry *script.Registry
	journals *script.Quests
	env      *script.Env
}

// bootQuests builds the journal store, the journal writer draining on
// worker, the script helpers' environment allocating item ids from ids,
// and the script registry, whose timers bound to no NPC or player queue
// run on timers.
func bootQuests(db *sql.DB, worker *persist.Worker, ids *sequentialIDs, timers *sim.Queue, o *options) *questBoot {
	so := o.scriptHelpers()
	store := &journalStore{QuestStore: gamesql.NewQuestStore(db), loadErr: o.questLoadErr}
	journals := script.NewQuests(store, worker, o.log)
	env := &script.Env{
		Quests:           journals,
		Rates:            script.Rates{Drop: 1, Reward: 1, RewardAdena: 1, XP: 1, SP: 1},
		PartyRange:       fixturePartyRange,
		MultipleItemDrop: !so.singleItemDrop,
		NewItemID:        ids.NextID,
		Rand:             rnd.Get,
	}
	if so.rates != nil {
		env.Rates = *so.rates
	}
	if so.rand != nil {
		env.Rand = so.rand
	}
	kindOf := func(id int32) (script.NPCKind, bool) {
		k, ok := so.kinds[id]
		return k, ok
	}
	registry := script.Build(so.list, so.catalog, script.Config{KindOf: kindOf, Log: o.log, Env: env, Queue: timers})
	return &questBoot{store: store, registry: registry, journals: journals, env: env}
}

// journalStore is the real journal store, with the faults a suite sets and
// a record of the writes it applied.
type journalStore struct {
	*gamesql.QuestStore
	loadErr error

	mu       sync.Mutex
	writeErr error
	applied  []questlog.Write
}

func (s *journalStore) ListByOwner(ctx context.Context, ownerID int32) ([]questlog.Row, error) {
	if s.loadErr != nil {
		return nil, s.loadErr
	}
	return s.QuestStore.ListByOwner(ctx, ownerID)
}

func (s *journalStore) ApplyJournal(ctx context.Context, ownerID int32, writes []questlog.Write) error {
	s.mu.Lock()
	err := s.writeErr
	s.mu.Unlock()
	if err != nil {
		return err
	}
	if err := s.QuestStore.ApplyJournal(ctx, ownerID, writes); err != nil {
		return err
	}
	s.mu.Lock()
	s.applied = append(s.applied, writes...)
	s.mu.Unlock()
	return nil
}

// FailJournalWrites makes every quest journal drain fail with err from now
// on, applying nothing; nil lets drains through again.
func (s *Server) FailJournalWrites(err error) {
	s.quests.store.mu.Lock()
	s.quests.store.writeErr = err
	s.quests.store.mu.Unlock()
}

// TakeJournalWrites returns, in order, the quest journal writes applied to
// the database since the last call.
func (s *Server) TakeJournalWrites() []questlog.Write {
	s.quests.store.mu.Lock()
	defer s.quests.store.mu.Unlock()
	out := s.quests.store.applied
	s.quests.store.applied = nil
	return out
}

// RunQuest runs fn on the online player objID's queue, where a script hook
// runs, and waits for it. fn gets the script engine's journal writer, the
// player and the script WithScripts registered under name.
func (s *Server) RunQuest(tb testing.TB, objID int32, name string, fn func(q *script.Quests, c *player.Character, sc *script.Script)) {
	tb.Helper()
	c := s.onlineCharacter(tb, objID)
	s.runScript(tb, c, name, func(sc *script.Script) { fn(s.quests.journals, c, sc) })
}

// RunScript runs fn on the online player objID's queue as one invocation
// of the script WithScripts registered under name, the way a hook runs,
// and waits for it: fn gets the script and a handle on the player. A panic
// in fn is recovered and logged, as a hook's is; RunScript reports whether
// fn returned.
func (s *Server) RunScript(tb testing.TB, objID int32, name string, fn func(sc *script.Script, p *script.Player)) bool {
	tb.Helper()
	obj, ok := s.State.Player(objID)
	if !ok {
		tb.Fatalf("world.Player(%d) missing", objID)
	}
	self, ok := obj.(attackable.Combatant)
	if !ok {
		tb.Fatalf("world.Player(%d) = %T is not a combatant", objID, obj)
	}
	return s.runScript(tb, s.onlineCharacter(tb, objID), name, func(sc *script.Script) { fn(sc, script.PlayerOf(self)) })
}

// runScript runs fn on c's queue as one invocation of the script name and
// waits for it, reporting whether fn returned.
func (s *Server) runScript(tb testing.TB, c *player.Character, name string, fn func(sc *script.Script)) bool {
	tb.Helper()
	if _, ok := s.quests.registry.JournalQuest(name); !ok {
		tb.Fatalf("no script %q registered", name)
	}
	done := make(chan bool, 1)
	if !c.Queue().Post(func() { done <- s.quests.registry.Invoke(name, fn) }) {
		tb.Fatalf("player %d's queue is closed", c.ObjectID())
	}
	return <-done
}

// QuestVars returns the online player objID's variables in the quest named
// name, nil when it has no state in that quest.
func (s *Server) QuestVars(tb testing.TB, objID int32, name string) map[string]string {
	tb.Helper()
	st := s.onlineCharacter(tb, objID).Quests().State(name)
	if st == nil {
		return nil
	}
	return st.Vars()
}

// PlayerItemCount returns how many units of templateID the online player
// objID holds: a stack's count, or the number of instances.
func (s *Server) PlayerItemCount(tb testing.TB, objID, templateID int32) int {
	tb.Helper()
	return s.onlineCharacter(tb, objID).ItemCount(int(templateID))
}
