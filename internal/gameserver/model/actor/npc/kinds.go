package npc

// The instance kinds a template's type names, one per NPC behavior
// category.
const (
	KindAdventurer InstanceKind = "Adventurer"
	// KindAuctioneer is the clan hall auctioneer's type.
	KindAuctioneer            InstanceKind = "Auctioneer"
	KindBabyPet               InstanceKind = "BabyPet"
	KindCastleBlacksmith      InstanceKind = "CastleBlacksmith"
	KindCastleChamberlain     InstanceKind = "CastleChamberlain"
	KindCastleDoorman         InstanceKind = "CastleDoorman"
	KindCastleGatekeeper      InstanceKind = "CastleGatekeeper"
	KindCastleMagician        InstanceKind = "CastleMagician"
	KindCastleWarehouseKeeper InstanceKind = "CastleWarehouseKeeper"
	KindChest                 InstanceKind = "Chest"
	// KindChristmasTree is the kind of the decoration a Christmas tree item
	// places and an administrator spawns. A click on one is answered
	// ActionFailed and never selects it.
	KindChristmasTree   InstanceKind = "ChristmasTree"
	KindClanHallDoorman InstanceKind = "ClanHallDoorman"
	// KindClanHallManagerNpc is the clan hall manager's type.
	KindClanHallManagerNpc   InstanceKind = "ClanHallManagerNpc"
	KindClassMaster          InstanceKind = "ClassMaster"
	KindCubic                InstanceKind = "Cubic"
	KindDawnPriest           InstanceKind = "DawnPriest"
	KindDerbyTrackManagerNpc InstanceKind = "DerbyTrackManagerNpc"
	KindDoor                 InstanceKind = "Door"
	KindDoorman              InstanceKind = "Doorman"
	KindDungeonGatekeeper    InstanceKind = "DungeonGatekeeper"
	KindDuskPriest           InstanceKind = "DuskPriest"
	KindEffectPoint          InstanceKind = "EffectPoint"
	KindFeedableBeast        InstanceKind = "FeedableBeast"
	KindFence                InstanceKind = "Fence"
	KindFestivalGuide        InstanceKind = "FestivalGuide"
	KindFestivalMonster      InstanceKind = "FestivalMonster"
	KindFisherman            InstanceKind = "Fisherman"
	// KindFlameTower and KindLifeTower are the control tower types. A castle's
	// control towers stand where its castles.xml entry places them from boot
	// on, for the castle's whole life: dead outside a siege, shown with
	// ServerObjectInfo as their own template, and working while a siege is
	// fought, shown with NpcInfo as their working form. Only a player, or a
	// player's summon, of a clan attacking the castle's siege may attack a
	// working tower; brought down to 1 HP, it falls dead again. A player's
	// interact does nothing on a tower.
	KindFlameTower      InstanceKind = "FlameTower"
	KindFolk            InstanceKind = "Folk"
	KindFriendlyMonster InstanceKind = "FriendlyMonster"
	KindGatekeeper      InstanceKind = "Gatekeeper"
	KindGrandBoss       InstanceKind = "GrandBoss"
	KindGuard           InstanceKind = "Guard"
	KindHalishaChest    InstanceKind = "HalishaChest"
	// KindHolyThing is the castle holy artifact's type: a civilian NPC no
	// creature may attack or damage and a player's interact does nothing on,
	// shown with ServerObjectInfo, and the only target a HOLY skill accepts.
	KindHolyThing           InstanceKind = "HolyThing"
	KindLifeTower           InstanceKind = "LifeTower"
	KindManorManagerNpc     InstanceKind = "ManorManagerNpc"
	KindMercenaryManagerNpc InstanceKind = "MercenaryManagerNpc"
	KindMerchant            InstanceKind = "Merchant"
	KindMonster             InstanceKind = "Monster"
	KindMutedFolk           InstanceKind = "MutedFolk"
	KindOlympiadManagerNpc  InstanceKind = "OlympiadManagerNpc"
	KindPet                 InstanceKind = "Pet"
	KindRaidBoss            InstanceKind = "RaidBoss"
	KindSchemeBuffer        InstanceKind = "SchemeBuffer"
	KindServitor            InstanceKind = "Servitor"
	// KindSiegeFlag is a siege headquarters' type: a civilian NPC a clan leader
	// puts up for the clan during a siege. A player's interact does nothing on
	// it, no effect lands on it, it can be neither healed nor struck by a
	// lethal blow, and its stats are its template's own, with none of a
	// creature's built-in stat bonuses. A hit warns its clan the base is under
	// attack, at most once every headquartersWarnHold. Its clan holds it until
	// it dies, which takes it out of the world at once. The anti-infantry Swoop
	// Cannon siege summon neither hurts it nor sets off the warning.
	KindSiegeFlag            InstanceKind = "SiegeFlag"
	KindSiegeGuard           InstanceKind = "SiegeGuard"
	KindSiegeNpc             InstanceKind = "SiegeNpc"
	KindSiegeSummon          InstanceKind = "SiegeSummon"
	KindSignsPriest          InstanceKind = "SignsPriest"
	KindStaticObject         InstanceKind = "StaticObject"
	KindSymbolMaker          InstanceKind = "SymbolMaker"
	KindTamedBeast           InstanceKind = "TamedBeast"
	KindTrainer              InstanceKind = "Trainer"
	KindVillageMaster        InstanceKind = "VillageMaster"
	KindVillageMasterDElf    InstanceKind = "VillageMasterDElf"
	KindVillageMasterDwarf   InstanceKind = "VillageMasterDwarf"
	KindVillageMasterFighter InstanceKind = "VillageMasterFighter"
	KindVillageMasterMystic  InstanceKind = "VillageMasterMystic"
	KindVillageMasterOrc     InstanceKind = "VillageMasterOrc"
	KindVillageMasterPriest  InstanceKind = "VillageMasterPriest"
	KindWarehouseKeeper      InstanceKind = "WarehouseKeeper"
	// KindWeddingManagerNpc is the wedding manager's type.
	KindWeddingManagerNpc InstanceKind = "WeddingManagerNpc"
	KindWyvernManagerNpc  InstanceKind = "WyvernManagerNpc"
)
