package gameservertest

import (
	"context"
	"time"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	petmodel "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/pet"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/shortcut"
)

// The wrappers below stand in for a degraded database: every write an
// in-world handler issues, and the pets-row read a summon needs, takes the
// configured delay (WithSlowStores). Each one embeds the real store, so a
// handler that type-asserts for a wider surface still finds it.
//
// Reads the suites themselves make go through Server's own store handles,
// which stay unwrapped.

type slowItemStore struct {
	*gamesql.ItemStore
	delay time.Duration
}

func (s slowItemStore) SaveState(ctx context.Context, st item.InstanceState) error {
	time.Sleep(s.delay)
	return s.ItemStore.SaveState(ctx, st)
}

func (s slowItemStore) UpdateState(ctx context.Context, st item.InstanceState) error {
	time.Sleep(s.delay)
	return s.ItemStore.UpdateState(ctx, st)
}

func (s slowItemStore) Delete(ctx context.Context, objectID int32) error {
	time.Sleep(s.delay)
	return s.ItemStore.Delete(ctx, objectID)
}

type slowShortcutStore struct {
	*gamesql.ShortcutStore
	delay time.Duration
}

func (s slowShortcutStore) Save(ctx context.Context, ownerID int32, sc shortcut.Shortcut) error {
	time.Sleep(s.delay)
	return s.ShortcutStore.Save(ctx, ownerID, sc)
}

func (s slowShortcutStore) Delete(ctx context.Context, ownerID int32, slot, page int32) error {
	time.Sleep(s.delay)
	return s.ShortcutStore.Delete(ctx, ownerID, slot, page)
}

type slowPetStore struct {
	*gamesql.PetStore
	delay time.Duration
}

func (s slowPetStore) Get(ctx context.Context, itemObjectID int32) (petmodel.State, bool, error) {
	time.Sleep(s.delay)
	return s.PetStore.Get(ctx, itemObjectID)
}

func (s slowPetStore) Save(ctx context.Context, itemObjectID int32, st petmodel.State) error {
	time.Sleep(s.delay)
	return s.PetStore.Save(ctx, itemObjectID, st)
}

func (s slowPetStore) NameTaken(ctx context.Context, name string) (bool, error) {
	time.Sleep(s.delay)
	return s.PetStore.NameTaken(ctx, name)
}

type slowCharacterSkillStore struct {
	*gamesql.CharacterSkillStore
	delay time.Duration
}

func (s slowCharacterSkillStore) SetKnownSkill(ctx context.Context, charObjID, classIndex int32, skillID, level int) error {
	time.Sleep(s.delay)
	return s.CharacterSkillStore.SetKnownSkill(ctx, charObjID, classIndex, skillID, level)
}

func (s slowCharacterSkillStore) DeleteKnownSkill(ctx context.Context, charObjID, classIndex int32, skillID int) error {
	time.Sleep(s.delay)
	return s.CharacterSkillStore.DeleteKnownSkill(ctx, charObjID, classIndex, skillID)
}

// KnownSkillStore is the character_skills surface skill persistence reads and
// writes through.
type KnownSkillStore interface {
	ListKnownSkills(ctx context.Context, charObjID, classIndex int32) (player.SkillLevels, error)
	SetKnownSkill(ctx context.Context, charObjID, classIndex int32, skillID, level int) error
	DeleteKnownSkill(ctx context.Context, charObjID, classIndex int32, skillID int) error
}

// SlowKnownSkills wraps store so every character_skills write takes d. A
// suite that builds its own skill persistence (WithSkills) uses it to get the
// degraded-database behavior WithSlowStores gives the boot-default one.
func SlowKnownSkills(store *gamesql.CharacterSkillStore, d time.Duration) KnownSkillStore {
	return slowCharacterSkillStore{CharacterSkillStore: store, delay: d}
}
