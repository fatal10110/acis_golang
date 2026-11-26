package skill

// SkillType is a skill classification tag: a level's skillType or an
// effect template's effectType. The loader accepts only the tags below
// (see ParseSkillType), written in upper case; Go code compares through
// these constants.
type SkillType string

// The skill classification tags.
const (
	// Damage
	SkillTypePDam           SkillType = "PDAM"
	SkillTypeFatal          SkillType = "FATAL"
	SkillTypeMDam           SkillType = "MDAM"
	SkillTypeCPDamPercent   SkillType = "CPDAMPERCENT"
	SkillTypeManaDam        SkillType = "MANADAM"
	SkillTypeDOT            SkillType = "DOT"
	SkillTypeMDOT           SkillType = "MDOT"
	SkillTypeDrainSoul      SkillType = "DRAIN_SOUL"
	SkillTypeDrain          SkillType = "DRAIN"
	SkillTypeDeathLink      SkillType = "DEATHLINK"
	SkillTypeBlow           SkillType = "BLOW"
	SkillTypeSignet         SkillType = "SIGNET"
	SkillTypeSignetCastTime SkillType = "SIGNET_CASTTIME"
	SkillTypeSeed           SkillType = "SEED"
	SkillTypeRealDamage     SkillType = "REAL_DAMAGE"

	// Disablers
	SkillTypeBleed     SkillType = "BLEED"
	SkillTypePoison    SkillType = "POISON"
	SkillTypeStun      SkillType = "STUN"
	SkillTypeRoot      SkillType = "ROOT"
	SkillTypeConfusion SkillType = "CONFUSION"
	SkillTypeFear      SkillType = "FEAR"
	SkillTypeSleep     SkillType = "SLEEP"
	SkillTypeMute      SkillType = "MUTE"
	SkillTypeParalyze  SkillType = "PARALYZE"
	SkillTypeWeakness  SkillType = "WEAKNESS"

	// HP, MP and CP
	SkillTypeHeal            SkillType = "HEAL"
	SkillTypeManaHeal        SkillType = "MANAHEAL"
	SkillTypeCombatPointHeal SkillType = "COMBATPOINTHEAL"
	SkillTypeHOT             SkillType = "HOT"
	SkillTypeMPHOT           SkillType = "MPHOT"
	SkillTypeBalanceLife     SkillType = "BALANCE_LIFE"
	SkillTypeHealStatic      SkillType = "HEAL_STATIC"
	SkillTypeManaRecharge    SkillType = "MANARECHARGE"
	SkillTypeHealPercent     SkillType = "HEAL_PERCENT"
	SkillTypeManaHealPercent SkillType = "MANAHEAL_PERCENT"
	SkillTypeGiveSP          SkillType = "GIVE_SP"

	// Aggro
	SkillTypeAggDamage     SkillType = "AGGDAMAGE"
	SkillTypeAggReduce     SkillType = "AGGREDUCE"
	SkillTypeAggRemove     SkillType = "AGGREMOVE"
	SkillTypeAggReduceChar SkillType = "AGGREDUCE_CHAR"
	SkillTypeAggDebuff     SkillType = "AGGDEBUFF"

	// Fishing
	SkillTypeFishing SkillType = "FISHING"
	SkillTypePumping SkillType = "PUMPING"
	SkillTypeReeling SkillType = "REELING"

	// Misc
	SkillTypeUnlock          SkillType = "UNLOCK"
	SkillTypeUnlockSpecial   SkillType = "UNLOCK_SPECIAL"
	SkillTypeDeluxeKeyUnlock SkillType = "DELUXE_KEY_UNLOCK"
	SkillTypeEnchantArmor    SkillType = "ENCHANT_ARMOR"
	SkillTypeEnchantWeapon   SkillType = "ENCHANT_WEAPON"
	SkillTypeSoulshot        SkillType = "SOULSHOT"
	SkillTypeSpiritshot      SkillType = "SPIRITSHOT"
	SkillTypeSiegeFlag       SkillType = "SIEGE_FLAG"
	SkillTypeTakeCastle      SkillType = "TAKE_CASTLE"
	SkillTypeSow             SkillType = "SOW"
	SkillTypeHarvest         SkillType = "HARVEST"
	SkillTypeGetPlayer       SkillType = "GET_PLAYER"
	SkillTypeDummy           SkillType = "DUMMY"
	SkillTypeInstantJump     SkillType = "INSTANT_JUMP"

	// Creation
	SkillTypeCommonCraft     SkillType = "COMMON_CRAFT"
	SkillTypeDwarvenCraft    SkillType = "DWARVEN_CRAFT"
	SkillTypeCreateItem      SkillType = "CREATE_ITEM"
	SkillTypeExtractable     SkillType = "EXTRACTABLE"
	SkillTypeExtractableFish SkillType = "EXTRACTABLE_FISH"

	// Summons
	SkillTypeSummon              SkillType = "SUMMON"
	SkillTypeFeedPet             SkillType = "FEED_PET"
	SkillTypeStriderSiegeAssault SkillType = "STRIDER_SIEGE_ASSAULT"
	SkillTypeErase               SkillType = "ERASE"
	SkillTypeBetray              SkillType = "BETRAY"
	SkillTypeSpawn               SkillType = "SPAWN"

	// Cancel
	SkillTypeCancel       SkillType = "CANCEL"
	SkillTypeMageBane     SkillType = "MAGE_BANE"
	SkillTypeWarriorBane  SkillType = "WARRIOR_BANE"
	SkillTypeNegate       SkillType = "NEGATE"
	SkillTypeCancelDebuff SkillType = "CANCEL_DEBUFF"

	// Other
	SkillTypeBuff             SkillType = "BUFF"
	SkillTypeDebuff           SkillType = "DEBUFF"
	SkillTypePassive          SkillType = "PASSIVE"
	SkillTypeCont             SkillType = "CONT"
	SkillTypeResurrect        SkillType = "RESURRECT"
	SkillTypeChargeDam        SkillType = "CHARGEDAM"
	SkillTypeLuck             SkillType = "LUCK"
	SkillTypeRecall           SkillType = "RECALL"
	SkillTypeTeleport         SkillType = "TELEPORT"
	SkillTypeSummonFriend     SkillType = "SUMMON_FRIEND"
	SkillTypeSummonParty      SkillType = "SUMMON_PARTY"
	SkillTypeSummonCreature   SkillType = "SUMMON_CREATURE"
	SkillTypeReflect          SkillType = "REFLECT"
	SkillTypeSpoil            SkillType = "SPOIL"
	SkillTypeSweep            SkillType = "SWEEP"
	SkillTypeFakeDeath        SkillType = "FAKE_DEATH"
	SkillTypeBeastFeed        SkillType = "BEAST_FEED"
	SkillTypeFusion           SkillType = "FUSION"
	SkillTypeChangeAppearance SkillType = "CHANGE_APPEARANCE"

	// COREDONE: the skill is done within the core; NOTDONE: unimplemented.
	SkillTypeCoreDone SkillType = "COREDONE"
	SkillTypeNotDone  SkillType = "NOTDONE"
)

// skillTypes is every tag ParseSkillType accepts.
var skillTypes = map[SkillType]bool{
	SkillTypePDam:                true,
	SkillTypeFatal:               true,
	SkillTypeMDam:                true,
	SkillTypeCPDamPercent:        true,
	SkillTypeManaDam:             true,
	SkillTypeDOT:                 true,
	SkillTypeMDOT:                true,
	SkillTypeDrainSoul:           true,
	SkillTypeDrain:               true,
	SkillTypeDeathLink:           true,
	SkillTypeBlow:                true,
	SkillTypeSignet:              true,
	SkillTypeSignetCastTime:      true,
	SkillTypeSeed:                true,
	SkillTypeRealDamage:          true,
	SkillTypeBleed:               true,
	SkillTypePoison:              true,
	SkillTypeStun:                true,
	SkillTypeRoot:                true,
	SkillTypeConfusion:           true,
	SkillTypeFear:                true,
	SkillTypeSleep:               true,
	SkillTypeMute:                true,
	SkillTypeParalyze:            true,
	SkillTypeWeakness:            true,
	SkillTypeHeal:                true,
	SkillTypeManaHeal:            true,
	SkillTypeCombatPointHeal:     true,
	SkillTypeHOT:                 true,
	SkillTypeMPHOT:               true,
	SkillTypeBalanceLife:         true,
	SkillTypeHealStatic:          true,
	SkillTypeManaRecharge:        true,
	SkillTypeHealPercent:         true,
	SkillTypeManaHealPercent:     true,
	SkillTypeGiveSP:              true,
	SkillTypeAggDamage:           true,
	SkillTypeAggReduce:           true,
	SkillTypeAggRemove:           true,
	SkillTypeAggReduceChar:       true,
	SkillTypeAggDebuff:           true,
	SkillTypeFishing:             true,
	SkillTypePumping:             true,
	SkillTypeReeling:             true,
	SkillTypeUnlock:              true,
	SkillTypeUnlockSpecial:       true,
	SkillTypeDeluxeKeyUnlock:     true,
	SkillTypeEnchantArmor:        true,
	SkillTypeEnchantWeapon:       true,
	SkillTypeSoulshot:            true,
	SkillTypeSpiritshot:          true,
	SkillTypeSiegeFlag:           true,
	SkillTypeTakeCastle:          true,
	SkillTypeSow:                 true,
	SkillTypeHarvest:             true,
	SkillTypeGetPlayer:           true,
	SkillTypeDummy:               true,
	SkillTypeInstantJump:         true,
	SkillTypeCommonCraft:         true,
	SkillTypeDwarvenCraft:        true,
	SkillTypeCreateItem:          true,
	SkillTypeExtractable:         true,
	SkillTypeExtractableFish:     true,
	SkillTypeSummon:              true,
	SkillTypeFeedPet:             true,
	SkillTypeStriderSiegeAssault: true,
	SkillTypeErase:               true,
	SkillTypeBetray:              true,
	SkillTypeSpawn:               true,
	SkillTypeCancel:              true,
	SkillTypeMageBane:            true,
	SkillTypeWarriorBane:         true,
	SkillTypeNegate:              true,
	SkillTypeCancelDebuff:        true,
	SkillTypeBuff:                true,
	SkillTypeDebuff:              true,
	SkillTypePassive:             true,
	SkillTypeCont:                true,
	SkillTypeResurrect:           true,
	SkillTypeChargeDam:           true,
	SkillTypeLuck:                true,
	SkillTypeRecall:              true,
	SkillTypeTeleport:            true,
	SkillTypeSummonFriend:        true,
	SkillTypeSummonParty:         true,
	SkillTypeSummonCreature:      true,
	SkillTypeReflect:             true,
	SkillTypeSpoil:               true,
	SkillTypeSweep:               true,
	SkillTypeFakeDeath:           true,
	SkillTypeBeastFeed:           true,
	SkillTypeFusion:              true,
	SkillTypeChangeAppearance:    true,
	SkillTypeCoreDone:            true,
	SkillTypeNotDone:             true,
}

// ParseSkillType returns the tag name spells, and false when name is no
// skill classification tag. The match is exact: tags are written in upper
// case.
func ParseSkillType(name string) (SkillType, bool) {
	t := SkillType(name)
	return t, skillTypes[t]
}

// KnownSkillType reports whether name is a skill classification tag (see
// ParseSkillType).
func KnownSkillType(name string) bool {
	_, ok := ParseSkillType(name)
	return ok
}
