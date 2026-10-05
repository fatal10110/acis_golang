# Running Loginserver And Gameserver

Run these commands from the repository root. To run everything in containers instead, see
[ops/docker.md](ops/docker.md). To move an existing aCis Java installation, see
[ops/migrating-from-java.md](ops/migrating-from-java.md). For backups, see
[ops/backup-restore.md](ops/backup-restore.md).

## Prerequisites

- MariaDB is running and the `acis` schema has been loaded from:

```bash
mysql -uroot acis < ../aCis_datapack/tools/full_install.sql
for f in ../aCis_datapack/sql/*.sql; do mysql -uroot acis < "$f"; done
```

- `../aCis_gameserver/config/loginserver.properties` and `../aCis_gameserver/config/server.properties` point at that database with the `URL`, `Login`, and `Password` keys.
- `server.properties` has `LoginHost = 127.0.0.1` and `LoginPort = 9014`, matching `loginserver.properties`.
- Optional: `MaxConnections` in `loginserver.properties` / `server.properties` sets that process's database pool size. The key is absent from the shipped files; omitting it keeps the current pool size of 8. Values below 1 are rejected at boot.
- XML datapack files stay on disk under `../aCis_datapack`; do not import them into the database.
- Geodata files stay on disk under `../aCis_datapack/data/geodata`. The shipped `GeoDataPath = ./data/geodata/` is resolved against `-data-root`, so L2OFF files should be named like `16_10_conv.dat` and L2J files like `16_10.l2j`.

## Build

```bash
go build ./cmd/loginserver ./cmd/gameserver ./cmd/gsregister
```

## Register A Gameserver ID

```bash
go run ./cmd/gsregister \
  -config ../aCis_gameserver/config/loginserver.properties \
  -names ../aCis_datapack/data/serverNames.xml
```

Choose server ID `1` unless the database already reserves another ID. Then install the generated hexid file:

```bash
mv 'hexid(server 1).txt' ../aCis_gameserver/config/hexid.txt
```

## Start Loginserver

```bash
go run ./cmd/loginserver \
  -config ../aCis_gameserver/config/loginserver.properties \
  -logging ../aCis_gameserver/config/logging.properties \
  -server-names ../aCis_datapack/data/serverNames.xml \
  -banned-ips ../aCis_gameserver/config/banned_ips.properties \
  -log-root log/login
```

Add `-debug-addr 127.0.0.1:6061` to serve the debug endpoints described under [Debug Endpoints](#debug-endpoints).

The loginserver binds the client listener from `LoginserverHostname/LoginserverPort` and the gameserver-link listener from `LoginHostname/LoginPort`.

## Start Gameserver

In a second terminal:

```bash
go run ./cmd/gameserver \
  -config ../aCis_gameserver/config/server.properties \
  -logging ../aCis_gameserver/config/logging.properties \
  -hexid ../aCis_gameserver/config/hexid.txt \
  -geo-config ../aCis_gameserver/config/geoengine.properties \
  -data-root ../aCis_datapack \
  -log-root log/gameserver
```

Add `-debug-addr 127.0.0.1:6060` to serve the debug endpoints described under [Debug Endpoints](#debug-endpoints).

Give each process its own `-log-root` and its own `-debug-addr` port. Both binaries open the same relative log paths from `logging.properties`. At boot each one truncates the console and error files, and it appends to the chat, gmaudit and item files and rotates them on its own size counter. A shared root therefore makes the two processes wipe and interleave each other's files, and a second listener on a port already in use fails boot. Log files, their JSON format, and every logging and metrics failure mode are described in [Logs, Metrics, And Failure Modes](observability.md).

The gameserver loads the minimal XML tables, loads geodata from `geoengine.properties`, links to the loginserver, and binds the game-client listener from `GameserverHostname/GameserverPort`. It loads exactly the regions listed as `X_Y` keys in `geoengine.properties`; unlisted regions use the null-region fallback even when their file exists, and a listed region whose file is missing, unreadable, or malformed fails boot with an error naming each failed file, `GeoDataPath`, and `GeoDataType`.

## Debug Endpoints

Both binaries accept an optional `-debug-addr host:port`. When set, that address serves `net/http/pprof` and `expvar`:

```bash
go run ./cmd/gameserver -debug-addr 127.0.0.1:6060 ...
curl -s http://127.0.0.1:6060/debug/vars
curl -s 'http://127.0.0.1:6060/debug/pprof/goroutine?debug=1'
```

`/debug/vars` publishes `connections` (live accepted connections, both processes), `players-online` (the gameserver's live player count; the loginserver publishes the key but always reports 0), and `tickers`, a map of `{"last":ns,"max":ns}` per scheduler ticker name.

**Bind this to loopback only.** The handlers are registered on `http.DefaultServeMux` with no authentication, and `net/http/pprof` exposes heap dumps, goroutine stacks, and `/debug/pprof/cmdline`. Never bind `-debug-addr` to `0.0.0.0` or a public interface. Omit the flag entirely to leave the listener off.

## Smoke Checks

- Loginserver should log both listeners: one for game clients from `LoginserverHostname/LoginserverPort`, and one for game servers from `LoginHostname/LoginPort`.
- Gameserver should link to the loginserver and then log its game-client listener from `GameserverHostname/GameserverPort`.
- With `AutoCreateAccounts = True` or the key omitted, a fresh client login creates the account and reaches the server list.
- After selecting the linked gameserver, the client can create/select a character and enter the empty world.
