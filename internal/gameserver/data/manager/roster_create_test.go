package manager

import (
	"context"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

type rosterTestIDs struct{ next int32 }

func (ids *rosterTestIDs) NextID() (int32, error) {
	ids.next++
	return ids.next, nil
}

type rosterTestCharacters struct {
	byAccount map[string][]*player.Character
	names     map[string]bool
}

func newRosterTestCharacters() *rosterTestCharacters {
	return &rosterTestCharacters{
		byAccount: make(map[string][]*player.Character),
		names:     make(map[string]bool),
	}
}

func (s *rosterTestCharacters) Create(_ context.Context, c *player.Character) error {
	s.byAccount[c.AccountName] = append(s.byAccount[c.AccountName], c)
	s.names[strings.ToLower(c.Name)] = true
	return nil
}

func (s *rosterTestCharacters) ListByAccount(_ context.Context, accountName string) ([]*player.Character, error) {
	return append([]*player.Character(nil), s.byAccount[accountName]...), nil
}

func (s *rosterTestCharacters) CountByAccount(_ context.Context, accountName string) (int, error) {
	return len(s.byAccount[accountName]), nil
}

func (s *rosterTestCharacters) NameTaken(_ context.Context, name string) (bool, error) {
	return s.names[strings.ToLower(name)], nil
}

func (s *rosterTestCharacters) SetDeleteAt(context.Context, int32, int64) error {
	return nil
}

func (s *rosterTestCharacters) SetPosition(context.Context, int32, location.Location, int) error {
	return nil
}

func (s *rosterTestCharacters) Delete(context.Context, int32) (bool, error) {
	return false, nil
}

type rosterTestItems struct{}

func (rosterTestItems) Create(context.Context, int32, item.Instance) error {
	return nil
}

func (rosterTestItems) DeleteByOwner(context.Context, int32) (int64, error) {
	return 0, nil
}

func newRosterForCreateTest(t *testing.T, chars *rosterTestCharacters, npcs *npc.Table) *Roster {
	t.Helper()
	if chars == nil {
		chars = newRosterTestCharacters()
	}
	templates, err := player.NewTemplateTable(map[int]*player.Template{
		0: {
			ID:        0,
			BaseLevel: 1,
			HPTable:   []float64{80},
			MPTable:   []float64{30},
			CPTable:   []float64{32},
			Spawns:    []location.Location{{X: 10, Y: 20, Z: 30}},
		},
	})
	if err != nil {
		t.Fatalf("build template table: %v", err)
	}
	return NewRoster(chars, rosterTestItems{}, nil, templates, item.NewTable(nil), npcs, &rosterTestIDs{next: 100}, DefaultDeleteAfter, nil)
}

func TestRosterCreateRejectsNPCNameCollision(t *testing.T) {
	roster := newRosterForCreateTest(t, nil, npc.NewTable([]*npc.Template{{ID: 30001, Name: "Gatekeeper"}}))

	_, outcome, err := roster.Create(context.Background(), "acct1", CreateRequest{
		Name: "Gatekeeper", ClassID: 0, Race: 0, Sex: player.SexMale,
	})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if outcome != CreateInvalidName {
		t.Fatalf("Create() outcome = %v, want CreateInvalidName", outcome)
	}
}

func TestRosterCreateRejectsNPCNameCollisionCaseInsensitive(t *testing.T) {
	roster := newRosterForCreateTest(t, nil, npc.NewTable([]*npc.Template{{ID: 30001, Name: "Gatekeeper"}}))

	_, outcome, err := roster.Create(context.Background(), "acct1", CreateRequest{
		Name: "gatekeeper", ClassID: 0, Race: 0, Sex: player.SexMale,
	})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if outcome != CreateInvalidName {
		t.Fatalf("Create() outcome = %v, want CreateInvalidName", outcome)
	}
}

func TestRosterCreateAcceptsValidNonNPCName(t *testing.T) {
	roster := newRosterForCreateTest(t, nil, npc.NewTable([]*npc.Template{{ID: 30001, Name: "Gatekeeper"}}))

	character, outcome, err := roster.Create(context.Background(), "acct1", CreateRequest{
		Name: "Newbie", ClassID: 0, Race: 0, Sex: player.SexMale,
	})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if outcome != CreateOK {
		t.Fatalf("Create() outcome = %v, want CreateOK", outcome)
	}
	if character.Name != "Newbie" {
		t.Fatalf("Create() character name = %q, want Newbie", character.Name)
	}
}

func TestRosterCreateRejectsExistingCharacterName(t *testing.T) {
	chars := newRosterTestCharacters()
	chars.names["newbie"] = true
	roster := newRosterForCreateTest(t, chars, npc.NewTable([]*npc.Template{{ID: 30001, Name: "Gatekeeper"}}))

	_, outcome, err := roster.Create(context.Background(), "acct1", CreateRequest{
		Name: "Newbie", ClassID: 0, Race: 0, Sex: player.SexMale,
	})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if outcome != CreateNameTaken {
		t.Fatalf("Create() outcome = %v, want CreateNameTaken", outcome)
	}
}
