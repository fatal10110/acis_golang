package npc

import "fmt"

// SkillType is the role a template's <skills> block assigns to one of its
// skills: the "type" attribute of a <skill> entry, one value per
// ';'-separated token. Behaviors pick the skill they cast by role
// (SkillTypeBuff, SkillTypeDDMagic, ...), not by skill id.
//
// The value order is significant: when a template grants one skill id
// under several roles, the lowest role wins the by-id lookup (Template.Skills).
type SkillType uint8

const (
	SkillTypePassive SkillType = iota
	SkillTypeAfflictSkill1
	SkillTypeAfflictSkill2
	SkillTypeAfflictSkill3
	SkillTypeArrowDefenceMode
	SkillTypeArrowNormalMode
	SkillTypeBigBodySkill
	SkillTypeBomb
	SkillTypeBomber
	SkillTypeBuff
	SkillTypeBuff1
	SkillTypeBuff2
	SkillTypeBuff3
	SkillTypeBuff4
	SkillTypeBuff5
	SkillTypeBuff6
	SkillTypeBuffUltimateShield
	SkillTypeCancel
	SkillTypeCancelMagic
	SkillTypeCaptureCancelA
	SkillTypeCaptureCancelB
	SkillTypeCaptureCancelC
	SkillTypeCaptureCancelAll
	SkillTypeCheckMagic
	SkillTypeCheckMagic1
	SkillTypeCheckMagic2
	SkillTypeCheckSkill1
	SkillTypeCheckSkill2
	SkillTypeClanBuf1
	SkillTypeClanBuff1
	SkillTypeClearCorpse
	SkillTypeCrazyMode
	SkillTypeDDMagic
	SkillTypeDDMagic1
	SkillTypeDDMagic2
	SkillTypeDDMagic3
	SkillTypeDDMagicSlow
	SkillTypeDDMagicA
	SkillTypeDDPhysic1
	SkillTypeDDPhysic2
	SkillTypeDDPhysic3
	SkillTypeDebuff
	SkillTypeDebuffA
	SkillTypeDefenceMode
	SkillTypeDebuff1
	SkillTypeDebuff1Cancel
	SkillTypeDebuff2
	SkillTypeDebuff2Cancel
	SkillTypeDebuff3
	SkillTypeDebuffCancel
	SkillTypeDispell
	SkillTypeDebuff4
	SkillTypeDebuff5
	SkillTypeEffectSkill
	SkillTypeEffectSkill2
	SkillTypeFury
	SkillTypeHeal
	SkillTypeHeal1
	SkillTypeHeal2
	SkillTypeHealMagic
	SkillTypeHealMagicA
	SkillTypeHeroSkill
	SkillTypeHoldMagic
	SkillTypeLongRangeDDMagic1
	SkillTypeLongRangePhysicalSpecial
	SkillTypeLongRangePhysicalSpecialA
	SkillTypeMagicHeal
	SkillTypeMagicSleep
	SkillTypeMobHate
	SkillTypeNormalBodySkill
	SkillTypePhysicalSpecial
	SkillTypePhysicalSpecial1
	SkillTypePhysicalSpecial2
	SkillTypePhysicalSpecial3
	SkillTypePhysicalSpecialRange
	SkillTypePhysicalSpecialA
	SkillTypePhysicalSpecialB
	SkillTypeRangeBuff
	SkillTypeRangeDD
	SkillTypeRangeDDMagic1
	SkillTypeRangeDDMagicA
	SkillTypeRangeDebuff
	SkillTypeRangeHoldA
	SkillTypeRangePhysicalSpecial
	SkillTypeSelfBuff
	SkillTypeSelfBuff1
	SkillTypeSelfBuff2
	SkillTypeSelfBuff3
	SkillTypeSelfBuff4
	SkillTypeSelfBuffA
	SkillTypeSelfDebuff1
	SkillTypeSelfDebuff2
	SkillTypeSelfDebuff3
	SkillTypeSelfExplosion
	SkillTypeSelfMagicHeal
	SkillTypeSelfRangeBuff
	SkillTypeSelfRangeBuff1
	SkillTypeSelfRangeBuffA
	SkillTypeSelfRangeCancelA
	SkillTypeSelfRangeCancelA1
	SkillTypeSelfRangeCancelA2
	SkillTypeSelfRangeDDMagic
	SkillTypeSelfRangeDDMagic1
	SkillTypeSelfRangeDDMagic2
	SkillTypeSelfRangeDDMagic3
	SkillTypeSelfRangeDebuff
	SkillTypeSelfRangeDebuff1
	SkillTypeSelfRangeDebuffAnotherA
	SkillTypeSelfRangeDebuffA
	SkillTypeSelfRangePhysicalSpecial
	SkillTypeSelfRangePhysicalSpecialA
	SkillTypeSetCurse
	SkillTypeSkill01ID
	SkillTypeSkill02ID
	SkillTypeSkill03ID
	SkillTypeSkill04ID
	SkillTypeSkill05ID
	SkillTypeSkill06ID
	SkillTypeSleepMagic
	SkillTypeSpecialAttack
	SkillTypeSpecialSkill
	SkillTypeStatusEffect
	SkillTypeSummonEffect
	SkillTypeSummonHeal1
	SkillTypeSummonHeal2
	SkillTypeSummonMagic
	SkillTypeSummonMode
	SkillTypeTeleportEffect
	SkillTypeWeakModeFalse
	SkillTypeWeakModeTrue
	SkillTypeWClanBuff
	SkillTypeWFiendArcher
	SkillTypeWLongRangeDDMagic
	SkillTypeWLongRangeDDMagic1
	SkillTypeWLongRangeDDMagic2
	SkillTypeWMiddleRangeDDMagic
	SkillTypeWRangeDebuff
	SkillTypeWRangeHeal
	SkillTypeWSelfRangeDDMagic
	SkillTypeWSelfRangeDebuff
	SkillTypeWShortRangeDDMagic

	skillTypeCount
)

// skillTypeNames maps each SkillType to its datapack token.
var skillTypeNames = [skillTypeCount]string{
	SkillTypePassive:                   "PASSIVE",
	SkillTypeAfflictSkill1:             "AFFLICT_SKILL1",
	SkillTypeAfflictSkill2:             "AFFLICT_SKILL2",
	SkillTypeAfflictSkill3:             "AFFLICT_SKILL3",
	SkillTypeArrowDefenceMode:          "ARROW_DEFENCE_MODE",
	SkillTypeArrowNormalMode:           "ARROW_NORMAL_MODE",
	SkillTypeBigBodySkill:              "BIG_BODY_SKILL",
	SkillTypeBomb:                      "BOMB",
	SkillTypeBomber:                    "BOMBER",
	SkillTypeBuff:                      "BUFF",
	SkillTypeBuff1:                     "BUFF1",
	SkillTypeBuff2:                     "BUFF2",
	SkillTypeBuff3:                     "BUFF3",
	SkillTypeBuff4:                     "BUFF4",
	SkillTypeBuff5:                     "BUFF5",
	SkillTypeBuff6:                     "BUFF6",
	SkillTypeBuffUltimateShield:        "BUFF_ULTIMATE_SHIELD",
	SkillTypeCancel:                    "CANCEL",
	SkillTypeCancelMagic:               "CANCEL_MAGIC",
	SkillTypeCaptureCancelA:            "CAPTURE_CANCEL_A",
	SkillTypeCaptureCancelB:            "CAPTURE_CANCEL_B",
	SkillTypeCaptureCancelC:            "CAPTURE_CANCEL_C",
	SkillTypeCaptureCancelAll:          "CAPTURE_CANCEL_ALL",
	SkillTypeCheckMagic:                "CHECK_MAGIC",
	SkillTypeCheckMagic1:               "CHECK_MAGIC1",
	SkillTypeCheckMagic2:               "CHECK_MAGIC2",
	SkillTypeCheckSkill1:               "CHECK_SKILL1",
	SkillTypeCheckSkill2:               "CHECK_SKILL2",
	SkillTypeClanBuf1:                  "CLAN_BUF1",
	SkillTypeClanBuff1:                 "CLAN_BUFF1",
	SkillTypeClearCorpse:               "CLEAR_CORPSE",
	SkillTypeCrazyMode:                 "CRAZY_MODE",
	SkillTypeDDMagic:                   "DD_MAGIC",
	SkillTypeDDMagic1:                  "DD_MAGIC1",
	SkillTypeDDMagic2:                  "DD_MAGIC2",
	SkillTypeDDMagic3:                  "DD_MAGIC3",
	SkillTypeDDMagicSlow:               "DD_MAGIC_SLOW",
	SkillTypeDDMagicA:                  "DD_MAGIC_A",
	SkillTypeDDPhysic1:                 "DD_PHYSIC1",
	SkillTypeDDPhysic2:                 "DD_PHYSIC2",
	SkillTypeDDPhysic3:                 "DD_PHYSIC3",
	SkillTypeDebuff:                    "DEBUFF",
	SkillTypeDebuffA:                   "DEBUFF_A",
	SkillTypeDefenceMode:               "DEFENCE_MODE",
	SkillTypeDebuff1:                   "DEBUFF1",
	SkillTypeDebuff1Cancel:             "DEBUFF1_CANCEL",
	SkillTypeDebuff2:                   "DEBUFF2",
	SkillTypeDebuff2Cancel:             "DEBUFF2_CANCEL",
	SkillTypeDebuff3:                   "DEBUFF3",
	SkillTypeDebuffCancel:              "DEBUFF_CANCEL",
	SkillTypeDispell:                   "DISPELL",
	SkillTypeDebuff4:                   "DEBUFF4",
	SkillTypeDebuff5:                   "DEBUFF5",
	SkillTypeEffectSkill:               "EFFECT_SKILL",
	SkillTypeEffectSkill2:              "EFFECT_SKILL2",
	SkillTypeFury:                      "FURY",
	SkillTypeHeal:                      "HEAL",
	SkillTypeHeal1:                     "HEAL1",
	SkillTypeHeal2:                     "HEAL2",
	SkillTypeHealMagic:                 "HEAL_MAGIC",
	SkillTypeHealMagicA:                "HEAL_MAGIC_A",
	SkillTypeHeroSkill:                 "HERO_SKILL",
	SkillTypeHoldMagic:                 "HOLD_MAGIC",
	SkillTypeLongRangeDDMagic1:         "LONG_RANGE_DD_MAGIC1",
	SkillTypeLongRangePhysicalSpecial:  "LONG_RANGE_PHYSICAL_SPECIAL",
	SkillTypeLongRangePhysicalSpecialA: "LONG_RANGE_PHYSICAL_SPECIAL_A",
	SkillTypeMagicHeal:                 "MAGIC_HEAL",
	SkillTypeMagicSleep:                "MAGIC_SLEEP",
	SkillTypeMobHate:                   "MOB_HATE",
	SkillTypeNormalBodySkill:           "NORMAL_BODY_SKILL",
	SkillTypePhysicalSpecial:           "PHYSICAL_SPECIAL",
	SkillTypePhysicalSpecial1:          "PHYSICAL_SPECIAL1",
	SkillTypePhysicalSpecial2:          "PHYSICAL_SPECIAL2",
	SkillTypePhysicalSpecial3:          "PHYSICAL_SPECIAL3",
	SkillTypePhysicalSpecialRange:      "PHYSICAL_SPECIAL_RANGE",
	SkillTypePhysicalSpecialA:          "PHYSICAL_SPECIAL_A",
	SkillTypePhysicalSpecialB:          "PHYSICAL_SPECIAL_B",
	SkillTypeRangeBuff:                 "RANGE_BUFF",
	SkillTypeRangeDD:                   "RANGE_DD",
	SkillTypeRangeDDMagic1:             "RANGE_DD_MAGIC1",
	SkillTypeRangeDDMagicA:             "RANGE_DD_MAGIC_A",
	SkillTypeRangeDebuff:               "RANGE_DEBUFF",
	SkillTypeRangeHoldA:                "RANGE_HOLD_A",
	SkillTypeRangePhysicalSpecial:      "RANGE_PHYSICAL_SPECIAL",
	SkillTypeSelfBuff:                  "SELF_BUFF",
	SkillTypeSelfBuff1:                 "SELF_BUFF1",
	SkillTypeSelfBuff2:                 "SELF_BUFF2",
	SkillTypeSelfBuff3:                 "SELF_BUFF3",
	SkillTypeSelfBuff4:                 "SELF_BUFF4",
	SkillTypeSelfBuffA:                 "SELF_BUFF_A",
	SkillTypeSelfDebuff1:               "SELF_DEBUFF1",
	SkillTypeSelfDebuff2:               "SELF_DEBUFF2",
	SkillTypeSelfDebuff3:               "SELF_DEBUFF3",
	SkillTypeSelfExplosion:             "SELF_EXPLOSION",
	SkillTypeSelfMagicHeal:             "SELF_MAGIC_HEAL",
	SkillTypeSelfRangeBuff:             "SELF_RANGE_BUFF",
	SkillTypeSelfRangeBuff1:            "SELF_RANGE_BUFF1",
	SkillTypeSelfRangeBuffA:            "SELF_RANGE_BUFF_A",
	SkillTypeSelfRangeCancelA:          "SELF_RANGE_CANCEL_A",
	SkillTypeSelfRangeCancelA1:         "SELF_RANGE_CANCEL_A1",
	SkillTypeSelfRangeCancelA2:         "SELF_RANGE_CANCEL_A2",
	SkillTypeSelfRangeDDMagic:          "SELF_RANGE_DD_MAGIC",
	SkillTypeSelfRangeDDMagic1:         "SELF_RANGE_DD_MAGIC1",
	SkillTypeSelfRangeDDMagic2:         "SELF_RANGE_DD_MAGIC2",
	SkillTypeSelfRangeDDMagic3:         "SELF_RANGE_DD_MAGIC3",
	SkillTypeSelfRangeDebuff:           "SELF_RANGE_DEBUFF",
	SkillTypeSelfRangeDebuff1:          "SELF_RANGE_DEBUFF1",
	SkillTypeSelfRangeDebuffAnotherA:   "SELF_RANGE_DEBUFF_ANOTHER_A",
	SkillTypeSelfRangeDebuffA:          "SELF_RANGE_DEBUFF_A",
	SkillTypeSelfRangePhysicalSpecial:  "SELF_RANGE_PHYSICAL_SPECIAL",
	SkillTypeSelfRangePhysicalSpecialA: "SELF_RANGE_PHYSICAL_SPECIAL_A",
	SkillTypeSetCurse:                  "SET_CURSE",
	SkillTypeSkill01ID:                 "SKILL01_ID",
	SkillTypeSkill02ID:                 "SKILL02_ID",
	SkillTypeSkill03ID:                 "SKILL03_ID",
	SkillTypeSkill04ID:                 "SKILL04_ID",
	SkillTypeSkill05ID:                 "SKILL05_ID",
	SkillTypeSkill06ID:                 "SKILL06_ID",
	SkillTypeSleepMagic:                "SLEEP_MAGIC",
	SkillTypeSpecialAttack:             "SPECIAL_ATTACK",
	SkillTypeSpecialSkill:              "SPECIAL_SKILL",
	SkillTypeStatusEffect:              "STATUS_EFFECT",
	SkillTypeSummonEffect:              "SUMMON_EFFECT",
	SkillTypeSummonHeal1:               "SUMMON_HEAL1",
	SkillTypeSummonHeal2:               "SUMMON_HEAL2",
	SkillTypeSummonMagic:               "SUMMON_MAGIC",
	SkillTypeSummonMode:                "SUMMON_MODE",
	SkillTypeTeleportEffect:            "TELEPORT_EFFECT",
	SkillTypeWeakModeFalse:             "WEAK_MODE_FALSE",
	SkillTypeWeakModeTrue:              "WEAK_MODE_TRUE",
	SkillTypeWClanBuff:                 "W_CLAN_BUFF",
	SkillTypeWFiendArcher:              "W_FIEND_ARCHER",
	SkillTypeWLongRangeDDMagic:         "W_LONG_RANGE_DD_MAGIC",
	SkillTypeWLongRangeDDMagic1:        "W_LONG_RANGE_DD_MAGIC1",
	SkillTypeWLongRangeDDMagic2:        "W_LONG_RANGE_DD_MAGIC2",
	SkillTypeWMiddleRangeDDMagic:       "W_MIDDLE_RANGE_DD_MAGIC",
	SkillTypeWRangeDebuff:              "W_RANGE_DEBUFF",
	SkillTypeWRangeHeal:                "W_RANGE_HEAL",
	SkillTypeWSelfRangeDDMagic:         "W_SELF_RANGE_DD_MAGIC",
	SkillTypeWSelfRangeDebuff:          "W_SELF_RANGE_DEBUFF",
	SkillTypeWShortRangeDDMagic:        "W_SHORT_RANGE_DD_MAGIC",
}

// skillTypesByName is the inverse of skillTypeNames.
var skillTypesByName = func() map[string]SkillType {
	m := make(map[string]SkillType, len(skillTypeNames))
	for i, name := range skillTypeNames {
		m[name] = SkillType(i)
	}
	return m
}()

// ParseSkillType returns the SkillType a datapack token names, matched
// exactly (case-sensitive), or false for an unknown token.
func ParseSkillType(name string) (SkillType, bool) {
	t, ok := skillTypesByName[name]
	return t, ok
}

// String returns t's datapack token.
func (t SkillType) String() string {
	if t < skillTypeCount {
		return skillTypeNames[t]
	}
	return fmt.Sprintf("SkillType(%d)", uint8(t))
}
