//go:build integration

package network

import (
	"context"
	"testing"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
)

// seedSelectableSQLCharacter inserts a selectable character through the real
// SQL character store, for tests that need a pre-existing row before the
// client dials.
func seedSelectableSQLCharacter(t *testing.T, chars *gamesql.CharacterStore, account, name string, level, sp int) *player.Character {
	t.Helper()
	tmpl, ok := testTemplates(t).Get(0)
	if !ok {
		t.Fatal("missing test class template")
	}
	ch, err := player.NewCharacter(100, tmpl, account, name, 1, 0, 0, player.SexMale)
	if err != nil {
		t.Fatalf("seed character: %v", err)
	}
	ch.CharLevel = level
	ch.SP = sp
	if err := chars.Create(context.Background(), ch); err != nil {
		t.Fatalf("seed character store: %v", err)
	}
	return ch
}

// sqlSoleObjectID returns the single character id persisted for the default
// test account.
func sqlSoleObjectID(t *testing.T, chars *gamesql.CharacterStore) int32 {
	t.Helper()
	characters, err := chars.ListByAccount(context.Background(), "player1")
	if err != nil {
		t.Fatalf("list characters: %v", err)
	}
	if len(characters) != 1 {
		t.Fatalf("character count = %d, want 1", len(characters))
	}
	return characters[0].ID
}
