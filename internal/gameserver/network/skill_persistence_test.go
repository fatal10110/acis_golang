package network

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// TestSkillPersistenceSaveWritesLiveEffectsAndReuseTimers proves Save reads
// straight off the live effect list, not a separate registry: the effect
// here is added the way a real skill cast lands one on a player
// (effect.List.Add via applyEffects), never through a restore path, so a
// regression back to the old write-only registry (which only RestoreSkillEffect
// ever populated) would leave this row empty.
func TestSkillPersistenceSaveWritesLiveEffectsAndReuseTimers(t *testing.T) {
	store := newMemorySkillSaveStore()
	now := time.Now().Truncate(time.Millisecond)
	c := skillPersistenceCharacter(1001)
	attachSkillPersistenceLive(t, c)
	def := modelskill.Definition{ID: 1204, Level: 2, Effects: []modelskill.EffectTemplate{{Name: "Buff", Count: 2, Time: 30}}}

	e, err := effect.New(effect.SkillFromDefinition(def), def.Effects[0])
	if err != nil {
		t.Fatalf("effect.New() error = %v", err)
	}
	e.Effector, e.Effected = c, c
	c.EffectList().Add(e)
	c.SetSkillReuse(skillRef(1204, 2), 1204*256+2, 45*time.Second, now.Add(45*time.Second))

	p := skillstate.NewPersistenceWithClock(store, skillTable(def), func() time.Time { return now })

	if err := p.Save(context.Background(), c, true); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got := store.rowsFor(c.ID, 0)
	want := []effect.SaveRow{{
		Skill:         skillRef(1204, 2),
		EffectCount:   2,
		EffectCurTime: 0,
		ReuseDelay:    45_000,
		SystemTime:    now.Add(45 * time.Second).UnixMilli(),
		RestoreType:   effect.RestoreTypeEffect,
		ClassIndex:    0,
		BuffIndex:     1,
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("saved rows = %+v, want %+v", got, want)
	}
}

// attachSkillPersistenceLive gives c a live effect list, the way spawning a
// real player does, so tests can exercise Save/ReplayEffects against actual
// effect.List state rather than the persistence registry directly.
func attachSkillPersistenceLive(t *testing.T, c *player.Character) {
	t.Helper()
	live, err := creature.NewLive(location.Location{}, 0, testGeo{}, c)
	if err != nil {
		t.Fatal(err)
	}
	c.Live = live
}

// TestSkillPersistenceSaveDropsAnExpiredRestoredEffectRatherThanResurrectingIt
// proves the other half of issue #1234: a restored effect that expires
// during the live session must not be re-persisted and replayed at every
// future relog. Before the fix, Save read the write-only restore registry
// (which nothing ever removed from), so an effect that had already expired
// on the live list kept getting saved and resurrected indefinitely; reading
// the live list instead means an expired effect — simply absent from it —
// can no longer be saved.
func TestSkillPersistenceSaveDropsAnExpiredRestoredEffectRatherThanResurrectingIt(t *testing.T) {
	store := newMemorySkillSaveStore()
	now := time.Now().Truncate(time.Millisecond)
	def := modelskill.Definition{ID: 1077, Level: 1, Effects: []modelskill.EffectTemplate{{Name: "Buff", Count: 1, Time: 1}}}
	store.seed(1007, 0, []effect.SaveRow{{
		Skill: skillRef(1077, 1), EffectCount: 1, EffectCurTime: 1,
		RestoreType: effect.RestoreTypeEffect, BuffIndex: 1,
	}})

	c := skillPersistenceCharacter(1007)
	p := skillstate.NewPersistenceWithClock(store, skillTable(def), func() time.Time { return now })
	if err := p.Restore(context.Background(), c); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}

	attachSkillPersistenceLive(t, c)
	p.ReplayEffects(c)
	if _, ok := c.EffectList().ActiveBySkillID(1077); !ok {
		t.Fatal("ReplayEffects did not apply the restored effect")
	}

	// The restored effect's remaining elapsed time (1s) already meets its
	// full period (1s), so it was seeded to fire and expire on its very
	// first tick — mirroring a buff that naturally ran out during the live
	// session.
	c.EffectList().Tick()
	if _, ok := c.EffectList().ActiveBySkillID(1077); ok {
		t.Fatal("effect still active after its tick should have expired it")
	}

	if err := p.Save(context.Background(), c, true); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if got := store.rowsFor(c.ID, 0); len(got) != 0 {
		t.Fatalf("saved rows for an expired effect = %+v, want none", got)
	}
}

func TestSkillPersistenceRestoreReinstatesEffectsAndReuseThenDeletesRows(t *testing.T) {
	store := newMemorySkillSaveStore()
	now := time.Now().Truncate(time.Millisecond)
	c := skillPersistenceCharacter(1002)
	store.seed(c.ID, 0, []effect.SaveRow{{
		Skill:         skillRef(1040, 3),
		EffectCount:   2,
		EffectCurTime: 15,
		ReuseDelay:    60_000,
		SystemTime:    now.Add(60 * time.Second).UnixMilli(),
		RestoreType:   effect.RestoreTypeEffect,
		BuffIndex:     1,
	}})

	p := skillstate.NewPersistenceWithClock(store, skillTable(
		modelskill.Definition{ID: 1040, Level: 3, Effects: []modelskill.EffectTemplate{{Name: "Buff"}}},
	), func() time.Time { return now })

	if err := p.Restore(context.Background(), c); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}

	if !c.SkillDisabled(1040*256 + 3) {
		t.Fatal("restored reuse key is not disabled")
	}
	effects := c.ActiveSkillEffects()
	wantEffects := []effect.ActiveEffect{{Skill: skillRef(1040, 3), ReuseGroup: 1040*256 + 3, Count: 2, Time: 15}}
	if !reflect.DeepEqual(effects, wantEffects) {
		t.Fatalf("restored effects = %+v, want %+v", effects, wantEffects)
	}
	if got := store.rowsFor(c.ID, 0); len(got) != 0 {
		t.Fatalf("rows after restore = %+v, want deleted", got)
	}
	if store.deleted != 1 {
		t.Fatalf("delete calls = %d, want 1", store.deleted)
	}
}

func TestSkillPersistenceRestoreSkipsStaleSkillAndDeletesRows(t *testing.T) {
	store := newMemorySkillSaveStore()
	now := time.Now().Truncate(time.Millisecond)
	c := skillPersistenceCharacter(1003)
	store.seed(c.ID, 0, []effect.SaveRow{{
		Skill:       skillRef(9999, 1),
		SystemTime:  now.Add(60 * time.Second).UnixMilli(),
		RestoreType: effect.RestoreTypeEffect,
		BuffIndex:   1,
	}})

	p := skillstate.NewPersistenceWithClock(store, skillTable(), func() time.Time { return now })

	if err := p.Restore(context.Background(), c); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}

	if effects := c.ActiveSkillEffects(); len(effects) != 0 {
		t.Fatalf("restored effects = %+v, want none", effects)
	}
	if timers := c.SkillReuseTimers(now); len(timers) != 0 {
		t.Fatalf("restored reuse timers = %+v, want none", timers)
	}
	if got := store.rowsFor(c.ID, 0); len(got) != 0 {
		t.Fatalf("rows after stale restore = %+v, want deleted", got)
	}
}

func TestSkillPersistenceRestoreReuseOnlyDoesNotRestoreEffect(t *testing.T) {
	store := newMemorySkillSaveStore()
	now := time.Now().Truncate(time.Millisecond)
	c := skillPersistenceCharacter(1004)
	store.seed(c.ID, 0, []effect.SaveRow{{
		Skill:         skillRef(1056, 1),
		EffectCount:   -1,
		EffectCurTime: -1,
		ReuseDelay:    90_000,
		SystemTime:    now.Add(90 * time.Second).UnixMilli(),
		RestoreType:   effect.RestoreTypeReuseOnly,
		BuffIndex:     1,
	}})

	p := skillstate.NewPersistenceWithClock(store, skillTable(
		modelskill.Definition{ID: 1056, Level: 1, Effects: []modelskill.EffectTemplate{{Name: "Buff"}}},
	), func() time.Time { return now })

	if err := p.Restore(context.Background(), c); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}

	if !c.SkillDisabled(1056*256 + 1) {
		t.Fatal("restored reuse-only key is not disabled")
	}
	if effects := c.ActiveSkillEffects(); len(effects) != 0 {
		t.Fatalf("restored effects = %+v, want none for reuse-only row", effects)
	}
	if got := store.rowsFor(c.ID, 0); len(got) != 0 {
		t.Fatalf("rows after reuse-only restore = %+v, want deleted", got)
	}
}

func TestSkillPersistenceRestoreDeletesExpiredRowsWithoutReinstatingReuse(t *testing.T) {
	store := newMemorySkillSaveStore()
	now := time.Now().Truncate(time.Millisecond)
	c := skillPersistenceCharacter(1005)
	store.seed(c.ID, 0, []effect.SaveRow{{
		Skill:       skillRef(1068, 1),
		ReuseDelay:  30_000,
		SystemTime:  now.Add(5 * time.Millisecond).UnixMilli(),
		RestoreType: effect.RestoreTypeReuseOnly,
		BuffIndex:   1,
	}})

	p := skillstate.NewPersistenceWithClock(store, skillTable(
		modelskill.Definition{ID: 1068, Level: 1},
	), func() time.Time { return now })

	if err := p.Restore(context.Background(), c); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}

	if c.SkillDisabled(1068*256 + 1) {
		t.Fatal("expired reuse key was disabled")
	}
	if got := store.rowsFor(c.ID, 0); len(got) != 0 {
		t.Fatalf("rows after expired restore = %+v, want deleted", got)
	}
}

func TestSkillPersistenceRestoreLoadsKnownSkillLevels(t *testing.T) {
	store := newMemorySkillSaveStore()
	c := skillPersistenceCharacter(1006)
	store.seedKnown(c.ID, 0, player.SkillLevels{
		248:  1,
		9999: 1,
	})
	p := skillstate.NewPersistence(store, skillTable(
		modelskill.Definition{ID: 248, Level: 1, Activation: modelskill.ActivationPassive},
	), store)

	if err := p.Restore(context.Background(), c); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}

	if got := c.SkillLevel(248); got != 1 {
		t.Fatalf("SkillLevel(248) = %d, want 1", got)
	}
	if got := c.SkillLevel(9999); got != 0 {
		t.Fatalf("stale SkillLevel(9999) = %d, want 0", got)
	}
}



func skillPersistenceCharacter(id int32) *player.Character {
	return &player.Character{ID: id, Name: "char", ClassID: 0, BaseClassID: 0}
}

func skillRef(id modelskill.ID, level int) modelskill.Ref {
	return modelskill.Ref{ID: id, Level: level}
}

func skillTable(defs ...modelskill.Definition) *modelskill.Table {
	return modelskill.NewTable(defs)
}

type memorySkillSaveStore struct {
	mu      sync.Mutex
	rows    map[skillSaveKey][]effect.SaveRow
	known   map[skillSaveKey]player.SkillLevels
	deleted int
}

type skillSaveKey struct {
	charObjID  int32
	classIndex int32
}

func newMemorySkillSaveStore() *memorySkillSaveStore {
	return &memorySkillSaveStore{rows: make(map[skillSaveKey][]effect.SaveRow), known: make(map[skillSaveKey]player.SkillLevels)}
}

func (s *memorySkillSaveStore) Replace(_ context.Context, charObjID int32, classIndex int32, rows []effect.SaveRow) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rows[skillSaveKey{charObjID: charObjID, classIndex: classIndex}] = append([]effect.SaveRow(nil), rows...)
	return nil
}

func (s *memorySkillSaveStore) ListByCharacter(_ context.Context, charObjID int32, classIndex int32) ([]effect.SaveRow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rowsForLocked(charObjID, classIndex), nil
}

func (s *memorySkillSaveStore) DeleteByCharacter(_ context.Context, charObjID int32, classIndex int32) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := skillSaveKey{charObjID: charObjID, classIndex: classIndex}
	n := int64(len(s.rows[key]))
	delete(s.rows, key)
	s.deleted++
	return n, nil
}

func (s *memorySkillSaveStore) seed(charObjID int32, classIndex int32, rows []effect.SaveRow) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rows[skillSaveKey{charObjID: charObjID, classIndex: classIndex}] = append([]effect.SaveRow(nil), rows...)
}

func (s *memorySkillSaveStore) rowsFor(charObjID int32, classIndex int32) []effect.SaveRow {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rowsForLocked(charObjID, classIndex)
}

func (s *memorySkillSaveStore) rowsForLocked(charObjID int32, classIndex int32) []effect.SaveRow {
	return append([]effect.SaveRow(nil), s.rows[skillSaveKey{charObjID: charObjID, classIndex: classIndex}]...)
}

func (s *memorySkillSaveStore) ListKnownSkills(_ context.Context, charObjID int32, classIndex int32) (player.SkillLevels, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	levels := s.known[skillSaveKey{charObjID: charObjID, classIndex: classIndex}]
	out := make(player.SkillLevels, len(levels))
	for id, level := range levels {
		out[id] = level
	}
	return out, nil
}

func (s *memorySkillSaveStore) SetKnownSkill(_ context.Context, charObjID int32, classIndex int32, skillID int, level int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := skillSaveKey{charObjID: charObjID, classIndex: classIndex}
	if s.known[key] == nil {
		s.known[key] = make(player.SkillLevels)
	}
	s.known[key][skillID] = level
	return nil
}

func (s *memorySkillSaveStore) knownFor(charObjID int32, classIndex int32) player.SkillLevels {
	s.mu.Lock()
	defer s.mu.Unlock()
	levels := s.known[skillSaveKey{charObjID: charObjID, classIndex: classIndex}]
	out := make(player.SkillLevels, len(levels))
	for id, level := range levels {
		out[id] = level
	}
	return out
}

func (s *memorySkillSaveStore) seedKnown(charObjID int32, classIndex int32, levels player.SkillLevels) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make(player.SkillLevels, len(levels))
	for id, level := range levels {
		cp[id] = level
	}
	s.known[skillSaveKey{charObjID: charObjID, classIndex: classIndex}] = cp
}
