# Server packet oracle

`../packets.golden` is the expected output of `TestPacketsMatchReferenceProbe`
(`../../packets_oracle_test.go`): one `name hex` line per packet, the hex being the
payload (opcode first, little-endian) that the reference packet's own `writeImpl`
wrote into a byte buffer.

`run.sh` compiles the following reference classes **unmodified** from
`aCis_gameserver/java/net/sf/l2j/` (through `javac -sourcepath stub:/ref`):

- every packet class `PacketProbe.java`, `ProbeSimple.java` and `ProbeWorld.java` construct,
  under `gameserver/network/serverpackets/`: the boat passenger and boat packets
  (`GetOnVehicle` ... `VehicleStarted`, `BoatSay`), chat and friend packets (`CreatureSay`,
  `L2Friend*`, `FriendList`, `EtcStatusUpdate`), the plain number, string and byte-array packets
  (targeting, rotation, shortcut, window, duel, party, pledge, variation, tutorial, board,
  screen message, sound, crest, skill info, camera, no-argument packets, `Ride`) and the packets
  that read a creature (`MoveToLocation`, `MoveToPawn`, `ValidateLocation`, `TeleportToLocation`,
  `SocialAction`, `DeleteObject`, `ChangeMoveType`, `ChangeWaitType`, `TargetUnselected`,
  `Revive`, `StopMove`, `TitleUpdate`, `Earthquake`, `MagicSkillUse`, `StatusUpdate`,
  `FlyToLocation`, `SetupGauge`)
- the packets that read the engine's item, clan, siege, castle, manor and character graphs
  (`ProbeEngine.java`): `SystemMessage`, `ConfirmDlg`, `Die`, `CharSelected`, `CharSelectInfo` and the
  manor windows (`ExShowSeedInfo`, `ExShowCropInfo`, `ExShowManorDefaultInfo`, `ExShowSeedSetting`,
  `ExShowCropSetting`, `ExShowSellCropList`, `ExShowProcureCropDetail`, `SellListProcure`,
  `BuyListSeed`, `ExSendManorList`)
- the item, shop, trade, private store, recipe shop, macro and symbol windows (`ProbeItems.java`,
  `ProbeShops.java`): `ItemList`, `InventoryUpdate`, `PetItemList`, `PetInventoryUpdate`,
  `GMViewItemList`, `SpawnItem`, `DropItem`, `GetItem`, `PackageToList`, `PackageSendableList`,
  `WarehouseDepositList`, `WarehouseWithdrawList`, `BuyList`, `SellList`, `MultiSellList`,
  `ShopPreviewList`, `ShopPreviewInfo`, `TradeStart`, `TradeOwnAdd`, `TradeOtherAdd`, `TradeUpdate`,
  `TradeItemUpdate`, the `PrivateStore*` lists and messages, the `RecipeShop*`, `RecipeBookItemList`
  and `RecipeItemMakeInfo` packets, `SendMacroList` and the `Henna*` windows
- the party, command channel and party-matching windows (`ProbeParty.java`): `PartySmallWindowAll`, `Add`,
  `Update` and `Delete`, `PartySpelled`, `PartyMemberPosition`, `PartyMatchList`, `PartyMatchDetail`,
  `ExPartyRoomMember`, `ExManagePartyRoomMember`, `ExListPartyMatchingWaitingRoom`, `ExMPCCShowPartyMemberInfo`,
  `ExMultiPartyCommandChannelInfo`, `ExMPCCPartyInfoUpdate`, `ExOpenMPCC`, `ExCloseMPCC` and `ExClosePartyRoom`
- their base classes `L2GameServerPacket`, `commons/mmocore/{SendablePacket,AbstractPacket,MMOClient,MMOConnection}`
- the value types `model/{Macro,trade/TradeItem,trade/ItemRequest,craft/ManufactureList,buylist/NpcBuyList,multisell/{Entry,ListContainer}}`, `model/records/{MacroCmd,ManufactureItem}`, `enums/items/{ItemState,CrystalType}`, `enums/actors/HennaType`, `model/item/instance/ItemInfo`, `model/location/{Location,SpawnLocation,Point2D}`, `model/manor/{Seed,SeedProduction,CropProcure}`,
  `model/records/CnfDlgData`, `model/CharSelectSlot`, `enums/{SayType,ZoneId,SiegeSide,Paperdoll}`,
  `enums/actors/{ClassId,ClassRace,Sex}`,
  `enums/actors/WeightPenalty`, `enums/skills/EffectFlag` and the rest of their closure.

Stand-ins (`stub/`, found first on the source path, so they shadow the reference
engine classes the packets only read through getters): `Config` (debug flag),
`GameClient`, `SystemMessageId` (numeric id only), `WorldObject`/`Creature`/`Boat`/`Player`
(plain value holders; `Creature` carries the title, run and water flags and the move
destination), `BoatInfo`, `BoatMove`, `BoatStatus`, `World`, `PlayerInfoTable`,
`RelationManager` (registries `FriendList` reads), `IntIntHolder` (the id/value pair
`StatusUpdate` holds, minus the skill lookup), `StatSet` (map-backed), `MathUtil` (never reached)
and `EffectHolder` (never written). The engine graphs behind the last family are plain stand-ins too:
`SystemMessageId` (id and parameter count; the shared no-parameter message is never cached),
`ItemInstance`/`Item`/`PcInventory`/`ItemData`, `Playable`, `Augmentation`, `Product`, `Ingredient`, `MultisellData`,
`TradeList`, `RecipeBook`, `Recipe`, `RecipeData`, `Henna`, `HennaList`, `HennaData`, `L2Skill`/`AbstractEffect` (never written), `Clan`,
`Siege`/`ClanHallSiege`/`Castle`, `CastleManager`/`ClanHallManager`/`ClanTable`, `Monster` with its
spoil state, the player's access level, appearance and status, `GameTimeTaskManager`,
`CastleManorManager` (the reference's own lookups over tables the probe fills), and for the character
list a `ConnectionPool` over `FakeDb`: in-memory tables answering the reference JDBC queries, so the
list's own loading rules (subclass override, unset base class, augmentation read) run unchanged. They carry no packet logic: every byte comes from
the reference `writeImpl`.

`PacketBytes` (in `stub/.../commons/mmocore`, the packet base package, to reach the
buffer slot) calls `writeImpl` directly by reflection into a little-endian buffer,
because the base class's `write()` swallows exceptions and would turn a failing probe
into silent short bytes; a packet that writes nothing fails the probe.

## Inputs

`ProbeSimple.ROWS` (ten ints per row: distinct bytes per field, negatives, zeros, int
limits), `STRS` (empty, ASCII, non-ASCII, 200 characters), `DOUBLES` and `BLOBS` are repeated
in `../../packets_oracle_cases_test.go` (`oracleRows`, `oracleStrings`, `oracleDoubles`,
`oracleBlobs`). A case is named `Packet/variant`; the Java constructor arguments and the Go call
arguments are the same values. `ProbeWorld` builds its creature from the first five fields of a
row, and the target of a skill cast from the next row.

## Adding a packet

1. Add `PacketProbe.emit("Name/variant", new Packet(...))` to `PacketProbe.java`, `ProbeSimple.java`,
   `ProbeWorld.java`, `ProbeEngine.java`, `ProbeCharacters.java`, `ProbeItems.java` or
   `ProbeShops.java`. `run.sh` compiles whatever the probe sources use through the source path,
   so there is no class list; extend `stub/` if a constructor reads another engine getter.
2. Regenerate (below).
3. Add the same-named case to `oracleSimpleCases` or `oracleWorldCases` in
   `../../packets_oracle_cases_test.go` (or `oracleCases` in `../../packets_oracle_test.go`).
4. Delete the hand-written byte test the case replaces.

The packets that describe a character, an npc or a summon (`ProbeCharacters.java`) are `UserInfo`,
`CharInfo`, the npc view `NpcInfo`, the summon view `SummonInfo`, `PetInfo`, `PetStatusUpdate` and
`ServerObjectInfo`. Their stand-ins are `Npc`, `Summon`, `Pet`, `Servitor`, `Cubic`, `NpcTemplate`,
`CreatureStatus` (the plain status values; the player's, the boat's and every npc's and summon's
extend or use it), `FishingStance` and `CursedWeaponManager`. Every field is a function of the shared
row; `variants()` then changes row "a" one thing at a time (flags, mount, cubics, invisibility, ...).
The Go cases are in `../../packets_oracle_character_test.go`.

The stat values the Go side computes from a template, the level and the buffs (attack, defense,
accuracy, speeds, multipliers, the maxima of a character with a template) are not free inputs: the
probe's status holds the numbers the Go calculators answer for the fixed inputs of the cases
(`ProbeCharacters.player` and `.live`), and `TestOracleCharacterInputsPinned` fails when a calculator
moves them. Those cases pin the packet layout and the plain fields; the calculators have their own
probes (`model/actor/{player,npc,summon}/testdata`).

Not probed there: the polymorphed views (`PcMorphInfo`, and the name and title `UserInfo` shows for a
polymorphed player), which have no Go counterpart; the rules that choose an npc's title, crest and
level label and a summon's owner-invisible hiding, which belong to the callers that build the snapshots;
and a summon that has not yet shown its summon animation. A player mounted with a summon out (the
summon's collision radius and height) cannot occur in the Go model.

The item and shop windows (`../../packets_oracle_item_test.go`, `../../packets_oracle_shop_test.go`) build
every field from the shared row, rotated by the item kind (`ProbeItems.item`): a worn augmented weapon,
a worn shadow armor, an augmented jewel, a quest stack and adena (plus a siege guard ticket for the
merchant lists). The inventory sets are linked sets in the stand-in, so their order is the insertion order
(the reference keeps them in an identity-hashed set). The selections a reference window makes from the
inventory before writing (what a trade or a warehouse deposit may list, which symbols a class may draw, which
crafts the recipe window shows) happen in the server before the Go encoder, so each Go case hands the encoder
the rows the stand-ins hand the reference.

The party, command channel and party-matching windows (`ProbeParty.java`, `../../packets_oracle_party_test.go`)
run over stand-ins for the party, room, channel, restart point table and room manager (`Party`, `PartyMatchRoom`,
`CommandChannel`, `RestartPointData`, `RestartPoint`, `PartyMatchRoomManager`; the player gains an active class and a
party). Like the shop windows, the members a window lists and the status and region of a room row are chosen before
the encoder on the Go side, so each Go case hands the encoder the rows the stand-ins hand the reference. The party
rows cast three doubles to int, so `ProbeParty.VITALS` adds NaN, infinity and values outside the int range; the
position list comes out in the order of a hash map keyed by object id, which the `PartyMemberPosition/ids*` cases
pin with ids that share a bucket, arrive out of order, and grow the map past its first resize.

Not yet probed (they read the engine clan, castle or skill graphs): the pledge windows that take a clan.

The manor set windows hold their seeds in a hash set and `SellListProcure` its rows in an
identity-hash map, so their probes use one seed per castle and one row; the other manor windows
iterate a map keyed by crop or castle id, whose order the golden pins.

## Regenerating

```sh
ACIS_JAVA=/path/to/aCis_gameserver/java internal/gameserver/network/serverpackets/testdata/oracle/run.sh
```

Needs docker with `eclipse-temurin:21-jdk-alpine`. Without `ACIS_JAVA` the script
looks for `aCis_gameserver/java` next to the Go checkout.
