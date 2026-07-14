package clientpackets

// Wire-safe in-game packet opcodes for M4/M5 systems whose full behavior is
// still being ported. The dispatcher accepts these so the client is not
// disconnected while movement, combat, and item systems are incomplete.
const (
	OpcodeAction                  = 0x04
	OpcodeAttackRequest           = 0x0a
	OpcodeRequestItemList         = 0x0f
	OpcodeRequestDropItem         = 0x12
	OpcodeTradeRequest            = 0x15
	OpcodeAddTradeItem            = 0x16
	OpcodeTradeDone               = 0x17
	OpcodeDummy1A                 = 0x1a
	OpcodeRequestSocialAction     = 0x1b
	OpcodeRequestChangeMoveType   = 0x1c
	OpcodeRequestChangeWaitType   = 0x1d
	OpcodeRequestSellItem         = 0x1e
	OpcodeRequestBuyItem          = 0x1f
	OpcodeDummy23                 = 0x23
	OpcodeDummy2E                 = 0x2e
	OpcodeRequestMagicSkillUse    = 0x2f
	OpcodeAppearing               = 0x30
	OpcodeSendWarehouseDeposit    = 0x31
	OpcodeSendWarehouseWithdraw   = 0x32
	OpcodeRequestShortCutReg      = 0x33
	OpcodeDummy34                 = 0x34
	OpcodeRequestShortCutDel      = 0x35
	OpcodeCannotMoveAnymore       = 0x36
	OpcodeRequestTargetCancel     = 0x37
	OpcodeDummy3E                 = 0x3e
	OpcodeRequestSkillList        = 0x3f
	OpcodeRequestGetOnVehicle     = 0x42
	OpcodeRequestGetOffVehicle    = 0x43
	OpcodeAnswerTradeRequest      = 0x44
	OpcodeRequestActionUse        = 0x45
	OpcodeRequestRestart          = 0x46
	OpcodeRequestEnchantItem      = 0x58
	OpcodeStartRotating           = 0x4a
	OpcodeFinishRotating          = 0x4b
	OpcodeRequestDestroyItem      = 0x59
	OpcodeRequestMoveInVehicle    = 0x5c
	OpcodeCannotMoveInVehicle     = 0x5d
	OpcodeRequestQuestListInGame  = 0x63
	OpcodeRequestQuestAbort       = 0x64
	OpcodeRequestAcquireSkillInfo = 0x6b
	OpcodeRequestAcquireSkill     = 0x6c
	OpcodeRequestRestartPoint     = 0x6d
	OpcodeRequestCrystallizeItem  = 0x72
	OpcodeRequestChangePetName    = 0x89
	OpcodeRequestPetUseItem       = 0x8a
	OpcodeRequestGiveItemToPet    = 0x8b
	OpcodeRequestGetItemFromPet   = 0x8c
	OpcodeRequestPetGetItem       = 0x8f
	OpcodeSendTimeCheck           = 0x97
	OpcodeRequestPackageItemList  = 0x9e
	OpcodeRequestPackageSend      = 0x9f
	OpcodeDlgAnswer               = 0xc5
	OpcodeGameGuardReply          = 0xca
	OpcodeRequestShowMiniMap      = 0xcd
)
