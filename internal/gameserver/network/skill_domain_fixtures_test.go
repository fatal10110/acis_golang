package network

import (
	"context"
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attack"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

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
func wireLiveAttackHooks(gcl *GameClientLink, live *livePlayer) {
	live.stopAttack = gcl.stopLiveAutoAttack
	live.attack.SetFinished(func() {
		gcl.finishDeferredPickup(live)
		gcl.finishDeferredMagicSkill(live)
		gcl.finishDeferredItemAICast(live)
		live.combat.Think()
	})
	live.attack.SetStarted(func() {
		gcl.startLiveAutoAttack(live)
	})
	live.Character.SetAttackBroadcaster(func(snapshot attack.Snapshot) {
		gcl.broadcastAttack(live, snapshot)
	})
	live.Character.SetMoveBroadcaster(func(event move.Event) {
		gcl.broadcastLiveMoveEvent(live, event)
	})
	live.Character.SetStatusBroadcaster(func() {
		gcl.broadcastLiveStatus(live)
	})
	live.move.SetArrived(func() {
		pos := live.move.Position()
		gcl.updateLivePlayerPosition(live, pos, live.CurrentHeading())
		live.combat.Think()
	})
}

// TestAttackLiveTargetRejectsOutOfControl pins AttackRequest.java:31's
// isOutOfControl() reject (Creature.java:652-655): a teleporting,
// immobile-until-attacked, stunned, sleeping, paralyzed, afraid, confused, or
// levelRefreshTable is a three-level table, so RealMaxLevel is 2 and a single
// level-up from 1 is legal.
func levelRefreshTable(t *testing.T) *player.LevelTable {
	t.Helper()
	table, err := player.NewLevelTable(map[int]player.Level{
		1: {RequiredExpToLevelUp: 0},
		2: {RequiredExpToLevelUp: 68},
		3: {RequiredExpToLevelUp: 363},
	})
	if err != nil {
		t.Fatalf("build level table: %v", err)
	}
	return table
}

// TestRefreshLiveLevelSkillsReconcilesAndSendsSkillList pins what the level
