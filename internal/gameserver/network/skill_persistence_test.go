package network

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

func TestSkillPersistenceSaveWritesLiveEffectsAndReuseTimers(t *testing.T) {
	store := newMemorySkillSaveStore()
	now := time.Now().Truncate(time.Millisecond)
	c := skillPersistenceCharacter(1001)
	c.AddActiveSkillEffect(effect.ActiveEffect{Skill: skillRef(1204, 2), ReuseGroup: 1204*256 + 2, Count: 3, Time: 20})
	c.SetSkillReuse(skillRef(1204, 2), 1204*256+2, 45*time.Second, now.Add(45*time.Second))

	p := skillstate.NewPersistenceWithClock(store, skillTable(
		modelskill.Definition{ID: 1204, Level: 2},
	), func() time.Time { return now })

	if err := p.Save(context.Background(), c, true); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got := store.rowsFor(c.ID, 0)
	want := []effect.SaveRow{{
		Skill:         skillRef(1204, 2),
		EffectCount:   3,
		EffectCurTime: 20,
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

func TestGameClientLinkEnterWorldRestoresPersistedSkillState(t *testing.T) {
	store := newMemorySkillSaveStore()
	now := time.Now().Truncate(time.Millisecond)
	p := skillstate.NewPersistenceWithClock(store, skillTable(
		modelskill.Definition{ID: 1040, Level: 3, Effects: []modelskill.EffectTemplate{{Name: "Buff"}}},
	), func() time.Time { return now })
	c, chars, _, _ := newLinkedGameClientWithSkills(t, p)

	c.send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	c.read() // CharCreateOk
	c.read() // CharSelectInfo
	objID := chars.soleObjectID(t)
	store.seed(objID, 0, []effect.SaveRow{{
		Skill:         skillRef(1040, 3),
		EffectCount:   2,
		EffectCurTime: 15,
		ReuseDelay:    60_000,
		SystemTime:    now.Add(60 * time.Second).UnixMilli(),
		RestoreType:   effect.RestoreTypeEffect,
		BuffIndex:     1,
	}})

	c.send(encodeRequestGameStart(0))
	c.read() // SSQInfo
	c.read() // CharSelected
	c.send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	char := chars.character(t, objID)
	if !char.SkillDisabled(1040*256 + 3) {
		t.Fatal("EnterWorld did not restore the reuse timer")
	}
	effects := char.ActiveSkillEffects()
	wantEffects := []effect.ActiveEffect{{Skill: skillRef(1040, 3), ReuseGroup: 1040*256 + 3, Count: 2, Time: 15}}
	if !reflect.DeepEqual(effects, wantEffects) {
		t.Fatalf("EnterWorld restored effects = %+v, want %+v", effects, wantEffects)
	}
	if got := store.rowsFor(objID, 0); len(got) != 0 {
		t.Fatalf("persisted rows after EnterWorld = %+v, want consumed", got)
	}
}

func TestGameClientLinkEnterWorldSendsKnownSkillList(t *testing.T) {
	store := newMemorySkillSaveStore()
	p := skillstate.NewPersistence(store, skillTable(
		modelskill.Definition{ID: 248, Level: 1, Activation: modelskill.ActivationPassive},
	), store)
	c, chars, _, _ := newLinkedGameClientWithSkills(t, p)

	c.send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	c.read() // CharCreateOk
	c.read() // CharSelectInfo
	objID := chars.soleObjectID(t)
	store.seedKnown(objID, 0, player.SkillLevels{248: 1})

	c.send(encodeRequestGameStart(0))
	c.read() // SSQInfo
	c.read() // CharSelected
	c.send(encodeEnterWorld())
	frames := readEnterWorldBurst(t, c, false)

	skillList := frames[5]
	if skillList[0] != serverpackets.OpcodeSkillList {
		t.Fatalf("EnterWorld skill frame opcode = %#x, want SkillList", skillList[0])
	}
	r := wire.NewReader(skillList[1:])
	if count := r.ReadInt32(); count != 1 {
		t.Fatalf("SkillList count = %d, want 1", count)
	}
	if passive, level, id := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); passive != 1 || level != 1 || id != 248 {
		t.Fatalf("SkillList entry = passive %d level %d id %d, want passive 1 level 1 id 248", passive, level, id)
	}
}

func TestGameClientLinkLogoutPersistsSkillState(t *testing.T) {
	store := newMemorySkillSaveStore()
	now := time.Now().Truncate(time.Millisecond)
	p := skillstate.NewPersistenceWithClock(store, skillTable(
		modelskill.Definition{ID: 1204, Level: 2},
	), func() time.Time { return now })
	c, chars, _, _ := newLinkedGameClientWithSkills(t, p)

	c.send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	c.read() // CharCreateOk
	c.read() // CharSelectInfo
	objID := chars.soleObjectID(t)
	c.send(encodeRequestGameStart(0))
	c.read() // SSQInfo
	c.read() // CharSelected
	c.send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	char := chars.character(t, objID)
	char.AddActiveSkillEffect(effect.ActiveEffect{Skill: skillRef(1204, 2), ReuseGroup: 1204*256 + 2, Count: 3, Time: 20})
	char.SetSkillReuse(skillRef(1204, 2), 1204*256+2, 45*time.Second, now.Add(45*time.Second))

	c.send(encodeSingleOpcode(clientpackets.OpcodeLogout))
	c.read() // LeaveWorld
	c.expectClosed()

	got := store.rowsFor(objID, 0)
	want := []effect.SaveRow{{
		Skill:         skillRef(1204, 2),
		EffectCount:   3,
		EffectCurTime: 20,
		ReuseDelay:    45_000,
		SystemTime:    now.Add(45 * time.Second).UnixMilli(),
		RestoreType:   effect.RestoreTypeEffect,
		ClassIndex:    0,
		BuffIndex:     1,
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Logout saved rows = %+v, want %+v", got, want)
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

func (s *fakeCharStore) character(t *testing.T, id int32) *player.Character {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.byID[id]
	if !ok {
		t.Fatalf("character %d missing", id)
	}
	return c
}
