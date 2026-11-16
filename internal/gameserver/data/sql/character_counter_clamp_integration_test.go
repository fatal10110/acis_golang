package sql

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
)

// TestCharacterStoreSaveClampsCounters pins the owner-approved fix of #5073:
// a PvP or PK count past the SMALLINT UNSIGNED pvpkills/pkkills columns, or a
// karma outside the INT UNSIGNED column, is stored clamped to the column
// range instead of failing the save, and the rest of the save (base class
// progression, subclass rows) still lands.
func TestCharacterStoreSaveClampsCounters(t *testing.T) {
	ctx := context.Background()
	db := sqltest.SharedDB(t)
	store := NewCharacterStore(db)
	c := testCharacter(0x10000001, "Reaper")
	if err := store.Create(ctx, c); err != nil {
		t.Fatalf("Create(): %v", err)
	}
	sub := player.SubClass{ClassID: 10, Index: 1, Exp: 0, SP: 0, Level: 40}
	if err := NewSubclassStore(db).Insert(ctx, c.ID, sub); err != nil {
		t.Fatalf("insert subclass: %v", err)
	}
	if !c.AddSubclass(sub) {
		t.Fatal("AddSubclass() = false")
	}

	cases := []struct {
		name            string
		pvp, pk, karma  int
		wantPvP, wantPK int
		wantKarma       int64
	}{
		{name: "at the column max", pvp: 65535, pk: 65535, karma: 4294967295, wantPvP: 65535, wantPK: 65535, wantKarma: 4294967295},
		{name: "one past the column max", pvp: 65536, pk: 65536, karma: 4294967296, wantPvP: 65535, wantPK: 65535, wantKarma: 4294967295},
		{name: "far past the column max", pvp: 1 << 20, pk: 1 << 20, karma: 1 << 33, wantPvP: 65535, wantPK: 65535, wantKarma: 4294967295},
		{name: "negative", pvp: -1, pk: -1, karma: -1, wantPvP: 0, wantPK: 0, wantKarma: 0},
	}
	for i, tc := range cases {
		st := c.SaveState()
		st.PvPKills, st.PKKills, st.Karma = tc.pvp, tc.pk, tc.karma
		exp := int64(1000 * (i + 1))
		st.Progression.Exp = exp
		st.Subclasses[0].Exp = exp
		if err := store.Save(ctx, st); err != nil {
			t.Fatalf("%s: Save(): %v", tc.name, err)
		}
		var pvp, pk int
		var karma, gotExp int64
		if err := db.QueryRowContext(ctx, "SELECT pvpkills, pkkills, karma, exp FROM characters WHERE obj_Id = ?", c.ID).Scan(&pvp, &pk, &karma, &gotExp); err != nil {
			t.Fatalf("%s: read characters row: %v", tc.name, err)
		}
		if pvp != tc.wantPvP || pk != tc.wantPK || karma != tc.wantKarma {
			t.Errorf("%s: stored pvpkills %d, pkkills %d, karma %d; want %d, %d, %d", tc.name, pvp, pk, karma, tc.wantPvP, tc.wantPK, tc.wantKarma)
		}
		if gotExp != exp {
			t.Errorf("%s: stored exp %d; want %d", tc.name, gotExp, exp)
		}
		if n := countRows(t, db, "SELECT COUNT(*) FROM character_subclasses WHERE char_obj_id = ? AND class_index = 1 AND exp = ?", c.ID, exp); n != 1 {
			t.Errorf("%s: subclass row exp not stored as %d", tc.name, exp)
		}
	}
}

// TestCursedWeaponReleaseHolderClampsCounters pins the same clamp (#5073,
// owner-approved) on the karma and PK count restored to a cursed weapon's
// former holder.
func TestCursedWeaponReleaseHolderClampsCounters(t *testing.T) {
	ctx := context.Background()
	db := sqltest.SharedDB(t)
	c := testCharacter(0x10000001, "Holder")
	if err := NewCharacterStore(db).Create(ctx, c); err != nil {
		t.Fatalf("Create(): %v", err)
	}
	store := NewCursedWeaponStore(db)
	for _, tc := range []struct {
		karma, pk int32
		wantKarma int64
		wantPK    int
	}{
		{karma: 2147483647, pk: 70000, wantKarma: 2147483647, wantPK: 65535},
		{karma: -5, pk: -5, wantKarma: 0, wantPK: 0},
	} {
		if err := store.ReleaseHolder(ctx, c.ID, 8190, tc.karma, tc.pk); err != nil {
			t.Fatalf("ReleaseHolder(karma %d, pk %d): %v", tc.karma, tc.pk, err)
		}
		var karma int64
		var pk int
		if err := db.QueryRowContext(ctx, "SELECT karma, pkkills FROM characters WHERE obj_Id = ?", c.ID).Scan(&karma, &pk); err != nil {
			t.Fatalf("read characters row: %v", err)
		}
		if karma != tc.wantKarma || pk != tc.wantPK {
			t.Errorf("ReleaseHolder(karma %d, pk %d) stored karma %d, pkkills %d; want %d, %d", tc.karma, tc.pk, karma, pk, tc.wantKarma, tc.wantPK)
		}
	}
}
