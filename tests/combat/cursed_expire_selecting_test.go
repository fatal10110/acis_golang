package combat

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/clientwire"
)

// TestCursedWeaponEndsWhileHolderIsSelected: a holder's weapon whose hunger
// runs out while a selection of the holder has already restored the weapon,
// its karma and its skill, but has not registered the player yet, leaves no
// second live copy (#4689; owner-approved divergence, the fix the issue's
// Expected section names). The selected character enters with its karma
// and PK kills back and without the weapon or its skill, the manager has the
// weapon free, S1_HAS_DISAPPEARED is heard, and once the character logs out
// its stored karma is back and no weapon or cursed_weapons row is left.
func TestCursedWeaponEndsWhileHolderIsSelected(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	clock := newCursedClock(base)
	var armed atomic.Bool
	var held atomic.Int32
	var srv *gameservertest.Server
	srv = cursedBoot(t, 10,
		gameservertest.WithSeed(seedDropVictim(25, 100, 3)),
		gameservertest.WithCursedWeaponClock(clock.Now),
		gameservertest.WithSelectionHold(func(int32) {
			if !armed.Load() {
				return
			}
			// The selection has restored the holder and its weapon: the
			// hunger runs out now, with the holder still not in the world.
			srv.TickCursedWeapons(base.Add(60 * time.Minute))
			srv.TickCursedWeapons(base.Add(61 * time.Minute))
			held.Add(1)
		}),
	)
	c, id := srv.Client, srv.SoleObjectID(t)
	seedHolder(t, srv, id, 61, base.Add(50*time.Hour))

	armed.Store(true)
	c.Send(clientwire.RequestGameStart(0))
	assertFrameOpcode(t, mustRead(t, c, "SSQInfo"), serverpackets.OpcodeSSQInfo, "SSQInfo")
	assertFrameOpcode(t, mustRead(t, c, "CharSelected"), serverpackets.OpcodeCharSelected, "CharSelected")
	armed.Store(false)
	if got := held.Load(); got != 1 {
		t.Fatalf("selection hold ran %d times, want once", got)
	}
	if len(srv.CursedWeapons.Active()) != 0 {
		t.Fatal("Zariche still out after its hunger ran out")
	}
	srv.Settle(t)
	frames := readQuiet(c)
	c.Send(clientwire.EnterWorld())
	frames = append(frames, readUntil(t, c, serverpackets.OpcodeSkillCoolTime, "login burst")...)
	srv.Settle(t)
	frames = append(frames, readQuiet(c)...)
	holder := victimCharacter(t, srv, id)
	if holder.CursedWeaponEquipped() || holder.CursedWeaponID() != 0 || holder.Karma() != 100 ||
		holder.ProgressionValues().PKKills != 3 || holder.SkillLevel(int(zaricheSkill)) != 0 {
		t.Fatalf("selected holder: weapon %d karma %d pk %d skill %d, want none, 100, 3, none",
			holder.CursedWeaponID(), holder.Karma(), holder.ProgressionValues().PKKills, holder.SkillLevel(int(zaricheSkill)))
	}
	if holder.Inventory().ItemByTemplateID(zaricheID) != nil {
		t.Fatal("selected holder still carries Zariche")
	}
	if len(srv.CursedWeapons.Active()) != 0 {
		t.Fatal("Zariche out again after the selection")
	}
	if indexOfMessage(frames, 0, serverpackets.SystemMessageS1HasDisappeared, itemParam(zaricheID)) < 0 {
		t.Fatal("no S1_HAS_DISAPPEARED when the hunger ran out")
	}

	logoutPersisted(t, srv, c)
	if _, _, _, _, _, _, ok := cursedRow(t, srv); ok {
		t.Fatal("cursed_weapons row kept")
	}
	var karma, pk, weapons int32
	ctx := context.Background()
	if err := srv.DB.QueryRowContext(ctx, "SELECT karma, pkkills FROM characters WHERE obj_Id = ?", id).Scan(&karma, &pk); err != nil {
		t.Fatalf("read character: %v", err)
	}
	if err := srv.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM items WHERE owner_id = ? AND item_id = ?", id, zaricheID).Scan(&weapons); err != nil {
		t.Fatalf("count items: %v", err)
	}
	if karma != 100 || pk != 3 || weapons != 0 {
		t.Fatalf("stored after logout: karma %d pk %d weapons %d, want 100, 3, 0", karma, pk, weapons)
	}
}
