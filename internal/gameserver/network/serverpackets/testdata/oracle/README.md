# Server packet oracle

`../packets.golden` is the expected output of `TestPacketsMatchReferenceProbe`
(`../../packets_oracle_test.go`): one `name hex` line per packet, the hex being the
payload (opcode first, little-endian) that the reference packet's own `writeImpl`
wrote into a byte buffer.

`run.sh` compiles the following reference classes **unmodified** from
`aCis_gameserver/java/net/sf/l2j/` (through `javac -sourcepath stub:/ref`):

- every packet class named in `run.sh` under `gameserver/network/serverpackets/`
  (`GetOnVehicle`, `GetOffVehicle`, `MoveToLocationInVehicle`, `StopMoveInVehicle`,
  `ValidateLocationInVehicle`, `VehicleInfo`, `OnVehicleCheckLocation`,
  `VehicleDeparture`, `VehicleStarted`, `BoatSay`, `CreatureSay`,
  `FriendAddRequestResult`, `FriendAddRequest`, `L2Friend`, `L2FriendStatus`,
  `L2FriendSay`, `FriendList`, `EtcStatusUpdate`)
- their base classes `L2GameServerPacket`, `commons/mmocore/{SendablePacket,AbstractPacket,MMOClient,MMOConnection}`
- the value types `model/location/{Location,SpawnLocation,Point2D}`, `enums/{SayType,ZoneId}`,
  `enums/actors/WeightPenalty`, `enums/skills/EffectFlag` and the rest of their closure.

Stand-ins (`stub/`, found first on the source path, so they shadow the reference
engine classes the packets only read through getters): `Config` (debug flag),
`GameClient`, `SystemMessageId` (numeric id only), `WorldObject`/`Creature`/`Boat`/`Player`
(plain value holders), `BoatInfo`, `BoatMove`, `BoatStatus`, `World`, `PlayerInfoTable`,
`RelationManager` (registries `FriendList` reads), `StatSet`, `MathUtil` (never reached)
and `EffectHolder` (never written). They carry no packet logic: every byte comes from
the reference `writeImpl`.

`PacketBytes` (in `stub/.../commons/mmocore`, the packet base package, to reach the
buffer slot) calls `writeImpl` directly by reflection into a little-endian buffer,
because the base class's `write()` swallows exceptions and would turn a failing probe
into silent short bytes; a packet that writes nothing fails the probe.

## Adding a packet

1. Add `emit("Name", new Packet(...))` to `PacketProbe.java` and list the class in `run.sh`
   (extend `stub/` if a constructor reads another engine getter).
2. Regenerate (below).
3. Add the same-named case to `oracleCases` in `../../packets_oracle_test.go`.

## Regenerating

```sh
ACIS_JAVA=/path/to/aCis_gameserver/java internal/gameserver/network/serverpackets/testdata/oracle/run.sh
```

Needs docker with `eclipse-temurin:21-jdk-alpine`. Without `ACIS_JAVA` the script
looks for `aCis_gameserver/java` next to the Go checkout.
