package script

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
)

// JournalWriteTimeout bounds one drain of a player's journal writes.
const JournalWriteTimeout = 2 * time.Second

// errJournalUnwritten reports a sealed journal whose writes did not all land.
var errJournalUnwritten = errors.New("quest journal writes not applied")

// JournalStore applies a player's journal writes to character_quests, in
// order, in one transaction.
type JournalStore interface {
	ApplyJournal(ctx context.Context, ownerID int32, writes []questlog.Write) error
}

// Quests writes players' quest journals. A change to a journal is made in
// memory at once and its rows are written by one drain job on the player's
// persistence lane, so the database sees the changes in the order they were
// made. A player leaving the world seals its journal; a sealed journal whose
// writes have not all landed is kept here until a drain applies them, which
// the player's next selection waits for.
type Quests struct {
	store JournalStore
	lanes *persist.Worker
	log   zerolog.Logger

	mu sync.Mutex
	// sealed holds, by owner, the sealed journals that may still owe writes.
	sealed map[int32]*questlog.Journal
}

// NewQuests returns the journal writer over store, draining on lanes.
func NewQuests(store JournalStore, lanes *persist.Worker, log zerolog.Logger) *Quests {
	return &Quests{store: store, lanes: lanes, log: log, sealed: make(map[int32]*questlog.Journal)}
}

// QuestState is a script's handle on one player's state in one quest.
type QuestState struct {
	quests *Quests
	player *player.Character
	state  *questlog.State
}

// State returns c's state in s, nil when c has none.
func (q *Quests) State(c *player.Character, s *Script) *QuestState {
	return q.stateNamed(c, s.Name)
}

// stateNamed returns c's state in the quest named name, nil when c has
// none.
func (q *Quests) stateNamed(c *player.Character, name string) *QuestState {
	st := c.Quests().State(name)
	if st == nil {
		return nil
	}
	return &QuestState{quests: q, player: c, state: st}
}

// QuestState returns p's state in the quest named name, nil when p has
// none.
func (s *Script) QuestState(p *Player, name string) *QuestState {
	return s.env.Quests.stateNamed(p.character(), name)
}

// NewState adds a state of s to c's journal, created, and returns it. It
// writes nothing.
func (q *Quests) NewState(c *player.Character, s *Script) *QuestState {
	st := c.Quests().Create(questlog.Quest{Name: s.Name, ID: s.QuestID, Items: s.Items})
	return &QuestState{quests: q, player: c, state: st}
}

// Abort exits, as repeatable, the first quest of c's journal whose quest id
// is questID. A quest id c has no state of does nothing; the reference
// aborts by id in the same way.
func (q *Quests) Abort(c *player.Character, questID int32) {
	st := c.Quests().StateByID(questID)
	if st == nil {
		return
	}
	(&QuestState{quests: q, player: c, state: st}).Exit(true)
}

// Exit exits c's state in the quest named name, as QuestState.Exit does. A
// quest c has no state in does nothing.
func (q *Quests) Exit(c *player.Character, name string, repeatable bool) {
	if qs := q.stateNamed(c, name); qs != nil {
		qs.Exit(repeatable)
	}
}

// Get returns the state's variable key.
func (qs *QuestState) Get(key string) (string, bool) { return qs.state.Get(key) }

// Status returns the state's status.
func (qs *QuestState) Status() questlog.Status { return qs.state.Status() }

// Cond returns the quest's condition, 0 when it has none. A condition that
// is not a 32-bit integer panics.
func (qs *QuestState) Cond() int32 {
	v, ok := qs.state.Get(questlog.KeyCond)
	if !ok {
		return 0
	}
	n, err := strconv.ParseInt(v, 10, 32)
	if err != nil {
		panic(fmt.Sprintf("script: quest %s condition %q: %v", qs.state.Quest().Name, v, err))
	}
	return int32(n)
}

// Player returns the handle on the state's player.
func (qs *QuestState) Player() *Player {
	self, _ := qs.player.WorldHandle().(attackable.Combatant)
	return PlayerOf(self)
}

// Set sets the variable key to value and writes it.
func (qs *QuestState) Set(key, value string) {
	if qs.state.Set(key, value) {
		qs.quests.commit(qs.player)
	}
}

// Unset removes the variable key and deletes its row.
func (qs *QuestState) Unset(key string) {
	if qs.state.Unset(key) {
		qs.quests.commit(qs.player)
	}
}

// SetStatus sets the state's status; an unchanged status writes nothing.
func (qs *QuestState) SetStatus(s questlog.Status) {
	if qs.state.SetStatus(s) {
		qs.quests.commit(qs.player)
	}
}

// SetCond sets the quest's condition, with its quest window flags. A real
// quest then shows the player its quest window and marks the quest.
func (qs *QuestState) SetCond(cond int32) {
	list, ok := qs.state.SetCond(cond)
	if !ok {
		return
	}
	qs.quests.commit(qs.player)
	if q := qs.state.Quest(); q.Real() {
		qs.player.NotifyQuestList(list)
		qs.player.NotifyQuestMarked(q.ID)
	}
}

// Exit ends a started quest: a repeatable one is forgotten, any other is
// kept completed. A real quest then shows the player its quest window, and
// the player loses every unit of the quest's items.
func (qs *QuestState) Exit(repeatable bool) {
	list, ok := qs.state.Exit(repeatable)
	if !ok {
		return
	}
	qs.quests.commit(qs.player)
	q := qs.state.Quest()
	if q.Real() {
		qs.player.NotifyQuestList(list)
	}
	for _, id := range q.Items {
		qs.player.TakeScriptItems(id, -1)
	}
}

// commit starts a drain of c's journal unless one is already owed.
func (q *Quests) commit(c *player.Character) {
	q.schedule(c.ObjectID(), c.Quests())
}

// schedule enqueues a drain of j on ownerID's lane unless one is already
// owed. A worker that refuses the job leaves the writes owed for a later
// drain.
func (q *Quests) schedule(ownerID int32, j *questlog.Journal) {
	if !j.ScheduleDrain() {
		return
	}
	if !q.lanes.Enqueue(ownerID, func() { _ = q.drain(ownerID, j) }) {
		j.Applied(j.TakePending(), false)
	}
}

// drain applies the writes j owes, on ownerID's lane. A sealed journal left
// with nothing owed is forgotten.
func (q *Quests) drain(ownerID int32, j *questlog.Journal) error {
	if writes := j.TakePending(); len(writes) > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), JournalWriteTimeout)
		err := q.store.ApplyJournal(ctx, ownerID, writes)
		cancel()
		j.Applied(writes, err == nil)
		if err != nil {
			q.log.Error().Err(err).Int32("object_id", ownerID).Int("writes", len(writes)).Msg("write quest journal")
			return err
		}
	}
	q.forget(ownerID, j)
	return nil
}

// forget drops j from the sealed journals once it owes nothing.
func (q *Quests) forget(ownerID int32, j *questlog.Journal) {
	if j.Dirty() {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.sealed[ownerID] == j {
		delete(q.sealed, ownerID)
	}
}

// Seal ends c's journal writes as c leaves the world, once c's
// MarkDetaching has sealed the journal: the writes it still owes are
// drained, and the journal is kept until they have landed.
func (q *Quests) Seal(c *player.Character) {
	id, j := c.ObjectID(), c.Quests()
	j.Seal()
	q.mu.Lock()
	q.sealed[id] = j
	q.mu.Unlock()
	q.schedule(id, j)
	q.forget(id, j)
}

// Settle runs before a selection of ownerID loads its journal, once its
// persistence lane holds nothing an earlier session queued: a sealed
// journal that still owes writes is drained first. An error means the
// writes did not land, and the selection must not load the journal.
func (q *Quests) Settle(ctx context.Context, ownerID int32) error {
	q.mu.Lock()
	j := q.sealed[ownerID]
	q.mu.Unlock()
	if j == nil {
		return nil
	}
	done := make(chan error, 1)
	if !q.lanes.Enqueue(ownerID, func() { done <- q.drain(ownerID, j) }) {
		return errJournalUnwritten
	}
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// DrainSealed enqueues a last drain of every sealed journal that still owes
// writes, for the shutdown to run before the persistence worker closes.
func (q *Quests) DrainSealed() {
	q.mu.Lock()
	sealed := make(map[int32]*questlog.Journal, len(q.sealed))
	for id, j := range q.sealed {
		sealed[id] = j
	}
	q.mu.Unlock()
	for id, j := range sealed {
		q.schedule(id, j)
	}
}
