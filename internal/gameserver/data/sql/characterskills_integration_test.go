//go:build integration

package sql

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
)

func TestCharacterSkillStore_ListKnownSkills(t *testing.T) {
	ctx := context.Background()
	db := sqltest.NewDB(t)
	if _, err := db.ExecContext(ctx,
		`INSERT INTO character_skills (char_obj_id, skill_id, skill_level, class_index) VALUES
			(?, ?, ?, ?), (?, ?, ?, ?)`,
		0x10000001, 248, 1, 0,
		0x10000001, 300, 2, 1,
	); err != nil {
		t.Fatalf("seed character_skills: %v", err)
	}

	got, err := NewCharacterSkillStore(db).ListKnownSkills(ctx, 0x10000001, 0)
	if err != nil {
		t.Fatalf("ListKnownSkills() error = %v", err)
	}
	if len(got) != 1 || got[248] != 1 {
		t.Fatalf("ListKnownSkills() = %+v, want only class 0 skill 248:1", got)
	}
}

func TestCharacterSkillStore_SetKnownSkill(t *testing.T) {
	ctx := context.Background()
	db := sqltest.NewDB(t)
	store := NewCharacterSkillStore(db)

	if err := store.SetKnownSkill(ctx, 0x10000001, 0, 3, 1); err != nil {
		t.Fatalf("SetKnownSkill first: %v", err)
	}
	if err := store.SetKnownSkill(ctx, 0x10000001, 0, 3, 2); err != nil {
		t.Fatalf("SetKnownSkill update: %v", err)
	}

	got, err := store.ListKnownSkills(ctx, 0x10000001, 0)
	if err != nil {
		t.Fatalf("ListKnownSkills() error = %v", err)
	}
	if len(got) != 1 || got[3] != 2 {
		t.Fatalf("ListKnownSkills() = %+v, want skill 3 level 2", got)
	}
}
