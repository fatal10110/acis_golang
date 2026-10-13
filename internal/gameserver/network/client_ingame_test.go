package network

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestGameInboundServesInGameOpcodes pins that every in-game opcode with a
// handler is dispatched through the registry, while the opcodes accepted
// but not wired yet still fall through to the read loop.
func TestGameInboundServesInGameOpcodes(t *testing.T) {
	for _, op := range []byte{
		clientpackets.OpcodeRequestSetPledgeCrest,
		clientpackets.OpcodeRequestJoinAlly,
		clientpackets.OpcodeRequestAnswerJoinAlly,
		clientpackets.OpcodeAllyLeave,
		clientpackets.OpcodeAllyDismiss,
		clientpackets.OpcodeRequestDismissAlly,
		clientpackets.OpcodeRequestSetAllyCrest,
		clientpackets.OpcodeRequestAllyInfo,
		clientpackets.OpcodeExtended,
		clientpackets.OpcodeRequestSkillCoolTime,
		clientpackets.OpcodeRequestMagicSkillUse,
		clientpackets.OpcodeAction,
		clientpackets.OpcodeAttackRequest,
		clientpackets.OpcodeLogout,
		clientpackets.OpcodeMoveBackwardToLocation,
		clientpackets.OpcodeCannotMoveAnymore,
		clientpackets.OpcodeRequestMoveInVehicle,
		clientpackets.OpcodeCannotMoveInVehicle,
		clientpackets.OpcodeRequestGetOnVehicle,
		clientpackets.OpcodeRequestGetOffVehicle,
		clientpackets.OpcodeValidatePosition,
		clientpackets.OpcodeRequestItemList,
		clientpackets.OpcodeUseItem,
		clientpackets.OpcodeRequestUnEquipItem,
		clientpackets.OpcodeRequestDropItem,
		clientpackets.OpcodeRequestDestroyItem,
		clientpackets.OpcodeRequestCrystallizeItem,
		clientpackets.OpcodeRequestRecipeBookOpen,
		clientpackets.OpcodeRequestRecipeBookDestroy,
		clientpackets.OpcodeRequestRecipeItemMakeInfo,
		clientpackets.OpcodeRequestRecipeItemMakeSelf,
		clientpackets.OpcodeRequestHennaItemList,
		clientpackets.OpcodeRequestHennaItemInfo,
		clientpackets.OpcodeRequestHennaEquip,
		clientpackets.OpcodeRequestHennaUnequipList,
		clientpackets.OpcodeRequestHennaUnequipInfo,
		clientpackets.OpcodeRequestHennaUnequip,
		clientpackets.OpcodeRequestPrivateStoreManageSell,
		clientpackets.OpcodeSetPrivateStoreListSell,
		clientpackets.OpcodeRequestPrivateStoreQuitSell,
		clientpackets.OpcodeSetPrivateStoreMsgSell,
		clientpackets.OpcodeRequestPrivateStoreBuy,
		clientpackets.OpcodeRequestPrivateStoreManageBuy,
		clientpackets.OpcodeSetPrivateStoreListBuy,
		clientpackets.OpcodeRequestPrivateStoreQuitBuy,
		clientpackets.OpcodeSetPrivateStoreMsgBuy,
		clientpackets.OpcodeRequestPrivateStoreSell,
		clientpackets.OpcodeRequestRecipeShopMessageSet,
		clientpackets.OpcodeRequestRecipeShopListSet,
		clientpackets.OpcodeRequestRecipeShopManageQuit,
		clientpackets.OpcodeRequestRecipeShopMakeInfo,
		clientpackets.OpcodeRequestRecipeShopMakeItem,
		clientpackets.OpcodeRequestRecipeShopManagePrev,
		clientpackets.OpcodeRequestSiegeAttackerList,
		clientpackets.OpcodeRequestSiegeDefenderList,
		clientpackets.OpcodeRequestJoinSiege,
		clientpackets.OpcodeRequestConfirmSiegeWaitingList,
		clientpackets.OpcodeMultiSellChoose,
		clientpackets.OpcodeRequestEnchantItem,
		clientpackets.OpcodeRequestSkillList,
		clientpackets.OpcodeRequestAcquireSkillInfo,
		clientpackets.OpcodeRequestAcquireSkill,
		clientpackets.OpcodeRequestActionUse,
		clientpackets.OpcodeRequestRestartPoint,
		clientpackets.OpcodeRequestSocialAction,
		clientpackets.OpcodeRequestChangeMoveType,
		clientpackets.OpcodeRequestChangeWaitType,
		clientpackets.OpcodeRequestLinkHtml,
		clientpackets.OpcodeRequestBypassToServer,
		clientpackets.OpcodeRequestShowBoard,
		clientpackets.OpcodeRequestBBSWrite,
		clientpackets.OpcodeSendBypassBuildCmd,
		clientpackets.OpcodeRequestGmList,
		clientpackets.OpcodeRequestPetition,
		clientpackets.OpcodeRequestPetitionCancel,
		clientpackets.OpcodePetitionVote,
		clientpackets.OpcodeRequestTargetCancel,
		clientpackets.OpcodeAppearing,
		clientpackets.OpcodeObserverReturn,
		clientpackets.OpcodeStartRotating,
		clientpackets.OpcodeFinishRotating,
		clientpackets.OpcodeRequestRestart,
		clientpackets.OpcodeSendTimeCheck,
		clientpackets.OpcodeRequestPackageItemList,
		clientpackets.OpcodeRequestPetUseItem,
		clientpackets.OpcodeRequestGiveItemToPet,
		clientpackets.OpcodeRequestGetItemFromPet,
		clientpackets.OpcodeRequestPetGetItem,
		clientpackets.OpcodeRequestJoinParty,
		clientpackets.OpcodeRequestAnswerJoinParty,
		clientpackets.OpcodeRequestWithdrawParty,
		clientpackets.OpcodeRequestOustPartyMember,
		clientpackets.OpcodeRequestListPartyWaiting,
		clientpackets.OpcodeRequestManagePartyRoom,
		clientpackets.OpcodeRequestJoinPartyRoom,
		clientpackets.OpcodeTradeRequest,
		clientpackets.OpcodeAnswerTradeRequest,
		clientpackets.OpcodeAddTradeItem,
		clientpackets.OpcodeTradeDone,
		clientpackets.OpcodeRequestShortCutReg,
		clientpackets.OpcodeRequestShortCutDel,
		clientpackets.OpcodeRequestMakeMacro,
		clientpackets.OpcodeRequestDeleteMacro,
		clientpackets.OpcodeRequestEvaluate,
		clientpackets.OpcodeRequestChangePetName,
		clientpackets.OpcodeRequestBuyItem,
		clientpackets.OpcodeRequestBuySeed,
		clientpackets.OpcodeRequestBuyProcure,
		clientpackets.OpcodeRequestPreviewItem,
		clientpackets.OpcodeRequestSSQStatus,
		clientpackets.OpcodeDlgAnswer,
		clientpackets.OpcodeRequestSellItem,
		clientpackets.OpcodeSendWarehouseDeposit,
		clientpackets.OpcodeSendWarehouseWithdraw,
		clientpackets.OpcodeRequestPackageSend,
		clientpackets.OpcodeRequestFriendInvite,
		clientpackets.OpcodeRequestAnswerFriendInvite,
		clientpackets.OpcodeRequestFriendList,
		clientpackets.OpcodeRequestFriendDel,
		clientpackets.OpcodeRequestBlock,
		clientpackets.OpcodeRequestSendL2FriendSay,
		clientpackets.OpcodeRequestJoinPledge,
		clientpackets.OpcodeRequestAnswerJoinPledge,
		clientpackets.OpcodeRequestOustPledgeMember,
		clientpackets.OpcodeRequestGiveNickName,
		clientpackets.OpcodeRequestPledgeInfo,
		clientpackets.OpcodeRequestPledgePower,
		clientpackets.OpcodeRequestWithdrawPledge,
		clientpackets.OpcodeRequestPledgeMemberList,
		clientpackets.OpcodeRequestStartPledgeWar,
		clientpackets.OpcodeRequestStopPledgeWar,
		clientpackets.OpcodeRequestSurrenderPledgeWar,
		clientpackets.OpcodeRequestSurrenderPersonally,
		clientpackets.OpcodeRequestReplyStartPledgeWar,
		clientpackets.OpcodeRequestReplyStopPledgeWar,
		clientpackets.OpcodeRequestReplySurrenderPledgeWar,
		clientpackets.OpcodeRequestShowMiniMap,
		clientpackets.OpcodeRequestRecordInfo,
		clientpackets.OpcodeSay2,
		clientpackets.OpcodeRequestUserCommand,
		clientpackets.OpcodeRequestQuestListInGame,
		clientpackets.OpcodeRequestTutorialLinkHTML,
		clientpackets.OpcodeRequestTutorialPassCmdToServer,
		clientpackets.OpcodeRequestTutorialQuestionMark,
		clientpackets.OpcodeRequestTutorialClientEvent,
		clientpackets.OpcodeRequestQuestAbort,
	} {
		if _, ok := gameInbound[op]; !ok {
			t.Errorf("gameInbound has no handler for opcode %#x", op)
		}
	}
	for _, op := range []byte{
		clientpackets.OpcodeDummy1A, clientpackets.OpcodeDummy23, clientpackets.OpcodeDummy2E,
		clientpackets.OpcodeDummy34, clientpackets.OpcodeDummy3E, clientpackets.OpcodeGameGuardReply,
	} {
		if _, ok := gameInbound[op]; ok {
			t.Errorf("gameInbound has a handler for unwired opcode %#x", op)
		}
	}
}

func TestGameExInboundServesExtendedSubOpcodes(t *testing.T) {
	for _, sub := range []uint16{
		clientpackets.OpcodeRequestAutoSoulShot,
		clientpackets.OpcodeRequestExEnchantSkillInfo,
		clientpackets.OpcodeRequestExEnchantSkill,
		clientpackets.OpcodeRequestManorList,
		clientpackets.OpcodeRequestSetSeed,
		clientpackets.OpcodeRequestSetCrop,
		clientpackets.OpcodeRequestProcureCropList,
		clientpackets.OpcodeRequestExPledgeCrestLarge,
		clientpackets.OpcodeRequestExSetPledgeCrestLarge,
		clientpackets.OpcodeRequestPledgePowerGrades,
		clientpackets.OpcodeRequestPledgeMemberPower,
		clientpackets.OpcodeRequestPledgeMemberDetail,
		clientpackets.OpcodeRequestPledgeSetGrade,
		clientpackets.OpcodeRequestPledgeWarList,
		clientpackets.OpcodeRequestPledgeReorganizeMember,
		clientpackets.OpcodeRequestPledgeSetAcademyMaster,
		clientpackets.OpcodeRequestGetBossRecord,
		clientpackets.OpcodeRequestWriteHeroWords,
		clientpackets.OpcodeRequestExFishRanking,
		clientpackets.OpcodeRequestCursedWeaponList,
		clientpackets.OpcodeRequestExMagicSkillUseGround,
		clientpackets.OpcodeRequestConfirmTargetItem,
		clientpackets.OpcodeRequestConfirmRefinerItem,
		clientpackets.OpcodeRequestConfirmGemStone,
		clientpackets.OpcodeRequestRefine,
		clientpackets.OpcodeRequestConfirmCancelItem,
		clientpackets.OpcodeRequestRefineCancel,
		clientpackets.OpcodeRequestChangePartyLeader,
		clientpackets.OpcodeRequestExAskJoinMPCC,
		clientpackets.OpcodeRequestExAcceptJoinMPCC,
		clientpackets.OpcodeRequestExOustFromMPCC,
		clientpackets.OpcodeRequestExMPCCShowPartyMembersInfo,
		clientpackets.OpcodeRequestDuelStart,
		clientpackets.OpcodeRequestDuelAnswerStart,
		clientpackets.OpcodeRequestDuelSurrender,
		clientpackets.OpcodeRequestOustFromPartyRoom,
		clientpackets.OpcodeRequestDismissPartyRoom,
		clientpackets.OpcodeRequestWithdrawPartyRoom,
		clientpackets.OpcodeRequestAskJoinPartyRoom,
		clientpackets.OpcodeAnswerJoinPartyRoom,
		clientpackets.OpcodeRequestListPartyMatchingWaitingRoom,
		clientpackets.OpcodeRequestExitPartyMatchingWaitingRoom,
		clientpackets.OpcodeRequestCursedWeaponLocation,
	} {
		if _, ok := gameExInbound[sub]; !ok {
			t.Errorf("gameExInbound has no handler for sub-opcode %#x", sub)
		}
	}
}

// TestExtendedFrameWithoutSubOpcodeIsDropped pins that an extended frame
// too short to carry its sub-opcode is dropped without closing the
// connection and without counting as an unknown packet.
func TestExtendedFrameWithoutSubOpcodeIsDropped(t *testing.T) {
	c, _, _, _, _ := newLinkedGameClientEnterWorld(t)

	for i := 0; i < maxUnknownPerMin+5; i++ {
		c.Send([]byte{clientpackets.OpcodeExtended, 0x01})
	}
	c.Send(encodeRequestManorList())
	if frame := c.Read(); frame[0] != serverpackets.OpcodeExtended {
		t.Fatalf("post-short-extended opcode = %#x, want ExSendManorList under Extended (%#x)", frame[0], serverpackets.OpcodeExtended)
	}
}
