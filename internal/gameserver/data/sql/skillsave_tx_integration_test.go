package sql

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// A save whose insert fails after the delete has run leaves the rows the
// character had before: the delete and the inserts commit together or not
// at all (#3242).
func TestSkillSaveStore_ReplaceFailureKeepsPreviousRows(t *testing.T) {
	ctx := context.Background()
	store := NewSkillSaveStore(sqltest.SharedDB(t))
	const charID, classIndex = 0x10000031, 0

	previous := []effect.SaveRow{
		{Skill: modelskill.Ref{ID: 1204, Level: 2}, EffectCount: 1, EffectCurTime: 300, RestoreType: effect.RestoreTypeEffect, BuffIndex: 1},
		{
			Skill: modelskill.Ref{ID: 1040, Level: 3}, EffectCount: -1, EffectCurTime: -1, ReuseDelay: 60_000,
			SystemTime: 1_700_000_000_000, RestoreType: effect.RestoreTypeReuseOnly, BuffIndex: 2,
		},
	}
	if err := store.Replace(ctx, charID, classIndex, previous); err != nil {
		t.Fatalf("seed Replace() unexpected error: %v", err)
	}

	// The same skill id and level twice violates the table's primary key,
	// so the insert fails once the delete has already run.
	failing := []effect.SaveRow{
		{Skill: modelskill.Ref{ID: 1068, Level: 1}, EffectCount: 1, EffectCurTime: 5, RestoreType: effect.RestoreTypeEffect, BuffIndex: 1},
		{Skill: modelskill.Ref{ID: 1068, Level: 1}, EffectCount: 1, EffectCurTime: 5, RestoreType: effect.RestoreTypeEffect, BuffIndex: 2},
	}
	if err := store.Replace(ctx, charID, classIndex, failing); err == nil {
		t.Fatal("Replace() with a duplicate key = nil error, want the insert's failure")
	}

	got, err := store.ListByCharacter(ctx, charID, classIndex)
	if err != nil {
		t.Fatalf("ListByCharacter() unexpected error: %v", err)
	}
	if len(got) != len(previous) {
		t.Fatalf("rows after a failed Replace = %+v, want the %d previous rows", got, len(previous))
	}
	for i, row := range got {
		want := previous[i]
		want.ClassIndex = classIndex
		if row != want {
			t.Errorf("row %d after a failed Replace = %+v, want %+v", i, row, want)
		}
	}
}
