# Running Loginserver And Gameserver

Run these commands from `/Users/arturkoshtei/workspace/acis_public/acis_golang`.

## Prerequisites

- MariaDB is running and the `acis` schema has been loaded from:

```bash
mysql -uroot acis < ../aCis_datapack/tools/full_install.sql
```

- `../aCis_gameserver/config/loginserver.properties` and `../aCis_gameserver/config/server.properties` point at that database with the `URL`, `Login`, and `Password` keys.
- `server.properties` has `LoginHost = 127.0.0.1` and `LoginPort = 9014`, matching `loginserver.properties`.

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

The loginserver binds the client listener from `LoginserverHostname/LoginserverPort` and the gameserver-link listener from `LoginHostname/LoginPort`.

## Start Gameserver

In a second terminal:

```bash
go run ./cmd/gameserver \
  -config ../aCis_gameserver/config/server.properties \
  -logging ../aCis_gameserver/config/logging.properties \
  -hexid ../aCis_gameserver/config/hexid.txt \
  -data-root ../aCis_datapack \
  -log-root .
```

The gameserver loads the minimal XML tables, links to the loginserver, and binds the game-client listener from `GameserverHostname/GameserverPort`.

## Current Limit

This boot wiring starts the processes, database pool, data loaders, TCP listeners, and GS-LS link. The login-client and game-client packet dispatchers are still not wired, so a real Interlude client smoke test will connect but cannot complete login or enter world until those dispatchers are added.
