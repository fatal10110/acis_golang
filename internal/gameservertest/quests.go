package gameservertest

import (
	"context"
	"database/sql"
	"sync"
	"testing"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

type scriptOptions struct {
	list    []script.Listing
	catalog script.Catalog
}

// WithScripts boots the script registry from list and catalog, as the
// production boot builds it from scripts.xml and its catalogs; without it
// the registry is empty. Every NPC id reads as having no template, so a
// script's NPC bindings are dropped: the scripts serve the quest journal.
func WithScripts(list []script.Listing, catalog script.Catalog) Option {
	return func(o *options) { o.scripts = &scriptOptions{list: list, catalog: catalog} }
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
}

// bootQuests builds the journal store, the script registry and the journal
// writer draining on worker.
func bootQuests(db *sql.DB, worker *persist.Worker, o *options) *questBoot {
	var list []script.Listing
	var catalog script.Catalog
	if o.scripts != nil {
		list, catalog = o.scripts.list, o.scripts.catalog
	}
	noTemplate := func(int32) (script.NPCKind, bool) { return script.KindOther, false }
	registry := script.Build(list, catalog, script.Config{KindOf: noTemplate, Log: o.log})
	store := &journalStore{QuestStore: gamesql.NewQuestStore(db), loadErr: o.questLoadErr}
	return &questBoot{store: store, registry: registry, journals: script.NewQuests(store, worker, o.log)}
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
	quest, ok := s.quests.registry.JournalQuest(name)
	if !ok {
		tb.Fatalf("no script %q registered", name)
	}
	sc := &script.Script{Name: quest.Name, QuestID: quest.ID}
	c := s.onlineCharacter(tb, objID)
	done := make(chan struct{})
	if !c.Queue().Post(func() {
		defer close(done)
		fn(s.quests.journals, c, sc)
	}) {
		tb.Fatalf("player %d's queue is closed", objID)
	}
	<-done
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
