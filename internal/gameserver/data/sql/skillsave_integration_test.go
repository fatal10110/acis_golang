//go:build integration

package sql

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

func TestSkillSaveStore_ReplaceAndListByCharacter(t *testing.T) {
	ctx := context.Background()
	store := NewSkillSaveStore(sqltest.NewDB(t))

	rows := []effect.SaveRow{
		{
			Skill: modelskill.Ref{ID: 1001, Level: 3}, EffectCount: 2, EffectCurTime: 15,
			ReuseDelay: 5000, SystemTime: 1_700_000_000_000, RestoreType: effect.RestoreTypeEffect, BuffIndex: 1,
		},
		{
			Skill: modelskill.Ref{ID: 1002, Level: 1}, EffectCount: -1, EffectCurTime: -1,
			ReuseDelay: 60000, SystemTime: 1_700_000_100_000, RestoreType: effect.RestoreTypeReuseOnly, BuffIndex: 2,
		},
	}

	if err := store.Replace(ctx, 0x10000001, 0, rows); err != nil {
		t.Fatalf("Replace() unexpected error: %v", err)
	}

	got, err := store.ListByCharacter(ctx, 0x10000001, 0)
	if err != nil {
		t.Fatalf("ListByCharacter() unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ListByCharacter() returned %d rows, want 2", len(got))
	}
	if got[0].BuffIndex != 1 || got[1].BuffIndex != 2 {
		t.Fatalf("ListByCharacter() = %+v, want rows ordered by buff_index", got)
	}
	if got[0].Skill != rows[0].Skill || got[0].EffectCount != 2 || got[0].EffectCurTime != 15 ||
		got[0].ReuseDelay != 5000 || got[0].SystemTime != 1_700_000_000_000 || got[0].RestoreType != effect.RestoreTypeEffect {
		t.Errorf("got[0] = %+v, want %+v", got[0], rows[0])
	}
	if got[1].Skill != rows[1].Skill || got[1].EffectCount != -1 || got[1].EffectCurTime != -1 ||
		got[1].RestoreType != effect.RestoreTypeReuseOnly {
		t.Errorf("got[1] = %+v, want %+v", got[1], rows[1])
	}
	for _, row := range got {
		if row.ClassIndex != 0 {
			t.Errorf("row.ClassIndex = %d, want 0", row.ClassIndex)
		}
	}
}

func TestSkillSaveStore_ReplaceClearsPreviousRows(t *testing.T) {
	ctx := context.Background()
	store := NewSkillSaveStore(sqltest.NewDB(t))

	first := []effect.SaveRow{{Skill: modelskill.Ref{ID: 1001, Level: 1}, RestoreType: effect.RestoreTypeReuseOnly, BuffIndex: 1}}
	if err := store.Replace(ctx, 0x10000001, 0, first); err != nil {
		t.Fatalf("Replace(first) unexpected error: %v", err)
	}

	second := []effect.SaveRow{{Skill: modelskill.Ref{ID: 2002, Level: 1}, RestoreType: effect.RestoreTypeReuseOnly, BuffIndex: 1}}
	if err := store.Replace(ctx, 0x10000001, 0, second); err != nil {
		t.Fatalf("Replace(second) unexpected error: %v", err)
	}

	got, err := store.ListByCharacter(ctx, 0x10000001, 0)
	if err != nil {
		t.Fatalf("ListByCharacter() unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].Skill.ID != 2002 {
		t.Fatalf("ListByCharacter() = %+v, want only the second Replace's row", got)
	}
}

func TestSkillSaveStore_ReplaceScopesByClassIndex(t *testing.T) {
	ctx := context.Background()
	store := NewSkillSaveStore(sqltest.NewDB(t))

	rowsClass0 := []effect.SaveRow{{Skill: modelskill.Ref{ID: 1001, Level: 1}, RestoreType: effect.RestoreTypeReuseOnly, BuffIndex: 1}}
	rowsClass1 := []effect.SaveRow{{Skill: modelskill.Ref{ID: 2002, Level: 1}, RestoreType: effect.RestoreTypeReuseOnly, BuffIndex: 1}}

	if err := store.Replace(ctx, 0x10000001, 0, rowsClass0); err != nil {
		t.Fatalf("Replace(class 0) unexpected error: %v", err)
	}
	if err := store.Replace(ctx, 0x10000001, 1, rowsClass1); err != nil {
		t.Fatalf("Replace(class 1) unexpected error: %v", err)
	}

	got0, err := store.ListByCharacter(ctx, 0x10000001, 0)
	if err != nil {
		t.Fatalf("ListByCharacter(class 0) unexpected error: %v", err)
	}
	if len(got0) != 1 || got0[0].Skill.ID != 1001 {
		t.Fatalf("ListByCharacter(class 0) = %+v, want only class 0's row untouched by class 1's Replace", got0)
	}

	got1, err := store.ListByCharacter(ctx, 0x10000001, 1)
	if err != nil {
		t.Fatalf("ListByCharacter(class 1) unexpected error: %v", err)
	}
	if len(got1) != 1 || got1[0].Skill.ID != 2002 {
		t.Fatalf("ListByCharacter(class 1) = %+v, want only class 1's row", got1)
	}
}

func TestSkillSaveStore_ListByCharacter_Empty(t *testing.T) {
	ctx := context.Background()
	store := NewSkillSaveStore(sqltest.NewDB(t))

	got, err := store.ListByCharacter(ctx, 0x10000999, 0)
	if err != nil {
		t.Fatalf("ListByCharacter() unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("ListByCharacter() = %v, want empty for a character with no saved rows", got)
	}
}

func TestSkillSaveStore_DeleteByCharacter(t *testing.T) {
	ctx := context.Background()
	store := NewSkillSaveStore(sqltest.NewDB(t))

	rows := []effect.SaveRow{
		{Skill: modelskill.Ref{ID: 1001, Level: 1}, RestoreType: effect.RestoreTypeReuseOnly, BuffIndex: 1},
		{Skill: modelskill.Ref{ID: 1002, Level: 1}, RestoreType: effect.RestoreTypeReuseOnly, BuffIndex: 2},
	}
	if err := store.Replace(ctx, 0x10000001, 0, rows); err != nil {
		t.Fatalf("Replace() unexpected error: %v", err)
	}

	n, err := store.DeleteByCharacter(ctx, 0x10000001, 0)
	if err != nil {
		t.Fatalf("DeleteByCharacter() unexpected error: %v", err)
	}
	if n != 2 {
		t.Errorf("DeleteByCharacter() deleted %d rows, want 2", n)
	}

	got, err := store.ListByCharacter(ctx, 0x10000001, 0)
	if err != nil {
		t.Fatalf("ListByCharacter() after delete unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("ListByCharacter() after delete = %v, want empty", got)
	}
}

func TestSkillSaveStore_ReplaceEmptyRowsOnlyClears(t *testing.T) {
	ctx := context.Background()
	store := NewSkillSaveStore(sqltest.NewDB(t))

	rows := []effect.SaveRow{{Skill: modelskill.Ref{ID: 1001, Level: 1}, RestoreType: effect.RestoreTypeReuseOnly, BuffIndex: 1}}
	if err := store.Replace(ctx, 0x10000001, 0, rows); err != nil {
		t.Fatalf("Replace(rows) unexpected error: %v", err)
	}

	if err := store.Replace(ctx, 0x10000001, 0, nil); err != nil {
		t.Fatalf("Replace(nil) unexpected error: %v", err)
	}

	got, err := store.ListByCharacter(ctx, 0x10000001, 0)
	if err != nil {
		t.Fatalf("ListByCharacter() unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("ListByCharacter() after Replace(nil) = %v, want empty", got)
	}
}
