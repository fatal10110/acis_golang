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
- their base classes `L2GameServerPacket`, `commons/mmocore/{SendablePacket,AbstractPacket,MMOClient,MMOConnection}`
- the value types `model/location/{Location,SpawnLocation,Point2D}`, `model/manor/{Seed,SeedProduction,CropProcure}`,
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
`ItemInstance`/`Item`/`PcInventory`/`ItemData`, `L2Skill`/`AbstractEffect` (never written), `Clan`,
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
   `ProbeWorld.java`, `ProbeEngine.java` or `ProbeCharacters.java`. `run.sh` compiles whatever the probe sources use through the source path,
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

Not yet probed (they read the engine item, clan, castle or skill graphs): the item, shop, trade,
private store, macro, henna and party windows, and the pledge windows that take a clan.

The manor set windows hold their seeds in a hash set and `SellListProcure` its rows in an
identity-hash map, so their probes use one seed per castle and one row; the other manor windows
iterate a map keyed by crop or castle id, whose order the golden pins.

## Regenerating

```sh
ACIS_JAVA=/path/to/aCis_gameserver/java internal/gameserver/network/serverpackets/testdata/oracle/run.sh
```

Needs docker with `eclipse-temurin:21-jdk-alpine`. Without `ACIS_JAVA` the script
looks for `aCis_gameserver/java` next to the Go checkout.
