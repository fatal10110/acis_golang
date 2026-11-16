package npcs

import (
	"context"
	"testing"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// whPotionCastSkillID is a self cast that consumes one potion.
const whPotionCastSkillID = 5

// TestDepositListHidesLastCastConsumeItem pins the available-item filter's
// cast term (ItemInstance.isAvailable reads CreatureCast.getCurrentSkill,
// which keeps the last started skill after its cast ends): once the
// potion-consuming cast is over, the private deposit list still leaves the
// potions out, listing the sword and adena.
func TestDepositListHidesLastCastConsumeItem(t *testing.T) {
	t.Parallel()
	db := sqltest.SharedDB(t)
	defs := []modelskill.Definition{{
		ID: whPotionCastSkillID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		HitTime: 500, StaticHitTime: true, SkillType: "BUFF", ItemConsumeID: potionID, ItemConsumeCount: 1,
	}}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Keeper", playerLevel, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithHTMLPages(warehousePages()),
		noBypassReuse,
		gameservertest.WithSkills(skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable(defs), gamesql.NewCharacterSkillStore(db))),
	)
	w := &whWorld{folkWorld: &folkWorld{srv: srv, c: srv.Client, player: srv.SoleObjectID(t)}, items: map[int32]int32{}}
	w.items[item.AdenaID] = srv.GiveItem(t, w.player, item.AdenaID, whAdena)
	w.items[swordID] = srv.GiveItem(t, w.player, swordID, 1)
	w.items[potionID] = srv.GiveItem(t, w.player, potionID, 10)
	if err := srv.KnownSkills.SetKnownSkill(context.Background(), w.player, 0, whPotionCastSkillID, 1); err != nil {
		t.Fatalf("seed known skill: %v", err)
	}
	w.enter(t)
	obj, ok := srv.State.Player(w.player)
	if !ok {
		t.Fatal("player missing from world state")
	}
	pc, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("player %T is not an online character", obj)
	}

	w.c.Send(encodeRequestMagicSkillUse(whPotionCastSkillID))
	srv.AdvanceUntil(t, "potion cast finished", func() bool {
		inst := pc.Inventory().ItemByObjectID(w.items[potionID])
		return inst != nil && inst.CountValue() == 9 && !pc.CastingNow()
	})
	drainUntilQuiet(t, w.c)
	w.openAnyNpcPage(t)

	frames := w.command(t, "DepositP")
	requireOpcodes(t, "DepositP", frames, serverpackets.OpcodeActionFailed, serverpackets.OpcodeWarehouseDepositList, serverpackets.OpcodeActionFailed)
	_, _, _, rows := decodeWarehouseList(t, frames[1], serverpackets.OpcodeWarehouseDepositList)
	want := map[int32]listRow{w.items[swordID]: {swordID, 1}, w.items[item.AdenaID]: {item.AdenaID, whAdena}}
	if len(rows) != len(want) {
		t.Fatalf("deposit list = %v, want %v", rows, want)
	}
	for objectID, row := range want {
		if rows[objectID] != row {
			t.Fatalf("deposit list row %d = %+v, want %+v", objectID, rows[objectID], row)
		}
	}
}
