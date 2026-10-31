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
- their base classes `L2GameServerPacket`, `commons/mmocore/{SendablePacket,AbstractPacket,MMOClient,MMOConnection}`
- the value types `model/location/{Location,SpawnLocation,Point2D}`, `enums/{SayType,ZoneId}`,
  `enums/actors/WeightPenalty`, `enums/skills/EffectFlag` and the rest of their closure.

Stand-ins (`stub/`, found first on the source path, so they shadow the reference
engine classes the packets only read through getters): `Config` (debug flag),
`GameClient`, `SystemMessageId` (numeric id only), `WorldObject`/`Creature`/`Boat`/`Player`
(plain value holders; `Creature` carries the title, run and water flags and the move
destination), `BoatInfo`, `BoatMove`, `BoatStatus`, `World`, `PlayerInfoTable`,
`RelationManager` (registries `FriendList` reads), `IntIntHolder` (the id/value pair
`StatusUpdate` holds, minus the skill lookup), `StatSet`, `MathUtil` (never reached)
and `EffectHolder` (never written). They carry no packet logic: every byte comes from
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

1. Add `PacketProbe.emit("Name/variant", new Packet(...))` to `PacketProbe.java`, `ProbeSimple.java`
   or `ProbeWorld.java`. `run.sh` compiles whatever the probe sources use through the source path,
   so there is no class list; extend `stub/` if a constructor reads another engine getter.
2. Regenerate (below).
3. Add the same-named case to `oracleSimpleCases` or `oracleWorldCases` in
   `../../packets_oracle_cases_test.go` (or `oracleCases` in `../../packets_oracle_test.go`).
4. Delete the hand-written byte test the case replaces.

Not yet probed (they read the engine item, player, clan, castle or skill graphs): `SystemMessage`,
`ConfirmDlg`, `CharSelectInfo`, `Die`, the manor windows, `UserInfo`/`CharInfo`/`NpcInfo`/`PetInfo`,
the item, shop, trade, private store, macro, henna and party windows, and the pledge windows that
take a clan.

## Regenerating

```sh
ACIS_JAVA=/path/to/aCis_gameserver/java internal/gameserver/network/serverpackets/testdata/oracle/run.sh
```

Needs docker with `eclipse-temurin:21-jdk-alpine`. Without `ACIS_JAVA` the script
looks for `aCis_gameserver/java` next to the Go checkout.
