package trade

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// potionCastSkillID is a self cast that consumes one potion.
const potionCastSkillID = 5

func potionCastSkills(t *testing.T) gameservertest.Option {
	t.Helper()
	db := sqltest.SharedDB(t)
	defs := []modelskill.Definition{{
		ID: potionCastSkillID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		HitTime: 500, StaticHitTime: true, SkillType: "BUFF", ItemConsumeID: potionID, ItemConsumeCount: 1,
	}}
	return gameservertest.WithSkills(skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable(defs), gamesql.NewCharacterSkillStore(db)))
}

func encodeMagicSkillUse(skillID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestMagicSkillUse)
	w.WriteInt32(skillID)
	w.WriteInt32(0)
	w.WriteUint8(0)
	return w.Bytes()
}

// TestTradeStartHidesLastCastConsumeItem pins the available-item filter's
// cast term (ItemInstance.isAvailable reads CreatureCast.getCurrentSkill,
// which keeps the last started skill after its cast ends): once the
// potion-consuming cast is over, the requester's TradeStart still leaves
// the potions out and lists its sword.
func TestTradeStartHidesLastCastConsumeItem(t *testing.T) {
	t.Parallel()
	h := bootTraders(t, potionCastSkills(t))
	potions := h.srv.GiveItem(t, h.firstID, potionID, 10)
	sword := h.srv.GiveItem(t, h.firstID, swordID, 1)
	if err := h.srv.KnownSkills.SetKnownSkill(context.Background(), h.firstID, 0, potionCastSkillID, 1); err != nil {
		t.Fatalf("seed known skill: %v", err)
	}
	h.enterAll(t)
	obj, ok := h.srv.State.Player(h.firstID)
	if !ok {
		t.Fatal("requester missing from world state")
	}
	pc, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("requester %T is not an online character", obj)
	}

	h.first.Send(encodeMagicSkillUse(potionCastSkillID))
	h.srv.AdvanceUntil(t, "potion cast finished", func() bool {
		inst := pc.Inventory().ItemByObjectID(potions)
		return inst != nil && inst.CountValue() == 9 && !pc.CastingNow()
	})
	drainUntilQuiet(t, h.first)
	drainUntilQuiet(t, h.second)

	h.first.Send(encodeTradeRequest(h.secondID))
	assertFrameOpcode(t, h.second.Read(), serverpackets.OpcodeSendTradeRequest, "SendTradeRequest")
	assertSystemMessageText(t, h.first.Read(), serverpackets.SystemMessageRequestS1ForTrade, "TraderTwo")
	h.second.Send(encodeAnswerTradeRequest(1))
	assertSystemMessageText(t, h.first.Read(), serverpackets.SystemMessageBeginTradeWithS1, "TraderTwo")
	frame := h.first.Read()
	assertFrameOpcode(t, frame, serverpackets.OpcodeTradeStart, "first TradeStart")

	r := wire.NewReader(frame[1:])
	r.ReadInt32() // partner id
	rows := r.ReadUint16()
	r.ReadUint16() // category
	listed := r.ReadInt32()
	if err := r.Err(); err != nil || rows != 1 || listed != sword {
		t.Fatalf("first TradeStart rows = %d, first object %d (err %v), want only the sword %d", rows, listed, err, sword)
	}
}
