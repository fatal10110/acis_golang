package npcs

import (
	"context"
	"testing"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: Player.checkItemManipulation (Player.java:2059-2089) returns
// null for an augmented item while its owner is casting (:2084-2086).
// SendWarehouseDepositList.java:86-90 then drops the whole deposit before
// any fee, silently; RequestPackageSend.java:83-91 leaves the row out with
// its fee still charged. Once the cast has ended the private warehouse
// takes the augmented weapon (ItemInstance.isDepositable(true),
// ItemInstance.java:535-538), while a package refuses it whole and silently
// because an augmented item is not tradable (ItemInstance.isTradable,
// :518-521; RequestPackageSend.java:93-94).

// augmentedCaster is a sender at a keeper holding an augmented sword and
// potions, with a five-second self cast to be mid-cast with.
type augmentedCaster struct {
	*freightWorld
	sword, potion int32
	aug           item.Augmentation
}

func bootAugmentedCaster(t *testing.T) *augmentedCaster {
	t.Helper()
	db := sqltest.SharedDB(t)
	skills := skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{{
		ID: longCastSkillID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		SkillType: "DUMMY", StaticHitTime: true, HitTime: 5000, StaticReuse: true, ReuseDelay: 0,
	}}), gamesql.NewCharacterSkillStore(db))
	a := &augmentedCaster{aug: item.Augmentation{Attributes: 0x12345, SkillID: 3203, SkillLevel: 2}}
	prepare := func(srv *gameservertest.Server, sender int32, _ *player.Character) {
		a.sword = srv.GiveItem(t, sender, swordID, 1)
		ctx := context.Background()
		if err := gamesql.NewAugmentationStore(srv.DB).Create(ctx, a.sword, a.aug); err != nil {
			t.Fatalf("seed augmentation: %v", err)
		}
		if err := srv.KnownSkills.SetKnownSkill(ctx, sender, 0, longCastSkillID, 1); err != nil {
			t.Fatalf("seed known skill: %v", err)
		}
	}
	a.freightWorld = bootFreightPrepared(t, prepare, freightAdena, [][2]int32{{potionID, 10}}, gameservertest.WithSkills(skills))
	if !a.srv.DrivesClock() {
		t.Skip("holding a cast open needs the driven clock")
	}
	a.potion = a.items[potionID]
	if inst := a.srv.PlayerInventory(t, a.player).ItemByObjectID(a.sword); inst == nil || !inst.Augmented() {
		t.Fatal("seeded sword not restored augmented")
	}
	return a
}

// startCast casts the long self skill and leaves it in flight.
func (a *augmentedCaster) startCast(t *testing.T) {
	t.Helper()
	a.c.Send(encodeRequestMagicSkillUse(longCastSkillID))
	readUntilOpcode(t, a.c, serverpackets.OpcodeMagicSkillUse, "long cast MagicSkillUse")
	readImmediate(a.c)
	a.requireCasting(t, "after the cast started")
}

// requireCasting fails unless the cast is still in flight: the request
// before it ran mid-cast.
func (a *augmentedCaster) requireCasting(t *testing.T, when string) {
	t.Helper()
	if !a.srv.PlayerCastingNow(t, a.player) {
		t.Fatalf("long cast not in flight %s", when)
	}
}

// sendMidCast sends frame, waits until the server has handled it without
// letting the cast's time pass, and returns what it answered.
func (a *augmentedCaster) sendMidCast(t *testing.T, frame []byte) [][]byte {
	t.Helper()
	a.c.Send(frame)
	a.srv.Settle(t)
	frames := readImmediate(a.c)
	a.requireCasting(t, "after the request")
	return frames
}

func (a *augmentedCaster) endCast(t *testing.T) {
	t.Helper()
	a.srv.AdvanceUntil(t, "the long cast ending", func() bool { return !a.srv.PlayerCastingNow(t, a.player) })
	drainUntilQuiet(t, a.c)
}

// TestAugmentedWeaponDepositRefusedWhileCasting deposits an augmented
// sword into the private warehouse mid-cast: the whole deposit is dropped
// with no answer, no fee and no warehouse row, the potion beside it
// included. Once the cast has ended the same deposit stores both rows for
// the fee, the sword keeping its augmentation.
func TestAugmentedWeaponDepositRefusedWhileCasting(t *testing.T) {
	t.Parallel()
	a := bootAugmentedCaster(t)
	a.command(t, "DepositP")
	a.startCast(t)

	deposit := encodeDeposit(whRow{a.potion, 1}, whRow{a.sword, 1})
	if frames := a.sendMidCast(t, deposit); len(frames) != 0 {
		t.Fatalf("mid-cast deposit of the augmented sword answered %x, want nothing", opcodes(frames))
	}
	if s, p, ad := a.held(t, a.sword), a.held(t, a.potion), a.adena(t); s != 1 || p != 10 || ad != freightAdena {
		t.Fatalf("after the mid-cast deposit: sword %d potions %d adena %d, want 1, 10 and %d", s, p, ad, freightAdena)
	}
	if got := rowsAt(saved(t, a.srv, a.player), item.LocationWarehouse); len(got) != 0 {
		t.Fatalf("saved warehouse rows after the mid-cast deposit = %v, want none", got)
	}

	a.endCast(t)
	a.c.Send(deposit)
	if frames := drainFrames(t, a.c); containsOpcode(frames, serverpackets.OpcodeSystemMessage) {
		t.Fatalf("deposit after the cast answered %x, want no message", opcodes(frames))
	}
	if s, p, ad := a.held(t, a.sword), a.held(t, a.potion), a.adena(t); s != 0 || p != 9 || ad != freightAdena-2*depositFee {
		t.Fatalf("after the deposit: sword %d potions %d adena %d, want 0, 9 and %d", s, p, ad, freightAdena-2*depositFee)
	}
	db := saved(t, a.srv, a.player)
	if got := rowsAt(db, item.LocationWarehouse); got[swordID] != 1 || got[potionID] != 1 || len(got) != 2 {
		t.Fatalf("saved warehouse rows = %v, want the sword and a potion", got)
	}
	if db[a.sword].loc != item.LocationWarehouse {
		t.Fatalf("saved sword row at %v, want the warehouse", db[a.sword].loc)
	}
	aug, ok, err := gamesql.NewAugmentationStore(a.srv.DB).Get(context.Background(), a.sword)
	if err != nil || !ok || aug != a.aug {
		t.Fatalf("stored sword's augmentation = %+v (found %v, err %v), want %+v", aug, ok, err, a.aug)
	}
}

// TestAugmentedWeaponPackageWhileCasting sends a package naming the
// augmented sword and a potion mid-cast: the sword's row is left out, its
// fee still charged, and the potion ships. Once the cast has ended a
// package naming the sword is refused whole and silently, charging
// nothing, since an augmented item is not tradable.
func TestAugmentedWeaponPackageWhileCasting(t *testing.T) {
	t.Parallel()
	a := bootAugmentedCaster(t)
	a.startCast(t)

	if frames := a.sendMidCast(t, encodePackageSend(a.receiver.ID, whRow{a.sword, 1}, whRow{a.potion, 1})); containsOpcode(frames, serverpackets.OpcodeSystemMessage) {
		t.Fatalf("mid-cast package answered %x, want no message", opcodes(frames))
	}
	if s, p, ad := a.held(t, a.sword), a.held(t, a.potion), a.adena(t); s != 1 || p != 9 || ad != freightAdena-2*freightPrice {
		t.Fatalf("after the mid-cast package: sword %d potions %d adena %d, want 1, 9 and %d", s, p, ad, freightAdena-2*freightPrice)
	}
	if got := rowsAt(saved(t, a.srv, a.receiver.ID), item.LocationFreight); got[potionID] != 1 || len(got) != 1 {
		t.Fatalf("receiver's saved freight = %v, want a potion alone", got)
	}

	a.endCast(t)
	a.c.Send(encodePackageSend(a.receiver.ID, whRow{a.sword, 1}, whRow{a.potion, 1}))
	if frames := drainFrames(t, a.c); len(frames) != 0 {
		t.Fatalf("package naming the augmented sword after the cast answered %x, want nothing", opcodes(frames))
	}
	if s, p, ad := a.held(t, a.sword), a.held(t, a.potion), a.adena(t); s != 1 || p != 9 || ad != freightAdena-2*freightPrice {
		t.Fatalf("after the refused package: sword %d potions %d adena %d, want unchanged 1, 9 and %d", s, p, ad, freightAdena-2*freightPrice)
	}
	if got := rowsAt(saved(t, a.srv, a.receiver.ID), item.LocationFreight); got[potionID] != 1 || len(got) != 1 {
		t.Fatalf("receiver's saved freight after the refused package = %v, want the one potion", got)
	}
}
