package network

import (
	"context"
	"errors"
	"testing"

	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

func TestGameClientLinkEnterWorldContinuesWhenSkillRestoreFails(t *testing.T) {
	skills := skillstate.NewPersistence(failingSkillSaveStore{}, skillTable())
	c, chars, _, state := newLinkedGameClientWithSkills(t, skills)

	c.Send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	c.Read() // CharCreateOk
	c.Read() // CharSelectInfo
	objectID := chars.soleObjectID(t)
	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	if _, ok := state.Player(objectID); !ok {
		t.Fatal("world player missing after EnterWorld with failed skill restore")
	}
}

type failingSkillSaveStore struct{}

func (failingSkillSaveStore) Replace(context.Context, int32, int32, []effect.SaveRow) error {
	return nil
}

func (failingSkillSaveStore) ListByCharacter(context.Context, int32, int32) ([]effect.SaveRow, error) {
	return nil, errors.New("skill save unavailable")
}

func (failingSkillSaveStore) DeleteByCharacter(context.Context, int32, int32) (int64, error) {
	return 0, nil
}
