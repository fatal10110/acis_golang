//go:build integration

package sql

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/shortcut"
)

func TestShortcutStoreRoundTrip(t *testing.T) {
	ctx := context.Background()
	db := sqltest.NewDB(t)
	store := NewShortcutStore(db)

	first := shortcut.Shortcut{Slot: 3, Page: 1, Type: shortcut.Skill, ID: 248, Level: 1, CharacterType: 1}
	if err := store.Save(ctx, 0x10000001, first); err != nil {
		t.Fatalf("Save first: %v", err)
	}
	updated := shortcut.Shortcut{Slot: 3, Page: 1, Type: shortcut.Action, ID: 5, Level: -1, CharacterType: 1}
	if err := store.Save(ctx, 0x10000001, updated); err != nil {
		t.Fatalf("Save update: %v", err)
	}

	got, err := store.ListByOwner(ctx, 0x10000001)
	if err != nil {
		t.Fatalf("ListByOwner: %v", err)
	}
	if len(got) != 1 || got[0] != updated {
		t.Fatalf("ListByOwner = %+v, want [%+v]", got, updated)
	}

	if err := store.Delete(ctx, 0x10000001, 3, 1); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	got, err = store.ListByOwner(ctx, 0x10000001)
	if err != nil {
		t.Fatalf("ListByOwner after delete: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("ListByOwner after delete = %+v, want empty", got)
	}
}
