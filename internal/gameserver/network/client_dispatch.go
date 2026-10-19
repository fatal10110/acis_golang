package network

import (
	"context"
	"errors"
	"fmt"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
)

// errDisconnect is what an inbound handler returns to close the connection.
var errDisconnect = errors.New("network: disconnect client")

// inboundFunc handles one client frame of the opcode it is registered for.
// It returns nil to read the next frame, errDisconnect (wrapped or not) to
// close the connection, and any other error to have it logged and read on.
type inboundFunc func(l *GameClientLink, ctx context.Context, cs *connState, payload []byte) error

// inboundOpcode is a dispatch key: the frame's opcode byte, or an extended
// frame's uint16 sub-opcode.
type inboundOpcode interface{ ~uint8 | ~uint16 }

// inboundRegistry maps the opcodes of one dispatch level (the frame's
// opcode byte, or an extended frame's uint16 sub-opcode) to their handlers.
// It is filled before any connection is served and only read afterwards, so
// every connection's read goroutine may share it.
type inboundRegistry[K inboundOpcode] map[K]inboundFunc

// gameInbound holds the handlers of the frame's opcode byte; the read loop
// dispatches every accepted frame through it.
var gameInbound = newGameInbound()

// gameExInbound holds the handlers of an extended frame's uint16
// sub-opcode; handleExtended asks it.
var gameExInbound = newGameExInbound()

// newGameInbound lists every opcode-byte handler. Which opcodes a client
// may send in its current state is the state gate's business, decided
// before any of these runs.
func newGameInbound() inboundRegistry[byte] {
	reg := inboundRegistry[byte]{}
	register(reg, clientpackets.OpcodeProtocolVersion, clientpackets.DecodeProtocolVersion, (*GameClientLink).handleProtocolVersion)
	register(reg, clientpackets.OpcodeAuthLogin, clientpackets.DecodeAuthLogin, (*GameClientLink).handleAuthLogin)
	register(reg, clientpackets.OpcodeRequestNewCharacter, noPayload, (*GameClientLink).handleRequestNewCharacter)
	register(reg, clientpackets.OpcodeRequestCharacterCreate, clientpackets.DecodeRequestCharacterCreate, (*GameClientLink).handleRequestCharacterCreate)
	register(reg, clientpackets.OpcodeRequestCharacterDelete, clientpackets.DecodeRequestCharacterDelete, (*GameClientLink).handleRequestCharacterDelete)
	register(reg, clientpackets.OpcodeCharacterRestore, clientpackets.DecodeCharacterRestore, (*GameClientLink).handleCharacterRestore)
	register(reg, clientpackets.OpcodeRequestGameStart, clientpackets.DecodeRequestGameStart, (*GameClientLink).handleRequestGameStart)
	register(reg, clientpackets.OpcodeEnterWorld, noPayload, (*GameClientLink).handleEnterWorld)
	register(reg, clientpackets.OpcodeRequestPledgeCrest, clientpackets.DecodeRequestPledgeCrest, (*GameClientLink).handleRequestPledgeCrest)
	register(reg, clientpackets.OpcodeRequestAllyCrest, clientpackets.DecodeRequestAllyCrest, (*GameClientLink).handleRequestAllyCrest)
	register(reg, clientpackets.OpcodeRequestSetPledgeCrest, clientpackets.DecodeRequestSetPledgeCrest, whenLive((*GameClientLink).requestSetPledgeCrest))
	register(reg, clientpackets.OpcodeRequestJoinAlly, clientpackets.DecodeRequestJoinAlly, whenLive((*GameClientLink).requestJoinAlly))
	register(reg, clientpackets.OpcodeRequestAnswerJoinAlly, clientpackets.DecodeRequestAnswerJoinAlly, whenLive((*GameClientLink).requestAnswerJoinAlly))
	register(reg, clientpackets.OpcodeAllyLeave, noPayload, whenLiveNoBody((*GameClientLink).allyLeave))
	register(reg, clientpackets.OpcodeAllyDismiss, clientpackets.DecodeAllyDismiss, whenLive((*GameClientLink).allyDismiss))
	register(reg, clientpackets.OpcodeRequestDismissAlly, noPayload, whenLiveNoBody((*GameClientLink).requestDismissAlly))
	register(reg, clientpackets.OpcodeRequestSetAllyCrest, clientpackets.DecodeRequestSetAllyCrest, whenLive((*GameClientLink).requestSetAllyCrest))
	register(reg, clientpackets.OpcodeRequestAllyInfo, noPayload, whenLiveNoBody((*GameClientLink).requestAllyInfo))
	registerFrame(reg, clientpackets.OpcodeExtended, (*GameClientLink).handleExtended)
	register(reg, clientpackets.OpcodeRequestSkillCoolTime, noPayload, (*GameClientLink).handleRequestSkillCoolTime)
	register(reg, clientpackets.OpcodeRequestMagicSkillUse, clientpackets.DecodeRequestMagicSkillUse, whenLive((*GameClientLink).handleMagicSkillUse))
	register(reg, clientpackets.OpcodeAction, clientpackets.DecodeAction, (*GameClientLink).handleAction)
	register(reg, clientpackets.OpcodeAttackRequest, clientpackets.DecodeAttackRequest, (*GameClientLink).handleAttackRequest)
	register(reg, clientpackets.OpcodeLogout, noPayload, (*GameClientLink).handleLogout)
	register(reg, clientpackets.OpcodeMoveBackwardToLocation, clientpackets.DecodeMoveBackwardToLocation, (*GameClientLink).handleMoveBackwardToLocation)
	register(reg, clientpackets.OpcodeCannotMoveAnymore, clientpackets.DecodeCannotMoveAnymore, (*GameClientLink).handleCannotMoveAnymore)
	register(reg, clientpackets.OpcodeRequestMoveInVehicle, clientpackets.DecodeRequestMoveToLocationInVehicle, whenLive((*GameClientLink).requestMoveInVehicle))
	register(reg, clientpackets.OpcodeCannotMoveInVehicle, clientpackets.DecodeCannotMoveAnymoreInVehicle, whenLive((*GameClientLink).cannotMoveInVehicle))
	register(reg, clientpackets.OpcodeRequestGetOnVehicle, clientpackets.DecodeRequestGetOnVehicle, whenLive((*GameClientLink).requestGetOnVehicle))
	register(reg, clientpackets.OpcodeRequestGetOffVehicle, clientpackets.DecodeRequestGetOffVehicle, whenLive((*GameClientLink).requestGetOffVehicle))
	register(reg, clientpackets.OpcodeValidatePosition, clientpackets.DecodeValidatePosition, (*GameClientLink).handleValidatePosition)
	register(reg, clientpackets.OpcodeRequestItemList, noPayload, (*GameClientLink).handleRequestItemList)
	register(reg, clientpackets.OpcodeUseItem, clientpackets.DecodeUseItem, (*GameClientLink).handleUseItem)
	register(reg, clientpackets.OpcodeRequestUnEquipItem, clientpackets.DecodeUnequipItem, (*GameClientLink).handleRequestUnEquipItem)
	register(reg, clientpackets.OpcodeRequestDropItem, clientpackets.DecodeRequestDropItem, (*GameClientLink).handleRequestDropItem)
	register(reg, clientpackets.OpcodeRequestDestroyItem, clientpackets.DecodeRequestDestroyItem, (*GameClientLink).handleRequestDestroyItem)
	register(reg, clientpackets.OpcodeRequestCrystallizeItem, clientpackets.DecodeRequestCrystallizeItem, whenLive((*GameClientLink).crystallizeLiveItem))
	register(reg, clientpackets.OpcodeRequestRecipeBookOpen, clientpackets.DecodeRequestRecipeBookOpen, whenLive((*GameClientLink).openRecipeBook))
	register(reg, clientpackets.OpcodeRequestRecipeBookDestroy, clientpackets.DecodeRequestRecipeBookDestroy, whenLive((*GameClientLink).destroyRecipe))
	register(reg, clientpackets.OpcodeRequestRecipeItemMakeInfo, clientpackets.DecodeRequestRecipeItemMakeInfo, whenLive((*GameClientLink).sendRecipeItemMakeInfo))
	register(reg, clientpackets.OpcodeRequestRecipeItemMakeSelf, clientpackets.DecodeRequestRecipeItemMakeSelf, (*GameClientLink).handleRequestRecipeItemMakeSelf)
	register(reg, clientpackets.OpcodeRequestHennaItemList, clientpackets.DecodeRequestHennaItemList, (*GameClientLink).handleRequestHennaItemList)
	register(reg, clientpackets.OpcodeRequestHennaItemInfo, clientpackets.DecodeRequestHennaItemInfo, whenLive((*GameClientLink).sendHennaItemInfo))
	register(reg, clientpackets.OpcodeRequestHennaEquip, clientpackets.DecodeRequestHennaEquip, whenLive((*GameClientLink).drawHenna))
	register(reg, clientpackets.OpcodeRequestHennaUnequipList, clientpackets.DecodeRequestHennaUnequipList, (*GameClientLink).handleRequestHennaUnequipList)
	register(reg, clientpackets.OpcodeRequestHennaUnequipInfo, clientpackets.DecodeRequestHennaUnequipInfo, whenLive((*GameClientLink).sendHennaUnequipInfo))
	register(reg, clientpackets.OpcodeRequestHennaUnequip, clientpackets.DecodeRequestHennaUnequip, whenLive((*GameClientLink).deleteHenna))
	// Private store and workshop requests share one decoder table.
	for _, op := range []byte{
		clientpackets.OpcodeRequestPrivateStoreManageSell, clientpackets.OpcodeSetPrivateStoreListSell,
		clientpackets.OpcodeRequestPrivateStoreQuitSell, clientpackets.OpcodeSetPrivateStoreMsgSell,
		clientpackets.OpcodeRequestPrivateStoreBuy, clientpackets.OpcodeRequestPrivateStoreManageBuy,
		clientpackets.OpcodeSetPrivateStoreListBuy, clientpackets.OpcodeRequestPrivateStoreQuitBuy,
		clientpackets.OpcodeSetPrivateStoreMsgBuy, clientpackets.OpcodeRequestPrivateStoreSell,
		clientpackets.OpcodeRequestRecipeShopMessageSet, clientpackets.OpcodeRequestRecipeShopListSet,
		clientpackets.OpcodeRequestRecipeShopManageQuit, clientpackets.OpcodeRequestRecipeShopMakeInfo,
		clientpackets.OpcodeRequestRecipeShopMakeItem, clientpackets.OpcodeRequestRecipeShopManagePrev,
	} {
		registerFrame(reg, op, func(l *GameClientLink, _ context.Context, cs *connState, payload []byte) error {
			if !l.dispatchPrivateStore(cs.client, cs.live, op, payload) {
				return errDisconnect
			}
			return nil
		})
	}
	register(reg, clientpackets.OpcodeRequestSiegeAttackerList, clientpackets.DecodeRequestSiegeList, whenLive((*GameClientLink).requestSiegeAttackerList))
	register(reg, clientpackets.OpcodeRequestSiegeDefenderList, clientpackets.DecodeRequestSiegeList, whenLive((*GameClientLink).requestSiegeDefenderList))
	register(reg, clientpackets.OpcodeRequestJoinSiege, clientpackets.DecodeRequestJoinSiege, whenLive((*GameClientLink).requestJoinSiege))
	register(reg, clientpackets.OpcodeRequestConfirmSiegeWaitingList, clientpackets.DecodeRequestConfirmSiegeWaitingList, whenLive((*GameClientLink).requestConfirmSiegeWaitingList))
	register(reg, clientpackets.OpcodeMultiSellChoose, clientpackets.DecodeMultiSellChoose, (*GameClientLink).handleMultiSellChoose)
	register(reg, clientpackets.OpcodeRequestEnchantItem, clientpackets.DecodeRequestEnchantItem, (*GameClientLink).handleRequestEnchantItem)
	register(reg, clientpackets.OpcodeRequestSkillList, noPayload, (*GameClientLink).handleRequestSkillList)
	register(reg, clientpackets.OpcodeRequestAcquireSkillInfo, clientpackets.DecodeRequestAcquireSkillInfo, whenLive((*GameClientLink).sendAcquireSkillInfo))
	register(reg, clientpackets.OpcodeRequestAcquireSkill, clientpackets.DecodeRequestAcquireSkill, whenLive((*GameClientLink).learnAcquireSkill))
	register(reg, clientpackets.OpcodeRequestActionUse, clientpackets.DecodeRequestActionUse, (*GameClientLink).handleRequestActionUse)
	register(reg, clientpackets.OpcodeRequestRestartPoint, clientpackets.DecodeRequestRestartPoint, whenLive((*GameClientLink).restartLivePlayer))
	register(reg, clientpackets.OpcodeRequestSocialAction, clientpackets.DecodeRequestSocialAction, (*GameClientLink).handleRequestSocialAction)
	register(reg, clientpackets.OpcodeRequestChangeMoveType, clientpackets.DecodeRequestChangeMoveType, (*GameClientLink).handleRequestChangeMoveType)
	register(reg, clientpackets.OpcodeRequestChangeWaitType, clientpackets.DecodeRequestChangeWaitType, (*GameClientLink).handleRequestChangeWaitType)
	register(reg, clientpackets.OpcodeRequestLinkHtml, clientpackets.DecodeRequestLinkHTML, (*GameClientLink).handleRequestLinkHTML)
	register(reg, clientpackets.OpcodeRequestBypassToServer, clientpackets.DecodeRequestBypassToServer, (*GameClientLink).handleRequestBypassToServer)
	register(reg, clientpackets.OpcodeRequestShowBoard, clientpackets.DecodeRequestShowBoard, whenLiveNoBody((*GameClientLink).requestShowBoard))
	register(reg, clientpackets.OpcodeRequestBBSWrite, clientpackets.DecodeRequestBBSWrite, whenLive((*GameClientLink).requestBBSWrite))
	register(reg, clientpackets.OpcodeSendBypassBuildCmd, clientpackets.DecodeSendBypassBuildCmd, whenLive((*GameClientLink).sendBypassBuildCmd))
	register(reg, clientpackets.OpcodeRequestGmList, noPayload, whenLiveNoBody((*GameClientLink).requestGmList))
	register(reg, clientpackets.OpcodeRequestPetition, clientpackets.DecodeRequestPetition, whenLive((*GameClientLink).requestPetition))
	register(reg, clientpackets.OpcodeRequestPetitionCancel, noPayload, whenLiveNoBody((*GameClientLink).requestPetitionCancel))
	register(reg, clientpackets.OpcodePetitionVote, clientpackets.DecodePetitionVote, whenLive((*GameClientLink).petitionVote))
	register(reg, clientpackets.OpcodeRequestTargetCancel, clientpackets.DecodeRequestTargetCancel, whenLive((*GameClientLink).requestTargetCancel))
	register(reg, clientpackets.OpcodeAppearing, clientpackets.DecodeAppearing, (*GameClientLink).handleAppearing)
	register(reg, clientpackets.OpcodeObserverReturn, clientpackets.DecodeObserverReturn, (*GameClientLink).handleObserverReturn)
	register(reg, clientpackets.OpcodeStartRotating, clientpackets.DecodeStartRotating, (*GameClientLink).handleStartRotating)
	register(reg, clientpackets.OpcodeFinishRotating, clientpackets.DecodeFinishRotating, (*GameClientLink).handleFinishRotating)
	register(reg, clientpackets.OpcodeRequestRestart, noPayload, (*GameClientLink).handleRequestRestart)
	register(reg, clientpackets.OpcodeSendTimeCheck, clientpackets.DecodeSendTimeCheck, (*GameClientLink).handleSendTimeCheck)
	register(reg, clientpackets.OpcodeRequestPackageItemList, clientpackets.DecodeRequestPackageSendableItemList, (*GameClientLink).handleRequestPackageItemList)
	register(reg, clientpackets.OpcodeRequestPetUseItem, clientpackets.DecodeRequestPetUseItem, (*GameClientLink).handleRequestPetUseItem)
	register(reg, clientpackets.OpcodeRequestGiveItemToPet, clientpackets.DecodeRequestGiveItemToPet, (*GameClientLink).handleRequestGiveItemToPet)
	register(reg, clientpackets.OpcodeRequestGetItemFromPet, clientpackets.DecodeRequestGetItemFromPet, (*GameClientLink).handleRequestGetItemFromPet)
	register(reg, clientpackets.OpcodeRequestPetGetItem, clientpackets.DecodeRequestPetGetItem, (*GameClientLink).handleRequestPetGetItem)
	register(reg, clientpackets.OpcodeRequestJoinParty, clientpackets.DecodeRequestJoinParty, whenLive((*GameClientLink).requestJoinParty))
	register(reg, clientpackets.OpcodeRequestAnswerJoinParty, clientpackets.DecodeRequestAnswerJoinParty, whenLive((*GameClientLink).requestAnswerJoinParty))
	// A player in no party leaves nothing and hears nothing: the menu
	// action waits on no answer.
	register(reg, clientpackets.OpcodeRequestWithdrawParty, noPayload, whenLiveNoBody((*GameClientLink).withdrawParty))
	register(reg, clientpackets.OpcodeRequestOustPartyMember, clientpackets.DecodeRequestOustPartyMember, whenLive((*GameClientLink).requestOustPartyMember))
	register(reg, clientpackets.OpcodeRequestListPartyWaiting, clientpackets.DecodeRequestListPartyWaiting, whenLive((*GameClientLink).requestListPartyWaiting))
	register(reg, clientpackets.OpcodeRequestManagePartyRoom, clientpackets.DecodeRequestManagePartyRoom, whenLive((*GameClientLink).requestManagePartyRoom))
	register(reg, clientpackets.OpcodeRequestJoinPartyRoom, clientpackets.DecodeRequestJoinPartyRoom, whenLive((*GameClientLink).requestJoinPartyRoom))
	register(reg, clientpackets.OpcodeTradeRequest, clientpackets.DecodeTradeRequest, whenLive((*GameClientLink).handleTradeRequest))
	register(reg, clientpackets.OpcodeAnswerTradeRequest, clientpackets.DecodeAnswerTradeRequest, whenLive((*GameClientLink).handleAnswerTradeRequest))
	register(reg, clientpackets.OpcodeAddTradeItem, clientpackets.DecodeAddTradeItem, whenLive((*GameClientLink).handleAddTradeItem))
	register(reg, clientpackets.OpcodeTradeDone, clientpackets.DecodeTradeDone, (*GameClientLink).serveTradeDone)
	register(reg, clientpackets.OpcodeRequestShortCutReg, clientpackets.DecodeRequestShortCutReg, whenLive((*GameClientLink).registerShortcut))
	register(reg, clientpackets.OpcodeRequestShortCutDel, clientpackets.DecodeRequestShortCutDel, whenLive((*GameClientLink).deleteShortcut))
	register(reg, clientpackets.OpcodeRequestMakeMacro, clientpackets.DecodeRequestMakeMacro, whenLive((*GameClientLink).makeMacro))
	register(reg, clientpackets.OpcodeRequestDeleteMacro, clientpackets.DecodeRequestDeleteMacro, whenLive((*GameClientLink).deleteMacro))
	register(reg, clientpackets.OpcodeRequestEvaluate, clientpackets.DecodeRequestEvaluate, whenLive((*GameClientLink).evaluate))
	register(reg, clientpackets.OpcodeRequestChangePetName, clientpackets.DecodeRequestChangePetName, (*GameClientLink).serveRequestChangePetName)
	registerCapped(reg, clientpackets.OpcodeRequestBuyItem, clientpackets.DecodeRequestBuyItem, whenLive((*GameClientLink).requestBuyItem))
	registerCapped(reg, clientpackets.OpcodeRequestBuySeed, clientpackets.DecodeRequestBuySeed, (*GameClientLink).handleRequestBuySeed)
	registerCapped(reg, clientpackets.OpcodeRequestBuyProcure, clientpackets.DecodeRequestBuyProcure, (*GameClientLink).handleRequestBuyProcure)
	register(reg, clientpackets.OpcodeRequestPreviewItem, clientpackets.DecodeRequestPreviewItem, whenLive((*GameClientLink).requestPreviewItem))
	register(reg, clientpackets.OpcodeRequestSSQStatus, clientpackets.DecodeRequestSSQStatus, whenLive((*GameClientLink).requestSSQStatus))
	register(reg, clientpackets.OpcodeDlgAnswer, clientpackets.DecodeDlgAnswer, whenLive((*GameClientLink).handleDlgAnswer))
	registerCapped(reg, clientpackets.OpcodeRequestSellItem, clientpackets.DecodeRequestSellItem, whenLive((*GameClientLink).requestSellItem))
	registerFrame(reg, clientpackets.OpcodeSendWarehouseDeposit, unlessWarehouseDisabled(cappedFrame(clientpackets.DecodeSendWarehouseDepositList, whenLive((*GameClientLink).requestWarehouseDeposit))))
	registerCapped(reg, clientpackets.OpcodeSendWarehouseWithdraw, clientpackets.DecodeSendWarehouseWithdrawList, whenLive((*GameClientLink).requestWarehouseWithdraw))
	registerCapped(reg, clientpackets.OpcodeRequestPackageSend, clientpackets.DecodeRequestPackageSend, whenLive((*GameClientLink).requestPackageSend))
	register(reg, clientpackets.OpcodeRequestFriendInvite, clientpackets.DecodeRequestFriendInvite, whenLive((*GameClientLink).handleRequestFriendInvite))
	register(reg, clientpackets.OpcodeRequestAnswerFriendInvite, clientpackets.DecodeRequestAnswerFriendInvite, whenLive((*GameClientLink).handleRequestAnswerFriendInvite))
	register(reg, clientpackets.OpcodeRequestFriendList, noPayload, whenLiveNoBody((*GameClientLink).handleRequestFriendList))
	register(reg, clientpackets.OpcodeRequestFriendDel, clientpackets.DecodeRequestFriendDel, whenLive((*GameClientLink).handleRequestFriendDel))
	register(reg, clientpackets.OpcodeRequestBlock, clientpackets.DecodeRequestBlock, whenLive((*GameClientLink).handleRequestBlock))
	register(reg, clientpackets.OpcodeRequestSendL2FriendSay, clientpackets.DecodeRequestSendL2FriendSay, whenLive((*GameClientLink).handleRequestSendL2FriendSay))
	register(reg, clientpackets.OpcodeRequestJoinPledge, clientpackets.DecodeRequestJoinPledge, whenLive((*GameClientLink).requestJoinPledge))
	register(reg, clientpackets.OpcodeRequestAnswerJoinPledge, clientpackets.DecodeRequestAnswerJoinPledge, whenLive((*GameClientLink).requestAnswerJoinPledge))
	register(reg, clientpackets.OpcodeRequestOustPledgeMember, clientpackets.DecodeRequestOustPledgeMember, whenLive((*GameClientLink).requestOustPledgeMember))
	register(reg, clientpackets.OpcodeRequestGiveNickName, clientpackets.DecodeRequestGiveNickName, whenLive((*GameClientLink).requestGiveNickName))
	register(reg, clientpackets.OpcodeRequestPledgeInfo, clientpackets.DecodeRequestPledgeInfo, whenLive((*GameClientLink).requestPledgeInfo))
	register(reg, clientpackets.OpcodeRequestPledgePower, clientpackets.DecodeRequestPledgePower, whenLive((*GameClientLink).requestPledgePower))
	register(reg, clientpackets.OpcodeRequestWithdrawPledge, noPayload, whenLiveNoBody((*GameClientLink).requestWithdrawPledge))
	register(reg, clientpackets.OpcodeRequestPledgeMemberList, noPayload, whenLiveNoBody((*GameClientLink).requestPledgeMemberList))
	register(reg, clientpackets.OpcodeRequestStartPledgeWar, clientpackets.DecodeRequestPledgeWarName, whenLive((*GameClientLink).requestStartPledgeWar))
	register(reg, clientpackets.OpcodeRequestStopPledgeWar, clientpackets.DecodeRequestPledgeWarName, whenLive((*GameClientLink).requestStopPledgeWar))
	register(reg, clientpackets.OpcodeRequestSurrenderPledgeWar, clientpackets.DecodeRequestPledgeWarName, whenLive((*GameClientLink).requestSurrenderPledgeWar))
	register(reg, clientpackets.OpcodeRequestSurrenderPersonally, clientpackets.DecodeRequestPledgeWarName, whenLive((*GameClientLink).requestSurrenderPersonally))
	for _, op := range []byte{
		clientpackets.OpcodeRequestReplyStartPledgeWar, clientpackets.OpcodeRequestReplyStopPledgeWar,
		clientpackets.OpcodeRequestReplySurrenderPledgeWar,
	} {
		register(reg, op, clientpackets.DecodeRequestPledgeWarReply, func(l *GameClientLink, _ context.Context, cs *connState, _ clientpackets.RequestPledgeWarReply) error {
			if cs.live != nil {
				onLive(cs.live, func() { l.requestReplyPledgeWar(cs.live, op) })
			}
			return nil
		})
	}
	register(reg, clientpackets.OpcodeRequestShowMiniMap, noPayload, (*GameClientLink).handleRequestShowMiniMap)
	register(reg, clientpackets.OpcodeRequestRecordInfo, noPayload, whenLiveNoBody((*GameClientLink).requestRecordInfo))
	register(reg, clientpackets.OpcodeSay2, clientpackets.DecodeSay2, (*GameClientLink).serveSay2)
	register(reg, clientpackets.OpcodeRequestUserCommand, clientpackets.DecodeRequestUserCommand, (*GameClientLink).handleRequestUserCommand)
	register(reg, clientpackets.OpcodeRequestQuestListInGame, noPayload, (*GameClientLink).handleRequestQuestListInGame)
	for _, op := range []byte{
		clientpackets.OpcodeRequestTutorialLinkHTML, clientpackets.OpcodeRequestTutorialPassCmdToServer,
		clientpackets.OpcodeRequestTutorialQuestionMark, clientpackets.OpcodeRequestTutorialClientEvent,
	} {
		register(reg, op, tutorialDecoder(op), (*GameClientLink).handleTutorialEvent)
	}
	register(reg, clientpackets.OpcodeRequestQuestAbort, clientpackets.DecodeRequestQuestAbort, (*GameClientLink).handleRequestQuestAbort)
	for _, op := range unwiredOpcodes {
		registerFrame(reg, op, func(l *GameClientLink, _ context.Context, _ *connState, _ []byte) error {
			l.logNotWired(op)
			return nil
		})
	}
	return reg
}

// unwiredOpcodes are accepted in-game opcodes that do nothing a client can
// observe: the client's filler packets and its game-guard reply, whose
// recorded answer nothing reads. Each one is read and dropped without an
// answer, and without counting as an unknown packet; the client waits on
// nothing after sending any of them.
var unwiredOpcodes = []byte{
	clientpackets.OpcodeDummy1A, clientpackets.OpcodeDummy23, clientpackets.OpcodeDummy2E,
	clientpackets.OpcodeDummy34, clientpackets.OpcodeDummy3E, clientpackets.OpcodeGameGuardReply,
}

// logNotWired logs a frame of one of the unwiredOpcodes.
func (l *GameClientLink) logNotWired(opcode byte) {
	l.log.Warn().Str("opcode", fmt.Sprintf("%#x", opcode)).Msg("Opcode not wired")
}

// clearsSpawnProtection reports whether a frame of opcode ends the player's
// spawn protection once its handler is done.
func clearsSpawnProtection(opcode byte) bool {
	switch opcode {
	case clientpackets.OpcodeEnterWorld, clientpackets.OpcodeAction,
		clientpackets.OpcodeRequestPledgeCrest, clientpackets.OpcodeAppearing,
		clientpackets.OpcodeRequestPledgeInfo, clientpackets.OpcodeSay2:
		return false
	default:
		return true
	}
}

// dispatch serves an accepted frame through gameInbound and reports whether
// the connection keeps reading. An accepted opcode with no handler is
// logged and the frame dropped.
func (l *GameClientLink) dispatch(ctx context.Context, cs *connState, payload []byte) (open bool) {
	opcode := payload[0]
	found, open := gameInbound.serve(l, ctx, cs, opcode, payload)
	if !found {
		l.log.Info().Str("opcode", fmt.Sprintf("%#x", opcode)).Str("state", cs.client.State().String()).
			Msg("game client: accepted opcode not implemented yet")
	}
	return open
}

// newGameExInbound lists every extended sub-opcode handler. Each decoder
// reads the whole frame, opcode byte and sub-opcode included.
func newGameExInbound() inboundRegistry[uint16] {
	reg := inboundRegistry[uint16]{}
	register(reg, clientpackets.OpcodeRequestAutoSoulShot, clientpackets.DecodeRequestAutoSoulShot, whenLive((*GameClientLink).handleAutoSoulShot))
	register(reg, clientpackets.OpcodeRequestExEnchantSkillInfo, clientpackets.DecodeRequestExEnchantSkillInfo, whenLive((*GameClientLink).sendEnchantSkillInfo))
	register(reg, clientpackets.OpcodeRequestExEnchantSkill, clientpackets.DecodeRequestExEnchantSkill, whenLive((*GameClientLink).applyEnchantSkill))
	register(reg, clientpackets.OpcodeRequestManorList, noPayload, (*GameClientLink).handleRequestManorList)
	registerCapped(reg, clientpackets.OpcodeRequestSetSeed, clientpackets.DecodeRequestSetSeed, (*GameClientLink).handleRequestSetSeed)
	registerCapped(reg, clientpackets.OpcodeRequestSetCrop, clientpackets.DecodeRequestSetCrop, whenLive((*GameClientLink).requestSetCrop))
	registerCapped(reg, clientpackets.OpcodeRequestProcureCropList, clientpackets.DecodeRequestProcureCropList, whenLive((*GameClientLink).requestProcureCropList))
	register(reg, clientpackets.OpcodeRequestExPledgeCrestLarge, clientpackets.DecodeRequestExPledgeCrestLarge, (*GameClientLink).handleRequestExPledgeCrestLarge)
	register(reg, clientpackets.OpcodeRequestExSetPledgeCrestLarge, clientpackets.DecodeRequestExSetPledgeCrestLarge, whenLive((*GameClientLink).requestSetLargePledgeCrest))
	register(reg, clientpackets.OpcodeRequestPledgePowerGrades, noPayload, whenLiveNoBody((*GameClientLink).requestPledgePowerGradeList))
	register(reg, clientpackets.OpcodeRequestPledgeMemberPower, pledgeMemberNameDecoder(clientpackets.OpcodeRequestPledgeMemberPower), whenLive((*GameClientLink).requestPledgeMemberPowerInfo))
	register(reg, clientpackets.OpcodeRequestPledgeMemberDetail, pledgeMemberNameDecoder(clientpackets.OpcodeRequestPledgeMemberDetail), whenLive((*GameClientLink).requestPledgeMemberInfo))
	register(reg, clientpackets.OpcodeRequestPledgeSetGrade, clientpackets.DecodeRequestPledgeSetMemberPowerGrade, whenLive((*GameClientLink).requestPledgeSetMemberPowerGrade))
	register(reg, clientpackets.OpcodeRequestPledgeWarList, clientpackets.DecodeRequestPledgeWarList, whenLive((*GameClientLink).requestPledgeWarList))
	register(reg, clientpackets.OpcodeRequestPledgeReorganizeMember, clientpackets.DecodeRequestPledgeReorganizeMember, whenLive((*GameClientLink).requestPledgeReorganizeMember))
	register(reg, clientpackets.OpcodeRequestPledgeSetAcademyMaster, clientpackets.DecodeRequestPledgeSetAcademyMaster, whenLive((*GameClientLink).requestPledgeSetAcademyMaster))
	register(reg, clientpackets.OpcodeRequestGetBossRecord, clientpackets.DecodeRequestGetBossRecord, (*GameClientLink).handleRequestGetBossRecord)
	register(reg, clientpackets.OpcodeRequestWriteHeroWords, clientpackets.DecodeRequestWriteHeroWords, whenLive((*GameClientLink).writeHeroWords))
	register(reg, clientpackets.OpcodeRequestExFishRanking, noPayload, whenLiveNoBody((*GameClientLink).requestFishRanking))
	register(reg, clientpackets.OpcodeRequestCursedWeaponList, noPayload, (*GameClientLink).handleRequestCursedWeaponList)
	register(reg, clientpackets.OpcodeRequestExMagicSkillUseGround, clientpackets.DecodeRequestExMagicSkillUseGround, whenLive((*GameClientLink).handleMagicSkillUseGround))
	register(reg, clientpackets.OpcodeRequestConfirmTargetItem, clientpackets.DecodeRequestConfirmTargetItem, whenLive((*GameClientLink).confirmAugmentTarget))
	register(reg, clientpackets.OpcodeRequestConfirmRefinerItem, clientpackets.DecodeRequestConfirmRefinerItem, whenLive((*GameClientLink).confirmAugmentRefiner))
	register(reg, clientpackets.OpcodeRequestConfirmGemStone, clientpackets.DecodeRequestConfirmGemStone, whenLive((*GameClientLink).confirmAugmentGemstone))
	register(reg, clientpackets.OpcodeRequestRefine, clientpackets.DecodeRequestRefine, whenLive((*GameClientLink).refineAugment))
	register(reg, clientpackets.OpcodeRequestConfirmCancelItem, clientpackets.DecodeRequestConfirmCancelItem, whenLive((*GameClientLink).confirmAugmentCancel))
	register(reg, clientpackets.OpcodeRequestRefineCancel, clientpackets.DecodeRequestRefineCancel, whenLive((*GameClientLink).cancelAugment))
	register(reg, clientpackets.OpcodeRequestChangePartyLeader, clientpackets.DecodeRequestChangePartyLeader, whenLive((*GameClientLink).requestChangePartyLeader))
	register(reg, clientpackets.OpcodeRequestExAskJoinMPCC, clientpackets.DecodeRequestExAskJoinMPCC, whenLive((*GameClientLink).requestAskJoinChannel))
	register(reg, clientpackets.OpcodeRequestExAcceptJoinMPCC, clientpackets.DecodeRequestExAcceptJoinMPCC, whenLive((*GameClientLink).requestAcceptJoinChannel))
	register(reg, clientpackets.OpcodeRequestExOustFromMPCC, clientpackets.DecodeRequestExOustFromMPCC, whenLive((*GameClientLink).requestOustFromChannel))
	register(reg, clientpackets.OpcodeRequestExMPCCShowPartyMembersInfo, clientpackets.DecodeRequestExMPCCShowPartyMembersInfo, whenLive((*GameClientLink).requestChannelPartyMembers))
	register(reg, clientpackets.OpcodeRequestDuelStart, clientpackets.DecodeRequestDuelStart, whenLive((*GameClientLink).requestDuelStart))
	register(reg, clientpackets.OpcodeRequestDuelAnswerStart, clientpackets.DecodeRequestDuelAnswerStart, whenLive((*GameClientLink).requestDuelAnswerStart))
	register(reg, clientpackets.OpcodeRequestDuelSurrender, noPayload, (*GameClientLink).handleRequestDuelSurrender)
	register(reg, clientpackets.OpcodeRequestOustFromPartyRoom, clientpackets.DecodeRequestOustFromPartyRoom, whenLive((*GameClientLink).requestOustFromPartyRoom))
	register(reg, clientpackets.OpcodeRequestDismissPartyRoom, clientpackets.DecodeRequestDismissPartyRoom, whenLive((*GameClientLink).requestDismissPartyRoom))
	register(reg, clientpackets.OpcodeRequestWithdrawPartyRoom, clientpackets.DecodeRequestWithdrawPartyRoom, whenLive((*GameClientLink).requestWithdrawPartyRoom))
	register(reg, clientpackets.OpcodeRequestAskJoinPartyRoom, clientpackets.DecodeRequestAskJoinPartyRoom, whenLive((*GameClientLink).requestAskJoinPartyRoom))
	register(reg, clientpackets.OpcodeAnswerJoinPartyRoom, clientpackets.DecodeAnswerJoinPartyRoom, whenLive((*GameClientLink).answerJoinPartyRoom))
	register(reg, clientpackets.OpcodeRequestListPartyMatchingWaitingRoom, clientpackets.DecodeRequestListPartyMatchingWaitingRoom, whenLive((*GameClientLink).requestListWaitingPlayers))
	register(reg, clientpackets.OpcodeRequestExitPartyMatchingWaitingRoom, noPayload, (*GameClientLink).handleRequestExitPartyMatchingWaitingRoom)
	register(reg, clientpackets.OpcodeRequestCursedWeaponLocation, noPayload, whenLiveNoBody((*GameClientLink).sendCursedWeaponLocations))
	register(reg, clientpackets.OpcodeRequestOlympiadObserverEnd, noPayload, whenLiveNoBody((*GameClientLink).requestOlympiadObserverEnd))
	register(reg, clientpackets.OpcodeRequestOlympiadMatchList, noPayload, whenLiveNoBody((*GameClientLink).requestOlympiadMatchList))
	return reg
}

// register binds op to handle, behind the one decode policy every client
// frame goes through: a frame whose decode calls for a disconnect closes the
// connection, any other decode failure (already logged) drops the frame
// and reads on, and a decoded frame reaches handle. Registering op twice is a
// wiring mistake and panics.
func register[K inboundOpcode, T any](
	reg inboundRegistry[K], op K,
	decode func([]byte) (T, error),
	handle func(*GameClientLink, context.Context, *connState, T) error,
) {
	registerFrame(reg, op, func(l *GameClientLink, ctx context.Context, cs *connState, payload []byte) error {
		return decodeAndHandle(l, ctx, cs, payload, decode, handle)
	})
}

// registerCapped is register for a request carrying an item list, whose
// decoder refuses more rows than the configured per-packet cap.
func registerCapped[K inboundOpcode, T any](
	reg inboundRegistry[K], op K,
	decode func(payload []byte, maxItems int) (T, error),
	handle func(*GameClientLink, context.Context, *connState, T) error,
) {
	registerFrame(reg, op, cappedFrame(decode, handle))
}

// cappedFrame is the frame handler registerCapped binds.
func cappedFrame[T any](
	decode func(payload []byte, maxItems int) (T, error),
	handle func(*GameClientLink, context.Context, *connState, T) error,
) inboundFunc {
	return func(l *GameClientLink, ctx context.Context, cs *connState, payload []byte) error {
		maxItems := l.playerConfig().InventorySlots.MaxItemInPacket()
		return decodeAndHandle(l, ctx, cs, payload, func(p []byte) (T, error) { return decode(p, maxItems) }, handle)
	}
}

// registerFrame binds op to a handler that reads the raw frame itself.
// Registering op twice is a wiring mistake and panics.
func registerFrame[K inboundOpcode](reg inboundRegistry[K], op K, handle inboundFunc) {
	if _, dup := reg[op]; dup {
		panic(fmt.Sprintf("network: inbound opcode %#x registered twice", op))
	}
	reg[op] = handle
}

// decodeAndHandle applies register's decode policy to one frame.
func decodeAndHandle[T any](
	l *GameClientLink, ctx context.Context, cs *connState, payload []byte,
	decode func([]byte) (T, error),
	handle func(*GameClientLink, context.Context, *connState, T) error,
) error {
	req, err := decodeClientPacket(l, cs.client, payload, decode)
	if err != nil {
		if errors.Is(err, errMalformedPacketDisconnect) {
			return errDisconnect
		}
		return nil
	}
	return handle(l, ctx, cs, req)
}

// whenLive adapts handle to run on the queue of the player in the world.
// With no player in the world the decoded request is dropped.
func whenLive[T any](handle func(*GameClientLink, *livePlayer, T)) func(*GameClientLink, context.Context, *connState, T) error {
	return func(l *GameClientLink, _ context.Context, cs *connState, req T) error {
		if live := cs.live; live != nil {
			onLive(live, func() { handle(l, live, req) })
		}
		return nil
	}
}

// whenLiveNoBody is whenLive for a request that carries no fields.
func whenLiveNoBody(handle func(*GameClientLink, *livePlayer)) func(*GameClientLink, context.Context, *connState, struct{}) error {
	return whenLive(func(l *GameClientLink, live *livePlayer, _ struct{}) { handle(l, live) })
}

// serve runs the handler registered for op on payload. found reports that
// op has one; open reports whether the connection keeps reading.
func (reg inboundRegistry[K]) serve(l *GameClientLink, ctx context.Context, cs *connState, op K, payload []byte) (found, open bool) {
	handle, ok := reg[op]
	if !ok {
		return false, true
	}
	err := handle(l, ctx, cs, payload)
	switch {
	case err == nil:
		return true, true
	case errors.Is(err, errDisconnect):
		return true, false
	default:
		l.log.Warn().Err(err).Str("opcode", fmt.Sprintf("%#x", op)).Str("account", cs.client.AccountName()).
			Msg("game client handler")
		return true, true
	}
}
