package clan

import (
	"context"
	"maps"
	"slices"
	"testing"

	"github.com/rs/zerolog"
)

func (f *fakeStore) RemoveSkill(context.Context, int32, int) error { return nil }
func (f *fakeStore) RemoveAllSkills(context.Context, int32) error  { return nil }

// skillRowStore keeps the clan_skills rows the service writes.
type skillRowStore struct {
	*fakeStore
	rows map[int]int
}

func (f *skillRowStore) SaveSkill(_ context.Context, _ int32, sk Skill) error {
	f.rows[sk.ID] = sk.Level
	return nil
}

func (f *skillRowStore) RemoveSkill(_ context.Context, _ int32, id int) error {
	delete(f.rows, id)
	return nil
}

func (f *skillRowStore) RemoveAllSkills(context.Context, int32) error {
	clear(f.rows)
	return nil
}

// editedClan is orderedClan whose store keeps the clan's skill rows,
// starting from the skills it knows.
func editedClan(t *testing.T, known map[int]int) (*Service, *Clan, *skillRowStore, *laneWriter) {
	t.Helper()
	_, cl, fake, _ := orderedClan(t, 1000, 0)
	cl.skills = maps.Clone(known)
	store := &skillRowStore{fakeStore: fake, rows: map[int]int{}}
	maps.Copy(store.rows, known)
	writes := &laneWriter{}
	return NewService(NewTable(), store, writes, nil, DefaultConfig(), nil, zerolog.Nop()), cl, store, writes
}

// TestSetSkillReplacesLevel sets a skill the clan does not know, then
// another level of it: each is known and stored at the level set. Setting
// the level it already knows changes nothing.
func TestSetSkillReplacesLevel(t *testing.T) {
	s, cl, store, writes := editedClan(t, nil)
	if !s.SetSkill(cl, Skill{ID: 370, Level: 3}) || !s.SetSkill(cl, Skill{ID: 370, Level: 1}) {
		t.Fatal("setting a new level reported no change")
	}
	if s.SetSkill(cl, Skill{ID: 370, Level: 1}) {
		t.Fatal("setting the known level reported a change")
	}
	writes.drain()
	if got := cl.Skills(); !slices.Equal(got, []Skill{{ID: 370, Level: 1}}) || !maps.Equal(store.rows, map[int]int{370: 1}) {
		t.Fatalf("clan %v, rows %v; want Clan Vitality 1 both", got, store.rows)
	}
}

// TestRaiseSkillsKeepsHigherLevels raises the clan to each skill's level:
// a skill it does not know or knows lower is raised and stored, one it
// knows at that level or higher is kept. Raising to what it knows changes
// nothing.
func TestRaiseSkillsKeepsHigherLevels(t *testing.T) {
	s, cl, store, writes := editedClan(t, map[int]int{370: 1, 371: 3, 372: 2})
	all := []Skill{{ID: 370, Level: 3}, {ID: 371, Level: 2}, {ID: 372, Level: 2}, {ID: 373, Level: 1}}
	if !s.RaiseSkills(cl, all) {
		t.Fatal("raising reported no change")
	}
	writes.drain()
	want := map[int]int{370: 3, 371: 3, 372: 2, 373: 1}
	if got := cl.Skills(); len(got) != len(want) || !maps.Equal(store.rows, want) {
		t.Fatalf("clan %v, rows %v; want %v", got, store.rows, want)
	}
	for _, sk := range cl.Skills() {
		if want[sk.ID] != sk.Level {
			t.Fatalf("clan knows %v, want %v", cl.Skills(), want)
		}
	}
	if s.RaiseSkills(cl, all) {
		t.Fatal("raising to the known levels reported a change")
	}
}

// TestRemoveSkills removes one skill, then every other: each is forgotten
// and its row deleted; removing what the clan does not know changes
// nothing.
func TestRemoveSkills(t *testing.T) {
	s, cl, store, writes := editedClan(t, map[int]int{370: 1, 375: 2, 371: 3})
	if !s.RemoveSkill(cl, 370) {
		t.Fatal("removing a known skill reported no change")
	}
	if s.RemoveSkill(cl, 370) {
		t.Fatal("removing an unknown skill reported a change")
	}
	writes.drain()
	if !maps.Equal(store.rows, map[int]int{375: 2, 371: 3}) {
		t.Fatalf("rows %v after removing 370", store.rows)
	}
	removed, ok := s.RemoveAllSkills(cl)
	if !ok || !slices.Equal(removed, []Skill{{ID: 371, Level: 3}, {ID: 375, Level: 2}}) {
		t.Fatalf("remove all = %v %v, want 371 and 375 by id", removed, ok)
	}
	writes.drain()
	if len(cl.Skills()) != 0 || len(store.rows) != 0 {
		t.Fatalf("clan %v, rows %v; want none", cl.Skills(), store.rows)
	}
	if _, ok := s.RemoveAllSkills(cl); ok {
		t.Fatal("removing every skill of a clan with none reported a change")
	}
}
