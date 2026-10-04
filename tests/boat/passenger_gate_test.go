package boat

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/privatestore"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Fixtures of the refusals aboard: the shared catalog's wolf collar, wyvern
// collar and decoration kit, mapped to their summon kinds; a servitor
// summon skill; and a fishing rod with the Fishing skill.
const (
	wolfCollarID     = int32(9600)
	wyvernCollarID   = int32(9601)
	treeKitID        = int32(9602)
	summonCatSkillID = 1111
	fishingSkillID   = 1312
	fishingRodID     = int32(6530)
)

func encodeUseItem(objectID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeUseItem)
	w.WriteInt32(objectID)
	w.WriteInt32(0) // ctrl
	return w.Bytes()
}

func encodeRequestMagicSkillUse(skillID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestMagicSkillUse)
	w.WriteInt32(skillID)
	w.WriteInt32(0) // ctrl
	w.WriteUint8(0) // shift
	return w.Bytes()
}

// TestSummonItemAboardRefused pins SummonItems' boat refusal for every
// summon kind: aboard, a pet collar, a wyvern collar and a decoration kit
// are each answered NOT_CALL_PET_FROM_THIS_LOCATION alone, and the item is
// kept.
func TestSummonItemAboardRefused(t *testing.T) {
	t.Parallel()
	summons, err := item.NewSummonItemTable([]item.SummonItem{
		{ItemID: wolfCollarID, NPCID: 12500, SummonType: 1},
		{ItemID: wyvernCollarID, NPCID: 12621, SummonType: 2},
		{ItemID: treeKitID, NPCID: 13006, SummonType: 0},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name     string
		template int32
	}{
		{"pet collar", wolfCollarID},
		{"wyvern collar", wyvernCollarID},
		{"decoration kit", treeKitID},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var used int32
			srv, c, me, b := bootPassengerWith(t, runeBoarding, 0, func(srv *gameservertest.Server, objID int32) {
				used = srv.GiveItem(t, objID, tt.template, 1)
			}, gameservertest.WithSummonItems(summons))
			board(t, srv, c, b)

			c.Send(encodeUseItem(used))
			assertLog(t, tt.name+" aboard", passengerLog(t, srv, c), sm(serverpackets.SystemMessageNotCallPetFromThisLocation))
			if got := srv.PlayerInventory(t, me).ItemCount(tt.template, -1, false); got != 1 {
				t.Fatalf("%s count %d after the refusal, want 1", tt.name, got)
			}
		})
	}
}

// TestServitorSummonAboardRefused pins the servitor cast refused aboard:
// NOT_CALL_PET_FROM_THIS_LOCATION alone, with no ActionFailed and no cast.
func TestServitorSummonAboardRefused(t *testing.T) {
	t.Parallel()
	db := sqltest.SharedDB(t)
	skills := skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{{
		ID: summonCatSkillID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		SkillType: "SUMMON", NpcID: 12600, SummonTotalLifeTime: 1_200_000,
		StaticHitTime: true, HitTime: 0, StaticReuse: true, ReuseDelay: 0,
	}}), gamesql.NewCharacterSkillStore(db))
	srv, c, me, b := bootPassengerWith(t, runeBoarding, 0, func(srv *gameservertest.Server, objID int32) {
		if err := srv.KnownSkills.SetKnownSkill(context.Background(), objID, 0, summonCatSkillID, 1); err != nil {
			t.Fatalf("seed known skill: %v", err)
		}
	}, gameservertest.WithSkills(skills))
	board(t, srv, c, b)

	c.Send(encodeRequestMagicSkillUse(summonCatSkillID))
	frames := srv.ReadQueued(t, c)
	if len(frames) != 1 {
		t.Fatalf("servitor cast aboard = opcodes %x, want one system message", opcodes(frames))
	}
	if s, _ := describePassenger(frames[0]); s != sm(serverpackets.SystemMessageNotCallPetFromThisLocation) {
		t.Fatalf("servitor cast aboard = %q, want %q", s, sm(serverpackets.SystemMessageNotCallPetFromThisLocation))
	}
	if srv.PlayerCastingNow(t, me) {
		t.Fatal("servitor cast aboard left a cast running")
	}
}

// TestFishingAboardRefused pins Fishing's boat refusal: a passenger with a
// rod in hand casting Fishing is told CANNOT_FISH_ON_BOAT, and no line is
// cast.
func TestFishingAboardRefused(t *testing.T) {
	t.Parallel()
	db := sqltest.SharedDB(t)
	skills := skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{{
		ID: fishingSkillID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf, SkillType: "FISHING",
	}}), gamesql.NewCharacterSkillStore(db))
	catalog := item.NewTable(append(gameservertest.ItemTemplates().All(), &item.Template{
		ID: fishingRodID, Name: "Albatross Rod", Kind: item.KindWeapon, Slot: item.SlotLRHand, Duration: -1,
		Crystal: item.CrystalD, Destroyable: true, DefaultAction: item.ActionEquip,
		Weapon: &item.WeaponDetail{Type: item.WeaponFishingRod},
	}))
	var rod int32
	srv, c, _, b := bootPassengerWith(t, runeBoarding, 0, func(srv *gameservertest.Server, objID int32) {
		if err := srv.KnownSkills.SetKnownSkill(context.Background(), objID, 0, fishingSkillID, 1); err != nil {
			t.Fatalf("seed known skill: %v", err)
		}
		rod = srv.GiveItem(t, objID, fishingRodID, 1)
	}, gameservertest.WithSkills(skills), gameservertest.WithItemTemplates(catalog))
	c.Send(encodeUseItem(rod))
	readUntilQuiet(c)
	board(t, srv, c, b)

	c.Send(encodeRequestMagicSkillUse(fishingSkillID))
	srv.Advance(t, 100*time.Millisecond)
	frames := srv.ReadQueued(t, c)
	var messages []string
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeExtended && wire.NewReader(f[1:]).ReadUint16() == serverpackets.OpcodeExFishingStart {
			t.Fatalf("a line was cast aboard: opcodes %x", opcodes(frames))
		}
		if s, ok := describePassenger(f); ok && strings.HasPrefix(s, "sm ") {
			messages = append(messages, s)
		}
	}
	if !slices.Contains(messages, sm(serverpackets.SystemMessageCannotFishOnBoat)) {
		t.Fatalf("fishing aboard: messages %q, want %q", messages, sm(serverpackets.SystemMessageCannotFishOnBoat))
	}
}

// TestOustedStoreClosedFirst pins a passenger keeping a store open who
// cannot pay the fare: its store is closed, and shown closed, before it is
// teleported ashore.
func TestOustedStoreClosedFirst(t *testing.T) {
	t.Parallel()
	srv, c, me, b := bootPassenger(t, runeBoarding, 0)
	board(t, srv, c, b)
	srv.SetPlayerOperateType(t, me, privatestore.OperateSell)

	sail(srv, 307)
	srv.ReadQueued(t, c)
	sail(srv, 1)
	userInfo, teleported := -1, -1
	for i, f := range srv.ReadQueued(t, c) {
		switch f[0] {
		case serverpackets.OpcodeUserInfo:
			if userInfo < 0 {
				userInfo = i
			}
		case serverpackets.OpcodeTeleportToLocation:
			teleported = i
		}
	}
	if teleported < 0 || userInfo < 0 || userInfo > teleported {
		t.Fatalf("UserInfo at %d, teleport at %d: want the store shown closed before the teleport", userInfo, teleported)
	}
	if got := srv.PlayerOperateType(t, me); got != privatestore.OperateNone {
		t.Fatalf("store after the oust %v, want closed", got)
	}
}

// opcodes lists the opcodes of frames.
func opcodes(frames [][]byte) []byte {
	out := make([]byte, 0, len(frames))
	for _, f := range frames {
		out = append(out, f[0])
	}
	return out
}
