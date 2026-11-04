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
- the pledge windows (`ProbePledge.java`): `PledgeShowMemberListAll`, `Add` and `Update`, `PledgeInfo`,
  `PledgeStatusChanged`, `PledgeShowInfoUpdate`, `ManagePledgePower`, `PledgeReceiveMemberInfo`, `PowerInfo`,
  `WarList` and `SubPledgeCreated`, `PledgeSkillList` and `ListAdd`, `PledgeCrest`, `AllianceInfo`,
  `GMViewPledgeInfo`, `ExHeroList` and `ShowMiniMap`
- the skill, effect, attack, door and gauge packets (`ProbeSkills.java`): `AbnormalStatusUpdate`,
  `ExOlympiadSpelledInfo`, `SkillList`, `SkillCoolTime`, `MagicSkillLaunched`, `ExEnchantSkillList`,
  `AcquireSkillList` (usual, fishing and clan), `AcquireSkillDone`, `ShortCutInit` and `ShortCutRegister`,
  `Attack`, `DoorInfo`, `DoorStatusUpdate`, `StaticObjectInfo`, `QuestList`, `ExOlympiadUserInfo` and
  `ExDuelUpdateUserInfo`; and the `vitals` rows (the current gauges as NaN, infinities and values outside
  the int range) of those two, `UserInfo`, `CharInfo` and the recipe windows
- the debug drawing `ExServerPrimitive` and the zone outlines `model/zone/form/{ZoneCuboid,ZoneCylinder,ZoneNPoly}`
  draw into it (`ProbePrimitive.java`), sent through `sendTo` to a stub `Player` that records what it is
  sent. The packet keeps its strokes in identity-hashed sets; the probe swaps the sets of every packet it
  fills with more than one point or line for insertion-ordered ones (reflection), so the bytes follow the
  order the strokes were added in, the order the Go drawing writes.
- the siege windows (`ProbeSiege.java`): `SiegeInfo` of a castle and of a siegable hall, `SiegeAttackerList` and
  `SiegeDefenderList`
- the Seven Signs sky and record (`ProbeSevenSigns.java`): `SSQInfo` (its four sky packets and the sky it picks
  for a state) and every page of `SSQStatus`
- the game master's views and the raid point record (`ProbeGMView.java`): `GMViewCharacterInfo`, `GMViewSkillInfo`,
  `GMViewQuestList`, `GMViewWarehouseWithdrawList` and `ExGetBossRecord`
- the remaining small families (`ProbeMisc.java`): `NpcHtmlMessage` (and its page size limit), the community
  board pages as `BaseBBSManager.separateAndSend` and `send1001` cut them, `ExFishingStart`,
  `ExFishingStartCombat`, `ExFishingHpRegen`, `ExStorageMaxCount`, `EnchantResult`, `ExShowVariationMakeWindow`,
  `ExShowVariationCancelWindow`, `ExShowQuestInfo`, `ExConfirmCancelItem`, `Dice` (the landing point
  `SpawnLocation.addOffsetBasedOnHeading` moves), `ObserverStart`, `ObserverEnd`, `ClanHallDecoration`,
  `ExColosseumFenceInfo`, `NpcSay`, `PetStatusShow`, `SunRise`, `SunSet`, `AuthLoginFail`, `VersionCheck` and
  `NewCharacterSuccess`
- their base classes `L2GameServerPacket`, `commons/mmocore/{SendablePacket,AbstractPacket,MMOClient,MMOConnection}`
- the value types `model/{Macro,trade/TradeItem,trade/ItemRequest,craft/ManufactureList,buylist/NpcBuyList,multisell/{Entry,ListContainer}}`, `model/records/{MacroCmd,ManufactureItem}`, `enums/items/{ItemState,CrystalType}`, `enums/actors/HennaType`, `model/item/instance/ItemInfo`, `model/location/{Location,SpawnLocation,ObserverLocation,Point2D}`, `commons/math/MathUtil`, `communitybbs/manager/BaseBBSManager`, `enums/FailReason`, `model/manor/{Seed,SeedProduction,CropProcure}`,
  `model/records/CnfDlgData`, `model/CharSelectSlot`, `enums/{SayType,ZoneId,SiegeSide,Paperdoll}`,
  `enums/actors/{ClassId,ClassRace,Sex}`,
  `enums/actors/WeightPenalty`, `enums/skills/EffectFlag` and the rest of their closure.

Stand-ins (`stub/`, found first on the source path, so they shadow the reference
engine classes the packets only read through getters): `Config` (debug flag),
`GameClient`, `SystemMessageId` (numeric id only), `WorldObject`/`Creature`/`Boat`/`Player`
(plain value holders; `Creature` carries the title, run and water flags and the move
destination), `BoatInfo`, `BoatMove`, `BoatStatus`, `World`, `PlayerInfoTable`,
`RelationManager` (registries `FriendList` reads), `IntIntHolder` (the id/value pair
`StatusUpdate` holds, minus the skill lookup), `StatSet` (map-backed)
and `EffectHolder` (never written). The engine graphs behind the last family are plain stand-ins too:
`SystemMessageId` (id and parameter count; the shared no-parameter message is never cached),
`ItemInstance`/`Item`/`PcInventory`/`ItemData`, `Playable`, `Augmentation`, `Product`, `Ingredient`, `MultisellData`,
`TradeList`, `RecipeBook`, `Recipe`, `RecipeData`, `Henna`, `HennaList`, `HennaData`, `L2Skill`/`AbstractEffect` (never written), `Clan`,
`Siege`/`ClanHallSiege`/`Castle`, `Door`/`StaticObject`, `CreatureAttack` (only its hit record), `QuestList`/`Quest`/`QuestState`,
`ShortcutList`, `EtcItem`, `Timestamp` (the same record read against a fixed clock, so the time left is a
function of the stamp), the player's skill map (kept in the order the probe adds them), shortcut bar,
quest log and reuse timers, `CastleManager`/`ClanHallManager`/`ClanTable`, `Monster` with its
spoil state, the player's access level, appearance and status, `GameTimeTaskManager`,
`CastleManorManager` (the reference's own lookups over tables the probe fills), `HtmCache` (never reached: the
probe sets every page itself), `ClanHall` (id and decoration depth per function), `Fence`, `PlayerData` and
`PlayerTemplate` (the root class templates; the race is the class's own), and for the character
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
   `ProbeWorld.java`, `ProbeEngine.java`, `ProbeCharacters.java`, `ProbeItems.java`,
   `ProbeShops.java` or `ProbePolymorph.java`. `run.sh` compiles whatever the probe sources use through the source path,
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

The polymorphed views (`PcMorphInfo`, the name and title `UserInfo` shows for a polymorphed player,
and the `NpcInfo` of a polymorphed npc) are probed by `ProbePolymorph`, over the same player and npc rows.

Not probed there: the rules that choose an npc's title, crest and level label and a summon's
owner-invisible hiding, which belong to the callers that build the snapshots; and a summon that has
not yet shown its summon animation. A player mounted with a summon out (the
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

The pledge windows (`ProbePledge.java`, `../../packets_oracle_pledge_test.go`) run the reference roster entry
(`ClanMember`) over stand-ins for the clan, clan table, crest cache, hero manager and Seven Signs period (`Clan`,
`ClanTable`, `CrestCache`, `HeroManager`, `SevenSignsManager`, `GameClient`, `StatSet`, `Config`; the player gains
the title, power grade and mentor links). Which members a window lists, the names it shows for a sub-unit and its
captain, and the order of the clan, alliance and hero tables are chosen before the encoder on the Go side, so each
Go case hands the encoder the rows the stand-ins hand the reference, in the order the reference's hash tables list
them (the roster table order is also covered by the clan registry oracle). The rows pin the quirks of the
reference: an online member with object id 0 counts as offline in `PledgeShowMemberListUpdate`; an offline member
shows no sex and race there but its stored ones in the full roster; the game master's view marks only a sponsor
where the roster marks an apprentice too; a mentor outside the roster is named `Error`; a crest of any size but
256 bytes is no crest; `AllianceInfo` of alliance 0 lists no clan; a rank without a mask has the empty one.
The skill list probe uses ids that one table run lists in ascending order (the Go order differs for other ids: #4404).

`StartPledgeWar`, `StopPledgeWar` and `SurrenderPledgeWar` are not probed: nothing in the reference ever builds
them, and the Go side has no encoder for them.
`Snoop` is not probed for the same reason.
`ShowBoard.STATIC_CLOSE` (the board's close page) is not probed: the Go side has no encoder for it.

The remaining families (`ProbeMisc.java`, `../../packets_oracle_misc_test.go`) run over the shared rows. The html
dialog cases run each page through `setHtml`, whose limit counts UTF-16 code units (a page of 8192 units is kept,
one more unit is replaced, a wide character counts one unit and a surrogate pair two); the Go cases run the page
through `NpcHtmlBody`. The board page cases record what the board's send helpers send to a stub `Player`: a page is
cut at 4090 and 8180 code units (a surrogate pair straddling the cut is split), a page of 12270 units or more sends
nothing, and an edit page of 8180 units or more sends nothing; the Go cases take the frames `FramesShowBoardPage`
and `FrameShowBoardEdit` build, so the number of parts is pinned too. `Dice` throws from a stub `Player`'s position
through the reference `SpawnLocation` and `MathUtil` heading arithmetic; the Go cases move the same point with
`OrientedLocation.Ahead`. `ExStorageMaxCount` reads plain limits from the status stand-in; the Go cases build a
character whose configured bases and limit stats give the same values (the formula is pinned by
`TestFrameExStorageMaxCountFollowsConfigAndLimitStats`). `NewCharacterSuccess` lists the root class templates in
the reference order, each with its own base stats.

The siege windows (`ProbeSiege.java`, `../../packets_oracle_siege_test.go`) run over stand-ins for the castle (its
owner), the siege (its attacker, defender and pending lists, the side of each clan and the date), the siegable hall
(`SiegableHall`: owner, attackers, next siege time) and the clan table. Whether the owner is a known clan is decided
before the encoder on the Go side, so the Go cases hand it the view the server builds. `SiegeInfo` writes the current
time: the probe checks the field holds the second the packet was written in and prints it as 0, the value the Go
cases pass; the siege dates include one past the int range in seconds and a negative one.

The Seven Signs record (`ProbeSevenSigns.java`, `../../packets_oracle_sevensigns_test.go`) runs `SSQStatus` over a
`SevenSignsManager` stand-in holding the calendar, the stone and festival scores, the seal owners and votes and the
sign-ups; its total score, winning cabal and member count are the reference's own arithmetic, copied unchanged. The
states reach every branch of the seal prediction, every period and a cabal with no member (votes over no member
count against one). The Go cases restore the same state into `sevensigns.State` and encode its `Record`, so the
shares, the winner and the prediction come from the Go rules. The festival page runs over a
`FestivalOfDarknessManager` stand-in holding each cabal's best score and comma-joined party; the Go cases split the
party the way the server does (`festival.Score.MemberNames`), so the reference's split rules (a blank party names one
empty member, trailing empty names are dropped) are pinned.

The game master's views (`ProbeGMView.java`, `../../packets_oracle_gmview_test.go`) reuse the character, skill,
quest and item stand-ins of the families above; the weapon template is a `Weapon` stand-in carrying its shot counts.
The raid point record lists its bosses from an insertion-ordered map (the server orders them; the reference's own
hash order is pinned by `tests/party/testdata/raid_points.golden`).

The manor set windows hold their seeds in a hash set and `SellListProcure` its rows in an
identity-hash map, so their probes use one seed per castle and one row; the other manor windows
iterate a map keyed by crop or castle id, whose order the golden pins.

## Regenerating

```sh
ACIS_JAVA=/path/to/aCis_gameserver/java internal/gameserver/network/serverpackets/testdata/oracle/run.sh
```

Needs docker with `eclipse-temurin:21-jdk-alpine`. Without `ACIS_JAVA` the script
looks for `aCis_gameserver/java` next to the Go checkout.
