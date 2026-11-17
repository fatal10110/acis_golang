package clientpackets

import (
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/testsupport/decodefuzz"
)

// fuzzMaxItems is the shipped item-list cap: the larger of the default
// players.properties base inventory sizes (80 non-dwarf, 100 dwarf).
const fuzzMaxItems = 100

// Seed field values: a Talking Island position, an item object id from the
// object-id range, and the Interlude protocol revision.
const (
	seedObjectID int32 = 0x10000001
	seedX        int32 = -84318
	seedY        int32 = 244579
	seedZ        int32 = -3730
	seedRevision int32 = 746
)

type gameDecoder struct {
	name   string
	decode func([]byte) error
	// seed is a well-formed packet for this decoder, opcode byte first,
	// with fields in the order the client writes them.
	seed []byte
}

// refusedSeeds names the decoders whose seed is a packet they refuse by
// design, with why. RequestBuyProcure checks eight bytes per row where each
// row reads twelve, so no non-empty list decodes.
var refusedSeeds = map[string]string{"RequestBuyProcure": "row check of 8 bytes against 12 read"}

func decodes[T any](decode func([]byte) (T, error)) func([]byte) error {
	return func(payload []byte) error {
		_, err := decode(payload)
		return err
	}
}

func decodesList[T any](decode func([]byte, int) (T, error)) func([]byte) error {
	return func(payload []byte) error {
		_, err := decode(payload, fuzzMaxItems)
		return err
	}
}

// seedPacket writes opcode and fields the way the client lays them out:
// int32 as a 4-byte word, uint16 as a 2-byte word, byte as one byte, and
// string as null-terminated UTF-16LE.
func seedPacket(opcode byte, fields ...any) []byte {
	w := wire.NewPacketWriter(opcode)
	for _, field := range fields {
		switch v := field.(type) {
		case int32:
			w.WriteInt32(v)
		case uint16:
			w.WriteUint16(v)
		case byte:
			w.WriteUint8(v)
		case string:
			w.WriteString(v)
		default:
			panic("seedPacket: unsupported field type")
		}
	}
	return w.Bytes()
}

func seedExtended(sub uint16, fields ...any) []byte {
	return seedPacket(OpcodeExtended, append([]any{sub}, fields...)...)
}

// gameDecoders lists every game client packet decoder the game listener
// dispatches to.
var gameDecoders = []gameDecoder{
	{"ProtocolVersion", decodes(DecodeProtocolVersion), seedPacket(OpcodeProtocolVersion, seedRevision)},
	{"MoveBackwardToLocation", decodes(DecodeMoveBackwardToLocation), seedPacket(OpcodeMoveBackwardToLocation, seedX+100, seedY+100, seedZ, seedX, seedY, seedZ, int32(1))},
	{"Appearing", decodes(DecodeAppearing), seedPacket(OpcodeAppearing)},
	{"ObserverReturn", decodes(DecodeObserverReturn), seedPacket(OpcodeObserverReturn)},
	{"Action", decodes(DecodeAction), seedPacket(OpcodeAction, seedObjectID, seedX, seedY, seedZ, byte(0))},
	{"AuthLogin", decodes(DecodeAuthLogin), seedPacket(OpcodeAuthLogin, "TestAccount", int32(0x2a2b2c2d), int32(0x1a1b1c1d), int32(0x0a0b0c0d), int32(0x3a3b3c3d))},
	{"AttackRequest", decodes(DecodeAttackRequest), seedPacket(OpcodeAttackRequest, seedObjectID, seedX, seedY, seedZ, byte(1))},
	{"RequestCharacterCreate", decodes(DecodeRequestCharacterCreate), seedPacket(OpcodeRequestCharacterCreate, "Newbie", int32(0), int32(0), int32(0), int32(40), int32(22), int32(22), int32(21), int32(21), int32(21), int32(1), int32(2), int32(0))},
	{"RequestCharacterDelete", decodes(DecodeRequestCharacterDelete), seedPacket(OpcodeRequestCharacterDelete, int32(0))},
	{"RequestGameStart", decodes(DecodeRequestGameStart), seedPacket(OpcodeRequestGameStart, int32(0), uint16(0), int32(0), int32(0), int32(0))},
	{"UnequipItem", decodes(DecodeUnequipItem), seedPacket(OpcodeRequestUnEquipItem, int32(0x4000))},
	{"RequestDropItem", decodes(DecodeRequestDropItem), seedPacket(OpcodeRequestDropItem, seedObjectID, int32(10), seedX, seedY, seedZ)},
	{"UseItem", decodes(DecodeUseItem), seedPacket(OpcodeUseItem, seedObjectID, int32(0))},
	{"TradeRequest", decodes(DecodeTradeRequest), seedPacket(OpcodeTradeRequest, seedObjectID)},
	{"AddTradeItem", decodes(DecodeAddTradeItem), seedPacket(OpcodeAddTradeItem, int32(1), seedObjectID, int32(5))},
	{"TradeDone", decodes(DecodeTradeDone), seedPacket(OpcodeTradeDone, int32(1))},
	{"RequestSocialAction", decodes(DecodeRequestSocialAction), seedPacket(OpcodeRequestSocialAction, int32(2))},
	{"RequestChangeMoveType", decodes(DecodeRequestChangeMoveType), seedPacket(OpcodeRequestChangeMoveType, int32(1))},
	{"RequestChangeWaitType", decodes(DecodeRequestChangeWaitType), seedPacket(OpcodeRequestChangeWaitType, int32(0))},
	{"RequestSellItem", decodesList(DecodeRequestSellItem), seedPacket(OpcodeRequestSellItem, int32(1), int32(2), seedObjectID, int32(57), int32(10), seedObjectID+1, int32(1060), int32(1))},
	{"RequestBuyItem", decodesList(DecodeRequestBuyItem), seedPacket(OpcodeRequestBuyItem, int32(1), int32(2), int32(1060), int32(5), int32(1835), int32(100))},
	{"RequestLinkHtml", decodes(DecodeRequestLinkHTML), seedPacket(OpcodeRequestLinkHtml, "default/30001-1.htm")},
	{"RequestBypassToServer", decodes(DecodeRequestBypassToServer), seedPacket(OpcodeRequestBypassToServer, "npc_268435457_Shop 1")},
	{"RequestGMCommand", decodes(DecodeRequestGMCommand), seedPacket(OpcodeRequestGMCommand, "Player", int32(6))},
	{"RequestMagicSkillUse", decodes(DecodeRequestMagicSkillUse), seedPacket(OpcodeRequestMagicSkillUse, int32(1177), int32(0), byte(0))},
	{"SendWarehouseDepositList", decodesList(DecodeSendWarehouseDepositList), seedPacket(OpcodeSendWarehouseDeposit, int32(1), seedObjectID, int32(100))},
	{"SendWarehouseWithdrawList", decodesList(DecodeSendWarehouseWithdrawList), seedPacket(OpcodeSendWarehouseWithdraw, int32(2), seedObjectID, int32(1), seedObjectID+1, int32(3))},
	{"RequestShortCutReg", decodes(DecodeRequestShortCutReg), seedPacket(OpcodeRequestShortCutReg, int32(2), int32(13), int32(1177), int32(1))},
	{"RequestShortCutDel", decodes(DecodeRequestShortCutDel), seedPacket(OpcodeRequestShortCutDel, int32(13))},
	{"RequestMakeMacro", decodes(DecodeRequestMakeMacro), seedPacket(OpcodeRequestMakeMacro, int32(0), "Heal", "self heal", "H", byte(1), byte(1), byte(1), byte(1), int32(1011), byte(0), "")},
	{"RequestDeleteMacro", decodes(DecodeRequestDeleteMacro), seedPacket(OpcodeRequestDeleteMacro, int32(1000))},
	{"RequestEvaluate", decodes(DecodeRequestEvaluate), seedPacket(OpcodeRequestEvaluate, seedObjectID)},
	{"CannotMoveAnymore", decodes(DecodeCannotMoveAnymore), seedPacket(OpcodeCannotMoveInVehicle, seedX, seedY, seedZ, int32(32768))},
	{"RequestTargetCancel", decodes(DecodeRequestTargetCancel), seedPacket(OpcodeRequestTargetCancel, uint16(0))},
	{"AnswerTradeRequest", decodes(DecodeAnswerTradeRequest), seedPacket(OpcodeAnswerTradeRequest, int32(1))},
	{"RequestActionUse", decodes(DecodeRequestActionUse), seedPacket(OpcodeRequestActionUse, int32(0), int32(0), byte(0))},
	{"ValidatePosition", decodes(DecodeValidatePosition), seedPacket(OpcodeValidatePosition, seedX, seedY, seedZ, int32(32768), int32(0))},
	{"StartRotating", decodes(DecodeStartRotating), seedPacket(OpcodeStartRotating, int32(16384), int32(1))},
	{"FinishRotating", decodes(DecodeFinishRotating), seedPacket(OpcodeFinishRotating, int32(16384), int32(0))},
	{"RequestEnchantItem", decodes(DecodeRequestEnchantItem), seedPacket(OpcodeRequestEnchantItem, seedObjectID)},
	{"RequestDestroyItem", decodes(DecodeRequestDestroyItem), seedPacket(OpcodeRequestDestroyItem, seedObjectID, int32(1))},
	{"CharacterRestore", decodes(DecodeCharacterRestore), seedPacket(OpcodeCharacterRestore, int32(0))},
	{"RequestPledgeCrest", decodes(DecodeRequestPledgeCrest), seedPacket(OpcodeRequestPledgeCrest, int32(1))},
	{"RequestGiveNickName", decodes(DecodeRequestGiveNickName), seedPacket(OpcodeRequestGiveNickName, "Member", "Knight")},
	{"RequestAcquireSkillInfo", decodes(DecodeRequestAcquireSkillInfo), seedPacket(OpcodeRequestAcquireSkillInfo, int32(1177), int32(1), int32(0))},
	{"RequestAcquireSkill", decodes(DecodeRequestAcquireSkill), seedPacket(OpcodeRequestAcquireSkill, int32(1177), int32(1), int32(0))},
	{"RequestRestartPoint", decodes(DecodeRequestRestartPoint), seedPacket(OpcodeRequestRestartPoint, int32(0))},
	{"RequestCrystallizeItem", decodes(DecodeRequestCrystallizeItem), seedPacket(OpcodeRequestCrystallizeItem, seedObjectID, int32(1))},
	{"RequestRecipeBookOpen", decodes(DecodeRequestRecipeBookOpen), seedPacket(OpcodeRequestRecipeBookOpen, int32(1))},
	{"RequestRecipeBookDestroy", decodes(DecodeRequestRecipeBookDestroy), seedPacket(OpcodeRequestRecipeBookDestroy, int32(686))},
	{"RequestRecipeItemMakeInfo", decodes(DecodeRequestRecipeItemMakeInfo), seedPacket(OpcodeRequestRecipeItemMakeInfo, int32(686))},
	{"RequestRecipeItemMakeSelf", decodes(DecodeRequestRecipeItemMakeSelf), seedPacket(OpcodeRequestRecipeItemMakeSelf, int32(686))},
	{"RequestHennaItemList", decodes(DecodeRequestHennaItemList), seedPacket(OpcodeRequestHennaItemList, int32(0))},
	{"RequestHennaItemInfo", decodes(DecodeRequestHennaItemInfo), seedPacket(OpcodeRequestHennaItemInfo, int32(1))},
	{"RequestHennaEquip", decodes(DecodeRequestHennaEquip), seedPacket(OpcodeRequestHennaEquip, int32(1))},
	{"RequestHennaUnequipList", decodes(DecodeRequestHennaUnequipList), seedPacket(OpcodeRequestHennaUnequipList, int32(0))},
	{"RequestHennaUnequipInfo", decodes(DecodeRequestHennaUnequipInfo), seedPacket(OpcodeRequestHennaUnequipInfo, int32(1))},
	{"RequestHennaUnequip", decodes(DecodeRequestHennaUnequip), seedPacket(OpcodeRequestHennaUnequip, int32(1))},
	{"SetPrivateStoreListSell", decodesList(DecodeSetPrivateStoreListSell), seedPacket(OpcodeSetPrivateStoreListSell, int32(0), int32(1), seedObjectID, int32(3), int32(100))},
	{"SetPrivateStoreListBuy", decodesList(DecodeSetPrivateStoreListBuy), seedPacket(OpcodeSetPrivateStoreListBuy, int32(1), int32(1060), uint16(0), uint16(0), int32(20), int32(35))},
	{"RequestPrivateStoreBuy", decodesList(DecodeRequestPrivateStoreBuy), seedPacket(OpcodeRequestPrivateStoreBuy, seedObjectID, int32(1), seedObjectID+1, int32(2), int32(40))},
	{"RequestPrivateStoreSell", decodesList(DecodeRequestPrivateStoreSell), seedPacket(OpcodeRequestPrivateStoreSell, seedObjectID, int32(1), seedObjectID+1, int32(1060), uint16(0), uint16(0), int32(2), int32(35))},
	{"SetPrivateStoreMsgSell", decodes(DecodeStoreMessage), seedPacket(OpcodeSetPrivateStoreMsgSell, "cheap potions")},
	{"RequestRecipeShopListSet", decodesList(DecodeRequestRecipeShopListSet), seedPacket(OpcodeRequestRecipeShopListSet, int32(1), int32(686), int32(300))},
	{"RequestRecipeShopMakeInfo", decodes(DecodeRequestRecipeShopMakeInfo), seedPacket(OpcodeRequestRecipeShopMakeInfo, seedObjectID, int32(686))},
	{"RequestRecipeShopMakeItem", decodes(DecodeRequestRecipeShopMakeItem), seedPacket(OpcodeRequestRecipeShopMakeItem, seedObjectID, int32(686), int32(0))},
	{"MultiSellChoose", decodes(DecodeMultiSellChoose), seedPacket(OpcodeMultiSellChoose, int32(1002), int32(1), int32(1))},
	{"RequestSiegeAttackerList", decodes(DecodeRequestSiegeList), seedPacket(OpcodeRequestSiegeAttackerList, int32(1))},
	{"RequestSiegeDefenderList", decodes(DecodeRequestSiegeList), seedPacket(OpcodeRequestSiegeDefenderList, int32(1))},
	{"RequestJoinSiege", decodes(DecodeRequestJoinSiege), seedPacket(OpcodeRequestJoinSiege, int32(1), int32(1), int32(1))},
	{"RequestConfirmSiegeWaitingList", decodes(DecodeRequestConfirmSiegeWaitingList), seedPacket(OpcodeRequestConfirmSiegeWaitingList, int32(1), seedObjectID, int32(1))},
	{"RequestAllyCrest", decodes(DecodeRequestAllyCrest), seedPacket(OpcodeRequestAllyCrest, int32(1))},
	{"RequestChangePetName", decodes(DecodeRequestChangePetName), seedPacket(OpcodeRequestChangePetName, "Kookaburra")},
	{"RequestFriendInvite", decodes(DecodeRequestFriendInvite), seedPacket(OpcodeRequestFriendInvite, "Kookaburra")},
	{"RequestAnswerFriendInvite", decodes(DecodeRequestAnswerFriendInvite), seedPacket(OpcodeRequestAnswerFriendInvite, int32(1))},
	{"RequestFriendDel", decodes(DecodeRequestFriendDel), seedPacket(OpcodeRequestFriendDel, "Kookaburra")},
	{"RequestBlock", decodes(DecodeRequestBlock), seedPacket(OpcodeRequestBlock, int32(0), "Kookaburra")},
	{"RequestSendL2FriendSay", decodes(DecodeRequestSendL2FriendSay), seedPacket(OpcodeRequestSendL2FriendSay, "hello", "Kookaburra")},
	{"Say2", decodes(DecodeSay2), seedPacket(OpcodeSay2, "hello", int32(2), "Kookaburra")},
	{"RequestShowBoard", decodes(DecodeRequestShowBoard), seedPacket(OpcodeRequestShowBoard, int32(0))},
	{"RequestBBSwrite", decodes(DecodeRequestBBSWrite), seedPacket(OpcodeRequestBBSWrite, "Mail", "Send", "0", "Kookaburra", "Hello", "How are you?")},
	{"RequestPetUseItem", decodes(DecodeRequestPetUseItem), seedPacket(OpcodeRequestPetUseItem, seedObjectID)},
	{"RequestGiveItemToPet", decodes(DecodeRequestGiveItemToPet), seedPacket(OpcodeRequestGiveItemToPet, seedObjectID, int32(1))},
	{"RequestGetItemFromPet", decodes(DecodeRequestGetItemFromPet), seedPacket(OpcodeRequestGetItemFromPet, seedObjectID, int32(1), int32(0))},
	{"RequestPetGetItem", decodes(DecodeRequestPetGetItem), seedPacket(OpcodeRequestPetGetItem, seedObjectID)},
	{"SendTimeCheck", decodes(DecodeSendTimeCheck), seedPacket(OpcodeSendTimeCheck, int32(1), int32(1))},
	{"RequestPackageSendableItemList", decodes(DecodeRequestPackageSendableItemList), seedPacket(OpcodeRequestPackageItemList, seedObjectID)},
	{"RequestPackageSend", decodesList(DecodeRequestPackageSend), seedPacket(OpcodeRequestPackageSend, seedObjectID, int32(1), seedObjectID+1, int32(1))},
	{"DlgAnswer", decodes(DecodeDlgAnswer), seedPacket(OpcodeDlgAnswer, int32(1510), int32(1), int32(0))},
	{"RequestAutoSoulShot", decodes(DecodeRequestAutoSoulShot), seedExtended(OpcodeRequestAutoSoulShot, int32(1835), int32(1))},
	{"RequestExEnchantSkillInfo", decodes(DecodeRequestExEnchantSkillInfo), seedExtended(OpcodeRequestExEnchantSkillInfo, int32(1177), int32(101))},
	{"RequestExEnchantSkill", decodes(DecodeRequestExEnchantSkill), seedExtended(OpcodeRequestExEnchantSkill, int32(1177), int32(101))},
	{"RequestExPledgeCrestLarge", decodes(DecodeRequestExPledgeCrestLarge), seedExtended(OpcodeRequestExPledgeCrestLarge, int32(1))},
	{"RequestConfirmTargetItem", decodes(DecodeRequestConfirmTargetItem), seedExtended(OpcodeRequestConfirmTargetItem, seedObjectID)},
	{"RequestConfirmRefinerItem", decodes(DecodeRequestConfirmRefinerItem), seedExtended(OpcodeRequestConfirmRefinerItem, seedObjectID, seedObjectID+1)},
	{"RequestConfirmGemStone", decodes(DecodeRequestConfirmGemStone), seedExtended(OpcodeRequestConfirmGemStone, seedObjectID, seedObjectID+1, seedObjectID+2, int32(20))},
	{"RequestRefine", decodes(DecodeRequestRefine), seedExtended(OpcodeRequestRefine, seedObjectID, seedObjectID+1, seedObjectID+2, int32(20))},
	{"RequestConfirmCancelItem", decodes(DecodeRequestConfirmCancelItem), seedExtended(OpcodeRequestConfirmCancelItem, seedObjectID)},
	{"RequestRefineCancel", decodes(DecodeRequestRefineCancel), seedExtended(OpcodeRequestRefineCancel, seedObjectID)},
	{"RequestSetSeed", decodesList(DecodeRequestSetSeed), seedExtended(OpcodeRequestSetSeed, int32(1), int32(1), int32(5016), int32(10), int32(5))},
	{"RequestSetCrop", decodesList(DecodeRequestSetCrop), seedExtended(OpcodeRequestSetCrop, int32(1), int32(1), int32(5073), int32(100), int32(50), byte(1))},
	{"RequestBuySeed", decodesList(DecodeRequestBuySeed), seedPacket(OpcodeRequestBuySeed, int32(1), int32(1), int32(5016), int32(1))},
	{"RequestProcureCropList", decodesList(DecodeRequestProcureCropList), seedExtended(OpcodeRequestProcureCropList, int32(1), seedObjectID, int32(5073), int32(1), int32(1))},
	{"RequestExMagicSkillUseGround", decodes(DecodeRequestExMagicSkillUseGround), seedExtended(OpcodeRequestExMagicSkillUseGround, seedX, seedY, seedZ, int32(1177), int32(0), byte(0))},
	{"RequestWriteHeroWords", decodes(DecodeRequestWriteHeroWords), seedExtended(OpcodeRequestWriteHeroWords, "Glory to the heroes")},
	{"RequestDuelStart", decodes(DecodeRequestDuelStart), seedExtended(OpcodeRequestDuelStart, "Rival", int32(1))},
	{"RequestDuelAnswerStart", decodes(DecodeRequestDuelAnswerStart), seedExtended(OpcodeRequestDuelAnswerStart, int32(0), int32(0), int32(1))},
	{"AllyDismiss", decodes(DecodeAllyDismiss), seedPacket(OpcodeAllyDismiss, "Allies")},
	{"AnswerJoinPartyRoom", decodes(DecodeAnswerJoinPartyRoom), seedExtended(OpcodeAnswerJoinPartyRoom, int32(1))},
	{"CannotMoveAnymoreInVehicle", decodes(DecodeCannotMoveAnymoreInVehicle), seedPacket(OpcodeCannotMoveInVehicle, int32(1), seedX, seedY, seedZ, int32(100))},
	{"PetitionVote", decodes(DecodePetitionVote), seedPacket(OpcodePetitionVote, int32(0), int32(5), "Well helped")},
	{"RequestAnswerJoinAlly", decodes(DecodeRequestAnswerJoinAlly), seedPacket(OpcodeRequestAnswerJoinAlly, int32(1))},
	{"RequestAnswerJoinParty", decodes(DecodeRequestAnswerJoinParty), seedPacket(OpcodeRequestAnswerJoinParty, int32(1))},
	{"RequestAnswerJoinPledge", decodes(DecodeRequestAnswerJoinPledge), seedPacket(OpcodeRequestAnswerJoinPledge, int32(1))},
	{"RequestAskJoinPartyRoom", decodes(DecodeRequestAskJoinPartyRoom), seedExtended(OpcodeRequestAskJoinPartyRoom, "Friend")},
	{"RequestBuyProcure", decodesList(DecodeRequestBuyProcure), seedPacket(OpcodeRequestBuyProcure, int32(1), int32(1), int32(0), int32(5073))},
	{"RequestChangePartyLeader", decodes(DecodeRequestChangePartyLeader), seedExtended(OpcodeRequestChangePartyLeader, "Friend")},
	{"RequestDismissPartyRoom", decodes(DecodeRequestDismissPartyRoom), seedExtended(OpcodeRequestDismissPartyRoom, int32(1), int32(0))},
	{"RequestExAcceptJoinMPCC", decodes(DecodeRequestExAcceptJoinMPCC), seedExtended(OpcodeRequestExAcceptJoinMPCC, int32(1))},
	{"RequestExAskJoinMPCC", decodes(DecodeRequestExAskJoinMPCC), seedExtended(OpcodeRequestExAskJoinMPCC, "Friend")},
	{"RequestExMPCCShowPartyMembersInfo", decodes(DecodeRequestExMPCCShowPartyMembersInfo), seedExtended(OpcodeRequestExMPCCShowPartyMembersInfo, seedObjectID)},
	{"RequestExOustFromMPCC", decodes(DecodeRequestExOustFromMPCC), seedExtended(OpcodeRequestExOustFromMPCC, "Friend")},
	{"RequestExSetPledgeCrestLarge", decodes(DecodeRequestExSetPledgeCrestLarge), append(seedExtended(OpcodeRequestExSetPledgeCrestLarge, int32(4)), 1, 2, 3, 4)},
	{"RequestGetBossRecord", decodes(DecodeRequestGetBossRecord), seedExtended(OpcodeRequestGetBossRecord, int32(1))},
	{"RequestGetOffVehicle", decodes(DecodeRequestGetOffVehicle), seedPacket(OpcodeRequestGetOffVehicle, int32(1), seedX, seedY, seedZ)},
	{"RequestGetOnVehicle", decodes(DecodeRequestGetOnVehicle), seedPacket(OpcodeRequestGetOnVehicle, int32(1), seedX, seedY, seedZ)},
	{"RequestJoinAlly", decodes(DecodeRequestJoinAlly), seedPacket(OpcodeRequestJoinAlly, seedObjectID)},
	{"RequestJoinParty", decodes(DecodeRequestJoinParty), seedPacket(OpcodeRequestJoinParty, "Friend", int32(0))},
	{"RequestJoinPartyRoom", decodes(DecodeRequestJoinPartyRoom), seedPacket(OpcodeRequestJoinPartyRoom, int32(1), int32(-1), int32(0))},
	{"RequestJoinPledge", decodes(DecodeRequestJoinPledge), seedPacket(OpcodeRequestJoinPledge, seedObjectID, int32(0))},
	{"RequestListPartyMatchingWaitingRoom", decodes(DecodeRequestListPartyMatchingWaitingRoom), seedExtended(OpcodeRequestListPartyMatchingWaitingRoom, int32(1), int32(1), int32(80), int32(1))},
	{"RequestListPartyWaiting", decodes(DecodeRequestListPartyWaiting), seedPacket(OpcodeRequestListPartyWaiting, int32(1), int32(-1), int32(0))},
	{"RequestManagePartyRoom", decodes(DecodeRequestManagePartyRoom), seedPacket(OpcodeRequestManagePartyRoom, int32(1), int32(9), int32(1), int32(80), int32(0), "Hunting")},
	{"RequestMoveToLocationInVehicle", decodes(DecodeRequestMoveToLocationInVehicle), seedPacket(OpcodeRequestMoveInVehicle, int32(1), seedX, seedY, seedZ, seedX+10, seedY+10, seedZ)},
	{"RequestOustFromPartyRoom", decodes(DecodeRequestOustFromPartyRoom), seedExtended(OpcodeRequestOustFromPartyRoom, seedObjectID)},
	{"RequestOustPartyMember", decodes(DecodeRequestOustPartyMember), seedPacket(OpcodeRequestOustPartyMember, "Friend")},
	{"RequestOustPledgeMember", decodes(DecodeRequestOustPledgeMember), seedPacket(OpcodeRequestOustPledgeMember, "Friend")},
	{"RequestPCCafeCouponUse", decodes(DecodeRequestPCCafeCouponUse), seedExtended(OpcodeRequestPCCafeCouponUse, "COUPON")},
	{"RequestPetition", decodes(DecodeRequestPetition), seedPacket(OpcodeRequestPetition, "I am stuck", int32(0))},
	{"RequestPledgeInfo", decodes(DecodeRequestPledgeInfo), seedPacket(OpcodeRequestPledgeInfo, seedObjectID)},
	{"RequestPledgeMemberPower", func(payload []byte) error {
		_, err := DecodeRequestPledgeMemberName(payload, OpcodeRequestPledgeMemberPower)
		return err
	}, seedExtended(OpcodeRequestPledgeMemberPower, int32(0), "Friend")},
	{"RequestPledgeMemberDetail", func(payload []byte) error {
		_, err := DecodeRequestPledgeMemberName(payload, OpcodeRequestPledgeMemberDetail)
		return err
	}, seedExtended(OpcodeRequestPledgeMemberDetail, int32(0), "Friend")},
	{"RequestPledgePower", decodes(DecodeRequestPledgePower), seedPacket(OpcodeRequestPledgePower, int32(1), int32(2), int32(0xff))},
	{"RequestPledgeReorganizeMember", decodes(DecodeRequestPledgeReorganizeMember), seedExtended(OpcodeRequestPledgeReorganizeMember, int32(1), "Friend", int32(100), "Mate")},
	{"RequestPledgeSetAcademyMaster", decodes(DecodeRequestPledgeSetAcademyMaster), seedExtended(OpcodeRequestPledgeSetAcademyMaster, int32(1), "Master", "Pupil")},
	{"RequestPledgeSetMemberPowerGrade", decodes(DecodeRequestPledgeSetMemberPowerGrade), seedExtended(OpcodeRequestPledgeSetGrade, "Friend", int32(3))},
	{"RequestPledgeWarList", decodes(DecodeRequestPledgeWarList), seedExtended(OpcodeRequestPledgeWarList, int32(0), int32(0))},
	{"RequestPledgeWarName", decodes(DecodeRequestPledgeWarName), seedPacket(OpcodeRequestStartPledgeWar, "Rivals")},
	{"RequestPledgeWarReply", decodes(DecodeRequestPledgeWarReply), seedPacket(OpcodeRequestReplyStartPledgeWar, int32(1))},
	{"RequestPreviewItem", decodes(DecodeRequestPreviewItem), seedPacket(OpcodeRequestPreviewItem, int32(0), int32(1), int32(2), int32(1060), int32(1835))},
	{"RequestQuestAbort", decodes(DecodeRequestQuestAbort), seedPacket(OpcodeRequestQuestAbort, int32(255))},
	{"RequestSSQStatus", decodes(DecodeRequestSSQStatus), seedPacket(OpcodeRequestSSQStatus, byte(1))},
	{"RequestSetAllyCrest", decodes(DecodeRequestSetAllyCrest), append(seedPacket(OpcodeRequestSetAllyCrest, int32(4)), 1, 2, 3, 4)},
	{"RequestSetPledgeCrest", decodes(DecodeRequestSetPledgeCrest), append(seedPacket(OpcodeRequestSetPledgeCrest, int32(4)), 1, 2, 3, 4)},
	{"RequestTutorialClientEvent", decodes(DecodeRequestTutorialClientEvent), seedPacket(OpcodeRequestTutorialClientEvent, int32(1))},
	{"RequestTutorialLinkHTML", decodes(DecodeRequestTutorialLinkHTML), seedPacket(OpcodeRequestTutorialLinkHTML, "tutorial_01.htm")},
	{"RequestTutorialPassCmdToServer", decodes(DecodeRequestTutorialPassCmdToServer), seedPacket(OpcodeRequestTutorialPassCmdToServer, "close")},
	{"RequestTutorialQuestionMark", decodes(DecodeRequestTutorialQuestionMark), seedPacket(OpcodeRequestTutorialQuestionMark, int32(1))},
	{"RequestUserCommand", decodes(DecodeRequestUserCommand), seedPacket(OpcodeRequestUserCommand, int32(0))},
	{"RequestWithdrawPartyRoom", decodes(DecodeRequestWithdrawPartyRoom), seedExtended(OpcodeRequestWithdrawPartyRoom, int32(1), int32(0))},
	{"SendBypassBuildCmd", decodes(DecodeSendBypassBuildCmd), seedPacket(OpcodeSendBypassBuildCmd, "admin")},
	{"SnoopQuit", decodes(DecodeSnoopQuit), seedPacket(OpcodeSnoopQuit, int32(1))},
}

// TestGameDecoderFuzzSeedsDecode keeps the fuzz seeds honest: each one must
// be a packet its own decoder accepts, so the seed corpus starts on every
// decoder's success path.
func TestGameDecoderFuzzSeedsDecode(t *testing.T) {
	for _, d := range gameDecoders {
		err := d.decode(d.seed)
		_, refused := refusedSeeds[d.name]
		switch {
		case refused && err == nil:
			t.Errorf("%s seed % x decoded, but the decoder refuses every list", d.name, d.seed)
		case !refused && err != nil:
			t.Errorf("%s seed % x: %v", d.name, d.seed, err)
		}
	}
}

// TestEveryGameDecoderIsFuzzed fails when an exported Decode function of this
// package is missing from gameDecoders, so a new client packet cannot ship
// outside FuzzGameClientPackets. A decoder that is not a game packet's
// (none today) would be listed in notFuzzed with the reason.
func TestEveryGameDecoderIsFuzzed(t *testing.T) {
	notFuzzed := map[string]string{}

	fset := token.NewFileSet()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	declared := map[string]bool{}
	fuzzed := map[string]bool{}
	for _, entry := range entries {
		file := entry.Name()
		if !strings.HasSuffix(file, ".go") {
			continue
		}
		f, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		if file == "clientpackets_fuzz_test.go" {
			ast.Inspect(f, func(n ast.Node) bool {
				if id, ok := n.(*ast.Ident); ok && strings.HasPrefix(id.Name, "Decode") {
					fuzzed[id.Name] = true
				}
				return true
			})
			continue
		}
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		for _, decl := range f.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv == nil && fn.Name.IsExported() && strings.HasPrefix(fn.Name.Name, "Decode") {
				declared[fn.Name.Name] = true
			}
		}
	}
	if len(declared) < 100 {
		t.Fatalf("found %d exported Decode functions, the scan is broken", len(declared))
	}
	for _, name := range slices.Sorted(maps.Keys(declared)) {
		if reason, skip := notFuzzed[name]; skip {
			if fuzzed[name] {
				t.Errorf("%s is fuzzed; drop its notFuzzed entry (%s)", name, reason)
			}
			continue
		}
		if !fuzzed[name] {
			t.Errorf("%s is not in gameDecoders: add it with a seed packet so FuzzGameClientPackets feeds it", name)
		}
	}
}

// FuzzGameClientPackets feeds every game client packet decoder the same
// arbitrary payload. Each must return rather than panic, and none may
// allocate more than the payload's length justifies.
func FuzzGameClientPackets(f *testing.F) {
	for _, d := range gameDecoders {
		f.Add(d.seed)
		f.Add(d.seed[:len(d.seed)-1])
	}
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, payload []byte) {
		for _, d := range gameDecoders {
			decodefuzz.Bounded(t, d.name, payload, func() { _ = d.decode(payload) })
		}
	})
}
