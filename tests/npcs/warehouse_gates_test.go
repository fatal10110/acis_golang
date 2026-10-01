package npcs

import (
	"context"
	"strconv"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: WarehouseKeeper.onBypassFeedback FreightChar (WarehouseKeeper.java:162-175)
// tags the deposited freight with the keeper's region when RegionBasedFreight
// is on, then answers ActionFailed and WarehouseDepositList(4);
// PcFreight.validateCapacity with no owner holds MaximumFreightSlots;
// SendWarehouseDepositList.java:80-81, SendWarehouseWithdrawList.java:76-77
// and RequestPackageSend.java:74-75 return silently for a player carrying
// karma while KarmaPlayerCanUseWareHouse is off; RequestPackageSend.java:99-100
// drops the whole package on a non-tradable or quest row before any fee,
// and :136-137 keeps a hero item back after the fee. aCis revision in the
// outer repo.

const (
	questItemID = 7001
	heroSwordID = 6611
)

// setCharacterColumn writes column of the persisted character objID.
func setCharacterColumn(t *testing.T, srv *gameservertest.Server, objID int32, column string, value int) {
	t.Helper()
	query := "UPDATE characters SET " + column + " = ? WHERE obj_Id = ?"
	if _, err := srv.DB.ExecContext(context.Background(), query, value, objID); err != nil {
		t.Fatalf("set %s for %d: %v", column, objID, err)
	}
}

// TestFreightCharDepositsIntoAccountCharacter walks FreightChar_<id> into
// the other character of the account: an id outside the account opens
// nothing; the receiver's freight opens as WarehouseDepositList(4) held to
// the base freight size, counting only rows of the keeper's region; the
// deposited row carries the keeper's region; at the receiver's next login a
// keeper of that region lists it while a row tagged for another region
// stays out of the list.
func TestFreightCharDepositsIntoAccountCharacter(t *testing.T) {
	t.Parallel()
	var elsewhere int32
	var elsewhereKey int
	var stranger *player.Character
	w := bootFreightPrepared(t, func(srv *gameservertest.Server, _ int32, receiver *player.Character) {
		stranger = srv.SeedCharacterFor(t, "stranger", "Stranger", playerLevel, 0)
		elsewhere = srv.NewObjectID()
		// A potion already waits in the receiver's freight, deposited at a
		// keeper two regions east of here.
		x, y, _ := receiver.Position()
		elsewhereKey = world.RegionKey(x+2*2048, y)
		if err := srv.Items.Create(context.Background(), receiver.ID, item.Instance{
			ObjectID: elsewhere, TemplateID: potionID, OwnerID: receiver.ID, Count: 2,
			Location: item.LocationFreight, LocationData: elsewhereKey,
		}); err != nil {
			t.Fatalf("seed freight row: %v", err)
		}
	}, whAdena, [][2]int32{{swordID, 1}, {potionID, 10}},
		gameservertest.WithStorageSlots(player.StorageSlots{WarehouseNoDwarf: 100, WarehouseDwarf: 100, Freight: 1, PrivateStoreNoDwarf: 4, PrivateStoreDwarf: 5, DwarfRecipe: 50, CommonRecipe: 50}))
	sword, potion := w.items[swordID], w.items[potionID]
	kx, ky, _ := w.keeper.Position()
	here := world.RegionKey(kx, ky)
	if here == 0 || elsewhereKey == 0 || here == elsewhereKey {
		t.Fatalf("region keys here %d elsewhere %d, want two distinct regions", here, elsewhereKey)
	}

	requireOpcodes(t, "FreightChar_ of a character on another account", w.command(t, "FreightChar_"+strconv.Itoa(int(stranger.ID))), serverpackets.OpcodeActionFailed)
	w.c.Send(encodeDeposit(whRow{sword, 1}))
	if frames := drainFrames(t, w.c); len(frames) != 0 {
		t.Fatalf("deposit after FreightChar_ of a stranger answered %x, want nothing", opcodes(frames))
	}
	if got := w.held(t, sword); got != 1 {
		t.Fatalf("sword held after the stranger's FreightChar_ deposit = %d, want 1", got)
	}

	frames := w.command(t, "FreightChar_"+strconv.Itoa(int(w.receiver.ID)))
	requireOpcodes(t, "FreightChar_", frames, serverpackets.OpcodeActionFailed, serverpackets.OpcodeWarehouseDepositList, serverpackets.OpcodeActionFailed)
	if whType, adena, _, rows := decodeWarehouseList(t, frames[1], serverpackets.OpcodeWarehouseDepositList); whType != 4 || adena != whAdena || rows[sword] != (listRow{swordID, 1}) {
		t.Fatalf("FreightChar_ deposit list = type %d adena %d %v, want 4, %d and the sword", whType, adena, rows, whAdena)
	}

	// The potion waiting elsewhere takes no slot here, and is no stack to
	// merge into: the sword and the potions need two of the one slot.
	w.c.Send(encodeDeposit(whRow{sword, 1}, whRow{potion, 3}))
	requireMessage(t, "freight deposit over the base freight size", drainFrames(t, w.c), int(quantityLimit), false)
	if s, p, a := w.held(t, sword), w.held(t, potion), w.adena(t); s != 1 || p != 10 || a != whAdena {
		t.Fatalf("after the refused freight deposit: sword %d potions %d adena %d, want 1, 10 and %d", s, p, a, whAdena)
	}

	w.c.Send(encodeDeposit(whRow{sword, 1}))
	if frames := drainFrames(t, w.c); containsOpcode(frames, serverpackets.OpcodeSystemMessage) {
		t.Fatalf("freight deposit answered %x, want no message", opcodes(frames))
	}
	if s, a := w.held(t, sword), w.adena(t); s != 0 || a != whAdena-depositFee {
		t.Fatalf("after the freight deposit: sword %d adena %d, want 0 and %d", s, a, whAdena-depositFee)
	}
	w.c.Send(encodeDeposit(whRow{potion, 1}))
	requireMessage(t, "freight deposit into the full freight", drainFrames(t, w.c), int(quantityLimit), false)

	w.srv.FlushItems(t)
	rows, err := w.srv.Items.ListByOwner(context.Background(), w.receiver.ID)
	if err != nil {
		t.Fatalf("list receiver items: %v", err)
	}
	var shipped *item.Instance
	for _, row := range rows {
		if row.ObjectID == sword {
			shipped = row
		}
	}
	if shipped == nil || shipped.Location != item.LocationFreight || shipped.LocationData != here {
		t.Fatalf("receiver's sword row = %+v, want in freight tagged %d", shipped, here)
	}

	rw := w.loginReceiver(t)
	rkx, rky, _ := rw.keeper.Position()
	if got := world.RegionKey(rkx, rky); got != here {
		t.Fatalf("receiver's keeper region = %d, want the sender's %d", got, here)
	}
	frames = rw.command(t, "WithdrawF")
	requireOpcodes(t, "receiver's WithdrawF", frames, serverpackets.OpcodeActionFailed, serverpackets.OpcodeWarehouseWithdrawList, serverpackets.OpcodeActionFailed)
	if _, _, _, listed := decodeWarehouseList(t, frames[1], serverpackets.OpcodeWarehouseWithdrawList); len(listed) != 1 || listed[sword] != (listRow{swordID, 1}) {
		t.Fatalf("receiver's freight here = %v, want only the sword, not the potion of the other region", listed)
	}
	// The withdrawal looks a row up by object id across every region, as
	// the reference's does: a row named directly still comes out.
	rw.c.Send(encodeWithdraw(whRow{elsewhere, 2}))
	drainUntilQuiet(t, rw.c)
	if got := rw.held(t, elsewhere); got != 2 {
		t.Fatalf("potions of the other region withdrawn by object id = %d, want 2", got)
	}
}

// TestPackageSendRefusesUntradableRows pins the whole-package refusal: a
// package naming a tradable sword beside a non-tradable tunic, or beside a
// quest item, charges nothing, moves nothing and writes no freight row.
func TestPackageSendRefusesUntradableRows(t *testing.T) {
	t.Parallel()
	catalog := gameservertest.ItemTemplates().All()
	var quest item.Template
	for _, tmpl := range catalog {
		switch tmpl.ID {
		case tunicID:
			tmpl.Tradable = false
		case potionID:
			quest = *tmpl
			etc := *tmpl.EtcItem
			etc.Type = item.EtcItemQuest
			quest.ID, quest.Name, quest.EtcItem = questItemID, "Quest Token", &etc
		}
	}
	catalog = append(catalog, &quest)
	w := bootFreight(t, freightAdena, [][2]int32{{swordID, 1}, {tunicID, 1}, {questItemID, 3}},
		gameservertest.WithItemTemplates(item.NewTable(catalog)))
	sword, tunic, token := w.items[swordID], w.items[tunicID], w.items[questItemID]

	for _, tc := range []struct {
		what string
		rows []whRow
	}{
		{"package with a non-tradable tunic", []whRow{{sword, 1}, {tunic, 1}}},
		{"package with a quest item", []whRow{{sword, 1}, {token, 3}}},
	} {
		w.c.Send(encodePackageSend(w.receiver.ID, tc.rows...))
		if frames := drainFrames(t, w.c); len(frames) != 0 {
			t.Fatalf("%s answered %x, want nothing", tc.what, opcodes(frames))
		}
		if s, tu, q, a := w.held(t, sword), w.held(t, tunic), w.held(t, token), w.adena(t); s != 1 || tu != 1 || q != 3 || a != freightAdena {
			t.Fatalf("%s: sword %d tunic %d tokens %d adena %d, want 1, 1, 3 and %d", tc.what, s, tu, q, a, freightAdena)
		}
		if got := saved(t, w.srv, w.receiver.ID); len(got) != 0 {
			t.Fatalf("%s: receiver's saved rows = %v, want none", tc.what, got)
		}
	}
}

// TestPackageSendKeepsHeroItemBack pins the hero-item skip: a package
// naming a hero weapon and potions charges both rows, ships the potions
// and keeps the weapon with the sender.
func TestPackageSendKeepsHeroItemBack(t *testing.T) {
	t.Parallel()
	catalog := gameservertest.ItemTemplates().All()
	var hero item.Template
	for _, tmpl := range catalog {
		if tmpl.ID == swordID {
			hero = *tmpl
		}
	}
	hero.ID, hero.Name = heroSwordID, "Infinity Blade"
	if !hero.HeroItem() || !hero.Tradable {
		t.Fatalf("template %d is not a tradable hero item", hero.ID)
	}
	catalog = append(catalog, &hero)
	w := bootFreight(t, freightAdena, [][2]int32{{heroSwordID, 1}, {potionID, 10}},
		gameservertest.WithItemTemplates(item.NewTable(catalog)))
	blade, potion := w.items[heroSwordID], w.items[potionID]

	w.c.Send(encodePackageSend(w.receiver.ID, whRow{blade, 1}, whRow{potion, 4}))
	if frames := drainFrames(t, w.c); containsOpcode(frames, serverpackets.OpcodeSystemMessage) {
		t.Fatalf("package answered %x, want no message", opcodes(frames))
	}
	if b, p, a := w.held(t, blade), w.held(t, potion), w.adena(t); b != 1 || p != 6 || a != freightAdena-2*freightPrice {
		t.Fatalf("after the package: hero blade %d potions %d adena %d, want 1, 6 and %d", b, p, a, freightAdena-2*freightPrice)
	}
	if got := rowsAt(saved(t, w.srv, w.receiver.ID), item.LocationFreight); len(got) != 1 || got[potionID] != 4 {
		t.Fatalf("receiver's saved freight = %v, want only the 4 potions", got)
	}
	if got := saved(t, w.srv, w.player)[blade].loc; got != item.LocationInventory {
		t.Fatalf("hero blade saved at %v, want the sender's inventory", got)
	}
}

// TestWarehouseKarmaGate pins KarmaPlayerCanUseWareHouse on the three
// requests of a player carrying karma: with the gate off a deposit, a
// withdrawal and a package each answer nothing and change nothing, though
// the keeper's windows still open; with it on, each goes through.
func TestWarehouseKarmaGate(t *testing.T) {
	t.Parallel()
	for _, allowed := range []bool{false, true} {
		t.Run("allowed="+strconv.FormatBool(allowed), func(t *testing.T) {
			t.Parallel()
			var stored int32
			w := bootFreightPrepared(t, func(srv *gameservertest.Server, sender int32, _ *player.Character) {
				setCharacterColumn(t, srv, sender, "karma", 240)
				stored = srv.NewObjectID()
				if err := srv.Items.Create(context.Background(), sender, item.Instance{
					ObjectID: stored, TemplateID: swordID, OwnerID: sender, Count: 1, Location: item.LocationWarehouse,
				}); err != nil {
					t.Fatalf("seed warehouse row: %v", err)
				}
			}, freightAdena, [][2]int32{{potionID, 10}}, gameservertest.WithKarmaServiceGates(false, false, allowed))
			potion := w.items[potionID]
			moved := 0
			if allowed {
				moved = 1
			}
			answered := func(what string) {
				t.Helper()
				frames := drainFrames(t, w.c)
				if !allowed && len(frames) != 0 {
					t.Fatalf("%s with karma answered %x, want nothing", what, opcodes(frames))
				}
				if containsOpcode(frames, serverpackets.OpcodeSystemMessage) {
					t.Fatalf("%s answered %x, want no message", what, opcodes(frames))
				}
			}

			requireOpcodes(t, "DepositP", w.command(t, "DepositP"), serverpackets.OpcodeActionFailed, serverpackets.OpcodeWarehouseDepositList, serverpackets.OpcodeActionFailed)
			w.c.Send(encodeDeposit(whRow{potion, 1}))
			answered("deposit")
			if p, a := w.held(t, potion), w.adena(t); p != 10-moved || a != freightAdena-moved*depositFee {
				t.Fatalf("after the deposit: potions %d adena %d, want %d and %d", p, a, 10-moved, freightAdena-moved*depositFee)
			}

			requireOpcodes(t, "WithdrawP", w.command(t, "WithdrawP"), serverpackets.OpcodeWarehouseWithdrawList, serverpackets.OpcodeActionFailed, serverpackets.OpcodeActionFailed)
			w.c.Send(encodeWithdraw(whRow{stored, 1}))
			answered("withdrawal")
			if got := w.held(t, stored); got != moved {
				t.Fatalf("sword held after the withdrawal = %d, want %d", got, moved)
			}

			w.c.Send(encodePackageSend(w.receiver.ID, whRow{potion, 2}))
			answered("package")
			afterDeposit := 10 - moved
			if p := w.held(t, potion); p != afterDeposit-2*moved {
				t.Fatalf("potions after the package = %d, want %d", p, afterDeposit-2*moved)
			}

			db := saved(t, w.srv, w.player)
			wantWarehouse := map[int32]int{swordID: 1}
			if allowed {
				wantWarehouse = map[int32]int{potionID: 1}
			}
			if got := rowsAt(db, item.LocationWarehouse); len(got) != len(wantWarehouse) || got[swordID] != wantWarehouse[swordID] || got[potionID] != wantWarehouse[potionID] {
				t.Fatalf("saved warehouse rows = %v, want %v", got, wantWarehouse)
			}
			if got := rowsAt(saved(t, w.srv, w.receiver.ID), item.LocationFreight); got[potionID] != 2*moved || len(got) != moved {
				t.Fatalf("receiver's saved freight = %v, want %d potions", got, 2*moved)
			}
		})
	}
}
