package olympiad

import (
	"context"
	"fmt"
	"slices"
	"testing"

	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// itemLocation reads objectID's stored location and slot.
func itemLocation(t *testing.T, srv *gameservertest.Server, objectID int32) (string, int) {
	t.Helper()
	var (
		loc  string
		slot int
	)
	if err := srv.DB.QueryRowContext(context.Background(), "SELECT loc, loc_data FROM items WHERE object_id = ?", objectID).Scan(&loc, &slot); err != nil {
		t.Fatal(err)
	}
	return loc, slot
}

// TestHeroItemNeedsAnActiveHero pins wearing a hero weapon: an active hero
// puts it on; anyone else, an elected hero yet to claim the status among
// them, is told the item cannot be equipped and keeps it in the
// inventory.
func TestHeroItemNeedsAnActiveHero(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		active int
		hero   bool
	}{
		{"active hero", 1, true},
		{"elected hero yet to claim", 0, false},
		{"no hero", -1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var stmts []string
			if tc.active >= 0 {
				stmts = append(stmts, fmt.Sprintf(`INSERT INTO heroes (char_id, class_id, count, played, active) SELECT obj_Id, 88, 1, 1, %d FROM characters WHERE char_name = 'Login'`, tc.active))
			}
			srv := gameservertest.Boot(t, append([]gameservertest.Option{
				gameservertest.WithCharacter("Login", 40, 0),
				gameservertest.WithWantChars(1),
				gameservertest.WithOlympiadSeed(seedStatements(t, stmts)),
			}, heroItemOptions(t)...)...)
			blade := srv.GiveItem(t, srv.SoleObjectID(t), infinityBlade, 1)
			c := srv.Client
			startInWorld(t, c)

			c.Send(encodeUseItem(blade))
			ids := systemMessageIDs(drainFrames(t, c))
			srv.FlushItems(t)
			loc, _ := itemLocation(t, srv, blade)
			if tc.hero {
				if !slices.Contains(ids, serverpackets.SystemMessageS1Equipped) || loc != "PAPERDOLL" {
					t.Fatalf("messages %v, location %s; want the blade equipped", ids, loc)
				}
				return
			}
			if !slices.Equal(ids, []int{serverpackets.SystemMessageCannotEquipItemDueToBadCondition}) || loc != "INVENTORY" {
				t.Fatalf("messages %v, location %s; want only the bad condition refusal, the blade in the inventory", ids, loc)
			}
		})
	}
}

// TestWornHeroItemAtLogin pins a hero item stored as worn: an active hero
// logs in wearing it; anyone else, a hero of a past era among them, logs
// in with it back in the inventory, and the move is stored.
func TestWornHeroItemAtLogin(t *testing.T) {
	t.Parallel()
	const blade = 7101
	for _, tc := range []struct {
		name           string
		played, active int
		wantLoc        string
		wantSlot       int
	}{
		{"active hero", 1, 1, "PAPERDOLL", 7},
		{"hero of a past era", 0, 1, "INVENTORY", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			stmts := []string{
				fmt.Sprintf(`INSERT INTO heroes (char_id, class_id, count, played, active) SELECT obj_Id, 88, 1, %d, %d FROM characters WHERE char_name = 'Login'`, tc.played, tc.active),
				fmt.Sprintf(`INSERT INTO items (owner_id, object_id, item_id, count, loc, loc_data) SELECT obj_Id, %d, %d, 1, 'PAPERDOLL', 7 FROM characters WHERE char_name = 'Login'`, blade, infinityBlade),
			}
			srv := gameservertest.Boot(t, append([]gameservertest.Option{
				gameservertest.WithCharacter("Login", 40, 0),
				gameservertest.WithWantChars(1),
				gameservertest.WithOlympiadSeed(seedStatements(t, stmts)),
			}, heroItemOptions(t)...)...)
			startInWorld(t, srv.Client)
			srv.FlushItems(t)
			if loc, slot := itemLocation(t, srv, blade); loc != tc.wantLoc || slot != tc.wantSlot {
				t.Fatalf("blade stored at %s %d, want %s %d", loc, slot, tc.wantLoc, tc.wantSlot)
			}
		})
	}
}

// TestGMHeroAura pins GMHeroAura: a game master who is no hero shows the
// hero aura in its UserInfo only while it is set.
func TestGMHeroAura(t *testing.T) {
	t.Parallel()
	data, err := gamexml.LoadAdminData(datapack.Path(t, "data", "xml"))
	if err != nil {
		t.Fatalf("load admin data: %v", err)
	}
	for _, aura := range []bool{true, false} {
		t.Run(fmt.Sprint("aura=", aura), func(t *testing.T) {
			t.Parallel()
			opts := []gameservertest.Option{gameservertest.WithAdmin(data)}
			if aura {
				opts = append(opts, gameservertest.WithGMHeroAura())
			}
			_, burst := bootNamed(t, "Master", []string{`UPDATE characters SET accesslevel = 7 WHERE char_name = 'Master'`}, opts...)
			want := byte(0)
			if aura {
				want = 1
			}
			if got := userInfoHero(firstFrame(t, burst, serverpackets.OpcodeUserInfo)); got != want {
				t.Fatalf("UserInfo hero byte = %d, want %d", got, want)
			}
		})
	}
}
