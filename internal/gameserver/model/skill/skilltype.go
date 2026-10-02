package skill

// skillTypeNames is every skill classification tag a data file may name, as
// a level's skillType or an effect template's effectType.
var skillTypeNames = map[string]bool{
	// Damage
	"PDAM": true, "FATAL": true, "MDAM": true, "CPDAMPERCENT": true, "MANADAM": true,
	"DOT": true, "MDOT": true, "DRAIN_SOUL": true, "DRAIN": true, "DEATHLINK": true,
	"BLOW": true, "SIGNET": true, "SIGNET_CASTTIME": true, "SEED": true, "REAL_DAMAGE": true,
	// Disablers
	"BLEED": true, "POISON": true, "STUN": true, "ROOT": true, "CONFUSION": true,
	"FEAR": true, "SLEEP": true, "MUTE": true, "PARALYZE": true, "WEAKNESS": true,
	// HP, MP, CP
	"HEAL": true, "MANAHEAL": true, "COMBATPOINTHEAL": true, "HOT": true, "MPHOT": true,
	"BALANCE_LIFE": true, "HEAL_STATIC": true, "MANARECHARGE": true, "HEAL_PERCENT": true,
	"MANAHEAL_PERCENT": true, "GIVE_SP": true,
	// Aggro
	"AGGDAMAGE": true, "AGGREDUCE": true, "AGGREMOVE": true, "AGGREDUCE_CHAR": true, "AGGDEBUFF": true,
	// Fishing
	"FISHING": true, "PUMPING": true, "REELING": true,
	// Misc
	"UNLOCK": true, "UNLOCK_SPECIAL": true, "DELUXE_KEY_UNLOCK": true, "ENCHANT_ARMOR": true,
	"ENCHANT_WEAPON": true, "SOULSHOT": true, "SPIRITSHOT": true, "SIEGE_FLAG": true,
	"TAKE_CASTLE": true, "SOW": true, "HARVEST": true, "GET_PLAYER": true, "DUMMY": true,
	"INSTANT_JUMP": true,
	// Creation
	"COMMON_CRAFT": true, "DWARVEN_CRAFT": true, "CREATE_ITEM": true, "EXTRACTABLE": true,
	"EXTRACTABLE_FISH": true,
	// Summons
	"SUMMON": true, "FEED_PET": true, "STRIDER_SIEGE_ASSAULT": true, "ERASE": true,
	"BETRAY": true, "SPAWN": true,
	// Cancel
	"CANCEL": true, "MAGE_BANE": true, "WARRIOR_BANE": true, "NEGATE": true, "CANCEL_DEBUFF": true,
	"BUFF": true, "DEBUFF": true, "PASSIVE": true, "CONT": true, "RESURRECT": true,
	"CHARGEDAM": true, "LUCK": true, "RECALL": true, "TELEPORT": true, "SUMMON_FRIEND": true,
	"SUMMON_PARTY": true, "SUMMON_CREATURE": true, "REFLECT": true, "SPOIL": true, "SWEEP": true,
	"FAKE_DEATH": true, "BEAST_FEED": true, "FUSION": true, "CHANGE_APPEARANCE": true,
	// Skill is done within the core.
	"COREDONE": true,
	// Unimplemented
	"NOTDONE": true,
}

// KnownSkillType reports whether name is a skill classification tag. The
// match is exact: tags are written in upper case.
func KnownSkillType(name string) bool {
	return skillTypeNames[name]
}
