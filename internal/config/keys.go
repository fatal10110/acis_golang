package config

import (
	"regexp"
	"slices"

	"github.com/rs/zerolog"
)

// Server names a server binary, whose properties files hold the keys it
// reads.
type Server int

const (
	// GameServer reads clans, events, geoengine, npcs, players, siege and
	// server.properties.
	GameServer Server = iota
	// LoginServer reads loginserver.properties.
	LoginServer
)

// keySet is how a server treats the keys of one properties file.
type keySet struct {
	// Read are the keys the server reads.
	Read []string
	// Ignored are keys the server accepts but that change nothing: logging
	// switches for diagnostics it does not have, and settings with no
	// effect.
	Ignored []string
}

// regionKey is a geoengine.properties geodata region, "X_Y": the server
// reads every region of its tile range by that built name.
var regionKey = regexp.MustCompile(`^[0-9]+_[0-9]+$`)

// UnreadKeys returns, sorted, the keys of p, the properties file named
// file, that server does not read: unsupported ones, whose value is lost,
// and ignored ones, which change nothing.
func UnreadKeys(server Server, file string, p *Properties) (unsupported, ignored []string) {
	set := serverKeys(server)[file]
	for _, key := range p.Keys() {
		switch {
		case slices.Contains(set.Read, key), file == "geoengine.properties" && regionKey.MatchString(key):
		case slices.Contains(set.Ignored, key):
			ignored = append(ignored, key)
		default:
			unsupported = append(unsupported, key)
		}
	}
	return unsupported, ignored
}

// ReportUnreadKeys logs, once per key, each key of p, the properties file
// named file, that server does not read: a warning for an unsupported key,
// an info line for an ignored one.
func ReportUnreadKeys(log zerolog.Logger, server Server, file string, p *Properties) {
	unsupported, ignored := UnreadKeys(server, file, p)
	for _, key := range unsupported {
		log.Warn().Str("file", file).Str("key", key).Msg("config key not supported; its value is ignored")
	}
	for _, key := range ignored {
		log.Info().Str("file", file).Str("key", key).Msg("config key has no effect on this server")
	}
}

// serverKeys returns server's key sets by file.
func serverKeys(server Server) map[string]keySet {
	if server == LoginServer {
		return loginServerKeys
	}
	return gameServerKeys
}

// gameServerKeys and loginServerKeys are the key sets of each server's
// files. A key read through a new reader moves into Read; the tests hold
// each Read key to a reader in the server's code.
var gameServerKeys = map[string]keySet{
	"clans.properties": {
		Read: []string{
			"ClanMembersForWar", "ClanWarPenaltyWhenEnded", "DaysBeforeAcceptNewClanWhenDismissed",
			"DaysBeforeCreateAClan", "DaysBeforeCreateNewAllyWhenDissolved", "DaysBeforeJoinAClan",
			"DaysBeforeJoinAllyWhenDismissed", "DaysBeforeJoinAllyWhenLeaved",
			"DaysToPassToDissolveAClan", "ManorApproveMin", "ManorApproveTime",
			"ManorMaintenanceMin", "ManorRefreshMin", "ManorRefreshTime", "ManorSavePeriodRate",
			"MaxNumOfClansInAlly", "MembersCanWithdrawFromClanWH",
		},
	},
	"events.properties": {
		Read: []string{
			"AllowFishChampionship", "AnakazelPortChance", "AutoJumpsDelayMin", "AutoJumpsDelayRnd",
			"CaptainCost", "CommanderCost", "FestivalChestSpawn", "FestivalCycleLength",
			"FestivalFirstSpawn", "FestivalFirstSwarm", "FestivalLength", "FestivalManagerStart",
			"FestivalMinPlayer", "FestivalSecondSpawn", "FestivalSecondSwarm",
			"FishChampionshipReward1", "FishChampionshipReward2",
			"FishChampionshipReward3", "FishChampionshipReward4", "FishChampionshipReward5",
			"FishChampionshipRewardItemId", "HeroCost", "Lottery2and1NumberPrize", "Lottery3NumberRate",
			"Lottery4NumberRate", "Lottery5NumberRate", "LotteryPrize", "LotteryTicketPrice",
			"MaxPlayerContrib", "NeededPartyMembers", "OfficerCost", "OlyAnnounceGames", "OlyBattle", "OlyCPeriod",
			"OlyClassedParticipants", "OlyClassedReward", "OlyDividerClassed",
			"OlyDividerNonClassed", "OlyGPPerPoint", "OlyHeroPoints", "OlyMaxPoints", "OlyMin",
			"OlyMinMatchesToBeClassed", "OlyNonClassedParticipants", "OlyNonClassedReward",
			"OlyStartPoints", "OlyStartTime", "OlyWaitBattle", "OlyWaitEnd", "OlyWaitTime",
			"OlyWeeklyPoints", "RecruitCost", "RiftMinPartySize", "SevenSignsBypassPrerequisites",
			"SoldierCost",
		},
	},
	"geoengine.properties": {
		Read: []string{
			"GeoDataPath", "GeoDataType", "HeuristicWeight", "MaxGeopathFailCount",
			"MaxIterations", "MaxObstacleHeight", "MoveWeight", "MoveWeightDiag", "ObstacleWeight",
			"PartOfCharacterHeight",
		},
	},
	"npcs.properties": {
		Read: []string{
			"AllowEntireTree", "AntharasWaitTime", "BufferMaxSchemesPerChar",
			"BufferStaticCostPerBuff", "ConfigClassMaster", "DefaultSeeRange", "DisableRaidCurse",
			"FreeTeleport", "FrintezzaWaitTime", "GuardAttackAggroMob", "MaxDriftRange",
			"MobAggroInPeaceZone", "RaidDefenceMultiplier", "RaidHpRegenMultiplier",
			"RaidMpRegenMultiplier", "RandomWalkRate", "RequiredCrystalsNumber",
			"RequiredStriderLevel", "ShowNpcCrest", "ShowNpcLevel", "ShowSummonCrest",
			"SpawnEvents", "SpawnMultiplier", "ValakasWaitTime", "WeddingAllowSameSex",
			"WeddingFormalWear", "WeddingPrice",
		},
	},
	"players.properties": {
		Read: []string{
			"AllowDelevel", "AugmentationBaseStatChance", "AugmentationHighGlowChance",
			"AugmentationHighSkillChance", "AugmentationMidGlowChance",
			"AugmentationMidSkillChance", "AugmentationNGGlowChance", "AugmentationNGSkillChance",
			"AugmentationTopGlowChance", "AugmentationTopSkillChance", "AutoLearnSkills",
			"AwardPKKillPVPPoint", "BlacksmithUseRecipes", "CanGMDropEquipment",
			"CancelLesserEffect", "CommonRecipeLimit", "CpRegenMultiplier", "CraftingEnabled",
			"DeathPenaltyChance",
			"DefaultAccessLevel", "DivineInspirationSpBookNeeded", "DwarfRecipeLimit",
			"EnchantChanceArmor", "EnchantChanceMagicWeapon", "EnchantChanceMagicWeapon15Plus",
			"EnchantChanceNonMagicWeapon", "EnchantChanceNonMagicWeapon15Plus", "EnchantMaxArmor",
			"EnchantMaxWeapon", "EnchantSafeMax", "EnchantSafeMaxFull", "EnchantSkillSpBookNeeded",
			"FreightPrice", "GMHeroAura", "GMStartupAutoList", "GMStartupBlockAll",
			"GMStartupInvisible", "GMStartupInvulnerable", "HpRegenMultiplier", "KarmaPlayerCanShop",
			"KarmaPlayerCanTeleport", "KarmaPlayerCanTrade", "KarmaPlayerCanUseGK",
			"KarmaPlayerCanUseWareHouse", "LifeCrystalNeeded", "ListOfNonDroppableItemsForPK",
			"ListOfPetItems", "MagicFailures", "MaxBuffsAmount", "MaxPetitionsPending",
			"MaxPetitionsPerPlayer", "MaxPvtStoreSlotsDwarf", "MaxPvtStoreSlotsOther",
			"MaximumFreightSlots", "MaximumSlotsForDwarf", "MaximumSlotsForNoDwarf",
			"MaximumSlotsForPet", "MaximumWarehouseSlotsForClan", "MaximumWarehouseSlotsForDwarf",
			"MaximumWarehouseSlotsForNoDwarf", "MinimumPKRequiredToDrop", "MpRegenMultiplier",
			"PartyRange",
			"PartyXpCutoffLevel", "PartyXpCutoffMethod", "PartyXpCutoffPercent",
			"PerfectShieldBlockRate", "PetitioningAllowed", "PlayerFakeDeathUpProtection",
			"PlayerSpawnProtection", "PvPVsNormalTime", "PvPVsPvPTime", "RegionBasedFreight",
			"RespawnRestoreHP", "SpBookNeeded", "StoreSkillCooltime", "SubClassWithoutQuests",
			"UseDeepBlueDropRules", "WeightLimit",
		},
	},
	"server.properties": {
		Read: []string{
			"AcceptAlternateID", "AllowBoat", "AllowCursedWeapons", "AllowDiscardItem",
			"AllowFreight", "AllowLottery", "AllowManor", "AllowWarehouse", "AllowWater",
			"AllowWear", "AutoDestroyEquipableItemTime", "AutoDestroyHerbTime",
			"AutoDestroyItemTime", "AutoDestroySpecialItemTime", "AutoLoot", "AutoLootHerbs",
			"AutoLootRaid", "BBSDefault", "CharacterSelectTime", "DeleteCharAfterDays",
			"DropItemTime", "EnableCommunityBoard", "EnableFallingDamage", "GMAudit",
			"GameserverHostname", "GameserverPort", "GlobalChatTime", "HeroVoiceTime", "Hostname",
			"KarmaDropLimit", "KarmaRateDrop", "KarmaRateDropEquip", "KarmaRateDropEquipWeapon",
			"KarmaRateDropItem", "L2WalkerProtection", "LogChat", "LogItems", "Login", "LoginHost",
			"LoginPort", "ManorTime", "ManufactureTime", "MaximumOnlineUsers", "MultipleItemDrop",
			"MultisellTime", "NoSpawns", "Password", "PetFoodRate", "PetXpRate", "PlayerDropLimit",
			"PlayerDroppedItemMultiplier", "PlayerRateDrop", "PlayerRateDropEquip",
			"PlayerRateDropEquipWeapon", "PlayerRateDropItem", "PvpServer", "RateDropCurrency",
			"RateDropHerbs", "RateDropItems", "RateDropManor", "RateDropSpoil", "RateKarmaExpLost",
			"RatePartySp", "RatePartyXp", "RateQuestDrop", "RateQuestReward",
			"RateQuestRewardAdena", "RateQuestRewardSP", "RateQuestRewardXP", "RateRaidDropItems",
			"RateSiegeGuardsPrice", "RateSp", "RateXp", "RollDiceTime", "ServerBypassTime",
			"ServerGMOnly", "ServerListAgeLimit", "ServerListBrackets", "ServerListClock",
			"ShowServerNews", "SinEaterXpRate", "SocialTime", "SubclassTime", "TestServer",
			"TradeChatTime", "URL", "UseBlowfishCipher", "WearDelay", "WearPrice", "ZoneTown",
		},
		Ignored: []string{"Developer", "PacketHandlerDebug", "RequestServerID", "SendMailTime"},
	},
	"siege.properties": {
		Read: []string{
			"AttackerMaxClans", "AttackerRespawn", "ChAttackerMaxClans", "ChSiegeClanMinLevel",
			"DefenderMaxClans", "SiegeClanMinLevel", "SiegeLength",
		},
	},
}

var loginServerKeys = map[string]keySet{
	"loginserver.properties": {
		Read: []string{
			"AcceptNewGameServer", "AutoCreateAccounts", "EnableFloodProtection",
			"FastConnectionLimit", "FastConnectionTime", "Hostname", "Login", "LoginBlockAfterBan",
			"LoginHostname", "LoginPort", "LoginTryBeforeBan", "LoginserverHostname",
			"LoginserverPort", "MaxConnectionPerIP", "NormalConnectionTime", "Password",
			"ShowLicence", "URL",
		},
	},
}
