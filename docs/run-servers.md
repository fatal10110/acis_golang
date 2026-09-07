# Running Loginserver And Gameserver

Run these commands from the repository root.

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
  -log-root .
```

Add `-debug-addr 127.0.0.1:6060` to serve the debug endpoints described under [Debug Endpoints](#debug-endpoints).

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
  -log-root .
```

Add `-debug-addr 127.0.0.1:6060` to serve the debug endpoints described under [Debug Endpoints](#debug-endpoints).

The gameserver loads the minimal XML tables, loads geodata from `geoengine.properties`, links to the loginserver, and binds the game-client listener from `GameserverHostname/GameserverPort`. Missing geodata region files use the null-region fallback; malformed region files that exist fail boot.

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
