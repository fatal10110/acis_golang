package network

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attack"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	gamecipher "github.com/fatal10110/acis_golang/internal/gameserver/network/cipher"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/link"
	"github.com/rs/zerolog"
)

// ---- from client_test.go ----
func TestStateStringNamesEachState(t *testing.T) {
	tests := []struct {
		state State
		want  string
	}{
		{StateConnected, "connected"},
		{StateAuthed, "authed"},
		{StateEntering, "entering"},
		{StateInGame, "in-game"},
		{State(99), "state(99)"},
	}
	for _, tt := range tests {
		if got := tt.state.String(); got != tt.want {
			t.Errorf("State(%d).String() = %q, want %q", int(tt.state), got, tt.want)
		}
	}
}

func TestAllowedGatesOpcodesByState(t *testing.T) {
	tests := []struct {
		name   string
		state  State
		opcode byte
		want   bool
	}{
		{"connected accepts protocol version", StateConnected, 0x00, true},
		{"connected accepts login", StateConnected, 0x08, true},
		{"connected rejects create character before auth", StateConnected, 0x0b, false},
		{"connected rejects enter world", StateConnected, 0x03, false},

		{"authed accepts create character", StateAuthed, 0x0b, true},
		{"authed accepts select character", StateAuthed, 0x0d, true},
		{"authed accepts logout", StateAuthed, 0x09, true},
		{"authed rejects protocol version replay", StateAuthed, 0x00, false},
		{"authed rejects enter world before slot chosen", StateAuthed, 0x03, false},

		{"entering accepts enter world", StateEntering, 0x03, true},
		{"entering accepts quest list", StateEntering, 0x3f, true},
		{"entering rejects create character", StateEntering, 0x0b, false},

		{"in-game accepts logout", StateInGame, 0x09, true},
		{"in-game accepts click movement", StateInGame, 0x01, true},
		{"in-game accepts validate position", StateInGame, 0x48, true},
		{"in-game accepts action", StateInGame, 0x04, true},
		{"in-game accepts attack request", StateInGame, 0x0a, true},
		{"in-game accepts item list refresh", StateInGame, 0x0f, true},
		{"in-game accepts skill list refresh", StateInGame, 0x3f, true},
		{"in-game accepts use item", StateInGame, 0x14, true},
		{"in-game accepts enchant item", StateInGame, 0x58, true},
		{"in-game accepts pet item use", StateInGame, 0x8a, true},
		{"in-game accepts bypass command", StateInGame, 0x21, true},
		{"in-game rejects create character", StateInGame, 0x0b, false},
		{"in-game rejects enter world replay", StateInGame, 0x03, false},

		{"unknown opcode rejected in every state", StateConnected, 0xFF, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Allowed(tt.state, tt.opcode); got != tt.want {
				t.Errorf("Allowed(%s, 0x%02x) = %v, want %v", tt.state, tt.opcode, got, tt.want)
			}
		})
	}
}

func TestAllowedAcceptsWireSafeInGameOpcodes(t *testing.T) {
	opcodes := []byte{
		clientpackets.OpcodeMoveBackwardToLocation,
		clientpackets.OpcodeAction,
		clientpackets.OpcodeAttackRequest,
		clientpackets.OpcodeRequestItemList,
		clientpackets.OpcodeRequestUnEquipItem,
		clientpackets.OpcodeRequestDropItem,
		clientpackets.OpcodeUseItem,
		clientpackets.OpcodeTradeRequest,
		clientpackets.OpcodeAddTradeItem,
		clientpackets.OpcodeTradeDone,
		clientpackets.OpcodeDummy1A,
		clientpackets.OpcodeRequestSocialAction,
		clientpackets.OpcodeRequestChangeMoveType,
		clientpackets.OpcodeRequestChangeWaitType,
		clientpackets.OpcodeRequestSellItem,
		clientpackets.OpcodeRequestBuyItem,
		clientpackets.OpcodeRequestLinkHtml,
		clientpackets.OpcodeRequestBypassToServer,
		clientpackets.OpcodeDummy23,
		clientpackets.OpcodeDummy2E,
		clientpackets.OpcodeRequestMagicSkillUse,
		clientpackets.OpcodeAppearing,
		clientpackets.OpcodeSendWarehouseDeposit,
		clientpackets.OpcodeSendWarehouseWithdraw,
		clientpackets.OpcodeRequestShortCutReg,
		clientpackets.OpcodeDummy34,
		clientpackets.OpcodeRequestShortCutDel,
		clientpackets.OpcodeCannotMoveAnymore,
		clientpackets.OpcodeRequestTargetCancel,
		clientpackets.OpcodeDummy3E,
		clientpackets.OpcodeRequestSkillList,
		clientpackets.OpcodeRequestGetOnVehicle,
		clientpackets.OpcodeRequestGetOffVehicle,
		clientpackets.OpcodeAnswerTradeRequest,
		clientpackets.OpcodeRequestActionUse,
		clientpackets.OpcodeRequestRestart,
		clientpackets.OpcodeValidatePosition,
		clientpackets.OpcodeStartRotating,
		clientpackets.OpcodeFinishRotating,
		clientpackets.OpcodeRequestEnchantItem,
		clientpackets.OpcodeRequestDestroyItem,
		clientpackets.OpcodeRequestMoveInVehicle,
		clientpackets.OpcodeCannotMoveInVehicle,
		clientpackets.OpcodeRequestQuestListInGame,
		clientpackets.OpcodeRequestQuestAbort,
		clientpackets.OpcodeRequestAcquireSkillInfo,
		clientpackets.OpcodeRequestAcquireSkill,
		clientpackets.OpcodeRequestRestartPoint,
		clientpackets.OpcodeRequestCrystallizeItem,
		clientpackets.OpcodeRequestAllyCrest,
		clientpackets.OpcodeRequestChangePetName,
		clientpackets.OpcodeRequestPetUseItem,
		clientpackets.OpcodeRequestGiveItemToPet,
		clientpackets.OpcodeRequestGetItemFromPet,
		clientpackets.OpcodeRequestPetGetItem,
		clientpackets.OpcodeSendTimeCheck,
		clientpackets.OpcodeRequestSkillCoolTime,
		clientpackets.OpcodeRequestPackageItemList,
		clientpackets.OpcodeRequestPackageSend,
		clientpackets.OpcodeDlgAnswer,
		clientpackets.OpcodeGameGuardReply,
		clientpackets.OpcodeRequestShowMiniMap,
		clientpackets.OpcodeExtended,
	}
	for _, opcode := range opcodes {
		if !Allowed(StateInGame, opcode) {
			t.Fatalf("Allowed(in-game, 0x%02x) = false, want true", opcode)
		}
	}
}

func TestClientStartsConnected(t *testing.T) {
	c := NewClient(nil)
	if got := c.State(); got != StateConnected {
		t.Fatalf("new client state = %s, want %s", got, StateConnected)
	}
	if name := c.AccountName(); name != "" {
		t.Fatalf("new client account name = %q, want empty", name)
	}
}

func TestClientAcceptRejectsCreateCharacterBeforeAuth(t *testing.T) {
	c := NewClient(nil)

	if c.Accept(0x0b) {
		t.Fatal("Accept(create character) = true before auth, want false")
	}

	c.SetAuthenticated("player1", link.SessionKey{})

	if !c.Accept(0x0b) {
		t.Fatal("Accept(create character) = false after auth, want true")
	}
}

func TestClientSetAuthenticatedAdvancesState(t *testing.T) {
	c := NewClient(nil)
	key := link.SessionKey{LoginKey1: 1, LoginKey2: 2, PlayKey1: 3, PlayKey2: 4}

	c.SetAuthenticated("player1", key)

	if got := c.State(); got != StateAuthed {
		t.Fatalf("state after SetAuthenticated = %s, want %s", got, StateAuthed)
	}
	if got := c.AccountName(); got != "player1" {
		t.Fatalf("account name after SetAuthenticated = %q, want %q", got, "player1")
	}
	if got := c.SessionKey(); got != key {
		t.Fatalf("session key after SetAuthenticated = %+v, want %+v", got, key)
	}
}

func TestClientStateTransitionsThroughLifecycle(t *testing.T) {
	c := NewClient(nil)

	steps := []State{StateConnected, StateAuthed, StateEntering, StateInGame}
	for i, want := range steps {
		if i > 0 {
			c.SetState(want)
		}
		if got := c.State(); got != want {
			t.Fatalf("step %d: state = %s, want %s", i, got, want)
		}
	}
}

func TestClientStateIsSafeForConcurrentAccess(t *testing.T) {
	c := NewClient(nil)

	var wg sync.WaitGroup
	states := []State{StateConnected, StateAuthed, StateEntering, StateInGame}
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			c.SetState(states[i%len(states)])
		}(i)
		go func() {
			defer wg.Done()
			c.State()
		}()
	}
	wg.Wait()
}

// ---- from dispatch_encoders_test.go ----
func encodeProtocolVersion(revision int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeProtocolVersion)
	w.WriteInt32(revision)
	return w.Bytes()
}

func encodeAuthLogin(name string, key link.SessionKey) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeAuthLogin)
	w.WriteString(name)
	w.WriteInt32(key.PlayKey2)
	w.WriteInt32(key.PlayKey1)
	w.WriteInt32(key.LoginKey1)
	w.WriteInt32(key.LoginKey2)
	return w.Bytes()
}

func encodeRequestCharacterCreate(name string, race, sex, classID int32, hairStyle, hairColor, face byte) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestCharacterCreate)
	w.WriteString(name)
	w.WriteInt32(race)
	w.WriteInt32(sex)
	w.WriteInt32(classID)
	for i := 0; i < 6; i++ {
		w.WriteInt32(0)
	}
	w.WriteInt32(int32(hairStyle))
	w.WriteInt32(int32(hairColor))
	w.WriteInt32(int32(face))
	return w.Bytes()
}

func encodeRequestGameStart(slot int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestGameStart)
	w.WriteInt32(slot)
	w.WriteUint16(0)
	w.WriteInt32(0)
	w.WriteInt32(0)
	w.WriteInt32(0)
	return w.Bytes()
}

func encodeEnterWorld() []byte {
	return wire.NewPacketWriter(clientpackets.OpcodeEnterWorld).Bytes()
}

func encodeRequestManorList() []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeExtended)
	w.WriteUint16(clientpackets.OpcodeRequestManorList)
	return w.Bytes()
}

func encodeUnknownExtendedOpcode() []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeExtended)
	w.WriteUint16(0xffff) // unmapped second-opcode, must count as unknown
	return w.Bytes()
}

func encodeRequestCursedWeaponList() []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeExtended)
	w.WriteUint16(clientpackets.OpcodeRequestCursedWeaponList)
	return w.Bytes()
}

func encodeRequestCursedWeaponLocation() []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeExtended)
	w.WriteUint16(clientpackets.OpcodeRequestCursedWeaponLocation)
	return w.Bytes()
}

func encodeRequestAutoSoulShot(itemID, typ int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeExtended)
	w.WriteUint16(clientpackets.OpcodeRequestAutoSoulShot)
	w.WriteInt32(itemID)
	w.WriteInt32(typ)
	return w.Bytes()
}

func encodeUseItem(objectID int32, ctrl bool) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeUseItem)
	w.WriteInt32(objectID)
	w.WriteInt32(wire.BoolInt32(ctrl))
	return w.Bytes()
}

func encodeRequestEnchantItem(objectID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestEnchantItem)
	w.WriteInt32(objectID)
	return w.Bytes()
}

func encodeRequestAcquireSkillInfo(skillID, level, skillType int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestAcquireSkillInfo)
	w.WriteInt32(skillID)
	w.WriteInt32(level)
	w.WriteInt32(skillType)
	return w.Bytes()
}

func encodeRequestAcquireSkill(skillID, level, skillType int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestAcquireSkill)
	w.WriteInt32(skillID)
	w.WriteInt32(level)
	w.WriteInt32(skillType)
	return w.Bytes()
}

func encodeRequestPackageSendableItemList(objectID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestPackageItemList)
	w.WriteInt32(objectID)
	return w.Bytes()
}

func encodeRequestMagicSkillUse(skillID int32, ctrl, shift bool) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestMagicSkillUse)
	w.WriteInt32(skillID)
	w.WriteInt32(wire.BoolInt32(ctrl))
	w.WriteUint8(wire.BoolByte(shift))
	return w.Bytes()
}

func encodeRequestExMagicSkillUseGround(x, y, z, skillID int32, ctrl, shift bool) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeExtended)
	w.WriteUint16(clientpackets.OpcodeRequestExMagicSkillUseGround)
	w.WriteInt32(x)
	w.WriteInt32(y)
	w.WriteInt32(z)
	w.WriteInt32(skillID)
	w.WriteInt32(wire.BoolInt32(ctrl))
	w.WriteUint8(wire.BoolByte(shift))
	return w.Bytes()
}

func encodeRequestSkillCoolTime() []byte {
	return wire.NewPacketWriter(clientpackets.OpcodeRequestSkillCoolTime).Bytes()
}

func encodeRequestActionUse(actionID int32, ctrl, shift bool) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestActionUse)
	w.WriteInt32(actionID)
	if ctrl {
		w.WriteInt32(1)
	} else {
		w.WriteInt32(0)
	}
	if shift {
		w.WriteUint8(1)
	} else {
		w.WriteUint8(0)
	}
	return w.Bytes()
}

func encodeMoveBackwardToLocation(target, origin location.Location, moveMovement int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeMoveBackwardToLocation)
	w.WriteInt32(int32(target.X))
	w.WriteInt32(int32(target.Y))
	w.WriteInt32(int32(target.Z))
	w.WriteInt32(int32(origin.X))
	w.WriteInt32(int32(origin.Y))
	w.WriteInt32(int32(origin.Z))
	w.WriteInt32(moveMovement)
	return w.Bytes()
}

func encodeValidatePosition(at location.Location, heading int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeValidatePosition)
	w.WriteInt32(int32(at.X))
	w.WriteInt32(int32(at.Y))
	w.WriteInt32(int32(at.Z))
	w.WriteInt32(heading)
	w.WriteInt32(0)
	return w.Bytes()
}

func encodeCannotMoveAnymore(at location.Location, heading int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeCannotMoveAnymore)
	w.WriteInt32(int32(at.X))
	w.WriteInt32(int32(at.Y))
	w.WriteInt32(int32(at.Z))
	w.WriteInt32(heading)
	return w.Bytes()
}

func encodeStartRotating(degree, side int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeStartRotating)
	w.WriteInt32(degree)
	w.WriteInt32(side)
	return w.Bytes()
}

func encodeFinishRotating(degree, side int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeFinishRotating)
	w.WriteInt32(degree)
	w.WriteInt32(side)
	return w.Bytes()
}

func encodeAction(objectID int32, origin location.Location, shift bool) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeAction)
	w.WriteInt32(objectID)
	w.WriteInt32(int32(origin.X))
	w.WriteInt32(int32(origin.Y))
	w.WriteInt32(int32(origin.Z))
	w.WriteUint8(wire.BoolByte(shift))
	return w.Bytes()
}

func encodeRequestDropItem(objectID, count int32, at location.Location) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestDropItem)
	w.WriteInt32(objectID)
	w.WriteInt32(count)
	w.WriteInt32(int32(at.X))
	w.WriteInt32(int32(at.Y))
	w.WriteInt32(int32(at.Z))
	return w.Bytes()
}

func encodeRequestDestroyItem(objectID, count int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestDestroyItem)
	w.WriteInt32(objectID)
	w.WriteInt32(count)
	return w.Bytes()
}

func encodeRequestCrystallizeItem(objectID, count int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestCrystallizeItem)
	w.WriteInt32(objectID)
	w.WriteInt32(count)
	return w.Bytes()
}

func encodeRequestShortCutReg(typ, slot, id, characterType int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestShortCutReg)
	w.WriteInt32(typ)
	w.WriteInt32(slot)
	w.WriteInt32(id)
	w.WriteInt32(characterType)
	return w.Bytes()
}

func encodeRequestShortCutDel(slot int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestShortCutDel)
	w.WriteInt32(slot)
	return w.Bytes()
}

func encodeSendTimeCheck(requestID, responseID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeSendTimeCheck)
	w.WriteInt32(requestID)
	w.WriteInt32(responseID)
	return w.Bytes()
}

func encodeSingleOpcode(opcode byte) []byte {
	return wire.NewPacketWriter(opcode).Bytes()
}

func encodeDlgAnswer(messageID, answer, requesterID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeDlgAnswer)
	w.WriteInt32(messageID)
	w.WriteInt32(answer)
	w.WriteInt32(requesterID)
	return w.Bytes()
}

// ---- from dispatch_testdata_test.go ----
func testTemplates(t testing.TB) *player.TemplateTable {
	t.Helper()
	tmpl := &player.Template{
		ID:        0,
		BaseLevel: 1,
		HPTable:   []float64{80},
		MPTable:   []float64{30},
		CPTable:   []float64{32},
		Spawns:    []location.Location{{X: 10, Y: 20, Z: 30}},
		RunSpeed:  120,
		WalkSpeed: 60,
		SwimSpeed: 50,
		// Skill 3 is a bought grant every acquire-skill test exercises at
		// level 5. Skill 900001's MinLevel of 50 keeps it out of every other
		// test's reach; only issue #1149's enter-world regression test uses
		// a character old enough to receive this free grant. Its id is
		// outside the low range other tests seed as directly-known skills,
		// so GiveSkills' correctInvalidSkills pull-back never touches them.
		Skills: []player.SkillGrant{
			{SkillID: 3, Level: 1, MinLevel: 5, Cost: 50},
			{SkillID: 900001, Level: 1, MinLevel: 50, Cost: 0},
		},
	}
	table, err := player.NewTemplateTable(map[int]*player.Template{0: tmpl})
	if err != nil {
		t.Fatalf("build template table: %v", err)
	}
	return table
}

func testItemTemplates() *item.Table {
	return item.NewTable([]*item.Template{
		{
			ID:          item.AdenaID,
			Name:        "Adena",
			Kind:        item.KindEtcItem,
			Duration:    -1,
			Stackable:   true,
			Dropable:    true,
			Tradable:    true,
			Destroyable: true,
			Depositable: true,
			EtcItem:     &item.EtcItemDetail{},
		},
		{
			ID:          20,
			Name:        "Potion",
			Kind:        item.KindEtcItem,
			Duration:    -1,
			Stackable:   true,
			Dropable:    true,
			Tradable:    true,
			Destroyable: true,
			Depositable: true,
			EtcItem:     &item.EtcItemDetail{Type: item.EtcItemPotion},
		},
		{
			ID:             1463,
			Name:           "Soulshot: No Grade",
			Kind:           item.KindEtcItem,
			Duration:       -1,
			Stackable:      true,
			Destroyable:    true,
			Crystal:        item.CrystalD,
			DefaultAction:  item.ActionSoulshot,
			EtcItem:        &item.EtcItemDetail{Type: item.EtcItemShot, Handler: "SoulShots"},
			AttachedSkills: []item.SkillRef{{ID: 2150, Level: 1}},
		},
		{
			ID:             2509,
			Name:           "Spiritshot: No Grade",
			Kind:           item.KindEtcItem,
			Duration:       -1,
			Stackable:      true,
			Destroyable:    true,
			Crystal:        item.CrystalD,
			DefaultAction:  item.ActionSpiritshot,
			EtcItem:        &item.EtcItemDetail{Type: item.EtcItemShot, Handler: "SpiritShots"},
			AttachedSkills: []item.SkillRef{{ID: 2047, Level: 1}},
		},
		{
			ID:             1464,
			Name:           "Soulshot: C Grade",
			Kind:           item.KindEtcItem,
			Duration:       -1,
			Stackable:      true,
			Destroyable:    true,
			Crystal:        item.CrystalC,
			DefaultAction:  item.ActionSoulshot,
			EtcItem:        &item.EtcItemDetail{Type: item.EtcItemShot, Handler: "SoulShots"},
			AttachedSkills: []item.SkillRef{{ID: 2151, Level: 1}},
		},
		{
			ID:             6645,
			Name:           "Beast Soulshot",
			Kind:           item.KindEtcItem,
			Duration:       -1,
			Stackable:      true,
			Destroyable:    true,
			EtcItem:        &item.EtcItemDetail{Type: item.EtcItemShot, Handler: "BeastSoulShots"},
			AttachedSkills: []item.SkillRef{{ID: 2033, Level: 1}},
		},
		{
			ID:             6646,
			Name:           "Beast Spiritshot",
			Kind:           item.KindEtcItem,
			Duration:       -1,
			Stackable:      true,
			Destroyable:    true,
			EtcItem:        &item.EtcItemDetail{Type: item.EtcItemShot, Handler: "BeastSpiritShots"},
			AttachedSkills: []item.SkillRef{{ID: 2008, Level: 1}},
		},
		{
			ID:           30,
			Name:         "Sword",
			Kind:         item.KindWeapon,
			Slot:         item.SlotRHand,
			Duration:     -1,
			Crystal:      item.CrystalD,
			CrystalCount: 10,
			Dropable:     true,
			Tradable:     true,
			Destroyable:  true,
			Depositable:  true,
			Weapon:       &item.WeaponDetail{Type: item.WeaponSword, SoulshotCount: 1, SpiritshotCount: 1},
		},
		{
			ID:        item.CrystalD.ItemID(),
			Name:      "D-grade Crystal",
			Kind:      item.KindEtcItem,
			Duration:  -1,
			Stackable: true,
			EtcItem:   &item.EtcItemDetail{},
		},
		{
			ID:        955,
			Name:      "Scroll: Enchant Weapon (D)",
			Kind:      item.KindEtcItem,
			Duration:  -1,
			Stackable: true,
			EtcItem:   &item.EtcItemDetail{Type: item.EtcItemScrollEnchantWeapon, Handler: "EnchantScrolls"},
		},
		{
			ID:             1060,
			Name:           "Lesser Healing Potion",
			Kind:           item.KindEtcItem,
			Duration:       -1,
			Stackable:      true,
			Dropable:       true,
			Tradable:       true,
			Destroyable:    true,
			Depositable:    true,
			EtcItem:        &item.EtcItemDetail{Type: item.EtcItemPotion, Handler: "ItemSkills", ReuseDelay: 10000, SharedReuseGroup: 8},
			AttachedSkills: []item.SkillRef{{ID: 2031, Level: 1}},
			UseConditions: []item.UseCondition{{
				Root:      item.Condition{Kind: "player", Attrs: map[string]string{"flying": "False"}},
				MessageID: int32(serverpackets.SystemMessageS1CannotBeUsed),
				AddName:   true,
			}},
		},
		{
			ID:             728,
			Name:           "Mana Potion",
			Kind:           item.KindEtcItem,
			Duration:       -1,
			Stackable:      true,
			Dropable:       true,
			Tradable:       true,
			Destroyable:    true,
			Depositable:    true,
			EtcItem:        &item.EtcItemDetail{Type: item.EtcItemPotion, Handler: "ItemSkills", ReuseDelay: 2000, SharedReuseGroup: -1},
			AttachedSkills: []item.SkillRef{{ID: 2279, Level: 2}},
		},
		{
			ID:             736,
			Name:           "Scroll: Escape",
			Kind:           item.KindEtcItem,
			Duration:       -1,
			Stackable:      true,
			Dropable:       true,
			Tradable:       true,
			Destroyable:    true,
			Depositable:    true,
			EtcItem:        &item.EtcItemDetail{Type: item.EtcItemScroll, Handler: "ItemSkills", SharedReuseGroup: -1},
			AttachedSkills: []item.SkillRef{{ID: 2013, Level: 1}},
		},
		{
			ID:             737,
			Name:           "Scroll: Escape (Shared Group)",
			Kind:           item.KindEtcItem,
			Duration:       -1,
			Stackable:      true,
			Dropable:       true,
			Tradable:       true,
			Destroyable:    true,
			Depositable:    true,
			EtcItem:        &item.EtcItemDetail{Type: item.EtcItemScroll, Handler: "ItemSkills", SharedReuseGroup: 5, ReuseDelay: 9000},
			AttachedSkills: []item.SkillRef{{ID: 2013, Level: 1}},
		},
		{
			ID:             5589,
			Name:           "Energy Stone",
			Kind:           item.KindEtcItem,
			Duration:       -1,
			Stackable:      true,
			Dropable:       true,
			Tradable:       true,
			Destroyable:    true,
			Depositable:    true,
			EtcItem:        &item.EtcItemDetail{Type: item.EtcItemPotion, Handler: "ItemSkills", SharedReuseGroup: -1},
			AttachedSkills: []item.SkillRef{{ID: 2165, Level: 1}},
		},
		{
			ID:          9001,
			Name:        "Quest Token",
			Kind:        item.KindEtcItem,
			Duration:    -1,
			Destroyable: true,
			EtcItem:     &item.EtcItemDetail{Type: item.EtcItemQuest},
		},
		{
			ID:        9500,
			Name:      "Heavy Ingot",
			Kind:      item.KindEtcItem,
			Duration:  -1,
			Stackable: true,
			EtcItem:   &item.EtcItemDetail{},
			Weight:    10,
		},
		{
			ID:        9502,
			Name:      "Greater Healing Potion",
			Kind:      item.KindEtcItem,
			Duration:  -1,
			Stackable: true,
			EtcItem:   &item.EtcItemDetail{Type: item.EtcItemPotion, SharedReuseGroup: 10},
		},
		{
			ID:          91000,
			Name:        "Wolf Collar",
			Kind:        item.KindEtcItem,
			Duration:    -1,
			Destroyable: true,
			EtcItem:     &item.EtcItemDetail{Handler: SummonItemsHandler},
		},
		{
			ID:          91001,
			Name:        "Wyvern Collar",
			Kind:        item.KindEtcItem,
			Duration:    -1,
			Destroyable: true,
			EtcItem:     &item.EtcItemDetail{Handler: SummonItemsHandler},
		},
	})
}

// ---- from session_bench_test.go ----
type discardConn struct{}

func (discardConn) Read([]byte) (int, error)         { return 0, io.EOF }
func (discardConn) Write(p []byte) (int, error)      { return len(p), nil }
func (discardConn) Close() error                     { return nil }
func (discardConn) LocalAddr() net.Addr              { return discardAddr{} }
func (discardConn) RemoteAddr() net.Addr             { return discardAddr{} }
func (discardConn) SetDeadline(time.Time) error      { return nil }
func (discardConn) SetReadDeadline(time.Time) error  { return nil }
func (discardConn) SetWriteDeadline(time.Time) error { return nil }

type discardAddr struct{}

func (discardAddr) Network() string { return "discard" }
func (discardAddr) String() string  { return "discard" }

func benchmarkSession(b *testing.B) *Session {
	b.Helper()
	cipher, err := gamecipher.NewCipher(make([]byte, gamecipher.KeySize))
	if err != nil {
		b.Fatalf("NewCipher: %v", err)
	}
	conn := newConn(discardConn{}, zerolog.Nop())
	b.Cleanup(func() { conn.Close() })
	return NewSession(conn, cipher)
}

func BenchmarkSessionSendAuthLoginFailFrame(b *testing.B) {
	session := benchmarkSession(b)
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		frame := serverpackets.FrameAuthLoginFail(serverpackets.LoginFailSystemErrorTryLater)
		if !session.SendFrame(frame) {
			b.Fatal("SendFrame returned false")
		}
	}
}

func benchmarkUserInfoSnapshot() serverpackets.UserInfoSnapshot {
	return serverpackets.UserInfoSnapshot{
		Character: &player.Character{Name: "Benchmark"},
		Template:  &player.Template{},
	}
}

func benchmarkUserInfoPayload(s serverpackets.UserInfoSnapshot) []byte {
	return serverpackets.EncodeUserInfo(s)
}

func TestBenchmarkUserInfoPayloadMatchesFrame(t *testing.T) {
	snapshot := benchmarkUserInfoSnapshot()
	frame := serverpackets.FrameUserInfo(snapshot)
	defer frame.Release()

	if got, want := benchmarkUserInfoPayload(snapshot), frame.Bytes()[frameHeaderSize:]; !bytes.Equal(got, want) {
		t.Fatal("unpooled UserInfo payload differs from FrameUserInfo")
	}
}

func BenchmarkSessionSendUserInfoFrame(b *testing.B) {
	snapshot := benchmarkUserInfoSnapshot()
	session := benchmarkSession(b)
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		if !session.SendFrame(serverpackets.FrameUserInfo(snapshot)) {
			b.Fatal("SendFrame returned false")
		}
	}
}

// ---- from skill_domain_fixtures_test.go ----
func skillTable(defs ...modelskill.Definition) *modelskill.Table {
	return modelskill.NewTable(defs)
}

type memorySkillSaveStore struct {
	mu      sync.Mutex
	rows    map[skillSaveKey][]effect.SaveRow
	known   map[skillSaveKey]player.SkillLevels
	deleted int
}

type skillSaveKey struct {
	charObjID  int32
	classIndex int32
}

func newMemorySkillSaveStore() *memorySkillSaveStore {
	return &memorySkillSaveStore{rows: make(map[skillSaveKey][]effect.SaveRow), known: make(map[skillSaveKey]player.SkillLevels)}
}

func (s *memorySkillSaveStore) Replace(_ context.Context, charObjID int32, classIndex int32, rows []effect.SaveRow) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rows[skillSaveKey{charObjID: charObjID, classIndex: classIndex}] = append([]effect.SaveRow(nil), rows...)
	return nil
}

func (s *memorySkillSaveStore) ListByCharacter(_ context.Context, charObjID int32, classIndex int32) ([]effect.SaveRow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rowsForLocked(charObjID, classIndex), nil
}

func (s *memorySkillSaveStore) DeleteByCharacter(_ context.Context, charObjID int32, classIndex int32) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := skillSaveKey{charObjID: charObjID, classIndex: classIndex}
	n := int64(len(s.rows[key]))
	delete(s.rows, key)
	s.deleted++
	return n, nil
}

func (s *memorySkillSaveStore) seed(charObjID int32, classIndex int32, rows []effect.SaveRow) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rows[skillSaveKey{charObjID: charObjID, classIndex: classIndex}] = append([]effect.SaveRow(nil), rows...)
}

func (s *memorySkillSaveStore) rowsFor(charObjID int32, classIndex int32) []effect.SaveRow {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rowsForLocked(charObjID, classIndex)
}

func (s *memorySkillSaveStore) rowsForLocked(charObjID int32, classIndex int32) []effect.SaveRow {
	return append([]effect.SaveRow(nil), s.rows[skillSaveKey{charObjID: charObjID, classIndex: classIndex}]...)
}

func (s *memorySkillSaveStore) ListKnownSkills(_ context.Context, charObjID int32, classIndex int32) (player.SkillLevels, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	levels := s.known[skillSaveKey{charObjID: charObjID, classIndex: classIndex}]
	out := make(player.SkillLevels, len(levels))
	for id, level := range levels {
		out[id] = level
	}
	return out, nil
}

func (s *memorySkillSaveStore) SetKnownSkill(_ context.Context, charObjID int32, classIndex int32, skillID int, level int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := skillSaveKey{charObjID: charObjID, classIndex: classIndex}
	if s.known[key] == nil {
		s.known[key] = make(player.SkillLevels)
	}
	s.known[key][skillID] = level
	return nil
}

func (s *memorySkillSaveStore) knownFor(charObjID int32, classIndex int32) player.SkillLevels {
	s.mu.Lock()
	defer s.mu.Unlock()
	levels := s.known[skillSaveKey{charObjID: charObjID, classIndex: classIndex}]
	out := make(player.SkillLevels, len(levels))
	for id, level := range levels {
		out[id] = level
	}
	return out
}

func (s *memorySkillSaveStore) seedKnown(charObjID int32, classIndex int32, levels player.SkillLevels) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make(player.SkillLevels, len(levels))
	for id, level := range levels {
		cp[id] = level
	}
	s.known[skillSaveKey{charObjID: charObjID, classIndex: classIndex}] = cp
}
func wireLiveAttackHooks(gcl *GameClientLink, live *livePlayer) {
	live.stopAttack = gcl.stopLiveAutoAttack
	live.attack.SetFinished(func() {
		gcl.finishDeferredPickup(live)
		gcl.finishDeferredMagicSkill(live)
		gcl.finishDeferredItemAICast(live)
		live.combat.Think()
	})
	live.attack.SetStarted(func() {
		gcl.startLiveAutoAttack(live)
	})
	live.Character.SetAttackBroadcaster(func(snapshot attack.Snapshot) {
		gcl.broadcastAttack(live, snapshot)
	})
	live.Character.SetMoveBroadcaster(func(event move.Event) {
		gcl.broadcastLiveMoveEvent(live, event)
	})
	live.Character.SetStatusBroadcaster(func() {
		gcl.broadcastLiveStatus(live)
	})
	live.move.SetArrived(func() {
		pos := live.move.Position()
		gcl.updateLivePlayerPosition(live, pos, live.CurrentHeading())
		live.combat.Think()
	})
}

// TestAttackLiveTargetRejectsOutOfControl pins AttackRequest.java:31's
// isOutOfControl() reject (Creature.java:652-655): a teleporting,
// immobile-until-attacked, stunned, sleeping, paralyzed, afraid, confused, or
// levelRefreshTable is a three-level table, so RealMaxLevel is 2 and a single
// level-up from 1 is legal.
func levelRefreshTable(t *testing.T) *player.LevelTable {
	t.Helper()
	table, err := player.NewLevelTable(map[int]player.Level{
		1: {RequiredExpToLevelUp: 0},
		2: {RequiredExpToLevelUp: 68},
		3: {RequiredExpToLevelUp: 363},
	})
	if err != nil {
		t.Fatalf("build level table: %v", err)
	}
	return table
}

// TestRefreshLiveLevelSkillsReconcilesAndSendsSkillList pins what the level

// ---- from decode_client_packet_test.go ----
// TestDecodeClientPacketClassifiesShortPacketVsValidationErrors proves
// decodeClientPacket only routes wire.ErrShortPacket-class decode errors
// (buffer-underflow-equivalent) toward disconnect, matching
// L2GameClientPacket.Read(): other decode validation errors are logged and
// the packet dropped without ever counting toward the underflow threshold
// or disconnecting, pre-auth or post-auth. Exercised directly against
// decodeClientPacket rather than through a specific wired opcode, since no
// currently-wired decoder both returns a non-underflow validation error and
// reaches decodeClientPacket without being pre-filtered by its caller's own
// switch (e.g. an extended sub-opcode mismatch never reaches the decoder
// the outer dispatch already selected by that same sub-opcode).
func TestDecodeClientPacketClassifiesShortPacketVsValidationErrors(t *testing.T) {
	shortPacket := func([]byte) (int, error) {
		return 0, fmt.Errorf("short: %w", wire.ErrShortPacket)
	}
	validation := func([]byte) (int, error) {
		return 0, errors.New("clientpackets: invalid value")
	}

	t.Run("short packet disconnects immediately pre-auth", func(t *testing.T) {
		l := &GameClientLink{}
		client := NewClient(nil)

		if _, err := decodeClientPacket(l, client, nil, shortPacket); !errors.Is(err, errMalformedPacketDisconnect) {
			t.Fatalf("decodeClientPacket() error = %v, want errMalformedPacketDisconnect", err)
		}
	})

	t.Run("validation error never disconnects pre-auth", func(t *testing.T) {
		l := &GameClientLink{}
		client := NewClient(nil)

		if _, err := decodeClientPacket(l, client, nil, validation); errors.Is(err, errMalformedPacketDisconnect) {
			t.Fatalf("decodeClientPacket() error = %v, want no disconnect", err)
		}
	})

	t.Run("short packet tolerates first, disconnects past threshold post-auth", func(t *testing.T) {
		l := &GameClientLink{}
		client := NewClient(nil)
		client.SetAuthenticated("acc", link.SessionKey{})

		for i := range maxUnderflowsPerMin {
			if _, err := decodeClientPacket(l, client, nil, shortPacket); errors.Is(err, errMalformedPacketDisconnect) {
				t.Fatalf("decodeClientPacket() call %d disconnected before threshold", i+1)
			}
		}
		if _, err := decodeClientPacket(l, client, nil, shortPacket); !errors.Is(err, errMalformedPacketDisconnect) {
			t.Fatalf("decodeClientPacket() past threshold error = %v, want errMalformedPacketDisconnect", err)
		}
	})

	t.Run("validation error never disconnects post-auth even past threshold", func(t *testing.T) {
		l := &GameClientLink{}
		client := NewClient(nil)
		client.SetAuthenticated("acc", link.SessionKey{})

		for i := range maxUnderflowsPerMin + 2 {
			if _, err := decodeClientPacket(l, client, nil, validation); errors.Is(err, errMalformedPacketDisconnect) {
				t.Fatalf("decodeClientPacket() call %d disconnected on validation error", i+1)
			}
		}
	})
}
